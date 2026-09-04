package orchestrator

import (
	"github.com/docker/docker/api/types/container"
	"os"
	"path/filepath"
	"testing"
)

// A project home path becomes a Docker bind SOURCE; every component below
// the base home is guest-replaceable, so a symlink anywhere in between must
// be refused (BUG-003).
func TestRealBelowRefusesSymlinkComponents(t *testing.T) {
	base := t.TempDir()
	real := filepath.Join(base, "p", "real")
	if err := os.MkdirAll(real, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := realBelow(base, real); err != nil {
		t.Errorf("real dir: %v", err)
	}
	outside := t.TempDir()
	linked := filepath.Join(base, "p", "linked")
	if err := os.Symlink(outside, linked); err != nil {
		t.Fatal(err)
	}
	if err := realBelow(base, linked); err == nil {
		t.Error("a symlinked last component must be refused")
	}
	// The intermediate component `p` replaced by a link.
	base2 := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(base2, "p")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(outside, "x"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := realBelow(base2, filepath.Join(base2, "p", "x")); err == nil {
		t.Error("a symlinked intermediate component must be refused")
	}
	if err := realBelow(base, filepath.Join(t.TempDir(), "elsewhere")); err == nil {
		t.Error("a path outside base must be refused")
	}
}

// The guard runs at the top of EnsureRunning on EVERY path (start included:
// Docker re-resolves a stored bind source on each start), so it must allow a
// home that does not exist yet and refuse a symlink whether or not its
// target exists.
func TestProjectHomeIsRealAllowsMissingRefusesLinks(t *testing.T) {
	base := t.TempDir()
	if err := projectHomeIsReal(base, filepath.Join(base, "p", "not-yet")); err != nil {
		t.Errorf("a missing home must be allowed: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(base, "p"), 0o700); err != nil {
		t.Fatal(err)
	}
	dangling := filepath.Join(base, "p", "dangling")
	if err := os.Symlink(filepath.Join(t.TempDir(), "gone"), dangling); err != nil {
		t.Fatal(err)
	}
	if err := projectHomeIsReal(base, dangling); err == nil {
		t.Error("a dangling symlink must be refused")
	}
	live := filepath.Join(base, "p", "live")
	if err := os.Symlink(t.TempDir(), live); err != nil {
		t.Fatal(err)
	}
	if err := projectHomeIsReal(base, live); err == nil {
		t.Error("a symlink to an existing host dir must be refused")
	}
}

// A setup container created before ADR-064 binds homes/<user>; one created
// after binds homes/<user>/base. Only the latter is kept.
func TestSetupHomeStale(t *testing.T) {
	old := []container.MountPoint{{Destination: "/home/simon", Source: "/x/homes/simon"}}
	cur := []container.MountPoint{{Destination: "/home/simon", Source: "/x/homes/simon/base/"}}
	if !setupHomeStale(old, "simon", "/x/homes/simon/base") {
		t.Error("the pre-ADR-064 bind must read as stale")
	}
	if setupHomeStale(cur, "simon", "/x/homes/simon/base") {
		t.Error("the current bind (trailing slash cleaned) must not read as stale")
	}
	if !setupHomeStale(nil, "simon", "/x/homes/simon/base") {
		t.Error("no home bind at all must read as stale")
	}
	if !setupHomeStale(cur, "other", "/x/homes/simon/base") {
		t.Error("a bind for a different guest name must read as stale")
	}
}
