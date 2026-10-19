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
	}
	if alpn := r.text("alpn", false); alpn != "" {
		out["alpn"] = strings.Split(alpn, ",")
	}
	if r.has("packetEncoding") {
		out["packet-encoding"] = r.text("packetEncoding", false)
	}
	network := r.text("net", false)
	if network == "" {
		network = "tcp"
	}
	kind := r.text("type", false)
	if network == "tcp" && kind == "http" {
		network = "http"
	} else if !oneOf(kind, "", "none", "gun") {
		r.fail("unsupported VMess transport header variant")
	}
	host := r.text("host", false)
	path := r.text("path", false)
	out["network"] = network
	switch network {
	case "ws":
		options := map[string]any{"path": path}
		if host != "" {
			options["headers"] = map[string]any{"Host": host}
		}
		out["ws-opts"] = options
	case "grpc":
		if host != "" {
			r.fail("VMess gRPC authority override is unsupported")
		}
		out["grpc-opts"] = map[string]any{"grpc-service-name": path}
	case "h2":
		options := map[string]any{"path": path}
		if host != "" {
			options["host"] = strings.Split(host, ",")
		}
		out["h2-opts"] = options
	case "http":
		options := map[string]any{}
		if path != "" {
			options["path"] = strings.Split(path, ",")
		}
		if host != "" {
			options["headers"] = map[string]any{"Host": strings.Split(host, ",")}
		}
		out["http-opts"] = options
	default:
		if host != "" || path != "" {
			r.fail("transport host or path would be ignored for this VMess transport")
		}
	}
	if err := r.finish(); err != nil {
		return model.ParsedNode{}, err
	}
	return normalizeNative(out)
}
