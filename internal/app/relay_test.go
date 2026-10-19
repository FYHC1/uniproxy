package app

import (
	"encoding/json"
	"fmt"
	"github.com/uniproxy/uniproxy/internal/importer"
	"github.com/uniproxy/uniproxy/internal/model"
	"strings"
	"testing"
)

const relayFixture = "proxies:\n  - {name: leaf, type: socks5, server: exit.example.com, port: 1080, udp: true, dialer-proxy: relay}\n  - {name: relay, type: socks5, server: relay.example.com, port: 1081, udp: true}\n  - {name: relay-alias, type: socks5, server: relay.example.com, port: 1081, udp: true}\n"

func TestRelayImportForwardReferenceDedupAndBackup(t *testing.T) {
	a, _ := fixture(t, true)
	code, out := request(t, a, "POST", "/api/import", map[string]any{"mode": "file", "content": relayFixture, "groupNames": []string{"relay-group"}})
	if code != 200 {
		t.Fatal(out)
	}
	s := a.store.Snapshot()
	if len(s.Nodes) != 2 {
		t.Fatalf("alias dedup: %d", len(s.Nodes))
	}
	var leaf, relay model.Node
	for _, n := range s.Nodes {
		if n.Name == "leaf" {
			leaf = n
		} else {
			relay = n
		}
	}
	if leaf.DialerID != relay.ID || leaf.DialerProxy != "" {
		t.Fatal("not remapped to stable IDs")
	}
	if _, exists := leaf.Config["dialer-proxy"]; exists {
		t.Fatal("raw reference leaked into stored config")
	}
	code, public := request(t, a, "GET", "/api/bootstrap", nil)
	if code != 200 {
		t.Fatal(public)
	}
	nodes := public["nodes"].([]any)
	for _, v := range nodes {
		n := v.(map[string]any)
		if n["name"] == "leaf" && (n["dialerId"] != relay.ID || n["dialerName"] != relay.Name) {
			t.Fatal("missing public relay metadata")
		}
	}
	// Re-import aliases and renamed relays keeps stable chain identity.
	parsed := importer.Parse(relayFixture)
	oldFP := leaf.Fingerprint
	if len(parsed.Errors) != 0 {
		t.Fatal(parsed.Errors)
	}
	if importer.Fingerprint(parsed.Nodes[0]) != oldFP {
		t.Fatal("persisted chain fingerprint differs")
	}
	_, _ = request(t, a, "PATCH", "/api/nodes/"+relay.ID, map[string]any{"name": "renamed relay"})
	code, out = request(t, a, "POST", "/api/import", map[string]any{"mode": "file", "content": relayFixture})
	if code != 200 || len(a.store.Snapshot().Nodes) != 2 {
		t.Fatal("display rename broke dedup", out)
	}
	backupCode, backup := request(t, a, "GET", "/api/export/backup", nil)
	if backupCode != 200 {
		t.Fatal(backup)
	}
	code, out = request(t, a, "POST", "/api/import/backup", backup)
	if code != 200 {
		t.Fatal(out)
	}
	s = a.store.Snapshot()
	if s.Nodes[nodeIndex(&s, leaf.ID)].Fingerprint != oldFP || s.Nodes[nodeIndex(&s, leaf.ID)].DialerID != relay.ID {
		t.Fatal("backup lost chain identity")
	}
	// YAML export preserves the relay even after a display rename.
	raw, e := json.Marshal(map[string]any{"proxies": exportNativeNodes(s)})
	if e != nil {
		t.Fatal(e)
	}
	exported := importer.Parse(string(raw))
	if len(exported.Errors) != 0 || len(exported.Nodes) != 2 {
		t.Fatal(exported.Errors)
	}
	if importer.Fingerprint(exported.Nodes[0]) != oldFP {
		t.Fatal("YAML export changed chain identity")
	}
	_, _ = request(t, a, "POST", "/api/nodes/batch", map[string]any{"ids": []string{relay.ID}, "action": "disable"})
	_, public = request(t, a, "GET", "/api/bootstrap", nil)
	for _, v := range public["nodes"].([]any) {
		n := v.(map[string]any)
		if n["id"] == leaf.ID && n["dialerError"] == nil {
			t.Fatal("disabled relay not explained")
		}
	}
	_, _ = request(t, a, "POST", "/api/nodes/batch", map[string]any{"ids": []string{relay.ID}, "action": "delete"})
	s = a.store.Snapshot()
	if len(s.Nodes) != 1 || s.Nodes[0].DialerID != relay.ID {
		t.Fatal("deletion removed chain guard")
	}
	if _, e := a.startJob([]string{leaf.ID}, "latency"); e == nil {
		t.Fatal("tested leaf with missing relay")
	}
	_, backup = request(t, a, "GET", "/api/export/backup", nil)
	code, out = request(t, a, "POST", "/api/import/backup", backup)
	if code != 200 {
		t.Fatal("missing relay backup should remain blocked", out)
	}
}
func TestRelayRefreshReconcilesWholeChainIdentity(t *testing.T) {
	s := model.DefaultState()
	s.Subscriptions = []model.Subscription{{ID: "source", Name: "relay source", GroupIDs: []string{}}}
	pr := importer.Parse(relayFixture)
	out := emptyImport()
	if e := addParsed(&s, pr.Nodes, nil, "source", &out); e != nil {
		t.Fatal(e)
	}
	if len(s.Nodes) != 2 {
		t.Fatal("dedup failure")
	}
	next := importer.Parse("proxies:\n  - {name: leaf, type: socks5, server: exit.example.com, port: 1080, udp: true, dialer-proxy: relay}\n  - {name: relay, type: socks5, server: relay.example.com, port: 1082, udp: true}\n")
	if _, e := reconcile(&s, "source", next, true); e != nil {
		t.Fatal(e)
	}
	if len(s.Nodes) != 2 {
		t.Fatal("old graph not pruned")
	}
	for _, n := range s.Nodes {
		if n.DialerID != "" {
			i := nodeIndex(&s, n.DialerID)
			if i < 0 || s.Nodes[i].Port != 1082 {
				t.Fatal("leaf linked to stale relay")
			}
		}
	}
}
func TestRelayCannotResolveAcrossSeparateImports(t *testing.T) {
	a, _ := fixture(t, true)
	code, _ := request(t, a, "POST", "/api/import", map[string]any{"mode": "file", "content": "proxies: [{name: leaf, type: http, server: example.com, port: 80, dialer-proxy: secret-other-import}]"})
	if code != 400 || len(a.store.Snapshot().Nodes) != 0 {
		t.Fatal("external reference accepted")
	}
}

