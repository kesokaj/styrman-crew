package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kesokaj/styrman-crew/internal/identity"
	"github.com/kesokaj/styrman-crew/internal/project"
)

// seed writes an empty file at rel under home, creating parents.
func seed(t *testing.T, home, rel string) {
	t.Helper()
	p := filepath.Join(home, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func toolsByName(t *testing.T, s *Server) map[string]toolInfo {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/settings/tools", nil)
	req = identity.WithUser(req, identity.User{Email: "tester@example.com", Username: "tester", Authorized: true})
	s.toolDetails(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	var body struct{ Tools []toolInfo }
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	out := map[string]toolInfo{}
	for _, ti := range body.Tools {
		out[ti.Name] = ti
	}
	return out
}

// The settings roster comes from a SEEDED project home, never the base home
// (which the setup VM, running MINIMAL, never seeds — the roster was empty
// forever). The newest project without a manifest is skipped; `*.bak.*`
// re-seed leftovers are not agents; agy agents live one directory each.
func TestRosterReadsTheNewestSeededProjectHome(t *testing.T) {
	s := testServer(t)
	sp, err := s.space("tester")
	if err != nil {
		t.Fatal(err)
	}
	add := func(name string, at time.Time) string {
		p, err := sp.projects.Add(project.Project{Name: name, CreatedAt: at})
		if err != nil {
			t.Fatal(err)
		}
		return p.ID
	}
	now := time.Now().UTC()
	old := add("old-seeded", now.Add(-2*time.Hour))
	mid := add("mid-seeded", now.Add(-time.Hour))
	add("new-unseeded", now)

	// Base home: what the OLD code read — must be ignored.
	base := s.cfg.UserHome("tester")
	seed(t, base, ".claude/agents/base-only.md")

	for _, id := range []string{old, mid} {
		h := s.cfg.ProjectHome("tester", id)
		seed(t, h, ".config/sh-agent/state.json")
		seed(t, h, ".claude/skills/"+id[:4]+"/SKILL.md")
	}
	h := s.cfg.ProjectHome("tester", mid)
	seed(t, h, ".claude/agents/reviewer.md")
	seed(t, h, ".claude/agents/verifier.md")
	seed(t, h, ".claude/agents/testing.md.bak.20260901-065226")
	seed(t, h, ".claude/agents/.hidden.md")
	seed(t, h, ".claude/agents/a.bak.tool.md") // legitimately named: NOT a re-seed backup
	seed(t, h, ".gemini/config/skills/golang/SKILL.md")
	seed(t, h, ".gemini/config/agents/reviewer/agent.md")
	seed(t, h, ".gemini/config/agents/verifier/agent.md")
	seed(t, h, ".gemini/config/agents/old.md.bak.20260901-065226")

	tools := toolsByName(t, s)
	claude, agy := tools["claude"], tools["agy"]

	if got, want := claude.Agents, []string{"a.bak.tool", "reviewer", "verifier"}; !equalStrings(got, want) {
		t.Errorf("claude agents = %v, want %v", got, want)
	}
	if got, want := claude.Skills, []string{mid[:4]}; !equalStrings(got, want) {
		t.Errorf("claude skills = %v, want %v (the NEWEST seeded home, not %s)", got, want, old[:4])
	}
	if claude.Commands != nil {
		t.Errorf("claude commands = %v, want none (the harness ships no commands)", claude.Commands)
	}
	if got, want := agy.Agents, []string{"reviewer", "verifier"}; !equalStrings(got, want) {
		t.Errorf("agy agents = %v, want %v", got, want)
	}
	if got, want := agy.Skills, []string{"golang"}; !equalStrings(got, want) {
		t.Errorf("agy skills = %v, want %v", got, want)
	}
}

// No seeded project → the roster fields are empty (and omitted), as before.
func TestRosterIsEmptyWithoutASeededProject(t *testing.T) {
	s := testServer(t)
	seed(t, s.cfg.UserHome("tester"), ".claude/agents/base-only.md")
	sp, err := s.space("tester")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sp.projects.Add(project.Project{Name: "unseeded", CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	tools := toolsByName(t, s)
	if c := tools["claude"]; c.Skills != nil || c.Agents != nil || c.Commands != nil {
		t.Errorf("roster should be empty without a seeded project, got %+v", c)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
