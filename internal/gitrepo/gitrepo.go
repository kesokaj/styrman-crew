// Package gitrepo makes git the substrate of every project workspace
// (R13): initialization, and checkpoints — automatic snapshots of the
// working tree stored under refs/styrman/checkpoints/*. Checkpoints never
// touch the user's branches and are never pushed by a plain `git push`
// (git only pushes refs/heads and refs/tags by default), so they stay out
// of normal history while keeping the tracked + untracked working tree
// restorable. They do NOT capture .git internals (branch tips, stash,
// in-progress rebase/merge state) or ignored files — only what's in the
// tree. `git log --all` and `git push --mirror` can still surface them.
//
// Everything runs host-side against the mounted home: it works whether or
// not the project's microVM is running, using a throwaway private index
// file so the user's real index and staging area are never disturbed.
package gitrepo

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// Sentinel errors so handlers can map to the right HTTP status.
var (
	ErrBadID    = errors.New("invalid checkpoint id")
	ErrNotFound = errors.New("checkpoint not found")
)

// keepCheckpoints bounds the per-project checkpoint ref count; the oldest
// are pruned on every new checkpoint. At the default triggers this is weeks
// of history.
const keepCheckpoints = 40

// idRe matches checkpoint ids as minted by Snapshot: a nanosecond UTC stamp
// (so ids sort chronologically even within one second) + short sha.
var idRe = regexp.MustCompile(`^[0-9]{8}T[0-9]{6}\.[0-9]{9}Z-[0-9a-f]{4,40}$`)

// wsLocks serializes snapshot/restore per workspace so an auto-snapshot
// can't capture a half-restored tree, and two snapshots can't race.
var (
	wsMu    sync.Mutex
	wsLocks = map[string]*sync.Mutex{}
)

func lockWorkspace(ws string) *sync.Mutex {
	wsMu.Lock()
	defer wsMu.Unlock()
	m, ok := wsLocks[ws]
	if !ok {
		m = &sync.Mutex{}
		wsLocks[ws] = m
	}
	return m
}

// git runs one git command in the workspace with the styrman-safe
// environment: safe.directory for foreign-uid mounts, and a neutral
// fallback identity so checkpoint commits work before the user has run
// styrman-setup. This identity applies only to commands run here — it never
// overrides the user's own gitconfig for their own commits.
func git(ctx context.Context, ws string, extraEnv []string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", ws}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=safe.directory", "GIT_CONFIG_VALUE_0=*",
		"GIT_AUTHOR_NAME=styrman", "GIT_AUTHOR_EMAIL=checkpoint@styrman.local",
		"GIT_COMMITTER_NAME=styrman", "GIT_COMMITTER_EMAIL=checkpoint@styrman.local",
	)
	cmd.Env = append(cmd.Env, extraEnv...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		// Callers log the detail; the trimmed output can contain host paths,
		// so it must not reach an API response verbatim.
		return "", fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

// gitDir returns the resolved git directory for the workspace, or an error
// if it is not a repository. Handles .git being a file (worktrees,
// submodules) as well as a directory.
func gitDir(ctx context.Context, ws string) (string, error) {
	out, err := git(ctx, ws, nil, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return "", err
	}
	return out, nil
}

// IsRepo reports whether the workspace is inside a git repository.
func IsRepo(ws string) bool {
	if _, err := os.Stat(ws); err != nil {
		return false
	}
	_, err := gitDir(context.Background(), ws)
	return err == nil
}

// EnsureRepo initializes a repository at the workspace root if none exists
// — the R13 invariant. Idempotent; never touches an existing repo.
func EnsureRepo(ws string) error {
	if IsRepo(ws) {
		return nil
	}
	_, err := git(context.Background(), ws, nil, "init", "-b", "main")
	return err
}

// Checkpoint is one snapshot of the working tree.
type Checkpoint struct {
	ID      string `json:"id"`
	Time    string `json:"time"`
	Subject string `json:"subject"`
}

// tempIndex returns a unique private index path and a cleanup func. Unique
// per call so concurrent snapshots never collide and a killed process can't
// leave a stale lock that permanently wedges checkpoints.
func tempIndex(ctx context.Context, ws string) (env []string, cleanup func(), err error) {
	dir, err := gitDir(ctx, ws)
	if err != nil {
		return nil, func() {}, err
	}
	f, err := os.CreateTemp(dir, "styrman-index-*")
	if err != nil {
		return nil, func() {}, err
	}
	path := f.Name()
	_ = f.Close()
	_ = os.Remove(path) // git wants to create it itself; we only reserved the name
	return []string{"GIT_INDEX_FILE=" + path},
		func() { _, _ = os.Stat(path); os.Remove(path); os.Remove(path + ".lock") },
		nil
}

// Snapshot records the current working tree (tracked + untracked, minus
// ignored files) as a checkpoint. Returns the checkpoint id; created is
// false when the tree is identical to the newest checkpoint, in which case
// that one's id is returned and nothing new is written. Serialized per
// workspace.
func Snapshot(ctx context.Context, ws, label string) (id string, created bool, err error) {
	if err := EnsureRepo(ws); err != nil {
		return "", false, err
	}
	l := lockWorkspace(ws)
	l.Lock()
	defer l.Unlock()
	return snapshotLocked(ctx, ws, label)
}

func snapshotLocked(ctx context.Context, ws, label string) (id string, created bool, err error) {
	idx, cleanup, err := tempIndex(ctx, ws)
	if err != nil {
		return "", false, err
	}
	defer cleanup()
	if _, err := git(ctx, ws, idx, "add", "-A"); err != nil {
		return "", false, err
	}
	tree, err := git(ctx, ws, idx, "write-tree")
	if err != nil {
		return "", false, err
	}
	existing, err := list(ctx, ws)
	if err != nil {
		return "", false, err
	}
	if len(existing) > 0 {
		lastTree, err := git(ctx, ws, nil, "rev-parse", refName(existing[0].ID)+"^{tree}")
		if err == nil && lastTree == tree {
			return existing[0].ID, false, nil
		}
	}
	label = sanitizeLabel(label)
	args := []string{"commit-tree", tree, "-m", "checkpoint: " + label}
	if head, err := git(ctx, ws, nil, "rev-parse", "--verify", "-q", "HEAD"); err == nil && head != "" {
		args = append(args, "-p", head)
	}
	sha, err := git(ctx, ws, nil, args...)
	if err != nil {
		return "", false, err
	}
	id = time.Now().UTC().Format("20060102T150405.000000000Z") + "-" + sha[:8]
	if _, err := git(ctx, ws, nil, "update-ref", refName(id), sha); err != nil {
		return "", false, err
	}
	prune(ctx, ws)
	return id, true, nil
}

func sanitizeLabel(label string) string {
	label = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' {
			return ' '
		}
		return r
	}, label)
	// Trim on a rune boundary so the commit subject stays valid UTF-8.
	for len(label) > 120 {
		_, size := utf8.DecodeLastRuneInString(label)
		label = label[:len(label)-size]
	}
	return label
}

