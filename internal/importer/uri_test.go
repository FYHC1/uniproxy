package importer

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/uniproxy/uniproxy/internal/model"
)

const testUUID = "12345678-1234-4234-8234-123456789abc"

var testPublicKey = base64.RawURLEncoding.EncodeToString([]byte("01234567890123456789012345678901"))

func parseOne(t *testing.T, content string) model.ParsedNode {
	t.Helper()
	result := Parse(content)
	if len(result.Errors) != 0 || len(result.Nodes) != 1 {
		t.Fatalf("expected one node without errors: nodes=%d errors=%+v", len(result.Nodes), result.Errors)
	}
	node := result.Nodes[0]
	if _, ok := node.Config["name"]; ok {
		t.Fatal("native config must not contain untrusted name")
	}
	if node.Config["type"] != node.Protocol || node.Config["server"] != node.Server || node.Config["port"] != node.Port {
		t.Fatal("native config disagrees with metadata")
	}
	if len(Fingerprint(node)) != 64 {
		t.Fatal("missing stable fingerprint")
	}
	return node
}
func requireInvalid(t *testing.T, content string) {
	t.Helper()
	result := Parse(content)
	if len(result.Errors) == 0 || len(result.Nodes) != 0 {
		t.Fatalf("expected rejected input; nodes=%d errors=%+v", len(result.Nodes), result.Errors)
	}
	for _, issue := range result.Errors {
		if issue.Line < 1 || issue.Message == "" {
			t.Fatal("invalid diagnostic")
		}
	}
}
func vmessLink(obj map[string]any) string {
	data, _ := json.Marshal(obj)
	return "vmess://" + base64.StdEncoding.EncodeToString(data)
}
func vmessBase() map[string]any {
	return map[string]any{"v": "2", "ps": "VMess 東京", "add": "Example.COM", "port": "443", "id": testUUID, "aid": "0", "scy": "auto", "net": "tcp", "type": "none", "tls": "tls", "sni": "tls.example"}
}

