package importer

import (
	"net/url"
	"strings"

	"github.com/uniproxy/uniproxy/internal/model"
)

// parseURIForPreview applies only preview-time normalizations. It never changes
// the strict parser and never returns a node for application: callers must keep
// the result in ParseResult.Degraded.
func parseURIForPreview(text string) (model.ParsedNode, []string, bool) {
	if !isURILine(text) {
		return model.ParsedNode{}, nil, false
	}
	u, err := url.Parse(text)
	if err != nil {
		return model.ParsedNode{}, nil, false
	}
	values, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return model.ParsedNode{}, nil, false
	}
	reasons := []string{}
	changed := false
	add := func(reason string) {
		changed = true
		for _, existing := range reasons {
			if existing == reason {
				return
			}
		}
		reasons = append(reasons, reason)
	}

	// Empty query values are exporter noise; they carry no option value. Drop
	// them before the strict parser's unknown-key check runs.
	for key, list := range values {
		if len(list) == 0 || strings.TrimSpace(list[0]) == "" {
			delete(values, key)
			add("空查询参数已忽略")
		}
	}

	// security=false is a common non-standard spelling of security=none.
	security, hasSecurity := previewValue(values, "security")
	security = strings.ToLower(strings.TrimSpace(security))
	if hasSecurity && security == "false" {
		previewSet(values, "security", "none")
		previewDelete(values, "tls")
		security = "none"
		add("security=false 按 none 处理")
	}

	// packetEncoding=none is an Xray exporter default, not a core encoding.
	if value, ok := previewValue(values, "packetEncoding", "packet-encoding"); ok && strings.EqualFold(strings.TrimSpace(value), "none") {
		previewDelete(values, "packetEncoding", "packet-encoding")
		add("packetEncoding=none 已忽略")
	}

	// type=none is emitted by some exporters for a plain TCP node.
	if value, ok := previewValue(values, "type", "network"); ok && strings.EqualFold(strings.TrimSpace(value), "none") {
		previewDelete(values, "type", "network")
		previewSet(values, "type", "tcp")
		add("type=none 按 tcp 处理")
	}
	if _, hasType := previewValue(values, "type"); hasType {
		if _, hasNetwork := previewValue(values, "network"); hasNetwork {
			previewDelete(values, "network")
			add("type/network 重复声明取 type")
		}
	}

	if value, ok := previewValue(values, "extra"); ok && strings.EqualFold(strings.TrimSpace(value), "null") {
		previewDelete(values, "extra")
		add("extra=null 已忽略")
	}

	// Empty and unsafe fingerprints are common exporter placeholders. The
	// preview drops them rather than inventing a TLS identity.
	if value, ok := previewValue(values, "fp", "client-fingerprint"); ok {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			previewDelete(values, "fp", "client-fingerprint")
			add("空 fp 已忽略")
		} else if strings.EqualFold(trimmed, "unsafe") {
			previewDelete(values, "fp", "client-fingerprint")
			add("fp=unsafe 仅预览，固定内核不支持该指纹")
		}
	}

	// Keep one representation when exporters repeat the same skip-verify flag.
	if previewCount(values, "allowInsecure", "allowinsecure", "insecure", "skip-cert-verify") > 1 {
		previewKeepOne(values, "allowInsecure", "allowinsecure", "insecure", "skip-cert-verify")
		add("skip-cert-verify 重复声明已归一")
	}

	// A double-encoded ALPN list is common in nested subscription exporters.
	if value, ok := previewValue(values, "alpn"); ok {
		if decoded, err := url.QueryUnescape(value); err == nil && decoded != value {
			previewSet(values, "alpn", decoded)
			add("alpn 二次编码已归一")
		}
	}

	// A WebSocket path without a leading slash is a lossy but common form.
	if value, ok := previewValue(values, "path"); ok && value != "" && !strings.HasPrefix(value, "/") {
		previewSet(values, "path", "/"+value)
		add("WebSocket path 缺少前导 /，仅预览修正")
	}

	tlsEnabled := false
	if value, ok := previewValue(values, "tls"); ok {
		tlsEnabled = strings.EqualFold(strings.TrimSpace(value), "true") || strings.TrimSpace(value) == "1"
	}
	plainTLS := hasSecurity && (security == "none" || security == "")
	if !hasSecurity && !tlsEnabled {
		scheme := strings.ToLower(u.Scheme)
		plainTLS = scheme == "vless" || scheme == "vmess"
	}
	if plainTLS {
		// With TLS disabled these are placeholders, not active security. Removing
		// them lets the preview describe the node without claiming TLS.
		for _, key := range []string{"sni", "servername", "peer", "serverName", "fp", "client-fingerprint", "alpn", "allowInsecure", "allowinsecure", "insecure", "skip-cert-verify", "name-cert-verify", "fingerprint", "ech", "pbk", "public-key", "sid", "short-id", "flow"} {
			if previewDelete(values, key) {
				add("security=none 下的 TLS 占位字段已忽略")
			}
		}
	}
	if security == "reality" && previewCount(values, "allowInsecure", "allowinsecure", "insecure", "skip-cert-verify") > 0 {
		previewDelete(values, "allowInsecure", "allowinsecure", "insecure", "skip-cert-verify")
		add("Reality 不能关闭证书校验，仅预览忽略")
	}
	if security == "reality" {
		if previewDelete(values, "alpn") {
			add("Reality 自定义 ALPN 仅预览忽略")
		}
	}

	// path/host are common exporter placeholders on plain TCP. Only drop them
	// when the transport is TCP and no HTTP header type is requested.
	network := "tcp"
	if value, ok := previewValue(values, "type", "network"); ok {
		network = strings.ToLower(strings.TrimSpace(value))
	}
	if network == "raw" || network == "none" || network == "" {
		network = "tcp"
	}
	if network == "tcp" {
		header, hasHeader := previewValue(values, "headerType")
		if !hasHeader || header == "" || strings.EqualFold(header, "none") {
			if previewDelete(values, "headerType") {
				add("TCP 传输的 headerType 占位已忽略")
			}
			if previewDelete(values, "path") {
				add("TCP 传输的 path 占位已忽略")
			}
			if previewDelete(values, "host") {
				add("TCP 传输的 host 占位已忽略")
			}
		}
	}

	// These are exporter-only parameters that have no URI meaning in the pinned
	// core. Dropping them is preview-only and keeps the row visible for diagnosis.
	for _, key := range []string{"u", "quicSecurity"} {
		if previewDelete(values, key) {
			add("未知导出器参数 " + key + " 仅预览忽略")
		}
	}

	// Xray-only Reality/finalmask fields have no equivalent in the pinned core.
	if value, ok := previewValue(values, "spx"); ok && value != "" && value != "/" {
		previewDelete(values, "spx")
		add("Reality spider-X 仅预览，固定内核无等价字段")
	}
	if value, ok := previewValue(values, "pqv"); ok && value != "" {
		previewDelete(values, "pqv")
		add("Reality ML-DSA-65 校验仅预览，固定内核无等价字段")
	}
	if value, ok := previewValue(values, "fm"); ok && value != "" {
		previewDelete(values, "fm")
		add("Xray finalmask 仅预览，固定内核无等价字段")
	}
	if value, ok := previewValue(values, "cs"); ok && value != "" {
		previewDelete(values, "cs")
		add("Xray finalmask 子字段仅预览，固定内核无等价字段")
	}

	// An unparsable XHTTP extra object can still be useful as a preview, but it
	// cannot become a native outbound.
	if value, ok := previewValue(values, "extra"); ok && value != "" {
		if _, err := parseXHTTPExtra(value); err != nil {
			previewDelete(values, "extra")
			add("XHTTP extra 无效，仅预览忽略")
		}
	}

	// Invalid VLESS encryption is a real unsupported feature; retain a preview
	// row but do not pass the malformed value into the native map.
	if value, ok := previewValue(values, "encryption"); ok {
		if value != "" && value != "none" && !validVLESSEncryption(value) {
			previewDelete(values, "encryption")
			add("VLESS Encryption 参数仅预览，固定内核无法应用")
		}
	}

	// gRPC authority and non-standard mode are informational only.
	if value, ok := previewValue(values, "authority"); ok && value != "" {
		previewDelete(values, "authority")
		add("gRPC authority 仅预览忽略")
	}
	if value, ok := previewValue(values, "mode"); ok && value != "" && !strings.EqualFold(strings.TrimSpace(value), "gun") {
		previewDelete(values, "mode")
		add("gRPC mode 仅预览忽略")
	}

	if !changed {
		return model.ParsedNode{}, nil, false
	}
	u.RawQuery = values.Encode()
	node, err := parseURI(u.String())
	if err != nil {
		return model.ParsedNode{}, nil, false
	}
	return node, reasons, true
}

