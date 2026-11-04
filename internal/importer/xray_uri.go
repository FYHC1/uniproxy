package importer

import (
	"encoding/json"
	"errors"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/uniproxy/uniproxy/internal/model"
)

// xrayOnlyURI reports whether the URI carries a field that the pinned Mihomo
// adapter cannot represent. The decision only selects the parser; it does not
// relax validation. A malformed Xray-only value therefore still fails strict
// parsing and remains a preview-only degraded row.
func xrayOnlyURI(protocol string, q *query) bool {
	if protocol != "vless" && protocol != "vmess" && protocol != "trojan" {
		return false
	}
	nonEmpty := func(keys ...string) bool {
		for _, key := range keys {
			if values, ok := q.values[key]; ok && len(values) > 0 && strings.TrimSpace(values[0]) != "" {
				return true
			}
		}
		return false
	}
	if nonEmpty("fm", "pqv", "cs") {
		return true
	}
	if values, ok := q.values["spx"]; ok && len(values) > 0 {
		value := strings.TrimSpace(values[0])
		if value != "" && value != "/" {
			return true
		}
	}
	if protocol == "trojan" && nonEmpty("flow") {
		// Trojan flow is not representable by the pinned Mihomo adapter; the
		// Xray parser consumes and validates it instead of rejecting the URI as
		// an unknown native query option.
		return true
	}
	if protocol == "vless" {
		if nonEmpty("encryption") {
			values := q.values["encryption"]
			if len(values) > 0 && !strings.EqualFold(strings.TrimSpace(values[0]), "none") {
				return true
			}
		}
		if values, ok := q.values["flow"]; ok && len(values) > 0 && strings.EqualFold(strings.TrimSpace(values[0]), "xtls-rprx-vision-udp443") {
			return true
		}
	}
	if protocol == "vmess" {
		for _, key := range []string{"type", "network"} {
			if values, ok := q.values[key]; ok && len(values) > 0 && strings.EqualFold(strings.TrimSpace(values[0]), "xhttp") {
				return true
			}
		}
	}
	return false
}

