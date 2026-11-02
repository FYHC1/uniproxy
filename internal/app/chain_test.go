package app

import (
	"fmt"
	"testing"
)

func seedChainNodes(t *testing.T, a *App, count int) []string {
	t.Helper()
	lines := ""
	for i := 0; i < count; i++ {
		if i > 0 {
			lines += "\n"
		}
		lines += fmt.Sprintf("socks5://127.0.0.1:%d#chain-node-%d", 31000+i, i+1)
	}
	code, out := request(t, a, "POST", "/api/import", map[string]any{"mode": "text", "content": lines, "groupIds": []string{}})
	if code != 200 {
		t.Fatalf("seed import %d %#v", code, out)
	}
	s := a.store.Snapshot()
	ids := make([]string, 0, len(s.Nodes))
	for _, n := range s.Nodes {
		ids = append(ids, n.ID)
	}
	if len(ids) != count {
		t.Fatalf("seed nodes = %d", len(ids))
	}
	return ids
}
