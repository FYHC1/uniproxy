package importer

import (
	"errors"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/uniproxy/uniproxy/internal/model"
)

type query struct {
	values url.Values
	used   map[string]bool
	err    error
}

func newQuery(raw string) *query {
	values, err := url.ParseQuery(raw)
	q := &query{values: values, used: map[string]bool{}}
	if err != nil {
		q.fail("malformed URI query encoding")
	}
	for key, v := range values {
		if len(v) != 1 {
			q.fail("duplicate URI query option")
		}
		if !utf8.ValidString(key) {
			q.fail("invalid URI query encoding")
		}
		for _, value := range v {
			if !utf8.ValidString(value) || strings.ContainsRune(value, 0) {
				q.fail("invalid URI query encoding")
			}
		}
	}
	return q
}
func (q *query) fail(s string) {
	if q.err == nil {
		q.err = errors.New(s)
	}
}
func (q *query) take(keys ...string) (string, bool) {
	value := ""
	present := false
	for _, key := range keys {
		q.used[key] = true
		if values, ok := q.values[key]; ok {
			if present {
				q.fail("conflicting URI option aliases")
			}
			present = true
			if len(values) > 0 {
				value = values[0]
			}
		}
	}
	return value, present
}
func (q *query) set(out map[string]any, key string, aliases ...string) {
	if value, ok := q.take(aliases...); ok {
		out[key] = value
	}
}
func (q *query) flag(out map[string]any, key string, aliases ...string) {
	if value, ok := q.take(aliases...); ok {
		b, valid := boolValue(value)
		if !valid {
			q.fail("invalid URI boolean option")
		}
		out[key] = b
	}
}
func (q *query) finish() error {
	unknown, unsafe := false, false
	for key := range q.values {
		if !q.used[key] {
			unknown = true
			unsafe = unsafe || unsafeOptionKey(key)
		}
	}
	if unsafe {
		q.fail("unsafe file or delegation URI option is not allowed")
	} else if unknown {
		q.fail("unsupported URI query option")
	}
	return q.err
}

