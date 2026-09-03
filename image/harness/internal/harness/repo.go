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
// recorded — so `sh-agent update` works from anywhere once installed.
func FindRepo(explicit, home string) (string, error) {
	candidates := []string{explicit, os.Getenv("SH_AGENT_REPO")}
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

// Inspect reads branch, commit, and cleanliness. A directory that is not a
// git checkout yields empty fields rather than an error: the harness works
// from a plain copy too, only `update` needs git.
func Inspect(repo string) RepoInfo {
	branch, _ := gitOut(repo, "rev-parse", "--abbrev-ref", "HEAD")
	commit, _ := gitOut(repo, "rev-parse", "--short", "HEAD")
	status, _ := gitOut(repo, "status", "--porcelain")
	return RepoInfo{Branch: branch, Commit: commit, Dirty: status != ""}
}

// Behind fetches the upstream and reports how many commits HEAD lags it.
func Behind(repo string) (int, error) {
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
// uncommitted edits to the harness is how local changes get lost.
func Pull(repo string, out io.Writer) error {
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

// Reinstall rebuilds the binary from the freshly pulled source and runs its
// install, so a code change in the repo takes effect in the same update.
func Reinstall(repo string, args []string, out io.Writer) error {
	if _, err := exec.LookPath("go"); err != nil {
		return errors.New("go is not on PATH; install Go to rebuild sh-agent")
	}
	cmd := exec.Command("go", append([]string{"run", "./cmd/sh-agent", "install"}, args...)...)
	cmd.Dir = repo
	cmd.Stdout, cmd.Stderr = out, out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go run ./cmd/sh-agent install: %w", err)
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
