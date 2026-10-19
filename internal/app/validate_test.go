package app

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/uniproxy/uniproxy/internal/model"
)

// A node label may legitimately reach the display ceiling, so the derived
// listener name must reserve room for its suffix instead of failing the whole
// batch with a misleading "invalid listener name" error.
func TestDerivedNameStaysWithinLimit(t *testing.T) {
	cases := []struct{ base, suffix string }{
		{"short", " · 独享"},
		{strings.Repeat("长", maxNameRunes), " · 独享"},
		{strings.Repeat("长", maxNameRunes+50), " · 独享"},
		{"  ", " · 独享"},
		{"name", ""},
	}
	for _, c := range cases {
		got := derivedName(c.base, c.suffix)
		if !nameOK(got) {
			t.Fatalf("derived name invalid (%d runes) for base %d runes", utf8.RuneCountInString(got), utf8.RuneCountInString(c.base))
		}
		if c.suffix != "" && !strings.HasSuffix(got, strings.TrimSpace(c.suffix)) {
			t.Fatalf("suffix lost: %q", got)
		}
	}
	if got := derivedName("abc", ""); got != "abc" {
		t.Fatalf("empty suffix changed the base: %q", got)
	}
}

func TestBatchListenerAcceptsNodeAtNameCeiling(t *testing.T) {
	a, _ := fixture(t, true)
	long := strings.Repeat("长", 200)
	code, out := request(t, a, "POST", "/api/import", map[string]any{"mode": "text", "content": "socks5://u:p@127.0.0.1:1080#" + long, "groupIds": []string{}})
	if code != 200 {
		t.Fatalf("import rejected a long but valid label: %d %#v", code, out)
	}
	node := a.store.Snapshot().Nodes[0]
	if utf8.RuneCountInString(node.Name) != maxNameRunes {
		t.Fatalf("stored label should be shortened to the ceiling, got %d runes", utf8.RuneCountInString(node.Name))
	}
	base := poolTestPortRange(t, 1)
	poolTestSetRange(t, a, base, base)
	code, out = request(t, a, "POST", "/api/listeners/batch", map[string]any{"nodeIds": []string{node.ID}, "protocol": "mixed", "host": "127.0.0.1", "udp": false, "users": []any{}})
	if code != 200 {
		t.Fatalf("batch listener rejected a node at the name ceiling: %d %#v", code, out)
	}
	listeners := a.store.Snapshot().Listeners
	if len(listeners) != 1 || !nameOK(listeners[0].Name) || !strings.HasSuffix(listeners[0].Name, "独享") {
		t.Fatalf("derived listener name invalid: %q", listeners[0].Name)
	}
}

// A rejected save must name the field that is actually wrong; the old combined
// message made a long listener name look like a port/host problem.
func TestNameValidationMessagesIdentifyTheField(t *testing.T) {
	a, _ := fixture(t, true)
	nodeID := importOne(t, a)
	tooLong := strings.Repeat("n", maxNameRunes+1)

	code, out := request(t, a, "PATCH", "/api/nodes/"+nodeID, map[string]any{"name": tooLong})
	if code != 400 || !strings.Contains(out["error"].(string), "节点名称") {
		t.Fatalf("node name rejection unclear: %d %#v", code, out)
	}
	code, out = request(t, a, "POST", "/api/listeners", map[string]any{"name": tooLong, "kind": "node", "targetId": nodeID, "protocol": "mixed", "host": "127.0.0.1", "port": 20001, "enabled": true, "users": []any{}})
	if code != 400 || !strings.Contains(out["error"].(string), "监听名称") {
		t.Fatalf("listener name rejection unclear: %d %#v", code, out)
	}
	code, out = request(t, a, "POST", "/api/listeners", map[string]any{"name": "ok", "kind": "node", "targetId": nodeID, "protocol": "mixed", "host": "listener.example", "port": 20001, "enabled": true, "users": []any{}})
	if code != 400 || !strings.Contains(out["error"].(string), "监听地址") {
		t.Fatalf("listener host rejection unclear: %d %#v", code, out)
	}
	code, out = request(t, a, "POST", "/api/groups", map[string]any{"name": strings.Repeat("g", maxNameRunes+1)})
	if code != 400 || !strings.Contains(out["error"].(string), "分组名称") {
		t.Fatalf("group name rejection unclear: %d %#v", code, out)
	}
}

// Subscription metadata is validated before any download, so a long name can no
// longer waste a fetch or fail after the network work already succeeded.
func TestSubscriptionMetadataRejectedBeforeDownload(t *testing.T) {
	a, _ := fixture(t, true)
	before := a.store.Snapshot()
	for _, body := range []map[string]any{
		{"mode": "subscriptions", "content": "https://127.0.0.1:1/never-fetched", "groupIds": []string{}, "name": strings.Repeat("n", maxNameRunes+1), "preview": true},
		{"mode": "subscriptions", "content": "https://127.0.0.1:1/never-fetched", "groupIds": []string{}, "groupNames": []string{strings.Repeat("g", maxNameRunes+1)}, "preview": true},
		{"mode": "subscriptions", "content": "https://127.0.0.1:1/never-fetched", "groupIds": []string{}, "intervalMinutes": 1, "preview": true},
	} {
		code, out := request(t, a, "POST", "/api/import", body)
		if code != 400 {
			t.Fatalf("invalid subscription metadata accepted: %d %#v", code, out)
		}
	}
	if current := a.store.Snapshot(); current.Revision != before.Revision || len(current.Subscriptions) != 0 {
		t.Fatal("rejected metadata changed stored state")
	}
}

// truncateRunes must never split a multi-byte character.
func TestTruncateRunesKeepsValidText(t *testing.T) {
	value := strings.Repeat("长", maxNameRunes+10)
	got := truncateRunes(value, maxNameRunes)
	if utf8.RuneCountInString(got) != maxNameRunes || !utf8.ValidString(got) || strings.ContainsRune(got, '\uFFFD') {
		t.Fatalf("rune-safe truncation failed: %d runes", utf8.RuneCountInString(got))
	}
	if truncateRunes("short", maxNameRunes) != "short" {
		t.Fatal("short value must be unchanged")
	}
	_ = model.Now
}
