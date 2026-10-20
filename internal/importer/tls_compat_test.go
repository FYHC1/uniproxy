package importer

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/uniproxy/uniproxy/internal/model"
	"gopkg.in/yaml.v3"
)

// These are exporter defaults, not requests to enable TLS. Their normalization
// must keep the old no-TLS identity, credentials, and plaintext transport.
func TestTLSDisabledInertDefaultsAcrossNativeFormats(t *testing.T) {
	for _, protocol := range []string{"vless", "vmess", "http", "socks5"} {
		for _, explicit := range []bool{false, true} {
			for _, flag := range []any{false, 0, "false", "0"} {
				t.Run(fmt.Sprintf("%s/explicit-%t/%T-%v", protocol, explicit, flag, flag), func(t *testing.T) {
					obj := nativeFixture(protocol)
					delete(obj, "tls")
					if explicit {
						obj["tls"] = false
					}
					if protocol == "vmess" || protocol == "vless" {
						obj["network"] = "ws"
						obj["ws-opts"] = map[string]any{"path": "/plain?q=1", "headers": map[string]any{"Host": "front.example.invalid"}}
					}
					want := parseOne(t, nativeDoc(t, obj))
					key := "sni"
					if protocol == "vmess" || protocol == "vless" {
						key = "servername"
					}
					obj["skip-cert-verify"] = flag
					obj[key] = ""
					doc := map[string]any{"proxies": []any{obj}}
					encoded, err := yaml.Marshal(doc)
					if err != nil {
						t.Fatal(err)
					}
					formats := []string{jsonDoc(t, doc), string(encoded), jsonDoc(t, obj), jsonDoc(t, []any{obj}), base64.StdEncoding.EncodeToString(encoded)}
					for _, input := range formats {
						got := parseOne(t, input)
						assertSameTLSIdentity(t, want, got)
						if got.Config["tls"] != false {
							t.Fatal("inert defaults enabled TLS")
						}
						for _, key := range []string{"sni", "servername", "skip-cert-verify"} {
							if _, ok := got.Config[key]; ok {
								t.Fatal("inert TLS field survived canonical output")
							}
						}
					}
				})
			}
		}
	}
}

func assertSameTLSIdentity(t *testing.T, want, got model.ParsedNode) {
	t.Helper()
	if !reflect.DeepEqual(want.Config, got.Config) || want.UDP != got.UDP || Fingerprint(want) != Fingerprint(got) {
		t.Fatal("TLS default normalization changed identity or transport")
	}
	round := parseOne(t, nativeDoc(t, got.Config))
	if Fingerprint(round) != Fingerprint(got) {
		t.Fatal("normalized TLS config is not idempotent")
	}
}

func TestTLSDisabledDefaultsAcrossURIAndVMessJSON(t *testing.T) {
	for _, base := range []string{"vless://" + testUUID + "@example.invalid:443?type=ws&host=front.example.invalid&path=%2Fplain", "vmess://" + testUUID + "@example.invalid:443?type=ws&host=front.example.invalid&path=%2Fplain", "http://u:p@example.invalid:8080?tls=false", "socks5://u:p@example.invalid:1080?tls=false"} {
		want := parseOne(t, base)
		for _, suffix := range []string{"&insecure=0&sni=", "&allowInsecure=false&serverName="} {
			assertSameTLSIdentity(t, want, parseOne(t, base+suffix))
		}
	}
	for _, tls := range []string{"", "none"} {
		obj := map[string]any{"v": "2", "ps": "plain fixture", "add": "example.invalid", "port": "443", "id": testUUID, "aid": "0", "scy": "auto", "net": "ws", "type": "none", "host": "front.example.invalid", "path": "/plain", "tls": tls, "sni": "", "fp": "", "alpn": ""}
		want := parseOne(t, vmessLink(obj))
		obj["allowInsecure"] = false
		assertSameTLSIdentity(t, want, parseOne(t, vmessLink(obj)))
	}
}

func TestTLSDisabledDefaultsNeverHideActiveOrMalformedOptions(t *testing.T) {
	for _, protocol := range []string{"vless", "vmess", "http", "socks5"} {
		serverKey := "sni"
		if protocol == "vless" || protocol == "vmess" {
			serverKey = "servername"
		}
		cases := []struct {
			key   string
			value any
		}{
			{"skip-cert-verify", true}, {"skip-cert-verify", "maybe"}, {"skip-cert-verify", nil}, {"skip-cert-verify", 2},
			{serverKey, "front.example.invalid"}, {serverKey, " "}, {serverKey, nil}, {serverKey, false},
			{"client-fingerprint", "chrome"}, {"client-fingerprint", ""}, {"alpn", []any{"h2"}}, {"alpn", []any{}},
			{"fingerprint", strings.Repeat("ab", 32)}, {"fingerprint", ""}, {"name-cert-verify", "front.example.invalid"},
			{"reality-opts", map[string]any{"public-key": testPublicKey}}, {"reality-opts", map[string]any{}}, {"reality-opts", nil},
			{"tls", nil}, {"tls", "not-a-boolean"}, {"unknown", false}, {"certificate-path", ""}, {"detour", ""},
		}
		for i, tc := range cases {
			t.Run(fmt.Sprintf("%s/%d", protocol, i), func(t *testing.T) {
				obj := nativeFixture(protocol)
				obj["tls"] = false
				obj[serverKey] = ""
				obj["skip-cert-verify"] = false
				obj[tc.key] = tc.value
				requireInvalid(t, nativeDoc(t, obj))
			})
		}
	}
	for _, protocol := range []string{"trojan", "hysteria", "hysteria2", "tuic"} {
		obj := nativeFixture(protocol)
		obj["tls"] = false
		obj["skip-cert-verify"] = false
		requireInvalid(t, nativeDoc(t, obj))
	}
	for _, scheme := range []string{"https", "socks5s"} {
		obj := nativeFixture(canonicalProtocol(scheme))
		obj["type"] = scheme
		obj["tls"] = false
		obj["skip-cert-verify"] = false
		requireInvalid(t, nativeDoc(t, obj))
	}
}

