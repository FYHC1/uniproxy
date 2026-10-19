package importer

import (
	"encoding/base64"
	"errors"
	"strconv"
	"strings"

	"github.com/uniproxy/uniproxy/internal/model"
)

func normalizeNative(input map[string]any) (model.ParsedNode, error) {
	r := newFields(input)
	rawType := r.text("type", true)
	protocol := canonicalProtocol(rawType)
	if protocol == "" {
		return model.ParsedNode{}, errors.New("unsupported proxy protocol or pseudo-outbound")
	}
	server, valid := validHost(r.text("server", true))
	if !valid {
		r.fail("invalid proxy server")
	}
	port := r.integer("port", 0, 1, 65535)
	if port == 0 {
		r.fail("missing required port")
	}
	name := r.text("name", false)
	if containsControl(name) || len(name) > 1024 {
		r.fail("invalid node name")
	}
	if name == "" {
		name = defaultName(protocol, server, port)
	}
	out := map[string]any{"type": protocol, "server": server, "port": port}
	if r.has("tfo") {
		copyBool(r, out, "tfo")
	}
	if r.has("ip-version") {
		v := r.text("ip-version", false)
		if !oneOf(v, "dual", "ipv4", "ipv6", "ipv4-prefer", "ipv6-prefer") {
			r.fail("unsupported IP version policy")
		}
		out["ip-version"] = v
	}
	udp := false
	switch protocol {
	case "vless", "vmess", "trojan", "ss", "socks5":
		udp = r.boolean("udp", false)
		out["udp"] = udp
	case "hysteria", "hysteria2", "tuic":
		udp = true
		if r.has("udp") && !r.boolean("udp", true) {
			r.fail("this protocol cannot disable UDP with the udp option")
		}
	case "ssh", "http":
		if r.has("udp") && r.boolean("udp", false) {
			r.fail("UDP is not supported by this protocol")
		}
	}
	switch protocol {
	case "vless":
		out["uuid"] = normalizeUUID(r, "uuid")
		if r.has("encryption") {
			v := r.text("encryption", false)
			if !oneOf(v, "", "none") {
				r.fail("unsupported VLESS encryption")
			}
		}
		if r.has("flow") {
			flow := r.text("flow", false)
			if !oneOf(flow, "", "xtls-rprx-vision") {
				r.fail("unsupported VLESS flow")
			}
			if flow != "" {
				out["flow"] = flow
			}
		}
		readPacketEncoding(r, out)
		readTLS(r, out, protocol, false)
		readTransport(r, out, protocol)
		if out["flow"] != "" && out["flow"] != nil {
			if out["tls"] != true || out["network"] != "tcp" {
				r.fail("VLESS Vision requires TLS over TCP")
			}
		}
	case "vmess":
		out["uuid"] = normalizeUUID(r, "uuid")
		out["alterId"] = r.integer("alterId", 0, 0, 65535)
		cipher := r.text("cipher", false)
		if cipher == "" {
			cipher = "auto"
		}
		if !oneOf(cipher, "auto", "aes-128-gcm", "chacha20-poly1305", "none", "zero") {
			r.fail("unsupported VMess cipher")
		}
		out["cipher"] = cipher
		readPacketEncoding(r, out)
		copyBool(r, out, "global-padding")
		copyBool(r, out, "authenticated-length")
		readTLS(r, out, protocol, false)
		readTransport(r, out, protocol)
	case "trojan":
		out["password"] = r.text("password", true)
		readTLS(r, out, protocol, true)
		readTransport(r, out, protocol)
	case "ss":
		out["password"] = r.text("password", true)
		cipher := r.text("cipher", true)
		if !oneOf(cipher, "aes-128-gcm", "aes-192-gcm", "aes-256-gcm", "chacha20-ietf-poly1305", "xchacha20-ietf-poly1305", "2022-blake3-aes-128-gcm", "2022-blake3-aes-256-gcm", "2022-blake3-chacha20-poly1305", "none", "dummy", "aes-128-ctr", "aes-192-ctr", "aes-256-ctr", "aes-128-cfb", "aes-192-cfb", "aes-256-cfb", "rc4-md5", "chacha20", "chacha20-ietf", "xchacha20") {
			r.fail("unsupported Shadowsocks cipher")
		}
		out["cipher"] = cipher
		if strings.HasPrefix(cipher, "2022-") {
			validateSS2022(r, cipher, out["password"].(string))
		}
		uot := r.boolean("udp-over-tcp", false)
		if r.has("udp-over-tcp") {
			out["udp-over-tcp"] = uot
		}
		if r.has("udp-over-tcp-version") {
			if !uot {
				r.fail("UDP-over-TCP version requires UDP-over-TCP")
			}
			out["udp-over-tcp-version"] = r.integer("udp-over-tcp-version", 2, 1, 2)
		}
		readPlugin(r, out)
	case "hysteria":
		if r.has("auth") && r.has("auth-str") {
			r.fail("ambiguous Hysteria authentication")
		}
		if r.has("auth") {
			value := r.text("auth", false)
			if _, err := base64.StdEncoding.Strict().DecodeString(value); err != nil {
				r.fail("invalid base64 Hysteria auth")
			}
			out["auth"] = value
		} else if r.has("auth-str") {
			out["auth-str"] = r.text("auth-str", false)
		}
		out["up"] = bandwidth(r, "up", true)
		out["down"] = bandwidth(r, "down", true)
		proto := r.text("protocol", false)
		if proto == "" {
			proto = "udp"
		}
		if proto != "udp" {
			r.fail("unsupported Hysteria transport; only UDP is allowed")
		}
		out["protocol"] = proto
		copyText(r, out, "obfs")
		readPortHopping(r, out, false)
		copyInt(r, out, "recv-window-conn", 1, 2147483647)
		copyInt(r, out, "recv-window", 1, 2147483647)
		if r.has("disable_mtu_discovery") {
			if r.has("disable-mtu-discovery") {
				r.fail("conflicting MTU discovery aliases")
			}
			out["disable-mtu-discovery"] = r.boolean("disable_mtu_discovery", false)
		}
		copyBool(r, out, "disable-mtu-discovery")
		copyBool(r, out, "fast-open")
		readTLS(r, out, protocol, true)
	case "hysteria2":
		out["password"] = r.text("password", true)
		if r.has("up") {
			out["up"] = bandwidth(r, "up", false)
		}
		if r.has("down") {
			out["down"] = bandwidth(r, "down", false)
		}
		if r.has("obfs") {
			obfs := r.text("obfs", false)
			if obfs != "salamander" {
				r.fail("unsupported Hysteria2 obfuscation")
			}
			out["obfs"] = obfs
			out["obfs-password"] = r.text("obfs-password", true)
		} else if r.has("obfs-password") {
			r.fail("obfuscation password requires an obfuscation mode")
		}
		readPortHopping(r, out, true)
		readTLS(r, out, protocol, true)
	case "tuic":
		if r.has("token") {
			out["token"] = r.text("token", true)
			if r.has("uuid") || r.has("password") {
				r.fail("ambiguous TUIC v4/v5 credentials")
			}
		} else {
			out["uuid"] = normalizeUUID(r, "uuid")
			out["password"] = r.text("password", true)
		}
		if r.has("udp-relay-mode") {
			mode := r.text("udp-relay-mode", false)
			if !oneOf(mode, "native", "quic") {
				r.fail("unsupported TUIC UDP relay mode")
			}
			out["udp-relay-mode"] = mode
		}
		if r.has("congestion-controller") {
			v := r.text("congestion-controller", false)
			if !oneOf(v, "cubic", "new_reno", "bbr") {
				r.fail("unsupported TUIC congestion controller")
			}
			out["congestion-controller"] = v
		}
		if r.has("disable-sni") {
			if r.boolean("disable-sni", false) {
				r.fail("TUIC disable-sni is unsafe: the core also disables certificate verification")
			}
			out["disable-sni"] = false
		}
		copyBool(r, out, "reduce-rtt")
		copyBool(r, out, "fast-open")
		copyInt(r, out, "heartbeat-interval", 1, 3600000)
		copyInt(r, out, "request-timeout", 1, 3600000)
		copyInt(r, out, "max-udp-relay-packet-size", 1, 65535)
		copyInt(r, out, "max-open-streams", 1, 65535)
		readTLS(r, out, protocol, true)
	case "ssh":
		out["username"] = r.text("username", true)
		if r.has("password") {
			out["password"] = r.text("password", false)
		}
		if r.has("private-key") {
			key := r.text("private-key", true)
			passphrase := r.text("private-key-passphrase", false)
			if err := validateSSHPrivateKey(key, passphrase); err != nil {
				r.fail(err.Error())
			}
			out["private-key"] = key
			if passphrase != "" {
				out["private-key-passphrase"] = passphrase
			}
		}
		if (out["password"] == nil || out["password"] == "") && out["private-key"] == nil {
			r.fail("SSH requires a password or inline private key")
		}
		if r.has("host-key") {
			keys := r.strings("host-key")
			for i, key := range keys {
				normalized, err := normalizeSSHHostKey(key)
				if err != nil {
					r.fail(err.Error())
				}
				keys[i] = normalized
			}
			out["host-key"] = keys
		}
		if r.has("host-key-algorithms") {
			algorithms := r.strings("host-key-algorithms")
			for _, algorithm := range algorithms {
				if !oneOf(algorithm, "ssh-ed25519", "ssh-rsa", "rsa-sha2-256", "rsa-sha2-512", "ecdsa-sha2-nistp256", "ecdsa-sha2-nistp384", "ecdsa-sha2-nistp521") {
					r.fail("unsupported SSH host key algorithm")
				}
			}
			out["host-key-algorithms"] = algorithms
		}
	case "socks5", "http":
		if r.has("username") {
			out["username"] = r.text("username", true)
		}
		if r.has("password") {
			out["password"] = r.text("password", false)
			if !r.has("username") {
				r.fail("password requires a username")
			}
		}
		if strings.EqualFold(rawType, "https") || strings.EqualFold(rawType, "socks5s") {
			if r.has("tls") && !r.boolean("tls", false) {
				r.fail("TLS scheme conflicts with tls false")
			}
			r.values["tls"] = true
		}
		readTLS(r, out, protocol, false)
		if protocol == "http" && r.has("headers") {
			headers, err := cleanHeaders(r.object("headers"), false)
			if err != nil {
				r.fail(err.Error())
			}
			out["headers"] = headers
		}
	}
	if err := r.finish(); err != nil {
		return model.ParsedNode{}, err
	}
	return model.ParsedNode{Name: name, Protocol: protocol, Server: server, Port: port, UDP: udp, Config: out}, nil
}

