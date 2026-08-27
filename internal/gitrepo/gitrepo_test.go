package gitrepo

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, ws, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filepath.Join(ws, name)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestEnsureRepoIdempotent(t *testing.T) {
	ws := t.TempDir()
	if IsRepo(ws) {
		t.Fatal("fresh dir must not be a repo")
	}
	for i := 0; i < 2; i++ {
		if err := EnsureRepo(ws); err != nil {
			t.Fatalf("EnsureRepo #%d: %v", i+1, err)
		}
	}
	if !IsRepo(ws) {
		t.Fatal("repo not created")
	}
	// HEAD is unborn until the user's first commit, but must point at main.
	if ref, err := git(context.Background(), ws, nil, "symbolic-ref", "HEAD"); err != nil || ref != "refs/heads/main" {
		t.Fatalf("HEAD = %q, err %v", ref, err)
	}
}

func TestSnapshotAndDedup(t *testing.T) {
	ws := t.TempDir()
	write(t, ws, "a.txt", "one")
	id1, created, err := Snapshot(context.Background(), ws, "first")
	if err != nil || !created {
		t.Fatalf("first snapshot: id=%q created=%v err=%v", id1, created, err)
	}
	// Identical tree → no new checkpoint.
	id2, created, err := Snapshot(context.Background(), ws, "dup")
	if err != nil || created || id2 != id1 {
		t.Fatalf("dedup failed: id=%q created=%v err=%v", id2, created, err)
	}
	write(t, ws, "a.txt", "two")
	id3, created, err := Snapshot(context.Background(), ws, "second")
	if err != nil || !created || id3 == id1 {
		t.Fatalf("changed tree must snapshot: id=%q created=%v err=%v", id3, created, err)
	}
	cps, err := List(ws)
	if err != nil || len(cps) != 2 {
		t.Fatalf("want 2 checkpoints, got %d (%v)", len(cps), err)
	}
	if cps[0].ID != id3 {
		t.Fatalf("newest first: got %q want %q", cps[0].ID, id3)
	}
	if cps[0].Subject != "second" {
		t.Fatalf("subject: got %q", cps[0].Subject)
	}
}

func TestSnapshotLeavesUserIndexAlone(t *testing.T) {
	ws := t.TempDir()
	write(t, ws, "a.txt", "one")
	if err := EnsureRepo(ws); err != nil {
		t.Fatal(err)
	}
	// User's index is empty (nothing staged); snapshot must keep it that way.
	if _, _, err := Snapshot(context.Background(), ws, "snap"); err != nil {
		t.Fatal(err)
	}
	out, err := git(context.Background(), ws, nil, "status", "--porcelain")
	if err != nil {
		t.Fatal(err)
	}
	if out != "?? a.txt" {
		t.Fatalf("user index disturbed; status = %q", out)
	}
}

func TestRestore(t *testing.T) {
	ws := t.TempDir()
	write(t, ws, "keep.txt", "v1")
	write(t, ws, "doomed.txt", "temporary")
	id, _, err := Snapshot(context.Background(), ws, "good state")
	if err != nil {
		t.Fatal(err)
	}
	// Mutate: change one file, delete one, add junk, add an ignored file.
	write(t, ws, "keep.txt", "v2 — agent damage")
	if err := os.Remove(filepath.Join(ws, "doomed.txt")); err != nil {
		t.Fatal(err)
	}
	write(t, ws, "junk.txt", "created after checkpoint")
	write(t, ws, ".gitignore", "secret.env\n")
	write(t, ws, "secret.env", "TOKEN=1")

	safety, err := Restore(context.Background(), ws, id)
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(ws, "keep.txt")); string(b) != "v1" {
		t.Fatalf("keep.txt not restored: %q", b)
	}
	if b, _ := os.ReadFile(filepath.Join(ws, "doomed.txt")); string(b) != "temporary" {
		t.Fatalf("deleted file not resurrected: %q", b)
	}
	if _, err := os.Stat(filepath.Join(ws, "junk.txt")); !os.IsNotExist(err) {
		t.Fatal("post-checkpoint file must be removed")
	}
	if _, err := os.Stat(filepath.Join(ws, "secret.env")); err != nil {
		t.Fatal("ignored files must survive a restore")
	}
	// The safety snapshot holds the pre-restore state — restore it back.
	if _, err := Restore(context.Background(), ws, safety); err != nil {
		t.Fatalf("restore safety: %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(ws, "keep.txt")); string(b) != "v2 — agent damage" {
		t.Fatalf("undo failed: %q", b)
	}
	if b, _ := os.ReadFile(filepath.Join(ws, "junk.txt")); string(b) != "created after checkpoint" {
		t.Fatalf("junk.txt not back: %q", b)
	}
}