func parseURI(text string) (model.ParsedNode, error) {
	if !isURILine(text) {
		return model.ParsedNode{}, errors.New("expected one proxy URI per line")
	}
	scheme := strings.ToLower(text[:strings.Index(text, "://")])
	protocol := canonicalProtocol(scheme)
	if protocol == "" {
		return model.ParsedNode{}, errors.New("unsupported URI protocol")
	}
	address, _, _ := strings.Cut(text, "#")
	if strings.ContainsAny(address, " \t\r\n") {
		return model.ParsedNode{}, errors.New("URI whitespace must be percent encoded")
	}
	if protocol == "vmess" && !strings.Contains(strings.SplitN(address, "?", 2)[0], "@") {
		return parseVMessJSON(text)
	}
	if protocol == "ss" {
		return parseSSURI(text)
	}
	u, err := url.Parse(text)
	if err != nil || u.Host == "" || u.Opaque != "" {
		return model.ParsedNode{}, errors.New("malformed proxy URI")
	}
	if u.Path != "" && u.Path != "/" {
		return model.ParsedNode{}, errors.New("URI paths must use an explicit transport path option")
	}
	q := newQuery(u.RawQuery)
	out := map[string]any{"type": protocol, "server": u.Hostname(), "name": u.Fragment}
	port := u.Port()
	if port == "" {
		switch scheme {
		case "http":
			port = "80"
		case "https":
			port = "443"
		case "ssh":
			port = "22"
		case "hysteria2", "hy2":
			port = "443"
		}
	}
	if port == "" {
		return model.ParsedNode{}, errors.New("missing proxy URI port")
	}
	if strings.HasSuffix(u.Host, ":") {
		return model.ParsedNode{}, errors.New("missing proxy URI port")
	}
	if strings.Contains(u.Hostname(), ":") && !strings.HasPrefix(u.Host, "[") {
		return model.ParsedNode{}, errors.New("IPv6 URI hosts must use brackets")
	}
	out["port"] = port
	switch protocol {
	case "vless", "vmess":
		if u.User == nil {
			return model.ParsedNode{}, errors.New("missing UUID credential")
		}
		if _, ok := u.User.Password(); ok {
			return model.ParsedNode{}, errors.New("unsupported UUID URI credentials")
		}
		out["uuid"] = u.User.Username()
		out["udp"] = true
		if protocol == "vless" {
			q.set(out, "encryption", "encryption")
			q.set(out, "flow", "flow")
		} else {
			out["alterId"] = 0
			q.set(out, "alterId", "alterId", "aid")
			q.set(out, "cipher", "encryption", "cipher", "scy")
		}
		q.set(out, "packet-encoding", "packetEncoding", "packet-encoding")
		uriTLS(q, out, protocol, false)
		uriTransport(q, out)
	case "trojan":
		out["password"] = passwordUser(u.User)
		out["udp"] = true
		uriTLS(q, out, protocol, true)
		uriTransport(q, out)
	case "hysteria":
		if u.User != nil {
			out["auth-str"] = passwordUser(u.User)
		}
		if value, ok := q.take("auth", "auth-str"); ok {
			if u.User != nil {
				q.fail("conflicting Hysteria credentials")
			}
			out["auth-str"] = value
		}
		q.set(out, "up", "upmbps", "up_mbps", "up")
		q.set(out, "down", "downmbps", "down_mbps", "down")
		q.set(out, "protocol", "protocol")
		if obfs, ok := q.take("obfs"); ok {
			if param, has := q.take("obfsParam", "obfs-param"); has {
				if obfs != "xplus" {
					q.fail("unsupported Hysteria obfuscation")
				}
				out["obfs"] = param
			} else {
				out["obfs"] = obfs
			}
		}
		q.set(out, "ports", "mport", "ports")
		q.flag(out, "fast-open", "fastopen", "fast-open")
		uriTLS(q, out, protocol, true)
	case "hysteria2":
		out["password"] = passwordUser(u.User)
		q.set(out, "obfs", "obfs")
		q.set(out, "obfs-password", "obfs-password", "obfsPassword")
		q.set(out, "up", "up", "upmbps")
		q.set(out, "down", "down", "downmbps")
		q.set(out, "ports", "mport", "ports")
		q.set(out, "hop-interval", "hop-interval", "hopInterval")
		uriTLS(q, out, protocol, true)
	case "tuic":
		if u.User == nil {
			return model.ParsedNode{}, errors.New("missing TUIC credentials")
		}
		password, hasPassword := u.User.Password()
		if hasPassword {
			out["uuid"] = u.User.Username()
			out["password"] = password
		} else {
			out["token"] = u.User.Username()
		}
		if v, ok := q.take("version"); ok {
			if v != "4" && v != "5" || v == "4" && hasPassword || v == "5" && !hasPassword {
				q.fail("unsupported or conflicting TUIC version")
			}
		}
		q.set(out, "congestion-controller", "congestion_control", "congestion-controller", "congestion-control")
		q.set(out, "udp-relay-mode", "udp_relay_mode", "udp-relay-mode")
		q.flag(out, "reduce-rtt", "reduce_rtt", "reduce-rtt", "zero_rtt_handshake")
		q.flag(out, "disable-sni", "disable_sni", "disable-sni")
		q.set(out, "heartbeat-interval", "heartbeat-interval")
		q.set(out, "request-timeout", "request-timeout")
		uriTLS(q, out, protocol, true)
	case "ssh":
		if u.User != nil {
			out["username"] = u.User.Username()
			if password, ok := u.User.Password(); ok {
				out["password"] = password
			}
		}
		if key, ok := q.take("private-key", "private_key"); ok {
			out["private-key"] = key
		}
		q.set(out, "private-key-passphrase", "private-key-passphrase", "private_key_passphrase")
		if keys, ok := q.take("host-key", "host_key"); ok {
			out["host-key"] = strings.Split(keys, ",")
		}
		if keys, ok := q.take("host-key-algorithms", "host_key_algorithms"); ok {
			out["host-key-algorithms"] = strings.Split(keys, ",")
		}
	case "socks5", "http":
		if u.User != nil {
			out["username"] = u.User.Username()
			if password, ok := u.User.Password(); ok {
				out["password"] = password
			}
		}
		if protocol == "socks5" {
			out["udp"] = false
		}
		secure := scheme == "https" || scheme == "socks5s"
		uriTLS(q, out, protocol, secure)
	}
	q.flag(out, "udp", "udp")
	q.flag(out, "tfo", "tfo")
	q.set(out, "ip-version", "ip-version")
	if err := q.finish(); err != nil {
		return model.ParsedNode{}, err
	}
	return normalizeNative(out)
}

