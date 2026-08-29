package conversation

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStoreLifecycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conversations.json")
	s, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	r1, err := s.Create("alpha", "")
	if err != nil {
		t.Fatal(err)
	}
	// An empty title gets a generated friendly name (adjective noun).
	if r1.ID != "1" || r1.Title == "" || len(strings.Fields(r1.Title)) < 2 {
		t.Errorf("first record = %+v", r1)
	}
	if r1.AgentSession == "" || len(strings.Split(r1.AgentSession, "-")) != 5 {
		t.Errorf("expected UUID agent session, got %q", r1.AgentSession)
	}
	r2, _ := s.Create("alpha", "My chat")
	if r2.ID != "2" || r2.Title != "My chat" {
		t.Errorf("second record = %+v", r2)
	}

	// Reload: persistence + validation-on-load.
	s2, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := s2.List("alpha"); len(got) != 2 {
		t.Fatalf("reloaded %d records, want 2", len(got))
	}
	rec, ok := s2.Get("alpha", "1")
	if !ok || rec.AgentSession != r1.AgentSession {
		t.Errorf("agent session not stable across reload")
	}

	if err := s2.Rename("alpha", "2", "Renamed"); err != nil {
		t.Fatal(err)
	}
	if rec, _ := s2.Get("alpha", "2"); rec.Title != "Renamed" {
		t.Error("rename did not stick")
	}
	if err := s2.Delete("alpha", "1"); err != nil {
		t.Fatal(err)
	}
	if _, ok := s2.Get("alpha", "1"); ok {
		t.Error("deleted record still present")
	}
	if err := s2.DeleteProject("alpha"); err != nil {
		t.Fatal(err)
	}
	if got := s2.List("alpha"); len(got) != 0 {
		t.Errorf("DeleteProject left %d records", len(got))
	}
}

// A cached terminal Store must survive its metadata dir being scrubbed under a
// running server (shared cause with the project/profile stores): the next save
// recreates users/<user>/ instead of failing with ENOENT.
func TestSaveRecreatesMissingDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "users", "ostling-simon")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	s, err := NewStore(filepath.Join(dir, "conversations.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create("alpha", ""); err != nil {
		t.Fatalf("first Create (dir present): %v", err)
	}
	if err := os.RemoveAll(dir); err != nil { // the scrub, store still live
		t.Fatalf("scrub: %v", err)
	}
	if _, err := s.Create("alpha", ""); err != nil {
		t.Fatalf("Create after the dir was scrubbed must recover, got: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "conversations.json")); err != nil {
		t.Fatalf("conversations.json not persisted after recovery: %v", err)
	}
}
