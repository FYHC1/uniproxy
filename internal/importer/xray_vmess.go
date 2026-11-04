package importer

import (
	"encoding/json"
	"net"
	"net/url"
	"strings"

	"github.com/uniproxy/uniproxy/internal/model"
)

// xrayVMessJSON reports whether a base64 VMess JSON link uses a transport that
// only Xray can run. VMess XHTTP is the first such case; keeping the decision
// next to the JSON parser avoids silently downgrading the link to Mihomo.
func xrayVMessJSON(obj map[string]any) bool {
	value, _ := obj["net"].(string)
	return strings.EqualFold(strings.TrimSpace(value), "xhttp")
}

// parseXrayVMessJSON converts the common VMess JSON shape into the same
// vnext/streamSettings template used by parseXrayURI. It intentionally accepts
// only fields that are meaningful to the Xray VMess outbound; unsupported
// exporter knobs fail rather than being silently dropped.
func parseXrayVMessJSON(obj map[string]any, fragment string) (model.ParsedNode, error) {
	r := newFields(obj)
	allowed := map[string]bool{
		"v": true, "ps": true, "add": true, "port": true, "id": true, "aid": true,
		"scy": true, "encryption": true, "net": true, "type": true, "host": true,
		"path": true, "tls": true, "security": true, "sni": true, "fp": true,
		"alpn": true, "allowInsecure": true, "insecure": true, "packetEncoding": true,
		"mode": true, "extra": true, "fm": true, "cs": true, "spx": true, "pqv": true,
		"pbk": true, "sid": true, "udp": true,
	}
	for key := range allowed {
		r.get(key)
	}
	for key := range obj {
		if !allowed[key] {
			r.fail("unsupported VMess URI option")
			break
		}
	}
	if value, ok := obj["v"]; ok && decimalString(value) != "2" {
		r.fail("unsupported VMess URI version")
	}
	name := r.text("ps", false)
	if fragment != "" {
		name = fragment
	}
	server := r.text("add", true)
	uuid := r.text("id", true)
	if !uuidPattern.MatchString(uuid) {
		r.fail("invalid UUID credential")
	}
	portValue, hasPort := obj["port"]
	port := decimalString(portValue)
	if !hasPort {
		r.fail("missing required port")
	} else if n, valid := integerValue(portValue); !valid || n < 1 || n > 65535 {
		r.fail("invalid proxy port")
	}
	if r.err != nil {
		return model.ParsedNode{}, r.err
	}

	q := url.Values{}
	if value, ok := obj["aid"]; ok {
		q.Set("alterId", decimalString(value))
	} else {
		q.Set("alterId", "0")
	}
	cipher := "auto"
	cipherSet := false
	if value := r.text("scy", false); value != "" {
		cipher = value
		cipherSet = true
	}
	if value := r.text("encryption", false); value != "" && !cipherSet {
		cipher = value
		cipherSet = true
	}
	securityValue := r.text("security", false)
	securityIsCipher := false
	if !cipherSet && oneOf(securityValue, "auto", "aes-128-gcm", "chacha20-poly1305", "none", "zero") {
		cipher = securityValue
		cipherSet = true
		securityIsCipher = true
	}
	q.Set("scy", cipher)
	if value := r.text("packetEncoding", false); value != "" {
		q.Set("packetEncoding", value)
	}
	network := r.text("net", false)
	if !strings.EqualFold(network, "xhttp") {
		r.fail("unsupported VMess Xray transport")
	} else {
		q.Set("type", "xhttp")
	}
	if value := r.text("type", false); value != "" && !strings.EqualFold(value, "none") {
		q.Set("headerType", value)
	}
	if value := r.text("host", false); value != "" {
		q.Set("host", value)
	}
	if value := r.text("path", false); value != "" {
		q.Set("path", value)
	}
	if value := r.text("mode", false); value != "" {
		q.Set("mode", value)
	}
	if value := r.text("sni", false); value != "" {
		q.Set("sni", value)
	}
	if value := r.text("fp", false); value != "" {
		q.Set("fp", value)
	}
	if value, ok := obj["alpn"]; ok {
		switch typed := value.(type) {
		case string:
			if typed != "" {
				q.Set("alpn", typed)
			}
		case []any:
			parts := make([]string, 0, len(typed))
			for _, item := range typed {
				if text, ok := item.(string); ok {
					parts = append(parts, text)
				} else {
					r.fail("invalid alpn")
				}
			}
			if len(parts) > 0 {
				q.Set("alpn", strings.Join(parts, ","))
			}
		default:
			r.fail("invalid alpn")
		}
	}
	if value, ok := obj["allowInsecure"]; ok {
		if b, valid := boolValue(value); valid {
			if b {
				q.Set("allowInsecure", "1")
			} else {
				q.Set("allowInsecure", "0")
			}
		} else {
			r.fail("invalid allowInsecure")
		}
	} else if value, ok := obj["insecure"]; ok {
		if b, valid := boolValue(value); valid {
			if b {
				q.Set("allowInsecure", "1")
			} else {
				q.Set("allowInsecure", "0")
			}
		} else {
			r.fail("invalid insecure")
		}
	}
	tlsValue := r.text("tls", false)
	if tlsValue != "" {
		switch strings.ToLower(tlsValue) {
		case "tls":
			q.Set("security", "tls")
		case "reality":
			q.Set("security", "reality")
		case "none", "false":
			q.Set("security", "none")
		default:
			r.fail("unsupported VMess TLS variant")
		}
	}
	if securityValue != "" && !securityIsCipher {
		switch strings.ToLower(securityValue) {
		case "tls":
			q.Set("security", "tls")
		case "reality":
			q.Set("security", "reality")
		case "none", "false":
			q.Set("security", "none")
		}
	}
	for _, key := range []string{"cs", "spx", "pqv", "pbk", "sid"} {
		if value := r.text(key, false); value != "" {
			q.Set(key, value)
		}
	}
	if value, ok := obj["fm"]; ok {
		switch typed := value.(type) {
		case string:
			if typed != "" {
				q.Set("fm", typed)
			}
		case map[string]any:
			encoded, err := json.Marshal(typed)
			if err != nil {
				r.fail("invalid finalmask")
			} else {
				q.Set("fm", string(encoded))
			}
		default:
			r.fail("invalid finalmask")
		}
	}
	if value, ok := obj["extra"]; ok {
		switch typed := value.(type) {
		case string:
			if typed != "" {
				q.Set("extra", typed)
			}
		case map[string]any:
			encoded, err := json.Marshal(typed)
			if err != nil {
				r.fail("invalid XHTTP extra")
			} else {
				q.Set("extra", string(encoded))
			}
		default:
			r.fail("invalid XHTTP extra")
		}
	}
	if value, ok := obj["udp"]; ok {
		if b, valid := boolValue(value); valid {
			if b {
				q.Set("udp", "1")
			} else {
				q.Set("udp", "0")
			}
		} else {
			r.fail("invalid udp")
		}
	}
	if err := r.finish(); err != nil {
		return model.ParsedNode{}, err
	}
	if r.err != nil {
		return model.ParsedNode{}, r.err
	}
	u := &url.URL{
		Scheme:   "vmess",
		Host:     net.JoinHostPort(server, port),
		User:     url.User(uuid),
		Fragment: name,
	}
	return parseXrayURI(u, "vmess", newQuery(q.Encode()))
}
