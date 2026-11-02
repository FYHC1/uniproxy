package importer

import (
	"encoding/json"
	"errors"
	"net/url"
	"strings"
)

import "github.com/uniproxy/uniproxy/internal/model"

func parseVMessJSON(text string) (model.ParsedNode, error) {
	payload := text[strings.Index(text, "://")+3:]
	fragment := ""
	if i := strings.IndexByte(payload, '#'); i >= 0 {
		var err error
		fragment, err = url.PathUnescape(payload[i+1:])
		if err != nil {
			return model.ParsedNode{}, errors.New("invalid VMess fragment encoding")
		}
		payload = payload[:i]
	}
	decoded, ok := decodeBase64(payload, false)
	if !ok || !json.Valid([]byte(decoded)) {
		return model.ParsedNode{}, errors.New("VMess URI requires a valid base64 JSON object")
	}
	root, err := documentNode(decoded)
	if err != nil {
		return model.ParsedNode{}, err
	}
	value, err := nodeValue(root)
	if err != nil {
		return model.ParsedNode{}, err
	}
	obj, ok := value.(map[string]any)
	if !ok {
		return model.ParsedNode{}, errors.New("VMess URI requires a JSON object")
	}
	r := newFields(obj)
	if v, ok := r.get("v"); ok && decimalString(v) != "2" {
		r.fail("unsupported VMess URI version")
	}
	name := r.text("ps", false)
	if fragment != "" {
		name = fragment
	}
	out := map[string]any{"type": "vmess", "name": name, "server": r.text("add", true), "uuid": r.text("id", true), "udp": true}
	if v, ok := r.get("port"); ok {
		out["port"] = v
	}
	if v, ok := r.get("aid"); ok {
		out["alterId"] = v
	}
	if v := r.text("scy", false); v != "" {
		out["cipher"] = v
	}
	tls := r.text("tls", false)
	if !oneOf(tls, "", "none", "tls") {
		r.fail("unsupported VMess TLS variant")
	}
	out["tls"] = tls == "tls"
	if sni := r.text("sni", false); sni != "" {
		out["servername"] = sni
	}
	if fp := r.text("fp", false); fp != "" {
		out["client-fingerprint"] = fp
	}
	if r.has("allowInsecure") {
		out["skip-cert-verify"] = r.boolean("allowInsecure", false)
	} else if r.has("insecure") {
		// v2rayN's VMess QR JSON uses "insecure" ("1"/"0"), while some
		// older exporters used allowInsecure. Accept both spellings.
		out["skip-cert-verify"] = r.boolean("insecure", false)
	}
	if alpn := r.text("alpn", false); alpn != "" {
		out["alpn"] = strings.Split(alpn, ",")
	}
	if r.has("packetEncoding") {
		out["packet-encoding"] = r.text("packetEncoding", false)
	}
	if vcn := r.text("vcn", false); vcn != "" {
		out["name-cert-verify"] = vcn
	}
	if pcs := r.text("pcs", false); pcs != "" {
		out["fingerprint"] = pcs
	}
	network := strings.ToLower(r.text("net", false))
	if network == "" || network == "raw" {
		network = "tcp"
	}
	if network == "mkcp" {
		network = "kcp"
	}
	kind := strings.ToLower(r.text("type", false))
	host := r.text("host", false)
	path := r.text("path", false)
	switch network {
	case "kcp":
		options := map[string]any{}
		if kind != "" && kind != "none" {
			options["header"] = kind
		}
		if path != "" {
			options["seed"] = path
		}
		if host != "" {
			r.fail("KCP transport does not support a host option")
		}
		out["network"] = "kcp"
		out["kcp-opts"] = options
	case "xhttp":
		r.fail("VMess XHTTP transport is unsupported by the pinned core")
	case "httpupgrade":
		if !oneOf(kind, "", "none", "gun") {
			r.fail("unsupported VMess transport header variant")
		}
		options := map[string]any{"v2ray-http-upgrade": true, "path": path}
		if path == "" {
			options["path"] = "/"
		}
		if host != "" {
			options["headers"] = map[string]any{"Host": host}
		}
		out["network"] = "ws"
		out["ws-opts"] = options
	case "ws":
		if !oneOf(kind, "", "none", "gun") {
			r.fail("unsupported VMess transport header variant")
		}
		options := map[string]any{"path": path}
		if host != "" {
			options["headers"] = map[string]any{"Host": host}
		}
		out["ws-opts"] = options
		out["network"] = network
	case "grpc":
		if !oneOf(kind, "", "none", "gun") {
			r.fail("unsupported VMess transport header variant")
		}
		if host != "" {
			r.fail("VMess gRPC authority override is unsupported")
		}
		out["grpc-opts"] = map[string]any{"grpc-service-name": path}
		out["network"] = network
	case "h2":
		if !oneOf(kind, "", "none", "gun") {
			r.fail("unsupported VMess transport header variant")
		}
		options := map[string]any{"path": path}
		if host != "" {
			options["host"] = strings.Split(host, ",")
		}
		out["h2-opts"] = options
		out["network"] = network
	case "http":
		if !oneOf(kind, "", "none", "gun") {
			r.fail("unsupported VMess transport header variant")
		}
		options := map[string]any{}
		if path != "" {
			options["path"] = strings.Split(path, ",")
		}
		if host != "" {
			options["headers"] = map[string]any{"Host": strings.Split(host, ",")}
		}
		out["http-opts"] = options
		out["network"] = network
	default:
		if kind != "" && kind != "none" {
			if kind == "http" && network == "tcp" {
				network = "http"
				out["network"] = network
				options := map[string]any{}
				if path != "" {
					options["path"] = strings.Split(path, ",")
				}
				if host != "" {
					options["headers"] = map[string]any{"Host": strings.Split(host, ",")}
				}
				out["http-opts"] = options
			} else {
				r.fail("unsupported VMess transport header variant")
			}
		} else {
			if host != "" || path != "" {
				r.fail("transport host or path would be ignored for this VMess transport")
			}
			out["network"] = network
		}
	}
	if err := r.finish(); err != nil {
		return model.ParsedNode{}, err
	}
	return normalizeNative(out)
}