func TestTLSDefaultsPreserveEnabledSecurityAndRelayGraph(t *testing.T) {
	obj := nativeFixture("vmess")
	obj["tls"] = true
	obj["servername"] = "front.example.invalid"
	obj["skip-cert-verify"] = false
	n := parseOne(t, nativeDoc(t, obj))
	if n.Config["tls"] != true || n.Config["servername"] != "front.example.invalid" || n.Config["skip-cert-verify"] != false {
		t.Fatal("active TLS verification was stripped")
	}
	upstream := nativeFixture("vmess")
	upstream["name"] = "upstream"
	upstream["tls"] = false
	leaf := nativeFixture("trojan")
	leaf["name"] = "leaf"
	leaf["dialer-proxy"] = "upstream"
	before := Parse(jsonDoc(t, map[string]any{"proxies": []any{leaf, upstream}}))
	if before.TotalErrors != 0 || len(before.Nodes) != 2 {
		t.Fatal("bad synthetic relay baseline")
	}
	upstream["skip-cert-verify"] = false
	upstream["servername"] = ""
	after := Parse(jsonDoc(t, map[string]any{"proxies": []any{leaf, upstream}}))
	if after.TotalErrors != 0 || len(after.Nodes) != 2 {
		t.Fatal("inert defaults broke relay resolution")
	}
	for i := range before.Nodes {
		if Fingerprint(before.Nodes[i]) != Fingerprint(after.Nodes[i]) {
			t.Fatal("inert defaults changed recursive relay identity")
		}
	}
	upstream["skip-cert-verify"] = true
	failed := Parse(jsonDoc(t, map[string]any{"proxies": []any{leaf, upstream}}))
	if failed.TotalErrors != 2 || len(failed.Nodes) != 0 {
		t.Fatal("invalid TLS upstream fell back to direct")
	}
}

func TestPluginInertVerificationDefaultDoesNotEnableTLS(t *testing.T) {
	obj := nativeFixture("ss")
	obj["plugin"] = "v2ray-plugin"
	options := map[string]any{"mode": "websocket", "tls": false, "path": "/plain", "host": "front.example.invalid"}
	obj["plugin-opts"] = options
	want := parseOne(t, nativeDoc(t, obj))
	options["skip-cert-verify"] = false
	assertSameTLSIdentity(t, want, parseOne(t, nativeDoc(t, obj)))
	options["skip-cert-verify"] = true
	requireInvalid(t, nativeDoc(t, obj))
	options["skip-cert-verify"] = "invalid"
	requireInvalid(t, nativeDoc(t, obj))
	uri := "ss://aes-128-gcm:p@example.invalid:8388?plugin="
	parseOne(t, uri+url.QueryEscape("v2ray-plugin;tls=0;skip-cert-verify=0;host=front.example.invalid"))
}

func TestSingboxInertTLSDefaults(t *testing.T) {
	for _, protocol := range []string{"vless", "vmess", "http"} {
		obj := singFixture(protocol)
		obj["tls"] = map[string]any{"enabled": false}
		want := parseOne(t, singDoc(t, obj))
		obj["tls"] = map[string]any{"enabled": false, "server_name": "", "insecure": false, "disable_sni": false, "utls": map[string]any{"enabled": false, "fingerprint": ""}, "reality": map[string]any{"enabled": false}}
		assertSameTLSIdentity(t, want, parseOne(t, singDoc(t, obj)))
		for _, change := range []map[string]any{{"insecure": true}, {"server_name": "front.example.invalid"}, {"alpn": []any{"h2"}}, {"utls": map[string]any{"enabled": true, "fingerprint": "chrome"}}, {"reality": map[string]any{"enabled": true, "public_key": testPublicKey}}, {"certificate_path": ""}, {"unknown": false}} {
			obj["tls"] = map[string]any{"enabled": false}
			for k, v := range change {
				obj["tls"].(map[string]any)[k] = v
			}
			requireInvalid(t, singDoc(t, obj))
		}
	}
}
