package history

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// A guest can write anywhere under its own ~/.styrman/logs, so it can replace
// a transcript — or the logs directory itself — with a FIFO. Without
// O_NONBLOCK, open(2) on a FIFO with no writer blocks forever, pinning the
// serving goroutine AND its OS thread; enough of them walk the runtime into
// its thread limit and take every tenant down with the process.
//
// internal/files set O_NONBLOCK for exactly this reason and said so in a
// comment; these three call sites did not (audit 2026-09-08). Each case below
// hangs indefinitely without the fix, so the timeout IS the assertion.
func mkfifo(t *testing.T, path string) {
	t.Helper()
	if err := syscall.Mkfifo(path, 0o644); err != nil {
		t.Skipf("mkfifo unavailable: %v", err)
	}
}

// withinDeadline runs fn and fails if it has not returned in time.
func withinDeadline(t *testing.T, what string, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() { defer close(done); fn() }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("%s blocked on a guest-planted FIFO — O_NONBLOCK is missing", what)
	}
}

func TestReadDoesNotBlockOnAPlantedFIFO(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".styrman", "logs", "proj-1")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	mkfifo(t, filepath.Join(dir, "conv-1.log"))

	withinDeadline(t, "history.Read", func() { _, _ = Read(home, "proj-1", "conv-1", 1<<20) })
}

func TestSearchDoesNotBlockOnAPlantedFIFO(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".styrman", "logs", "proj-1")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// One real transcript so the walk has something to find, and one FIFO.
	if err := os.WriteFile(filepath.Join(dir, "conv-1.log"), []byte("needle here\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mkfifo(t, filepath.Join(dir, "conv-2.log"))

	withinDeadline(t, "history.Search", func() { _, _ = Search(t.Context(), home, "proj-1", "needle") })
}

// The logs directory itself replaced by a FIFO — List opens that path.
func TestListDoesNotBlockOnAPlantedFIFO(t *testing.T) {
	home := t.TempDir()
	parent := filepath.Join(home, ".styrman", "logs")
	if err := os.MkdirAll(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	mkfifo(t, filepath.Join(parent, "proj-1"))

	withinDeadline(t, "history.List", func() { _, _ = List(home, "proj-1") })
}
