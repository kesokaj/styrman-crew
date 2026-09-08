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

// LanChecker answers whether a project may currently reach the local network
// (ADR-066 part 4). It is a separate interface from Resolver because the type
// that can answer it is not the one that resolves IPs: the orchestrator has
// no view of Project.LanAccess, while the API server holds both the per-user
// project stores and root's last word.
type LanChecker interface {
	// LanExposed reports whether the project has EVER had local-network
	// access. It is a LATCH on the project record, and it must not consult
	// root's firewall status at all.
	//
	// This comment used to describe the opposite — a union of the record,
	// root's applied set and "cannot confirm", citing a netpolicy.Exposed
	// that no longer exists. That rule was superseded by ADR-066's latch
	// amendment precisely because the live-state question has no reliable
	// answer: a failed revocation leaves the rule present while the project
	// drops out of root's applied set, which handed the key to a project that
	// still had LAN reach. The stale comment had no runtime effect (the
	// wrapper below fails closed regardless) but it told the next
	// implementer to build the version that was removed (audit 2026-09-08).
	LanExposed(user, project string) (bool, error)
}

// isHostReachingFile delegates to credstore so the broker and the API's
// prune cannot disagree about which credentials are host-reaching — they did,
// on "./"-spelled paths (security review, 2026-09-08).
//
// Withholding these from a LAN-enabled project closes the transitive path
// ADR-065 follow-up (g) recorded: project -> a LAN host that trusts the
// operator's key -> back to this one. It is NOT what keeps a project off this
// host; this host has no authorized_keys, verified 2026-09-08.
func isHostReachingFile(path string) bool { return credstore.IsHostReaching(path) }

// Broker serves credentials to projects over their bridge.
type Broker struct {
	store   *credstore.Store
	resolve Resolver
	lan     LanChecker
	logger  *slog.Logger
}

// New builds the broker. lan may be nil only in tests; a nil checker is
// treated as "cannot determine" and therefore withholds, so a wiring mistake
// costs ssh push loudly rather than granting silently.
func New(store *credstore.Store, resolve Resolver, lan LanChecker, logger *slog.Logger) *Broker {
	return &Broker{store: store, resolve: resolve, lan: lan, logger: logger}
}

// lanExposed is the fail-closed wrapper: any inability to answer withholds.
func (b *Broker) lanExposed(user, project string) bool {
	if b.lan == nil {
		b.logger.Error("cred broker: no LAN checker wired — withholding host-reaching credentials", "user", user, "project", project)
		return true
	}
	exposed, err := b.lan.LanExposed(user, project)
	if err != nil {
		b.logger.Error("cred broker: cannot determine LAN exposure — withholding host-reaching credentials",
			"user", user, "project", project, "error", err)
		return true
	}
	return exposed
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
	// Project-scoped variables (ADR-043) merge OVER the user-wide tier — the
	// caller's own project is authoritative on a shared key. Only this
	// project's callers ever see them; other projects still get just the
	// user-wide map. Best-effort: a project-tier read error must not take the
	// user-wide credentials down with it.
	if pm, perr := b.store.ProjectEnv(user, project); perr == nil {
		for k, v := range pm {
			m[k] = v
		}
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
	// A LAN-enabled project is not served host-reaching credentials (ADR-066).
	// The check runs once per request rather than per file, and only when
	// there is something it could withhold.
	exposed := false
	for _, c := range fcs {
		if isHostReachingFile(c.Path) {
			exposed = b.lanExposed(user, project)
			break
		}
	}
	out := make([]fileOut, 0, len(fcs))
	withheld := 0
	for _, c := range fcs {
		if exposed && isHostReachingFile(c.Path) {
			withheld++
			continue
		}
		out = append(out, fileOut{Path: c.Path, Mode: c.Mode, Data: base64.StdEncoding.EncodeToString(c.Data)})
	}
	if withheld > 0 {
		b.logger.Info("cred broker: withheld host-reaching credentials from a LAN-enabled project",
			"user", user, "project", project, "withheld", withheld)
	}
	if len(out) > 0 {
		b.logger.Info("cred broker: served file credentials", "user", user, "project", project, "count", len(out))
	}
	writeJSON(w, out)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
