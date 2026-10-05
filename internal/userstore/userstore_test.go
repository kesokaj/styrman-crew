package userstore

import (
	"path/filepath"
	"testing"

	"github.com/kesokaj/styrman-crew/internal/identity"
)

func newStore(t *testing.T, env ...string) *Store {
	t.Helper()
	s, err := New(filepath.Join(t.TempDir(), "users.json"), env)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

func TestAddAuthorizeRemove(t *testing.T) {
	s := newStore(t, "admin@corp.com")
	if !s.IsAuthorized("admin@corp.com") || !s.IsAdmin("admin@corp.com") {
		t.Fatal("env admin must be authorized + admin")
	}
	if s.IsAuthorized("nobody@corp.com") {
		t.Fatal("unknown email must not be authorized")
	}
	if err := s.Add("Bob@Corp.com", "admin@corp.com"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if !s.IsAuthorized("bob@corp.com") { // normalized (lowercased)
		t.Fatal("added email must be authorized, case-insensitively")
	}
	if s.IsAdmin("bob@corp.com") {
		t.Fatal("a runtime-added user is not an admin")
	}
	if err := s.Remove("bob@corp.com"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if s.IsAuthorized("bob@corp.com") {
		t.Fatal("removed email must no longer be authorized")
	}
	// env-derived users cannot be removed.
	if err := s.Remove("admin@corp.com"); err == nil {
		t.Fatal("env user must not be removable")
	}
}

func TestInvalidEmailRejected(t *testing.T) {
	s := newStore(t)
	for _, bad := range []string{"", "   ", "no-at-sign"} {
		if err := s.Add(bad, "x"); err == nil {
			t.Fatalf("Add(%q) should fail", bad)
		}
	}
}

// H3: two authorized emails must never collapse to the same username slug.
func TestSlugCollision(t *testing.T) {
	// Add-time: a colliding local part across domains is rejected.
	s := newStore(t)
	if err := s.Add("alice@a.com", "admin"); err != nil {
		t.Fatalf("first Add: %v", err)
	}
	if err := s.Add("alice@b.com", "admin"); err == nil {
		t.Fatal("alice@b.com collides with alice@a.com (both slug to \"alice\") and must be rejected")
	}
	if s.IsAuthorized("alice@b.com") {
		t.Fatal("rejected email must not be authorized")
	}
	// A distinct slug is fine.
	if err := s.Add("bob@b.com", "admin"); err != nil {
		t.Fatalf("non-colliding Add: %v", err)
	}

	// Startup: a colliding pair in the env allowlist fails closed.
	if _, err := New(filepath.Join(t.TempDir(), "u.json"), []string{"joe@a.com", "joe@b.com"}); err == nil {
		t.Fatal("New must refuse two env emails that slug to the same username")
	}
	// The same email in both env and (future) file is not a collision.
	if _, err := New(filepath.Join(t.TempDir(), "u.json"), []string{"joe@a.com", "JOE@a.com"}); err != nil {
		t.Fatalf("duplicate of the same email must be allowed: %v", err)
	}
}

func TestIsAdminUsername(t *testing.T) {
	s, err := New(filepath.Join(t.TempDir(), "users.json"), []string{"Boss@Example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Add("member@example.com", "boss@example.com"); err != nil {
		t.Fatal(err)
	}
	if !s.IsAdminUsername(identity.DeriveUsername("boss@example.com")) {
		t.Error("the env admin's username is not admin")
	}
	if s.IsAdminUsername(identity.DeriveUsername("member@example.com")) {
		t.Error("a runtime-added user's username reads as admin")
	}
	if s.IsAdminUsername("") || s.IsAdminUsername("nobody") {
		t.Error("an unknown username reads as admin")
	}
}
