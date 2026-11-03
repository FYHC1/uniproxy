package importer

import (
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
)

// readVLESSEncryption validates the Xray VLESS Encryption secret carried in a
// URI. The pinned Mihomo core accepts this field directly, but its parser is
// strict about the four-part prefix and about at least one key block. Doing the
// same checks here prevents a value from being accepted by import and then
// silently producing a core that cannot start.
func readVLESSEncryption(r *fields, out map[string]any) {
	if !r.has("encryption") {
		return
	}
	value := r.text("encryption", false)
	if value == "" || value == "none" {
		return
	}
	if !validVLESSEncryption(value) {
		r.fail("unsupported VLESS encryption")
		return
	}
	out["encryption"] = value
}

func validVLESSEncryption(s string) bool {
	if len(s) > 65536 || containsControl(s) {
		return false
	}
	parts := strings.Split(s, ".")
	if len(parts) < 4 || parts[0] != "mlkem768x25519plus" {
		return false
	}
	if !oneOf(parts[1], "native", "xorpub", "random") || !oneOf(parts[2], "1rtt", "0rtt") {
		return false
	}
	keys, paddings := 0, []string{}
	for _, part := range parts[3:] {
		if len(part) < 20 {
			paddings = append(paddings, part)
			continue
		}
		b, err := base64.RawURLEncoding.Strict().DecodeString(part)
		if err != nil || len(b) != 32 && len(b) != 1088 && len(b) != 1184 {
			return false
		}
		keys++
	}
	if keys == 0 {
		return false
	}
	if len(paddings) == 0 {
		return true
	}
	padding := strings.Join(paddings, ".")
	segments := strings.Split(padding, ".")
	if len(segments) == 0 {
		return false
	}
	for i, segment := range segments {
		fields := strings.Split(segment, "-")
		if len(fields) < 3 {
			return false
		}
		values := make([]int, 3)
		for j := range values {
			if fields[j] == "" {
				return false
			}
			n, err := strconv.Atoi(fields[j])
			if err != nil || n < 0 {
				return false
			}
			values[j] = n
		}
		if i == 0 && (values[0] < 100 || values[1] < 35 || values[2] < 35) {
			return false
		}
		if values[1] > 65553 || values[2] > 65553 {
			return false
		}
	}
	return true
}

// readECHOptions accepts only the ECH fields the pinned core understands.
// Keeping enable=false with config/query absent is canonicalised away so old
// disabled placeholders do not change the node fingerprint.
func readECHOptions(r *fields, out map[string]any, protocol string) {
	if !r.has("ech-opts") {
		return
	}
	if !oneOf(protocol, "vless", "vmess") {
		r.fail("ECH is unsupported for this protocol")
		return
	}
	options, err := readECHMap(r.object("ech-opts"))
	if err != nil {
		r.fail("unsupported or invalid ECH options")
		return
	}
	if options != nil {
		out["ech-opts"] = options
	}
}

func readECHMap(raw map[string]any) (map[string]any, error) {
	f := newFields(raw)
	enable := f.boolean("enable", false)
	config := f.text("config", false)
	query := f.text("query-server-name", false)
	if !enable {
		if config != "" || query != "" {
			return nil, errors.New("ECH options require enable")
		}
		if err := f.finish(); err != nil {
			return nil, err
		}
		return nil, nil
	}
	if config != "" {
		if _, err := base64.StdEncoding.Strict().DecodeString(config); err != nil {
			return nil, errors.New("invalid ECH config")
		}
	}
	if query != "" {
		if _, ok := validHost(query); !ok {
			return nil, errors.New("invalid ECH query server name")
		}
	}
	if err := f.finish(); err != nil {
		return nil, err
	}
	options := map[string]any{"enable": true}
	if config != "" {
		options["config"] = config
	}
	if query != "" {
		options["query-server-name"] = query
	}
	return options, nil
}

