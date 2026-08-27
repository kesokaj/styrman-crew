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
