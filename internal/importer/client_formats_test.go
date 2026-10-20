package importer

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	"github.com/uniproxy/uniproxy/internal/model"
)

const clientTestUUID = "12345678-1234-4234-8234-123456789abc"

func parseClientForTest(t *testing.T, text string) model.ParseResult {
	t.Helper()
	result := model.ParseResult{Nodes: []model.ParsedNode{}, Errors: []model.ImportIssue{}}
	if !tryParseClientText(text, &result) {
		t.Fatal("expected client format recognition")
	}
	if len(result.Errors) != min(result.TotalErrors, maxIssues) || result.ErrorsTruncated != (result.TotalErrors > maxIssues) {
		t.Fatal("client error totals disagree with detail cap")
	}
	for _, problem := range result.Errors {
		if problem.Line < 1 || problem.Message == "" {
			t.Fatal("invalid client diagnostic")
		}
	}
	for _, node := range result.Nodes {
		if len(Fingerprint(node)) != 64 || node.DialerProxy != "" || node.DialerFingerprint != "" {
			t.Fatal("client node has invalid identity or imported a delegation")
		}
		for _, key := range []string{"name", "dialer-proxy", "script", "provider", "no-error-alert"} {
			if _, exists := node.Config[key]; exists {
				t.Fatal("client config contains an untrusted or presentation-only field")
			}
		}
		if node.Config["type"] != node.Protocol || node.Config["server"] != node.Server || node.Config["port"] != node.Port {
			t.Fatal("client metadata differs from its strict native map")
		}
	}
	return result
}

func oneClientForTest(t *testing.T, text string) model.ParsedNode {
	t.Helper()
	result := parseClientForTest(t, text)
	if len(result.Nodes) != 1 || result.TotalErrors != 0 {
		t.Fatalf("expected one static client node; nodes=%d errors=%+v", len(result.Nodes), result.Errors)
	}
	return result.Nodes[0]
}

func invalidClientForTest(t *testing.T, text string) model.ParseResult {
	t.Helper()
	result := parseClientForTest(t, text)
	if len(result.Nodes) != 0 || result.TotalErrors == 0 {
		t.Fatalf("expected rejected client input; nodes=%d errors=%+v", len(result.Nodes), result.Errors)
	}
	return result
}

func TestClientFormatsTrojanSamples(t *testing.T) {
	fixtures := []struct{ name, text string }{
		{"qx", "[server_local]\ntrojan=node.invalid:443,password=p,over-tls=true,tls-host=tls.invalid,tls-verification=false,fast-open=true,tag=label"},
		{"surge", "[Proxy]\nlabel = trojan,node.invalid,443,password=p,sni=tls.invalid,skip-cert-verify=true,tfo=true"},
		{"loon", "[Proxy]\nlabel = trojan,node.invalid,443,\"p\",tls-name=tls.invalid,skip-cert-verify=true,fast-open=true"},
	}
	var first model.ParsedNode
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			node := oneClientForTest(t, fixture.text)
			if node.Name != "label" || node.Protocol != "trojan" || node.Server != "node.invalid" || node.Port != 443 || node.UDP {
				t.Fatal("sample metadata changed or implicit UDP was enabled")
			}
			if node.Config["password"] != "p" || node.Config["skip-cert-verify"] != true || node.Config["sni"] != "tls.invalid" || node.Config["tfo"] != true {
				t.Fatal("sample credentials or TLS/fast-open values changed")
			}
			if first.Config == nil {
				first = node
			} else if !reflect.DeepEqual(first.Config, node.Config) || Fingerprint(first) != Fingerprint(node) {
				t.Fatal("equivalent client formats produced different native identity")
			}
		})
	}
}

func TestClientFormatsRawListAndStaticProtocols(t *testing.T) {
	tests := []struct {
		name, text, protocol, credentialKey, credential string
	}{
		{"qx-ss", "shadowsocks=node.invalid:8388,method=aes-128-gcm,password=p,tag=ss", "ss", "password", "p"},
		{"surge-ss", "ss-name=ss,node.invalid,8388,encrypt-method=aes-128-gcm,password=p", "ss", "password", "p"},
		{"loon-ss", "ss-name=Shadowsocks,node.invalid,8388,aes-128-gcm,\"p\"", "ss", "password", "p"},
		{"qx-vmess", "vmess=node.invalid:80,method=chacha20-ietf-poly1305,password=" + clientTestUUID + ",aead=true", "vmess", "uuid", clientTestUUID},
		{"surge-vmess", "vm-name=vmess,node.invalid,80,username=" + clientTestUUID + ",vmess-aead=true", "vmess", "uuid", clientTestUUID},
		{"loon-vmess", "VMess=VMess,node.invalid,80,auto,\"" + clientTestUUID + "\",alterId=0,transport=tcp", "vmess", "uuid", clientTestUUID},
		{"qx-vless", "vless=node.invalid:80,method=none,password=" + clientTestUUID, "vless", "uuid", clientTestUUID},
		{"loon-vless", "vl-name=VLESS,node.invalid,80,\"" + clientTestUUID + "\",transport=tcp", "vless", "uuid", clientTestUUID},
		{"qx-socks", "socks5=node.invalid:1080,username=u,password=p", "socks5", "password", "p"},
		{"surge-socks", "so-name=socks5,node.invalid,1080,username=u,password=p", "socks5", "password", "p"},
		{"loon-socks", "so-name=socks5,node.invalid,1080,u,\"p\"", "socks5", "password", "p"},
		{"qx-http", "http=node.invalid:8080,username=u,password=p", "http", "password", "p"},
		{"surge-http", "ht-name=http,node.invalid,8080,username=u,password=p", "http", "password", "p"},
		{"loon-http", "ht-name=http,node.invalid,8080,u,\"p\"", "http", "password", "p"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := oneClientForTest(t, tt.text)
			if node.Protocol != tt.protocol || node.Config[tt.credentialKey] != tt.credential || node.UDP {
				t.Fatal("static protocol or exact credentials were not preserved")
			}
			if tt.name == "qx-vmess" && node.Config["cipher"] != "chacha20-poly1305" || tt.name == "surge-vmess" && node.Config["cipher"] != "aes-128-gcm" {
				t.Fatal("documented VMess cipher default or alias was not preserved")
			}
		})
	}
	result := parseClientForTest(t, "http=first.invalid:80,tag=repeated\nlabel=socks5,second.invalid,1080\nhttp=third.invalid:80,tag=repeated")
	if len(result.Nodes) != 3 || result.TotalErrors != 0 || result.Nodes[0].Name != result.Nodes[2].Name {
		t.Fatal("raw assignments must preserve order and repeated display labels")
	}
}

