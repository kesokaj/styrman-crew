package project

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The project's size (ADR-083): round-trips through a fresh Store, is absent
// from the bytes while all three are zero, is a no-op when unchanged, and
// rolls back when the save fails.
func TestSetResourcesRoundTrip(t *testing.T) {
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
	for _, k := range []string{"cpus", "memoryGiB", "diskGB"} {
		if strings.Contains(string(raw), k) {
			t.Fatalf("%s serialised while zero: %s", k, raw)
		}
	}
	if err := s.SetResources(p.ID, 2, 4, 25); err != nil {
		t.Fatal(err)
	}
	s2, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := s2.Get(p.ID); got.CPUs != 2 || got.MemoryGiB != 4 || got.DiskGB != 25 {
		t.Fatalf("after a reopen: %+v", got)
	}
	// Unlimited again: CPUs and memory drop out of the file.
	if err := s.SetResources(p.ID, 0, 0, 25); err != nil {
		t.Fatal(err)
	}
	raw, _ = os.ReadFile(path)
	if strings.Contains(string(raw), `"cpus"`) || strings.Contains(string(raw), "memoryGiB") || !strings.Contains(string(raw), `"diskGB": 25`) {
		t.Fatalf("after unlimited: %s", raw)
	}
}

func TestSetResourcesUnchangedDoesNotWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "projects.json")
	s, _ := NewStore(path)
	p, _ := s.Add(Project{Name: "alpha", CreatedAt: time.Now().UTC(), DiskGB: 20})
	// A directory where the file was: any write would fail.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := s.SetResources(p.ID, 0, 0, 20); err != nil {
		t.Fatalf("an unchanged size wrote the file: %v", err)
	}
}

func TestSetResourcesRollsBackOnSaveFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "projects.json")
	s, _ := NewStore(path)
	p, _ := s.Add(Project{Name: "alpha", CreatedAt: time.Now().UTC(), DiskGB: 20})
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := s.SetResources(p.ID, 2, 4, 30); err == nil {
		t.Fatal("save should have failed")
	}
	if got, _ := s.Get(p.ID); got.CPUs != 0 || got.MemoryGiB != 0 || got.DiskGB != 20 {
		t.Fatalf("memory must match disk after a failed save: %+v", got)
	}
}

func TestSetResourcesUnknownProject(t *testing.T) {
	s, _ := NewStore(filepath.Join(t.TempDir(), "projects.json"))
	if err := s.SetResources("nope", 1, 1, 20); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// A record from before ADR-083 has none of the three fields: it loads as
// unlimited with a disk of 0 ("keep what the container has").
func TestPreADR083RecordLoadsAsUnlimited(t *testing.T) {
	path := filepath.Join(t.TempDir(), "projects.json")
	const id = "3e6a772c-b177-4cfc-b209-b14db514bac2"
	if err := os.WriteFile(path, []byte(`{"projects":[{"id":"`+id+`","name":"alpha","dir":"alpha","createdAt":"2026-09-08T12:00:00Z"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Get(id); got.CPUs != 0 || got.MemoryGiB != 0 || got.DiskGB != 0 {
		t.Fatalf("legacy record: %+v", got)
	}
}