func previewValue(values url.Values, keys ...string) (string, bool) {
	for _, key := range keys {
		if list, ok := values[key]; ok && len(list) > 0 {
			return list[0], true
		}
	}
	for key, list := range values {
		for _, wanted := range keys {
			if strings.EqualFold(key, wanted) && len(list) > 0 {
				return list[0], true
			}
		}
	}
	return "", false
}

func previewDelete(values url.Values, keys ...string) bool {
	deleted := false
	for key := range values {
		for _, wanted := range keys {
			if strings.EqualFold(key, wanted) {
				delete(values, key)
				deleted = true
				break
			}
		}
	}
	return deleted
}

func previewSet(values url.Values, key, value string) {
	previewDelete(values, key)
	values.Set(key, value)
}

func previewCount(values url.Values, keys ...string) int {
	count := 0
	for key, list := range values {
		for _, wanted := range keys {
			if strings.EqualFold(key, wanted) && len(list) > 0 {
				count++
				break
			}
		}
	}
	return count
}

func previewKeepOne(values url.Values, keys ...string) {
	kept := false
	for key := range values {
		for _, wanted := range keys {
			if !strings.EqualFold(key, wanted) {
				continue
			}
			if kept {
				delete(values, key)
			} else {
				kept = true
			}
			break
		}
	}
}