func TestClientFormatsFullDocumentExtraction(t *testing.T) {
	ignored := "[General]\nloglevel=notify\n[DNS]\ndoh-server=https://dns.invalid/dns-query?token=none\n[Script]\nhttps://script.invalid/script.js?token=none\nnot valid,proxy=trojan\n[MITM]\nca-p12=synthetic-not-a-certificate\n[Remote Rule]\nhttps://rules.invalid/list\n[Proxy Group]\nP=select,DIRECT\n[rewrite_remote]\nhttps://rewrite.invalid/rules\n"
	for _, tt := range []struct{ section, node string }{
		{"Proxy", "label=trojan,node.invalid,443,password=p"},
		{"server_local", "trojan=node.invalid:443,password=p,tag=label"},
	} {
		t.Run(tt.section, func(t *testing.T) {
			text := ignored + "[" + tt.section + "]\n" + tt.node + "\n" + ignored
			node := oneClientForTest(t, text)
			if node.Protocol != "trojan" || node.Name != "label" {
				t.Fatal("full document did not extract only its static node")
			}
		})
	}
	oneClientForTest(t, "[server_remote]\n# no providers\n; https://provider.invalid/list\n// comment\n[Remote Proxy]\n[Proxy]\nlabel=http,node.invalid,80")
}

func TestClientFormatsManagedProfileHeader(t *testing.T) {
	prefix := "#!MANAGED-CONFIG https://subscription.invalid/profile?token=synthetic interval=86400 strict=false\n"
	node := oneClientForTest(t, prefix+"[General]\nloglevel=notify\n[Proxy]\nlabel=http,node.invalid,80")
	if node.Protocol != "http" {
		t.Fatal("managed refresh metadata affected node extraction")
	}
	for _, header := range []string{
		"#!MANAGED-CONFIG",
		"#!MANAGED-CONFIGX https://subscription.invalid/",
		"#!MANAGED-CONFIG " + strings.Repeat("x", 8192),
		"#!MANAGED-CONFIG https://subscription.invalid/\tinterval=1",
		"#!MANAGED-CONFIG https://subscription.invalid/\x00interval=1",
		strings.TrimSuffix(prefix+prefix, "\n"),
	} {
		result := parseClientForTest(t, header+"\n[Proxy]\nlabel=http,node.invalid,80")
		if result.TotalErrors == 0 || len(result.Nodes) != 1 {
			t.Fatal("malformed managed metadata must error without losing valid sibling")
		}
	}
	invalidClientForTest(t, "[Proxy]\n"+prefix)
}

func TestClientFormatsBuiltinsAreOnlyDocumentPolicy(t *testing.T) {
	for _, declarations := range []string{"DIRECT=direct\nREJECT=reject", "  DIRECT = DIRECT \n REJECT = REJECT"} {
		oneClientForTest(t, "[Proxy]\n"+declarations+"\nlabel=http,node.invalid,80")
		invalidClientForTest(t, "[Proxy]\n"+declarations)
	}
	for _, bad := range []string{"arbitrary=direct", "arbitrary=reject", "direct=direct", "DIRECT=reject", "DIRECT=direct,interface=en0", "DIRECT=direct,", "REJECT=reject,foo=bar", "PASS=pass", "select=select,A,B", "\"DIRECT\"=direct"} {
		t.Run(fmt.Sprint(bad), func(t *testing.T) {
			invalidClientForTest(t, "[Proxy]\n"+bad)
		})
	}
	for _, prefix := range []string{"", "[server_local]\n"} {
		result := parseClientForTest(t, prefix+"DIRECT=direct\ntrojan=node.invalid:443,password=p")
		if result.TotalErrors != 1 || len(result.Nodes) != 1 {
			t.Fatal("raw/QX pseudo-node must not be silently ignored")
		}
	}
}

func TestClientFormatsRemoteNodeProvidersAndDirectives(t *testing.T) {
	for _, section := range []string{"server_remote", "Remote Proxy"} {
		text := "[Proxy]\nlabel=http,node.invalid,80\n[" + section + "]\nhttps://subscription.invalid/private?token=canary\nprovider=https://subscription.invalid/other,enabled=false\n[Rule]\nhttps://rule.invalid/ignored\n[Proxy]\nlater=http,later.invalid,80"
		result := parseClientForTest(t, text)
		if len(result.Nodes) != 2 || result.TotalErrors != 2 || result.Errors[0].Line != 4 || result.Errors[1].Line != 5 {
			t.Fatal("populated remote providers must be counted as failures, even if disabled")
		}
	}
	for _, directive := range []string{"#!include secret.conf", "#!include https://subscription.invalid/private", "#!REQUIREMENT SURGE-MAC", "#!IOS-ONLY", "#!UNKNOWN"} {
		result := parseClientForTest(t, "[Proxy]\n"+directive+"\nlabel=http,node.invalid,80")
		if len(result.Nodes) != 1 || result.TotalErrors != 1 || result.Errors[0].Line != 2 {
			t.Fatal("node include or conditional directive masqueraded as comment")
		}
	}
	oneClientForTest(t, "[Rule]\n#!include https://rule.invalid/ignored\n[Script]\n#!include script.js\n[Proxy]\nlabel=http,node.invalid,80")
}

