package importer

import (
	"errors"
	"strings"
	"time"
)

func fromSingbox(input map[string]any) (map[string]any, error) {
	r := newFields(input)
	rawType := r.text("type", true)
	protocol := canonicalProtocol(rawType)
	if !oneOf(rawType, "vless", "vmess", "trojan", "shadowsocks", "hysteria", "hysteria2", "tuic", "ssh", "socks", "http") || protocol == "" {
		return nil, errors.New("unsupported sing-box protocol or pseudo-outbound")
	}
	out := map[string]any{"type": protocol, "server": r.text("server", true), "name": r.text("tag", false)}
	if v, ok := r.get("server_port"); ok {
		out["port"] = v
	}
	if r.has("tcp_fast_open") {
		out["tfo"] = r.boolean("tcp_fast_open", false)
	}
	if r.has("domain_strategy") {
		strategy := r.text("domain_strategy", false)
		switch strategy {
		case "", "as_is":
		case "prefer_ipv4":
			out["ip-version"] = "ipv4-prefer"
		case "prefer_ipv6":
			out["ip-version"] = "ipv6-prefer"
		case "ipv4_only":
			out["ip-version"] = "ipv4"
		case "ipv6_only":
			out["ip-version"] = "ipv6"
		default:
			r.fail("unsupported sing-box domain strategy")
		}
	}
	if oneOf(protocol, "vless", "vmess", "trojan", "ss", "socks5") {
		out["udp"] = true
	}
	if r.has("network") {
		network := r.text("network", false)
		switch network {
		case "":
		case "tcp":
			if oneOf(protocol, "hysteria", "hysteria2", "tuic") {
				r.fail("TCP-only restriction cannot be represented for this protocol")
			}
			if protocol != "http" && protocol != "ssh" {
				out["udp"] = false
			}
		default:
			r.fail("unsupported sing-box network restriction")
		}
	}
	switch protocol {
	case "vless":
		out["uuid"] = r.text("uuid", true)
		if r.has("flow") {
			out["flow"] = r.text("flow", false)
		}
		if r.has("packet_encoding") {
			out["packet-encoding"] = r.text("packet_encoding", false)
		}
		singTLS(r, out, protocol, false)
		singTransport(r, out)
	case "vmess":
		out["uuid"] = r.text("uuid", true)
		if v, ok := r.get("alter_id"); ok {
			out["alterId"] = v
		}
		if r.has("security") {
			out["cipher"] = r.text("security", false)
		}
		if r.has("global_padding") {
			out["global-padding"] = r.boolean("global_padding", false)
		}
		if r.has("authenticated_length") {
			out["authenticated-length"] = r.boolean("authenticated_length", false)
		}
		if r.has("packet_encoding") {
			out["packet-encoding"] = r.text("packet_encoding", false)
		}
		singTLS(r, out, protocol, false)
		singTransport(r, out)
	case "trojan":
		out["password"] = r.text("password", true)
		singTLS(r, out, protocol, true)
		singTransport(r, out)
	case "ss":
		out["cipher"] = r.text("method", true)
		out["password"] = r.text("password", true)
		if r.has("plugin") {
			name := r.text("plugin", true)
			options := r.text("plugin_opts", false)
			name, mapped, err := pluginString(name + ";" + options)
			if err != nil {
				r.fail(err.Error())
			}
			out["plugin"] = name
			out["plugin-opts"] = mapped
		}
		if value, ok := r.get("udp_over_tcp"); ok {
			if b, ok := value.(bool); ok {
				out["udp-over-tcp"] = b
			} else if obj, ok := value.(map[string]any); ok {
				options := newFields(obj)
				enabled := options.boolean("enabled", false)
				out["udp-over-tcp"] = enabled
				if v, ok := options.get("version"); ok {
					out["udp-over-tcp-version"] = v
				}
				if err := options.finish(); err != nil {
					r.fail("invalid sing-box UDP-over-TCP options")
				}
			} else {
				r.fail("invalid sing-box UDP-over-TCP options")
			}
		}
	case "hysteria":
		if v, ok := r.get("up_mbps"); ok {
			out["up"] = v
		}
		if v, ok := r.get("down_mbps"); ok {
			out["down"] = v
		}
		if r.has("auth_str") {
			out["auth-str"] = r.text("auth_str", false)
		}
		if r.has("auth") {
			out["auth"] = r.text("auth", false)
		}
		if r.has("obfs") {
			out["obfs"] = r.text("obfs", false)
		}
		if r.has("recv_window_conn") {
			out["recv-window-conn"] = r.integer("recv_window_conn", 0, 1, 2147483647)
		}
		if r.has("recv_window") {
			out["recv-window"] = r.integer("recv_window", 0, 1, 2147483647)
		}
		if r.has("disable_mtu_discovery") {
			out["disable-mtu-discovery"] = r.boolean("disable_mtu_discovery", false)
		}
		singTLS(r, out, protocol, true)
	case "hysteria2":
		out["password"] = r.text("password", true)
		if v, ok := r.get("up_mbps"); ok {
			out["up"] = v
		}
		if v, ok := r.get("down_mbps"); ok {
			out["down"] = v
		}
		if r.has("obfs") {
			obfs := newFields(r.object("obfs"))
			out["obfs"] = obfs.text("type", true)
			out["obfs-password"] = obfs.text("password", true)
			if err := obfs.finish(); err != nil {
				r.fail("invalid sing-box Hysteria2 obfuscation")
			}
		}
		if r.has("server_ports") {
			ports := r.strings("server_ports")
			// sing-box uses ':' for port ranges; Mihomo uses '-'.
			for i, port := range ports {
				ports[i] = strings.ReplaceAll(port, ":", "-")
			}
			out["ports"] = strings.Join(ports, ",")
			if out["port"] == nil && len(ports) > 0 {
				out["port"] = strings.Split(ports[0], "-")[0]
			}
		}
		if r.has("hop_interval") {
			out["hop-interval"] = durationUnit(r, "hop_interval", time.Second)
		}
		singTLS(r, out, protocol, true)
	case "tuic":
		out["uuid"] = r.text("uuid", true)
		out["password"] = r.text("password", true)
		if r.has("congestion_control") {
			out["congestion-controller"] = r.text("congestion_control", false)
		}
		if r.has("udp_relay_mode") {
			out["udp-relay-mode"] = r.text("udp_relay_mode", false)
		}
		if r.has("zero_rtt_handshake") {
			out["reduce-rtt"] = r.boolean("zero_rtt_handshake", false)
		}
		if r.has("heartbeat") {
			out["heartbeat-interval"] = durationUnit(r, "heartbeat", time.Millisecond)
		}
		if r.has("udp_over_stream") && r.boolean("udp_over_stream", false) {
			r.fail("TUIC UDP-over-stream is unsupported")
		}
		singTLS(r, out, protocol, true)
	case "ssh":
		out["username"] = r.text("user", true)
		if r.has("password") {
			out["password"] = r.text("password", false)
		}
		if v, ok := r.get("private_key"); ok {
			if s, ok := v.(string); ok {
				out["private-key"] = s
			} else if list, ok := v.([]any); ok {
				lines := []string{}
				for _, item := range list {
					s, ok := item.(string)
					if !ok {
						r.fail("invalid inline SSH private key")
					}
					lines = append(lines, s)
				}
				out["private-key"] = strings.Join(lines, "\n")
			} else {
				r.fail("invalid inline SSH private key")
			}
		}
		if r.has("private_key_passphrase") {
			out["private-key-passphrase"] = r.text("private_key_passphrase", false)
		}
		if r.has("host_key") {
			out["host-key"] = r.strings("host_key")
		}
		if r.has("host_key_algorithms") {
			out["host-key-algorithms"] = r.strings("host_key_algorithms")
		}
	case "socks5":
		if r.has("version") && r.text("version", false) != "5" {
			r.fail("only SOCKS version 5 is supported")
		}
		if r.has("username") {
			out["username"] = r.text("username", false)
		}
		if r.has("password") {
			out["password"] = r.text("password", false)
		}
		// SOCKS5 sing-box has no TLS field. Any such field is rejected by finish.
	case "http":
		if r.has("username") {
			out["username"] = r.text("username", false)
		}
		if r.has("password") {
			out["password"] = r.text("password", false)
		}
		if r.has("headers") {
			headers := r.object("headers")
			mapped := map[string]any{}
			for key, v := range headers {
				if s, ok := v.(string); ok {
					mapped[key] = s
				} else if list, ok := v.([]any); ok && len(list) == 1 {
					mapped[key] = list[0]
				} else {
					r.fail("multiple HTTP proxy header values cannot be represented")
				}
			}
			out["headers"] = mapped
		}
		if r.has("path") && r.text("path", false) != "" {
			r.fail("HTTP proxy path overrides are unsupported")
		}
		singTLS(r, out, protocol, false)
	}
	if err := r.finish(); err != nil {
		return nil, err
	}
	return out, nil
}

