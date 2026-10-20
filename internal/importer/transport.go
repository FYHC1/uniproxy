package importer

import "strings"

func readTransport(r *fields, out map[string]any, protocol string) {
	network := r.text("network", false)
	if network == "" {
		network = "tcp"
	}
	if !oneOf(network, "tcp", "ws", "grpc", "http", "h2") || protocol == "trojan" && !oneOf(network, "tcp", "ws", "grpc") {
		r.fail("unsupported transport; transport will not be silently downgraded")
	}
	out["network"] = network
	// The pinned core's WebSocket branches do not use Reality at all. Accepting
	// this combination would silently replace Reality authentication with TLS.
	if network == "ws" && out["reality-opts"] != nil {
		r.fail("Reality over WebSocket is unsupported by the pinned core")
	}
	if protocols, ok := out["alpn"].([]string); ok {
		required := ""
		if network == "h2" || network == "grpc" {
			required = "h2"
		}
		if network == "ws" && protocol != "trojan" {
			required = "http/1.1"
		}
		if required != "" && (len(protocols) != 1 || protocols[0] != required) {
			r.fail("configured ALPN would be overridden by this transport")
		}
	}
	switch network {
	case "ws":
		ws := newFields(r.object("ws-opts"))
		options := map[string]any{}
		path := ws.text("path", false)
		if path == "" {
			path = "/"
		}
		if !strings.HasPrefix(path, "/") || containsControl(path) || len(path) > 8192 {
			ws.fail("invalid WebSocket path")
		}
		options["path"] = path
		if ws.has("headers") {
			headers, err := cleanHeaders(ws.object("headers"), false)
			if err != nil {
				ws.fail(err.Error())
			}
			options["headers"] = headers
		}
		copyInt(ws, options, "max-early-data", 0, 65535)
		if ws.has("early-data-header-name") {
			header := ws.text("early-data-header-name", true)
			if !headerName(header) {
				ws.fail("invalid early data header name")
			}
			options["early-data-header-name"] = header
		}
		for _, key := range []string{"v2ray-http-upgrade", "v2ray-http-upgrade-fast-open"} {
			if ws.has(key) {
				if ws.boolean(key, false) {
					ws.fail("WebSocket HTTP upgrade variants are unsupported")
				}
			}
		}
		if err := ws.finish(); err != nil {
			r.fail(err.Error())
		}
		out["ws-opts"] = options
	case "grpc":
		grpc := newFields(r.object("grpc-opts"))
		options := map[string]any{}
		service := grpc.text("grpc-service-name", false)
		if containsControl(service) || len(service) > 8192 {
			grpc.fail("invalid gRPC service name")
		}
		options["grpc-service-name"] = service
		if grpc.has("grpc-user-agent") {
			value := grpc.text("grpc-user-agent", false)
			if containsControl(value) {
				grpc.fail("invalid gRPC user agent")
			}
			options["grpc-user-agent"] = value
		}
		if err := grpc.finish(); err != nil {
			r.fail(err.Error())
		}
		out["grpc-opts"] = options
	case "http":
		http := newFields(r.object("http-opts"))
		options := map[string]any{}
		method := http.text("method", false)
		if method == "" {
			method = "GET"
		}
		if !headerName(method) {
			http.fail("invalid HTTP transport method")
		}
		options["method"] = method
		paths := http.strings("path")
		if len(paths) == 0 {
			paths = []string{"/"}
		}
		for _, path := range paths {
			if !strings.HasPrefix(path, "/") {
				http.fail("invalid HTTP transport path")
			}
		}
		options["path"] = paths
		if http.has("headers") {
			headers, err := cleanHeaders(http.object("headers"), true)
			if err != nil {
				http.fail(err.Error())
			}
			options["headers"] = headers
		}
		if err := http.finish(); err != nil {
			r.fail(err.Error())
		}
		out["http-opts"] = options
	case "h2":
		h2 := newFields(r.object("h2-opts"))
		options := map[string]any{}
		if out["tls"] != true {
			h2.fail("HTTP/2 transport requires TLS")
		}
		path := h2.text("path", false)
		if path == "" {
			path = "/"
		}
		if !strings.HasPrefix(path, "/") || containsControl(path) {
			h2.fail("invalid HTTP/2 path")
		}
		options["path"] = path
		hosts := h2.strings("host")
		for _, host := range hosts {
			if _, ok := validHost(host); !ok {
				h2.fail("invalid HTTP/2 host")
			}
		}
		if len(hosts) > 0 {
			options["host"] = hosts
		}
		if err := h2.finish(); err != nil {
			r.fail(err.Error())
		}
		out["h2-opts"] = options
	}
}

func readPlugin(r *fields, out map[string]any) {
	if !r.has("plugin") {
		return
	}
	plugin := r.text("plugin", true)
	if plugin == "obfs-local" || plugin == "simple-obfs" {
		plugin = "obfs"
	}
	p := newFields(r.object("plugin-opts"))
	options := map[string]any{}
	switch plugin {
	case "obfs":
		mode := p.text("mode", true)
		if !oneOf(mode, "http", "tls") {
			p.fail("unsupported Shadowsocks obfs mode")
		}
		options["mode"] = mode
		if p.has("host") {
			host := p.text("host", true)
			if _, ok := validHost(host); !ok {
				p.fail("invalid obfs host")
			}
			options["host"] = host
		}
	case "v2ray-plugin":
		mode := p.text("mode", false)
		if mode == "" {
			mode = "websocket"
		}
		if mode != "websocket" {
			p.fail("unsupported Shadowsocks plugin transport")
		}
		options["mode"] = mode
		tls := p.boolean("tls", false)
		options["tls"] = tls
		if p.has("skip-cert-verify") {
			insecure := p.boolean("skip-cert-verify", false)
			if !tls && insecure {
				p.fail("plugin TLS options require TLS")
			}
			if tls {
				options["skip-cert-verify"] = insecure
			}
		}
		if p.has("name-cert-verify") {
			value := p.text("name-cert-verify", true)
			if _, ok := validHost(value); !ok || !tls {
				p.fail("invalid plugin certificate verification name")
			}
			options["name-cert-verify"] = value
		}
		if p.has("fingerprint") {
			pin, ok := normalizePin(p.text("fingerprint", true))
			if !ok || !tls {
				p.fail("invalid plugin certificate fingerprint")
			}
			options["fingerprint"] = pin
		}
		if p.has("host") {
			host := p.text("host", true)
			if _, ok := validHost(host); !ok {
				p.fail("invalid plugin host")
			}
			options["host"] = host
		}
		path := p.text("path", false)
		if path == "" {
			path = "/"
		}
		if !strings.HasPrefix(path, "/") || containsControl(path) {
			p.fail("invalid plugin WebSocket path")
		}
		options["path"] = path
		copyBool(p, options, "mux")
		if p.has("headers") {
			headers, err := cleanHeaders(p.object("headers"), false)
			if err != nil {
				p.fail(err.Error())
			}
			options["headers"] = headers
		}
		if p.has("v2ray-http-upgrade") && p.boolean("v2ray-http-upgrade", false) {
			p.fail("unsupported plugin HTTP upgrade variant")
		}
	default:
		p.fail("unsupported or executable Shadowsocks plugin")
	}
	if err := p.finish(); err != nil {
		r.fail(err.Error())
	}
	out["plugin"] = plugin
	out["plugin-opts"] = options
}