func TestClientFormatsIndirectGroupProviders(t *testing.T) {
	for _, group := range []string{
		"external=select,policy-path=https://provider.invalid/nodes?token=canary",
		"external=select,policy-path=/local/nodes.conf",
		"external=select,policy-path=",
		"external=select,policy-path=\"https://provider.invalid/a,b?x=y\",enabled=false",
		"external=select,policy-path=first,policy-path=second",
		"external=select,Policy-Path=https://provider.invalid/",
		"external=select,policy_path=https://provider.invalid/",
		"external=select,\"policy-path\"=https://provider.invalid/",
		"external=select,policy-path=\"unterminated",
		"#!include https://provider.invalid/groups",
		"#!REQUIREMENT CLIENT-MAC",
	} {
		for _, section := range []string{"Proxy Group", "Proxy  Group", "policy"} {
			text := "[Proxy]\nlocal=http,local.invalid,80\n[" + section + "]\n" + group + "\n[Proxy]\nlater=http,later.invalid,80"
			result := parseClientForTest(t, text)
			if len(result.Nodes) != 2 || result.TotalErrors != 1 || result.Errors[0].Line != 4 {
				t.Fatalf("indirect group node expansion was not an explicit source failure: nodes=%d errors=%+v", len(result.Nodes), result.Errors)
			}
			// Exercise the adapter on base64-decoded text without depending on
			// Lead-owned parser routing. The adapter itself never decodes/fetches.
			encoded := base64.StdEncoding.EncodeToString([]byte(text))
			decoded, ok := decodeBase64(encoded, true)
			if !ok {
				t.Fatal("synthetic client base64 did not decode")
			}
			if got := parseClientForTest(t, decoded); !reflect.DeepEqual(got, result) {
				t.Fatal("base64-decoded group source lost its completeness error")
			}
		}
	}
	for _, text := range []string{
		"[Proxy Group]\nexternal=select,policy-path=https://provider.invalid/",
		"[Proxy Group]\n#!include groups.conf",
		"#!include groups.conf\n[Proxy]\nlocal=http,local.invalid,80",
		"[Proxy Group]\nexternal=select,policy-path=https://provider.invalid/\n[Proxy]\nlocal=http,local.invalid,80",
	} {
		result := parseClientForTest(t, text)
		if result.TotalErrors != 1 {
			t.Fatal("provider-only or leading group include must remain a source error")
		}
	}
	for _, label := range []string{"policy-path", "\"policy-path\""} {
		result := parseClientForTest(t, "[Proxy]\nlocal=http,local.invalid,80\n[Proxy Group]\n"+label+"=select,local,policy-path=https://provider.invalid/")
		if result.TotalErrors != 1 || len(result.Nodes) != 1 {
			t.Fatal("group label exemption hid a real provider option")
		}
	}
	result := parseClientForTest(t, "[Proxy]\nlocal=http,local.invalid,80\n[Proxy Group]\n"+strings.Repeat("external=select,policy-path=https://provider.invalid/\n", maxIssues+1)+"[Proxy]\nlater=http,later.invalid,80")
	if len(result.Nodes) != 2 || result.TotalErrors != maxIssues+1 || !result.ErrorsTruncated {
		t.Fatal("group completeness errors were truncated instead of counted")
	}
}

func TestClientFormatsOrdinaryGroupsRemainInert(t *testing.T) {
	for _, group := range []string{
		"all=select,local,DIRECT",
		"policy-path=select,local,DIRECT",
		"\"policy-path\"=select,local,DIRECT",
		"auto=url-test,local,url=https://test.invalid/check,interval=300",
		"all=select,include-all-proxies=true,include-other-group=\"local,other\"",
		"all=select,policy-regex-filter=\"policy-path=not-a-provider\"",
		"all=select,\"policy-path=literal-member-name\"",
		"all=select," + strings.Repeat("member,", 200) + "DIRECT",
	} {
		for _, section := range []string{"Proxy Group", "policy"} {
			oneClientForTest(t, "["+section+"]\n"+group+"\n[Proxy]\nlocal=http,local.invalid,80")
		}
	}
	oneClientForTest(t, "[Rule]\nPOLICY,policy-path=not-a-node\n[Script]\npolicy-path=https://script.invalid/ignored\n[Proxy]\nlocal=http,local.invalid,80")
}

func TestClientFormatsDoNotRecognizeOtherFormats(t *testing.T) {
	uri := "trojan://p@node.invalid:443?sni=tls.invalid#label"
	assignment := "label=trojan,node.invalid,443,password=p"
	for _, text := range []string{
		"", "# only comment\n; still comment\n// again", uri,
		"STATUS=traffic\n" + uri,
		"STATUS=vmess\n" + uri,
		assignment + "\n" + uri,
		"bad URI\n" + uri,
		"https://subscription.invalid/?label=trojan,node.invalid,443,password=p",
		"trojan://p@node.invalid:443?name=trojan,node.invalid,443,password=p",
		"proxies:\n - name: '" + assignment + "'\n   type: trojan",
		"\"proxies\":\n  - \"" + assignment + "\"",
		"{\"content\": \"" + assignment + "\"}",
		"[\"" + assignment + "\"]",
		"[ {type: http, server: node.invalid, port: 80} ]",
		"payload: |\n  " + assignment,
		"? proxies\n:\n  - name: " + assignment + "\n    type: http\n    server: node.invalid\n    port: 80",
		assignment + ": yaml-value",
		"trojan=node.invalid:443,password=p: yaml-value",
		"|\n  " + assignment,
		">-\n  " + assignment,
		"\"multiline\n" + assignment + "\"",
		"'multiline\n" + assignment + "'",
		"---\n" + assignment,
		"%YAML 1.2\n" + assignment,
		"&anchor\n" + assignment,
		"!tag\n" + assignment,
		"<html>\n" + assignment + "\n</html>",
		base64.StdEncoding.EncodeToString([]byte(assignment)),
		base64.RawURLEncoding.EncodeToString([]byte(assignment)),
		"[Script]\n" + assignment + "\nhttps://script.invalid/",
		"[General]\nproxy=" + assignment,
	} {
		result := model.ParseResult{TotalErrors: 2, Errors: []model.ImportIssue{{Line: 1, Message: "existing"}}, Nodes: []model.ParsedNode{{Name: "existing"}}}
		before, _ := json.Marshal(result)
		if tryParseClientText(text, &result) {
			t.Fatal("another format was incorrectly claimed by client adapter")
		}
		after, _ := json.Marshal(result)
		if string(before) != string(after) {
			t.Fatal("unrecognized format mutated parse result")
		}
	}
}

