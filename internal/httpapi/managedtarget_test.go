package httpapi

import (
	"os"
	"path/filepath"
	"testing"
)

// managedHomeTarget keys on the SCOPE, and the four platform-managed names
// are matched by string. That much was always true; these pin it.
func TestManagedHomeTargetMatchesTheManagedNames(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"projects", ".dockerd", ".styrman", ".ssh"} {
		if !managedHomeTarget(root, true, name+"/anything") {
			t.Errorf("home scope: %q is platform-managed and was allowed", name)
		}
		// Project scope is its own workspace dir (ADR-027), where these are
		// ordinary user directories.
		if managedHomeTarget(root, false, name+"/anything") {
			t.Errorf("project scope: %q was refused, but the guard keys on the scope", name)
		}
	}
	if managedHomeTarget(root, true, "notes.md") {
		t.Error("an ordinary home file was refused")
	}
}

// The name match alone was never a boundary: os.Root follows symlinks that
// stay inside the root, so `ln -s .dockerd x` then deleting `x/disk.img`
// reached a managed subtree through an unmanaged name (probed 2026-09-22 —
// Root.RemoveAll succeeded). The guard now refuses a top-level link outright.
func TestManagedHomeTargetRefusesASymlinkedFirstComponent(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".dockerd"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(".dockerd", filepath.Join(root, "x")); err != nil {
		t.Fatal(err)
	}

	if !managedHomeTarget(root, true, "x/disk.img") {
		t.Error("a managed subtree was reachable through a top-level symlink")
	}
	// Same link, project scope: still an ordinary directory there.
	if managedHomeTarget(root, false, "x/disk.img") {
		t.Error("project scope refused a symlink; the guard is home-scope only")
	}
	// The link ITSELF is not a managed subtree: removing a bad symlink
	// removes only the link, and the explorer has always allowed it
	// (TestFileDeleteSymlinkEscape). A guard that refuses this is too broad.
	if managedHomeTarget(root, true, "x") {
		t.Error("deleting the symlink itself was refused; only paths THROUGH it are managed")
	}
}

// A real directory that merely shares a link's position is not refused —
// otherwise the test above would pass on a guard that refuses everything.
func TestManagedHomeTargetAllowsARealDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "work"), 0o755); err != nil {
		t.Fatal(err)
	}
	if managedHomeTarget(root, true, "work/notes.md") {
		t.Error("an ordinary home directory was refused as managed")
	}
}

// The guard stands in front of a delete, so it keeps the data when its own
// check cannot run: an unreadable root answers "managed", not "go ahead".
// An absent name is the one exception — it is no subtree at all.
func TestManagedHomeTargetFailsClosedWhenItCannotLook(t *testing.T) {
	if managedHomeTarget(filepath.Join(t.TempDir(), "no-such-home"), true, "whatever/x") {
		// An unopenable root must refuse.
	} else {
		t.Error("an unopenable root was treated as unmanaged: the guard failed open")
	}
	root := t.TempDir()
	if managedHomeTarget(root, true, "absent/x") {
		t.Error("an absent name was refused; the operation behind the guard fails on its own")
	}
}

// A symlink does not have to sit at the top of the path. With
// `work/sub/link -> ../../.dockerd`, the first component is a real directory,
// so a guard that inspects only the first component says "not managed" —
// while os.Root follows the link and the delete lands in the managed subtree
// anyway. Same attack as the test above, moved one level down (code review,
// 2026-09-22).
func TestManagedHomeTargetRefusesASymlinkNestedDeeper(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".dockerd"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "work", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("..", "..", ".dockerd"), filepath.Join(root, "work", "sub", "link")); err != nil {
		t.Fatal(err)
	}

	if !managedHomeTarget(root, true, "work/sub/link/disk.img") {
		t.Error("a managed subtree was reachable through a symlink below the first component")
	}
	// The link itself is still ordinary housekeeping, at any depth.
	if managedHomeTarget(root, true, "work/sub/link") {
		t.Error("deleting the nested symlink itself was refused; only paths THROUGH it are managed")
	}
	// And a path through real directories stays allowed.
	if managedHomeTarget(root, true, "work/sub/notes.md") {
		t.Error("an ordinary nested path was refused as managed")
	}
}
