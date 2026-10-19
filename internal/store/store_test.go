package store

import (
	"bytes"
	"github.com/uniproxy/uniproxy/internal/model"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestEncryptedRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s, e := Open(dir)
	if e != nil {
		t.Fatal(e)
	}
	state := s.Snapshot()
	state.Nodes = append(state.Nodes, model.Node{ParsedNode: model.ParsedNode{Name: "test", Config: map[string]any{"password": "secret-do-not-store-plaintext"}}, ID: "a", Enabled: true})
	state.Revision = 4
	if e = s.Save(state); e != nil {
		t.Fatal(e)
	}
	s.Activity("info", "test event")
	if len(s.Activities()) != 1 {
		t.Fatal("activity missing")
	}
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile(filepath.Join(dir, "uniproxy.db"))
	if e != nil {
		t.Fatal(e)
	}
	if bytes.Contains(b, []byte("secret-do-not-store-plaintext")) {
		t.Fatal("secret in plaintext database")
	}
	s, e = Open(dir)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	got := s.Snapshot()
	if got.Revision != 4 || got.Nodes[0].Config["password"] != "secret-do-not-store-plaintext" {
		t.Fatal("state mismatch")
	}
	got.Nodes[0].Name = "changed"
	if s.Snapshot().Nodes[0].Name == "changed" {
		t.Fatal("snapshot was mutable alias")
	}
}
func TestMissingKeyDoesNotOverwrite(t *testing.T) {
	dir := t.TempDir()
	s, e := Open(dir)
	if e != nil {
		t.Fatal(e)
	}
	s.Close()
	if e = os.Remove(filepath.Join(dir, "master.key")); e != nil {
		t.Fatal(e)
	}
	if s, e = Open(dir); e == nil {
		s.Close()
		t.Fatal("expected missing-key error")
	}
}

// A byte-sliced activity message used to split a multi-byte rune, which the UI
// then rendered as replacement characters. Bound the text by runes instead.
func TestActivityTruncationKeepsValidUTF8(t *testing.T) {
	s, e := Open(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	s.Activity("warning", strings.Repeat("测", 3000))
	activities := s.Activities()
	if len(activities) != 1 {
		t.Fatalf("expected one activity, got %d", len(activities))
	}
	message := activities[0].Message
	if !utf8.ValidString(message) || strings.ContainsRune(message, utf8.RuneError) {
		t.Fatal("activity text was cut inside a multi-byte character")
	}
	if runes := utf8.RuneCountInString(message); runes > 2000 {
		t.Fatalf("activity text exceeds the rune bound: %d", runes)
	}
	// A long byte string made of few runes must survive intact.
	s.Activity("info", strings.Repeat("a", 1999))
	if got := s.Activities()[0].Message; got != strings.Repeat("a", 1999) {
		t.Fatal("short message was altered")
	}
}