func TestClientFormatsLeaveExplicitYAMLKeysToDocumentParser(t *testing.T) {
	body := "? proxies\n:\n  - name: n=trojan,node.invalid,443,password=p\n    type: http\n    server: node.invalid\n    port: 80\n"
	for _, input := range []string{body, base64.StdEncoding.EncodeToString([]byte(body))} {
		node := parseOne(t, input)
		if node.Protocol != "http" || node.Port != 80 {
			t.Fatal("explicit YAML mapping was mistaken for client assignments")
		}
	}
}

func TestClientFormatsQuotedCredentialsAndIPv6(t *testing.T) {
	secret := "p,=a#b;//c+%2F@:\\end\""
	quoted := "\"" + strings.NewReplacer("\\", "\\\\", "\"", "\\\"").Replace(secret) + "\""
	for _, text := range []string{
		"trojan=[2001:db8::1]:443,password=" + quoted + ",tag=\"comma,=label\"",
		"\"comma,=label\"=trojan,[2001:db8::1],443,password=" + quoted,
		"\"comma,=label\"=trojan,[2001:db8::1],443," + quoted,
	} {
		node := oneClientForTest(t, text)
		if node.Name != "comma,=label" || node.Server != "2001:db8::1" || node.Config["password"] != secret {
			t.Fatal("CSV quoting, escape or IPv6 normalization changed data")
		}
	}
	for _, secret := range []string{"a#b", "p;rest", "https://secret.invalid/path", "=a==", "#first", ";first", "//first", "percent%2Fplus+"} {
		node := oneClientForTest(t, "trojan=node.invalid:443,password="+secret)
		if node.Config["password"] != secret {
			t.Fatal("unquoted secret was truncated or URL decoded")
		}
	}
	node := oneClientForTest(t, "label=trojan,node.invalid,443,\"  password  \"")
	if node.Config["password"] != "  password  " {
		t.Fatal("quoted significant whitespace was trimmed")
	}
	oneClientForTest(t, "\ufeff;comment\r\n# comment\r\n // comment\r\n[PrOxY]\r\n label = trojan, NODE.INVALID, 443, password = p\r\n")
	for _, endpoint := range []string{"2001:db8::1:443", "[2001:db8::1]", "node.invalid:", ":443", "[fe80::1%zone]:443"} {
		invalidClientForTest(t, "trojan="+endpoint+",password=p")
	}
}

func TestClientFormatsWebSocketTLSMappings(t *testing.T) {
	tests := []struct{ name, text, protocol, host, sni string }{
		{"qx-vmess", "vmess=node.invalid:443,method=none,password=" + clientTestUUID + ",obfs=wss,obfs-host=cdn.invalid,obfs-uri=\"/ws?a=b,c\",tls-verification=true", "vmess", "cdn.invalid", "cdn.invalid"},
		{"qx-vless", "vless=node.invalid:443,method=none,password=" + clientTestUUID + ",obfs=wss,obfs-host=cdn.invalid,obfs-uri=\"/ws?a=b,c\"", "vless", "cdn.invalid", "cdn.invalid"},
		{"qx-trojan", "trojan=node.invalid:443,password=p,obfs=wss,obfs-host=cdn.invalid,obfs-uri=\"/ws?a=b,c\"", "trojan", "cdn.invalid", "cdn.invalid"},
		{"surge-vmess", "n=vmess,node.invalid,443,username=" + clientTestUUID + ",vmess-aead=true,tls=true,sni=tls.invalid,ws=true,ws-path=\"/ws?a=b,c\",ws-headers=\"Host:cdn.invalid|X-Token:a=b,c\"", "vmess", "cdn.invalid", "tls.invalid"},
		{"loon-vless", "n=vless,node.invalid,443,\"" + clientTestUUID + "\",transport=ws,over-tls=true,tls-name=tls.invalid,host=cdn.invalid,path=\"/ws?a=b,c\"", "vless", "cdn.invalid", "tls.invalid"},
		{"loon-trojan", "n=trojan,node.invalid,443,\"p\",transport=http,tls-name=tls.invalid,host=cdn.invalid,path=\"/ws?a=b,c\"", "trojan", "cdn.invalid", "tls.invalid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := oneClientForTest(t, tt.text)
			ws := node.Config["ws-opts"].(map[string]any)
			headers := ws["headers"].(map[string]any)
			sni := "sni"
			if tt.protocol == "vmess" || tt.protocol == "vless" {
				sni = "servername"
			}
			if node.Config["network"] != "ws" || ws["path"] != "/ws?a=b,c" || headers["Host"] != tt.host || node.Config[sni] != tt.sni || node.Config["skip-cert-verify"] != false {
				t.Fatal("WebSocket/TLS active settings were not preserved")
			}
			if tt.name == "surge-vmess" && headers["X-Token"] != "a=b,c" {
				t.Fatal("quoted custom WebSocket header was changed")
			}
		})
	}
	for _, text := range []string{
		"vmess=node.invalid:443,method=none,password=" + clientTestUUID + ",obfs=over-tls,obfs-host=tls.invalid",
		"n=https,node.invalid,443,username=u,password=p,sni=tls.invalid",
		"http=node.invalid:443,username=u,password=p,over-tls=true,tls-host=tls.invalid",
		"n=socks5-tls,node.invalid,443,u,p,skip-cert-verify=false",
		"socks5=node.invalid:443,username=u,password=p,over-tls=true,tls-verification=true",
	} {
		node := oneClientForTest(t, text)
		if node.Config["tls"] != true || node.Config["skip-cert-verify"] != false {
			t.Fatal("TLS was omitted or certificate verification was weakened")
		}
	}
}

