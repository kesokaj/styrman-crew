package history

import (
	"github.com/kesokaj/styrman-crew/internal/files"
	"os"
	"path/filepath"
	"testing"
)

func TestSearch(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".styrman", "logs", "proj-1")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// ANSI-colored content: search must match the stripped text.
	os.WriteFile(filepath.Join(dir, "conv-1.log"),
		[]byte("$ \x1b[32mdocker\x1b[0m compose up\r\nneedle-alpha here\r\nplain line\r\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "conv-2.log"),
		[]byte("nothing to see\nNEEDLE-ALPHA uppercase\n"), 0o644)

	m, err := Search(t.Context(), home, "proj-1", "needle-alpha")
	if err != nil {
		t.Fatal(err)
	}
	if len(m) != 2 {
		t.Fatalf("matches = %d, want 2: %+v", len(m), m)
	}
	seen := map[string]bool{}
	for _, x := range m {
		seen[x.Transcript] = true
	}
	if !seen["conv-1"] || !seen["conv-2"] {
		t.Errorf("transcripts hit = %+v", m)
	}

	// Case-insensitive + ANSI-stripped match inside a colored line.
	m, _ = Search(t.Context(), home, "proj-1", "DOCKER compose")
	if len(m) != 1 || m[0].Line != "$ docker compose up" {
		t.Errorf("ansi-stripped match = %+v", m)
	}

	// Empty and missing-project queries are calm no-ops.
	if m, _ := Search(t.Context(), home, "proj-1", "  "); len(m) != 0 {
		t.Error("empty query should return nothing")
	}
	if m, _ := Search(t.Context(), home, "no-such", "x"); len(m) != 0 {
		t.Error("missing project should return nothing")
	}

	// A symlinked log escaping the home is never followed.
	outside := t.TempDir()
	os.WriteFile(filepath.Join(outside, "secret.log"), []byte("needle-alpha secret\n"), 0o644)
	os.Symlink(filepath.Join(outside, "secret.log"), filepath.Join(dir, "conv-3.log"))
	m, _ = Search(t.Context(), home, "proj-1", "needle-alpha")
	for _, x := range m {
		if x.Transcript == "conv-3" {
			t.Error("search followed a symlink escaping the home")
		}
	}
}

// Transcript listing runs through a root below a trusted tenant base in
// production (the project home under HomesDir). A sub-root's
// DirEntry.Info() fails under Go 1.25 and would drop every transcript
// (release e2e 2026-09-04, BUG-004): List must read sizes via the root.
func TestListBelowATrustedBase(t *testing.T) {
	base := t.TempDir()
	files.Trust(base)
	home := filepath.Join(base, "alice", "p", "11111111-1111-4111-8111-111111111111")
	logs := filepath.Join(home, ".styrman", "logs", "proj")
	if err := os.MkdirAll(logs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(logs, "conv-1.log"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := List(home, "proj")
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].ID != "conv-1" || out[0].Size != 5 {
		t.Fatalf("List below a trusted base = %+v, want one transcript conv-1 of size 5", out)
	}
}
