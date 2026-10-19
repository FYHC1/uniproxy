package app

import (
	"context"
	"github.com/uniproxy/uniproxy/internal/core"
	"github.com/uniproxy/uniproxy/internal/importer"
	"github.com/uniproxy/uniproxy/internal/model"
	"os"
	"testing"
	"time"
)

func TestPrivateClashFilePreviewImportAndReimport(t *testing.T) {
	source := os.Getenv("UNIPROXY_TEST_IMPORT_FILE")
	if source == "" {
		t.Skip("set UNIPROXY_TEST_IMPORT_FILE for private offline app validation")
	}
	raw, e := os.ReadFile(source)
	if e != nil {
		t.Fatal("cannot read provided file")
	}
	parsed := importer.Parse(string(raw))
	if len(parsed.Errors) > 0 {
		t.Fatalf("private file has %d parse errors", parsed.TotalErrors)
	}
	a, _ := fixture(t, false)
	preview, e := a.importNodes(context.Background(), importRequest{Mode: "file", Content: string(raw), Preview: true})
	if e != nil {
		t.Fatal(e)
	}
	if pr, ok := preview.(model.ParseResult); ok {
		if len(pr.Nodes) != len(parsed.Nodes) || pr.TotalErrors != 0 {
			t.Fatal("incomplete private preview")
		}
		for _, n := range pr.Nodes {
			if n.Config != nil {
				t.Fatal("private preview contains credentials")
			}
		}
	} else {
		t.Fatal("unexpected preview type")
	}
	if preview == nil || len(a.store.Snapshot().Nodes) != 0 {
		t.Fatal("preview changed stored data")
	}
	_, e = a.importNodes(context.Background(), importRequest{Mode: "file", Content: string(raw), GroupNames: []string{"private-import-test"}})
	if e != nil {
		t.Fatal(e)
	}
	first := a.store.Snapshot()
	ids := map[string]bool{}
	chained := 0
	for _, n := range first.Nodes {
		ids[n.ID] = true
		if n.DialerID != "" {
			chained++
		}
	}
	for _, n := range first.Nodes {
		if n.DialerID != "" && !ids[n.DialerID] {
			t.Fatal("unresolved stored relay")
		}
	}
	_, e = a.importNodes(context.Background(), importRequest{Mode: "file", Content: string(raw)})
	if e != nil {
		t.Fatal(e)
	}
	second := a.store.Snapshot()
	if len(second.Nodes) != len(first.Nodes) {
		t.Fatal("reimport created duplicates")
	}
	for _, n := range second.Nodes {
		if !ids[n.ID] {
			t.Fatal("stable IDs changed")
		}
	}
	if binary := os.Getenv("UNIPROXY_TEST_CORE_PATH"); binary != "" {
		manager := core.New(core.Options{BinaryPath: binary, DataDir: t.TempDir()})
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		if e = manager.Validate(ctx, second); e != nil {
			t.Fatal(e)
		}
	}
	t.Logf("isolated preview/import/reimport: %d source entries, %d stored unique nodes, %d dependent nodes; current user DB untouched", len(parsed.Nodes), len(second.Nodes), chained)
}