func readKCPOptions(r *fields, out map[string]any) {
	if !r.has("kcp-opts") {
		return
	}
	if out["network"] != "kcp" {
		r.fail("KCP options require the KCP transport")
		return
	}
	f := newFields(r.object("kcp-opts"))
	options := map[string]any{}
	if f.has("header") {
		header := f.text("header", false)
		if header != "" {
			if !oneOf(header, "none", "srtp", "utp", "wechat-video", "dtls", "wireguard") {
				f.fail("unsupported KCP header")
			} else {
				options["header"] = header
			}
		}
	}
	if f.has("seed") {
		seed := f.text("seed", false)
		if containsControl(seed) || len(seed) > 8192 {
			f.fail("invalid KCP seed")
		} else if seed != "" {
			options["seed"] = seed
		}
	}
	copyInt(f, options, "mtu", 576, 1500)
	copyInt(f, options, "tti", 10, 60000)
	copyInt(f, options, "uplink-capacity", 0, 2147483647)
	copyInt(f, options, "downlink-capacity", 0, 2147483647)
	copyBool(f, options, "congestion")
	copyInt(f, options, "write-buffer", 0, 2147483647)
	copyInt(f, options, "read-buffer", 0, 2147483647)
	if err := f.finish(); err != nil {
		r.fail("unsupported or invalid KCP options")
		return
	}
	out["kcp-opts"] = options
}

func readXHTTPOptions(r *fields, out map[string]any) {
	if !r.has("xhttp-opts") {
		return
	}
	options, err := readXHTTPMap(r.object("xhttp-opts"))
	if err != nil {
		r.fail(err.Error())
		return
	}
	out["xhttp-opts"] = options
}

func readXHTTPMap(raw map[string]any) (map[string]any, error) {
	f := newFields(raw)
	options := map[string]any{}
	path := f.text("path", false)
	if path == "" {
		path = "/"
	}
	if !strings.HasPrefix(path, "/") || containsControl(path) || len(path) > 8192 {
		f.fail("invalid XHTTP path")
	}
	options["path"] = path
	if f.has("host") {
		host := f.text("host", false)
		if host != "" {
			if _, ok := validHost(host); !ok {
				f.fail("invalid XHTTP host")
			}
			options["host"] = host
		}
	}
	if f.has("mode") {
		mode := f.text("mode", false)
		if !oneOf(mode, "", "auto", "packet-up", "stream-up", "stream-one") {
			f.fail("unsupported XHTTP mode")
		} else if mode != "" {
			options["mode"] = mode
		}
	}
	if f.has("headers") {
		headers, err := cleanHeaders(f.object("headers"), false)
		if err != nil {
			f.fail(err.Error())
		} else if _, hasHost := headers["Host"]; hasHost {
			f.fail("XHTTP headers must not include Host")
		} else {
			options["headers"] = headers
		}
	}
	copyBool(f, options, "no-grpc-header")
	for _, key := range []string{
		"x-padding-key", "x-padding-header", "x-padding-placement", "x-padding-method",
		"uplink-http-method", "session-placement", "session-key", "session-table",
		"seq-placement", "seq-key", "uplink-data-placement", "uplink-data-key",
	} {
		if f.has(key) {
			value := f.text(key, false)
			if containsControl(value) || len(value) > 8192 {
				f.fail("invalid XHTTP option")
			} else if value != "" {
				options[key] = value
			}
		}
	}
	for _, key := range []string{
		"x-padding-bytes", "session-length", "uplink-chunk-size",
		"sc-max-each-post-bytes", "sc-min-posts-interval-ms",
	} {
		if f.has(key) {
			value, ok := xhttpRange(fieldValue(f, key))
			if !ok {
				f.fail("invalid XHTTP range option")
			} else {
				options[key] = value
			}
		}
	}
	if f.has("x-padding-obfs-mode") {
		options["x-padding-obfs-mode"] = f.boolean("x-padding-obfs-mode", false)
	}
	if f.has("reuse-settings") {
		reuse, err := readXHTTPReuseMap(f.object("reuse-settings"))
		if err != nil {
			f.fail(err.Error())
		} else {
			options["reuse-settings"] = reuse
		}
	}
	if f.has("download-settings") {
		download, err := readXHTTPDownloadMap(f.object("download-settings"))
		if err != nil {
			f.fail(err.Error())
		} else {
			options["download-settings"] = download
		}
	}
	if err := f.finish(); err != nil {
		return nil, errors.New("unsupported or invalid XHTTP options")
	}
	if options["mode"] == "stream-one" && options["download-settings"] != nil {
		return nil, errors.New("XHTTP stream-one cannot use download settings")
	}
	return options, nil
}