func TestClientFormatsWebSocketTLSIdentityDefaults(t *testing.T) {
	for _, server := range []string{"node.invalid", "192.0.2.1", "[2001:db8::1]"} {
		canonical, _ := validHost(server)
		for _, base := range []string{
			"n=vmess," + server + ",443,username=" + clientTestUUID + ",vmess-aead=true,tls=true,ws=true",
			"n=VMess," + server + ",443,auto,\"" + clientTestUUID + "\",over-tls=true,transport=ws",
			"n=VLESS," + server + ",443,\"" + clientTestUUID + "\",over-tls=true,transport=ws",
		} {
			for _, suffix := range []string{"", ",sni=", ",host=" + server, ",sni=,host=" + server} {
				node := oneClientForTest(t, base+suffix)
				if node.Config["servername"] != canonical || node.Config["skip-cert-verify"] != false {
					t.Fatal("unambiguous TLS identity did not use the canonical endpoint")
				}
			}
			for _, suffix := range []string{",host=distinct.invalid", ",host=distinct.invalid,sni=", ",ws-headers=Host:distinct.invalid", ",ws-headers=Host:distinct.invalid,tls-name=\"\""} {
				invalidClientForTest(t, base+suffix)
			}
			node := oneClientForTest(t, base+",host=distinct.invalid,sni=certificate.invalid")
			ws := node.Config["ws-opts"].(map[string]any)
			headers := ws["headers"].(map[string]any)
			if node.Config["servername"] != "certificate.invalid" || headers["Host"] != "distinct.invalid" {
				t.Fatal("explicit TLS identity was conflated with the WebSocket Host")
			}
		}
		for _, protocol := range []string{"vmess", "vless"} {
			base := protocol + "=" + server + ":443,method=none,password=" + clientTestUUID + ",obfs=wss"
			node := oneClientForTest(t, base)
			if node.Config["servername"] != canonical {
				t.Fatal("QX absent obfuscation host did not retain endpoint identity")
			}
			node = oneClientForTest(t, base+",obfs-host=cdn.invalid")
			if node.Config["servername"] != "cdn.invalid" || node.Config["ws-opts"].(map[string]any)["headers"].(map[string]any)["Host"] != "cdn.invalid" {
				t.Fatal("documented QX coupled TLS and HTTP host was changed")
			}
		}
		for _, suffix := range []string{"", ",sni=", ",sni=certificate.invalid"} {
			node := oneClientForTest(t, "n=trojan,"+server+",443,password=p,ws=true,ws-headers=Host:distinct.invalid"+suffix)
			want := canonical
			if suffix == ",sni=certificate.invalid" {
				want = "certificate.invalid"
			}
			if node.Config["sni"] != want || node.Config["ws-opts"].(map[string]any)["headers"].(map[string]any)["Host"] != "distinct.invalid" {
				t.Fatal("Trojan endpoint TLS identity changed with independent HTTP Host")
			}
		}
	}
}

func TestClientFormatsExplicitTransportHosts(t *testing.T) {
	for _, mode := range []string{"http", "tls", "ws", "wss"} {
		base := "shadowsocks=node.invalid:443,method=aes-128-gcm,password=p,obfs=" + mode
		for _, suffix := range []string{"", ",obfs-host=", ",obfs-host=\"\""} {
			invalidClientForTest(t, base+suffix)
		}
		node := oneClientForTest(t, base+",obfs-host=obfs.invalid")
		if node.Config["plugin-opts"].(map[string]any)["host"] != "obfs.invalid" {
			t.Fatal("explicit obfuscation host changed")
		}
	}
	for _, base := range []string{
		"n=ss,node.invalid,443,encrypt-method=aes-128-gcm,password=p,obfs=tls",
		"n=Shadowsocks,node.invalid,443,aes-128-gcm,\"p\",obfs-name=http",
	} {
		invalidClientForTest(t, base)
		oneClientForTest(t, base+",obfs-host=obfs.invalid")
	}
	for _, server := range []string{"node.invalid", "192.0.2.1", "[2001:db8::1]"} {
		for _, credential := range []string{"password=p,ws=true", "\"p\",transport=ws"} {
			base := "n=trojan," + server + ",443," + credential
			for _, suffix := range []string{",sni=certificate.invalid", ",sni=certificate.invalid,host=", ",sni=certificate.invalid,ws-headers=\"Host:\""} {
				invalidClientForTest(t, base+suffix)
			}
			for _, suffix := range []string{",sni=" + server, "", ",sni="} {
				oneClientForTest(t, base+suffix)
			}
			node := oneClientForTest(t, base+",sni=certificate.invalid,host=http.invalid")
			if node.Config["sni"] != "certificate.invalid" || node.Config["ws-opts"].(map[string]any)["headers"].(map[string]any)["Host"] != "http.invalid" {
				t.Fatal("explicit independent Trojan TLS and HTTP identities changed")
			}
		}
	}
	oneClientForTest(t, "trojan=node.invalid:443,password=p,obfs=wss,obfs-host=coupled.invalid")
}

