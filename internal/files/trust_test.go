package files

import (
	"os"
	"path/filepath"
	"testing"
)

// A project home lives INSIDE the base home the setup VM mounts with a root
// guest, so the guest can replace `p/<uuid>` with a symlink. Below a trusted
// tenant base the root must be reached with symlink confinement: a link to a
// host path or to another tenant is refused; a link that stays inside the
// tenant still works (BUG-002).
func TestTrustedRootRefusesAPlantedSymlink(t *testing.T) {
	base := t.TempDir()
	Trust(base)
	victim := filepath.Join(base, "bob", "p", "x")
	hostDir := t.TempDir() // stands in for /etc
	for _, d := range []string{filepath.Join(base, "alice", "p"), victim} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []string{filepath.Join(victim, "secret"), filepath.Join(hostDir, "passwd")} {
		if err := os.WriteFile(f, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	toHost := filepath.Join(base, "alice", "p", "to-host")
	toBob := filepath.Join(base, "alice", "p", "to-bob")
	if err := os.Symlink(hostDir, toHost); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, toBob); err != nil {
		t.Fatal(err)
	}
	if Exists(toHost, "passwd") || ListNames(toHost, ".") != nil {
		t.Error("a root symlinked to a host path was followed")
	}
	if Exists(toBob, "secret") || ListNames(toBob, ".") != nil {
		t.Error("a root symlinked to another tenant was followed")
	}
	// Within the tenant: a real dir, and a symlink that stays inside alice.
	real := filepath.Join(base, "alice", "p", "real")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(real, "ok"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !Exists(real, "ok") {
		t.Error("a real project home under the tenant must open")
	}
	// A RELATIVE symlink that stays inside the tenant still opens; an
	// absolute one is refused by os.Root even when its target is inside
	// (stricter than needed, and nothing legitimate plants one there).
	inside := filepath.Join(base, "alice", "p", "inside")
	if err := os.Symlink("real", inside); err != nil {
		t.Fatal(err)
	}
	if !Exists(inside, "ok") {
		t.Error("a relative symlink that stays inside the tenant must still open")
	}
	insideAbs := filepath.Join(base, "alice", "p", "inside-abs")
	if err := os.Symlink(real, insideAbs); err != nil {
		t.Fatal(err)
	}
	if Exists(insideAbs, "ok") {
		t.Error("an absolute symlink is refused even inside the tenant")
	}
	// A root outside every trusted base keeps the plain behaviour.
	other := t.TempDir()
	if err := os.WriteFile(filepath.Join(other, "f"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !Exists(other, "f") {
		t.Error("an untrusted root must still open as before")
	}
	if Exists("", "f") || ListNames("", ".") != nil {
		t.Error("an empty root must fail closed, not resolve to the cwd")
	}
}

// ListNames reads a guest-writable directory on a poll; the cap bounds what
// one tenant can make the shared server read per request.
func TestListNamesIsCapped(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "many")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < maxListNames+200; i++ {
		if err := os.WriteFile(filepath.Join(dir, "e"+string(rune('a'+i%26))+fmtInt(i)), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if n := len(ListNames(root, "many")); n != maxListNames {
		t.Errorf("ListNames returned %d entries, want the cap %d", n, maxListNames)
	}
}

func fmtInt(i int) string {
	return string(rune('0'+i/1000%10)) + string(rune('0'+i/100%10)) + string(rune('0'+i/10%10)) + string(rune('0'+i%10))
}

// The whole explorer runs through roots below a trusted base: List must
// return every entry with its size there too. Under Go 1.25 a sub-root's
// DirEntry.Info() fails, and the first Trust cut turned every project
// listing into "Empty directory" (release e2e, 2026-09-04).
func TestListWorksBelowATrustedBase(t *testing.T) {
	base := t.TempDir()
	Trust(base)
	ws := filepath.Join(base, "alice", "proj")
	if err := os.MkdirAll(filepath.Join(ws, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "README.md"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	lst, err := List(ws, "")
	if err != nil {
		t.Fatal(err)
	}
	entries := lst.Entries
	if len(entries) != 2 {
		t.Fatalf("List below a trusted base returned %d entries, want 2: %+v", len(entries), entries)
	}
	var sawFile bool
	for _, e := range entries {
		if e.Name == "README.md" {
			sawFile = true
			if e.Size != 5 || e.Dir {
				t.Errorf("README.md entry = %+v, want size 5, not a dir", e)
			}
		}
	}
	if !sawFile {
		t.Error("README.md missing from the listing")
	}
}