func TestRestoreRejectsBadID(t *testing.T) {
	ws := t.TempDir()
	if err := EnsureRepo(ws); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"", "../../etc", "HEAD", "main", "20260101T000000Z-zzzz"} {
		if _, err := Restore(context.Background(), ws, id); !errors.Is(err, ErrBadID) {
			t.Fatalf("id %q: want ErrBadID, got %v", id, err)
		}
	}
	// Well-formed but absent id → ErrNotFound, distinct from ErrBadID.
	if _, err := Restore(context.Background(), ws, "20260101T000000.000000000Z-deadbeef"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("absent id: want ErrNotFound, got %v", err)
	}
}

func TestConcurrentSnapshots(t *testing.T) {
	ws := t.TempDir()
	write(t, ws, "a.txt", "one")
	if err := EnsureRepo(ws); err != nil {
		t.Fatal(err)
	}
	// Per-workspace serialization must let simultaneous snapshots all
	// succeed (no shared-index lock collision) and dedup to one checkpoint.
	const n = 6
	errc := make(chan error, n)
	for i := 0; i < n; i++ {
		go func() {
			_, _, err := Snapshot(context.Background(), ws, "concurrent")
			errc <- err
		}()
	}
	for i := 0; i < n; i++ {
		if err := <-errc; err != nil {
			t.Fatalf("concurrent snapshot failed: %v", err)
		}
	}
	cps, err := List(ws)
	if err != nil {
		t.Fatal(err)
	}
	if len(cps) != 1 {
		t.Fatalf("identical tree must dedup to 1 checkpoint, got %d", len(cps))
	}
}

func TestRestoreClearsNestedClone(t *testing.T) {
	ws := t.TempDir()
	write(t, ws, "a.txt", "one")
	id, _, err := Snapshot(context.Background(), ws, "before clone")
	if err != nil {
		t.Fatal(err)
	}
	// Simulate an agent cloning a helper repo into the workspace after the
	// checkpoint (a committed nested repo — what a real clone is); restore
	// must remove the whole dir (clean -ffd), not leave a husk.
	nested := filepath.Join(ws, "vendor")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := git(context.Background(), nested, nil, "init", "-b", "main"); err != nil {
		t.Fatal(err)
	}
	write(t, ws, "vendor/lib.txt", "third-party")
	if _, err := git(context.Background(), nested, nil, "add", "-A"); err != nil {
		t.Fatal(err)
	}
	if _, err := git(context.Background(), nested, []string{
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
	}, "commit", "-m", "vendored"); err != nil {
		t.Fatal(err)
	}
	if _, err := Restore(context.Background(), ws, id); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if _, err := os.Stat(nested); !os.IsNotExist(err) {
		t.Fatal("nested clone must be removed by restore")
	}
}

func TestCheckpointsInvisibleToUser(t *testing.T) {
	ws := t.TempDir()
	write(t, ws, "a.txt", "one")
	if _, _, err := Snapshot(context.Background(), ws, "snap"); err != nil {
		t.Fatal(err)
	}
	// No branches were created and HEAD stays unborn on main.
	branches, err := git(context.Background(), ws, nil, "branch", "--list")
	if err != nil {
		t.Fatal(err)
	}
	if branches != "" {
		t.Fatalf("snapshot leaked a branch: %q", branches)
	}
}
