package files

import (
	"os"
	"path/filepath"
	"testing"
)

// setup builds two sibling "home" roots and a victim file outside the
// attacker's root, mirroring the multi-tenant layout.
func setup(t *testing.T) (root, victim string) {
	t.Helper()
	base := t.TempDir()
	root = filepath.Join(base, "homes", "attacker")
	victimHome := filepath.Join(base, "homes", "victim")
	for _, d := range []string{root, victimHome, filepath.Join(root, "projects", "p")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	victim = filepath.Join(victimHome, "secret")
	if err := os.WriteFile(victim, []byte("VICTIM-SECRET"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "projects", "p", "ok.txt"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, victim
}

func TestReadConfinedToRoot(t *testing.T) {
	root, victim := setup(t)
	// A legit read inside the root works.
	if fc, err := Read(root, "projects/p/ok.txt"); err != nil || fc.Content != "mine" {
		t.Fatalf("legit read: err=%v content=%q", err, fc.Content)
	}
	// Symlink projects/p/foo -> victim's home; reading through it must fail.
	link := filepath.Join(root, "projects", "p", "foo")
	if err := os.Symlink(filepath.Dir(victim), link); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(root, "projects/p/foo/secret"); err == nil {
		t.Fatal("read through escaping symlink must fail")
	}
	// A symlinked leaf pointing directly at the victim file must fail too.
	leaf := filepath.Join(root, "projects", "p", "leak")
	if err := os.Symlink(victim, leaf); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(root, "projects/p/leak"); err == nil {
		t.Fatal("read through symlinked leaf must fail")
	}
}

func TestTraversalRefused(t *testing.T) {
	root, _ := setup(t)
	for _, rel := range []string{"../victim/secret", "../../etc/passwd", "projects/../../victim/secret"} {
		if _, err := Read(root, rel); err == nil {
			t.Fatalf("traversal %q must fail", rel)
		}
	}
}

func TestListDoesNotFollowEscape(t *testing.T) {
	root, victim := setup(t)
	link := filepath.Join(root, "projects", "p", "esc")
	if err := os.Symlink(filepath.Dir(victim), link); err != nil {
		t.Fatal(err)
	}
	// Listing the symlink target as a directory must fail (can't enumerate
	// the victim's home through it).
	if _, err := List(root, "projects/p/esc"); err == nil {
		t.Fatal("listing through escaping symlink must fail")
	}
	// But the entry itself shows in the parent listing, flagged as a symlink.
	entries, err := List(root, "projects/p")
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, e := range entries {
		if e.Name == "esc" {
			found = true
			if !e.Symlink {
				t.Error("esc should be flagged as a symlink")
			}
		}
	}
	if !found {
		t.Error("esc entry missing from listing")
	}
}

func TestWriteAndSaveConfined(t *testing.T) {
	root, victim := setup(t)
	// Legit write.
	if err := Write(root, "projects/p/new.txt", "hi"); err != nil {
		t.Fatal(err)
	}
	// Symlinked target must not be followed on write (arbitrary-overwrite).
	link := filepath.Join(root, "projects", "p", "evil")
	if err := os.Symlink(victim, link); err != nil {
		t.Fatal(err)
	}
	if err := Write(root, "projects/p/evil", "OVERWRITTEN"); err == nil {
		t.Fatal("write through symlinked leaf must fail")
	}
	if b, _ := os.ReadFile(victim); string(b) != "VICTIM-SECRET" {
		t.Fatalf("victim file was overwritten: %q", b)
	}
	// Save (with parent mkdir) is likewise confined.
	if err := Save(root, "a/b/c.txt", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Lexical ".." is clamped to the root (writes inside), never escaping to
	// the sibling victim home.
	_ = Save(root, "../victim/planted", []byte("x"), 0o644)
	if _, err := os.Stat(filepath.Join(filepath.Dir(victim), "planted")); err == nil {
		t.Fatal("Save escaped root via ..")
	}
}

func TestReadRawAndExists(t *testing.T) {
	root, victim := setup(t)
	if b, err := ReadRaw(root, "projects/p/ok.txt", 0); err != nil || string(b) != "mine" {
		t.Fatalf("ReadRaw: %v / %q", err, b)
	}
	if !Exists(root, "projects/p/ok.txt") {
		t.Fatal("Exists should be true for a real file")
	}
	// Symlink escaping the root reads as not-existing.
	link := filepath.Join(root, "escsecret")
	if err := os.Symlink(victim, link); err != nil {
		t.Fatal(err)
	}
	if Exists(root, "escsecret") {
		t.Fatal("Exists must be false for an escaping symlink")
	}
	if _, err := ReadRaw(root, "escsecret", 0); err == nil {
		t.Fatal("ReadRaw through escaping symlink must fail")
	}
}

func TestContainedPath(t *testing.T) {
	root, victim := setup(t)
	if _, err := ContainedPath(root, "projects/p"); err != nil {
		t.Fatalf("legit path: %v", err)
	}
	link := filepath.Join(root, "projects", "p", "esc")
	if err := os.Symlink(filepath.Dir(victim), link); err != nil {
		t.Fatal(err)
	}
	if _, err := ContainedPath(root, "projects/p/esc"); err == nil {
		t.Fatal("ContainedPath through escaping symlink must fail")
	}
	// Lexical ".." is clamped inside the root, so the returned path never
	// escapes even when the input tries to.
	for _, rel := range []string{"../victim", "../../etc/passwd"} {
		got, err := ContainedPath(root, rel)
		if err != nil {
			continue // rejected is also fine
		}
		real, _ := filepath.EvalSymlinks(root)
		if rp, _ := filepath.Rel(real, got); rp == ".." || filepath.IsAbs(rp) || len(rp) > 2 && rp[:3] == ".."+string(filepath.Separator) {
			t.Fatalf("ContainedPath %q escaped root: %q", rel, got)
		}
	}
}
