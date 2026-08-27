// Package files serves the file explorer: directory listings and file
// contents under a fixed root. The styrman server has the shared home
// mounted at the same path as the host, so this reads the very files the
// agents see inside the microVMs — no exec needed, works while a project
// is stopped.
//
// Every operation is confined with os.Root (Go 1.24+): the root must be a
// real directory (the user's home, created 0700 by the server), and traversal
// refuses any symlink that would leave the root — atomically, at every path
// component, with no check-then-use window. A guest running as the same uid
// can symlink freely inside its own home but cannot reach another tenant's
// home or the state dir. This is the tenancy boundary; callers pass the user
// home as root and address files by home-relative path.
package files

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"syscall"
	"unicode/utf8"
)

// MaxReadBytes caps file reads served to the browser.
const MaxReadBytes = 2 << 20 // 2 MiB

// Entry is one directory entry.
type Entry struct {
	Name     string `json:"name"`
	Path     string `json:"path"` // root-relative, slash-separated
	Dir      bool   `json:"dir"`
	Size     int64  `json:"size"`
	Modified string `json:"modified"`
	Symlink  bool   `json:"symlink"`
}

// cleanRel normalizes a root-relative path to something os.Root accepts,
// collapsing "" to "." (the root itself).
func cleanRel(rel string) string {
	rel = strings.TrimPrefix(path.Clean("/"+strings.TrimSpace(rel)), "/")
	if rel == "" {
		return "."
	}
	return rel
}

// openRoot opens a confined root. The root itself is opened following
// symlinks (it is server-controlled and guaranteed real); everything
// underneath is symlink-confined.
func openRoot(root string) (*os.Root, error) {
	return os.OpenRoot(root)
}

// openReg opens rel for reading and refuses anything that is not a regular
// file. O_NONBLOCK means a guest-planted FIFO/device returns immediately
// instead of wedging the goroutine forever (a cross-tenant DoS); the flag is
// a no-op for regular-file reads. Caller closes the returned file.
func openReg(rt *os.Root, rel string) (*os.File, os.FileInfo, error) {
	f, err := rt.OpenFile(rel, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, nil, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, nil, err
	}
	if !st.Mode().IsRegular() {
		f.Close()
		return nil, nil, fmt.Errorf("not a regular file")
	}
	return f, st, nil
}

// ContainedPath resolves rel under root and returns the real (symlink-
// evaluated) absolute path, erroring if it escapes root. For callers that
// need a filesystem PATH rather than a handle — chiefly exec-based tools
// like git (`git -C <path>`) that cannot use an os.Root handle. Direct I/O
// should use the confined helpers above (List/Read/Write/…) instead, which
// have no check-then-use window.
func ContainedPath(root, rel string) (string, error) {
	rel = strings.TrimPrefix(path.Clean("/"+strings.TrimSpace(rel)), "/")
	full := filepath.Join(root, rel)
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	realFull, err := filepath.EvalSymlinks(full)
	if err != nil {
		realParent, perr := filepath.EvalSymlinks(filepath.Dir(full))
		if perr != nil {
			return "", err
		}
		realFull = filepath.Join(realParent, filepath.Base(full))
	}
	if realFull != realRoot && !strings.HasPrefix(realFull, realRoot+string(os.PathSeparator)) {
		return "", fmt.Errorf("path escapes root")
	}
	return full, nil
}

// List returns the entries in the directory at rel, directories first.
// Dotfiles are included — this is a developer's home, they matter.
func List(root, rel string) ([]Entry, error) {
	rt, err := openRoot(root)
	if err != nil {
		return nil, err
	}
	defer rt.Close()
	rel = cleanRel(rel)
	d, err := rt.Open(rel)
	if err != nil {
		return nil, err
	}
	defer d.Close()
	des, err := d.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	base := ""
	if rel != "." {
		base = rel
	}
	out := make([]Entry, 0, len(des))
	for _, de := range des {
		info, err := de.Info()
		if err != nil {
			continue
		}
		isDir := de.IsDir()
		symlink := de.Type()&os.ModeSymlink != 0
		if symlink {
			// Report symlinked directories as directories so they expand —
			// but only if the target stays inside the root (Stat is confined).
			if st, err := rt.Stat(path.Join(rel, de.Name())); err == nil {
				isDir = st.IsDir()
			}
		}
		out = append(out, Entry{
			Name:     de.Name(),
			Path:     path.Join(base, de.Name()),
			Dir:      isDir,
			Size:     info.Size(),
			Modified: info.ModTime().UTC().Format("2006-01-02 15:04"),
			Symlink:  symlink,
		})
	}
	sortEntries(out)
	return out, nil
}

func sortEntries(out []Entry) {
	// directories first, then case-insensitive by name
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && less(out[j], out[j-1]); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
}

func less(a, b Entry) bool {
	if a.Dir != b.Dir {
		return a.Dir
	}
	return strings.ToLower(a.Name) < strings.ToLower(b.Name)
}

