package links

import (
	"path/filepath"
	"testing"
	"time"
)

func TestStoreLifecycle(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(filepath.Join(dir, "links.json"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_700_000_000, 0)
	l, err := s.Create("proj-1", "owner", 3000, "me@x.com", time.Hour, now)
	if err != nil {
		t.Fatal(err)
	}
	if !ValidToken(l.Token) {
		t.Fatalf("token %q not valid shape", l.Token)
	}
	// live before expiry
	if _, ok := s.Get(l.Token, now.Add(time.Minute)); !ok {
		t.Error("link should be live before expiry")
	}
	// expired after TTL
	if _, ok := s.Get(l.Token, now.Add(2*time.Hour)); ok {
		t.Error("link should be expired after TTL")
	}
	// list shows it while live
	if got := s.ListFor("proj-1", now); len(got) != 1 {
		t.Errorf("ListFor = %d links, want 1", len(got))
	}
	// wrong project => nothing
	if got := s.ListFor("proj-2", now); len(got) != 0 {
		t.Errorf("ListFor(other) = %d, want 0", len(got))
	}
	// revoke
	if err := s.Delete(l.Token); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Get(l.Token, now); ok {
		t.Error("revoked link should be gone")
	}
	// persistence: reload from disk
	l2, _ := s.Create("proj-1", "owner", 4000, "me@x.com", time.Hour, now)
	s2, err := NewStore(filepath.Join(dir, "links.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s2.Get(l2.Token, now); !ok {
		t.Error("link should survive reload")
	}
}

func TestValidToken(t *testing.T) {
	if ValidToken("nope") || ValidToken("") || ValidToken("ZZ"+"0123456789abcdef0123456789ab") {
		t.Error("bad tokens accepted")
	}
	if !ValidToken("0123456789abcdef0123456789abcdef") {
		t.Error("valid token rejected")
	}
}

func TestCapAndCascade(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewStore(filepath.Join(dir, "links.json"))
	now := time.Unix(1_700_000_000, 0)
	// Fill to the cap for project A.
	for i := 0; i < MaxLivePerProject; i++ {
		if _, err := s.Create("A", "o", 3000, "e", time.Hour, now); err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
	}
	if _, err := s.Create("A", "o", 3000, "e", time.Hour, now); err != ErrTooManyLinks {
		t.Errorf("expected ErrTooManyLinks at cap, got %v", err)
	}
	// A different project is unaffected.
	if _, err := s.Create("B", "o", 3000, "e", time.Hour, now); err != nil {
		t.Errorf("project B create failed: %v", err)
	}
	// Cascade delete drops all of A's links, leaves B.
	if err := s.DeleteProject("A"); err != nil {
		t.Fatal(err)
	}
	if got := len(s.ListFor("A", now)); got != 0 {
		t.Errorf("after DeleteProject(A): %d links, want 0", got)
	}
	if got := len(s.ListFor("B", now)); got != 1 {
		t.Errorf("project B: %d links, want 1", got)
	}
}

func TestListForPrunesExpired(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewStore(filepath.Join(dir, "links.json"))
	now := time.Unix(1_700_000_000, 0)
	s.Create("A", "o", 3000, "e", time.Hour, now)
	// After expiry, ListFor prunes and returns nothing, and the map shrinks.
	_ = s.ListFor("A", now.Add(2*time.Hour))
	if len(s.links) != 0 {
		t.Errorf("expired link not pruned by ListFor: %d remain", len(s.links))
	}
}