func readXHTTPReuseMap(raw map[string]any) (map[string]any, error) {
	f := newFields(raw)
	options := map[string]any{}
	for _, key := range []string{
		"max-concurrency", "max-connections", "c-max-reuse-times",
		"h-max-request-times", "h-max-reusable-secs",
	} {
		if f.has(key) {
			value, ok := xhttpRange(fieldValue(f, key))
			if !ok {
				f.fail("invalid XHTTP reuse range")
			} else {
				options[key] = value
			}
		}
	}
	copyInt(f, options, "h-keep-alive-period", 0, 2147483647)
	if err := f.finish(); err != nil {
		return nil, errors.New("unsupported or invalid XHTTP reuse settings")
	}
	return options, nil
}

func readXHTTPDownloadMap(raw map[string]any) (map[string]any, error) {
	f := newFields(raw)
	options := map[string]any{}
	if f.has("path") {
		path := f.text("path", false)
		if path != "" {
			if !strings.HasPrefix(path, "/") || containsControl(path) || len(path) > 8192 {
				f.fail("invalid XHTTP download path")
			}
			options["path"] = path
		}
	}
	if f.has("host") {
		host := f.text("host", false)
		if host != "" {
			if _, ok := validHost(host); !ok {
				f.fail("invalid XHTTP download host")
			}
			options["host"] = host
		}
	}
	if f.has("headers") {
		headers, err := cleanHeaders(f.object("headers"), false)
		if err != nil {
			f.fail(err.Error())
		} else if _, hasHost := headers["Host"]; hasHost {
			f.fail("XHTTP download headers must not include Host")
		} else {
			options["headers"] = headers
		}
	}
	if f.has("reuse-settings") {
		reuse, err := readXHTTPReuseMap(f.object("reuse-settings"))
		if err != nil {
			f.fail(err.Error())
		} else {
			options["reuse-settings"] = reuse
		}
	}
	if f.has("server") {
		server := f.text("server", true)
		if _, ok := validHost(server); !ok {
			f.fail("invalid XHTTP download server")
		}
		options["server"] = server
	}
	copyInt(f, options, "port", 1, 65535)
	tls := false
	if f.has("tls") {
		tls = f.boolean("tls", false)
		options["tls"] = tls
	}
	if f.has("alpn") {
		values := f.strings("alpn")
		for _, v := range values {
			if len(v) > 255 {
				f.fail("invalid XHTTP download ALPN")
			}
		}
		options["alpn"] = values
	}
	if f.has("skip-cert-verify") {
		options["skip-cert-verify"] = f.boolean("skip-cert-verify", false)
	}
	if f.has("name-cert-verify") {
		value := f.text("name-cert-verify", true)
		if _, ok := validHost(value); !ok || !tls {
			f.fail("invalid XHTTP download certificate verification name")
		}
		options["name-cert-verify"] = value
	}
	if f.has("fingerprint") {
		value, ok := normalizePin(f.text("fingerprint", true))
		if !ok || !tls {
			f.fail("invalid XHTTP download certificate fingerprint")
		}
		options["fingerprint"] = value
	}
	if f.has("servername") {
		value := f.text("servername", true)
		if _, ok := validHost(value); !ok || !tls {
			f.fail("invalid XHTTP download server name")
		}
		options["servername"] = value
	}
	if f.has("client-fingerprint") {
		value := f.text("client-fingerprint", true)
		if !oneOf(value, "chrome", "firefox", "safari", "iOS", "android", "edge", "360", "qq", "random", "randomized") {
			f.fail("unsupported XHTTP download client fingerprint")
		}
		options["client-fingerprint"] = value
	}
	if f.has("ech-opts") {
		ech, err := readECHMap(f.object("ech-opts"))
		if err != nil {
			f.fail("invalid XHTTP download ECH options")
		} else if ech != nil {
			if !tls {
				f.fail("XHTTP download ECH requires TLS")
			} else {
				options["ech-opts"] = ech
			}
		}
	}
	if f.has("reality-opts") {
		reality, err := readRealityMap(f.object("reality-opts"))
		if err != nil {
			f.fail(err.Error())
		} else if len(reality) != 0 && !tls {
			f.fail("XHTTP download Reality requires TLS")
		} else if tls || len(reality) != 0 {
			options["reality-opts"] = reality
		}
	}
	if err := f.finish(); err != nil {
		return nil, errors.New("unsupported or invalid XHTTP download settings")
	}
	return options, nil
}

