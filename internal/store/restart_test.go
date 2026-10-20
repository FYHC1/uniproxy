package store

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

// journal_mode needs a lock before any other initialization SQL can run. The
// busy timeout must already be active, including while recovering/reopening a
// store immediately after another owner releases it.
func TestOpenWaitsForTransientJournalLock(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	state := s.Snapshot()
	state.Revision = 63
	if err := s.Save(state); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	holder, err := sql.Open("sqlite", filepath.ToSlash(filepath.Join(dir, "uniproxy.db")))
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Close()
	holder.SetMaxOpenConns(1)
	if _, err := holder.Exec("PRAGMA journal_mode=DELETE"); err != nil {
		t.Fatal(err)
	}
	if _, err := holder.Exec("BEGIN EXCLUSIVE"); err != nil {
		t.Fatal(err)
	}
	defer holder.Exec("ROLLBACK")
	type result struct {
		store *Store
		err   error
	}
	done := make(chan result, 1)
	go func() {
		s, err := Open(dir)
		done <- result{s, err}
	}()
	select {
	case got := <-done:
		if got.store != nil {
			got.store.Close()
		}
		t.Fatalf("startup returned before the temporary lock was released: %v", got.err)
	case <-time.After(150 * time.Millisecond):
	}
	if _, err := holder.Exec("COMMIT"); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-done:
		if got.err != nil {
			t.Fatal(got.err)
		}
		defer got.store.Close()
		if got.store.Snapshot().Revision != 63 {
			t.Fatal("journal recovery replaced the existing configuration")
		}
	case <-time.After(6 * time.Second):
		t.Fatal("startup did not resume after transient lock release")
	}
}