func TestImportPreviewCountsAllErrorsAndRetainsLaterValidNodes(t *testing.T) {
	a, _ := fixture(t, true)
	var input strings.Builder
	input.WriteString("proxies:\n")
	for i := 0; i < 150; i++ {
		fmt.Fprintf(&input, "  - {name: bad%d, type: http, server: example.com, port: 80, dialer-proxy: missing}\n", i)
	}
	input.WriteString("  - {name: last-valid, type: http, server: example.com, port: 8081}\n")
	code, out := request(t, a, "POST", "/api/import", map[string]any{"mode": "file", "content": input.String(), "preview": true})
	if code != 200 {
		t.Fatal(out)
	}
	if out["totalErrors"] != float64(150) || out["errorsTruncated"] != true || len(out["errors"].([]any)) != 128 || len(out["nodes"].([]any)) != 1 {
		t.Fatal("incorrect full-scan diagnostic summary")
	}
	if len(a.store.Snapshot().Nodes) != 0 {
		t.Fatal("preview committed state")
	}
}

func TestRelayExportReservedNamesAndMissingTargetsCannotBeCaptured(t *testing.T) {
	for _, name := range []string{"direct", "PASS", "REJECT-DROP", " Direct "} {
		t.Run(name, func(t *testing.T) {
			s := model.DefaultState()
			pr := importer.Parse(relayFixture)
			out := emptyImport()
			if e := addParsed(&s, pr.Nodes, nil, "", &out); e != nil {
				t.Fatal(e)
			}
			for i := range s.Nodes {
				if s.Nodes[i].DialerID == "" {
					s.Nodes[i].Name = name
				}
			}
			raw, _ := json.Marshal(map[string]any{"proxies": exportNativeNodes(s)})
			got := importer.Parse(string(raw))
			if len(got.Errors) != 0 || len(got.Nodes) != 2 {
				t.Fatal("reserved alias broke export")
			}
		})
	}
	s := model.DefaultState()
	pr := importer.Parse(relayFixture)
	out := emptyImport()
	if e := addParsed(&s, pr.Nodes, nil, "", &out); e != nil {
		t.Fatal(e)
	}
	var relayID string
	for _, n := range s.Nodes {
		if n.DialerID != "" {
			relayID = n.DialerID
		}
	}
	pruneNodes(&s, []string{relayID})
	other := importer.Parse("http://other.example.com:80")
	other.Nodes[0].Name = "missing-" + relayID
	if e := addParsed(&s, other.Nodes, nil, "", &out); e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(map[string]any{"proxies": exportNativeNodes(s)})
	got := importer.Parse(string(raw))
	if got.TotalErrors != 1 || len(got.Nodes) != 1 || got.Nodes[0].Protocol != "http" {
		t.Fatal("missing dependency captured by another name")
	}
}

