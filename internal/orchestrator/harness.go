package orchestrator

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// Harness channel (ADR-073), the server side. The host's channel exports the
// newest verified styrman-harness tag under <HarnessDir>/<commit>/ and points
// <HarnessDir>/current at it; a project VM mounts that export read-only at the
// same guest paths the image's baked mirror uses, so the entrypoint's seed
// prefers the channel and falls back to the image when nothing is mounted.

const (
	harnessGuestRepo = "/styrman/harness"
	harnessGuestBin  = "/styrman/bin/sh-agent"
)

// harnessBinds returns the two read-only binds a project container mounts
// from the channel at dir, on the UNRESOLVED <dir>/current path: Docker
// re-resolves a stored bind source on every start, so a stopped VM picks up
// a swapped harness on its next start, as it picks up an image upgrade.
//
// Empty dir: the channel is disabled — nil, nil. Nothing at current (or no
// dir at all): absent — nil and an error wrapping fs.ErrNotExist; the image's
// baked harness seeds the project. Anything the server will not mount into
// every project VM — the content dir or the export not owned by root or the
// server, group/world-writable, the content dir or current not what a
// symlink-free channel leaves there, current resolving outside the content
// dir, the export missing repo/COMMIT or bin/sh-agent — is nil and an error
// that is NOT fs.ErrNotExist: untrusted, which the caller treats differently
// from absent (a refused channel never rebuilds a container).
func harnessBinds(dir string) ([]string, error) {
	if dir == "" {
		return nil, nil
	}
	src := filepath.Join(dir, "current")
	if _, err := os.Lstat(src); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("harness channel absent at %s: %w", src, err)
		}
		return nil, fmt.Errorf("harness channel: %w", err)
	}
	if err := harnessTrusted(dir, src); err != nil {
		return nil, err
	}
	return []string{src + "/repo:" + harnessGuestRepo + ":ro", src + "/bin/sh-agent:" + harnessGuestBin + ":ro"}, nil
}

// harnessTrusted is the server's half of the content dir's trust rules (the
// channel script asserts the same before it writes). The dir's parent,
// /var/lib/styrman, is operator-owned by design, so the dir ITSELF is the
// boundary: never a symlink, owned by root or the server, no group/world
// write; current a symlink resolving inside the dir to a directory under the
// same ownership and mode rules, holding repo/COMMIT and bin/sh-agent as
// regular files (a plain-copy export carries no symlinks at all).
func harnessTrusted(dir, src string) error {
	clean := filepath.Clean(dir)
	if fi, err := os.Lstat(clean); err != nil {
		return fmt.Errorf("harness channel: %w", err)
	} else if fi.Mode()&fs.ModeSymlink != 0 {
		return fmt.Errorf("harness channel: %s is a symlink — refusing", clean)
	}
	if err := ownedAndPrivate(clean); err != nil {
		return err
	}
	realDir, err := filepath.EvalSymlinks(clean) // the parents may be links (a tmp dir); the dir itself is not
	if err != nil {
		return fmt.Errorf("harness channel: %w", err)
	}
	if fi, err := os.Lstat(src); err != nil {
		return fmt.Errorf("harness channel: %w", err)
	} else if fi.Mode()&fs.ModeSymlink == 0 {
		return fmt.Errorf("harness channel: %s is not a symlink (a planted bind source?) — refusing", src)
	}
	target, err := filepath.EvalSymlinks(src)
	if err != nil {
		return fmt.Errorf("harness channel: %w", err)
	}
	if !strings.HasPrefix(target, realDir+string(filepath.Separator)) {
		return fmt.Errorf("harness channel: %s resolves outside %s (%s) — refusing", src, clean, target)
	}
	if err := ownedAndPrivate(target); err != nil {
		return err
	}
	for _, f := range []string{"repo/COMMIT", "bin/sh-agent"} {
		if st, err := os.Lstat(filepath.Join(target, f)); err != nil || !st.Mode().IsRegular() {
			return fmt.Errorf("harness channel: %s/%s is not a regular file — refusing an incomplete export", target, f)
		}
	}
	return nil
}

// ownedAndPrivate: p is a directory owned by root or the server's own uid,
// with neither the group nor the world write bit.
func ownedAndPrivate(p string) error {
	st, err := os.Lstat(p)
	if err != nil {
		return fmt.Errorf("harness channel: %w", err)
	}
	if !st.IsDir() {
		return fmt.Errorf("harness channel: %s is not a directory — refusing", p)
	}
	sys, ok := st.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("harness channel: cannot read the owner of %s — refusing", p)
	}
	if uid := int(sys.Uid); uid != 0 && uid != os.Getuid() {
		return fmt.Errorf("harness channel: %s is owned by uid %d, not root or the server — refusing", p, uid)
	}
	if st.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("harness channel: %s is group/world-writable (%04o) — refusing", p, st.Mode().Perm())
	}
	return nil
}

// hasHarnessBind reports whether a container's binds already mount the
// channel's repo (the two binds travel together).
func hasHarnessBind(binds []string) bool {
	for _, b := range binds {
		if strings.Contains(b, ":"+harnessGuestRepo+":") || strings.HasSuffix(b, ":"+harnessGuestRepo) {
			return true
		}
	}
	return false
}

// harnessChannelState is what the trusted channel currently delivers: the
// export's TAG and COMMIT. The error is harnessBinds's.
func harnessChannelState(dir string) (tag, commit string, err error) {
	if _, err := harnessBinds(dir); err != nil {
		return "", "", err
	}
	read := func(name string) string {
		b, _ := os.ReadFile(filepath.Join(dir, "current", "repo", name))
		return strings.TrimSpace(string(b))
	}
	return read("TAG"), read("COMMIT"), nil
}

// logHarnessChannel is the one line per server start that says where the
// harness every project seeds from comes from.
func logHarnessChannel(logger *slog.Logger, dir string) {
	if dir == "" {
		logger.Info("harness channel disabled (STYRMAN_HARNESS_DIR unset); the image's baked harness seeds projects")
		return
	}
	tag, commit, err := harnessChannelState(dir)
	switch {
	case err == nil:
		logger.Info("harness channel", "tag", tag, "commit", commit, "path", filepath.Join(dir, "current"))
	case errors.Is(err, fs.ErrNotExist):
		logger.Warn("harness channel absent; the image's baked harness seeds projects", "path", filepath.Join(dir, "current"))
	default:
		logger.Warn("harness channel refused; the image's baked harness seeds projects", "error", err)
	}
}
