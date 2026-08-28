package dotfiles

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeploySeedsAndPreserves(t *testing.T) {
	home := t.TempDir()
	if err := Deploy(home); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{".bashrc", ".profile", ".bash_aliases", ".bash_exports", ".bash_secrets"} {
		if _, err := os.Stat(filepath.Join(home, f)); err != nil {
			t.Errorf("%s not seeded: %v", f, err)
		}
	}
	if st, _ := os.Stat(filepath.Join(home, ".bash_secrets")); st.Mode().Perm() != 0o600 {
		t.Errorf(".bash_secrets mode = %o, want 600", st.Mode().Perm())
	}

	// A user-edited file is never overwritten.
	custom := "# my own bashrc\n"
	os.WriteFile(filepath.Join(home, ".bashrc"), []byte(custom), 0o644)
	secrets := "export TOKEN=abc\n"
	os.WriteFile(filepath.Join(home, ".bash_secrets"), []byte(secrets), 0o600)
	if err := Deploy(home); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(home, ".bashrc")); string(b) != custom {
		t.Error("user .bashrc was overwritten")
	}
	if b, _ := os.ReadFile(filepath.Join(home, ".bash_secrets")); string(b) != secrets {
		t.Error(".bash_secrets was overwritten (R10 violation)")
	}
}

// TestDeployKeepsAppendedLegacy is the case the exact-match requirement
// exists for: a user who APPENDED to the seeded .bashrc still has the legacy
// first line — their file must survive untouched.
func TestDeployKeepsAppendedLegacy(t *testing.T) {
	home := t.TempDir()
	appended := "# styrman home\ncase $- in *i*) ;; *) return ;; esac\n[ -f ~/.bash_aliases ] && . ~/.bash_aliases\n[ -f ~/.bash_exports ] && . ~/.bash_exports\n[ -f ~/.bash_secrets ] && . ~/.bash_secrets\nexport PATH=\"$HOME/.local/bin:$HOME/go/bin:$PATH\"\nPS1=\"\\[\\e[36m\\]\\u@\\h\\[\\e[0m\\]:\\[\\e[34m\\]\\w\\[\\e[0m\\]\\$ \"\nexport MY_THING=1\n"
	os.WriteFile(filepath.Join(home, ".bashrc"), []byte(appended), 0o644)
	if err := Deploy(home); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(home, ".bashrc")); string(b) != appended {
		t.Error("user-appended legacy .bashrc was replaced")
	}
}

// TestDeployRefusesSymlinks: the guest (same uid on the shared mount) can
// plant symlinks; Deploy must neither follow one as an existing file nor
// write through a dangling one.
func TestDeployRefusesSymlinks(t *testing.T) {
	home := t.TempDir()
	outside := t.TempDir()
	// Dangling symlink pointing outside the home at the seed path.
	if err := os.Symlink(filepath.Join(outside, "target"), filepath.Join(home, ".bashrc")); err != nil {
		t.Fatal(err)
	}
	// Symlink to an existing outside file with legacy content — must not be
	// "upgraded" (that would rewrite the outside target's directory).
	os.WriteFile(filepath.Join(outside, "legacy"), []byte("# styrman home\n"), 0o644)
	os.Symlink(filepath.Join(outside, "legacy"), filepath.Join(home, ".bash_aliases"))
	if err := Deploy(home); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(outside, "target")); !os.IsNotExist(err) {
		t.Error("Deploy wrote through a dangling symlink outside the home")
	}
	if st, err := os.Lstat(filepath.Join(home, ".bash_aliases")); err != nil || st.Mode()&os.ModeSymlink == 0 {
		t.Error("symlinked .bash_aliases was replaced")
	}
	if b, _ := os.ReadFile(filepath.Join(outside, "legacy")); string(b) != "# styrman home\n" {
		t.Error("outside symlink target was modified")
	}
}

func TestDeployUpgradesLegacySeeds(t *testing.T) {
	home := t.TempDir()
	// The exact pre-v1.3 ensureHome seed (byte-for-byte).
	legacy := "# styrman home\ncase $- in *i*) ;; *) return ;; esac\n[ -f ~/.bash_aliases ] && . ~/.bash_aliases\n[ -f ~/.bash_exports ] && . ~/.bash_exports\n[ -f ~/.bash_secrets ] && . ~/.bash_secrets\nexport PATH=\"$HOME/.local/bin:$HOME/go/bin:$PATH\"\nPS1=\"\\[\\e[36m\\]\\u@\\h\\[\\e[0m\\]:\\[\\e[34m\\]\\w\\[\\e[0m\\]\\$ \"\n"
	os.WriteFile(filepath.Join(home, ".bashrc"), []byte(legacy), 0o644)
	os.WriteFile(filepath.Join(home, ".bash_aliases"), []byte("alias ll=\"ls -la\"\n"), 0o644)
	if err := Deploy(home); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(home, ".bashrc"))
	if !strings.Contains(string(b), "__git_branch") {
		t.Error("legacy .bashrc was not upgraded to the full userprofile")
	}
	if _, err := os.Stat(filepath.Join(home, ".bashrc.pre-v1.3")); err != nil {
		t.Error("legacy .bashrc backup missing")
	}
	if b, _ := os.ReadFile(filepath.Join(home, ".bash_aliases")); !strings.Contains(string(b), "alias gs=") {
		t.Error("legacy .bash_aliases was not upgraded")
	}
}
