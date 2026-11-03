package app

import (
	"net/http"
	"testing"
)

func TestPreviewSeparatesDegradedNodes(t *testing.T) {
	a, _ := fixture(t, true)
	content := "socks5://127.0.0.1:1080\nvless://12345678-1234-4234-8234-123456789abc@example.com:443?security=tls&fp=unsafe#degraded"
	code, out := request(t, a, "POST", "/api/import", map[string]any{"mode": "text", "content": content, "preview": true})
	if code != 200 {
		t.Fatalf("preview status %d: %#v", code, out)
	}
	nodes, _ := out["nodes"].([]any)
	degraded, _ := out["degraded"].([]any)
	if len(nodes) != 1 || len(degraded) != 1 {
		t.Fatalf("nodes=%d degraded=%d want 1/1: %#v", len(nodes), len(degraded), out)
	}
	if len(a.store.Snapshot().Nodes) != 0 {
		t.Fatal("preview persisted nodes")
	}
}

func TestPreviewSubscriptionKeepsDegradedRowsDespiteErrors(t *testing.T) {
	a, _ := fixture(t, true)
	const source = "https://subscription.example.invalid/preview"
	body := "socks5://127.0.0.1:1080\nvless://12345678-1234-4234-8234-123456789abc@example.com:443?security=tls&fp=unsafe#degraded\nvless://not-a-uuid@example.com:443?security=tls#bad"
	a.client = &http.Client{Transport: diagnosticRoundTripper(func(r *http.Request) (*http.Response, error) {
		return diagnosticResponse(r, body), nil
	})}
	code, out := request(t, a, "POST", "/api/import", map[string]any{"mode": "subscriptions", "content": source, "preview": true})
	if code != 200 {
		t.Fatalf("preview status %d: %#v", code, out)
	}
	nodes, _ := out["nodes"].([]any)
	degraded, _ := out["degraded"].([]any)
	errors, _ := out["errors"].([]any)
	if len(nodes) != 1 || len(degraded) != 1 || len(errors) != 1 {
		t.Fatalf("nodes=%d degraded=%d errors=%d want 1/1/1: %#v", len(nodes), len(degraded), len(errors), out)
	}
	if len(a.store.Snapshot().Nodes) != 0 {
		t.Fatal("preview persisted nodes")
	}
}
