package importer

import (
	"bytes"
	"encoding/base64"
	"net/url"
	"strings"
	"testing"
)

func TestParsePreviewSeparatesStrictAndDegradedNodes(t *testing.T) {
	content := strings.Join([]string{
		"vless://" + testUUID + "@example.com:443?security=false&type=tcp&encryption=none#plain",
		"vless://" + testUUID + "@example.net:443?security=tls&fp=unsafe&sni=example.net#unsafe",
	}, "\n")
	strict := Parse(content)
	if len(strict.Nodes) != 1 || strict.TotalErrors != 1 {
		t.Fatalf("strict result = nodes %d errors %d, want 1/1; nodes=%#v errors=%#v", len(strict.Nodes), strict.TotalErrors, strict.Nodes, strict.Errors)
	}
	preview := ParsePreview(content)
	if len(preview.Nodes) != 1 || len(preview.Degraded) != 1 || preview.TotalErrors != 0 {
		t.Fatalf("preview result = strict %d degraded %d errors %d, want 1/1/0", len(preview.Nodes), len(preview.Degraded), preview.TotalErrors)
	}
	if !preview.Degraded[0].Degraded || len(preview.Degraded[0].DegradedReasons) == 0 {
		t.Fatalf("degraded metadata missing: %#v", preview.Degraded[0])
	}
}

func TestParsePreviewNormalizesPlainTLSAndWebSocketPath(t *testing.T) {
	content := "vless://" + testUUID + "@example.com:443?security=none&sni=example.com&fp=chrome&type=ws&path=ws#plain"
	if result := Parse(content); result.TotalErrors == 0 {
		t.Fatal("strict parse unexpectedly accepted TLS placeholders on a plaintext node")
	}
	result := ParsePreview(content)
	if len(result.Degraded) != 1 || len(result.Nodes) != 0 {
		t.Fatalf("preview result = nodes %d degraded %d, want 0/1", len(result.Nodes), len(result.Degraded))
	}
	node := result.Degraded[0]
	if node.Config["network"] != "ws" {
		t.Fatalf("network = %#v, want ws", node.Config["network"])
	}
	ws, ok := node.Config["ws-opts"].(map[string]any)
	if !ok || ws["path"] != "/ws" {
		t.Fatalf("ws-opts = %#v, want path /ws", node.Config["ws-opts"])
	}
}

func TestVLESSEncryptionAccepts1184ByteKey(t *testing.T) {
	key := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0x42}, 1184))
	value := "mlkem768x25519plus.native.0rtt." + key
	q := url.Values{}
	q.Set("security", "tls")
	q.Set("encryption", value)
	n := parseOne(t, "vless://"+testUUID+"@example.com:443?"+q.Encode())
	requireConfig(t, n.Config, "encryption", value)
}

func TestParsePreviewDropsRealityCustomALPN(t *testing.T) {
	key := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0x02}, 32))
	q := url.Values{}
	q.Set("security", "reality")
	q.Set("pbk", key)
	q.Set("sid", "0123456789abcdef")
	q.Set("alpn", "h2,http/1.1")
	q.Set("type", "tcp")
	content := "vless://" + testUUID + "@example.com:443?" + q.Encode()
	if result := Parse(content); result.TotalErrors == 0 {
		t.Fatal("strict parse unexpectedly accepted custom Reality ALPN")
	}
	result := ParsePreview(content)
	if len(result.Degraded) != 1 || result.TotalErrors != 0 {
		t.Fatalf("preview result = degraded %d errors %d, want 1/0", len(result.Degraded), result.TotalErrors)
	}
}

func TestParsePreviewDropsXrayOnlyRealityFields(t *testing.T) {
	key := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0x01}, 32))
	q := url.Values{}
	q.Set("security", "reality")
	q.Set("pbk", key)
	q.Set("sid", "0123456789abcdef")
	q.Set("pqv", "verify")
	q.Set("fm", "mask")
	q.Set("type", "tcp")
	content := "vless://" + testUUID + "@example.com:443?" + q.Encode()
	if result := Parse(content); result.TotalErrors == 0 {
		t.Fatal("strict parse unexpectedly accepted Xray-only fields")
	}
	result := ParsePreview(content)
	if len(result.Degraded) != 1 || result.TotalErrors != 0 {
		t.Fatalf("preview result = degraded %d errors %d, want 1/0", len(result.Degraded), result.TotalErrors)
	}
}

func TestParsePreviewKeepsCleanNodeStrict(t *testing.T) {
	content := "vless://" + testUUID + "@example.com:443?security=tls&sni=example.com&fp=ios&type=ws&path=%2Fws#clean"
	result := ParsePreview(content)
	if len(result.Nodes) != 1 || len(result.Degraded) != 0 || result.TotalErrors != 0 {
		t.Fatalf("preview result = nodes %d degraded %d errors %d, want 1/0/0", len(result.Nodes), len(result.Degraded), result.TotalErrors)
	}
	if result.Nodes[0].Config["client-fingerprint"] != "iOS" {
		t.Fatalf("client fingerprint = %#v, want iOS", result.Nodes[0].Config["client-fingerprint"])
	}
}