func TestDeletedRelayBackupRestoreThenReimportRepairsExistingLeaf(t *testing.T) {
	a, _ := fixture(t, true)
	code, out := request(t, a, "POST", "/api/import", map[string]any{"mode": "file", "content": relayFixture})
	if code != 200 {
		t.Fatal(out)
	}
	s := a.store.Snapshot()
	var leaf model.Node
	for _, n := range s.Nodes {
		if n.DialerID != "" {
			leaf = n
		}
	}
	code, out = request(t, a, "POST", "/api/listeners", map[string]any{"name": "keep leaf", "kind": "node", "targetId": leaf.ID, "protocol": "http", "host": "127.0.0.1", "port": 21289, "enabled": true})
	if code != 200 {
		t.Fatal(out)
	}
	_, _ = request(t, a, "PATCH", "/api/nodes/"+leaf.ID, map[string]any{"name": "renamed before delete"})
	if a.store.Snapshot().Nodes[0].DialerIdentity == "" {
		t.Fatal("PATCH cleared persistent relay identity")
	}
	_, _ = request(t, a, "POST", "/api/nodes/batch", map[string]any{"ids": []string{leaf.DialerID}, "action": "delete"})
	_, backup := request(t, a, "GET", "/api/export/backup", nil)
	code, out = request(t, a, "POST", "/api/import/backup", backup)
	if code != 200 {
		t.Fatal(out)
	}
	restored := a.store.Snapshot()
	if restored.Nodes[0].Fingerprint != leaf.Fingerprint {
		t.Fatal("backup changed dangling leaf identity")
	}
	code, out = request(t, a, "POST", "/api/import", map[string]any{"mode": "file", "content": relayFixture})
	if code != 200 {
		t.Fatal(out)
	}
	current := a.store.Snapshot()
	if len(current.Nodes) != 2 || nodeIndex(&current, leaf.ID) < 0 || current.Listeners[0].TargetID != leaf.ID || len(model.RelayErrors(current.Nodes)) != 0 {
		t.Fatal("reimport did not restore original leaf and listener")
	}
}

func TestRelayPublicResponsesOmitInternalIdentity(t *testing.T) {
	a, _ := fixture(t, true)
	code, out := request(t, a, "POST", "/api/import", map[string]any{"mode": "file", "content": relayFixture})
	if code != 200 {
		t.Fatal(out)
	}
	s := a.store.Snapshot()
	var id string
	for _, n := range s.Nodes {
		if n.DialerID != "" {
			id = n.ID
		}
	}
	_, public := request(t, a, "GET", "/api/bootstrap", nil)
	for _, v := range public["nodes"].([]any) {
		n := v.(map[string]any)
		if n["dialerIdentity"] != nil || n["fingerprint"] != nil || n["config"] != nil {
			t.Fatal("internal identity/config leaked")
		}
	}
	_, patch := request(t, a, "PATCH", "/api/nodes/"+id, map[string]any{"enabled": true})
	data := patch["data"].(map[string]any)
	if data["dialerIdentity"] != nil {
		t.Fatal("patch leaked relay identity")
	}
	s = a.store.Snapshot()
	if s.Nodes[nodeIndex(&s, id)].DialerIdentity == "" {
		t.Fatal("public projection erased stored identity")
	}
}

func TestRelayBackupDoesNotTrustSuppliedNodeFingerprint(t *testing.T) {
	a, _ := fixture(t, true)
	_, out := request(t, a, "POST", "/api/import", map[string]any{"mode": "file", "content": relayFixture})
	if out["error"] != nil {
		t.Fatal(out)
	}
	state := a.store.Snapshot()
	for i := range state.Nodes {
		state.Nodes[i].Fingerprint = strings.Repeat("a", 64)
		if state.Nodes[i].DialerID != "" {
			state.Nodes[i].DialerIdentity = strings.Repeat("b", 64)
		}
	}
	code, res := request(t, a, "POST", "/api/import/backup", map[string]any{"state": state})
	if code != 200 {
		t.Fatal(res)
	}
	saved := a.store.Snapshot()
	for _, n := range saved.Nodes {
		if n.Fingerprint == strings.Repeat("a", 64) {
			t.Fatal("raw backup fingerprint trusted")
		}
		if n.DialerID != "" {
			i := nodeIndex(&saved, n.DialerID)
			if n.DialerIdentity != saved.Nodes[i].Fingerprint {
				t.Fatal("existing relay identity not recomputed")
			}
		}
	}
}
