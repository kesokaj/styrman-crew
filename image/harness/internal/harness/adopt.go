package harness

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Adopt copies a skill installed somewhere else — by hand, or by
// `npx skills add -g` into ~/.agents/skills — into the repo's skills/, so it
// is versioned and deployed to both CLIs on the next install. src is a path
// or a bare skill name looked up in the usual global locations.
func Adopt(repo, home, src string, force bool) (string, error) {
	candidates := []string{
		src,
		filepath.Join(home, ".agents", "skills", src),
		filepath.Join(home, ClaudeRoot, "skills", src),
		filepath.Join(home, AgyRoot, "skills", src),
	}
	var from string
	for _, c := range candidates {
		resolved, err := filepath.EvalSymlinks(c)
		if err != nil {
			continue
		}
		if _, err := os.Stat(filepath.Join(resolved, "SKILL.md")); err == nil {
			from = resolved
			break
		}
	}
	if from == "" {
		return "", fmt.Errorf("no skill directory with a SKILL.md found for %q", src)
	}

	// EvalSymlinks leaves a relative path relative, so ".", ".." and
	// "skills/<name>" reached Base unchanged and named the deletion root
	// itself, or the source (BUG-037).
	from, err := filepath.Abs(from)
	if err != nil {
		return "", err
	}
	name := filepath.Base(from)
	if err := validName("skill", src, name); err != nil {
		return "", err
	}
	dest, err := filepath.Abs(filepath.Join(repo, "skills", name))
	if err != nil {
		return "", err
	}
	if within(from, dest) || within(dest, from) {
		return "", fmt.Errorf("%s and %s overlap; adopt copies a skill directory from outside the repo's skills/", from, dest)
	}
	if _, err := os.Stat(dest); err == nil {
		if !force {
			return "", fmt.Errorf("%s already exists; pass --force to replace it", dest)
		}
		if err := os.RemoveAll(dest); err != nil {
			return "", err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if err := copyTree(from, dest); err != nil {
		return "", err
	}
	return dest, nil
}

// within reports whether child is parent itself or lies inside it. Adopt
// removes dest before copying, so neither path may contain the other.
func within(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func copyTree(src, dest string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		// Same refusal as Plan's walk: a link would be copied as its target's
		// bytes under the link's mode (security review 2026-09-14).
		if d.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("%s is a symlink — refused", path)
		}
		target := filepath.Join(dest, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if d.Name() == ".DS_Store" {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return writeAtomic(target, data, info.Mode().Perm()|0o600)
	})
}
