// Package backup creates and restores per-project archives (ADR-044): a
// tar.gz holding the project workspace and per-project home, streamed to a
// server-controlled file. Restores are defensive even though the archives are
// server-written: absolute paths and ".." segments are rejected, only
// regular files, directories and symlinks are extracted, and ownership is
// restored best-effort (a non-root server keeps its own uid).
package backup

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Skip names the top-level entries excluded from a source dir. The docker
// disk (~/.dockerd) is reproducible cache measured in GB — never archived.
var Skip = map[string]bool{".dockerd": true}

// Create streams a tar.gz of the given sources into w. sources maps an
// archive prefix ("workspace", "home") to a source directory; a missing
// source dir is skipped (a never-booted project has no home yet).
func Create(w io.Writer, sources map[string]string) error {
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	for prefix, src := range sources {
		if err := addTree(tw, prefix, src); err != nil {
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return gz.Close()
}

func addTree(tw *tar.Writer, prefix, src string) error {
	info, err := os.Lstat(src)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("backup source %s is not a directory", src)
	}
	return filepath.Walk(src, func(path string, fi fs.FileInfo, err error) error {
		if err != nil {
			// A file vanishing mid-walk (live project) is fine; skip it.
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		// Top-level exclusions (the docker disk).
		if parts := strings.SplitN(rel, string(os.PathSeparator), 2); Skip[parts[0]] {
			if fi.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		var link string
		if fi.Mode()&os.ModeSymlink != 0 {
			if link, err = os.Readlink(path); err != nil {
				return nil // unreadable link — skip
			}
		} else if !fi.Mode().IsRegular() && !fi.IsDir() {
			return nil // sockets, fifos, devices: never archived
		}
		hdr, err := tar.FileInfoHeader(fi, link)
		if err != nil {
			return err
		}
		hdr.Name = prefix + "/" + filepath.ToSlash(rel)
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if fi.Mode().IsRegular() {
			f, err := os.Open(path)
			if err != nil {
				return nil // vanished/unreadable mid-walk — entry stays empty
			}
			// The file can shrink between Lstat and read on a live project;
			// CopyN pads nothing, so pad short reads to keep the tar valid.
			n, cerr := io.CopyN(tw, f, hdr.Size)
			f.Close()
			if cerr != nil && cerr != io.EOF {
				return cerr
			}
			if n < hdr.Size {
				if _, err := tw.Write(make([]byte, hdr.Size-n)); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// Extract unpacks an archive created by Create. dests maps each archive
// prefix to its destination directory (created if missing); entries with an
// unknown prefix, an absolute path, or a ".." segment are rejected.
func Extract(r io.Reader, dests map[string]string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		name := filepath.ToSlash(hdr.Name)
		parts := strings.SplitN(name, "/", 2)
		dest, ok := dests[parts[0]]
		if !ok || len(parts) != 2 || parts[1] == "" {
			continue // unknown prefix or the prefix dir itself
		}
		rel := parts[1]
		if strings.HasPrefix(rel, "/") || hasDotDot(rel) {
			return fmt.Errorf("archive entry escapes destination: %q", hdr.Name)
		}
		target := filepath.Join(dest, filepath.FromSlash(rel))
		// Defense in depth: refuse to write THROUGH a symlink component — a
		// crafted archive could otherwise place `a -> /etc` then `a/passwd`
		// and escape dest past the abs/".." checks. Unreachable from Create
		// (filepath.Walk never descends into symlinks), but Extract must not
		// depend on who wrote the archive.
		if err := rejectSymlinkParents(dest, filepath.FromSlash(rel)); err != nil {
			return fmt.Errorf("archive entry %q: %w", hdr.Name, err)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, fs.FileMode(hdr.Mode)&0o777); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			f, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, fs.FileMode(hdr.Mode)&0o777)
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return err
			}
			if err := f.Close(); err != nil {
				return err
			}
		case tar.TypeSymlink:
			// The link TARGET is restored verbatim (it may point anywhere,
			// same as the guest could create itself); the link NAME is
			// confined like every other entry. Never followed server-side.
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			os.Remove(target)
			if err := os.Symlink(hdr.Linkname, target); err != nil {
				return err
			}
		default:
			continue // hardlinks/devices/fifos are never created
		}
		// Ownership back to the guest uid — root restores it exactly, a
		// non-root server keeps its own uid (same uid as the guest in dev).
		_ = os.Lchown(target, hdr.Uid, hdr.Gid)
	}
}

func hasDotDot(rel string) bool {
	for _, seg := range strings.Split(rel, "/") {
		if seg == ".." {
			return true
		}
	}
	return false
}

// rejectSymlinkParents errors if any existing path component between dest and
// dest/rel (exclusive of the leaf) is a symlink.
func rejectSymlinkParents(dest, rel string) error {
	cur := dest
	segs := strings.Split(rel, string(os.PathSeparator))
	for _, seg := range segs[:len(segs)-1] {
		cur = filepath.Join(cur, seg)
		fi, err := os.Lstat(cur)
		if err != nil {
			if os.IsNotExist(err) {
				return nil // rest of the path doesn't exist yet — safe
			}
			return err
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("parent %q is a symlink", cur)
		}
	}
	return nil
}
