package httpapi

import (
	"os"
	"path/filepath"
	"testing"
)

// A cached profileStore can outlive its metadata directory: space() creates
// users/<user>/ and caches the store, then an operator scrub of the users tree
// under a running server removes the dir while the store stays cached. The
// next profile write must recreate the dir, not fail with ENOENT (the live
// regression seen on 2026-08-29). Reproduced faithfully: populate the dir, hold
// the store, delete the dir out from under it, then write again.
func TestSetGuestUserRecreatesMissingDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "users", "ostling-simon")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	p := &profileStore{path: filepath.Join(dir, "profile.json"), def: "ostling-simon"}
	if err := p.setGuestUser("simon"); err != nil {
		t.Fatalf("first write (dir present) must succeed, got: %v", err)
	}
	// The scrub: remove the whole users/<user>/ dir while the store stays live.
	if err := os.RemoveAll(dir); err != nil {
		t.Fatalf("scrub: %v", err)
	}
	if err := p.setGuestUser("simon2"); err != nil {
		t.Fatalf("write after the dir was scrubbed must recover, got: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "profile.json")); err != nil {
		t.Fatalf("profile.json not persisted after recovery: %v", err)
	}
}

func TestSetGuestUserRejectsReserved(t *testing.T) {
	p := &profileStore{path: filepath.Join(t.TempDir(), "profile.json"), def: "tester"}
	for _, bad := range []string{"root", "daemon", "docker", "nobody"} {
		if err := p.setGuestUser(bad); err == nil {
			t.Errorf("reserved name %q must be rejected", bad)
		}
	}
	for _, bad := range []string{"Bad", "1abc", "no spaces", "a/b"} {
		if err := p.setGuestUser(bad); err == nil {
			t.Errorf("invalid name %q must be rejected", bad)
		}
	}
	if err := p.setGuestUser("simon"); err != nil {
		t.Errorf("valid name rejected: %v", err)
	}
	if p.GuestUser() != "simon" {
		t.Errorf("GuestUser = %q, want simon", p.GuestUser())
	}
}
