// Package credbroker is the credential broker's HTTP surface (ADR-030): a
// credential-ONLY endpoint reachable by a project on its OWN bridge (the
// cloud metadata-server pattern). The caller is identified by network
// position — the source IP maps to the project that owns that bridge — so no
// secret-zero token is needed. It serves ONLY credentials; the main API
// (ingress-token-gated) is a separate listener.
//
// Slice 1: git/GitHub tokens. Other tools' adapters (docker/gcloud helpers,
// gh/npm env, claude apiKeyHelper, agy keyring seed) attach here later.
package credbroker

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"

	"github.com/kesokaj/styrman-crew/internal/credstore"
)

// Resolver maps a caller's source IP to the owning (user, project).
type Resolver interface {
	ResolveProjectByIP(ctx context.Context, ip string) (user, project string, err error)
}

// Broker serves credentials to projects over their bridge.
type Broker struct {
	store   *credstore.Store
	resolve Resolver
	logger  *slog.Logger
}

// New builds the broker.
func New(store *credstore.Store, resolve Resolver, logger *slog.Logger) *Broker {
	return &Broker{store: store, resolve: resolve, logger: logger}
}

// Handler returns the broker's HTTP routes.
func (b *Broker) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("ok"))
	})
	mux.HandleFunc("GET /git", b.git)
	return mux
}

// caller resolves the request's source IP to (user, project).
func (b *Broker) caller(r *http.Request) (user, project string, ok bool) {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}
	u, p, err := b.resolve.ResolveProjectByIP(r.Context(), ip)
	if err != nil {
		b.logger.Warn("cred broker: unresolved caller", "ip", ip, "error", err)
		return "", "", false
	}
	return u, p, true
}

// git serves the git/GitHub credential for ?host= (default github.com) to
// the calling project. Response shape matches what the in-guest
// git-credential helper expects.
func (b *Broker) git(w http.ResponseWriter, r *http.Request) {
	user, project, ok := b.caller(r)
	if !ok {
		http.Error(w, "unknown caller", http.StatusForbidden)
		return
	}
	host := r.URL.Query().Get("host")
	if host == "" {
		host = "github.com"
	}
	c, err := b.store.GetGit(user, host)
	if err != nil {
		// Not onboarded for this host — tell git "no credential" (404) so it
		// falls through rather than erroring the operation.
		http.Error(w, "no credential", http.StatusNotFound)
		return
	}
	b.logger.Info("cred broker: served git credential", "user", user, "project", project, "host", host)
	writeJSON(w, map[string]string{"username": c.Username, "token": c.Token})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