func passwordUser(user *url.Userinfo) string {
	if user == nil {
		return ""
	}
	password := user.Username()
	if rest, ok := user.Password(); ok {
		password += ":" + rest
	}
	return password
}

func uriTLS(q *query, out map[string]any, protocol string, defaultTLS bool) {
	security, hasSecurity := q.take("security")
	tlsOption, hasTLS := q.take("tls")
	enabled := defaultTLS
	reality := false
	if hasSecurity {
		switch security {
		case "tls":
			enabled = true
		case "reality":
			enabled = true
			reality = true
		case "none", "":
			enabled = false
		default:
			q.fail("unsupported TLS security variant")
		}
	}
	if hasTLS {
		b, ok := boolValue(tlsOption)
		if !ok {
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
	out["tls"] = enabled
	key := "sni"
	if protocol == "vless" || protocol == "vmess" {
		key = "servername"
	}
	q.set(out, key, "sni", "peer", "serverName", "servername")
	q.flag(out, "skip-cert-verify", "allowInsecure", "allowinsecure", "insecure", "skip-cert-verify")
	q.set(out, "client-fingerprint", "fp", "client-fingerprint")
	q.set(out, "fingerprint", "fingerprint", "pinSHA256", "pinSHA256Cert")
	q.set(out, "name-cert-verify", "name-cert-verify")
	if alpn, ok := q.take("alpn"); ok {
		out["alpn"] = strings.Split(alpn, ",")
	}
	publicKey, hasKey := q.take("pbk", "public-key")
	shortID, hasID := q.take("sid", "short-id")
	if reality {
		out["reality-opts"] = map[string]any{"public-key": publicKey, "short-id": shortID}
	} else if hasKey || hasID {
		q.fail("Reality options require security=reality")
	}
}

func uriTransport(q *query, out map[string]any) {
	network, ok := q.take("type", "network")
	if !ok || network == "" {
		network = "tcp"
	}
	if network == "websocket" {
		network = "ws"
	}
	if header, ok := q.take("headerType"); ok {
		if header == "http" && network == "tcp" {
			network = "http"
		} else if header != "none" && header != "" {
			q.fail("unsupported transport header variant")
		}
	}
	out["network"] = network
	switch network {
	case "ws":
		options := map[string]any{}
		if path, ok := q.take("path"); ok {
			options["path"] = path
		}
		if host, ok := q.take("host"); ok {
			options["headers"] = map[string]any{"Host": host}
		}
		q.set(options, "max-early-data", "ed", "max-early-data")
		q.set(options, "early-data-header-name", "eh", "early-data-header-name")
		out["ws-opts"] = options
	case "grpc":
		options := map[string]any{}
		q.set(options, "grpc-service-name", "serviceName", "service-name")
		if mode, ok := q.take("mode"); ok && !oneOf(mode, "", "gun") {
			q.fail("unsupported gRPC mode")
		}
		if authority, ok := q.take("authority"); ok && authority != "" {
			q.fail("gRPC authority override is unsupported")
		}
		out["grpc-opts"] = options
	case "http":
		options := map[string]any{}
		if path, ok := q.take("path"); ok {
			options["path"] = strings.Split(path, ",")
		}
		if host, ok := q.take("host"); ok {
			options["headers"] = map[string]any{"Host": strings.Split(host, ",")}
		}
		out["http-opts"] = options
	case "h2":
		options := map[string]any{}
		q.set(options, "path", "path")
		if host, ok := q.take("host"); ok {
			options["host"] = strings.Split(host, ",")
		}
		out["h2-opts"] = options
	}
}

func parseSSURI(text string) (model.ParsedNode, error) {
	rest := text[strings.Index(text, "://")+3:]
	fragment := ""
	if i := strings.IndexByte(rest, '#'); i >= 0 {
		var err error
		fragment, err = url.PathUnescape(rest[i+1:])
		if err != nil {
			return model.ParsedNode{}, errors.New("invalid URI fragment encoding")
		}
		rest = rest[:i]
	}
	rawQuery := ""
	if i := strings.IndexByte(rest, '?'); i >= 0 {
		rawQuery = rest[i+1:]
		rest = rest[:i]
	}
	rest = strings.TrimSuffix(rest, "/")
	if !strings.Contains(rest, "@") {
		decoded, ok := decodeBase64(rest, false)
		if !ok {
			return model.ParsedNode{}, errors.New("invalid Shadowsocks base64 URI")
		}
		// The legacy format base64-encodes the entire method:password@host:port.
		at := strings.LastIndex(decoded, "@")
		if at < 1 {
			return model.ParsedNode{}, errors.New("missing Shadowsocks credentials")
		}
		method, password, ok := strings.Cut(decoded[:at], ":")
		if !ok {
			return model.ParsedNode{}, errors.New("invalid Shadowsocks credentials")
		}
		rest = url.UserPassword(method, password).String() + "@" + decoded[at+1:]
	} else {
		at := strings.LastIndex(rest, "@")
		user := rest[:at]
		if !strings.Contains(user, ":") {
			escaped, err := url.PathUnescape(user)
			if err != nil {
				return model.ParsedNode{}, errors.New("invalid Shadowsocks userinfo encoding")
			}
			decoded, ok := decodeBase64(escaped, false)
			if !ok {
				return model.ParsedNode{}, errors.New("invalid Shadowsocks userinfo base64")
			}
			method, password, ok := strings.Cut(decoded, ":")
			if !ok {
				return model.ParsedNode{}, errors.New("invalid Shadowsocks credentials")
			}
			user = url.UserPassword(method, password).String()
		}
		rest = user + rest[at:]
	}
	u, err := url.Parse("ss://" + rest)
	if err != nil || u.User == nil || u.Hostname() == "" || u.Port() == "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return model.ParsedNode{}, errors.New("malformed Shadowsocks URI")
	}
	if strings.Contains(u.Hostname(), ":") && !strings.HasPrefix(u.Host, "[") {
		return model.ParsedNode{}, errors.New("IPv6 URI hosts must use brackets")
	}
	password, ok := u.User.Password()
	if !ok {
		return model.ParsedNode{}, errors.New("missing Shadowsocks password")
	}
	out := map[string]any{"type": "ss", "server": u.Hostname(), "port": u.Port(), "cipher": u.User.Username(), "password": password, "name": fragment, "udp": true}
	q := newQuery(rawQuery)
	q.flag(out, "udp", "udp")
	q.flag(out, "tfo", "tfo")
	q.flag(out, "udp-over-tcp", "udp-over-tcp", "uot")
	q.set(out, "udp-over-tcp-version", "udp-over-tcp-version")
	if plugin, ok := q.take("plugin"); ok {
		name, options, err := pluginString(plugin)
		if err != nil {
			return model.ParsedNode{}, err
		}
		out["plugin"] = name
		out["plugin-opts"] = options
	}
	if err := q.finish(); err != nil {
		return model.ParsedNode{}, err
	}
	return normalizeNative(out)
}

func pluginString(s string) (string, map[string]any, error) {
	parts := strings.Split(s, ";")
	name := parts[0]
	if !oneOf(name, "obfs-local", "simple-obfs", "obfs", "v2ray-plugin") {
		return "", nil, errors.New("unsupported or executable Shadowsocks plugin")
	}
	options := map[string]any{}
	for _, part := range parts[1:] {
		if part == "" {
			continue
		}
		key, value, has := strings.Cut(part, "=")
		if name != "v2ray-plugin" {
			if key == "obfs" {
				key = "mode"
			}
			if key == "obfs-host" {
				key = "host"
			}
		}
		if _, exists := options[key]; exists {
			return "", nil, errors.New("duplicate plugin option")
		}
		if !has {
			if !oneOf(key, "tls", "mux", "skip-cert-verify") {
				return "", nil, errors.New("unsupported plugin flag")
			}
			options[key] = true
		} else {
			options[key] = value
		}
	}
	return name, options, nil
}

func decimalString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	if n, ok := integerValue(v); ok {
		return strconv.Itoa(n)
	}
	return ""
}