func readPacketEncoding(r *fields, out map[string]any) {
	if r.has("packet-encoding") {
		v := r.text("packet-encoding", false)
		if !oneOf(v, "", "xudp", "packetaddr") {
			r.fail("unsupported packet encoding")
		}
		if v != "" {
			out["packet-encoding"] = v
		}
	}
}
func validateSS2022(r *fields, cipher, password string) {
	length := 32
	if cipher == "2022-blake3-aes-128-gcm" {
		length = 16
	}
	parts := strings.Split(password, ":")
	if cipher == "2022-blake3-chacha20-poly1305" && len(parts) > 1 {
		r.fail("unsupported Shadowsocks 2022 identity chain")
	}
	if len(parts) > 16 {
		r.fail("Shadowsocks 2022 identity chain is too long")
	}
	for _, p := range parts {
		b, err := base64.StdEncoding.Strict().DecodeString(p)
		if err != nil {
			b, err = base64.RawStdEncoding.Strict().DecodeString(p)
		}
		if err != nil || len(b) != length {
			r.fail("invalid Shadowsocks 2022 key length")
		}
	}
}
func readPortHopping(r *fields, out map[string]any, interval bool) {
	if r.has("ports") {
		s := r.text("ports", true)
		if !validatePortRanges(s) {
			r.fail("invalid hopping port ranges")
		}
		out["ports"] = s
	}
	if interval && r.has("hop-interval") {
		if !r.has("ports") {
			r.fail("hop interval requires hopping ports")
		}
		out["hop-interval"] = strconv.Itoa(r.integer("hop-interval", 30, 5, 86400))
	}
}
func readTLS(r *fields, out map[string]any, protocol string, implicit bool) {
	enabled := implicit
	if r.has("tls") {
		enabled = r.boolean("tls", false)
		if implicit && !enabled {
			r.fail("TLS cannot be disabled for this protocol")
		}
	}
	if !implicit {
		out["tls"] = enabled
	}
	serverKey := "sni"
	if protocol == "vless" || protocol == "vmess" {
		serverKey = "servername"
	}
	tlsKeys := []string{serverKey, "skip-cert-verify", "name-cert-verify", "fingerprint", "alpn", "client-fingerprint", "reality-opts"}
	for _, key := range tlsKeys {
		if r.has(key) && !enabled {
			r.fail("TLS options require TLS to be enabled")
		}
	}
	if r.has(serverKey) {
		if protocol == "socks5" {
			r.fail("SOCKS5 SNI overrides are unsupported")
		}
		value := r.text(serverKey, false)
		if value != "" {
			if _, ok := validHost(value); !ok {
				r.fail("invalid TLS server name")
			}
			out[serverKey] = value
		}
	}
	if enabled {
		out["skip-cert-verify"] = r.boolean("skip-cert-verify", false)
	}
	if r.has("name-cert-verify") {
		value := r.text("name-cert-verify", true)
		if _, ok := validHost(value); !ok {
			r.fail("invalid certificate verification name")
		}
		out["name-cert-verify"] = value
	}
	if r.has("fingerprint") {
		value, ok := normalizePin(r.text("fingerprint", true))
		if !ok {
			r.fail("invalid SHA-256 certificate fingerprint")
		}
		out["fingerprint"] = value
	}
	if r.has("alpn") {
		if protocol == "socks5" || protocol == "http" {
			r.fail("ALPN customization is unsupported for this protocol")
		}
		values := r.strings("alpn")
		for _, v := range values {
			if len(v) > 255 {
				r.fail("invalid ALPN protocol")
			}
		}
		out["alpn"] = values
	}
	if r.has("client-fingerprint") {
		if !oneOf(protocol, "vless", "vmess", "trojan") {
			r.fail("uTLS is unsupported for this protocol")
		}
		value := r.text("client-fingerprint", true)
		if !oneOf(value, "chrome", "firefox", "safari", "iOS", "android", "edge", "360", "qq", "random", "randomized") {
			r.fail("unsupported TLS client fingerprint")
		}
		out["client-fingerprint"] = value
	}
	if r.has("reality-opts") {
		if !oneOf(protocol, "vless", "vmess", "trojan") {
			r.fail("Reality is unsupported for this protocol")
		}
		reality := newFields(r.object("reality-opts"))
		key := reality.text("public-key", true)
		sid := reality.text("short-id", false)
		if !validPublicKey(key) || !validShortID(sid) {
			r.fail("invalid Reality public key or short ID")
		}
		if err := reality.finish(); err != nil {
			r.fail("unsupported or invalid Reality options")
		}
		if out["skip-cert-verify"] == true {
			r.fail("Reality cannot disable certificate verification")
		}
		if r.has("fingerprint") || r.has("name-cert-verify") {
			r.fail("certificate pin/name checks are not applied by the core Reality handshake")
		}
		if r.has("alpn") {
			r.fail("custom ALPN is not applied by the core Reality handshake")
		}
		out["reality-opts"] = map[string]any{"public-key": strings.TrimRight(key, "="), "short-id": strings.ToLower(sid)}
		if out["client-fingerprint"] == nil {
			out["client-fingerprint"] = "chrome"
		}
	}
}