func TestURIProtocols(t *testing.T) {
	tests := []struct {
		name, uri, protocol, credentialKey, credential string
		port                                           int
		udp                                            bool
	}{
		{"vless", "vless://" + testUUID + "@Example.COM:443?security=tls&sni=tls.example#Node%20one", "vless", "uuid", testUUID, 443, true},
		{"vmess-json", vmessLink(vmessBase()), "vmess", "uuid", testUUID, 443, true},
		{"vmess-authority", "vmess://" + testUUID + "@example.com:8080?encryption=auto", "vmess", "uuid", testUUID, 8080, true},
		{"trojan", "trojan://p%40ss%3Aword%2B%25@example.com:443#T", "trojan", "password", "p@ss:word+%", 443, true},
		{"shadowsocks", "ss://" + base64.RawURLEncoding.EncodeToString([]byte("aes-256-gcm:p@ss:word+/=")) + "@example.com:8388#SS", "ss", "password", "p@ss:word+/=", 8388, true},
		{"hysteria1", "hysteria://example.com:443?auth=p%40ss%3Aword&upmbps=30&downmbps=100&peer=tls.example", "hysteria", "auth-str", "p@ss:word", 443, true},
		{"hysteria2", "hysteria2://p%40ss%3Aword@example.com:443/?sni=tls.example", "hysteria2", "password", "p@ss:word", 443, true},
		{"tuic-v5", "tuic://" + testUUID + ":p%40ss%3Aword@example.com:443?congestion_control=bbr&udp_relay_mode=native", "tuic", "password", "p@ss:word", 443, true},
		{"tuic-v4", "tuic://secret-token@example.com:443?version=4", "tuic", "token", "secret-token", 443, true},
		{"ssh", "ssh://u%40ser:p%40ss%3Aword@example.com#ssh", "ssh", "password", "p@ss:word", 22, false},
		{"socks5", "socks5://u%40ser:p%40ss%3Aword@example.com:1080?udp=true", "socks5", "password", "p@ss:word", 1080, true},
		{"http", "http://u%40ser:p%40ss%3Aword@example.com", "http", "password", "p@ss:word", 80, false},
		{"https", "https://u:p@example.com", "http", "password", "p", 443, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := parseOne(t, tt.uri)
			if n.Protocol != tt.protocol || n.Port != tt.port || n.Server != "example.com" || n.UDP != tt.udp {
				t.Fatalf("wrong metadata: protocol=%s port=%d server=%s udp=%v", n.Protocol, n.Port, n.Server, n.UDP)
			}
			if n.Config[tt.credentialKey] != tt.credential {
				t.Fatal("credentials changed")
			}
			if n.Name == "" {
				t.Fatal("missing display name")
			}
		})
	}
}
func TestURIAliasesAndIPv6(t *testing.T) {
	tests := []struct{ uri, protocol string }{
		{"VLESS://" + testUUID + "@[2001:db8::1]:443", "vless"},
		{"shadowsocks://aes-128-gcm:password@[2001:db8::1]:8388", "ss"},
		{"hy://[2001:db8::1]:443?up=10&down=20&auth=a", "hysteria"},
		{"hysteria1://[2001:db8::1]:443?up=10&down=20&auth=a", "hysteria"},
		{"hy1://[2001:db8::1]:443?up=10&down=20&auth=a", "hysteria"},
		{"hy2://password@[2001:db8::1]", "hysteria2"},
		{"socks://[2001:db8::1]:1080", "socks5"},
		{"socks5h://[2001:db8::1]:1080", "socks5"},
		{"socks5s://u:p@[2001:db8::1]:1080", "socks5"},
		{"https://[2001:db8::1]", "http"},
		{"ssh://u:p@[2001:db8::1]", "ssh"},
	}
	for i, tt := range tests {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			n := parseOne(t, tt.uri)
			if n.Protocol != tt.protocol || n.Server != "2001:db8::1" {
				t.Fatalf("incorrect alias or IPv6: %+v", n)
			}
		})
	}
}
func TestTLSRealityAndTransports(t *testing.T) {
	t.Run("reality", func(t *testing.T) {
		n := parseOne(t, "vless://"+testUUID+"@example.com:443?security=reality&pbk="+testPublicKey+"&sid=ABCD&fp=chrome&sni=tls.example&flow=xtls-rprx-vision&packetEncoding=xudp")
		if n.Config["tls"] != true || n.Config["servername"] != "tls.example" || n.Config["skip-cert-verify"] != false || n.Config["flow"] != "xtls-rprx-vision" || n.Config["packet-encoding"] != "xudp" {
			t.Fatal("Reality security fields not preserved")
		}
		if !reflect.DeepEqual(n.Config["reality-opts"], map[string]any{"public-key": testPublicKey, "short-id": "abcd"}) {
			t.Fatal("Reality options not preserved")
		}
	})
	t.Run("websocket", func(t *testing.T) {
		n := parseOne(t, "vless://"+testUUID+"@example.com:443?security=tls&type=ws&host=front.example&path=%2Fa%3Fed%3D2048&ed=2048&eh=Sec-WebSocket-Protocol&alpn=http%2F1.1")
		expected := map[string]any{"path": "/a?ed=2048", "headers": map[string]any{"Host": "front.example"}, "max-early-data": 2048, "early-data-header-name": "Sec-WebSocket-Protocol"}
		if !reflect.DeepEqual(n.Config["ws-opts"], expected) || n.Config["network"] != "ws" {
			t.Fatal("WebSocket mapping incorrect")
		}
		if !reflect.DeepEqual(n.Config["alpn"], []string{"http/1.1"}) {
			t.Fatal("ALPN changed")
		}
	})
	t.Run("grpc", func(t *testing.T) {
		n := parseOne(t, "trojan://password@example.com:443?type=grpc&serviceName=my%2Fservice&mode=gun&sni=tls.example")
		if !reflect.DeepEqual(n.Config["grpc-opts"], map[string]any{"grpc-service-name": "my/service"}) || n.Config["sni"] != "tls.example" {
			t.Fatal("gRPC mapping incorrect")
		}
	})
	t.Run("http-header", func(t *testing.T) {
		n := parseOne(t, "vmess://"+testUUID+"@example.com:80?type=tcp&headerType=http&host=front.example&path=%2Fa,%2Fb")
		if n.Config["network"] != "http" {
			t.Fatal("HTTP obfuscation mapping incorrect")
		}
	})
	t.Run("h2", func(t *testing.T) {
		n := parseOne(t, "vless://"+testUUID+"@example.com:443?security=tls&type=h2&host=front.example&path=%2Fh2")
		if n.Config["network"] != "h2" {
			t.Fatal("HTTP/2 mapping incorrect")
		}
	})
	t.Run("certificate-pin", func(t *testing.T) {
		n := parseOne(t, "hy2://p@example.com?pinSHA256="+strings.Repeat("ab", 32))
		if n.Config["fingerprint"] != strings.Repeat("ab", 32) {
			t.Fatal("certificate pin lost")
		}
	})
	t.Run("explicit-insecure", func(t *testing.T) {
		n := parseOne(t, "trojan://p@example.com:443?allowInsecure=1")
		if n.Config["skip-cert-verify"] != true {
			t.Fatal("explicit insecure choice lost")
		}
	})
}
func TestVMessVariants(t *testing.T) {
	for _, network := range []string{"tcp", "ws", "grpc", "h2", "http"} {
		t.Run(network, func(t *testing.T) {
			obj := vmessBase()
			obj["net"] = network
			if network == "ws" || network == "h2" || network == "http" {
				obj["host"] = "front.example"
				obj["path"] = "/path"
			}
			if network == "grpc" {
				obj["path"] = "service"
			}
			n := parseOne(t, vmessLink(obj))
			if n.Config["network"] != network || n.Name != "VMess 東京" {
				t.Fatal("VMess transport or name lost")
			}
		})
	}
	t.Run("fragment-label", func(t *testing.T) {
		n := parseOne(t, vmessLink(vmessBase())+"#Override%20name")
		if n.Name != "Override name" {
			t.Fatal("VMess label override not decoded")
		}
	})
	for _, mutation := range []func(map[string]any){
		func(m map[string]any) { m["v"] = "3" }, func(m map[string]any) { m["net"] = "kcp" }, func(m map[string]any) { m["tls"] = "xtls" }, func(m map[string]any) { m["host"] = "ignored.example" }, func(m map[string]any) { m["type"] = "srtp" }, func(m map[string]any) { m["scy"] = "unknown" }, func(m map[string]any) { m["id"] = "not-a-uuid" }, func(m map[string]any) { m["dialer-proxy"] = "other" }, func(m map[string]any) { m["port"] = 1.5 },
	} {
		obj := vmessBase()
		mutation(obj)
		requireInvalid(t, vmessLink(obj))
	}
}
func TestShadowsocksFormatsAndPlugins(t *testing.T) {
	password := "p:@/+# ?秘密"
	auth := "aes-256-gcm:" + password
	forms := []string{
		"ss://" + base64.StdEncoding.EncodeToString([]byte(auth+"@[2001:db8::1]:8388")),
		"ss://" + base64.RawStdEncoding.EncodeToString([]byte(auth)) + "@[2001:db8::1]:8388",
		"ss://" + base64.RawURLEncoding.EncodeToString([]byte(auth)) + "@[2001:db8::1]:8388",
		"ss://" + url.UserPassword("aes-256-gcm", password).String() + "@[2001:db8::1]:8388/",
	}
	for i, form := range forms {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			n := parseOne(t, form)
			if n.Config["password"] != password || n.Server != "2001:db8::1" {
				t.Fatal("Shadowsocks credentials or IPv6 changed")
			}
		})
	}
	t.Run("obfs", func(t *testing.T) {
		n := parseOne(t, "ss://aes-128-gcm:p@example.com:8388?plugin="+url.QueryEscape("obfs-local;obfs=tls;obfs-host=front.example"))
		if n.Config["plugin"] != "obfs" || !reflect.DeepEqual(n.Config["plugin-opts"], map[string]any{"mode": "tls", "host": "front.example"}) {
			t.Fatal("obfs plugin mapping wrong")
		}
	})
	t.Run("v2ray", func(t *testing.T) {
		n := parseOne(t, "ss://aes-128-gcm:p@example.com:8388?plugin="+url.QueryEscape("v2ray-plugin;mode=websocket;tls;host=front.example;path=/a;mux=0"))
		p := n.Config["plugin-opts"].(map[string]any)
		if p["tls"] != true || p["path"] != "/a" || p["mux"] != false {
			t.Fatal("v2ray plugin mapping wrong")
		}
	})
	t.Run("ss2022", func(t *testing.T) {
		key := base64.StdEncoding.EncodeToString(make([]byte, 16))
		n := parseOne(t, "ss://"+base64.RawStdEncoding.EncodeToString([]byte("2022-blake3-aes-128-gcm:"+key))+"@example.com:443")
		if n.Config["password"] != key {
			t.Fatal("SS2022 key lost")
		}
	})
}
func TestQUICOptions(t *testing.T) {
	t.Run("hy1-obfs", func(t *testing.T) {
		n := parseOne(t, "hy://example.com:443?auth=a&up=10&down=50&obfs=xplus&obfsParam=secret&mport=443,1000-2000&fastopen=1")
		if n.Config["obfs"] != "secret" || n.Config["ports"] != "443,1000-2000" || n.Config["fast-open"] != true {
			t.Fatal("Hysteria options lost")
		}
	})
	t.Run("hy2-obfs", func(t *testing.T) {
		n := parseOne(t, "hy2://user:pass@example.com/?obfs=salamander&obfs-password=secret&ports=443,1000-2000&hop-interval=30&up=10&down=50")
		if n.Config["password"] != "user:pass" || n.Config["hop-interval"] != "30" {
			t.Fatal("Hysteria2 options lost")
		}
	})
	t.Run("tuic", func(t *testing.T) {
		n := parseOne(t, "tuic://"+testUUID+":p@example.com:443?version=5&disable_sni=false&reduce_rtt=1&heartbeat-interval=10000&request-timeout=8000&congestion_control=new_reno&udp_relay_mode=quic")
		if n.Config["disable-sni"] != false || n.Config["reduce-rtt"] != true || n.Config["heartbeat-interval"] != 10000 {
			t.Fatal("TUIC options lost")
		}
	})
}
