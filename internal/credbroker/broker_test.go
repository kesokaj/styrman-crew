package credbroker

import (
	"context"
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

func newBroker(t *testing.T, r Resolver) (*Broker, *credstore.Store) {
	t.Helper()
	cs, err := credstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return New(cs, r, slog.New(slog.DiscardHandler)), cs
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