// FileContent is a file read for the viewer.
type FileContent struct {
	Path      string `json:"path"`
	Content   string `json:"content"`
	Size      int64  `json:"size"`
	Truncated bool   `json:"truncated"`
	Binary    bool   `json:"binary"`
}

// Read returns a file's contents, capped and binary-safe.
func Read(root, rel string) (*FileContent, error) {
	rt, err := openRoot(root)
	if err != nil {
		return nil, err
	}
	defer rt.Close()
	f, st, err := openReg(rt, cleanRel(rel))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	buf := make([]byte, MaxReadBytes)
	n, _ := io.ReadFull(f, buf)
	buf = buf[:n]
	fc := &FileContent{
		Path:      strings.TrimPrefix(rel, "/"),
		Size:      st.Size(),
		Truncated: st.Size() > int64(n),
	}
	if !utf8.Valid(buf) || strings.IndexByte(string(buf), 0) >= 0 {
		fc.Binary = true
		return fc, nil
	}
	fc.Content = string(buf)
	return fc, nil
}

// Write saves content to a file, creating it if needed (confined to root).
func Write(root, rel, content string) error {
	rt, err := openRoot(root)
	if err != nil {
		return err
	}
	defer rt.Close()
	crel := cleanRel(rel)
	if st, err := rt.Stat(crel); err == nil && st.IsDir() {
		return fmt.Errorf("path is a directory")
	}
	f, err := rt.OpenFile(crel, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(content)
	return err
}

// SaveStream writes an io.Reader to rel (confined), used by uploads.
func SaveStream(root, rel string, src io.Reader) error {
	rt, err := openRoot(root)
	if err != nil {
		return err
	}
	defer rt.Close()
	f, err := rt.OpenFile(cleanRel(rel), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, src)
	return err
}

// OpenForDownload returns a confined, seekable file handle plus its info for
// streaming; caller must Close the returned file.
func OpenForDownload(root, rel string) (*os.File, os.FileInfo, error) {
	rt, err := openRoot(root)
	if err != nil {
		return nil, nil, err
	}
	// The *os.File outlives rt; that's fine — the fd is already resolved and
	// confined by the time Open returns.
	defer rt.Close()
	return openReg(rt, cleanRel(rel))
}

// ReadRaw returns up to max bytes of a file (0 = all up to MaxReadBytes),
// confined to root. For small config reads (setup/tool details).
func ReadRaw(root, rel string, max int64) ([]byte, error) {
	rt, err := openRoot(root)
	if err != nil {
		return nil, err
	}
	defer rt.Close()
	f, _, err := openReg(rt, cleanRel(rel))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if max <= 0 || max > MaxReadBytes {
		max = MaxReadBytes
	}
	return io.ReadAll(io.LimitReader(f, max))
}

// ListNames returns the entry names in a directory (confined), or nil if it
// does not exist. For enumerating skills/subagents/commands dirs.
func ListNames(root, rel string) []string {
	rt, err := openRoot(root)
	if err != nil {
		return nil
	}
	defer rt.Close()
	d, err := rt.Open(cleanRel(rel))
	if err != nil {
		return nil
	}
	defer d.Close()
	des, err := d.ReadDir(-1)
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(des))
	for _, de := range des {
		names = append(names, de.Name())
	}
	return names
}

// Save writes data to rel under root, creating parent directories, all
// confined — a symlinked component (e.g. a marker planted at another
// tenant's file) is refused, not followed.
func Save(root, rel string, data []byte, perm os.FileMode) error {
	rt, err := openRoot(root)
	if err != nil {
		return err
	}
	defer rt.Close()
	crel := cleanRel(rel)
	if dir := path.Dir(crel); dir != "." {
		if err := rt.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	f, err := rt.OpenFile(crel, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(data)
	return err
}

// MkdirAll creates rel and any missing parents under root, confined — a
// symlinked component escaping the root is refused.
func MkdirAll(root, rel string) error {
	rt, err := openRoot(root)
	if err != nil {
		return err
	}
	defer rt.Close()
	return rt.MkdirAll(cleanRel(rel), 0o755)
}

// RemoveAll removes rel and its contents under root, confined — a symlinked
// component escaping the root is refused, so a wipe cannot recurse into
// another tenant's home.
func RemoveAll(root, rel string) error {
	rt, err := openRoot(root)
	if err != nil {
		return err
	}
	defer rt.Close()
	return rt.RemoveAll(cleanRel(rel))
}

// IsRegular reports whether rel is a regular file (not a FIFO/device/socket
// that would block or misbehave on Open), following symlinks confined to root.
func IsRegular(root, rel string) bool {
	rt, err := openRoot(root)
	if err != nil {
		return false
	}
	defer rt.Close()
	st, err := rt.Stat(cleanRel(rel))
	return err == nil && st.Mode().IsRegular()
}

// Exists reports whether rel exists under root (confined). Symlinks that
// escape the root count as not-existing.
func Exists(root, rel string) bool {
	rt, err := openRoot(root)
	if err != nil {
		return false
	}
	defer rt.Close()
	_, err = rt.Stat(cleanRel(rel))
	return err == nil
}
