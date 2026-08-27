package files

import (
	"os"
	"path/filepath"
	"testing"
)

// Guards the DELETE ?wipe=true cross-tenant destruction the reviewer found:
// a guest symlinks ~/projects at another home, then a wipe must NOT recurse
// into it.
func TestRemoveAllRefusesSymlinkedProjects(t *testing.T) {
	base := t.TempDir()
	attacker := filepath.Join(base, "homes", "attacker")
	victimHome := filepath.Join(base, "homes", "victim")
	victimProjects := filepath.Join(victimHome, "projects")
	if err := os.MkdirAll(victimProjects, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(attacker, 0o755); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(victimProjects, "important.txt")
	if err := os.WriteFile(keep, []byte("VICTIM"), 0o644); err != nil {
		t.Fatal(err)
	}
	// attacker's ~/projects is a symlink into the victim's home
	if err := os.Symlink(victimHome, filepath.Join(attacker, "projects")); err != nil {
		t.Fatal(err)
	}
	// The wipe path: files.RemoveAll(home, "projects/<dir>")
	_ = RemoveAll(attacker, "projects/anything")
	if _, err := os.Stat(keep); err != nil {
		t.Fatalf("victim data was destroyed through the symlink: %v", err)
	}
}