func singTLS(r *fields, out map[string]any, protocol string, required bool) {
	tls := newFields(r.object("tls"))
	enabled := tls.boolean("enabled", false)
	if required && !enabled {
		tls.fail("TLS must be explicitly enabled for this sing-box protocol")
	}
	out["tls"] = enabled
	if !enabled {
		readDisabledSingTLS(tls)
		if err := tls.finish(); err != nil {
			r.fail(err.Error())
		}
		return
	}
	key := "sni"
	if protocol == "vless" || protocol == "vmess" {
		key = "servername"
	}
	if tls.has("server_name") {
		out[key] = tls.text("server_name", false)
	}
	if tls.has("insecure") {
		out["skip-cert-verify"] = tls.boolean("insecure", false)
	}
	if tls.has("alpn") {
		out["alpn"] = tls.strings("alpn")
	}
	if tls.has("disable_sni") {
		disabled := tls.boolean("disable_sni", false)
		if protocol == "tuic" {
			out["disable-sni"] = disabled
		} else if disabled {
			tls.fail("disabling SNI is unsupported for this protocol")
		}
	}
	if tls.has("utls") {
		utls := newFields(tls.object("utls"))
		active := utls.boolean("enabled", false)
		fp := utls.text("fingerprint", false)
		if active {
			if fp == "" {
				fp = "chrome"
			}
			out["client-fingerprint"] = fp
		} else if fp != "" {
			utls.fail("disabled uTLS contains active options")
		}
		if err := utls.finish(); err != nil {
			tls.fail("invalid or unsupported sing-box uTLS options")
		}
	}
	if tls.has("reality") {
		reality := newFields(tls.object("reality"))
		active := reality.boolean("enabled", false)
		if active {
			out["reality-opts"] = map[string]any{"public-key": reality.text("public_key", true), "short-id": reality.text("short_id", false)}
		}
		if err := reality.finish(); err != nil {
			tls.fail("invalid or unsupported sing-box Reality options")
		}
	}
	if err := tls.finish(); err != nil {
		r.fail(err.Error())
	}
}

