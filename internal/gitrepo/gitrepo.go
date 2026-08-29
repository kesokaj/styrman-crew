// Package gitrepo makes git the substrate of every project workspace (R13):
// it ensures the working tree is a git repository at creation and re-creates
// one if it ever goes missing. Automatic checkpoints were removed (ADR-035;
// backups are microVM volume snapshots) and the interactive git client now
// runs INSIDE the guest VM (internal/httpapi/git.go), so nothing here reads a
// diff/status or writes a commit against the guest-controlled tree from the
// host — that host-side path was a guest→server code-execution vector.
//
// The two commands that remain (`git init`, `git rev-parse`) still run
// host-side against a bind-mounted, guest-writable workspace, so they are
// hardened to neutralize config-driven code execution (fsmonitor, hooks, the
// ext protocol): a repo whose .git/config an attacker controls must never make
// git exec a program as the server. `git init` additionally runs only when no
// repository — hence no attacker-controlled config — exists yet.
package gitrepo

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// hardened are -c overrides that stop a hostile repo-local config from making
// git exec an arbitrary program. The working tree is bind-mounted into an
// untrusted microVM (guest root), so its .git/config is attacker-controlled;
// these win over the repo's own values for every exec-capable key.
var hardened = []string{
	"-c", "core.fsmonitor=",
	"-c", "core.hooksPath=/dev/null",
	"-c", "core.pager=cat",
	"-c", "core.editor=false",
	"-c", "protocol.ext.allow=never",
	"-c", "uploadpack.packObjectsHook=",
}

func git(ctx context.Context, ws string, args ...string) (string, error) {
	full := append(append([]string{"-C", ws}, hardened...), args...)
	cmd := exec.CommandContext(ctx, "git", full...)
	// Containers see a foreign uid on some mounts; git refuses those without
	// safe.directory. The path is ours by construction (ADR-027). Ignore any
	// host global/system config so only the -c overrides above and the
	// (untrusted) repo config apply — and the overrides win where it matters.
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=safe.directory", "GIT_CONFIG_VALUE_0=*",
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		// The trimmed output can contain host paths, so callers must not put it
		// into an API response verbatim.
		return "", fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

// gitDir returns the resolved git directory for the workspace, or an error if
// it is not a repository. Handles .git being a file (worktrees) as well.
func gitDir(ctx context.Context, ws string) (string, error) {
	return git(ctx, ws, "rev-parse", "--absolute-git-dir")
}

// IsRepo reports whether the workspace is inside a git repository.
func IsRepo(ctx context.Context, ws string) bool {
	if _, err := os.Stat(ws); err != nil {
		return false
	}
	_, err := gitDir(ctx, ws)
	return err == nil
}

// EnsureRepo initializes a repository at the workspace root if none exists —
// the R13 invariant. Idempotent; never touches an existing repo (so it never
// runs against an attacker-controlled config).
func EnsureRepo(ctx context.Context, ws string) error {
	if IsRepo(ctx, ws) {
		return nil
	}
	_, err := git(ctx, ws, "init", "-b", "main")
	return err
}
