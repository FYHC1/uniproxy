package importer

import (
	"net/url"
	"testing"

	"github.com/uniproxy/uniproxy/internal/model"
)

func TestXrayFinalMaskPreserved(t *testing.T) {
	mask := `{"tcp":[{"type":"fragment","settings":{"packets":"tlshello","lengths":["0","104","1"],"delays":["0"],"maxSplit":"0"}}]}`
	q := url.Values{}
	q.Set("security", "tls")
	q.Set("type", "ws")
	q.Set("path", "/ws")
	q.Set("host", "front.example")
	q.Set("fm", mask)
	n := parseOne(t, "vless://"+testUUID+"@example.com:443?"+q.Encode())
	if n.Engine != model.EngineXray {
		t.Fatalf("engine = %q, want xray", n.Engine)
	}
	stream := xrayStream(t, n)
	finalmask, ok := stream["finalmask"].(map[string]any)
	if !ok {
		t.Fatalf("finalmask missing: %#v", stream)
	}
	tcp, ok := finalmask["tcp"].([]any)
	if !ok || len(tcp) != 1 {
		t.Fatalf("finalmask tcp = %#v", finalmask["tcp"])
	}
}

func TestXrayRealityExtensionsPreserved(t *testing.T) {
	q := url.Values{}
	q.Set("security", "reality")
	q.Set("pbk", testPublicKey)
	q.Set("sid", "0123456789abcdef")
	q.Set("type", "tcp")
	q.Set("spx", "/a9942fb003e9d0e")
	q.Set("pqv", "verify-key")
	q.Set("flow", "xtls-rprx-vision")
	n := parseOne(t, "vless://"+testUUID+"@example.com:443?"+q.Encode())
	if n.Engine != model.EngineXray {
		t.Fatalf("engine = %q, want xray", n.Engine)
	}
	stream := xrayStream(t, n)
	reality, ok := stream["realitySettings"].(map[string]any)
	if !ok {
		t.Fatalf("reality settings missing: %#v", stream)
	}
	if reality["spiderX"] != "/a9942fb003e9d0e" || reality["mldsa65Verify"] != "verify-key" {
		t.Fatalf("reality extensions lost: %#v", reality)
	}
}

func TestXrayCipherSuitesPreserved(t *testing.T) {
	q := url.Values{}
	q.Set("security", "tls")
	q.Set("type", "ws")
	q.Set("path", "/ws")
	q.Set("cs", "TLS_AES_128_GCM_SHA256:TLS_AES_256_GCM_SHA384")
	n := parseOne(t, "vless://"+testUUID+"@example.com:443?"+q.Encode())
	if n.Engine != model.EngineXray {
		t.Fatalf("engine = %q, want xray", n.Engine)
	}
	tlsSettings, ok := xrayStream(t, n)["tlsSettings"].(map[string]any)
	if !ok || tlsSettings["cipherSuites"] != "TLS_AES_128_GCM_SHA256:TLS_AES_256_GCM_SHA384" {
		t.Fatalf("cipher suites lost: %#v", tlsSettings)
	}
}

func TestXrayVMessXHTTPAuthority(t *testing.T) {
	q := url.Values{}
	q.Set("type", "xhttp")
	q.Set("security", "tls")
	q.Set("path", "/xhttp")
	q.Set("host", "front.example")
	n := parseOne(t, "vmess://"+testUUID+"@example.com:443?"+q.Encode())
	if n.Engine != model.EngineXray {
		t.Fatalf("engine = %q, want xray", n.Engine)
	}
	stream := xrayStream(t, n)
	if stream["network"] != "xhttp" || stream["security"] != "tls" {
		t.Fatalf("unexpected stream: %#v", stream)
	}
	xhttp, ok := stream["xhttpSettings"].(map[string]any)
	if !ok || xhttp["path"] != "/xhttp" || xhttp["host"] != "front.example" {
		t.Fatalf("xhttp settings wrong: %#v", xhttp)
	}
}