func readRealityMap(raw map[string]any) (map[string]any, error) {
	if len(raw) == 0 {
		return map[string]any{}, nil
	}
	f := newFields(raw)
	key := f.text("public-key", true)
	sid := f.text("short-id", false)
	if !validPublicKey(key) || !validShortID(sid) {
		return nil, errors.New("invalid Reality public key or short ID")
	}
	if err := f.finish(); err != nil {
		return nil, errors.New("unsupported or invalid Reality options")
	}
	return map[string]any{"public-key": strings.TrimRight(key, "="), "short-id": strings.ToLower(sid)}, nil
}

// parseXHTTPExtra parses the Xray XHTTP extra object with the same strict YAML
// document rules used by native configuration import.
func parseXHTTPExtra(text string) (map[string]any, error) {
	root, err := documentNode(text)
	if err != nil {
		return nil, err
	}
	value, err := nodeValue(root)
	if err != nil {
		return nil, err
	}
	obj, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("XHTTP extra must be an object")
	}
	return obj, nil
}

func readKCPURI(q *query, out map[string]any) {
	options := map[string]any{}
	if header, ok := q.take("headerType", "header"); ok {
		if header != "" && !oneOf(header, "none", "srtp", "utp", "wechat-video", "dtls", "wireguard") {
			q.fail("unsupported KCP header")
		} else if header != "" {
			options["header"] = header
		}
	}
	if seed, ok := q.take("seed"); ok {
		if containsControl(seed) || len(seed) > 8192 {
			q.fail("invalid KCP seed")
		} else if seed != "" {
			options["seed"] = seed
		}
	} else if seed, ok := q.take("path"); ok {
		if containsControl(seed) || len(seed) > 8192 {
			q.fail("invalid KCP seed")
		} else if seed != "" {
			options["seed"] = seed
		}
	}
	if host, ok := q.take("host"); ok && host != "" {
		q.fail("KCP transport does not support a host option")
	}
	if value, ok := q.take("mtu"); ok {
		mtu, valid := integerValue(value)
		if !valid || mtu < 576 || mtu > 1500 {
			q.fail("invalid KCP MTU")
		} else {
			options["mtu"] = mtu
		}
	}
	out["kcp-opts"] = options
}

func fieldValue(f *fields, key string) any {
	value, _ := f.get(key)
	return value
}

func xhttpRangePositive(value any) (string, bool) {
	formatted, ok := xhttpRange(value)
	if !ok {
		return "", false
	}
	first := strings.SplitN(formatted, "-", 2)[0]
	n, err := strconv.Atoi(first)
	if err != nil || n <= 0 {
		return "", false
	}
	return formatted, true
}

func xhttpRange(value any) (string, bool) {
	if n, ok := integerValue(value); ok {
		if n < 0 {
			return "", false
		}
		return strconv.Itoa(n), true
	}
	s, ok := value.(string)
	if !ok || s == "" || len(s) > 128 || containsControl(s) {
		return "", false
	}
	parts := strings.Split(s, "-")
	if len(parts) == 1 {
		n, err := strconv.Atoi(parts[0])
		if err != nil || n < 0 {
			return "", false
		}
		return strconv.Itoa(n), true
	}
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", false
	}
	first, err := strconv.Atoi(parts[0])
	if err != nil || first < 0 {
		return "", false
	}
	last, err := strconv.Atoi(parts[1])
	if err != nil || last < first {
		return "", false
	}
	return strconv.Itoa(first) + "-" + strconv.Itoa(last), true
}

