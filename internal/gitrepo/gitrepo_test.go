package gitrepo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureRepoIdempotent(t *testing.T) {
	ws := t.TempDir()
	if IsRepo(t.Context(), ws) {
		t.Fatal("fresh dir must not be a repo")
	}
	for i := 0; i < 2; i++ {
		if err := EnsureRepo(t.Context(), ws); err != nil {
			t.Fatalf("EnsureRepo #%d: %v", i+1, err)
		}
	}
	if !IsRepo(t.Context(), ws) {
		t.Fatal("repo not created")
	}
	// HEAD is unborn until the user's first commit, but must point at main.
	if ref, err := git(t.Context(), ws, "symbolic-ref", "HEAD"); err != nil || ref != "refs/heads/main" {
		t.Fatalf("HEAD = %q, err %v", ref, err)
	}
}

// TestHardenedGitIgnoresHostileRepoConfig proves the C1 fix: a repo whose
// .git/config sets an exec-capable key (here core.hooksPath / core.fsmonitor to
// a script that would drop a sentinel file) must NOT run that program when the
// host-side git helper touches the repo — the -c overrides win.
func TestHardenedGitIgnoresHostileRepoConfig(t *testing.T) {
	ws := t.TempDir()
	if err := EnsureRepo(t.Context(), ws); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(ws, "pwned")
	// A "hook"/"fsmonitor" that would create the sentinel if git ever ran it.
	evil := filepath.Join(ws, "evil.sh")
	if err := os.WriteFile(evil, []byte("#!/bin/sh\ntouch "+sentinel+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Write hostile keys straight into the repo config (as a guest could).
	for _, kv := range [][2]string{
		{"core.fsmonitor", evil},
		{"core.hooksPath", filepath.Dir(evil)},
	} {
		if _, err := git(t.Context(), ws, "config", "--local", kv[0], kv[1]); err != nil {
			t.Fatal(err)
		}
	}
	// Exercise the remaining host-side operations; none may run evil.sh.
	_ = IsRepo(t.Context(), ws)
	_ = EnsureRepo(t.Context(), ws)
	if _, err := os.Stat(sentinel); err == nil {
		t.Fatal("hostile repo config executed a program as the server — hardening failed")
	}
}
