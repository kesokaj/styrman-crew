package files

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// The homes Remove operates on are bind-mounted into microVMs where the guest
// is root, so a guest can replace an intermediate directory with a symlink
// into ANOTHER tenant's home. os.Root confinement is what stops that; a
// path-string check does not, because the string never contains "..".
func TestRemoveRefusesSymlinkedEscape(t *testing.T) {
	base := t.TempDir()
	attacker := filepath.Join(base, "attacker")
	victim := filepath.Join(base, "victim")
	for _, d := range []string{filepath.Join(attacker), filepath.Join(victim, "gcloud")} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	secret := filepath.Join(victim, "gcloud", "creds.json")
	if err := os.WriteFile(secret, []byte("victim token"), 0o600); err != nil {
		t.Fatal(err)
	}
	// The guest plants ~/.config -> the victim's home.
	if err := os.Symlink(victim, filepath.Join(attacker, ".config")); err != nil {
		t.Fatal(err)
	}

	// The exact call the credential sweep makes, with the stored path that
	// looks entirely benign: ".config/gcloud/creds.json".
	err := Remove(attacker, ".config/gcloud/creds.json")
	if err == nil {
		t.Fatal("Remove followed a symlink out of the root — cross-tenant deletion")
	}
	if _, statErr := os.Stat(secret); statErr != nil {
		t.Fatalf("the other tenant's file was deleted: %v", statErr)
	}
	// The attacker's own symlink is untouched too — Remove refused, it did not
	// half-succeed.
	if _, lerr := os.Lstat(filepath.Join(attacker, ".config")); lerr != nil {
		t.Errorf("symlink itself disturbed: %v", lerr)
	}
}

// The ordinary case still has to work, or the revoke silently stops revoking.
func TestRemoveDeletesARealFile(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".config", "gcloud"), 0o700); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(root, ".config", "gcloud", "creds.json")
	if err := os.WriteFile(p, []byte("token"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Remove(root, ".config/gcloud/creds.json"); err != nil {
		t.Fatalf("Remove on a real file: %v", err)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Errorf("file still present after Remove: %v", err)
	}
	// A missing file reports NotExist so the caller can ignore it.
	if err := Remove(root, ".config/gcloud/creds.json"); !os.IsNotExist(err) {
		t.Errorf("second Remove = %v, want IsNotExist", err)
	}
	// Never recursive: Remove on a non-empty dir must fail, not wipe a subtree.
	if err := Remove(root, ".config"); err == nil {
		t.Error("Remove emptied a non-empty directory — it must not recurse")
	}
}

// A traversal in the stored path string is refused too.
func TestRemoveRefusesDotDot(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "home")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(base, "outside.json")
	if err := os.WriteFile(outside, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = Remove(root, "../outside.json")
	if _, err := os.Stat(outside); err != nil {
		t.Errorf("a file outside the root was deleted: %v", err)
	}
}

// Create is the server-side guarantee behind "that name is taken": the UI's
// own check races the last-loaded listing, so only O_EXCL can actually stop a
// raw API caller (or two people at once) truncating an existing file.
func TestCreateIsExclusive(t *testing.T) {
	root := t.TempDir()
	if err := Create(root, "notes.md", "first"); err != nil {
		t.Fatalf("Create on a free name: %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(root, "notes.md")); string(b) != "first" {
		t.Errorf("content = %q, want %q", b, "first")
	}
	if err := Create(root, "notes.md", "second"); !errors.Is(err, ErrExists) {
		t.Errorf("Create over an existing file = %v, want ErrExists", err)
	}
	// And it must not have been truncated on the way to failing.
	if b, _ := os.ReadFile(filepath.Join(root, "notes.md")); string(b) != "first" {
		t.Errorf("Create truncated the existing file: %q", b)
	}
	// Write still overwrites deliberately — that is Save.
	if err := Write(root, "notes.md", "third"); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(root, "notes.md")); string(b) != "third" {
		t.Errorf("Write did not overwrite: %q", b)
	}
	// Confinement still applies to the exclusive path.
	if err := Create(root, "../escape.md", "x"); err == nil {
		if _, statErr := os.Stat(filepath.Join(filepath.Dir(root), "escape.md")); statErr == nil {
			t.Error("Create escaped the root")
		}
	}
}
