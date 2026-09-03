package harness

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
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

	dest := filepath.Join(repo, "skills", filepath.Base(from))
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

func copyTree(src, dest string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
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
