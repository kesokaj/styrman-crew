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

func plantFile(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func listTree(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	_ = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err == nil && p != root {
			rel, _ := filepath.Rel(root, p)
			out = append(out, rel)
		}
		return nil
	})
	return out
}

// Before ADR-064 the setup VM mounted homes/<user> itself, with every project
// home inside it. The migration moves everything but base/ and p/ into base/,
// leaves p/ alone, is idempotent, skips an already-migrated tenant, refuses a
// planted `base` symlink, and ignores a symlinked tenant entry.
func TestMigrateBaseHomes(t *testing.T) {
	homes := t.TempDir()
	cfg := &config.Config{HomesDir: homes}
	log := slog.New(slog.DiscardHandler)
	uuid := "11111111-1111-4111-8111-111111111111"

	// alice: the old layout
	alice := filepath.Join(homes, "alice")
	for _, f := range []string{".claude/marker", ".styrman/setup-done", "projects/legacy-ws/keep.txt", "stray.txt", "p/" + uuid + "/keep.txt"} {
		plantFile(t, filepath.Join(alice, f))
	}
	before := listTree(t, filepath.Join(alice, "p"))
	// bob: already migrated
	bob := filepath.Join(homes, "bob")
	plantFile(t, filepath.Join(bob, "base", ".claude", "x"))
	plantFile(t, filepath.Join(bob, "p", uuid, "keep.txt"))
	bobBefore := listTree(t, bob)
	// carol: a planted symlink at base
	carol := filepath.Join(homes, "carol")
	outside := t.TempDir()
	plantFile(t, filepath.Join(carol, ".claude", "x"))
	if err := os.Symlink(outside, filepath.Join(carol, "base")); err != nil {
		t.Fatal(err)
	}
	// dave: the tenant entry itself is a symlink
	daveTarget := t.TempDir()
	plantFile(t, filepath.Join(daveTarget, ".claude", "x"))
	if err := os.Symlink(daveTarget, filepath.Join(homes, "dave")); err != nil {
		t.Fatal(err)
	}

	migrateBaseHomes(cfg, log)

	for _, f := range []string{"base/.claude/marker", "base/.styrman/setup-done", "base/projects/legacy-ws/keep.txt", "base/stray.txt", "p/" + uuid + "/keep.txt"} {
		if _, err := os.Stat(filepath.Join(alice, f)); err != nil {
			t.Errorf("alice: %s missing after migration: %v", f, err)
		}
	}
	for _, f := range []string{".claude", ".styrman", "projects", "stray.txt"} {
		if _, err := os.Lstat(filepath.Join(alice, f)); err == nil {
			t.Errorf("alice: %s still in the tenant dir", f)
		}
	}
	if got := listTree(t, filepath.Join(alice, "p")); len(got) != len(before) {
		t.Errorf("alice: p/ changed: %v -> %v", before, got)
	}
	if got := listTree(t, bob); len(got) != len(bobBefore) {
		t.Errorf("bob (already migrated) changed: %v -> %v", bobBefore, got)
	}
	if _, err := os.Stat(filepath.Join(carol, ".claude", "x")); err != nil {
		t.Errorf("carol: a planted base symlink must be refused, nothing moved: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outside, ".claude")); err == nil {
		t.Error("carol: files were moved through the planted symlink")
	}
	if _, err := os.Stat(filepath.Join(daveTarget, ".claude", "x")); err != nil {
		t.Errorf("dave: a symlinked tenant entry must be skipped: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(daveTarget, "base")); err == nil {
		t.Error("dave: migration ran through a symlinked tenant entry")
	}

	after := listTree(t, homes)
	migrateBaseHomes(cfg, log)
	if again := listTree(t, homes); len(again) != len(after) {
		t.Errorf("second run changed the tree: %d -> %d entries", len(after), len(again))
	}
}

// The pre-multitenancy home migrates straight into the ADR-064 layout: its
// target homes/<user>/base needs the tenant dir created first.
func TestMigrateLegacyHomeLandsInBase(t *testing.T) {
	state := t.TempDir()
	cfg := &config.Config{StateDir: state, HomesDir: filepath.Join(state, "homes"), HomeDir: filepath.Join(state, "home"), Username: "tester"}
	plantFile(t, filepath.Join(cfg.HomeDir, ".claude", "marker"))
	migrateLegacy(cfg, slog.New(slog.DiscardHandler))
	if _, err := os.Stat(filepath.Join(cfg.HomesDir, "tester", "base", ".claude", "marker")); err != nil {
		t.Fatalf("legacy home must land at homes/tester/base: %v", err)
	}
	if _, err := os.Stat(cfg.HomeDir); !os.IsNotExist(err) {
		t.Errorf("legacy home dir should be gone after the rename, stat err = %v", err)
	}
	migrateLegacy(cfg, slog.New(slog.DiscardHandler)) // no-op
}

// An entry that already exists under base/ (a partial earlier run) is left
// in place, never replaced; the rest still moves.
func TestMigrateBaseHomesLeavesCollisionsInPlace(t *testing.T) {
	homes := t.TempDir()
	cfg := &config.Config{HomesDir: homes}
	erin := filepath.Join(homes, "erin")
	plantFile(t, filepath.Join(erin, ".claude", "new"))
	plantFile(t, filepath.Join(erin, "base", ".claude", "old"))
	plantFile(t, filepath.Join(erin, "note.txt"))
	migrateBaseHomes(cfg, slog.New(slog.DiscardHandler))
	if _, err := os.Stat(filepath.Join(erin, ".claude", "new")); err != nil {
		t.Errorf("a colliding entry must stay in the tenant dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(erin, "base", ".claude", "old")); err != nil {
		t.Errorf("the existing base entry must be untouched: %v", err)
	}
	if _, err := os.Stat(filepath.Join(erin, "base", "note.txt")); err != nil {
		t.Errorf("the non-colliding entry must still move: %v", err)
	}
}

// A half-migrated host (a legacy home AND a populated tenant dir without
// base/) must not get the legacy home renamed on top of the tenant's own
// entries: step 1 is skipped, both stay where they are.
func TestMigrateLegacySkipsAHalfMigratedTenant(t *testing.T) {
	state := t.TempDir()
	cfg := &config.Config{StateDir: state, HomesDir: filepath.Join(state, "homes"), HomeDir: filepath.Join(state, "home"), Username: "tester"}
	plantFile(t, filepath.Join(cfg.HomeDir, ".claude", "legacy"))
	plantFile(t, filepath.Join(cfg.TenantDir("tester"), ".claude", "own"))
	migrateLegacy(cfg, slog.New(slog.DiscardHandler))
	if _, err := os.Stat(filepath.Join(cfg.HomeDir, ".claude", "legacy")); err != nil {
		t.Errorf("the legacy home must stay put on a half-migrated tenant: %v", err)
	}
	if _, err := os.Stat(cfg.UserHome("tester")); !os.IsNotExist(err) {
		t.Errorf("base/ must not be created by the legacy step here, stat err = %v", err)
	}
}

// --- restored from v1.20.0 (overwritten by the first ADR-064 test file, BUG-005) ---

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
