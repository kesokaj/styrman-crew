// Package credbroker is the credential broker's HTTP surface (ADR-030): a
// credential-ONLY endpoint reachable by a project on its OWN bridge (the
// cloud metadata-server pattern). The caller is identified by network
// position — the source IP maps to the project that owns that bridge — so no
// secret-zero token is needed. It serves ONLY credentials; the main API
// (ingress-token-gated) is a separate listener.
//
// Three tiers attach here (ADR-032): GET /git (git credential helper), GET /env
// (shell-variable creds — gh/npm/claude tokens, exported into every login
// shell), and GET /files (materialized files — gcloud ADC, ssh key). All are
// identified by network position; all serve the CURRENT copy, so a refresh in
// the store propagates to every project on its next login shell.
package credbroker

import (
	"context"
	"encoding/base64"
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
	mux.HandleFunc("GET /env", b.env)
	mux.HandleFunc("GET /files", b.files)
	mux.HandleFunc("GET /keyring", b.keyring)
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

// env serves the calling project's env-token credentials (GH_TOKEN, NPM_TOKEN,
// CLAUDE_CODE_OAUTH_TOKEN, …) as a key→value map. The in-guest login snippet
// exports each into the shell. Always 200 (an empty map when nothing is
// onboarded) so the snippet stays quiet.
func (b *Broker) env(w http.ResponseWriter, r *http.Request) {
	user, project, ok := b.caller(r)
	if !ok {
		http.Error(w, "unknown caller", http.StatusForbidden)
		return
	}
	m, err := b.store.AllEnv(user)
	if err != nil {
		http.Error(w, "store error", http.StatusInternalServerError)
		return
	}
	if len(m) > 0 {
		b.logger.Info("cred broker: served env credentials", "user", user, "project", project, "count", len(m))
	}
	writeJSON(w, m)
}

// fileOut is the wire shape for a file credential (base64 data).
type fileOut struct {
	Path string `json:"path"`
	Mode string `json:"mode"`
	Data string `json:"data"` // base64
}

// files serves the calling project's file credentials (gcloud ADC, ssh key, …).
// The in-guest login snippet writes each into the home at its mode. Always 200
// with a (possibly empty) array.
func (b *Broker) files(w http.ResponseWriter, r *http.Request) {
	user, project, ok := b.caller(r)
	if !ok {
		http.Error(w, "unknown caller", http.StatusForbidden)
		return
	}
	fcs, err := b.store.AllFiles(user)
	if err != nil {
		http.Error(w, "store error", http.StatusInternalServerError)
		return
	}
	out := make([]fileOut, 0, len(fcs))
	for _, c := range fcs {
		out = append(out, fileOut{Path: c.Path, Mode: c.Mode, Data: base64.StdEncoding.EncodeToString(c.Data)})
	}
	if len(out) > 0 {
		b.logger.Info("cred broker: served file credentials", "user", user, "project", project, "count", len(out))
	}
	writeJSON(w, out)
}

// keyringOut is the wire shape for a Secret Service item (base64 secret).
type keyringOut struct {
	Service  string `json:"service"`
	Username string `json:"username"`
	Label    string `json:"label"`
	Secret   string `json:"secret"` // base64
}

// keyring serves the calling project's Secret Service items (agy's OAuth
// token, ADR-032 keyring tier). The in-guest login snippet secret-tool-stores
// each into the project's unlocked gnome-keyring. Always 200 with a (possibly
// empty) array.
func (b *Broker) keyring(w http.ResponseWriter, r *http.Request) {
	user, project, ok := b.caller(r)
	if !ok {
		http.Error(w, "unknown caller", http.StatusForbidden)
		return
	}
	kcs, err := b.store.AllKeyring(user)
	if err != nil {
		http.Error(w, "store error", http.StatusInternalServerError)
		return
	}
	out := make([]keyringOut, 0, len(kcs))
	for _, c := range kcs {
		out = append(out, keyringOut{Service: c.Service, Username: c.Username, Label: c.Label,
			Secret: base64.StdEncoding.EncodeToString(c.Secret)})
	}
	if len(out) > 0 {
		b.logger.Info("cred broker: served keyring credentials", "user", user, "project", project, "count", len(out))
	}
	writeJSON(w, out)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