func TestClientFormatsShadowsocksPlugins(t *testing.T) {
	for _, text := range []string{
		"shadowsocks=node.invalid:443,method=aes-128-gcm,password=p,obfs=tls,obfs-host=cdn.invalid",
		"n=ss,node.invalid,443,encrypt-method=aes-128-gcm,password=p,obfs=tls,obfs-host=cdn.invalid",
		"n=Shadowsocks,node.invalid,443,aes-128-gcm,\"p\",obfs-name=tls,obfs-host=cdn.invalid",
	} {
		node := oneClientForTest(t, text)
		opts := node.Config["plugin-opts"].(map[string]any)
		if node.Config["plugin"] != "obfs" || opts["mode"] != "tls" || opts["host"] != "cdn.invalid" || node.Config["tls"] != nil {
			t.Fatal("simple-obfs TLS was incorrectly promoted to actual TLS")
		}
	}
	for _, mode := range []string{"ws", "wss"} {
		text := "shadowsocks=node.invalid:443,method=aes-128-gcm,password=p,obfs=" + mode + ",obfs-host=cdn.invalid,obfs-uri=/ws"
		if mode == "wss" {
			text += ",tls-verification=true"
		}
		node := oneClientForTest(t, text)
		opts := node.Config["plugin-opts"].(map[string]any)
		if node.Config["plugin"] != "v2ray-plugin" || opts["mode"] != "websocket" || opts["host"] != "cdn.invalid" || opts["path"] != "/ws" || opts["tls"] != (mode == "wss") || opts["mux"] != false {
			t.Fatal("Quantumult X SS WebSocket was not mapped to non-mux plugin")
		}
	}
	for _, option := range []string{"obfs=over-tls", "obfs=tls,obfs-uri=/must-not-drop", "obfs=http,obfs-uri=/must-not-drop", "obfs=ws,tls-verification=false", "obfs=wss,tls-host=separate.invalid", "obfs=wss,over-tls=true", "obfs=unknown", "obfs-host=orphan.invalid", "obfs=none,obfs-uri=/orphan", "plugin=/file/exec", "udp-over-tcp=true"} {
		invalidClientForTest(t, "shadowsocks=node.invalid:443,method=aes-128-gcm,password=p,"+option)
	}
}

func TestClientFormatsHTTPTransportAndExplicitFlags(t *testing.T) {
	for _, text := range []string{
		"vless=node.invalid:80,method=none,password=" + clientTestUUID + ",obfs=http,obfs-host=cdn.invalid,obfs-uri=/h",
		"n=VMess,node.invalid,80,auto,\"" + clientTestUUID + "\",transport=http,host=cdn.invalid,path=/h",
	} {
		node := oneClientForTest(t, text)
		opts := node.Config["http-opts"].(map[string]any)
		if node.Config["network"] != "http" || node.Config["tls"] != false || !reflect.DeepEqual(opts["path"], []string{"/h"}) {
			t.Fatal("HTTP transport was not explicitly preserved")
		}
	}
	for _, text := range []string{
		"trojan=node.invalid:443,password=p,udp-relay=true,fast-open=1",
		"n=trojan,node.invalid,443,\"p\",udp=TRUE,tfo=true",
		"n=socks5,node.invalid,1080,udp-relay=true,tfo=true",
	} {
		node := oneClientForTest(t, text)
		if !node.UDP || node.Config["udp"] != true || node.Config["tfo"] != true {
			t.Fatal("explicit native UDP/TFO setting was not preserved")
		}
	}
	node := oneClientForTest(t, "n=http,node.invalid,80,no-error-alert=true")
	if node.Config["no-error-alert"] != nil {
		t.Fatal("presentation-only client setting reached native map")
	}
}

func TestClientFormatsMalformedAndDuplicateFields(t *testing.T) {
	for _, tail := range []string{
		"password=p,password=p", "password=p,Password=p", "password=p,tfo=true,fast-open=true",
		"password=p,tls=true,over-tls=true", "password=p,sni=tls.invalid,tls-name=tls.invalid",
		"password=p,tls-verification=true,skip-cert-verify=false", "password=p,udp=true,udp-relay=true",
		"password=p,foo=secret", "password=p,foo", "password=p,,tfo=true", "password=p,",
		"password=\"unterminated", "password=\"p\"tail", "password=p\"quote\"", "password=\"bad\\n\"",
		"password=p,tfo=yes", "password=p,tfo=2", "password=p,tfo=", "password=p,tfo=\" true \"",
		"password=", "password=\"\"", "password=p,ws=true,transport=ws", "password=p,ws=true,ws=false",
		"password=p,ws=true,ws-headers=\"Host:a.invalid|host:b.invalid\"",
		"password=p,ws=true,ws-host=a.invalid,ws-headers=Host:b.invalid",
		"password=p,ws=true,host=a.invalid,ws-headers=broken",
		"password=p,ws=true,path=/p,ws-path=/p",
		"password=p # comment", "password=p ; comment", "password=p // comment",
		"password=p,=empty-key", "password=p,bad.key=value", "password=p,\"tfo\"=true",
		"password=p," + strings.Repeat("a", 65) + "=x",
	} {
		t.Run(fmt.Sprint(tail), func(t *testing.T) {
			invalidClientForTest(t, "n=trojan,node.invalid,443,"+tail)
		})
	}
	for _, line := range []string{
		"n=trojan,node.invalid,443,p,password=p", "n=trojan,node.invalid,443,p,p",
		"n=ss,node.invalid,443,aes-128-gcm,p,password=p", "n=ss,node.invalid,443,aes-128-gcm,p,cipher=aes-128-gcm",
		"n=socks5,node.invalid,1080,u", "n=http,node.invalid,80,u,p,username=u",
		"n=http,node.invalid,80,password=p", "n=trojan,node.invalid,0,password=p",
		"n=trojan,node.invalid,+443,password=p", "n=trojan,node.invalid,65536,password=p",
		"n=trojan", "n=trojan,node.invalid", "=trojan,node.invalid,443,password=p",
		"trojan=node.invalid:443,p", "trojan=node.invalid:443,password=p,tag=a,tag=b",
		"n=vmess,node.invalid,443,username=" + clientTestUUID,
		"n=vmess,node.invalid,443,username=" + clientTestUUID + ",vmess-aead=false",
		"n=vmess,node.invalid,443,username=" + clientTestUUID + ",vmess-aead=true,alterId=1",
		"vmess=node.invalid:443,method=none,password=" + clientTestUUID + ",aead=false",
		"vmess=node.invalid:443,method=none,password=" + clientTestUUID + ",aead=true,vmess-aead=true",
	} {
		invalidClientForTest(t, line)
	}
	invalidClientForTest(t, "[Proxy]\ntrojan=node.invalid:443,password=p")
	invalidClientForTest(t, "[server_local]\nn=trojan,node.invalid,443,password=p")
	result := parseClientForTest(t, "[Proxy]\n[broken\nn=trojan,node.invalid,443,password=p\n[Proxy]\nvalid=http,node.invalid,80")
	if result.TotalErrors != 1 || len(result.Nodes) != 1 {
		t.Fatal("malformed section did not remain a source error")
	}
}

