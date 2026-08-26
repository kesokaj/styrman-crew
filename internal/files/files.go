// Package files serves the file explorer: directory listings and file
// contents under a fixed root. The styrman server has the shared home
// mounted at the same path as the host, so this reads the very files the
// agents see inside the microVMs — no exec needed, works while a project
// is stopped.
package files

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
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

// Resolve joins a root-relative path onto root, refusing anything that
// escapes it (traversal, absolute paths, or symlinks pointing outside).
func Resolve(root, rel string) (string, error) {
	rel = strings.TrimPrefix(filepath.Clean("/"+strings.TrimSpace(rel)), "/")
	full := filepath.Join(root, rel)

	// Cheap check first: lexical containment.
	if rel != "" {
		if r, err := filepath.Rel(root, full); err != nil || r == ".." || strings.HasPrefix(r, "../") {
			return "", fmt.Errorf("path escapes root")
		}
	}
	// Symlink check: the real path must still be inside the real root.
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	realFull, err := filepath.EvalSymlinks(full)
	if err != nil {
		// Non-existent target (e.g. saving a new file): validate its parent.
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
	full, err := Resolve(root, rel)
	if err != nil {
		return nil, err
	}
	des, err := os.ReadDir(full)
	if err != nil {
		return nil, err
	}
	rel = strings.TrimPrefix(filepath.Clean("/"+rel), "/")
	out := make([]Entry, 0, len(des))
	for _, de := range des {
		info, err := de.Info()
		if err != nil {
			continue
		}
		isDir := de.IsDir()
		symlink := de.Type()&os.ModeSymlink != 0
		if symlink {
			// Report symlinked directories as directories so they expand.
			if st, err := os.Stat(filepath.Join(full, de.Name())); err == nil {
				isDir = st.IsDir()
			}
		}
		out = append(out, Entry{
			Name:     de.Name(),
			Path:     strings.TrimPrefix(filepath.Join(rel, de.Name()), "/"),
			Dir:      isDir,
			Size:     info.Size(),
			Modified: info.ModTime().UTC().Format("2006-01-02 15:04"),
			Symlink:  symlink,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Dir != out[j].Dir {
			return out[i].Dir
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
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
	full, err := Resolve(root, rel)
	if err != nil {
		return nil, err
	}
	st, err := os.Stat(full)
	if err != nil {
		return nil, err
	}
	if st.IsDir() {
		return nil, fmt.Errorf("%s is a directory", rel)
	}
	f, err := os.Open(full)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	buf := make([]byte, MaxReadBytes)
	n, _ := f.Read(buf)
	buf = buf[:n]
	fc := &FileContent{
		Path:      rel,
		Size:      st.Size(),
		Truncated: st.Size() > int64(n),
	}
	// NUL byte or invalid UTF-8 ⇒ treat as binary; don't ship bytes the
	// browser would render as mojibake.
	if !utf8.Valid(buf) || strings.IndexByte(string(buf), 0) >= 0 {
		fc.Binary = true
		return fc, nil
	}
	fc.Content = string(buf)
	return fc, nil
}

// Write saves content to a file, creating it if needed.
func Write(root, rel, content string) error {
	full, err := Resolve(root, rel)
	if err != nil {
		return err
	}
	if st, err := os.Stat(full); err == nil && st.IsDir() {
		return fmt.Errorf("%s is a directory", rel)
	}
	return os.WriteFile(full, []byte(content), 0o644)
}
