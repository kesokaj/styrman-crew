// Package dotfiles owns the user shell environment seeded into every home:
// the userprofile set ported from the devbox predecessor (history, readline,
// git-branch prompt, aliases, PATH). The server deploys it write-once on
// every start (ensureHome), so it heals homes that predate it; the guest
// needs no seeding of its own — the home is the shared mount.
//
// Every filesystem operation is confined with os.Root: the guest runs as the
// same uid on this mount and can plant symlinks, so an unconfined write here
// would be a cross-tenant file-creation primitive (same threat model as
// internal/files).
package dotfiles

import (
	"embed"
	"io"
	"os"
	"strings"
)

//go:embed userprofile
var userprofile embed.FS

// legacySeeds holds the EXACT byte content the pre-v1.3 seeders wrote
// (ensureHome in Go, home-init.sh via printf '%s\n'). Only a byte-for-byte
// match — modulo a trailing-newline difference — is upgraded; a file the
// user appended so much as one line to is theirs and is never touched.
// .bash_secrets is deliberately absent (R10: never migrate secrets).
var legacySeeds = map[string][]string{
	".bashrc": {
		"# styrman home\ncase $- in *i*) ;; *) return ;; esac\n[ -f ~/.bash_aliases ] && . ~/.bash_aliases\n[ -f ~/.bash_exports ] && . ~/.bash_exports\n[ -f ~/.bash_secrets ] && . ~/.bash_secrets\nexport PATH=\"$HOME/.local/bin:$HOME/go/bin:$PATH\"\nPS1=\"\\[\\e[36m\\]\\u@\\h\\[\\e[0m\\]:\\[\\e[34m\\]\\w\\[\\e[0m\\]\\$ \"",
		"# styrman shared home\ncase $- in *i*) ;; *) return ;; esac\n[ -f ~/.bash_aliases ] && . ~/.bash_aliases\n[ -f ~/.bash_exports ] && . ~/.bash_exports\n[ -f ~/.bash_secrets ] && . ~/.bash_secrets\nexport PATH=\"$HOME/.local/bin:$HOME/go/bin:$PATH\"\nPS1=\"\\[\\e[36m\\]\\u@\\h\\[\\e[0m\\]:\\[\\e[34m\\]\\w\\[\\e[0m\\]\\$ \"",
	},
	".bash_aliases": {`alias ll="ls -la"`},
	".bash_exports": {`export EDITOR=vim`},
}

func isLegacy(name, content string) bool {
	c := strings.TrimRight(content, "\n")
	for _, seed := range legacySeeds[name] {
		if c == strings.TrimRight(seed, "\n") {
			return true
		}
	}
	return false
}

// files maps embedded name → home dotfile name and mode.
var files = []struct {
	src, dst string
	mode     os.FileMode
}{
	{"bashrc", ".bashrc", 0o644},
	{"profile", ".profile", 0o644},
	{"bash_aliases", ".bash_aliases", 0o644},
	{"bash_exports", ".bash_exports", 0o644},
	{"bash_secrets", ".bash_secrets", 0o600},
}

// Deploy seeds the userprofile into home. Existing files are kept — except
// the recognized legacy minimal seeds, which are moved aside (.pre-v1.3
// suffix) and replaced. Idempotent; safe to run on every server start.
func Deploy(home string) error {
	rt, err := os.OpenRoot(home)
	if err != nil {
		return err
	}
	defer rt.Close()
	for _, f := range files {
		st, err := rt.Lstat(f.dst)
		switch {
		case err == nil && !st.Mode().IsRegular():
			continue // symlink or odd type — not ours, leave it alone
		case err == nil:
			cur, rerr := readAll(rt, f.dst)
			if rerr != nil || !isLegacy(f.dst, cur) {
				continue // user's file — never touch it (R10 for .bash_secrets)
			}
			if _, err := rt.Lstat(f.dst + ".pre-v1.3"); err == nil {
				continue // backup already exists — don't clobber it
			}
			if err := rt.Rename(f.dst, f.dst+".pre-v1.3"); err != nil {
				continue
			}
		case !os.IsNotExist(err):
			continue
		}
		data, err := userprofile.ReadFile("userprofile/" + f.src)
		if err != nil {
			return err
		}
		// O_EXCL: never write through a pre-existing path (a dangling
		// symlink Lstat'd as missing would otherwise become a confined
		// write to its in-root target).
		w, err := rt.OpenFile(f.dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, f.mode)
		if err != nil {
			continue
		}
		_, werr := w.Write(data)
		if cerr := w.Close(); werr == nil {
			werr = cerr
		}
		if werr != nil {
			return werr
		}
	}
	return nil
}

func readAll(rt *os.Root, rel string) (string, error) {
	f, err := rt.Open(rel)
	if err != nil {
		return "", err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 1<<20))
	return string(b), err
}
