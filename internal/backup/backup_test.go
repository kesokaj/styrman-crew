package backup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	src := t.TempDir()
	ws := filepath.Join(src, "ws")
	home := filepath.Join(src, "home")
	os.MkdirAll(filepath.Join(ws, ".git"), 0o755)
	os.WriteFile(filepath.Join(ws, "main.go"), []byte("package main"), 0o644)
	os.WriteFile(filepath.Join(ws, ".git", "HEAD"), []byte("ref: refs/heads/main"), 0o644)
	os.MkdirAll(filepath.Join(home, ".config"), 0o700)
	os.WriteFile(filepath.Join(home, ".bashrc"), []byte("export X=1"), 0o644)
	os.WriteFile(filepath.Join(home, ".config", "secret"), []byte("s3cr3t"), 0o600)
	os.Symlink(".bashrc", filepath.Join(home, "link"))
	// The docker disk must be excluded.
	os.MkdirAll(filepath.Join(home, ".dockerd"), 0o700)
	os.WriteFile(filepath.Join(home, ".dockerd", "huge.img"), bytes.Repeat([]byte("x"), 1024), 0o600)

	var buf bytes.Buffer
	if err := Create(&buf, map[string]string{"workspace": ws, "home": home}); err != nil {
		t.Fatal(err)
	}

	dst := t.TempDir()
	dests := map[string]string{
		"workspace": filepath.Join(dst, "ws"),
		"home":      filepath.Join(dst, "home"),
	}
	if err := Extract(bytes.NewReader(buf.Bytes()), dests); err != nil {
		t.Fatal(err)
	}

	for path, want := range map[string]string{
		"ws/main.go":          "package main",
		"ws/.git/HEAD":        "ref: refs/heads/main",
		"home/.bashrc":        "export X=1",
		"home/.config/secret": "s3cr3t",
	} {
		got, err := os.ReadFile(filepath.Join(dst, path))
		if err != nil || string(got) != want {
			t.Errorf("%s = %q, %v (want %q)", path, got, err, want)
		}
	}
	// Mode preserved on the secret.
	if fi, _ := os.Stat(filepath.Join(dst, "home/.config/secret")); fi.Mode().Perm() != 0o600 {
		t.Errorf("secret mode = %v, want 0600", fi.Mode().Perm())
	}
	// Symlink preserved as a symlink.
	if l, err := os.Readlink(filepath.Join(dst, "home/link")); err != nil || l != ".bashrc" {
		t.Errorf("symlink = %q, %v", l, err)
	}
	// .dockerd excluded.
	if _, err := os.Stat(filepath.Join(dst, "home/.dockerd")); !os.IsNotExist(err) {
		t.Error(".dockerd was archived — must be excluded")
	}
	// Missing source dir is fine (never-booted project has no home).
	var b2 bytes.Buffer
	if err := Create(&b2, map[string]string{"home": filepath.Join(src, "nope")}); err != nil {
		t.Fatalf("missing source dir: %v", err)
	}
}

// A hostile archive (absolute path / .. segment / unknown prefix) must never
// write outside the destination dirs.
func TestExtractRejectsEscapes(t *testing.T) {
	hostile := func(name string) []byte {
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		tw := tar.NewWriter(gz)
		tw.WriteHeader(&tar.Header{Name: name, Typeflag: tar.TypeReg, Mode: 0o644, Size: 4})
		tw.Write([]byte("evil"))
		tw.Close()
		gz.Close()
		return buf.Bytes()
	}
	dst := t.TempDir()
	dests := map[string]string{"workspace": filepath.Join(dst, "ws")}
	for _, name := range []string{
		"workspace/../escape.txt",
		"workspace/a/../../escape.txt",
		"workspace//../escape.txt",
	} {
		if err := Extract(bytes.NewReader(hostile(name)), dests); err == nil {
			t.Errorf("Extract(%q) accepted a traversal", name)
		}
	}
	// Unknown prefix is skipped silently, not written anywhere.
	if err := Extract(bytes.NewReader(hostile("elsewhere/x.txt")), dests); err != nil {
		t.Errorf("unknown prefix should be skipped: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "escape.txt")); !os.IsNotExist(err) {
		t.Fatal("traversal escaped the destination")
	}
}

// A symlink dir entry followed by a file beneath it must not write through
// the link outside the destination (defense-in-depth; Create can't emit this).
func TestExtractRejectsSymlinkParent(t *testing.T) {
	outside := t.TempDir()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	tw.WriteHeader(&tar.Header{Name: "home/a", Typeflag: tar.TypeSymlink, Linkname: outside, Mode: 0o777})
	tw.WriteHeader(&tar.Header{Name: "home/a/evil.txt", Typeflag: tar.TypeReg, Mode: 0o644, Size: 4})
	tw.Write([]byte("evil"))
	tw.Close()
	gz.Close()

	dst := t.TempDir()
	err := Extract(bytes.NewReader(buf.Bytes()), map[string]string{"home": filepath.Join(dst, "home")})
	if err == nil {
		t.Fatal("Extract accepted a write through a symlink parent")
	}
	if _, serr := os.Stat(filepath.Join(outside, "evil.txt")); !os.IsNotExist(serr) {
		t.Fatal("file escaped through the symlink parent")
	}
}
