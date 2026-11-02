package app

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestChainWithListenerAtomicCreateAndRedaction(t *testing.T) {
	a, _ := fixture(t, true)
	ids := seedChainNodes(t, a, 2)
	body := map[string]any{
		"chain":    map[string]any{"name": "two hop", "nodeIds": ids, "enabled": true},
		"listener": map[string]any{"name": "chain entry", "protocol": "mixed", "host": "127.0.0.1", "port": 28081, "udp": false, "enabled": true, "users": []any{}},
	}
	code, out := request(t, a, "POST", "/api/chains/with-listener", body)
	if code != 200 {
		t.Fatalf("create %d %#v", code, out)
	}
	raw, _ := json.Marshal(out)
	if bytes.Contains(raw, []byte("password")) {
		t.Fatalf("response leaked credentials: %s", raw)
	}
	s := a.store.Snapshot()
	if len(s.Chains) != 1 || len(s.Listeners) != 1 {
		t.Fatalf("state chains=%d listeners=%d", len(s.Chains), len(s.Listeners))
	}
	if s.Listeners[0].Kind != "chain" || s.Listeners[0].TargetID != s.Chains[0].ID {
		t.Fatalf("listener target = %#v", s.Listeners[0])
	}
	if s.Chains[0].NodeIDs[0] != ids[0] || s.Chains[0].NodeIDs[1] != ids[1] {
		t.Fatal("chain order changed")
	}
	code, out = request(t, a, "GET", "/api/bootstrap", nil)
	if code != 200 {
		t.Fatal(code)
	}
	var boot struct {
		Chains []map[string]any `json:"chains"`
	}
	b, _ := json.Marshal(out)
	_ = json.Unmarshal(b, &boot)
	if len(boot.Chains) != 1 || boot.Chains[0]["name"] != "two hop" {
		t.Fatalf("bootstrap chains: %#v", boot.Chains)
	}
}

func TestChainListenerDefaultsAndPortAllocation(t *testing.T) {
	a, _ := fixture(t, true)
	ids := seedChainNodes(t, a, 2)
	body := map[string]any{
		"chain":    map[string]any{"name": "default entry", "nodeIds": ids, "enabled": true},
		"listener": map[string]any{"port": 0},
	}
	code, out := request(t, a, "POST", "/api/chains/with-listener", body)
	if code != 200 {
		t.Fatalf("create %d %#v", code, out)
	}
	s := a.store.Snapshot()
	if len(s.Chains) != 1 || len(s.Listeners) != 1 {
		t.Fatalf("state chains=%d listeners=%d", len(s.Chains), len(s.Listeners))
	}
	l := s.Listeners[0]
	if l.Name != "default entry" || l.Protocol != "mixed" || l.Host != "127.0.0.1" || !l.Enabled {
		t.Fatalf("listener defaults = %#v", l)
	}
	if l.Port < s.Settings.PortRangeStart || l.Port > s.Settings.PortRangeEnd {
		t.Fatalf("allocated port %d outside %d-%d", l.Port, s.Settings.PortRangeStart, s.Settings.PortRangeEnd)
	}
	if l.Kind != "chain" || l.TargetID != s.Chains[0].ID || l.UDP {
		t.Fatalf("listener target = %#v", l)
	}
}

func TestChainListenerRejectsInvalidOrDuplicateEntryAtomically(t *testing.T) {
	a, _ := fixture(t, true)
	ids := seedChainNodes(t, a, 2)
	code, _ := request(t, a, "POST", "/api/chains/with-listener", map[string]any{
		"chain":    map[string]any{"name": "first entry", "nodeIds": ids, "enabled": true},
		"listener": map[string]any{"name": "first", "protocol": "mixed", "host": "127.0.0.1", "port": 28084},
	})
	if code != 200 {
		t.Fatalf("seed listener %d", code)
	}
	before := a.store.Snapshot()
	for name, listener := range map[string]map[string]any{
		"duplicate-port": {"port": 28084},
		"domain-host":    {"port": 0, "host": "localhost"},
		"bad-protocol":   {"port": 0, "protocol": "https"},
		"udp-http":       {"port": 0, "protocol": "http", "udp": true},
		"public-no-auth": {"port": 0, "host": "0.0.0.0"},
		"port-too-low":   {"port": 80},
	} {
		t.Run(name, func(t *testing.T) {
			code, _ := request(t, a, "POST", "/api/chains/with-listener", map[string]any{
				"chain":    map[string]any{"name": "invalid " + name, "nodeIds": ids, "enabled": true},
				"listener": listener,
			})
			if code != 400 {
				t.Fatalf("status %d", code)
			}
		})
	}
	after := a.store.Snapshot()
	if after.Revision != before.Revision || len(after.Chains) != 1 || len(after.Listeners) != 1 {
		t.Fatalf("invalid listener changed state revision=%d chains=%d listeners=%d", after.Revision, len(after.Chains), len(after.Listeners))
	}
}

func TestChainValidationRejectsBadGraphsAtomically(t *testing.T) {
	a, _ := fixture(t, true)
	ids := seedChainNodes(t, a, 2)
	before := a.store.Snapshot()
	for name, chain := range map[string]map[string]any{
		"one-hop":   {"name": "one", "nodeIds": ids[:1], "enabled": true},
		"duplicate": {"name": "dupe", "nodeIds": []string{ids[0], ids[0]}, "enabled": true},
		"missing":   {"name": "ghost", "nodeIds": []string{ids[0], "does-not-exist"}, "enabled": true},
		"too-many":  {"name": "long", "nodeIds": []string{ids[0], ids[1], ids[0], ids[1], ids[0], ids[1], ids[0], ids[1], ids[0], ids[1], ids[0], ids[1], ids[0], ids[1], ids[0], ids[1], ids[0]}, "enabled": true},
	} {
		t.Run(name, func(t *testing.T) {
			code, _ := request(t, a, "POST", "/api/chains/with-listener", map[string]any{"chain": chain})
			if code != 400 {
				t.Fatalf("status %d", code)
			}
		})
	}
	if a.store.Snapshot().Revision != before.Revision || len(a.store.Snapshot().Chains) != 0 {
		t.Fatal("invalid chain mutation committed")
	}
}
