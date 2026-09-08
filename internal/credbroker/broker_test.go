package credbroker

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kesokaj/styrman-crew/internal/credstore"
)

type fakeResolver struct {
	user, project string
	err           error
}

func (f fakeResolver) ResolveProjectByIP(_ context.Context, _ string) (string, string, error) {
	return f.user, f.project, f.err
}

// fakeLan stands in for *httpapi.Server. The zero value is "not exposed",
// which is what every pre-ADR-066 test expects.
type fakeLan struct {
	exposed bool
	err     error
}

func (f fakeLan) LanExposed(_, _ string) (bool, error) { return f.exposed, f.err }

func newBroker(t *testing.T, r Resolver) (*Broker, *credstore.Store) {
	t.Helper()
	return newBrokerLan(t, r, fakeLan{})
}

func newBrokerLan(t *testing.T, r Resolver, lan LanChecker) (*Broker, *credstore.Store) {
	t.Helper()
	cs, err := credstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return New(cs, r, lan, slog.New(slog.DiscardHandler)), cs
}

func TestBrokerGit(t *testing.T) {
	b, cs := newBroker(t, fakeResolver{user: "alice", project: "p1"})
	cs.PutGit("alice", credstore.GitCred{Host: "github.com", Username: "x-access-token", Token: "sekret"})
	srv := httptest.NewServer(b.Handler())
	defer srv.Close()

	// Known caller + onboarded host → the credential.
	resp, err := http.Get(srv.URL + "/git?host=github.com")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("git = %d, want 200", resp.StatusCode)
	}
	var got map[string]string
	json.NewDecoder(resp.Body).Decode(&got)
	resp.Body.Close()
	if got["token"] != "sekret" || got["username"] != "x-access-token" {
		t.Fatalf("payload = %+v", got)
	}

	// Onboarded host defaults to github.com when ?host= omitted.
	resp, _ = http.Get(srv.URL + "/git")
	if resp.StatusCode != 200 {
		t.Errorf("default host = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()

	// A host with no stored credential → 404 (git falls through, not errors).
	resp, _ = http.Get(srv.URL + "/git?host=gitlab.com")
	if resp.StatusCode != 404 {
		t.Errorf("unknown host = %d, want 404", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestBrokerEnv(t *testing.T) {
	b, cs := newBroker(t, fakeResolver{user: "alice", project: "p1"})
	cs.PutEnv("alice", "GH_TOKEN", "ghs_1")
	cs.PutEnv("alice", "NPM_TOKEN", "npm_2")
	srv := httptest.NewServer(b.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/env")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("env = %d, want 200", resp.StatusCode)
	}
	var got map[string]string
	json.NewDecoder(resp.Body).Decode(&got)
	resp.Body.Close()
	if got["GH_TOKEN"] != "ghs_1" || got["NPM_TOKEN"] != "npm_2" {
		t.Fatalf("env payload = %+v", got)
	}
}

func TestBrokerFiles(t *testing.T) {
	b, cs := newBroker(t, fakeResolver{user: "alice", project: "p1"})
	cs.PutFile("alice", credstore.FileCred{ID: "ssh-id_ed25519", Path: "~/.ssh/id_ed25519", Mode: "0600", Data: []byte("KEYBYTES")})
	srv := httptest.NewServer(b.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/files")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("files = %d, want 200", resp.StatusCode)
	}
	var got []struct{ Path, Mode, Data string }
	json.NewDecoder(resp.Body).Decode(&got)
	resp.Body.Close()
	if len(got) != 1 || got[0].Path != "~/.ssh/id_ed25519" || got[0].Mode != "0600" {
		t.Fatalf("files payload = %+v", got)
	}
	// Data is base64 of the raw bytes.
	raw, err := base64.StdEncoding.DecodeString(got[0].Data)
	if err != nil || string(raw) != "KEYBYTES" {
		t.Fatalf("files data = %q (%v)", got[0].Data, err)
	}
}

// An unresolved caller gets 403 on every credential endpoint, not just /git.
func TestBrokerEnvFilesForbidden(t *testing.T) {
	b, _ := newBroker(t, fakeResolver{err: errors.New("no bridge")})
	srv := httptest.NewServer(b.Handler())
	defer srv.Close()
	for _, path := range []string{"/env", "/files"} {
		resp, _ := http.Get(srv.URL + path)
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s unresolved = %d, want 403", path, resp.StatusCode)
		}
		resp.Body.Close()
	}
}

func TestBrokerUnknownCaller(t *testing.T) {
	b, _ := newBroker(t, fakeResolver{err: errors.New("no project bridge owns it")})
	srv := httptest.NewServer(b.Handler())
	defer srv.Close()
	resp, _ := http.Get(srv.URL + "/git?host=github.com")
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("unresolved caller = %d, want 403", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestStoreRoundTripAndIsolation(t *testing.T) {
	cs, err := credstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cs.GetGit("alice", "github.com"); !errors.Is(err, credstore.ErrNotFound) {
		t.Fatalf("empty store = %v, want ErrNotFound", err)
	}
	cs.PutGit("alice", credstore.GitCred{Host: "github.com", Username: "u", Token: "t1"})
	// A different user must not see alice's credential.
	if _, err := cs.GetGit("bob", "github.com"); !errors.Is(err, credstore.ErrNotFound) {
		t.Fatal("cross-user credential leak")
	}
	// Replace (refresh) is read back.
	cs.PutGit("alice", credstore.GitCred{Host: "github.com", Username: "u", Token: "t2"})
	c, _ := cs.GetGit("alice", "github.com")
	if c.Token != "t2" {
		t.Fatalf("refresh not read back: %q", c.Token)
	}
}

// Project-scoped vars (ADR-043) merge OVER the user-wide tier for the calling
// project only — the project value wins on a shared key, and another
// project's vars never appear.
func TestBrokerEnvProjectMerge(t *testing.T) {
	b, cs := newBroker(t, fakeResolver{user: "alice", project: "p1"})
	cs.PutEnv("alice", "GH_TOKEN", "ghs_global")
	cs.PutEnv("alice", "SHARED", "global")
	cs.PutProjectEnv("alice", "p1", "SHARED", "project-wins")
	cs.PutProjectEnv("alice", "p1", "DATABASE_URL", "postgres://p1")
	cs.PutProjectEnv("alice", "p2", "OTHER_PROJECT", "must-not-leak")
	srv := httptest.NewServer(b.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/env")
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]string
	json.NewDecoder(resp.Body).Decode(&got)
	resp.Body.Close()
	if got["GH_TOKEN"] != "ghs_global" {
		t.Fatalf("user-wide var lost: %+v", got)
	}
	if got["SHARED"] != "project-wins" {
		t.Fatalf("project value must win on a shared key: %+v", got)
	}
	if got["DATABASE_URL"] != "postgres://p1" {
		t.Fatalf("project var missing: %+v", got)
	}
	if _, leaked := got["OTHER_PROJECT"]; leaked {
		t.Fatalf("another project's var leaked: %+v", got)
	}
}

func TestIsHostReachingFile(t *testing.T) {
	for _, c := range []struct {
		path string
		want bool
	}{
		{"~/.ssh/id_ed25519", true},
		{"~/.ssh/config", true},
		{"$HOME/.ssh/id_rsa", true},
		{".ssh/id_ed25519", true},
		{"~/.ssh", true},
		{"~/.config/gcloud/application_default_credentials.json", false},
		{"~/.claude/.credentials.json", false},
		{"~/.gemini/antigravity-cli/antigravity-oauth-token", false},
		{"~/.sshfoo/key", false},
		{"~/notes/.ssh-notes.md", false},
	} {
		if got := isHostReachingFile(c.path); got != c.want {
			t.Errorf("isHostReachingFile(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

// seedFiles puts one host-reaching and one ordinary credential in the store.
func seedFiles(t *testing.T, cs *credstore.Store) {
	t.Helper()
	if err := cs.PutFile("alice", credstore.FileCred{ID: "ssh-id", Path: "~/.ssh/id_ed25519", Mode: "0600", Data: []byte("KEY")}); err != nil {
		t.Fatal(err)
	}
	if err := cs.PutFile("alice", credstore.FileCred{ID: "adc", Path: "~/.config/gcloud/application_default_credentials.json", Mode: "0600", Data: []byte("ADC")}); err != nil {
		t.Fatal(err)
	}
}

func servedPaths(t *testing.T, b *Broker) []string {
	t.Helper()
	srv := httptest.NewServer(b.Handler())
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/files")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out []fileOut
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	paths := make([]string, 0, len(out))
	for _, f := range out {
		paths = append(paths, f.Path)
	}
	return paths
}

func hasSSH(paths []string) bool {
	for _, p := range paths {
		if isHostReachingFile(p) {
			return true
		}
	}
	return false
}

// A LAN-enabled project loses the SSH key and keeps everything else; the
// decision fails CLOSED, so an error or an unwired checker withholds too
// (ADR-066 part 4).
func TestFilesWithheldFromLanEnabledProject(t *testing.T) {
	r := fakeResolver{user: "alice", project: "p1"}

	b, cs := newBrokerLan(t, r, fakeLan{exposed: false})
	seedFiles(t, cs)
	paths := servedPaths(t, b)
	if len(paths) != 2 || !hasSSH(paths) {
		t.Fatalf("not LAN-exposed: want both credentials, got %v", paths)
	}

	b, cs = newBrokerLan(t, r, fakeLan{exposed: true})
	seedFiles(t, cs)
	paths = servedPaths(t, b)
	if len(paths) != 1 || paths[0] != "~/.config/gcloud/application_default_credentials.json" {
		t.Fatalf("LAN-exposed: want only the non-ssh credential, got %v", paths)
	}

	b, cs = newBrokerLan(t, r, fakeLan{err: errors.New("store is on fire")})
	seedFiles(t, cs)
	paths = servedPaths(t, b)
	if len(paths) != 1 || isHostReachingFile(paths[0]) {
		t.Fatalf("checker error must withhold, got %v", paths)
	}

	b, cs = newBrokerLan(t, r, nil)
	seedFiles(t, cs)
	paths = servedPaths(t, b)
	if len(paths) != 1 || isHostReachingFile(paths[0]) {
		t.Fatalf("nil checker must withhold, got %v", paths)
	}
}
