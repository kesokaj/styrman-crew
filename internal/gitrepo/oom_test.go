package gitrepo

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

// The server runs at oom_score_adj -999 (ADR-085) and its children inherit
// it; the host-side git it runs against a guest-writable workspace must not
// keep that protection (security review of v1.42.1, high). setOOMScoreAdj is
// what git() applies to its child; a test process cannot LOWER a score, so it
// RAISES one to prove the write lands on the child, not on the caller.
func TestSetOOMScoreAdjWritesTheChild(t *testing.T) {
	cmd := exec.Command("sleep", "5")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	if err := setOOMScoreAdj(cmd.Process.Pid, 500); err != nil {
		t.Fatalf("setOOMScoreAdj: %v", err)
	}
	got, err := os.ReadFile("/proc/" + strconv.Itoa(cmd.Process.Pid) + "/oom_score_adj")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(got)) != "500" {
		t.Fatalf("child oom_score_adj = %q, want 500", got)
	}
	self, _ := os.ReadFile("/proc/self/oom_score_adj")
	if strings.TrimSpace(string(self)) == "500" {
		t.Fatal("the caller's own score changed")
	}
}

// git() still returns git's output after the reset is wired in.
func TestGitStillRunsWithTheReset(t *testing.T) {
	ws := t.TempDir()
	if err := EnsureRepo(context.Background(), ws); err != nil {
		t.Fatalf("EnsureRepo: %v", err)
	}
	if !IsRepo(context.Background(), ws) {
		t.Fatal("not a repo after EnsureRepo")
	}
}