// readDisabledSingTLS recognizes only explicit typed no-ops emitted by common
// exporters. Unknown fields, file references, active uTLS/Reality and nonempty
// verification settings remain errors; none are copied to a plaintext node.
func readDisabledSingTLS(tls *fields) {
	if tls.has("server_name") && tls.text("server_name", false) != "" {
		tls.fail("disabled TLS contains options that would be ignored")
	}
	for _, key := range []string{"insecure", "disable_sni"} {
		if tls.has(key) && tls.boolean(key, false) {
			tls.fail("disabled TLS contains options that would be ignored")
		}
	}
	for _, key := range []string{"utls", "reality"} {
		if !tls.has(key) {
			continue
		}
		options := newFields(tls.object(key))
		if options.boolean("enabled", false) {
			options.fail("disabled TLS contains active security options")
		}
		if key == "utls" && options.has("fingerprint") && options.text("fingerprint", false) != "" {
			options.fail("disabled uTLS contains active options")
		}
		if err := options.finish(); err != nil {
			tls.fail("disabled TLS contains invalid or active security options")
		}
	}
}

func singTransport(r *fields, out map[string]any) {
	if !r.has("transport") {
		return
	}
	transport := newFields(r.object("transport"))
	kind := transport.text("type", true)
	out["network"] = kind
	switch kind {
	case "ws":
		options := map[string]any{}
		if transport.has("path") {
			options["path"] = transport.text("path", false)
		}
		if transport.has("headers") {
			options["headers"] = transport.object("headers")
		}
		if v, ok := transport.get("max_early_data"); ok {
			options["max-early-data"] = v
		}
		if transport.has("early_data_header_name") {
			options["early-data-header-name"] = transport.text("early_data_header_name", false)
		}
		out["ws-opts"] = options
	case "grpc":
		options := map[string]any{"grpc-service-name": transport.text("service_name", false)}
		out["grpc-opts"] = options
	default:
		transport.fail("unsupported sing-box transport variant")
	}
	if err := transport.finish(); err != nil {
		r.fail(err.Error())
	}
}

func durationUnit(r *fields, key string, unit time.Duration) int {
	value := r.text(key, true)
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 || duration%unit != 0 || duration/unit > 2147483647 {
		r.fail("invalid or unrepresentable duration")
		return 0
	}
	return int(duration / unit)
}
