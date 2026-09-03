package harness

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"time"
)

// Install writes the planned files under home, removes files a previous
// install wrote that the plan no longer contains, and records the manifest.
// Writing is idempotent: an unchanged file is neither rewritten nor reported.
func Install(repo, home string, files []File, dryRun bool, out io.Writer) error {
	old, _, err := LoadState(home)
	if err != nil {
		return err
	}

	counts := map[string]int{}
	var manifest []string
	for _, f := range files {
		dest := filepath.Join(home, f.Rel)
		existing, readErr := os.ReadFile(dest)
		switch {
		case f.KeepForeign && readErr == nil && !IsOurs(existing) && !bytes.Equal(existing, f.Data):
			counts["kept"]++
			fmt.Fprintf(out, "keep    ~/%s (not written by sh-agent; add the line `@AGENTS.md` to it yourself)\n", f.Rel)
			continue
		case readErr == nil && bytes.Equal(existing, f.Data):
			counts["unchanged"]++
		default:
			counts["written"]++
			if dryRun {
				fmt.Fprintf(out, "write   ~/%s\n", f.Rel)
			} else if err := writeAtomic(dest, f.Data, f.Mode); err != nil {
				return fmt.Errorf("writing %s: %w", dest, err)
			}
		}
		manifest = append(manifest, f.Rel)
	}

	// Resync: anything the previous install wrote that this one does not is
	// stale — a renamed skill, a deleted agent — and goes away, so the
	// deployed set is always exactly the repo's set.
	planned := map[string]bool{}
	for _, rel := range manifest {
		planned[rel] = true
	}
	for _, rel := range old.Files {
		if planned[rel] {
			continue
		}
		dest := filepath.Join(home, rel)
		if _, err := os.Lstat(dest); err != nil {
			continue
		}
		counts["removed"]++
		if dryRun {
			fmt.Fprintf(out, "remove  ~/%s (no longer in the repo)\n", rel)
			continue
		}
		if err := os.Remove(dest); err != nil {
			return fmt.Errorf("removing stale %s: %w", dest, err)
		}
		pruneEmptyDirs(home, filepath.Dir(dest))
	}

	sort.Strings(manifest)
	commit, _ := gitOut(repo, "rev-parse", "--short", "HEAD")
	st := State{Repo: repo, Commit: commit, InstalledAt: time.Now().Format(time.RFC3339), Files: manifest}
	if !dryRun {
		if err := SaveState(home, st); err != nil {
			return fmt.Errorf("saving manifest: %w", err)
		}
	}

	prefix := ""
	if dryRun {
		prefix = "[dry-run] "
	}
	fmt.Fprintf(out, "%s%d written, %d unchanged, %d removed, %d kept — %d files managed\n",
		prefix, counts["written"], counts["unchanged"], counts["removed"], counts["kept"], len(manifest))
	return nil
}

// Entry is the state of one deployed path.
type Entry struct {
	Rel   string
	State string // ok | missing | differs | foreign | stale
}

// Report is what `sh-agent status` prints.
type Report struct {
	Entries []Entry
	Counts  map[string]int
}

// Drift reports whether anything differs from the plan.
func (r Report) Drift() bool {
	return r.Counts["missing"]+r.Counts["differs"]+r.Counts["stale"] > 0
}

// Status compares the plan against home without changing anything.
func Status(home string, files []File) (Report, error) {
	st, _, err := LoadState(home)
	if err != nil {
		return Report{}, err
	}
	r := Report{Counts: map[string]int{}}
	planned := map[string]bool{}
	for _, f := range files {
		planned[f.Rel] = true
		existing, err := os.ReadFile(filepath.Join(home, f.Rel))
		state := "ok"
		switch {
		case errors.Is(err, os.ErrNotExist):
			state = "missing"
		case err != nil:
			return Report{}, err
		case bytes.Equal(existing, f.Data):
		case f.KeepForeign && !IsOurs(existing):
			state = "foreign"
		default:
			state = "differs"
		}
		r.Counts[state]++
		r.Entries = append(r.Entries, Entry{Rel: f.Rel, State: state})
	}
	for _, rel := range st.Files {
		// The binary is compared by no one: status and diff plan without it,
		// since its bytes depend on the toolchain, not on the repo.
		if planned[rel] || rel == BinPath {
			continue
		}
		if _, err := os.Lstat(filepath.Join(home, rel)); err == nil {
			r.Counts["stale"]++
			r.Entries = append(r.Entries, Entry{Rel: rel, State: "stale"})
		}
	}
	return r, nil
}

