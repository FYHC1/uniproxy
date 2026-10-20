package importer

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/uniproxy/uniproxy/internal/model"
)

func TestRejectMalformedAndUnsupportedURIs(t *testing.T) {
	tests := []string{
		"unknown://secret@example.com:443", "ssr://not-supported", "file:///etc/passwd", "vless://missing@example.com:443",
		"vless://" + testUUID + "@example.com", "vless://" + testUUID + "@example.com:0", "vless://" + testUUID + "@example.com:65536", "vless://" + testUUID + "@example.com:notaport",
		"vless://" + testUUID + "@2001:db8::1:443", "vless://" + testUUID + "@[fe80::1%25eth0]:443", "vless://" + testUUID + "@example.com:443/unmapped",
		"vless://" + testUUID + "@example.com:443?type=kcp", "vless://" + testUUID + "@example.com:443?type=xhttp", "vless://" + testUUID + "@example.com:443?encryption=mlkem768x25519plus.native.0rtt",
		"vless://" + testUUID + "@example.com:443?security=xtls", "vless://" + testUUID + "@example.com:443?security=none&sni=tls.example", "vless://" + testUUID + "@example.com:443?flow=xtls-rprx-vision",
		"vless://" + testUUID + "@example.com:443?security=tls&flow=xtls-rprx-vision&type=ws", "vless://" + testUUID + "@example.com:443?security=tls&flow=xtls-rprx-direct",
		"vless://" + testUUID + "@example.com:443?security=reality&pbk=invalid", "vless://" + testUUID + "@example.com:443?pbk=" + testPublicKey,
		"vless://" + testUUID + "@example.com:443?security=reality&pbk=" + testPublicKey + "&sid=1", "vless://" + testUUID + "@example.com:443?security=reality&pbk=" + testPublicKey + "&allowInsecure=1",
		"vless://" + testUUID + "@example.com:443?security=tls&fp=invalid", "vless://" + testUUID + "@example.com:443?security=tls&tls=false",
		"vless://" + testUUID + "@example.com:443?security=tls&sni=a&sni=b", "vless://" + testUUID + "@example.com:443?security=tls&sni=a&peer=b",
		"vless://" + testUUID + "@example.com:443?unknown=secret", "vless://" + testUUID + "@example.com:443?security=tls&allowInsecure=maybe",
		"vless://" + testUUID + "@example.com:443?path=%2Fignored", "vless://" + testUUID + "@example.com:443?type=grpc&mode=multi",
		"vless://" + testUUID + "@example.com:443?type=grpc&authority=other", "vless://" + testUUID + "@example.com:443?type=ws&path=relative",
		"trojan://%ZZ@example.com:443", "trojan://secret@example.com:443?x=%ZZ", "trojan://secret@example.com:443?security=none", "trojan://@example.com:443",
		"trojan://secret@example.com:443?type=h2", "trojan://secret@example.com:443?allowInsecure=1;evil=1", "trojan://secret@example.com:443#bad%0Aname",
		"ss://invalid", "ss://aes-128-gcm@example.com:8388", "ss://unknown:secret@example.com:8388", "ss://aes-128-gcm:secret@example.com:8388?plugin=evil-command",
		"ss://aes-128-gcm:secret@example.com:8388?plugin=v2ray-plugin%3Bmode%3Dquic", "ss://aes-128-gcm:secret@example.com:8388?plugin=obfs-local%3Bobfs%3Dtls%3Bobfs%3Dhttp",
		"ss://2022-blake3-aes-128-gcm:short@example.com:8388", "ss://aes-128-gcm:secret@example.com:8388?udp-over-tcp-version=2",
		"hysteria://example.com:443?auth=a", "hysteria://example.com:443?up=-1&down=1", "hysteria://example.com:443?up=10&down=10&protocol=faketcp",
		"hy2://@example.com", "hy2://secret@example.com?obfs=unknown&obfs-password=p", "hy2://secret@example.com?obfs=salamander", "hy2://secret@example.com?obfs-password=p",
		"hy2://secret@example.com?ports=65536", "hy2://secret@example.com?ports=1000-1", "hy2://secret@example.com?hop-interval=10", "hy2://secret@example.com?fingerprint=invalid",
		"tuic://" + testUUID + ":secret@example.com:443?version=4", "tuic://token@example.com:443?version=5", "tuic://invalid:secret@example.com:443", "tuic://token@example.com:443?udp_relay_mode=unknown",
		"ssh://user@example.com", "ssh://user:secret@example.com?private-key=%2Froot%2F.ssh%2Fid_rsa", "socks5://user:secret@example.com:1080?tls=1&sni=other.example",
		"http://example.com/subscription?token=secret", "http://example.com:8080?udp=true", "https://example.com?security=none", "http://example.com:8080?detour=other",
		"vmess://!!!!", "vmess://" + base64.StdEncoding.EncodeToString([]byte("not json")), "trojan://raw space@example.com:443",
	}
	for i, input := range tests {
		t.Run(fmt.Sprint(i), func(t *testing.T) { requireInvalid(t, input) })
	}
}
func TestRejectUnsafeNativeOptions(t *testing.T) {
	for _, key := range []string{"dialer-proxy", "detour", "script", "exec", "command", "interface-name", "routing-mark", "certificate", "certificate-path", "ca", "ca-str", "private-key", "key", "key-path", "ca-path", "plugin-path", "realm-opts", "smux", "unknown"} {
		t.Run(key, func(t *testing.T) {
			obj := nativeFixture("trojan")
			obj[key] = "secret-value"
			requireInvalid(t, nativeDoc(t, obj))
		})
	}
	for _, path := range []string{"/etc/ssh/private_key", "C:\\Users\\admin\\.ssh\\id_rsa", "\\\\server\\share\\key", "~/.ssh/id_rsa", "./key.pem", "relative.pem", "file:///etc/key", "https://example.com/key"} {
		obj := nativeFixture("ssh")
		obj["private-key"] = path
		requireInvalid(t, nativeDoc(t, obj))
	}
	tests := []struct {
		name, protocol string
		mutate         func(map[string]any)
	}{
		{"TLS-ignored", "vless", func(m map[string]any) { m["tls"] = false; m["skip-cert-verify"] = true }},
		{"network-ignored", "vless", func(m map[string]any) { m["network"] = "unknown" }},
		{"ws-without-ws-network", "vless", func(m map[string]any) { m["ws-opts"] = map[string]any{"path": "/secret"} }},
		{"nested-path", "vless", func(m map[string]any) {
			m["network"] = "ws"
			m["ws-opts"] = map[string]any{"certificate-path": "/etc/key"}
		}},
		{"ws-crlf-header", "vless", func(m map[string]any) {
			m["network"] = "ws"
			m["ws-opts"] = map[string]any{"headers": map[string]any{"Host": "ok\r\nInjected:secret"}}
		}},
		{"case-duplicate-header", "vless", func(m map[string]any) {
			m["network"] = "ws"
			m["ws-opts"] = map[string]any{"headers": map[string]any{"Host": "a", "host": "b"}}
		}},
		{"ws-upgrade", "vless", func(m map[string]any) { m["network"] = "ws"; m["ws-opts"] = map[string]any{"v2ray-http-upgrade": true} }},
		{"reality-short-id", "vless", func(m map[string]any) {
			m["reality-opts"] = map[string]any{"public-key": testPublicKey, "short-id": "xyz"}
		}},
		{"reality-private-key", "vless", func(m map[string]any) {
			m["reality-opts"] = map[string]any{"public-key": testPublicKey, "private-key": "secret"}
		}},
		{"trojan-no-tls", "trojan", func(m map[string]any) { m["tls"] = false }},
		{"hy2-disable-udp", "hysteria2", func(m map[string]any) { m["udp"] = false }},
		{"tuic-ambiguous-credentials", "tuic", func(m map[string]any) { m["token"] = "token" }},
		{"hy1-ambiguous-auth", "hysteria", func(m map[string]any) { m["auth"] = "c2VjcmV0" }},
		{"ssh-passphrase-without-key", "ssh", func(m map[string]any) { m["private-key-passphrase"] = "secret" }},
		{"ssh-udp", "ssh", func(m map[string]any) { m["udp"] = true }},
		{"http-password-no-user", "http", func(m map[string]any) { delete(m, "username") }},
		{"numeric-password", "trojan", func(m map[string]any) { m["password"] = 1234 }},
		{"bad-port", "http", func(m map[string]any) { m["port"] = 1.25 }},
		{"nil-port", "http", func(m map[string]any) { m["port"] = nil }},
		{"bad-host", "http", func(m map[string]any) { m["server"] = "https://example.com" }},
		{"name-control", "http", func(m map[string]any) { m["name"] = "bad\nname" }},
		{"nil-reality", "vless", func(m map[string]any) { m["reality-opts"] = nil }},
		{"bad-alpn", "trojan", func(m map[string]any) { m["alpn"] = []any{"h2", 2} }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := nativeFixture(tt.protocol)
			tt.mutate(obj)
			requireInvalid(t, nativeDoc(t, obj))
		})
	}
}
func TestRejectMalformedDocuments(t *testing.T) {
	tests := []string{
		"proxies: [", "proxies: string", "proxies:\n  - not-an-object", "unknown: value", "type: direct", "---\nproxies: []\n---\nproxies: []", "proxies: []\nproxies: []",
		"proxies:\n - type: http\n   type: socks5\n   server: example.com\n   port: 80", "proxies: [{type: http, server: example.com, port: 80, password: !!binary c2VjcmV0}]",
		"a: &a [*a]\nproxies: []", "proxies:\n  - &a {type: http, server: example.com, port: 80}\n  - *a", "base: &base {type: http}\nproxies: [{<<: *base, server: example.com, port: 80}]",
		"proxies: []\nscript: secret", "proxies: []\nproxy-providers: {}", "proxies: []\nexternal-ui: /etc", "proxies: []\noutbounds: []",
		`{"proxies":[{"type":"http","server":"example.com","port":80,"port":81}]}`,
		`{"proxies":[],"proxies":[]}`, `{"proxies":[]} trailing`, `{"proxies":[]} {"proxies":[]}`, `{"proxies":[],}`,
		"proxies: [{type: http, server: example.com, port: .nan}]", "proxies: [{type: http, server: example.com, port: 1e1000}]", "proxies: [{1: invalid}]", "proxies: [{type: http, server: example.com, port: 80, 1: invalid}]",
		string([]byte{0xff, 0xfe}), strings.Repeat("x", MaxContentBytes+1),
	}
	for i, input := range tests {
		t.Run(fmt.Sprint(i), func(t *testing.T) { requireInvalid(t, input) })
	}
}
func TestLimitsAndLargeLines(t *testing.T) {
	t.Run("nodes-at-limit", func(t *testing.T) {
		result := Parse(strings.Repeat("http://example.com:8080\n", MaxNodes))
		if len(result.Nodes) != MaxNodes || len(result.Errors) != 0 {
			t.Fatal("node limit rejected legal count")
		}
	})
	t.Run("nodes-over-limit", func(t *testing.T) {
		result := Parse(strings.Repeat("http://example.com:8080\n", MaxNodes+1))
		if len(result.Nodes) > MaxNodes || len(result.Errors) == 0 {
			t.Fatal("URI node limit not enforced")
		}
	})
	t.Run("document-node-limit", func(t *testing.T) {
		items := make([]any, MaxNodes+1)
		for i := range items {
			items[i] = nativeFixture("http")
		}
		requireInvalid(t, jsonDoc(t, map[string]any{"proxies": items}))
	})
	t.Run("depth", func(t *testing.T) {
		content := `{"unknown":` + strings.Repeat("[", maxDepth+1) + "0" + strings.Repeat("]", maxDepth+1) + `,"proxies":[]}`
		requireInvalid(t, content)
	})
	t.Run("value-count", func(t *testing.T) {
		content := "unknown: [" + strings.Repeat("0,", maxDocumentValues) + "0]\nproxies: []"
		requireInvalid(t, content)
	})
	t.Run("bounded-issues", func(t *testing.T) {
		result := Parse(strings.Repeat("unsupported://secret\n", 1000))
		if len(result.Errors) != maxIssues || len(result.Nodes) != 0 || result.TotalErrors != 1000 || !result.ErrorsTruncated {
			t.Fatal("issue details must be bounded without hiding the true total")
		}
	})
	t.Run("large-base64-line", func(t *testing.T) {
		password := strings.Repeat("p", 60000)
		content := "trojan://" + password + "@example.com:443"
		result := Parse(base64.StdEncoding.EncodeToString([]byte(content)))
		if len(result.Errors) != 0 || len(result.Nodes) != 1 {
			t.Fatal("scanner silently truncated a >64KiB base64 line")
		}
	})
}
func TestDiagnosticsNeverEchoSecrets(t *testing.T) {
	secret := "NEVER_ECHO_THIS_CREDENTIAL_9ec1f478"
	cases := []string{
		"unknown://" + secret + "@example.com:443", "vless://" + secret + "@example.com:443", "trojan://" + secret + "@example.com:notaport", "trojan://" + secret + "@example.com:443?" + secret + "=value", "proxies: [" + secret,
		`{"proxies":[{"type":"trojan","server":"example.com","port":443,"password":"` + secret + `","unknown":"` + secret + `"}]}`,
	}
	for _, content := range cases {
		result := Parse(content)
		if len(result.Errors) == 0 {
			t.Fatal("fixture must fail")
		}
		for _, issue := range result.Errors {
			if strings.Contains(issue.Message, secret) || strings.Contains(issue.Message, content) {
				t.Fatal("credential leaked into diagnostic")
			}
		}
	}
}
func TestFingerprintIdentity(t *testing.T) {
	original := parseOne(t, "trojan://secret@example.com:443#First")
	renamed := parseOne(t, "trojan://secret@example.com:443#Second")
	if Fingerprint(original) != Fingerprint(renamed) {
		t.Fatal("label affected identity")
	}
	renamed.Config["name"] = "ignored display name"
	if Fingerprint(original) != Fingerprint(renamed) {
		t.Fatal("config name affected identity")
	}
	snapshot, _ := json.Marshal(original)
	for _, mutate := range []func(*model.ParsedNode){
		func(n *model.ParsedNode) { n.Config["password"] = "other" }, func(n *model.ParsedNode) { n.Config["skip-cert-verify"] = true }, func(n *model.ParsedNode) { n.Config["sni"] = "other.example" }, func(n *model.ParsedNode) { n.Port = 8443 }, func(n *model.ParsedNode) { n.Server = "other.example" }, func(n *model.ParsedNode) { n.UDP = false }, func(n *model.ParsedNode) { n.Config["network"] = "ws" },
	} {
		var changed model.ParsedNode
		if err := json.Unmarshal(snapshot, &changed); err != nil {
			t.Fatal(err)
		}
		mutate(&changed)
		if Fingerprint(changed) == Fingerprint(original) {
			t.Fatal("connection or credential change failed to affect identity")
		}
	}
	var persisted model.ParsedNode
	json.Unmarshal(snapshot, &persisted)
	if Fingerprint(persisted) != Fingerprint(original) {
		t.Fatal("JSON integer representation affected identity")
	}
	after, _ := json.Marshal(original)
	if string(after) != string(snapshot) {
		t.Fatal("fingerprint mutated caller data")
	}
	bad := original
	bad.Config = map[string]any{"invalid": math.NaN()}
	if Fingerprint(bad) != "" {
		t.Fatal("invalid maps must not receive a valid identity")
	}
	cycle := map[string]any{}
	cycle["self"] = cycle
	bad.Config = cycle
	if Fingerprint(bad) != "" {
		t.Fatal("cyclic maps must not panic or receive identity")
	}
}
func TestParseConcurrentAndDeterministic(t *testing.T) {
	input := "vless://" + testUUID + "@example.com:443?security=tls&type=ws&host=front.example&path=%2Fws"
	expected := Parse(input)
	var group sync.WaitGroup
	for i := 0; i < 32; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			for j := 0; j < 10; j++ {
				if actual := Parse(input); !reflect.DeepEqual(expected, actual) {
					t.Error("concurrent parsing is not deterministic")
				}
			}
		}()
	}
	group.Wait()
}
func FuzzParseNeverPanics(f *testing.F) {
	seeds := []string{"", "proxies: []", `{"outbounds":[]}`, "http://example.com:80", "vless://" + testUUID + "@example.com:443", "ss://YWVzLTEyOC1nY206cGFzc3dvcmQ@example.com:443", "proxies: [*bad]", vmessLink(vmessBase()), "trojan://p%00@example.com:443"}
	seeds = append(seeds,
		"[Proxy]\nDIRECT = direct\nfixture = trojan, example.invalid, 443, password=synthetic-only,sni=front.example.invalid,skip-cert-verify=false\n[Rule]\nFINAL,DIRECT",
		"[server_local]\nvmess=example.invalid:443,password="+testUUID+",method=auto,obfs=wss,obfs-host=front.example.invalid,tag=fixture",
		"STATUS=synthetic status\ntrojan://p@example.invalid:443?sni=front.example.invalid&peer=front.example.invalid",
		"proxies: [{name: exit, type: http, server: exit.example, port: 80, dialer-proxy: relay}, {name: relay, type: http, server: relay.example, port: 80}]",
		"proxies: [{name: a, type: http, server: a.example, port: 80, dialer-proxy: b}, {name: b, type: http, server: b.example, port: 80, dialer-proxy: a}]",
	)
	for _, seed := range seeds {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, content string) {
		result := Parse(content)
		if result.Nodes == nil || result.Errors == nil || len(result.Nodes) > MaxNodes || len(result.Errors) > maxIssues || len(result.Errors) != min(result.TotalErrors, maxIssues) || result.ErrorsTruncated != (result.TotalErrors > maxIssues) {
			t.Fatal("parser invariant violated")
		}
		for _, node := range result.Nodes {
			if node.Port < 1 || node.Port > 65535 || node.Config["name"] != nil || node.Config["dialer-proxy"] != nil || len(Fingerprint(node)) != 64 || (node.DialerProxy == "") != (node.DialerFingerprint == "") {
				t.Fatal("normalized node invariant violated")
			}
			if _, err := json.Marshal(node.Config); err != nil {
				t.Fatal("config is not JSON compatible")
			}
		}
	})
}