func TestXrayVMessXHTTPJSON(t *testing.T) {
	obj := vmessBase()
	obj["net"] = "xhttp"
	obj["path"] = "/xhttp"
	obj["host"] = "front.example"
	obj["tls"] = "tls"
	obj["sni"] = "front.example"
	n := parseOne(t, vmessLink(obj))
	if n.Engine != model.EngineXray {
		t.Fatalf("engine = %q, want xray", n.Engine)
	}
	stream := xrayStream(t, n)
	if stream["network"] != "xhttp" || stream["security"] != "tls" {
		t.Fatalf("unexpected stream: %#v", stream)
	}
	xhttp, ok := stream["xhttpSettings"].(map[string]any)
	if !ok || xhttp["path"] != "/xhttp" || xhttp["host"] != "front.example" {
		t.Fatalf("xhttp settings wrong: %#v", xhttp)
	}
}

func TestXrayVMessXHTTPSecurityFields(t *testing.T) {
	obj := vmessBase()
	obj["net"] = "xhttp"
	delete(obj, "scy")
	obj["security"] = "aes-128-gcm"
	n := parseOne(t, vmessLink(obj))
	if n.Engine != model.EngineXray {
		t.Fatalf("engine = %q, want xray", n.Engine)
	}
	user := xrayVLESSUser(t, n)
	if user["security"] != "aes-128-gcm" {
		t.Fatalf("cipher = %#v", user["security"])
	}
	if xrayStream(t, n)["security"] != "tls" {
		t.Fatalf("TLS from tls field was lost")
	}

	obj = vmessBase()
	obj["net"] = "xhttp"
	delete(obj, "tls")
	obj["security"] = "tls"
	n = parseOne(t, vmessLink(obj))
	if xrayStream(t, n)["security"] != "tls" {
		t.Fatalf("TLS from security field was lost")
	}
}

func TestXrayVMessXHTTPNoneCipherWithTLS(t *testing.T) {
	obj := vmessBase()
	obj["net"] = "xhttp"
	delete(obj, "scy")
	obj["security"] = "none"
	obj["tls"] = "tls"
	n := parseOne(t, vmessLink(obj))
	if n.Engine != model.EngineXray {
		t.Fatalf("engine = %q, want xray", n.Engine)
	}
	user := xrayVLESSUser(t, n)
	if user["security"] != "none" {
		t.Fatalf("cipher = %#v, want none", user["security"])
	}
	if xrayStream(t, n)["security"] != "tls" {
		t.Fatalf("TLS from tls field was lost when security was a cipher")
	}
}

func TestXrayBadFinalMaskFallsBackToPreview(t *testing.T) {
	q := url.Values{}
	q.Set("security", "tls")
	q.Set("type", "ws")
	q.Set("path", "/ws")
	q.Set("fm", "{")
	content := "vless://" + testUUID + "@example.com:443?" + q.Encode()
	if result := Parse(content); result.TotalErrors == 0 {
		t.Fatal("strict parse accepted malformed finalmask")
	}
	result := ParsePreview(content)
	if len(result.Degraded) != 1 || result.TotalErrors != 0 {
		t.Fatalf("preview result = degraded %d errors %d, want 1/0", len(result.Degraded), result.TotalErrors)
	}
	if result.Degraded[0].Engine != "" {
		t.Fatalf("degraded engine = %q, want mihomo default", result.Degraded[0].Engine)
	}
}

func TestXrayPlainPublicVLESSFallsBackToPreview(t *testing.T) {
	q := url.Values{}
	q.Set("security", "none")
	q.Set("encryption", "none")
	q.Set("type", "ws")
	q.Set("path", "/ws")
	q.Set("fm", `{"tcp":[{"type":"fragment","settings":{"packets":"tlshello"}}]}`)
	content := "vless://" + testUUID + "@8.8.8.8:443?" + q.Encode()
	if result := Parse(content); result.TotalErrors == 0 {
		t.Fatal("strict parse accepted plaintext public VLESS")
	}
	result := ParsePreview(content)
	if len(result.Degraded) != 1 || result.TotalErrors != 0 {
		t.Fatalf("preview result = degraded %d errors %d, want 1/0", len(result.Degraded), result.TotalErrors)
	}
}
