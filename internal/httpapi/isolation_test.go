package httpapi

import (
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kesokaj/styrman-crew/internal/config"
	"github.com/kesokaj/styrman-crew/internal/identity"
	"github.com/kesokaj/styrman-crew/internal/orchestrator"
	"github.com/kesokaj/styrman-crew/internal/project"
	"github.com/kesokaj/styrman-crew/internal/userstore"
)

// TestProjectIsolationAcrossUsers is the explicit IDOR regression (review test
// gap): two DISTINCT authorized users must never reach each other's projects by
// UUID, and one user's project list must not leak another's.
func TestProjectIsolationAcrossUsers(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{
		StateDir:     dir,
		HomesDir:     filepath.Join(dir, "homes"),
		Username:     "tester",
		AuthMode:     "header",
		IngressToken: "test-ingress-token", // matches doAs()
	}
	logger := slog.New(slog.DiscardHandler)
	users, err := userstore.New(filepath.Join(dir, "users.json"),
		[]string{"alice@gmail.com", "bob@gmail.com"}) // both authorized
	if err != nil {
		t.Fatal(err)
	}
	orch, err := orchestrator.New(cfg, logger)
	if err != nil {
		t.Fatal(err)
	}
	s := New(cfg, users, orch, logger)
	spAlice, err := s.space(identity.DeriveUsername("alice@gmail.com"))
	if err != nil {
		t.Fatal(err)
	}
	p, err := spAlice.projects.Add(project.Project{Name: "aliceproj"})
	if err != nil {
		t.Fatal(err)
	}
	h := s.Routes()

	// bob (a different authorized user) must get 404 for alice's project on
	// every project-scoped route — getProjectFor resolves the UUID through the
	// REQUESTER's own store, so it simply isn't found.
	for _, path := range []string{
		"/api/projects/" + p.ID,
		"/api/projects/" + p.ID + "/history",
		"/api/projects/" + p.ID + "/git/status",
		"/api/projects/" + p.ID + "/network",
	} {
		if w := doAs(t, h, "bob@gmail.com", "GET", path, ""); w.Code != http.StatusNotFound {
			t.Errorf("bob GET %s: got %d, want 404 (cross-user access must be denied)", path, w.Code)
		}
	}

	// The project must appear in alice's list and NOT in bob's.
	if body := doAs(t, h, "bob@gmail.com", "GET", "/api/projects", "").Body.String(); strings.Contains(body, p.ID) {
		t.Errorf("bob's project list leaked alice's project id %s", p.ID)
	}
	if body := doAs(t, h, "alice@gmail.com", "GET", "/api/projects", "").Body.String(); !strings.Contains(body, p.ID) {
		t.Errorf("alice's own project missing from her list")
	}
}
