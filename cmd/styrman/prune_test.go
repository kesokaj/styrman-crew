package main

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kesokaj/styrman-crew/internal/config"
)

// The janitor runs as root in production every six hours and deletes under
// <homes>/<u>/p/<uuid>/.styrman/logs. `p/<uuid>` sits inside the base home
// the setup VM mounts with a root guest, so a guest can replace it with a
// symlink: the janitor must prune a real project home and refuse a
// symlinked one (BUG-003).
func TestPruneTranscriptsRefusesASymlinkedProjectHome(t *testing.T) {
	homes := t.TempDir()
	cfg := &config.Config{HomesDir: homes, TranscriptTTL: time.Hour}
	old := time.Now().Add(-48 * time.Hour)
	plant := func(dir string) string {
		logs := filepath.Join(dir, ".styrman", "logs", "proj")
		if err := os.MkdirAll(logs, 0o755); err != nil {
			t.Fatal(err)
		}
		f := filepath.Join(logs, "conv-1.log")
		if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(f, old, old); err != nil {
			t.Fatal(err)
		}
		return f
	}
	real := plant(filepath.Join(homes, "alice", "p", "11111111-1111-4111-8111-111111111111"))
	// A "host" directory outside every home, reachable only through the link.
	host := t.TempDir()
	victim := plant(host)
	if err := os.Symlink(host, filepath.Join(homes, "alice", "p", "22222222-2222-4222-8222-222222222222")); err != nil {
		t.Fatal(err)
	}

	pruneTranscripts(cfg, slog.New(slog.DiscardHandler))

	if _, err := os.Stat(real); !os.IsNotExist(err) {
		t.Errorf("the real project home's old transcript should be pruned, stat err = %v", err)
	}
	if _, err := os.Stat(victim); err != nil {
		t.Errorf("a transcript behind a symlinked project home must NOT be touched: %v", err)
	}
}
