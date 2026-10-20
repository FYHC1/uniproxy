package store

import (
	"bytes"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/uniproxy/uniproxy/internal/model"
)

func TestTruncatedDatabaseReportsCorruptionWithoutReset(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	state := s.Snapshot()
	state.Revision = 25
	state.Nodes = []model.Node{{ID: "synthetic", ParsedNode: model.ParsedNode{Name: "synthetic", Config: map[string]any{"padding": strings.Repeat("synthetic-secret-CANARY", 20000)}}}}
	if err := s.Save(state); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "uniproxy.db")
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	pageSize := int(binary.BigEndian.Uint16(original[16:18]))
	if len(original) <= pageSize*4 {
		t.Fatal("fixture did not allocate overflow pages")
	}
	// This is exclusively a newly created synthetic fixture, never a user file.
	truncated := original[:pageSize*4]
	if err := os.WriteFile(path, truncated, 0600); err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(dir, "master.key")
	key, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	bad, err := Open(dir)
	if bad != nil {
		bad.Close()
		t.Fatal("truncated database was accepted")
	}
	if !errors.Is(err, ErrCorrupt) || !strings.Contains(err.Error(), "WAL/SHM") || strings.Contains(err.Error(), "CANARY") {
		t.Fatalf("corruption was not safely diagnosed: %v", err)
	}
	after, readErr := os.ReadFile(path)
	if readErr != nil || !bytes.Equal(after, truncated) {
		t.Fatal("failed startup rewrote the corrupted database")
	}
	keyAfter, err := os.ReadFile(keyPath)
	if err != nil || !bytes.Equal(key, keyAfter) {
		t.Fatal("failed startup changed the matching master key")
	}
}

// Reproduce the incident without private files: an old, small database has a
// pending activity WAL; a different large database replaces its main file while
// the connection is open. Close checkpoints the old WAL's page count over the
// replacement. This unsupported operation must be diagnosed, never "repaired"
// by silently initializing an empty configuration on the next Open.
func TestLiveDatabaseOverwriteIsDiagnosedRatherThanReset(t *testing.T) {
	root := t.TempDir()
	source, live := filepath.Join(root, "source"), filepath.Join(root, "live")
	s, err := Open(source)
	if err != nil {
		t.Fatal(err)
	}
	state := s.Snapshot()
	state.Revision = 25
	state.Nodes = []model.Node{{ID: "synthetic", ParsedNode: model.ParsedNode{Name: "synthetic", Config: map[string]any{"padding": strings.Repeat("synthetic", 40000)}}}}
	if err := s.Save(state); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	key, err := os.ReadFile(filepath.Join(source, "master.key"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(live, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(live, "master.key"), key, 0600); err != nil {
		t.Fatal(err)
	}
	initial, err := Open(live)
	if err != nil {
		t.Fatal(err)
	}
	if err := initial.Close(); err != nil {
		t.Fatal(err)
	}
	running, err := Open(live)
	if err != nil {
		t.Fatal(err)
	}
	defer running.Close()
	for i := 0; i < 9; i++ {
		running.Activity("info", "synthetic pending WAL")
	}
	image, err := os.ReadFile(filepath.Join(source, "uniproxy.db"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(live, "uniproxy.db")
	if err := os.WriteFile(path, image, 0600); err != nil {
		t.Fatal(err)
	}
	if err := running.Close(); err != nil {
		t.Fatal(err)
	}
	truncated, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(truncated) >= len(image) {
		t.Fatal("fixture did not reproduce the old WAL's truncation")
	}
	bad, err := Open(live)
	if bad != nil {
		bad.Close()
		t.Fatal("unsafe live replacement accepted")
	}
	if !errors.Is(err, ErrCorrupt) {
		t.Fatalf("expected corruption: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(after, truncated) {
		t.Fatal("failed reopen changed corrupted evidence")
	}
}

type testSQLiteError int

func (e testSQLiteError) Error() string { return "private raw error must not leak" }
func (e testSQLiteError) Code() int     { return int(e) }

func TestOpenErrorClassification(t *testing.T) {
	for _, code := range []int{11, 11 | (2 << 8), 26} {
		err := describeOpenError(testSQLiteError(code))
		if !errors.Is(err, ErrCorrupt) || strings.Contains(err.Error(), "private raw") {
			t.Fatalf("code %d: %v", code, err)
		}
	}
	busy := testSQLiteError(5)
	if got := describeOpenError(busy); got != busy {
		t.Fatal("ordinary lock error was misclassified as corruption")
	}
}
