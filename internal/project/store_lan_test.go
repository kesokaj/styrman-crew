package project

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The LAN-access intent (ADR-065): round-trips through a fresh Store, is
// absent from the bytes while off, stamps LanChangedAt only on a real change,
// and rolls back when the save fails.
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
	if strings.Contains(string(raw), "lanAccess") || strings.Contains(string(raw), "lanChangedAt") {
		t.Fatalf("off is the default and must not be serialised: %s", raw)
	}

	t1 := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	if err := s.SetLanAccess(p.ID, true, t1); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Get(p.ID)
	if !got.LanAccess || !got.LanChangedAt.Equal(t1) {
		t.Fatalf("after enable: %+v", got)
	}
	// Idempotent re-enable must not move the clock (a repeated PUT re-kicks
	// the reconciler but does not restart the pending window).
	if err := s.SetLanAccess(p.ID, true, t1.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if got, _ = s.Get(p.ID); !got.LanChangedAt.Equal(t1) {
		t.Fatalf("repeated enable moved LanChangedAt: %v", got.LanChangedAt)
	}

	s2, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ = s2.Get(p.ID); !got.LanAccess || !got.LanChangedAt.Equal(t1) {
		t.Fatalf("reloaded: %+v", got)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Error("tmp file left behind")
	}

	t2 := t1.Add(2 * time.Hour)
	if err := s2.SetLanAccess(p.ID, false, t2); err != nil {
		t.Fatal(err)
	}
	if got, _ = s2.Get(p.ID); got.LanAccess || !got.LanChangedAt.Equal(t2) {
		t.Fatalf("after disable: %+v", got)
	}
	raw, _ = os.ReadFile(path)
	if strings.Contains(string(raw), `"lanAccess"`) {
		t.Fatalf("lanAccess false must be omitted: %s", raw)
	}
	if !strings.Contains(string(raw), "lanChangedAt") {
		t.Fatalf("lanChangedAt must persist after a disable so the closing state can be dated: %s", raw)
	}
	if err := s2.SetLanAccess("00000000-0000-4000-8000-000000000000", true, t2); err != ErrNotFound {
		t.Fatalf("unknown id: %v, want ErrNotFound", err)
	}
}

// RestoreLanAccess puts back an exact (value, timestamp) pair — the
// handler's rollback must not stamp "now" (review, 2026-09-08).
func TestRestoreLanAccessKeepsTheOldClock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "projects.json")
	s, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.Add(Project{Name: "alpha", CreatedAt: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	t1 := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	if err := s.SetLanAccess(p.ID, true, t1); err != nil {
		t.Fatal(err)
	}
	// A later change, then a rollback to the earlier pair.
	if err := s.SetLanAccess(p.ID, false, t1.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := s.RestoreLanAccess(p.ID, true, t1); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Get(p.ID)
	if !got.LanAccess || !got.LanChangedAt.Equal(t1) {
		t.Fatalf("restore must reinstate the exact pair: %+v", got)
	}
	// Idempotent and persisted.
	if err := s.RestoreLanAccess(p.ID, true, t1); err != nil {
		t.Fatal(err)
	}
	s2, _ := NewStore(path)
	if got, _ = s2.Get(p.ID); !got.LanAccess || !got.LanChangedAt.Equal(t1) {
		t.Fatalf("restore not persisted: %+v", got)
	}
	if err := s2.RestoreLanAccess("00000000-0000-4000-8000-000000000000", false, t1); err != ErrNotFound {
		t.Fatalf("unknown id: %v", err)
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
	if err := s.SetLanAccess(p.ID, true, time.Now()); err == nil {
		t.Fatal("save should have failed")
	}
	if got, _ := s.Get(p.ID); got.LanAccess || !got.LanChangedAt.IsZero() {
		t.Fatalf("memory must match disk after a failed save: %+v", got)
	}
}

// A hand-edited store with lanAccess:true and no timestamp gets a clock at
// load time, so pending→error can still be derived.
func TestLoadDatesUndatedLanAccess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "projects.json")
	f := storeFile{Projects: []Project{{
		ID: "3e6a772c-b177-4cfc-b209-b14db514bac2", Name: "alpha", Dir: "alpha",
		CreatedAt: time.Now().UTC(), LanAccess: true,
	}}}
	data, _ := json.Marshal(f)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := s.Get("3e6a772c-b177-4cfc-b209-b14db514bac2")
	if !got.LanAccess || got.LanChangedAt.IsZero() {
		t.Fatalf("undated lanAccess should be dated at load: %+v", got)
	}
}