// xhttpExtraOptions maps Xray's camelCase SplitHTTPConfig JSON onto Mihomo's
// kebab-case XHTTP option keys. Outer transport path/host/mode are authoritative:
// Xray itself overwrites those fields inside extra after decoding them.
func xhttpExtraOptions(raw map[string]any, path, host, mode string, hasPath, hasHost, hasMode bool) (map[string]any, error) {
	f := newFields(raw)
	options := map[string]any{}
	// Xray overwrites path/host/mode inside extra from the outer stream settings
	// after decoding. When v2rayN omits an outer parameter, the extra value is
	// the only source for that field; explicit outer values (including empty)
	// remain authoritative.
	innerPath, innerHost, innerMode := "", "", ""
	if f.has("path") {
		innerPath = f.text("path", false)
	}
	if f.has("host") {
		innerHost = f.text("host", false)
	}
	if f.has("mode") {
		innerMode = f.text("mode", false)
	}
	if !hasPath && innerPath != "" {
		path = innerPath
	}
	if !hasHost && innerHost != "" {
		host = innerHost
	}
	if !hasMode && innerMode != "" {
		mode = innerMode
	}
	if path == "" {
		path = "/"
	}
	if !strings.HasPrefix(path, "/") || containsControl(path) || len(path) > 8192 {
		return nil, errors.New("invalid XHTTP path")
	}
	options["path"] = path
	if host != "" {
		if _, ok := validHost(host); !ok {
			return nil, errors.New("invalid XHTTP host")
		}
		options["host"] = host
	}
	if mode != "" {
		if !oneOf(mode, "auto", "packet-up", "stream-up", "stream-one") {
			return nil, errors.New("unsupported XHTTP mode")
		}
		options["mode"] = mode
	}
	if f.has("headers") {
		headers, err := cleanHeaders(f.object("headers"), false)
		if err != nil {
			return nil, err
		}
		if _, hasHost := headers["Host"]; hasHost {
			return nil, errors.New("XHTTP headers must not include Host")
		}
		options["headers"] = headers
	}
	if f.has("xPaddingBytes") {
		v, ok := xhttpRangePositive(fieldValue(f, "xPaddingBytes"))
		if !ok {
			return nil, errors.New("invalid XHTTP padding range")
		}
		options["x-padding-bytes"] = v
	}
	if f.has("xPaddingObfsMode") {
		options["x-padding-obfs-mode"] = f.boolean("xPaddingObfsMode", false)
	}
	if enabled, _ := options["x-padding-obfs-mode"].(bool); enabled {
		// Xray fills these defaults when padding obfuscation is enabled; the
		// pinned core leaves them empty, which changes the wire framing.
		for key, fallback := range map[string]string{
			"x-padding-key": "x_padding", "x-padding-header": "X-Padding",
			"x-padding-placement": "queryInHeader", "x-padding-method": "repeat-x",
		} {
			if _, exists := options[key]; !exists {
				options[key] = fallback
			}
		}
	}
	for camel, key := range map[string]string{
		"xPaddingKey": "x-padding-key", "xPaddingHeader": "x-padding-header",
		"xPaddingPlacement": "x-padding-placement", "xPaddingMethod": "x-padding-method",
		"uplinkHTTPMethod": "uplink-http-method", "sessionIDPlacement": "session-placement",
		"sessionIDKey": "session-key", "sessionIDTable": "session-table",
		"seqPlacement": "seq-placement", "seqKey": "seq-key",
		"uplinkDataPlacement": "uplink-data-placement", "uplinkDataKey": "uplink-data-key",
	} {
		if f.has(camel) {
			value := f.text(camel, false)
			if containsControl(value) || len(value) > 8192 {
				return nil, errors.New("invalid XHTTP option")
			}
			if value != "" {
				options[key] = value
			}
		}
	}
	for camel, key := range map[string]string{
		"sessionIDLength": "session-length", "uplinkChunkSize": "uplink-chunk-size",
		"scMaxEachPostBytes": "sc-max-each-post-bytes", "scMinPostsIntervalMs": "sc-min-posts-interval-ms",
	} {
		if f.has(camel) {
			v, ok := xhttpRangePositive(fieldValue(f, camel))
			if !ok {
				return nil, errors.New("invalid XHTTP range option")
			}
			options[key] = v
		}
	}
	if f.has("noGRPCHeader") {
		options["no-grpc-header"] = f.boolean("noGRPCHeader", false)
	}
	if f.has("xmux") {
		reuse, err := xhttpXMux(f.object("xmux"))
		if err != nil {
			return nil, err
		}
		options["reuse-settings"] = reuse
	}
	if f.has("downloadSettings") {
		download, err := xhttpDownloadExtra(f.object("downloadSettings"))
		if err != nil {
			return nil, err
		}
		options["download-settings"] = download
	}
	// These Xray-only knobs are intentionally not silently ignored: the pinned
	// core has no equivalent, so accepting them would misrepresent the node.
	for _, key := range []string{"noSSEHeader", "scMaxBufferedPosts", "scStreamUpServerSecs", "serverMaxHeaderBytes", "h2Params", "extra"} {
		if f.has(key) {
			return nil, errors.New("unsupported XHTTP extra option")
		}
	}
	if err := f.finish(); err != nil {
		return nil, errors.New("unsupported XHTTP extra option")
	}
	if options["mode"] == "stream-one" && options["download-settings"] != nil {
		return nil, errors.New("XHTTP stream-one cannot use download settings")
	}
	return options, nil
}

