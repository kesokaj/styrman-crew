package main

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kesokaj/styrman-crew/internal/config"
	"github.com/kesokaj/styrman-crew/internal/orchestrator"
	"github.com/kesokaj/styrman-crew/internal/project"
)

// TestMigrateWorkspaces: pre-ADR-027 workspaces (inside the home) move to
// their per-project dirs exactly once; conflicts are left alone.
func TestMigrateWorkspaces(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{StateDir: dir, HomesDir: filepath.Join(dir, "homes")}
	logger := slog.New(slog.DiscardHandler)
	// Status() for a nonexistent container is "absent" (or errors without a
	// docker daemon) — either way the migration proceeds without stopping.
	orch, err := orchestrator.New(cfg, logger)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll(cfg.UserStateDir("alice"), 0o755); err != nil {
		t.Fatal(err)
	}
	store, err := project.NewStore(filepath.Join(cfg.UserStateDir("alice"), "projects.json"))
	if err != nil {
		t.Fatal(err)
	}
	p, err := store.Add(project.Project{Name: "demo", CreatedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}

	oldWS := filepath.Join(cfg.UserHome("alice"), "projects", p.Dir)
	os.MkdirAll(oldWS, 0o755)
	os.WriteFile(filepath.Join(oldWS, "main.go"), []byte("package main"), 0o644)

	migrateWorkspaces(cfg, orch, logger)

	newWS := cfg.ProjectWorkspace("alice", p.ID)
	if b, err := os.ReadFile(filepath.Join(newWS, "main.go")); err != nil || string(b) != "package main" {
		t.Fatalf("migrated file missing: %v", err)
	}
	if _, err := os.Stat(oldWS); !os.IsNotExist(err) {
		t.Fatal("old workspace still present after migration")
	}

	// Idempotent: a second run must not touch anything.
	migrateWorkspaces(cfg, orch, logger)
	if _, err := os.Stat(filepath.Join(newWS, "main.go")); err != nil {
		t.Fatal("second run disturbed the migrated workspace")
	}

	// Conflict (both exist): the new location wins, the old is left alone.
	os.MkdirAll(oldWS, 0o755)
	os.WriteFile(filepath.Join(oldWS, "stale.txt"), []byte("x"), 0o644)
	migrateWorkspaces(cfg, orch, logger)
	if _, err := os.Stat(filepath.Join(newWS, "stale.txt")); !os.IsNotExist(err) {
		t.Fatal("conflicting old workspace was merged over the new one")
	}

	// ensureRepos initializes the migrated (and any missing) workspace.
	ensureRepos(cfg, logger)
	if _, err := os.Stat(filepath.Join(newWS, ".git")); err != nil {
		t.Fatal("R13 sweep did not initialize the migrated workspace")
	}
}

// TestEnsureReposRefusesUnmigrated: while the real tree still sits in the
// home (failed migration), the R13 sweep must NOT create the new location —
// an empty dir there would shadow the real workspace forever.
func TestEnsureReposRefusesUnmigrated(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{StateDir: dir, HomesDir: filepath.Join(dir, "homes")}
	logger := slog.New(slog.DiscardHandler)
	os.MkdirAll(cfg.UserStateDir("bob"), 0o755)
	store, err := project.NewStore(filepath.Join(cfg.UserStateDir("bob"), "projects.json"))
	if err != nil {
		t.Fatal(err)
	}
	p, _ := store.Add(project.Project{Name: "stuck", CreatedAt: time.Now()})
	oldWS := filepath.Join(cfg.UserHome("bob"), "projects", p.Dir)
	os.MkdirAll(oldWS, 0o755)
	// Non-empty: an EMPTY legacy dir is just the bind's mount-point stub and
	// deliberately does not trigger the guard.
	os.WriteFile(filepath.Join(oldWS, "real.go"), []byte("package real"), 0o644)

	ensureRepos(cfg, logger) // migration did NOT run — the guard must hold
	if _, err := os.Stat(cfg.ProjectWorkspace("bob", p.ID)); !os.IsNotExist(err) {
		t.Fatal("ensureRepos created the new workspace while the legacy one exists")
	}
}