func TestClientFormatsUnsafeTLSAndDelegation(t *testing.T) {
	for _, option := range []string{
		"tls=false", "over-tls=false", "sni=off", "ws=false,ws-path=/ignored", "transport=tcp,host=ignored.invalid",
		"transport=grpc", "transport=h2", "tls13=true", "tls-pubkey-sha256=" + strings.Repeat("a", 64),
		"tls-alpn=026832", "client-cert=private", "tls-profile=chrome", "private-key=/file/key",
		"server-cert-fingerprint-sha256=" + strings.Repeat("a", 64) + ",skip-cert-verify=true",
		"underlying-proxy=relay", "dialer-proxy=relay", "detour=relay", "interface=en0",
		"server-dns=https://dns.invalid/query", "server_check_url=https://check.invalid/", "test-url=https://check.invalid/",
		"script=canary", "exec=canary", "plugin=canary", "provider=canary", "parser-plugin=canary", "foo-file=canary",
		"block-quic=false", "hybrid=off", "enabled=false", "no-error-alert=invalid",
	} {
		invalidClientForTest(t, "n=trojan,node.invalid,443,password=p,"+option)
	}
	for _, text := range []string{
		"n=https,node.invalid,443,tls=false", "n=socks5-tls,node.invalid,443,over-tls=false",
		"socks5=node.invalid:443,over-tls=true,tls-host=override.invalid",
		"http=node.invalid:443,over-tls=true,alpn=h2", "n=http,node.invalid,80,udp=true",
		"n=vless,node.invalid,80,\"" + clientTestUUID + "\",over-tls=false,sni=tls.invalid",
		"n=vless,node.invalid,80,\"" + clientTestUUID + "\",over-tls=false,skip-cert-verify=true",
		"vless=node.invalid:443,method=none,password=" + clientTestUUID + ",obfs=wss,over-tls=true",
		"trojan=node.invalid:443,password=p,obfs=wss,tls-host=override.invalid,obfs-host=cdn.invalid",
		"trojan=node.invalid:443,password=p,obfs=wss,tls-host=override.invalid",
		"vmess=node.invalid:443,method=none,password=" + clientTestUUID + ",obfs=over-tls,sni=override.invalid",
		"trojan=node.invalid:443,password=p,obfs=ws", "trojan=node.invalid:443,password=p,obfs=over-tls",
	} {
		invalidClientForTest(t, text)
	}
}

func TestClientFormatsCertificateVerificationNameConflicts(t *testing.T) {
	for _, flag := range []string{"skip-cert-verify=true", "tls-verification=false"} {
		for _, options := range []string{flag + ",server-cert-verify-name=verify.invalid", "server-cert-verify-name=verify.invalid," + flag} {
			invalidClientForTest(t, "n=trojan,node.invalid,443,password=p,"+options)
			invalidClientForTest(t, "shadowsocks=node.invalid:443,method=aes-128-gcm,password=p,obfs=wss,obfs-host=obfs.invalid,"+options)
		}
		node := oneClientForTest(t, "n=trojan,node.invalid,443,password=p,"+flag)
		if node.Config["skip-cert-verify"] != true || node.Config["name-cert-verify"] != nil {
			t.Fatal("explicit insecure without a verification override was changed")
		}
	}
	for _, flag := range []string{"skip-cert-verify=false", "tls-verification=true"} {
		node := oneClientForTest(t, "n=trojan,node.invalid,443,password=p,"+flag+",server-cert-verify-name=verify.invalid")
		if node.Config["skip-cert-verify"] != false || node.Config["name-cert-verify"] != "verify.invalid" {
			t.Fatal("explicit verification name and enabled validation were not retained")
		}
	}
}

func TestClientFormatsPinAndALPNPreservation(t *testing.T) {
	pin := strings.Repeat("AB", 32)
	node := oneClientForTest(t, "n=trojan,node.invalid,443,password=p,tls-cert-sha256="+pin+",alpn=\"h2,http/1.1\",server-cert-verify-name=verify.invalid")
	if node.Config["fingerprint"] != strings.ToLower(pin) || node.Config["name-cert-verify"] != "verify.invalid" || !reflect.DeepEqual(node.Config["alpn"], []string{"h2", "http/1.1"}) {
		t.Fatal("certificate pin, verification name or ALPN was changed")
	}
	invalidClientForTest(t, "vmess=node.invalid:443,method=none,password="+clientTestUUID+",obfs=wss,alpn=h2")
}

func TestClientFormatsErrorsAreSecretSafeAndDeterministic(t *testing.T) {
	canary := "CLIENT_PRIVATE_CANARY"
	inputs := []string{
		"[Proxy]\n" + canary + "=trojan," + canary + ".invalid,443,password=" + canary + "," + canary + "=value,exec=" + canary,
		"[Remote Proxy]\nhttps://subscription.invalid/?token=" + canary,
		"trojan=node.invalid:443,password=\"" + canary,
		"[Proxy]\n" + canary + "=unsupported," + canary + ".invalid,443," + canary,
		"[Proxy]\n#!include " + canary,
	}
	for _, input := range inputs {
		first := invalidClientForTest(t, input)
		bytes, _ := json.Marshal(first)
		if strings.Contains(string(bytes), canary) {
			t.Fatal("client diagnostic echoed input key/value/name/provider data")
		}
		for i := 0; i < 30; i++ {
			result := invalidClientForTest(t, input)
			if !reflect.DeepEqual(first, result) {
				t.Fatal("client diagnostics are nondeterministic")
			}
		}
	}
}