// Diff prints a unified diff for every file that differs, using the system
// diff so the output looks like every other diff the operator reads.
func Diff(home string, files []File, out io.Writer) error {
	for _, f := range files {
		dest := filepath.Join(home, f.Rel)
		existing, err := os.ReadFile(dest)
		if errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(out, "missing ~/%s\n", f.Rel)
			continue
		}
		if err != nil {
			return err
		}
		if bytes.Equal(existing, f.Data) || (f.KeepForeign && !IsOurs(existing)) {
			continue
		}
		tmp, err := os.CreateTemp("", "sh-agent-diff-*")
		if err != nil {
			return err
		}
		if _, err := tmp.Write(f.Data); err != nil {
			return err
		}
		tmp.Close()
		cmd := exec.Command("diff", "-u", "-L", "installed ~/"+f.Rel, "-L", "repo ~/"+f.Rel, dest, tmp.Name())
		cmd.Stdout, cmd.Stderr = out, out
		if err := cmd.Run(); err != nil {
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() > 1 {
				os.Remove(tmp.Name())
				return fmt.Errorf("diff %s: %w", f.Rel, err)
			}
		}
		os.Remove(tmp.Name())
	}
	return nil
}

// Uninstall removes every path in the manifest and nothing else, then the
// manifest itself. Directories left empty are removed on the way up.
func Uninstall(home string, dryRun bool, out io.Writer) error {
	st, found, err := LoadState(home)
	if err != nil {
		return err
	}
	if !found {
		fmt.Fprintln(out, "nothing installed (no manifest at ~/.config/sh-agent/state.json)")
		return nil
	}
	removed := 0
	for _, rel := range st.Files {
		dest := filepath.Join(home, rel)
		if _, err := os.Lstat(dest); err != nil {
			continue
		}
		removed++
		if dryRun {
			fmt.Fprintf(out, "remove  ~/%s\n", rel)
			continue
		}
		if err := os.Remove(dest); err != nil {
			return fmt.Errorf("removing %s: %w", dest, err)
		}
		pruneEmptyDirs(home, filepath.Dir(dest))
	}
	if dryRun {
		fmt.Fprintf(out, "[dry-run] would remove %d files and the manifest\n", removed)
		return nil
	}
	if err := RemoveState(home); err != nil {
		return err
	}
	fmt.Fprintf(out, "removed %d files and the manifest\n", removed)
	return nil
}

// writeAtomic writes through a temp file and rename, so a half-written
// context file is never visible and a running sh-agent binary can be
// replaced on Linux, where opening it for writing would fail with ETXTBSY.
func writeAtomic(dest string, data []byte, mode fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dest), ".sh-agent-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, dest); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}

// pruneEmptyDirs removes now-empty directories from dir upward, stopping at
// the vendor roots and anything above them so uninstall never removes
// ~/.claude or ~/.gemini/config themselves.
func pruneEmptyDirs(home, dir string) {
	stop := map[string]bool{
		home:                                       true,
		filepath.Join(home, ClaudeRoot):            true,
		filepath.Join(home, ".gemini"):             true,
		filepath.Join(home, AgyRoot):               true,
		filepath.Join(home, ".config"):             true,
		filepath.Join(home, ".local"):              true,
		filepath.Join(home, filepath.Dir(BinPath)): true,
	}
	for !stop[dir] && len(dir) > len(home) {
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) > 0 {
			return
		}
		if err := os.Remove(dir); err != nil {
			return
		}
		dir = filepath.Dir(dir)
	}
}
