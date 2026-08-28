package credstore

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnvCreds(t *testing.T) {
	cs, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// Empty store → empty map, no error.
	m, err := cs.AllEnv("alice")
	if err != nil || len(m) != 0 {
		t.Fatalf("empty AllEnv = %v, %v", m, err)
	}
	if err := cs.PutEnv("alice", "GH_TOKEN", "ghs_xxx"); err != nil {
		t.Fatal(err)
	}
	if err := cs.PutEnv("alice", "NPM_TOKEN", "npm_yyy"); err != nil {
		t.Fatal(err)
	}
	m, _ = cs.AllEnv("alice")
	if m["GH_TOKEN"] != "ghs_xxx" || m["NPM_TOKEN"] != "npm_yyy" {
		t.Fatalf("AllEnv = %+v", m)
	}
	// Cross-user isolation.
	if m2, _ := cs.AllEnv("bob"); len(m2) != 0 {
		t.Fatal("cross-user env leak")
	}
	// Refresh is read back.
	cs.PutEnv("alice", "GH_TOKEN", "ghs_zzz")
	m, _ = cs.AllEnv("alice")
	if m["GH_TOKEN"] != "ghs_zzz" {
		t.Fatalf("refresh not read back: %q", m["GH_TOKEN"])
	}
	// Bad key rejected.
	for _, bad := range []string{"", "1LEADING", "has-dash", "has space", "a=b"} {
		if err := cs.PutEnv("alice", bad, "v"); err == nil {
			t.Errorf("PutEnv(%q) accepted a bad key", bad)
		}
	}
	// A value with a control character is rejected — it would inject an extra
	// exported variable into the login snippet's line-based env delivery.
	for _, bad := range []string{"a\nb", "a\tb", "a\rb", "a\x00b"} {
		if err := cs.PutEnv("alice", "GOOD_KEY", bad); err == nil {
			t.Errorf("PutEnv accepted a control-char value %q", bad)
		}
	}
	// The good keys are unaffected.
	m, _ = cs.AllEnv("alice")
	if m["GOOD_KEY"] != "" {
		t.Fatal("a rejected value was still stored")
	}
	// An absurdly long value (a paste mistake) is rejected.
	if err := cs.PutEnv("alice", "GOOD_KEY", strings.Repeat("x", 5000)); err == nil {
		t.Error("PutEnv accepted a >4096-byte value")
	}
}

// A corrupt store file must not surface as a ghost list row (empty ID the UI
// can never delete) — it is skipped.
func TestListSkipsCorruptFiles(t *testing.T) {
	cs, _ := New(t.TempDir())
	cs.PutEnv("alice", "GOOD", "v")
	os.WriteFile(filepath.Join(cs.userDir("alice"), "env-BROKEN.json"), []byte("{not json"), 0o600)
	os.WriteFile(filepath.Join(cs.userDir("alice"), "file-empty.json"), []byte("{}"), 0o600)
	items, err := cs.List("alice")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != "GOOD" {
		t.Fatalf("List with corrupt files = %+v, want only GOOD", items)
	}
}