func parseXrayURI(u *url.URL, protocol string, q *query) (model.ParsedNode, error) {
	server, valid := validHost(u.Hostname())
	if !valid {
		return model.ParsedNode{}, errors.New("invalid proxy server")
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil || port < 1 || port > 65535 {
		return model.ParsedNode{}, errors.New("invalid proxy port")
	}
	name := u.Fragment
	if containsControl(name) || utf8.RuneCountInString(name) > maxNodeNameRunes {
		return model.ParsedNode{}, errors.New("invalid node name")
	}
	if strings.TrimSpace(name) == "" {
		name = defaultName(protocol, server, port)
	}
	name = truncateName(name, MaxNodeNameRunes)

	udp := true
	if value, ok := q.take("udp"); ok {
		b, valid := boolValue(value)
		if !valid {
			q.fail("invalid URI boolean option")
		}
		udp = b
	}

	settings := map[string]any{}
	vlessEncryption := ""
	switch protocol {
	case "vless":
		if u.User == nil {
			return model.ParsedNode{}, errors.New("missing UUID credential")
		}
		if _, hasPassword := u.User.Password(); hasPassword {
			return model.ParsedNode{}, errors.New("unsupported UUID URI credentials")
		}
		uuid := u.User.Username()
		if !uuidPattern.MatchString(uuid) {
			return model.ParsedNode{}, errors.New("invalid UUID credential")
		}
		encryption := "none"
		if value, ok := q.take("encryption"); ok {
			if value != "" && value != "none" {
				if !validVLESSEncryption(value) {
					q.fail("unsupported VLESS encryption")
				}
				encryption = value
			}
		}
		vlessEncryption = encryption
		user := map[string]any{"id": strings.ToLower(uuid), "encryption": encryption}
		if value, ok := q.take("flow"); ok && value != "" {
			if !oneOf(value, "xtls-rprx-vision", "xtls-rprx-vision-udp443") {
				q.fail("unsupported VLESS flow")
			}
			user["flow"] = value
		}
		if value, ok := q.take("packetEncoding", "packet-encoding"); ok && value != "" && value != "none" {
			if !oneOf(value, "xudp", "packetaddr") {
				q.fail("unsupported packet encoding")
			}
			user["packetEncoding"] = value
		}
		settings["vnext"] = []any{map[string]any{"address": server, "port": port, "users": []any{user}}}
	case "vmess":
		if u.User == nil {
			return model.ParsedNode{}, errors.New("missing UUID credential")
		}
		if _, hasPassword := u.User.Password(); hasPassword {
			return model.ParsedNode{}, errors.New("unsupported UUID URI credentials")
		}
		uuid := u.User.Username()
		if !uuidPattern.MatchString(uuid) {
			return model.ParsedNode{}, errors.New("invalid UUID credential")
		}
		alterID := 0
		if value, ok := q.take("alterId", "aid"); ok && value != "" {
			n, valid := integerValue(value)
			if !valid || n < 0 || n > 65535 {
				q.fail("invalid VMess alterId")
			}
			alterID = n
		}
		cipher := "auto"
		if value, ok := q.take("encryption", "cipher", "scy"); ok && value != "" {
			if !oneOf(value, "auto", "aes-128-gcm", "chacha20-poly1305", "none", "zero") {
				q.fail("unsupported VMess cipher")
			}
			cipher = value
		}
		user := map[string]any{"id": strings.ToLower(uuid), "alterId": alterID, "security": cipher}
		if value, ok := q.take("packetEncoding", "packet-encoding"); ok && value != "" && value != "none" {
			if !oneOf(value, "xudp", "packetaddr") {
				q.fail("unsupported packet encoding")
			}
			user["packetEncoding"] = value
		}
		settings["vnext"] = []any{map[string]any{"address": server, "port": port, "users": []any{user}}}
	case "trojan":
		password := passwordUser(u.User)
		if password == "" {
			return model.ParsedNode{}, errors.New("missing Trojan password")
		}
		entry := map[string]any{"address": server, "port": port, "password": password}
		if value, ok := q.take("flow"); ok && value != "" {
			// The pinned Xray 26.10.31 rejects Flow on Trojan outbounds;
			// accepting it here would produce a node that always fails
			// xray -test at apply time.
			q.fail("Trojan flow is not supported")
		}
		settings["servers"] = []any{entry}
	}

	stream, err := xrayTransport(q)
	if err != nil {
		return model.ParsedNode{}, err
	}
	if err := xrayTLS(q, stream, protocol == "trojan"); err != nil {
		return model.ParsedNode{}, err
	}
	if protocol == "vless" && stream["security"] == "none" && (vlessEncryption == "" || vlessEncryption == "none") && publicIPAddress(server) {
		return model.ParsedNode{}, errors.New("VLESS without TLS or encryption is not permitted for a public IP address")
	}
	if protocol == "trojan" {
		if servers, ok := settings["servers"].([]any); ok && len(servers) == 1 {
			if entry, ok := servers[0].(map[string]any); ok {
				if flow, _ := entry["flow"].(string); flow != "" && stream["security"] != "tls" && stream["security"] != "reality" {
					return model.ParsedNode{}, errors.New("Trojan flow requires TLS or Reality")
				}
			}
		}
	}
	if value, ok := q.take("fm"); ok && value != "" {
		mask, err := parseFinalMask(value)
		if err != nil {
			return model.ParsedNode{}, err
		}
		stream["finalmask"] = mask
	}
	if value, ok := q.take("tfo"); ok {
		b, valid := boolValue(value)
		if !valid {
			q.fail("invalid URI boolean option")
		} else {
			sockopt, _ := stream["sockopt"].(map[string]any)
			if sockopt == nil {
				sockopt = map[string]any{}
			}
			sockopt["tcpFastOpen"] = b
			stream["sockopt"] = sockopt
		}
	}
	if value, ok := q.take("ip-version"); ok && value != "" {
		strategies := map[string]string{
			"dual": "UseIP", "ipv4": "UseIPv4", "ipv6": "UseIPv6",
			"ipv4-prefer": "UseIPv4v6", "ipv6-prefer": "UseIPv6v4",
		}
		strategy, valid := strategies[value]
		if !valid {
			q.fail("unsupported IP version policy")
		} else {
			sockopt, _ := stream["sockopt"].(map[string]any)
			if sockopt == nil {
				sockopt = map[string]any{}
			}
			sockopt["domainStrategy"] = strategy
			stream["sockopt"] = sockopt
		}
	}
	if err := q.finish(); err != nil {
		return model.ParsedNode{}, err
	}
	outbound := map[string]any{
		"protocol":       protocol,
		"settings":       settings,
		"streamSettings": stream,
	}
	return model.ParsedNode{
		Name:     name,
		Protocol: protocol,
		Engine:   model.EngineXray,
		Server:   server,
		Port:     port,
		UDP:      udp,
		Config:   map[string]any{"xray": outbound},
	}, nil
}

func xrayTLS(q *query, stream map[string]any, defaultTLS bool) error {
	security, hasSecurity := q.take("security")
	tlsOption, hasTLS := q.take("tls")
	enabled, reality := defaultTLS, false
	if hasSecurity {
		switch strings.ToLower(strings.TrimSpace(security)) {
		case "tls":
			enabled = true
		case "reality":
			enabled, reality = true, true
		case "false", "none", "":
			enabled = false
		default:
			q.fail("unsupported TLS security variant")
		}
	}
	if hasTLS {
		b, valid := boolValue(tlsOption)
		if !valid {
			q.fail("invalid TLS boolean")
		}
		if hasSecurity && enabled != b {
			q.fail("conflicting TLS options")
		}
		enabled = b
	}
	if defaultTLS && !enabled {
		q.fail("TLS cannot be disabled for this URI scheme")
	}

	serverName := ""
	if value, ok := q.takeEquivalentSNI(); ok {
		serverName = value
	}
	tlsSettings := map[string]any{}
	realitySettings := map[string]any{}
	if serverName != "" {
		if reality {
			realitySettings["serverName"] = serverName
		} else {
			tlsSettings["serverName"] = serverName
		}
	}
	if value, ok := q.take("allowInsecure", "allowinsecure", "insecure", "skip-cert-verify"); ok {
		b, valid := boolValue(value)
		if !valid {
			q.fail("invalid certificate verification option")
		} else if reality {
			q.fail("Reality cannot disable certificate verification")
		} else {
			tlsSettings["allowInsecure"] = b
		}
	}
	if value, ok := q.take("fp", "client-fingerprint"); ok {
		value = strings.TrimSpace(value)
		if value != "" {
			if strings.EqualFold(value, "ios") {
				value = "iOS"
			}
			if !oneOf(value, "chrome", "firefox", "safari", "iOS", "android", "edge", "360", "qq", "random", "randomized", "unsafe") {
				q.fail("unsupported client fingerprint")
			} else if reality {
				realitySettings["fingerprint"] = value
			} else {
				tlsSettings["fingerprint"] = value
			}
		}
	}
	if value, ok := q.take("alpn"); ok && value != "" {
		if reality {
			q.fail("Reality does not support custom ALPN")
		} else {
			parts := strings.Split(value, ",")
			list := make([]any, 0, len(parts))
			for _, part := range parts {
				part = strings.TrimSpace(part)
				if part == "" || len(part) > 255 || containsControl(part) {
					q.fail("invalid ALPN")
					break
				}
				list = append(list, part)
			}
			if len(list) > 0 && q.err == nil {
				tlsSettings["alpn"] = list
			}
		}
	}
	if value, ok := q.take("cs"); ok && value != "" {
		if reality {
			q.fail("Reality does not support custom cipher suites")
		} else if !validCipherSuites(value) {
			q.fail("invalid TLS cipher suites")
		} else {
			tlsSettings["cipherSuites"] = value
		}
	}
	// Certificate pinning and ECH are not Xray stream settings in this adapter.
	for _, key := range []string{"name-cert-verify", "vcn", "fingerprint", "pinSHA256", "pinSHA256Cert", "pcs", "ech"} {
		if value, ok := q.take(key); ok && value != "" {
			q.fail("unsupported Xray TLS option")
		}
	}
	publicKey, hasKey := q.take("pbk", "public-key")
	shortID, hasID := q.take("sid", "short-id")
	if reality {
		if !validPublicKey(publicKey) {
			q.fail("invalid Reality public key")
		}
		if !validShortID(shortID) {
			q.fail("invalid Reality short ID")
		}
		realitySettings["publicKey"] = strings.TrimRight(publicKey, "=")
		realitySettings["shortId"] = strings.ToLower(shortID)
	} else if hasKey || hasID {
		q.fail("Reality options require security=reality")
	}
	if value, ok := q.take("spx"); ok && value != "" && value != "/" {
		if !reality {
			q.fail("Reality spider-X requires security=reality")
		} else if containsControl(value) || len(value) > 8192 {
			q.fail("invalid Reality spider-X")
		} else {
			realitySettings["spiderX"] = value
		}
	}
	if value, ok := q.take("pqv"); ok && value != "" {
		if !reality {
			q.fail("Reality ML-DSA-65 verification requires security=reality")
		} else if containsControl(value) || len(value) > 8192 {
			q.fail("invalid Reality ML-DSA-65 verification value")
		} else {
			realitySettings["mldsa65Verify"] = value
		}
	}

	switch {
	case reality:
		stream["security"] = "reality"
		stream["realitySettings"] = realitySettings
	case enabled:
		stream["security"] = "tls"
		stream["tlsSettings"] = tlsSettings
	default:
		stream["security"] = "none"
	}
	return q.err
}

func publicIPAddress(host string) bool {
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	return addr.IsGlobalUnicast() && !addr.IsPrivate() && !addr.IsLoopback() && !addr.IsLinkLocalUnicast()
}

func validCipherSuites(value string) bool {
	if len(value) > 8192 || containsControl(value) {
		return false
	}
	parts := strings.Split(value, ":")
	if len(parts) == 0 || len(parts) > 256 {
		return false
	}
	for _, part := range parts {
		if part == "" || len(part) > 128 {
			return false
		}
		for _, c := range part {
			if c != '-' && c != '_' && (c < '0' || c > '9') && (c < 'A' || c > 'Z') && (c < 'a' || c > 'z') {
				return false
			}
		}
	}
	return true
}

func parseFinalMask(value string) (map[string]any, error) {
	if len(value) > 65536 || containsControl(value) {
		return nil, errors.New("invalid Xray finalmask")
	}
	var raw any
	if err := json.Unmarshal([]byte(value), &raw); err != nil {
		return nil, errors.New("invalid Xray finalmask")
	}
	mask, ok := raw.(map[string]any)
	if !ok {
		return nil, errors.New("invalid Xray finalmask")
	}
	allowed := map[string]map[string]bool{
		"tcp": {"filter": true, "header-custom": true, "fragment": true, "sudoku": true, "xmc": true},
		"udp": {"filter": true, "header-custom": true, "mkcp-legacy": true, "noise": true, "salamander": true, "sudoku": true, "xdns": true, "xicmp": true, "realm": true, "udphop": true},
	}
	for key, child := range mask {
		switch key {
		case "tcp", "udp":
			list, ok := child.([]any)
			if !ok {
				return nil, errors.New("invalid Xray finalmask")
			}
			for _, item := range list {
				entry, ok := item.(map[string]any)
				if !ok {
					return nil, errors.New("invalid Xray finalmask")
				}
				kind, _ := entry["type"].(string)
				if !allowed[key][kind] {
					return nil, errors.New("invalid Xray finalmask")
				}
				if settings, exists := entry["settings"]; exists {
					if _, ok := settings.(map[string]any); !ok {
						return nil, errors.New("invalid Xray finalmask")
					}
				}
			}
		case "quicParams":
			if _, ok := child.(map[string]any); !ok {
				return nil, errors.New("invalid Xray finalmask")
			}
		default:
			return nil, errors.New("invalid Xray finalmask")
		}
	}
	return mask, nil
}

func xrayTransport(q *query) (map[string]any, error) {
	network, ok := q.take("type", "network")
	if !ok || network == "" {
		network = "tcp"
	}
	network = strings.ToLower(network)
	switch network {
	case "raw", "tcp", "none":
		network = "tcp"
	case "websocket":
		network = "ws"
	case "mkcp":
		network = "kcp"
	}
	stream := map[string]any{"network": network}
	switch network {
	case "tcp":
		header, hasHeader := q.take("headerType")
		if hasHeader && header != "" && header != "none" {
			if header != "http" {
				q.fail("unsupported transport header variant")
				break
			}
			tcp := map[string]any{"header": map[string]any{"type": "http"}}
			request := map[string]any{}
			if path, ok := q.take("path"); ok && path != "" {
				request["path"] = []any{path}
			}
			if host, ok := q.take("host"); ok && host != "" {
				request["headers"] = map[string]any{"Host": []any{host}}
			}
			if len(request) > 0 {
				tcp["header"].(map[string]any)["request"] = request
			}
			stream["tcpSettings"] = tcp
		}
	case "ws":
		if header, ok := q.take("headerType"); ok && header != "" && header != "none" {
			q.fail("unsupported transport header variant")
		}
		ws := map[string]any{}
		path := "/"
		if value, ok := q.take("path"); ok && value != "" {
			path = value
		}
		if !strings.HasPrefix(path, "/") || containsControl(path) || len(path) > 8192 {
			q.fail("invalid WebSocket path")
		}
		ws["path"] = path
		if host, ok := q.take("host"); ok && host != "" {
			if _, valid := validHost(host); !valid {
				q.fail("invalid WebSocket host")
			} else {
				ws["headers"] = map[string]any{"Host": host}
			}
		}
		if value, ok := q.take("ed", "max-early-data"); ok && value != "" {
			n, valid := integerValue(value)
			if !valid || n < 0 || n > 2147483647 {
				q.fail("invalid WebSocket early data")
			} else {
				ws["maxEarlyData"] = n
			}
		}
		if value, ok := q.take("eh", "early-data-header-name"); ok && value != "" {
			ws["earlyDataHeaderName"] = value
		}
		stream["wsSettings"] = ws
	case "grpc":
		grpc := map[string]any{}
		if value, ok := q.take("serviceName", "service-name", "grpc-service-name"); ok && value != "" {
			grpc["serviceName"] = value
		}
		if value, ok := q.take("authority"); ok && value != "" {
			grpc["authority"] = value
		}
		if value, ok := q.take("mode"); ok && value != "" && !strings.EqualFold(value, "gun") {
			q.fail("unsupported gRPC mode")
		}
		stream["grpcSettings"] = grpc
	case "xhttp":
		if header, ok := q.take("headerType"); ok && header != "" && header != "none" {
			q.fail("unsupported XHTTP header variant")
		}
		xhttp := map[string]any{}
		path := "/"
		if value, ok := q.take("path"); ok && value != "" {
			path = value
		}
		if !strings.HasPrefix(path, "/") || containsControl(path) || len(path) > 8192 {
			q.fail("invalid XHTTP path")
		}
		xhttp["path"] = path
		if host, ok := q.take("host"); ok && host != "" {
			if _, valid := validHost(host); !valid {
				q.fail("invalid XHTTP host")
			} else {
				xhttp["host"] = host
			}
		}
		if value, ok := q.take("mode"); ok && value != "" {
			if !oneOf(value, "auto", "packet-up", "stream-up", "stream-one") {
				q.fail("unsupported XHTTP mode")
			} else {
				xhttp["mode"] = value
			}
		}
		if value, ok := q.take("extra"); ok && strings.TrimSpace(value) != "" && !strings.EqualFold(strings.TrimSpace(value), "null") {
			extra, err := parseXHTTPExtra(value)
			if err != nil {
				q.fail("invalid XHTTP extra")
			} else {
				xhttp["extra"] = extra
			}
		}
		stream["xhttpSettings"] = xhttp
	case "httpupgrade":
		if header, ok := q.take("headerType"); ok && header != "" && header != "none" {
			q.fail("unsupported HTTP upgrade header variant")
		}
		upgrade := map[string]any{}
		path := "/"
		if value, ok := q.take("path"); ok && value != "" {
			path = value
		}
		if !strings.HasPrefix(path, "/") || containsControl(path) || len(path) > 8192 {
			q.fail("invalid HTTP upgrade path")
		}
		upgrade["path"] = path
		if host, ok := q.take("host"); ok && host != "" {
			if _, valid := validHost(host); !valid {
				q.fail("invalid HTTP upgrade host")
			} else {
				upgrade["host"] = host
			}
		}
		stream["httpupgradeSettings"] = upgrade
	case "http", "h2":
		if network == "h2" {
			stream["network"] = "http"
		}
		httpSettings := map[string]any{}
		if value, ok := q.take("path"); ok && value != "" {
			httpSettings["path"] = value
		}
		if value, ok := q.take("host"); ok && value != "" {
			hosts := []any{}
			for _, host := range strings.Split(value, ",") {
				if _, valid := validHost(host); !valid {
					q.fail("invalid HTTP host")
					break
				}
				hosts = append(hosts, host)
			}
			httpSettings["host"] = hosts
		}
		stream["httpSettings"] = httpSettings
	case "kcp":
		kcp := map[string]any{}
		if header, ok := q.take("headerType", "header"); ok && header != "" && !oneOf(header, "none", "srtp", "utp", "wechat-video", "dtls", "wireguard") {
			q.fail("unsupported KCP header")
		} else if header != "" && header != "none" {
			kcp["header"] = map[string]any{"type": header}
		}
		seed := ""
		if value, ok := q.take("seed"); ok {
			seed = value
		} else if value, ok := q.take("path"); ok {
			seed = value
		}
		if seed != "" {
			if containsControl(seed) || len(seed) > 8192 {
				q.fail("invalid KCP seed")
			} else {
				kcp["seed"] = seed
			}
		}
		if value, ok := q.take("mtu"); ok {
			n, valid := integerValue(value)
			if !valid || n < 576 || n > 1500 {
				q.fail("invalid KCP MTU")
			} else {
				kcp["mtu"] = n
			}
		}
		stream["kcpSettings"] = kcp
	default:
		q.fail("unsupported transport network")
	}
	return stream, q.err
}
