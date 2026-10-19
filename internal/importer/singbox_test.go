package importer

import (
	"reflect"
	"strings"
	"testing"
)

func singFixture(protocol string) map[string]any {
	obj := map[string]any{"type": protocol, "tag": "sing node", "server": "example.com", "server_port": 443}
	tls := map[string]any{"enabled": true, "server_name": "tls.example"}
	switch protocol {
	case "vless":
		obj["uuid"] = testUUID
		obj["tls"] = tls
	case "vmess":
		obj["uuid"] = testUUID
		obj["security"] = "auto"
		obj["alter_id"] = 0
		obj["tls"] = tls
	case "trojan":
		obj["password"] = "secret"
		obj["tls"] = tls
	case "shadowsocks":
		obj["method"] = "aes-128-gcm"
		obj["password"] = "secret"
	case "hysteria":
		obj["auth_str"] = "secret"
		obj["up_mbps"] = 10
		obj["down_mbps"] = 50
		obj["tls"] = tls
	case "hysteria2":
		obj["password"] = "secret"
		obj["tls"] = tls
	case "tuic":
		obj["uuid"] = testUUID
		obj["password"] = "secret"
		obj["tls"] = tls
	case "ssh":
		obj["user"] = "root"
		obj["password"] = "secret"
	case "socks":
		obj["version"] = "5"
		obj["username"] = "user"
		obj["password"] = "secret"
	case "http":
		obj["username"] = "user"
		obj["password"] = "secret"
		obj["tls"] = tls
	}
	return obj
}
func TestSingboxEveryProtocol(t *testing.T) {
	for _, protocol := range []string{"vless", "vmess", "trojan", "shadowsocks", "hysteria", "hysteria2", "tuic", "ssh", "socks", "http"} {
		t.Run(protocol, func(t *testing.T) {
			obj := singFixture(protocol)
			node := parseOne(t, singDoc(t, obj))
			if node.Protocol != canonicalProtocol(protocol) || node.Name != "sing node" {
				t.Fatal("sing-box metadata not converted")
			}
			expectedUDP := protocol != "ssh" && protocol != "http"
			if node.UDP != expectedUDP {
				t.Fatal("wrong UDP capabilities")
			}
			standalone := parseOne(t, jsonDoc(t, obj))
			if !reflect.DeepEqual(node, standalone) {
				t.Fatal("standalone sing-box conversion differs")
			}
			for _, key := range []string{"server_port", "tag", "transport", "user", "method", "packet_encoding"} {
				if _, exists := node.Config[key]; exists {
					t.Fatalf("sing-box key %s leaked into native map", key)
				}
			}
			parseOne(t, nativeDoc(t, node.Config))
		})
	}
}
func TestSingboxSecurityTransportOptions(t *testing.T) {
	t.Run("reality-vision", func(t *testing.T) {
		obj := singFixture("vless")
		obj["flow"] = "xtls-rprx-vision"
		obj["packet_encoding"] = "xudp"
		tls := obj["tls"].(map[string]any)
		tls["utls"] = map[string]any{"enabled": true, "fingerprint": "firefox"}
		tls["reality"] = map[string]any{"enabled": true, "public_key": testPublicKey, "short_id": "ab12"}
		n := parseOne(t, singDoc(t, obj))
		if n.Config["tls"] != true || n.Config["client-fingerprint"] != "firefox" || n.Config["servername"] != "tls.example" {
			t.Fatal("Reality TLS mapping lost")
		}
		if !reflect.DeepEqual(n.Config["reality-opts"], map[string]any{"public-key": testPublicKey, "short-id": "ab12"}) {
			t.Fatal("Reality keys lost")
		}
	})
	t.Run("ws", func(t *testing.T) {
		obj := singFixture("vmess")
		obj["transport"] = map[string]any{"type": "ws", "path": "/ws", "headers": map[string]any{"Host": "front.example"}, "max_early_data": 2048, "early_data_header_name": "Sec-WebSocket-Protocol"}
		n := parseOne(t, singDoc(t, obj))
		if n.Config["network"] != "ws" {
			t.Fatal("WebSocket transport lost")
		}
		options := n.Config["ws-opts"].(map[string]any)
		if options["max-early-data"] != 2048 {
			t.Fatal("WebSocket early data lost")
		}
	})
	t.Run("grpc", func(t *testing.T) {
		obj := singFixture("trojan")
		obj["transport"] = map[string]any{"type": "grpc", "service_name": "service"}
		n := parseOne(t, singDoc(t, obj))
		if !reflect.DeepEqual(n.Config["grpc-opts"], map[string]any{"grpc-service-name": "service"}) {
			t.Fatal("gRPC service lost")
		}
	})
	t.Run("hy2-hopping", func(t *testing.T) {
		obj := singFixture("hysteria2")
		delete(obj, "server_port")
		obj["server_ports"] = []string{"443", "1000:2000"}
		obj["hop_interval"] = "30s"
		obj["obfs"] = map[string]any{"type": "salamander", "password": "obfs-secret"}
		n := parseOne(t, singDoc(t, obj))
		if n.Config["ports"] != "443,1000-2000" || n.Port != 443 || n.Config["hop-interval"] != "30" {
			t.Fatal("hopping conversion failed")
		}
	})
	t.Run("tuic-options", func(t *testing.T) {
		obj := singFixture("tuic")
		obj["congestion_control"] = "bbr"
		obj["udp_relay_mode"] = "quic"
		obj["zero_rtt_handshake"] = true
		obj["heartbeat"] = "10s"
		obj["tls"].(map[string]any)["disable_sni"] = false
		n := parseOne(t, singDoc(t, obj))
		if n.Config["heartbeat-interval"] != 10000 || n.Config["reduce-rtt"] != true || n.Config["disable-sni"] != false {
			t.Fatal("TUIC conversion failed")
		}
	})
	t.Run("ss-plugin", func(t *testing.T) {
		obj := singFixture("shadowsocks")
		obj["plugin"] = "obfs-local"
		obj["plugin_opts"] = "obfs=http;obfs-host=front.example"
		obj["udp_over_tcp"] = map[string]any{"enabled": true, "version": 2}
		n := parseOne(t, singDoc(t, obj))
		if n.Config["plugin"] != "obfs" || n.Config["udp-over-tcp-version"] != 2 {
			t.Fatal("Shadowsocks plugin/UOT conversion failed")
		}
	})
	t.Run("ssh-inline", func(t *testing.T) {
		obj := singFixture("ssh")
		key := testSSHPrivatePEM(t)
		obj["private_key"] = strings.Split(strings.TrimSpace(key), "\n")
		obj["host_key"] = []string{"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJjlL7Hx1LhvVClSvFVoRKJBKSjPxNMIZmsIXKuGyMGv"}
		n := parseOne(t, singDoc(t, obj))
		if n.Config["private-key"] != strings.TrimSpace(key) || n.Config["username"] != "root" {
			t.Fatal("SSH mapping changed key or username")
		}
	})
	t.Run("http-headers", func(t *testing.T) {
		obj := singFixture("http")
		obj["headers"] = map[string]any{"X-Example": []string{"single"}}
		n := parseOne(t, singDoc(t, obj))
		if !reflect.DeepEqual(n.Config["headers"], map[string]any{"X-Example": "single"}) {
			t.Fatal("HTTP header conversion failed")
		}
	})
	t.Run("dial-options", func(t *testing.T) {
		obj := singFixture("vless")
		obj["domain_strategy"] = "prefer_ipv6"
		obj["tcp_fast_open"] = true
		obj["network"] = "tcp"
		n := parseOne(t, singDoc(t, obj))
		if n.UDP || n.Config["tfo"] != true || n.Config["ip-version"] != "ipv6-prefer" {
			t.Fatal("safe dial options conversion failed")
		}
	})
	t.Run("plaintext-vless", func(t *testing.T) {
		obj := singFixture("vless")
		delete(obj, "tls")
		n := parseOne(t, singDoc(t, obj))
		if n.Config["tls"] != false {
			t.Fatal("plaintext VLESS incorrectly upgraded")
		}
	})
}
func TestSingboxRejectsUnsafeAndUnrepresentable(t *testing.T) {
	tests := []struct {
		name, protocol string
		modify         func(map[string]any)
	}{
		{"detour", "vless", func(m map[string]any) { m["detour"] = "other" }},
		{"private-key-path", "ssh", func(m map[string]any) { m["private_key_path"] = "/root/.ssh/id_rsa" }},
		{"private-key-as-path", "ssh", func(m map[string]any) { m["private_key"] = "/root/.ssh/id_rsa" }},
		{"certificate-file", "trojan", func(m map[string]any) { m["tls"].(map[string]any)["certificate_path"] = "/etc/private.pem" }},
		{"inline-ca-unmapped", "trojan", func(m map[string]any) { m["tls"].(map[string]any)["certificate"] = []string{"PEM"} }},
		{"tls-min-version", "trojan", func(m map[string]any) { m["tls"].(map[string]any)["min_version"] = "1.3" }},
		{"ech", "vless", func(m map[string]any) { m["tls"].(map[string]any)["ech"] = map[string]any{"enabled": true} }},
		{"disabled-required-tls", "trojan", func(m map[string]any) { m["tls"].(map[string]any)["enabled"] = false }},
		{"missing-required-tls", "hysteria2", func(m map[string]any) { delete(m, "tls") }},
		{"insecure-disabled-tls", "vless", func(m map[string]any) { m["tls"] = map[string]any{"enabled": false, "insecure": true} }},
		{"multiplex", "vless", func(m map[string]any) { m["multiplex"] = map[string]any{"enabled": true} }},
		{"quic-transport", "vless", func(m map[string]any) { m["transport"] = map[string]any{"type": "quic"} }},
		{"httpupgrade", "vless", func(m map[string]any) { m["transport"] = map[string]any{"type": "httpupgrade"} }},
		{"grpc-options", "vmess", func(m map[string]any) {
			m["transport"] = map[string]any{"type": "grpc", "service_name": "x", "idle_timeout": "2m"}
		}},
		{"socks4", "socks", func(m map[string]any) { m["version"] = "4" }},
		{"socks-tls", "socks", func(m map[string]any) { m["tls"] = map[string]any{"enabled": true} }},
		{"udp-only", "vless", func(m map[string]any) { m["network"] = "udp" }},
		{"bind-interface", "vless", func(m map[string]any) { m["bind_interface"] = "eth0" }},
		{"tuic-uos", "tuic", func(m map[string]any) { m["udp_over_stream"] = true }},
		{"http-path", "http", func(m map[string]any) { m["path"] = "/specific" }},
		{"http-multiple-headers", "http", func(m map[string]any) { m["headers"] = map[string]any{"X-Test": []string{"a", "b"}} }},
		{"duration-truncation", "hysteria2", func(m map[string]any) { m["server_ports"] = []string{"443"}; m["hop_interval"] = "10.5s" }},
		{"tls-disable-sni", "hysteria2", func(m map[string]any) { m["tls"].(map[string]any)["disable_sni"] = true }},
		{"plugin-path", "shadowsocks", func(m map[string]any) { m["plugin"] = "/usr/bin/plugin" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := singFixture(tt.protocol)
			tt.modify(obj)
			requireInvalid(t, singDoc(t, obj))
		})
	}
	for _, protocol := range []string{"direct", "block", "dns", "selector", "urltest", "wireguard", "shadowtls", "anytls"} {
		requireInvalid(t, singDoc(t, map[string]any{"type": protocol, "tag": "rejected"}))
	}
}