func TestFileCreds(t *testing.T) {
	cs, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	fcs, err := cs.AllFiles("alice")
	if err != nil || len(fcs) != 0 {
		t.Fatalf("empty AllFiles = %v, %v", fcs, err)
	}
	if err := cs.PutFile("alice", FileCred{ID: "ssh-id_ed25519", Path: "~/.ssh/id_ed25519", Mode: "0600", Data: []byte("PRIVATE-KEY\x00\xff")}); err != nil {
		t.Fatal(err)
	}
	fcs, _ = cs.AllFiles("alice")
	if len(fcs) != 1 || fcs[0].Path != "~/.ssh/id_ed25519" || string(fcs[0].Data) != "PRIVATE-KEY\x00\xff" {
		t.Fatalf("AllFiles = %+v", fcs)
	}
	if fcs[0].Mode != "0600" {
		t.Fatalf("mode = %q", fcs[0].Mode)
	}
	// Path-escape and absolute paths rejected.
	for _, bad := range []string{"/etc/passwd", "~/../../etc/passwd", "a/../../b", ""} {
		if err := cs.PutFile("alice", FileCred{ID: "x", Path: bad, Mode: "0600", Data: []byte("x")}); err == nil {
			t.Errorf("PutFile path %q accepted", bad)
		}
	}
	// Bad mode rejected.
	if err := cs.PutFile("alice", FileCred{ID: "y", Path: "~/x", Mode: "notoctal", Data: []byte("x")}); err == nil {
		t.Error("bad mode accepted")
	}
	// Bad id rejected.
	if err := cs.PutFile("alice", FileCred{ID: "bad/id", Path: "~/x", Mode: "0600", Data: []byte("x")}); err == nil {
		t.Error("bad id accepted")
	}
	// The stored file is 0600 on disk.
	info, err := os.Stat(filepath.Join(cs.userDir("alice"), "file-ssh-id_ed25519.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("store file mode = %o", info.Mode().Perm())
	}
}

func TestKeyringCreds(t *testing.T) {
	cs, _ := New(t.TempDir())
	kcs, err := cs.AllKeyring("alice")
	if err != nil || len(kcs) != 0 {
		t.Fatalf("empty AllKeyring = %v, %v", kcs, err)
	}
	secret := []byte("oauth-json-{\"refresh\":\"tok\"}\x00\xff")
	if err := cs.PutKeyring("alice", KeyringCred{
		ID: "agy", Service: "antigravity", Username: "user@example.com",
		Label: "Password for 'user@example.com' on 'antigravity'", Secret: secret,
	}); err != nil {
		t.Fatal(err)
	}
	kcs, _ = cs.AllKeyring("alice")
	if len(kcs) != 1 || kcs[0].Service != "antigravity" || kcs[0].Username != "user@example.com" || string(kcs[0].Secret) != string(secret) {
		t.Fatalf("AllKeyring = %+v", kcs)
	}
	// Cross-user isolation.
	if k2, _ := cs.AllKeyring("bob"); len(k2) != 0 {
		t.Fatal("cross-user keyring leak")
	}
	// Refresh (replace) read back.
	cs.PutKeyring("alice", KeyringCred{ID: "agy", Service: "antigravity", Username: "user@example.com", Label: "l", Secret: []byte("new")})
	kcs, _ = cs.AllKeyring("alice")
	if string(kcs[0].Secret) != "new" {
		t.Fatalf("refresh not read back: %q", kcs[0].Secret)
	}
	// Control chars / empties rejected (they'd break the TSV delivery).
	for _, bad := range []KeyringCred{
		{ID: "x", Service: "a\tb", Username: "u", Secret: []byte("s")},
		{ID: "x", Service: "a", Username: "u\nv", Secret: []byte("s")},
		{ID: "x", Service: "", Username: "u", Secret: []byte("s")},
		{ID: "x", Service: "a", Username: "", Secret: []byte("s")},
		{ID: "x", Service: "a", Username: "u", Label: "l\nl", Secret: []byte("s")},
		{ID: "x", Service: "a", Username: "u", Secret: nil},
		{ID: "bad/id", Service: "a", Username: "u", Secret: []byte("s")},
	} {
		if err := cs.PutKeyring("alice", bad); err == nil {
			t.Errorf("PutKeyring accepted invalid %+v", bad)
		}
	}
	// List + delete round-trip.
	items, _ := cs.List("alice")
	found := false
	for _, it := range items {
		if it.Kind == "keyring" && it.ID == "agy" && strings.Contains(it.Name, "antigravity") {
			found = true
		}
	}
	if !found {
		t.Fatalf("keyring item missing from List: %+v", items)
	}
	if err := cs.Delete("alice", "keyring", "agy"); err != nil {
		t.Fatal(err)
	}
	if kcs, _ := cs.AllKeyring("alice"); len(kcs) != 0 {
		t.Fatal("keyring not deleted")
	}
}

func TestListAndDelete(t *testing.T) {
	cs, _ := New(t.TempDir())
	cs.PutGit("alice", GitCred{Host: "github.com", Username: "u", Token: "t"})
	cs.PutEnv("alice", "GH_TOKEN", "x")
	cs.PutFile("alice", FileCred{ID: "gcloud-adc", Path: "~/.config/gcloud/adc.json", Mode: "0600", Data: []byte("{}")})

	items, _ := cs.List("alice")
	if len(items) != 3 {
		t.Fatalf("List = %+v", items)
	}
	kinds := map[string]string{}
	for _, it := range items {
		kinds[it.Kind] = it.Name
	}
	if kinds["git"] != "github.com" || kinds["env"] != "GH_TOKEN" || kinds["file"] != "~/.config/gcloud/adc.json" {
		t.Fatalf("List names = %+v", kinds)
	}

	// Delete each kind.
	if err := cs.Delete("alice", "env", "GH_TOKEN"); err != nil {
		t.Fatal(err)
	}
	if m, _ := cs.AllEnv("alice"); len(m) != 0 {
		t.Fatal("env not deleted")
	}
	if err := cs.Delete("alice", "file", "gcloud-adc"); err != nil {
		t.Fatal(err)
	}
	if fcs, _ := cs.AllFiles("alice"); len(fcs) != 0 {
		t.Fatal("file not deleted")
	}
	if err := cs.Delete("alice", "git", "github.com"); err != nil {
		t.Fatal(err)
	}
	// Deleting a missing item → ErrNotFound.
	if err := cs.Delete("alice", "env", "GH_TOKEN"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete missing = %v, want ErrNotFound", err)
	}
	// Unknown kind → error.
	if err := cs.Delete("alice", "bogus", "x"); err == nil {
		t.Fatal("unknown kind accepted")
	}
}