func xhttpXMux(raw map[string]any) (map[string]any, error) {
	f := newFields(raw)
	options := map[string]any{}
	for camel, key := range map[string]string{
		"maxConcurrency": "max-concurrency", "maxConnections": "max-connections",
		"cMaxReuseTimes": "c-max-reuse-times", "hMaxRequestTimes": "h-max-request-times",
		"hMaxReusableSecs": "h-max-reusable-secs",
	} {
		if f.has(camel) {
			v, ok := xhttpRange(fieldValue(f, camel))
			if !ok {
				return nil, errors.New("invalid XHTTP reuse range")
			}
			options[key] = v
		}
	}
	if f.has("hKeepAlivePeriod") {
		options["h-keep-alive-period"] = f.integer("hKeepAlivePeriod", 0, 0, 2147483647)
	}
	if err := f.finish(); err != nil {
		return nil, errors.New("unsupported XHTTP reuse setting")
	}
	return options, nil
}

func xhttpDownloadExtra(raw map[string]any) (map[string]any, error) {
	f := newFields(raw)
	options := map[string]any{}
	if f.has("path") {
		value, _ := f.get("path")
		path, ok := value.(string)
		if !ok {
			return nil, errors.New("invalid XHTTP download path")
		}
		if path != "" {
			if !strings.HasPrefix(path, "/") || containsControl(path) || len(path) > 8192 {
				return nil, errors.New("invalid XHTTP download path")
			}
			options["path"] = path
		}
	}
	if f.has("host") {
		value, _ := f.get("host")
		host, ok := value.(string)
		if !ok {
			return nil, errors.New("invalid XHTTP download host")
		}
		if host != "" {
			if _, valid := validHost(host); !valid {
				return nil, errors.New("invalid XHTTP download host")
			}
			options["host"] = host
		}
	}
	if f.has("headers") {
		headers, err := cleanHeaders(f.object("headers"), false)
		if err != nil {
			return nil, err
		}
		if _, hasHost := headers["Host"]; hasHost {
			return nil, errors.New("XHTTP download headers must not include Host")
		}
		options["headers"] = headers
	}
	if f.has("reuseSettings") {
		reuse, err := xhttpXMux(f.object("reuseSettings"))
		if err != nil {
			return nil, err
		}
		options["reuse-settings"] = reuse
	}
	address := ""
	if f.has("address") {
		address = f.text("address", false)
	}
	if f.has("server") {
		server := f.text("server", true)
		if address != "" && address != server {
			return nil, errors.New("conflicting XHTTP download server")
		}
		address = server
	}
	if address != "" {
		if _, ok := validHost(address); !ok {
			return nil, errors.New("invalid XHTTP download server")
		}
		options["server"] = address
	}
	if f.has("port") {
		options["port"] = f.integer("port", 0, 1, 65535)
	}
	tls := false
	tlsSelected := false
	if f.has("tls") {
		tls = f.boolean("tls", false)
		options["tls"] = tls
		tlsSelected = true
	}
	security := ""
	if f.has("security") {
		security = strings.ToLower(f.text("security", false))
		switch security {
		case "tls", "reality":
			tls = true
			tlsSelected = true
			options["tls"] = true
		case "", "none":
			tls = false
			tlsSelected = true
			options["tls"] = false
		default:
			return nil, errors.New("unsupported XHTTP download security")
		}
	}
	// Xray keeps inactive TLS/Reality blocks around in its JSON. Only the block
	// selected by security may contribute settings; consume the rest without
	// letting stale credentials change the transport.
	if f.has("tlsSettings") {
		settings, err := xhttpExtraTLSSettings(f.object("tlsSettings"), tlsSelected && tls && security != "reality")
		if err != nil {
			return nil, err
		}
		for key, value := range settings {
			options[key] = value
		}
	}
	if tlsSelected && tls {
		// Mihomo inherits omitted download TLS fields from the uplink. Xray
		// selects an independent security mode with verification on by default,
		// so pin the inherited Reality and certificate-verification state off.
		if _, exists := options["skip-cert-verify"]; !exists {
			options["skip-cert-verify"] = false
		}
		if _, exists := options["reality-opts"]; !exists {
			options["reality-opts"] = map[string]any{}
		}
	}
	if f.has("realitySettings") {
		settings, err := xhttpExtraRealitySettings(f.object("realitySettings"), security == "reality")
		if err != nil {
			return nil, err
		}
		for key, value := range settings {
			options[key] = value
		}
	}
	if f.has("xhttpSettings") {
		settings, err := xhttpExtraSettings(f.object("xhttpSettings"))
		if err != nil {
			return nil, err
		}
		for key, value := range settings {
			options[key] = value
		}
	}
	// Flat native form used by the Clash/Mihomo xhttp-opts object.
	if f.has("alpn") {
		options["alpn"] = f.strings("alpn")
	}
	if f.has("skipCertVerify") {
		options["skip-cert-verify"] = f.boolean("skipCertVerify", false)
	}
	if f.has("nameCertVerify") {
		value := f.text("nameCertVerify", true)
		if _, ok := validHost(value); !ok || !tls {
			return nil, errors.New("invalid XHTTP download certificate verification name")
		}
		options["name-cert-verify"] = value
	}
	if f.has("fingerprint") {
		value, ok := normalizePin(f.text("fingerprint", true))
		if !ok || !tls {
			return nil, errors.New("invalid XHTTP download fingerprint")
		}
		options["fingerprint"] = value
	}
	if f.has("serverName") {
		value := f.text("serverName", true)
		if _, ok := validHost(value); !ok || !tls {
			return nil, errors.New("invalid XHTTP download server name")
		}
		options["servername"] = value
	}
	if f.has("clientFingerprint") {
		value := f.text("clientFingerprint", true)
		if !oneOf(value, "chrome", "firefox", "safari", "iOS", "android", "edge", "360", "qq", "random", "randomized") {
			return nil, errors.New("unsupported XHTTP download client fingerprint")
		}
		options["client-fingerprint"] = value
	}
	if f.has("echOpts") {
		ech, err := readECHMap(f.object("echOpts"))
		if err != nil || ech == nil || !tls {
			return nil, errors.New("invalid XHTTP download ECH options")
		}
		options["ech-opts"] = ech
	}
	if f.has("realityOpts") {
		reality, err := readRealityMap(f.object("realityOpts"))
		if err != nil || !tls {
			return nil, errors.New("invalid XHTTP download Reality options")
		}
		options["reality-opts"] = reality
	}
	if security == "reality" {
		reality, _ := options["reality-opts"].(map[string]any)
		if len(reality) == 0 {
			return nil, errors.New("XHTTP download Reality requires a public key")
		}
	}
	for _, key := range []string{"shadowTLSOpts", "restlsOpts", "jlsOpts", "certificate", "privateKey"} {
		if f.has(key) {
			return nil, errors.New("unsupported XHTTP download security option")
		}
	}
	if err := f.finish(); err != nil {
		return nil, errors.New("unsupported XHTTP download setting")
	}
	return options, nil
}

