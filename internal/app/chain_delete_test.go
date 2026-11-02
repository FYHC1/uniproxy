package app

import (
	"errors"
	"testing"
)

func TestChainUpdateDeleteAndTest(t *testing.T) {
	a, _ := fixture(t, true)
	ids := seedChainNodes(t, a, 2)
	code, created := request(t, a, "POST", "/api/chains", map[string]any{"name": "editable", "nodeIds": ids, "enabled": true})
	if code != 200 {
		t.Fatalf("create %d %#v", code, created)
	}
	id := created["data"].(map[string]any)["id"].(string)
	code, out := request(t, a, "POST", "/api/chains/"+id+"/test", nil)
	if code != 200 || out["chainId"] != id || out["ok"] != true {
		t.Fatalf("test %d %#v", code, out)
	}
	code, out = request(t, a, "PUT", "/api/chains/"+id, map[string]any{"name": "renamed", "nodeIds": []string{ids[1], ids[0]}, "enabled": false})
	if code != 200 {
		t.Fatalf("update %d %#v", code, out)
	}
	if c := a.store.Snapshot().Chains[0]; c.Name != "renamed" || c.Enabled || c.NodeIDs[0] != ids[1] {
		t.Fatalf("stored chain %#v", c)
	}
	code, _ = request(t, a, "DELETE", "/api/chains/"+id, nil)
	if code != 200 || len(a.store.Snapshot().Chains) != 0 {
		t.Fatalf("delete %d", code)
	}
}

func TestChainTestFailureIsReturnedAsCheckResult(t *testing.T) {
	a, core := fixture(t, true)
	ids := seedChainNodes(t, a, 2)
	code, created := request(t, a, "POST", "/api/chains", map[string]any{"name": "failed test", "nodeIds": ids, "enabled": true})
	if code != 200 {
		t.Fatalf("create %d %#v", code, created)
	}
	id := created["data"].(map[string]any)["id"].(string)
	core.chainCheckErr = errors.New("chain is disabled or absent from the live configuration")
	code, out := request(t, a, "POST", "/api/chains/"+id+"/test", nil)
	if code != 200 || out["chainId"] != id || out["ok"] != false {
		t.Fatalf("failed test %d %#v", code, out)
	}
	if out["error"] != core.chainCheckErr.Error() {
		t.Fatalf("error %#v", out["error"])
	}
}

func TestDeletingChainNodeDisablesWholeRouteAndUDPEntry(t *testing.T) {
	a, _ := fixture(t, true)
	ids := seedChainNodes(t, a, 2)
	s := a.store.Snapshot()
	for i := range s.Nodes {
		s.Nodes[i].UDP = true
	}
	if err := a.store.Save(s); err != nil {
		t.Fatal(err)
	}
	body := map[string]any{
		"chain":    map[string]any{"name": "udp delete guarded", "nodeIds": ids, "enabled": true},
		"listener": map[string]any{"name": "udp entry", "protocol": "mixed", "host": "127.0.0.1", "port": 28083, "udp": true, "enabled": true},
	}
	code, out := request(t, a, "POST", "/api/chains/with-listener", body)
	if code != 200 {
		t.Fatalf("create %d %#v", code, out)
	}
	code, out = request(t, a, "POST", "/api/nodes/batch", map[string]any{"ids": []string{ids[0]}, "action": "delete"})
	if code != 200 {
		t.Fatalf("delete node with UDP chain listener %d %#v", code, out)
	}
	s = a.store.Snapshot()
	if len(s.Chains) != 1 || s.Chains[0].Enabled {
		t.Fatalf("chain not disabled: %#v", s.Chains)
	}
	if len(s.Listeners) != 1 || s.Listeners[0].Enabled {
		t.Fatalf("UDP entry not disabled: %#v", s.Listeners)
	}
	if len(s.Chains[0].NodeIDs) != 2 {
		t.Fatal("chain was silently shortened")
	}
}

func TestDeletingChainNodeDisablesWholeRouteAndEntry(t *testing.T) {
	a, _ := fixture(t, true)
	ids := seedChainNodes(t, a, 2)
	body := map[string]any{
		"chain":    map[string]any{"name": "delete guarded", "nodeIds": ids, "enabled": true},
		"listener": map[string]any{"name": "entry", "protocol": "mixed", "host": "127.0.0.1", "port": 28082, "enabled": true},
	}
	code, _ := request(t, a, "POST", "/api/chains/with-listener", body)
	if code != 200 {
		t.Fatal(code)
	}
	code, _ = request(t, a, "POST", "/api/nodes/batch", map[string]any{"ids": []string{ids[0]}, "action": "delete"})
	if code != 200 {
		t.Fatalf("delete node %d", code)
	}
	s := a.store.Snapshot()
	if len(s.Chains) != 1 || s.Chains[0].Enabled {
		t.Fatalf("chain not disabled: %#v", s.Chains)
	}
	if len(s.Listeners) != 1 || s.Listeners[0].Enabled {
		t.Fatalf("entry not disabled: %#v", s.Listeners)
	}
	if len(s.Chains[0].NodeIDs) != 2 {
		t.Fatal("chain was silently shortened")
	}
}
