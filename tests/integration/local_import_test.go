package integration

import (
	"context"
	"encoding/json"
	"github.com/uniproxy/uniproxy/internal/core"
	"github.com/uniproxy/uniproxy/internal/importer"
	"github.com/uniproxy/uniproxy/internal/model"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Reads an explicitly supplied file only; never copies its contents into fixtures,
// logs credentials, starts its proxy nodes, or contacts its servers.
func TestPrivateClashRelayFileOffline(t *testing.T) {
	source := os.Getenv("UNIPROXY_TEST_IMPORT_FILE")
	if source == "" {
		t.Skip("set UNIPROXY_TEST_IMPORT_FILE for private offline import validation")
	}
	raw, e := os.ReadFile(source)
	if e != nil {
		t.Fatal("cannot read provided import file")
	}
	var doc struct{ Proxies []map[string]any }
	if e = yaml.Unmarshal(raw, &doc); e != nil {
		t.Fatal("provided file is not valid YAML")
	}
	result := importer.Parse(string(raw))
	if result.TotalErrors != 0 || len(result.Errors) != 0 {
		t.Fatalf("parse failed: nodes=%d errors=%d detailCount=%d", len(result.Nodes), result.TotalErrors, len(result.Errors))
	}
	if len(result.Nodes) != len(doc.Proxies) {
		t.Fatalf("expected all %d entries, got %d", len(doc.Proxies), len(result.Nodes))
	}
	state := model.DefaultState()
	ids := map[string]string{}
	for i, n := range result.Nodes {
		fp := importer.Fingerprint(n)
		if fp == "" {
			t.Fatal("empty fingerprint")
		}
		if ids[fp] == "" {
			ids[fp] = "n" + time.Unix(int64(i), 0).UTC().Format("150405")
		}
	}
	seen := map[string]bool{}
	chains := 0
	for _, n := range result.Nodes {
		fp := importer.Fingerprint(n)
		if n.DialerProxy != "" {
			chains++
		}
		if seen[fp] {
			continue
		}
		seen[fp] = true
		relay := ""
		if n.DialerFingerprint != "" {
			relay = ids[n.DialerFingerprint]
			if relay == "" {
				t.Fatal("missing resolved dependency")
			}
		}
		n.DialerProxy = ""
		n.DialerFingerprint = ""
		state.Nodes = append(state.Nodes, model.Node{ParsedNode: n, ID: ids[fp], Fingerprint: fp, DialerID: relay, Enabled: true})
	}
	if e = model.ValidateRelayGraph(state.Nodes); e != nil {
		t.Fatal(e)
	}
	binary := os.Getenv("UNIPROXY_TEST_CORE_PATH")
	if binary != "" {
		binary, e = filepath.Abs(binary)
		if e != nil {
			t.Fatal(e)
		}
		m := core.New(core.Options{BinaryPath: binary, DataDir: t.TempDir()})
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		if e = m.Validate(ctx, state); e != nil {
			t.Fatal(e)
		}
	}
	// Fingerprints survive JSON persistence; raw dialer names are not persisted.
	encoded, e := json.Marshal(state)
	if e != nil {
		t.Fatal(e)
	}
	var restored model.State
	if e = json.Unmarshal(encoded, &restored); e != nil {
		t.Fatal(e)
	}
	if len(model.RelayErrors(restored.Nodes)) != 0 {
		t.Fatal("valid graph lost in persistence")
	}
	t.Logf("offline validation: %d entries, %d chained definitions, %d unique stored nodes; no server contacted", len(result.Nodes), chains, len(state.Nodes))
}