func xhttpExtraTLSSettings(raw map[string]any, selected bool) (map[string]any, error) {
	if !selected {
		return nil, nil
	}
	f := newFields(raw)
	options := map[string]any{}
	if f.has("serverName") {
		value := f.text("serverName", false)
		if value != "" {
			if _, ok := validHost(value); !ok {
				return nil, errors.New("invalid XHTTP download server name")
			}
			options["servername"] = value
		}
	}
	if f.has("allowInsecure") {
		options["skip-cert-verify"] = f.boolean("allowInsecure", false)
	}
	if f.has("fingerprint") {
		value := f.text("fingerprint", false)
		if value != "" {
			if !oneOf(value, "chrome", "firefox", "safari", "iOS", "android", "edge", "360", "qq", "random", "randomized") {
				return nil, errors.New("unsupported XHTTP download client fingerprint")
			}
			options["client-fingerprint"] = value
		}
	}
	if f.has("alpn") {
		options["alpn"] = f.strings("alpn")
	}
	if err := f.finish(); err != nil {
		return nil, errors.New("unsupported XHTTP download TLS setting")
	}
	return options, nil
}

func xhttpExtraRealitySettings(raw map[string]any, selected bool) (map[string]any, error) {
	if !selected {
		return nil, nil
	}
	f := newFields(raw)
	options := map[string]any{}
	key := f.text("publicKey", true)
	sid := f.text("shortId", false)
	if !validPublicKey(key) || !validShortID(sid) {
		return nil, errors.New("invalid XHTTP download Reality options")
	}
	options["reality-opts"] = map[string]any{"public-key": strings.TrimRight(key, "="), "short-id": strings.ToLower(sid)}
	if f.has("serverName") {
		value := f.text("serverName", false)
		if value != "" {
			if _, ok := validHost(value); !ok {
				return nil, errors.New("invalid XHTTP download server name")
			}
			options["servername"] = value
		}
	}
	if f.has("fingerprint") {
		value := f.text("fingerprint", false)
		if value != "" {
			if !oneOf(value, "chrome", "firefox", "safari", "iOS", "android", "edge", "360", "qq", "random", "randomized") {
				return nil, errors.New("unsupported XHTTP download client fingerprint")
			}
			options["client-fingerprint"] = value
		}
	}
	if f.has("spiderX") {
		value := f.text("spiderX", false)
		if value != "" && value != "/" {
			return nil, errors.New("XHTTP download spider-X is unsupported by the pinned core")
		}
	}
	if err := f.finish(); err != nil {
		return nil, errors.New("unsupported XHTTP download Reality setting")
	}
	return options, nil
}

