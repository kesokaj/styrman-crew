package harness

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// FindRepo locates the harness checkout: an explicit path, then $SH_AGENT_REPO,
// then the working directory and its parents, then the path the last install
// recorded — so `sh-agent update` works from anywhere once installed. A path
// the operator named is used or refused, never silently replaced by one the
// walk finds: install builds and runs the checkout's code, so a typo must
// not hand that to whatever harness-shaped tree the shell happens to be in.
func FindRepo(explicit, home string) (string, error) {
	for _, named := range []string{explicit, os.Getenv("SH_AGENT_REPO")} {
		if named == "" {
			continue
		}
		if !IsRepo(named) {
			return "", fmt.Errorf("%s is not a harness checkout (no context/AGENTS.md and skills/)", named)
		}
		return filepath.Abs(named)
	}
	var candidates []string
	if wd, err := os.Getwd(); err == nil {
		for dir := wd; ; dir = filepath.Dir(dir) {
			candidates = append(candidates, dir)
			if filepath.Dir(dir) == dir {
				break
			}
		}
	}
	if st, found, err := LoadState(home); err == nil && found {
		candidates = append(candidates, st.Repo)
	}
	for _, c := range candidates {
		if c != "" && IsRepo(c) {
			return filepath.Abs(c)
		}
	}
	return "", errors.New("harness repo not found: run from inside the checkout, pass --repo, or set SH_AGENT_REPO")
}

// IsRepo reports whether dir looks like the harness repo.
func IsRepo(dir string) bool {
	if _, err := os.Stat(filepath.Join(dir, "context", "AGENTS.md")); err != nil {
		return false
	}
	info, err := os.Stat(filepath.Join(dir, "skills"))
	return err == nil && info.IsDir()
}

// RepoInfo is the git state `sh-agent status` shows.
type RepoInfo struct {
	Branch string
	Commit string
	Dirty  bool
}

// errPlainCopy is returned by Behind and Pull for a tree git cannot update.
var errPlainCopy = errors.New("not its own git checkout; a plain copy cannot be updated with git")

// IsCheckout reports whether repo is the top of its own git checkout: a
// `.git` entry sits directly in it (a directory, or the file a worktree
// carries). A harness-shaped tree nested inside another repository (a mirror
// kept under review, a copy inside a project) has none and is a plain copy;
// asking git would answer for the enclosing repository, and git's answer
// can be redirected by its environment, so the check never asks git.
func IsCheckout(repo string) bool {
	_, err := os.Lstat(filepath.Join(repo, ".git"))
	return err == nil
}

// Inspect reads branch, commit, and cleanliness. A directory that is not its
// own git checkout yields empty fields rather than an error: the harness
// works from a plain copy too, only `update` needs git.
func Inspect(repo string) RepoInfo {
	if !IsCheckout(repo) {
		return RepoInfo{}
	}
	branch, _ := gitOut(repo, "rev-parse", "--abbrev-ref", "HEAD")
	commit, _ := gitOut(repo, "rev-parse", "--short", "HEAD")
	status, _ := gitOut(repo, "status", "--porcelain")
	return RepoInfo{Branch: branch, Commit: commit, Dirty: status != ""}
}

// Behind fetches the upstream and reports how many commits HEAD lags it.
func Behind(repo string) (int, error) {
	if !IsCheckout(repo) {
		return 0, errPlainCopy
	}
	if _, err := gitOut(repo, "fetch", "--quiet"); err != nil {
		return 0, fmt.Errorf("git fetch: %w", err)
	}
	n, err := gitOut(repo, "rev-list", "--count", "HEAD..@{upstream}")
	if err != nil {
		return 0, fmt.Errorf("no upstream for the current branch: %w", err)
	}
	return strconv.Atoi(n)
}

// Pull fast-forwards to upstream. It refuses a dirty tree: pulling over
// uncommitted edits to the harness is how local changes get lost. Git runs
// in the operator's own environment on purpose: GIT_* variables, git config,
// GIT_SSH_COMMAND and PATH are theirs to set, exactly as for a `git pull`
// typed by hand; only the plain-copy check above is kept independent of
// git so a typo or a nested copy can never redirect a build (security
// reviews of 2026-09-13, rounds 1–3).
func Pull(repo string, out io.Writer) error {
	if !IsCheckout(repo) {
		return errPlainCopy
	}
	if Inspect(repo).Dirty {
		return errors.New("the harness repo has uncommitted changes; commit or stash them before `sh-agent update`")
	}
	cmd := exec.Command("git", "-C", repo, "pull", "--ff-only", "--quiet")
	cmd.Stdout, cmd.Stderr = out, out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git pull --ff-only: %w", err)
	}
	return nil
}

// ChildEnv marks the process Reinstall spawns from a fresh build, so that
// process does the plain install instead of rebuilding again.
const ChildEnv = "SH_AGENT_CHILD"

// Reinstall builds the binary from the repo's source and runs that build's
// install. It is the only path that changes the deployed binary: `go run`
// leaves no VCS stamp, and an installed copy re-installing itself never
// changes (BUG-006). The build lands in the repo's scratch space, one
// directory per process, and is removed afterwards.
func Reinstall(repo string, args []string, out io.Writer) error {
	if _, err := exec.LookPath("go"); err != nil {
		return errors.New("go is not on PATH; install Go to rebuild sh-agent")
	}
	dir := filepath.Join(repo, ".agents", "scratch", fmt.Sprintf("sh-agent-build-%d", os.Getpid()))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating build directory: %w", err)
	}
	defer os.RemoveAll(dir)
	bin := filepath.Join(dir, "sh-agent")
	fmt.Fprintf(out, "building sh-agent from %s\n", repo)
	build := exec.Command("go", "build", "-o", bin, "./cmd/sh-agent")
	build.Dir = repo
	build.Stdout, build.Stderr = out, out
	if err := build.Run(); err != nil {
		return fmt.Errorf("go build ./cmd/sh-agent: %w", err)
	}
	cmd := exec.Command(bin, append([]string{"install", "--repo", repo}, args...)...)
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), ChildEnv+"=1")
	cmd.Stdout, cmd.Stderr = out, out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("sh-agent install: %w", err)
	}
	return nil
}

func gitOut(repo string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	data, err := cmd.Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(string(exit.Stderr)))
		}
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}
