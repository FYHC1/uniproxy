package app

import (
	"encoding/json"
	"fmt"
	"github.com/uniproxy/uniproxy/internal/model"
	"github.com/uniproxy/uniproxy/internal/store"
	"strings"
	"testing"
)

func BenchmarkValidate500Nodes(b *testing.B) {
	s := model.DefaultState()
	for i := 0; i < 500; i++ {
		s.Nodes = append(s.Nodes, model.Node{ID: fmt.Sprintf("node-%d", i), Enabled: true, Manual: true, ParsedNode: model.ParsedNode{Name: fmt.Sprintf("Node %d", i), Protocol: "socks5", Server: "127.0.0.1", Port: 1080, Config: map[string]any{"type": "socks5", "server": "127.0.0.1", "port": 1080}}})
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if e := ValidateState(s, 9090); e != nil {
			b.Fatal(e)
		}
	}
}
func BenchmarkSnapshot500Nodes(b *testing.B) {
	s := model.DefaultState()
	for i := 0; i < 500; i++ {
		s.Nodes = append(s.Nodes, model.Node{ID: fmt.Sprintf("node-%d", i), Enabled: true, Manual: true, ParsedNode: model.ParsedNode{Name: fmt.Sprintf("Node %d", i), Protocol: "socks5", Server: "127.0.0.1", Port: 1080, Config: map[string]any{"type": "socks5", "server": "127.0.0.1", "port": 1080, "password": "redacted"}}})
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = store.Clone(s)
	}
}
func TestPublicUDPAndUsernameHashRejected(t *testing.T) {
	a, _ := fixture(t, true)
	code, out := request(t, a, "POST", "/api/import", map[string]any{"mode": "text", "content": "socks5://127.0.0.1:1080?udp=true"})
	if code != 200 {
		t.Fatal(out)
	}
	id := a.store.Snapshot().Nodes[0].ID
	body := map[string]any{"name": "unsafe UDP", "kind": "node", "targetId": id, "protocol": "socks", "host": "0.0.0.0", "port": 18081, "udp": true, "enabled": true, "users": []map[string]string{{"username": "x", "password": "y"}}}
	code, _ = request(t, a, "POST", "/api/listeners", body)
	if code != 400 {
		t.Fatal("public unauthenticated UDP should not be allowed even with TCP auth")
	}
	body["host"] = "127.0.0.1"
	code, out = request(t, a, "POST", "/api/listeners", body)
	if code != 200 {
		t.Fatal(out)
	}
}

func TestAcceptedLargeBackupRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("large backup regression")
	}
	a, _ := fixture(t, true)
	s := model.DefaultState()
	password := strings.Repeat("p", 10000)
	for i := 0; i < 1300; i++ {
		s.Nodes = append(s.Nodes, model.Node{ID: fmt.Sprintf("large-%d", i), Enabled: true, Manual: true, Status: "unknown", ParsedNode: model.ParsedNode{Name: fmt.Sprintf("Node %d", i), Protocol: "http", Server: "example.com", Port: 8080, Config: map[string]any{"type": "http", "server": "example.com", "port": 8080, "username": "u", "password": password}}})
	}
	if e := ValidateState(s, 9090); e != nil {
		t.Fatal(e)
	}
	if e := a.store.Save(s); e != nil {
		t.Fatal(e)
	}
	code, backup := request(t, a, "GET", "/api/export/backup", nil)
	if code != 200 {
		t.Fatal(code)
	}
	encoded, _ := json.Marshal(backup)
	if len(encoded) <= 12*1024*1024 {
		t.Fatal("fixture below original restore limit")
	}
	if e := a.store.Save(model.DefaultState()); e != nil {
		t.Fatal(e)
	}
	code, result := request(t, a, "POST", "/api/import/backup", backup)
	if code != 200 {
		t.Fatal(result)
	}
	state := a.store.Snapshot()
	if len(state.Nodes) != 1300 || state.Nodes[0].Config["password"] != password {
		t.Fatal("large backup restore lost data")
	}
}
