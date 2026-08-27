package httpapi

import (
	"path/filepath"
	"testing"
)

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
