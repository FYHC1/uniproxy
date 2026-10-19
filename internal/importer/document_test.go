package importer

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func nativeFixture(protocol string) map[string]any {
	obj := map[string]any{"name": "node", "type": protocol, "server": "example.com", "port": 443}
	switch protocol {
	case "vless":
		obj["uuid"] = testUUID
		obj["udp"] = true
		obj["tls"] = true
	case "vmess":
		obj["uuid"] = testUUID
		obj["alterId"] = 0
		obj["cipher"] = "auto"
		obj["udp"] = true
	case "trojan":
		obj["password"] = "secret"
		obj["udp"] = true
	case "ss":
		obj["cipher"] = "aes-256-gcm"
		obj["password"] = "secret"
		obj["udp"] = true
	case "hysteria":
		obj["auth-str"] = "secret"
		obj["up"] = "20 Mbps"
		obj["down"] = "100 Mbps"
	case "hysteria2":
		obj["password"] = "secret"
	case "tuic":
		obj["uuid"] = testUUID
		obj["password"] = "secret"
	case "ssh":
		obj["username"] = "root"
		obj["password"] = "secret"
	case "socks5":
		obj["username"] = "user"
		obj["password"] = "secret"
		obj["udp"] = true
	case "http":
		obj["username"] = "user"
		obj["password"] = "secret"
	}
	return obj
}
func jsonDoc(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
func nativeDoc(t *testing.T, obj map[string]any) string {
	t.Helper()
	return jsonDoc(t, map[string]any{"proxies": []any{obj}})
}
func singDoc(t *testing.T, obj map[string]any) string {
	t.Helper()
	return jsonDoc(t, map[string]any{"outbounds": []any{obj}})
}

func TestClashYAMLAndJSONEveryProtocol(t *testing.T) {
	for _, protocol := range []string{"vless", "vmess", "trojan", "ss", "hysteria", "hysteria2", "tuic", "ssh", "socks5", "http"} {
		t.Run(protocol, func(t *testing.T) {
			obj := nativeFixture(protocol)
			doc := map[string]any{"proxies": []any{obj}, "mode": "rule", "rules": []string{"MATCH,PROXY"}, "proxy-groups": []any{map[string]any{"name": "PROXY", "type": "select", "proxies": []string{"node"}}}}
			yamlBytes, err := yaml.Marshal(doc)
			if err != nil {
				t.Fatal(err)
			}
			jsonNode := parseOne(t, jsonDoc(t, doc))
			yamlNode := parseOne(t, string(yamlBytes))
			if !reflect.DeepEqual(jsonNode, yamlNode) || jsonNode.Protocol != protocol {
				t.Fatal("YAML and JSON parsing diverged")
			}
			standalone := parseOne(t, jsonDoc(t, obj))
			sequence := parseOne(t, jsonDoc(t, []any{obj}))
			if !reflect.DeepEqual(jsonNode, standalone) || !reflect.DeepEqual(jsonNode, sequence) {
				t.Fatal("standalone or array parsing diverged")
			}
			// Configs survive JSON persistence and can be exported back to Clash.
			copy := map[string]any{}
			for key, value := range jsonNode.Config {
				copy[key] = value
			}
			copy["name"] = "renamed"
			roundTrip := parseOne(t, nativeDoc(t, copy))
			if Fingerprint(roundTrip) != Fingerprint(jsonNode) {
				t.Fatal("native map round trip changed identity")
			}
		})
	}
}
func TestNativeOptions(t *testing.T) {
	t.Run("ws", func(t *testing.T) {
		obj := nativeFixture("vmess")
		obj["network"] = "ws"
		obj["tls"] = true
		obj["ws-opts"] = map[string]any{"path": "/w", "headers": map[string]any{"host": "front.example", "X-Test": "value"}, "max-early-data": 1024, "early-data-header-name": "Sec-WebSocket-Protocol"}
		n := parseOne(t, nativeDoc(t, obj))
		ws := n.Config["ws-opts"].(map[string]any)
		if ws["max-early-data"] != 1024 || ws["headers"].(map[string]any)["Host"] != "front.example" {
			t.Fatal("WebSocket options lost")
		}
	})
	t.Run("grpc", func(t *testing.T) {
		obj := nativeFixture("vless")
		obj["network"] = "grpc"
		obj["grpc-opts"] = map[string]any{"grpc-service-name": "hello", "grpc-user-agent": "agent"}
		n := parseOne(t, nativeDoc(t, obj))
		if n.Config["grpc-opts"].(map[string]any)["grpc-user-agent"] != "agent" {
			t.Fatal("gRPC user agent lost")
		}
	})
	t.Run("tls-reality", func(t *testing.T) {
		obj := nativeFixture("vless")
		obj["reality-opts"] = map[string]any{"public-key": testPublicKey, "short-id": "ab12"}
		obj["flow"] = "xtls-rprx-vision"
		obj["servername"] = "front.example"
		n := parseOne(t, nativeDoc(t, obj))
		if n.Config["client-fingerprint"] != "chrome" || n.Config["skip-cert-verify"] != false {
			t.Fatal("safe Reality defaults missing")
		}
	})
	t.Run("tls-pin", func(t *testing.T) {
		obj := nativeFixture("http")
		obj["tls"] = true
		obj["fingerprint"] = strings.TrimSuffix(strings.Repeat("AB:", 32), ":")
		n := parseOne(t, nativeDoc(t, obj))
		if n.Config["fingerprint"] != strings.Repeat("ab", 32) {
			t.Fatal("pin normalization wrong")
		}
	})
	t.Run("ssh-inline-key", func(t *testing.T) {
		obj := nativeFixture("ssh")
		delete(obj, "password")
		key := testSSHPrivatePEM(t)
		obj["private-key"] = key
		obj["host-key"] = []string{"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJjlL7Hx1LhvVClSvFVoRKJBKSjPxNMIZmsIXKuGyMGv"}
		obj["host-key-algorithms"] = []string{"ssh-ed25519"}
		n := parseOne(t, nativeDoc(t, obj))
		if n.Config["private-key"] != key {
			t.Fatal("inline SSH key changed")
		}
	})
	t.Run("hy1-binary-auth", func(t *testing.T) {
		obj := nativeFixture("hysteria")
		delete(obj, "auth-str")
		obj["auth"] = base64.StdEncoding.EncodeToString([]byte{0, 1, 2, 3})
		obj["recv-window"] = 4096
		obj["recv-window-conn"] = 8192
		obj["disable_mtu_discovery"] = true
		n := parseOne(t, nativeDoc(t, obj))
		if n.Config["auth"] != obj["auth"] {
			t.Fatal("binary auth lost")
		}
	})
	t.Run("tuic-v4", func(t *testing.T) {
		obj := nativeFixture("tuic")
		delete(obj, "uuid")
		delete(obj, "password")
		obj["token"] = "token"
		parseOne(t, nativeDoc(t, obj))
	})
	t.Run("http-headers", func(t *testing.T) {
		obj := nativeFixture("http")
		obj["headers"] = map[string]any{"X-Token": "secret", "Host": "front.example"}
		n := parseOne(t, nativeDoc(t, obj))
		if n.Config["headers"].(map[string]any)["X-Token"] != "secret" {
			t.Fatal("HTTP headers changed")
		}
	})
	t.Run("explicit-udp-disabled", func(t *testing.T) {
		obj := nativeFixture("ss")
		obj["udp"] = false
		n := parseOne(t, nativeDoc(t, obj))
		if n.UDP {
			t.Fatal("UDP disabled flag ignored")
		}
	})
	t.Run("uot", func(t *testing.T) {
		obj := nativeFixture("ss")
		obj["udp-over-tcp"] = true
		obj["udp-over-tcp-version"] = 2
		n := parseOne(t, nativeDoc(t, obj))
		if n.Config["udp-over-tcp"] != true {
			t.Fatal("UOT lost")
		}
	})
	t.Run("aliases", func(t *testing.T) {
		for alias, canonical := range map[string]string{"shadowsocks": "ss", "hy2": "hysteria2", "https": "http", "socks": "socks5"} {
			obj := nativeFixture(canonical)
			obj["type"] = alias
			n := parseOne(t, nativeDoc(t, obj))
			if n.Protocol != canonical {
				t.Fatal("protocol alias lost")
			}
			if alias == "https" && n.Config["tls"] != true {
				t.Fatal("https downgraded")
			}
		}
	})
}
func TestBase64Subscriptions(t *testing.T) {
	content := "# subscription\r\n\r\nvless://" + testUUID + "@example.com:443#First\r\ntrojan://password@example.com:443#Second\r\n"
	for i, encoding := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			encoded := encoding.EncodeToString([]byte(content))
			wrapped := encoded[:20] + "\n\t" + encoded[20:]
			result := Parse(wrapped)
			if len(result.Errors) != 0 || len(result.Nodes) != 2 {
				t.Fatalf("base64 subscription rejected: %+v", result.Errors)
			}
			if result.Nodes[1].Config["password"] != "password" {
				t.Fatal("base64 credential changed")
			}
		})
	}
	yamlContent := "proxies:\n  - type: http\n    server: example.com\n    port: 8080\n"
	parseOne(t, base64.StdEncoding.EncodeToString([]byte(yamlContent)))
	content = "http://example.com:8080"
	for depth := 0; depth < maxBase64Depth; depth++ {
		content = base64.StdEncoding.EncodeToString([]byte(content))
		parseOne(t, content)
	}
	content = base64.StdEncoding.EncodeToString([]byte(content))
	requireInvalid(t, content)
}
func TestPartialInputAndLineNumbers(t *testing.T) {
	content := "# note\n\nnot-a-link\ntrojan://password@example.com:443\nunknown://secret@example.com:10\nhttp://example.com:8080\n"
	result := Parse(content)
	if len(result.Nodes) != 2 || len(result.Errors) != 2 || result.Errors[0].Line != 3 || result.Errors[1].Line != 5 {
		t.Fatalf("partial URI recovery/line numbers failed: %+v", result.Errors)
	}
	yamlContent := "proxies:\n  - type: http\n    server: good.example\n    port: 80\n  - type: unsupported\n    server: bad.example\n    port: 80\n  - type: http\n    server: good2.example\n    port: 81\n"
	result = Parse(yamlContent)
	if len(result.Nodes) != 2 || len(result.Errors) != 1 || result.Errors[0].Line != 5 {
		t.Fatalf("partial YAML recovery/line number failed: %+v", result.Errors)
	}
	for _, empty := range []string{"", " \r\n", "# only comments\n"} {
		r := Parse(empty)
		if r.Nodes == nil || r.Errors == nil {
			t.Fatal("collections must not be null")
		}
		requireInvalid(t, empty)
	}
	for _, empty := range []string{"proxies: []", `{"outbounds":[]}`, "[]"} {
		r := Parse(empty)
		if len(r.Nodes) != 0 || len(r.Errors) != 0 || r.Nodes == nil || r.Errors == nil {
			t.Fatal("empty explicit collection should allow caller shrink policy")
		}
	}
	n := parseOne(t, "\ufeffhttp://example.com:8080#東京%20server")
	if n.Name != "東京 server" {
		t.Fatal("BOM/UTF-8 label handling failed")
	}
}