func TestClientFormatsErrorCapContinuesScanning(t *testing.T) {
	for _, errors := range []int{maxIssues - 1, maxIssues, maxIssues + 1, 1000} {
		for _, prefix := range []string{"", "[Proxy]\n"} {
			text := prefix + "before=http,before.invalid,80\n" + strings.Repeat("bad=trojan,node.invalid,443,password=p,unknown=canary\n", errors) + "after=http,after.invalid,80"
			result := parseClientForTest(t, text)
			if len(result.Nodes) != 2 || result.TotalErrors != errors || result.Nodes[0].Name != "before" || result.Nodes[1].Name != "after" {
				t.Fatal("client detail cap stopped parsing later valid siblings")
			}
			offset := 2
			if prefix != "" {
				offset++
			}
			for i, problem := range result.Errors {
				if problem.Line != i+offset {
					t.Fatal("client diagnostics lost original line order")
				}
			}
		}
	}
}

func TestClientFormatsBounds(t *testing.T) {
	for _, count := range []int{MaxNodes, MaxNodes + 1} {
		result := parseClientForTest(t, "[Proxy]\nDIRECT=direct\n"+strings.Repeat("n=http,node.invalid,80\n", count))
		if len(result.Nodes) != MaxNodes || result.TotalErrors != count-MaxNodes {
			t.Fatal("client entry bound or non-node builtin count is wrong")
		}
		if count > MaxNodes && result.Errors[0].Line != MaxNodes+3 {
			t.Fatal("node bound has wrong source line")
		}
	}
	for _, input := range []string{
		"[Proxy]\n" + strings.Repeat("x", MaxContentBytes),
		"\ufeff[Proxy]\n#" + strings.Repeat("x", MaxContentBytes-len("[Proxy]\n#")),
		"[Proxy]\n" + strings.Repeat("x", maxClientLineBytes+1),
		"trojan=node.invalid:443,password=" + strings.Repeat("x", maxClientFieldBytes+1),
		"trojan=node.invalid:443,password=p," + strings.Repeat("x=y,", maxClientFields),
		"trojan=node.invalid:443,password=p\x00",
		"trojan=node.invalid:443,password=p\xff",
	} {
		invalidClientForTest(t, input)
	}
	// A valid line slightly beyond Scanner's usual 64 KiB ceiling must survive.
	secret := strings.Repeat("p", maxClientFieldBytes)
	node := oneClientForTest(t, "trojan=node.invalid:443,password="+secret)
	if node.Config["password"] != secret {
		t.Fatal("large valid client field was truncated")
	}
	longName := strings.Repeat("界", MaxNodeNameRunes+20)
	node = oneClientForTest(t, "trojan=node.invalid:443,password=p,tag="+longName)
	if !utf8.ValidString(node.Name) || utf8.RuneCountInString(node.Name) > MaxNodeNameRunes || !strings.HasSuffix(node.Name, "…") {
		t.Fatal("client display-name normalization differs from native")
	}
}

func TestClientFormatsConcurrentDeterminism(t *testing.T) {
	text := "[Proxy]\nn=trojan,node.invalid,443,password=p,sni=tls.invalid\nbad=trojan,node.invalid,443,password=p,unknown=canary,exec=canary"
	want := parseClientForTest(t, text)
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var result model.ParseResult
			if !tryParseClientText(text, &result) || !reflect.DeepEqual(want, result) {
				t.Error("concurrent client parsing changed its result")
			}
		}()
	}
	wg.Wait()
}

func FuzzClientFormatsNeverPanic(f *testing.F) {
	for _, seed := range []string{
		"[Proxy]\nn=trojan,node.invalid,443,password=p",
		"[server_local]\nvmess=node.invalid:443,method=none,password=" + clientTestUUID + ",obfs=wss",
		"[Remote Proxy]\nhttps://subscription.invalid/",
		"[Proxy Group]\nexternal=select,policy-path=https://provider.invalid/",
		"[Proxy Group]\n#!include groups.conf\n[Proxy]\nn=http,node.invalid,80",
		"shadowsocks=node.invalid:443,method=aes-128-gcm,password=p,obfs=wss",
		"n=trojan,node.invalid,443,password=p,ws=true,sni=certificate.invalid",
		"n=vmess,node.invalid,443,username=" + clientTestUUID + ",vmess-aead=true,tls=true,ws=true,ws-headers=Host:distinct.invalid",
		"n=trojan,node.invalid,443,\"p,=q\",ws=true,host=cdn.invalid,ws-headers=broken",
		"[Proxy]\nDIRECT=direct",
		"trojan://p@node.invalid:443?sni=tls.invalid",
		"proxies:\n  - {name: n, type: http, server: node.invalid, port: 80}",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, text string) {
		var result model.ParseResult
		recognized := tryParseClientText(text, &result)
		if !recognized {
			if result.TotalErrors != 0 || len(result.Errors) != 0 || len(result.Nodes) != 0 {
				t.Fatal("unrecognized client input mutated output")
			}
			return
		}
		if len(result.Nodes) > MaxNodes || len(result.Errors) > maxIssues || len(result.Errors) != min(result.TotalErrors, maxIssues) || result.ErrorsTruncated != (result.TotalErrors > maxIssues) {
			t.Fatal("client parsing exceeded a bound or miscounted errors")
		}
		for _, node := range result.Nodes {
			if Fingerprint(node) == "" || node.DialerProxy != "" || node.DialerFingerprint != "" {
				t.Fatal("client parser emitted an invalid strict native node")
			}
		}
	})
}
