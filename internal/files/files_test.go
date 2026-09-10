package files

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
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

func TestSaveStreamAndDownloadConfined(t *testing.T) {
	root, victim := setup(t)
	if err := SaveStream(root, "projects/p/up.txt", strings.NewReader("data")); err != nil {
		t.Fatal(err)
	}
	f, st, err := OpenForDownload(root, "projects/p/up.txt")
	if err != nil || st.Size() != 4 {
		t.Fatalf("download: %v size=%d", err, st.Size())
	}
	f.Close()
	// Escaping symlink refused on both.
	link := filepath.Join(root, "projects", "p", "esc")
	if err := os.Symlink(victim, link); err != nil {
		t.Fatal(err)
	}
	if err := SaveStream(root, "projects/p/esc", strings.NewReader("x")); err == nil {
		t.Fatal("SaveStream through symlink must fail")
	}
	if _, _, err := OpenForDownload(root, "projects/p/esc"); err == nil {
		t.Fatal("download through symlink must fail")
	}
}

func TestRejectNonRegular(t *testing.T) {
	root, _ := setup(t)
	fifo := filepath.Join(root, "projects", "p", "pipe")
	if err := syscall.Mkfifo(fifo, 0o644); err != nil {
		t.Skipf("mkfifo unsupported: %v", err)
	}
	// Must return promptly with an error, not block forever.
	done := make(chan error, 1)
	go func() { _, err := Read(root, "projects/p/pipe"); done <- err }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("reading a FIFO must fail")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Read blocked on a FIFO (DoS)")
	}
	// The no-follow read must refuse it just as promptly (O_NONBLOCK, then
	// the regular-file check).
	go func() { _, err := ReadRawNoFollow(root, "projects/p/pipe", 0); done <- err }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("ReadRawNoFollow of a FIFO must fail")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("ReadRawNoFollow blocked on a FIFO (DoS)")
	}
	// The WRITE paths must refuse it just as promptly (audit finding: a
	// write-only open of a FIFO blocks until a reader appears).
	go func() { done <- Write(root, "projects/p/pipe", "x") }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("writing a FIFO must fail")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Write blocked on a FIFO (DoS)")
	}
	go func() { done <- SaveStream(root, "projects/p/pipe", strings.NewReader("x")) }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("SaveStream to a FIFO must fail")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("SaveStream blocked on a FIFO (DoS)")
	}
}

func TestListNamesConfined(t *testing.T) {
	root, victim := setup(t)
	_ = Save(root, "projects/p/skills/a.md", []byte("x"), 0o644)
	names := ListNames(root, "projects/p/skills")
	if len(names) != 1 || names[0] != "a.md" {
		t.Fatalf("ListNames: %v", names)
	}
	link := filepath.Join(root, "projects", "p", "esc")
	if err := os.Symlink(filepath.Dir(victim), link); err != nil {
		t.Fatal(err)
	}
	if ns := ListNames(root, "projects/p/esc"); ns != nil {
		t.Fatalf("ListNames through escaping symlink must be nil, got %v", ns)
	}
}

func TestListReportsScopeRelativePaths(t *testing.T) {
	// Regression guard: entries under a subdir report the FULL root-relative
	// path; the httpapi layer strips the scope prefix before returning. Here
	// we assert List's own contract (root-relative), which the handler relies
	// on to strip exactly `prefix+"/"`.
	root, _ := setup(t)
	_ = Save(root, "projects/p/sub/deep.txt", []byte("x"), 0o644)
	entries, err := List(root, "projects/p/sub")
	if err != nil {
		t.Fatal(err)
	}
	var got string
	for _, e := range entries {
		if e.Name == "deep.txt" {
			got = e.Path
		}
	}
	if got != "projects/p/sub/deep.txt" {
		t.Fatalf("entry path = %q, want root-relative projects/p/sub/deep.txt", got)
	}
}

// A .pub that is a symlink — even to a file INSIDE the root, such as the
// private key beside it — must not be read: ReadRaw follows confined links by
// design, so the UI's echo of the public key needs the no-follow read.
func TestReadRawNoFollowRefusesASymlinkedFinalComponent(t *testing.T) {
	root, _ := setup(t)
	dir := filepath.Join(root, "projects", "p", ".ssh")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	priv := filepath.Join(dir, "id_ed25519")
	if err := os.WriteFile(priv, []byte("-----BEGIN OPENSSH PRIVATE KEY-----\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "real.pub"), []byte("ssh-ed25519 AAAA me\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("id_ed25519", filepath.Join(dir, "id_ed25519.pub")); err != nil {
		t.Fatal(err)
	}
	// The confined read follows it: this is the behaviour the UI must not use.
	if b, err := ReadRaw(root, "projects/p/.ssh/id_ed25519.pub", 0); err != nil || !strings.Contains(string(b), "PRIVATE") {
		t.Fatalf("precondition: ReadRaw follows a confined symlink; got %v / %q", err, b)
	}
	if _, err := ReadRawNoFollow(root, "projects/p/.ssh/id_ed25519.pub", 0); err == nil {
		t.Fatal("ReadRawNoFollow read through a symlinked .pub — the private key would reach the UI")
	}
	if b, err := ReadRawNoFollow(root, "projects/p/.ssh/real.pub", 0); err != nil || string(b) != "ssh-ed25519 AAAA me\n" {
		t.Fatalf("a real .pub must still read: %v / %q", err, b)
	}
	if _, err := ReadRawNoFollow(root, "projects/p/.ssh/missing.pub", 0); err == nil {
		t.Fatal("a missing file is an error, not empty content")
	}
	// The documented limit: a HARD link is not a symlink and IS read. The
	// caller (setup.go) keeps one line and requires a public-key shape, which
	// is what stands between this and the UI.
	if err := os.Link(priv, filepath.Join(dir, "hard.pub")); err != nil {
		t.Skipf("hard links unsupported here: %v", err)
	}
	if b, err := ReadRawNoFollow(root, "projects/p/.ssh/hard.pub", 0); err != nil || !strings.Contains(string(b), "PRIVATE") {
		t.Fatalf("a hard-linked .pub reads (documented limit); got %v / %q", err, b)
	}
}
