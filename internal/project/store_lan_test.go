package project

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The network mode (ADR-067): round-trips through a fresh Store, is absent
// from the bytes while off, and rolls back when the save fails.
func TestSetLanAccessRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "projects.json")
	s, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.Add(Project{Name: "alpha", CreatedAt: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	if strings.Contains(string(raw), "lanAccess") {
		t.Fatalf("sandbox is the default and must not be serialised: %s", raw)
	}

	if err := s.SetLanAccess(p.ID, true); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Get(p.ID); !got.LanAccess {
		t.Fatalf("after enable: %+v", got)
	}
	// Survives a reopen — the record is the only source of truth for the mode,
	// so a restart must not silently sandbox a LAN project.
	s2, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := s2.Get(p.ID); !got.LanAccess {
		t.Fatalf("mode did not survive a reopen: %+v", got)
	}
	if err := s.SetLanAccess(p.ID, false); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Get(p.ID); got.LanAccess {
		t.Fatalf("after disable: %+v", got)
	}
	raw, _ = os.ReadFile(path)
	if strings.Contains(string(raw), "lanAccess") {
		t.Fatalf("disable must drop the field again: %s", raw)
	}
}

func TestSetLanAccessRollsBackOnSaveFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "projects.json")
	s, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.Add(Project{Name: "alpha", CreatedAt: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	// Make the save fail: the store path becomes a directory.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := s.SetLanAccess(p.ID, true); err == nil {
		t.Fatal("save should have failed")
	}
	if got, _ := s.Get(p.ID); got.LanAccess {
		t.Fatalf("memory must match disk after a failed save: %+v", got)
	}
}

// A record written by v1.23/v1.24 carries lanChangedAt and lanEverEnabled,
// which ADR-067 deleted. Loading one must keep the mode, ignore the two dead
// fields, and drop them from the file on the next write — an upgrade must not
// lose a project's LAN setting, and must not carry the latch forward either
// (the latch is what permanently withheld the SSH key).
func TestLoadDropsPreADR067Fields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "projects.json")
	const id = "3e6a772c-b177-4cfc-b209-b14db514bac2"
	legacy := `{"projects":[{"id":"` + id + `","name":"alpha","dir":"alpha",` +
		`"createdAt":"2026-09-08T12:00:00Z","lanAccess":true,` +
		`"lanChangedAt":"2026-09-08T12:00:00Z","lanEverEnabled":true}]}`
	if err := os.WriteFile(path, []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if !got.LanAccess {
		t.Fatalf("upgrade lost the project's LAN mode: %+v", got)
	}
	// Force a rewrite and confirm both dead fields are gone from the bytes.
	if err := s.SetLanAccess(id, false); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	for _, dead := range []string{"lanChangedAt", "lanEverEnabled"} {
		if strings.Contains(string(raw), dead) {
			t.Fatalf("%s survived the rewrite: %s", dead, raw)
		}
	}
	var f storeFile
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatalf("rewritten store is not valid JSON: %v", err)
	}
}