func xhttpExtraSettings(raw map[string]any) (map[string]any, error) {
	f := newFields(raw)
	options := map[string]any{}
	if f.has("host") {
		value := f.text("host", false)
		if value != "" {
			if _, ok := validHost(value); !ok {
				return nil, errors.New("invalid XHTTP download host")
			}
			options["host"] = value
		}
	}
	if f.has("path") {
		value := f.text("path", false)
		if value != "" {
			if !strings.HasPrefix(value, "/") || containsControl(value) || len(value) > 8192 {
				return nil, errors.New("invalid XHTTP download path")
			}
			options["path"] = value
		}
	}
	if f.has("headers") {
		headers, err := cleanHeaders(f.object("headers"), false)
		if err != nil {
			return nil, err
		}
		if _, hasHost := headers["Host"]; hasHost {
			return nil, errors.New("XHTTP download headers must not include Host")
		}
		options["headers"] = headers
	}
	if f.has("extra") {
		extra, ok := f.get("extra")
		if !ok {
			return nil, errors.New("invalid XHTTP download extra")
		}
		rawExtra, ok := extra.(map[string]any)
		if !ok {
			return nil, errors.New("invalid XHTTP download extra")
		}
		ef := newFields(rawExtra)
		if ef.has("xmux") {
			value, ok := ef.get("xmux")
			if !ok {
				return nil, errors.New("invalid XHTTP download xmux")
			}
			rawXMux, ok := value.(map[string]any)
			if !ok {
				return nil, errors.New("invalid XHTTP download xmux")
			}
			reuse, err := xhttpXMux(rawXMux)
			if err != nil {
				return nil, err
			}
			options["reuse-settings"] = reuse
		}
		if err := ef.finish(); err != nil {
			return nil, errors.New("unsupported XHTTP download extra")
		}
	}
	if err := f.finish(); err != nil {
		return nil, errors.New("unsupported XHTTP download setting")
	}
	return options, nil
}