func TestLongDisplayNameIsShortenedNotDropped(t *testing.T) {
	// Upstream labels are presentation only; identity is the fingerprint. A label
	// longer than the management API ceiling must not silently drop the node, and
	// preview must show exactly the label that would be stored.
	for _, name := range []string{strings.Repeat("长", 161), strings.Repeat("a", 300)} {
		node := parseOne(t, "socks5://u:p@127.0.0.1:1080#"+url.PathEscape(name))
		if runes := []rune(node.Name); len(runes) != MaxNodeNameRunes || runes[len(runes)-1] != '…' {
			t.Fatalf("label not shortened on a rune boundary: %d runes", len(runes))
		}
		if node.Server != "127.0.0.1" || node.Port != 1080 {
			t.Fatal("shortening a label must not change routing metadata")
		}
	}
	// A short label is preserved byte-for-byte, including multi-byte characters.
	if node := parseOne(t, "socks5://u:p@127.0.0.1:1080#"+url.PathEscape("东京节点")); node.Name != "东京节点" {
		t.Fatalf("short label changed: %q", node.Name)
	}
	// Implausible labels stay rejected instead of being silently truncated.
	result := Parse("socks5://u:p@127.0.0.1:1080#" + url.PathEscape(strings.Repeat("a", 1025)))
	if len(result.Nodes) != 0 || len(result.Errors) != 1 {
		t.Fatalf("oversized label should remain invalid: %+v", result.Errors)
	}
}