func refName(id string) string { return "refs/styrman/checkpoints/" + id }

// List returns the project's checkpoints, newest first.
func List(ws string) ([]Checkpoint, error) {
	if !IsRepo(ws) {
		return []Checkpoint{}, nil
	}
	return list(context.Background(), ws)
}

func list(ctx context.Context, ws string) ([]Checkpoint, error) {
	out, err := git(ctx, ws, nil, "for-each-ref", "--sort=-refname",
		"--format=%(refname:lstrip=3)%09%(committerdate:iso-strict)%09%(subject)",
		"refs/styrman/checkpoints/")
	if err != nil {
		return nil, err
	}
	cps := []Checkpoint{}
	for _, line := range strings.Split(out, "\n") {
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) != 3 || !idRe.MatchString(parts[0]) {
			continue
		}
		cps = append(cps, Checkpoint{
			ID:      parts[0],
			Time:    parts[1],
			Subject: strings.TrimPrefix(parts[2], "checkpoint: "),
		})
	}
	sort.Slice(cps, func(i, j int) bool { return cps[i].ID > cps[j].ID })
	return cps, nil
}

// Restore rewrites the working tree to exactly match the checkpoint: files
// are overwritten, files that didn't exist then are deleted (including
// untracked directories and nested clones — clean -ffd; ignored files are
// left alone). The current state is snapshotted first, so a restore is
// itself undoable. Branches, HEAD, and commits are never touched; the
// user's real index (staging area) is NOT modified — a throwaway index
// drives the checkout. Serialized per workspace.
func Restore(ctx context.Context, ws, id string) (safetyID string, err error) {
	if !idRe.MatchString(id) {
		return "", ErrBadID
	}
	if !IsRepo(ws) {
		return "", ErrNotFound
	}
	l := lockWorkspace(ws)
	l.Lock()
	defer l.Unlock()

	if _, err := git(ctx, ws, nil, "rev-parse", "--verify", "-q", refName(id)); err != nil {
		return "", ErrNotFound
	}
	safetyID, _, err = snapshotLocked(ctx, ws, "before restoring "+id)
	if err != nil {
		return "", fmt.Errorf("pre-restore snapshot: %w", err)
	}
	idx, cleanup, err := tempIndex(ctx, ws)
	if err != nil {
		return safetyID, err
	}
	defer cleanup()
	// Populate the throwaway index from the checkpoint tree, then force the
	// working tree to match it — the user's .git/index is never touched.
	if _, err := git(ctx, ws, idx, "read-tree", "--reset", refName(id)); err != nil {
		return safetyID, err
	}
	if _, err := git(ctx, ws, idx, "checkout-index", "-f", "-a"); err != nil {
		return safetyID, err
	}
	// Remove files/dirs created after the checkpoint. clean must read the
	// SAME throwaway index (not the user's real, possibly-empty index) or it
	// treats the just-restored files as untracked and deletes them. -ffd
	// also clears nested clones; ignored files stay out of scope.
	if _, err := git(ctx, ws, idx, "clean", "-ffd"); err != nil {
		return safetyID, err
	}
	return safetyID, nil
}

// prune drops the oldest checkpoint refs beyond keepCheckpoints and lets git
// reclaim the now-unreachable objects. Best effort — failures only mean
// extra history/disk, never a broken checkpoint.
func prune(ctx context.Context, ws string) {
	cps, err := list(ctx, ws)
	if err != nil {
		return
	}
	pruned := false
	for i := keepCheckpoints; i < len(cps); i++ {
		if _, err := git(ctx, ws, nil, "update-ref", "-d", refName(cps[i].ID)); err == nil {
			pruned = true
		}
	}
	if pruned {
		// commit-tree/update-ref never trigger gc --auto, so snapshots would
		// otherwise accumulate loose objects forever.
		_, _ = git(ctx, ws, nil, "gc", "--auto", "--quiet")
	}
}
