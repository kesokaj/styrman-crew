// Package httpapi exposes styrman's REST + WebSocket API and serves the
// embedded UI. JSON follows the camelCase conventions from the json-api-style
// standard; errors are {"error":{"message":...}}. Every request carries an
// identity (Google email via the ingress, or the configured default on the
// loopback dev listener); projects, terminals, files, and history are all
// scoped to that user's own home and stores (multi-tenancy, ADR-019).
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/kesokaj/styrman-crew/internal/config"
	"github.com/kesokaj/styrman-crew/internal/conversation"
	"github.com/kesokaj/styrman-crew/internal/credstore"
	"github.com/kesokaj/styrman-crew/internal/gitrepo"
	"github.com/kesokaj/styrman-crew/internal/identity"
	"github.com/kesokaj/styrman-crew/internal/links"
	"github.com/kesokaj/styrman-crew/internal/orchestrator"
	"github.com/kesokaj/styrman-crew/internal/project"
	"github.com/kesokaj/styrman-crew/internal/terminal"
	"github.com/kesokaj/styrman-crew/internal/userstore"
	"github.com/kesokaj/styrman-crew/web"
)

// Version is stamped via -ldflags at build time.
var Version = "dev"

// userSpace holds one user's metadata stores and profile.
type userSpace struct {
	projects *project.Store
	terms    *conversation.Store
	profile  *profileStore
}

// Server wires the API handlers to their dependencies.
type Server struct {
	cfg    *config.Config
	orch   *orchestrator.Orchestrator
	tmux   *conversation.Manager
	bridge *terminal.Bridge
	users  *userstore.Store
	logger *slog.Logger
	links  *links.Store

	spacesMu sync.Mutex
	spaces   map[string]*userSpace

	// proxyMu/proxies cache port-preview reverse proxies (see proxy.go).
	proxyMu sync.Mutex
	proxies map[string]*httputil.ReverseProxy

	// sharesMu serializes setShares' store-write + link-mint/delete pair so
	// concurrent PUTs can't interleave into "shares without a link" (or the
	// reverse). Consistency only — authz re-checks the list on every request.
	sharesMu sync.Mutex

	// creds is the credential broker's store (ADR-030); nil disables the
	// onboarding endpoints. Set by main after construction.
	creds *credstore.Store

	// deletingMu guards both maps (key user+"/"+projectID): deleting tracks
	// in-flight background destroys so the UI can show a "deleting" state, a
	// repeated DELETE is idempotent, and boot paths refuse to resurrect the
	// container mid-destroy; deleteErrs keeps the last failed destroy's
	// message so a 202'd delete that later fails is not silently invisible.
	deletingMu sync.Mutex
	deleting   map[string]bool
	deleteErrs map[string]string
}

// New creates the Server.
func New(cfg *config.Config, users *userstore.Store, orch *orchestrator.Orchestrator, logger *slog.Logger) *Server {
	s := &Server{
		cfg:        cfg,
		orch:       orch,
		tmux:       conversation.NewManager(orch, cfg),
		users:      users,
		logger:     logger,
		spaces:     map[string]*userSpace{},
		deleting:   map[string]bool{},
		deleteErrs: map[string]string{},
	}
	// Public preview-link store (best-effort: a load failure degrades to
	// no-links, never blocks server start).
	if ls, err := links.NewStore(filepath.Join(cfg.StateDir, "links.json")); err != nil {
		logger.Error("links store unavailable — public links disabled", "error", err)
	} else {
		s.links = ls
	}
	s.bridge = terminal.NewBridge(orch, cfg, logger, s.termStoreFor, s.ensureFor, s.guestUserFor)
	return s
}

// SetCredStore wires the credential broker's store so onboarding endpoints
// can write to it (ADR-030). Called by main after New.
//
// The broker's ADDRESS is delivered to each project as the STYRMAN_CRED_BROKER
// container env (a fixed per-bridge IP the server also binds on attach), so
// there is no per-project hint file — see orchestrator.brokerIPOnBridge.
func (s *Server) SetCredStore(cs *credstore.Store) { s.creds = cs }

// ensureFor lets the WS bridge boot a project it only knows by user+UUID.
func (s *Server) ensureFor(ctx context.Context, user, projectID string) (containerID, guestUser, dir string, err error) {
	// A terminal WS auto-reconnects when its container is force-removed —
	// without this guard it would resurrect the container mid-destroy and
	// leave an orphan microVM with no project record.
	if inFlight, _ := s.deletingState(user, projectID); inFlight {
		return "", "", "", fmt.Errorf("project is being deleted")
	}
	sp, err := s.space(user)
	if err != nil {
		return "", "", "", err
	}
	p, err := sp.projects.Get(projectID)
	if err != nil {
		return "", "", "", err
	}
	guest := sp.profile.GuestUser()
	id, err := s.orch.EnsureRunning(ctx, user, p.ID, orchestrator.RunOpts{Name: p.Name, Dir: p.Dir, GuestUser: guest})
	return id, guest, p.Dir, err
}

// guestUserFor resolves the in-guest username for a user.
func (s *Server) guestUserFor(user string) string {
	sp, err := s.space(user)
	if err != nil {
		return user
	}
	return sp.profile.GuestUser()
}

// space returns (lazily creating) a user's stores and home skeleton.
func (s *Server) space(user string) (*userSpace, error) {
	s.spacesMu.Lock()
	defer s.spacesMu.Unlock()
	if sp, ok := s.spaces[user]; ok {
		return sp, nil
	}
	// Homes are per-project now (ADR-031) and seeded by the image entrypoint,
	// so space() no longer creates a home — only the user's metadata dir.
	dir := s.cfg.UserStateDir(user)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	projects, err := project.NewStore(filepath.Join(dir, "projects.json"))
	if err != nil {
		return nil, err
	}
	terms, err := conversation.NewStore(filepath.Join(dir, "conversations.json"))
	if err != nil {
		return nil, err
	}
	prof, err := newProfileStore(filepath.Join(dir, "profile.json"), user)
	if err != nil {
		return nil, err
	}
	sp := &userSpace{projects: projects, terms: terms, profile: prof}
	s.spaces[user] = sp
	return sp, nil
}

// termStoreFor adapts the per-user terminal store for the WS bridge.
func (s *Server) termStoreFor(user string) (terminal.TermStore, error) {
	sp, err := s.space(user)
	if err != nil {
		return nil, err
	}
	return sp.terms, nil
}

// Routes returns the root handler with the configured auth mode.
func (s *Server) Routes() http.Handler { return s.RoutesWithAuth(s.cfg.AuthMode) }

// RoutesWithAuth builds the handler with an explicit auth mode — the dev
// listener (loopback-published, see config.DevAddr) uses "none".
func (s *Server) RoutesWithAuth(authMode string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/projects", s.listProjects)
	mux.HandleFunc("POST /api/projects", s.createProject)
	mux.HandleFunc("GET /api/projects/{project}", s.getProject)
	mux.HandleFunc("DELETE /api/projects/{project}", s.deleteProject)
	mux.HandleFunc("POST /api/projects/{project}/start", s.startProject)
	mux.HandleFunc("POST /api/projects/{project}/stop", s.stopProject)
	mux.HandleFunc("GET /api/projects/{project}/stats", s.projectStats)
	// Git side panel — a real git client (runs in the guest VM).
	mux.HandleFunc("GET /api/projects/{project}/git/status", s.gitStatus)
	mux.HandleFunc("GET /api/projects/{project}/git/diff", s.gitDiff)
	mux.HandleFunc("POST /api/projects/{project}/git/stage", s.gitStage)
	mux.HandleFunc("POST /api/projects/{project}/git/unstage", s.gitUnstage)
	mux.HandleFunc("POST /api/projects/{project}/git/discard", s.gitDiscard)
	mux.HandleFunc("POST /api/projects/{project}/git/commit", s.gitCommit)
	mux.HandleFunc("POST /api/projects/{project}/git/push", s.gitPush)
	// Terminals: durable records, one tmux session each (ADR-016).
	mux.HandleFunc("GET /api/projects/{project}/terminals", s.listTerminals)
	mux.HandleFunc("POST /api/projects/{project}/terminals", s.createTerminal)
	mux.HandleFunc("DELETE /api/projects/{project}/terminals", s.deleteAllTerminals)
	mux.HandleFunc("PATCH /api/projects/{project}/terminals/{term}", s.updateTerminal)
	mux.HandleFunc("DELETE /api/projects/{project}/terminals/{term}", s.deleteTerminal)
	mux.HandleFunc("POST /api/projects/{project}/terminals/{term}/pause", s.pauseTerminal)
	mux.Handle("GET /ws/term/{project}/{term}", s.bridge)
	mux.HandleFunc("GET /ws/setup", s.bridge.ServeSetup)
	// Ports + preview proxy. /p/... are the shareable links; /proxy/... is
	// the terminal-independent form. Per-method registration: a method-less
	// pattern conflicts with the "GET /" SPA catch-all.
	mux.HandleFunc("GET /api/projects/{project}/ports", s.listPorts)
	// Port-preview sharing (owner grants specific Google emails access to ONE
	// exposed port).
	mux.HandleFunc("GET /api/projects/{project}/ports/{port}/shares", s.getShares)
	mux.HandleFunc("PUT /api/projects/{project}/ports/{port}/shares", s.setShares)
	// Public TTL preview links (owner mints/lists/revokes; the link itself is
	// public — see the /link/{token} route below).
	mux.HandleFunc("POST /api/projects/{project}/ports/{port}/links", s.createLink)
	mux.HandleFunc("GET /api/projects/{project}/links", s.listLinks)
	mux.HandleFunc("DELETE /api/projects/{project}/links/{token}", s.revokeLink)
	// Public capability route (authGuard-exempt; token-validated in serveLink).
	for _, m := range []string{"GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"} {
		mux.HandleFunc(m+" /link/{token}/", s.serveLink)
	}
	mux.HandleFunc("GET /link/{token}", redirectSlash)
	// Private share route: like /link but served on the OAuth-gated private
	// host — authGuard resolves the visitor's identity, serveShared checks it
	// against the port's shared-email list.
	for _, m := range []string{"GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"} {
		mux.HandleFunc(m+" /shared/{token}/", s.serveShared)
	}
	mux.HandleFunc("GET /shared/{token}", redirectSlash)
	for _, m := range []string{"GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"} {
		mux.HandleFunc(m+" /proxy/{project}/{port}/", s.proxyPort)
		mux.HandleFunc(m+" /p/{project}/{term}/port/{port}/", s.proxyPort)
	}
	mux.HandleFunc("GET /p/{project}/{term}/port/{port}", redirectSlash)
	mux.HandleFunc("GET /proxy/{project}/{port}", redirectSlash)
	// File explorer: the user's project workspace and their home.
	mux.HandleFunc("GET /api/projects/{project}/files", s.listFiles)
	mux.HandleFunc("GET /api/projects/{project}/file", s.readFile)
	mux.HandleFunc("PUT /api/projects/{project}/file", s.writeFile)
	mux.HandleFunc("DELETE /api/projects/{project}/file", s.deleteFile)
	mux.HandleFunc("POST /api/projects/{project}/file/rename", s.renameFile)
	mux.HandleFunc("POST /api/projects/{project}/mkdir", s.makeDir)
	mux.HandleFunc("POST /api/projects/{project}/upload", s.uploadFile)
	mux.HandleFunc("GET /api/projects/{project}/download", s.downloadFile)
	// The project's OWN home (ADR-031: homes are per-project). Pattern
	// carries "/home/" so scopeFor roots at ProjectHome.
	mux.HandleFunc("GET /api/projects/{project}/home/files", s.listFiles)
	mux.HandleFunc("GET /api/projects/{project}/home/file", s.readFile)
	mux.HandleFunc("PUT /api/projects/{project}/home/file", s.writeFile)
	mux.HandleFunc("DELETE /api/projects/{project}/home/file", s.deleteFile)
	mux.HandleFunc("POST /api/projects/{project}/home/file/rename", s.renameFile)
	mux.HandleFunc("POST /api/projects/{project}/home/mkdir", s.makeDir)
	mux.HandleFunc("POST /api/projects/{project}/home/upload", s.uploadFile)
	mux.HandleFunc("GET /api/projects/{project}/home/download", s.downloadFile)
	// What happened: terminal transcripts + working-tree diff.
	mux.HandleFunc("GET /api/projects/{project}/history", s.listHistory)
	mux.HandleFunc("GET /api/projects/{project}/history/search", s.searchHistory)
	mux.HandleFunc("GET /api/projects/{project}/history/{id}", s.readHistory)
	mux.HandleFunc("GET /api/projects/{project}/diff", s.projectDiff)
	// Checkpoints: whole-tree snapshots under refs/styrman/* (R13).
	mux.HandleFunc("GET /api/projects/{project}/checkpoints", s.listCheckpoints)
	mux.HandleFunc("POST /api/projects/{project}/checkpoints", s.createCheckpoint)
	mux.HandleFunc("POST /api/projects/{project}/checkpoints/{id}/restore", s.restoreCheckpoint)
	// Settings + admin.
	mux.HandleFunc("GET /api/notifications", s.listNotifications)
	mux.HandleFunc("GET /api/settings", s.settings)
	mux.HandleFunc("POST /api/settings/profile", s.updateProfile)
	mux.HandleFunc("PATCH /api/projects/{project}", s.renameProject)
	mux.HandleFunc("GET /api/settings/tools", s.toolDetails)
	mux.HandleFunc("PATCH /api/settings/runtime", s.updateRuntime)
	mux.HandleFunc("POST /api/settings/credentials/git", s.importGitCredential)
	mux.HandleFunc("GET /api/settings/credentials", s.credentialsList)
	mux.HandleFunc("POST /api/settings/credentials/import", s.importCredentials)
	mux.HandleFunc("POST /api/settings/credentials/token", s.putTokenCredential)
	mux.HandleFunc("DELETE /api/settings/credentials/{kind}/{name}", s.deleteCredential)
	mux.HandleFunc("POST /api/settings/users", s.addUser)
	mux.HandleFunc("DELETE /api/settings/users/{email}", s.removeUser)
	mux.HandleFunc("GET /api/setup/status", s.getSetupStatus)
	mux.HandleFunc("POST /api/setup/start", s.startSetup)
	mux.HandleFunc("POST /api/setup/done", s.finishSetup)
	mux.HandleFunc("DELETE /api/setup", s.discardSetup)
	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("GET /api/version", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"version": Version})
	})

	// The embedded SPA: real files served directly, everything else falls
	// back to the prerendered SPA shell so client-side routes deep-link.
	// Unknown /api and /ws paths get a JSON 404; directories are excluded so
	// the bundle can't be listing-browsed.
	dist, err := fs.Sub(web.Dist, "dist/client")
	if err != nil {
		panic("embedded UI missing: " + err.Error()) // built into the binary; cannot fail at runtime
	}
	fileServer := http.FileServerFS(dist)
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/ws/") {
			writeErr(w, http.StatusNotFound, fmt.Errorf("no such endpoint: %s", r.URL.Path))
			return
		}
		// A preview-only guest reaches authGuard for preview-shaped paths; any
		// that fall through to here (a malformed /proxy/… that didn't match the
		// proxy routes) must NOT be served the SPA shell (I1). Only real styrman
		// users get the app.
		if !identity.FromContext(r.Context()).Authorized {
			writeErr(w, http.StatusNotFound, fmt.Errorf("no such endpoint: %s", r.URL.Path))
			return
		}
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p != "" {
			if st, err := fs.Stat(dist, p); err == nil && !st.IsDir() {
				// Vite content-hashes /assets/* filenames — safe to cache
				// forever; a new build references new names.
				if strings.HasPrefix(p, "assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				} else {
					w.Header().Set("Cache-Control", "no-cache")
				}
				fileServer.ServeHTTP(w, r)
				return
			}
		}
		// The shell must never be cached: without this, browsers
		// heuristically cache it and keep loading a stale app after
		// deploys — invisible, maddening "my fix didn't deploy" bugs.
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFileFS(w, r, dist, "_shell.html")
	})
	// previewHostRewrite outermost so a <token>.preview host is mapped to the
	// public /link route before auth even looks at it; then auth (identity /
	// permission), then CSRF, then routing.
	return s.previewHostRewrite(s.authGuard(authMode, csrfGuard(mux)))
}

// previewHostRewrite maps a preview subdomain to its internal token route:
// <token>.STYRMAN_PREVIEW_BASE → /link/{token}/… (public, no login) and
// <token>.STYRMAN_PRIVATE_BASE → /shared/{token}/… (email-gated, OAuth at the
// ingress). The token lives in the subdomain, so the previewed app gets its
// OWN origin and root — its root-relative asset paths work unchanged. The two
// routes are distinct on purpose: the serve handlers reject a token of the
// wrong kind, so a private token pasted on the public host (or vice versa)
// can never change its auth model. No-op when neither base matches.
func (s *Server) previewHostRewrite(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if i := strings.IndexByte(host, ':'); i >= 0 {
			host = host[:i]
		}
		host = strings.ToLower(strings.TrimSuffix(host, ".")) // normalize (nit)
		bases := []struct{ base, route string }{
			{s.cfg.PreviewBase, "/link/"},
			{s.cfg.PrivateBase, "/shared/"},
		}
		// Longest base first: if one base is a suffix of the other (e.g.
		// crew.x + private.crew.x), the shorter would otherwise shadow the
		// longer and 404 every one of its links.
		if len(bases[1].base) > len(bases[0].base) {
			bases[0], bases[1] = bases[1], bases[0]
		}
		for _, m := range bases {
			if m.base == "" {
				continue
			}
			suffix := "." + m.base
			if !strings.HasSuffix(host, suffix) {
				continue
			}
			token := strings.TrimSuffix(host, suffix)
			if !links.ValidToken(token) {
				// A preview host whose subdomain isn't a live token must
				// serve NOTHING (not /api/health, not the SPA) — 404 (S6).
				http.NotFound(w, r)
				return
			}
			// Stash the original path so serveProxy's cold-attach redirect
			// bounces to the URL the visitor used, not the internal token
			// form (B1).
			r = r.WithContext(context.WithValue(r.Context(), origPathKey{}, r.URL.Path))
			r.URL.Path = m.route + token + r.URL.Path
			break
		}
		next.ServeHTTP(w, r)
	})
}

// origPathKey carries the pre-rewrite request path from previewHostRewrite to
// serveProxy (for the cold-attach redirect).
type origPathKey struct{}

// redirectSlash sends /p/x/1/port/3000 → …/3000/ so the proxied app's
// relative asset paths resolve under the prefix. The query string must
// survive the redirect (audit finding: a preview URL with ?params silently
// lost them, and browsers cache the 301).
func redirectSlash(w http.ResponseWriter, r *http.Request) {
	target := r.URL.Path + "/"
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}
	http.Redirect(w, r, target, http.StatusMovedPermanently)
}

// csrfGuard rejects cross-origin mutating requests. Browsers always send
// Origin on cross-origin POST/DELETE; same-origin requests either omit it or
// match Host. curl and other non-browser clients send no Origin and pass.
// (WebSocket origin checking is handled separately by websocket.Accept.)
func csrfGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Preview / link paths merely tunnel bytes to a guest app — styrman's
		// CSRF concern (protecting its OWN API) doesn't apply, and the JSON body
		// cap would truncate the app's uploads. A sandboxed preview also sends
		// Origin: null, which would 403 every mutation the app makes (B2).
		if isPreviewPath(r.URL.Path) || isLinkPath(r.URL.Path) || isSharedPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if origin := r.Header.Get("Origin"); origin != "" {
				if u, err := url.Parse(origin); err != nil || u.Host != r.Host {
					writeErr(w, http.StatusForbidden, fmt.Errorf("cross-origin request rejected"))
					return
				}
			}
			// Uploads carry real payloads; editor saves carry file content
			// (2 MiB read cap × JSON escaping overhead — without the higher
			// cap a 1–2 MiB file opens editable but can never save);
			// everything else is small JSON.
			limit := int64(1 << 20)
			switch {
			case strings.HasSuffix(r.URL.Path, "/upload"):
				limit = 512 << 20
			case strings.HasSuffix(r.URL.Path, "/file"):
				limit = 8 << 20
			}
			r.Body = http.MaxBytesReader(w, r.Body, limit)
		}
		next.ServeHTTP(w, r)
	})
}

// who returns the request's user and their space.
func (s *Server) who(r *http.Request) (identity.User, *userSpace, error) {
	u := identity.FromContext(r.Context())
	sp, err := s.space(u.Username)
	return u, sp, err
}

// getProjectFor validates that the user owns the named project.
func (s *Server) getProjectFor(r *http.Request) (identity.User, *userSpace, project.Project, error) {
	u, sp, err := s.who(r)
	if err != nil {
		return u, sp, project.Project{}, err
	}
	p, err := sp.projects.Get(r.PathValue("project"))
	return u, sp, p, err
}

type projectView struct {
	project.Project
	State string `json:"state"`
	// DeleteError carries the last failed background destroy's message —
	// the DELETE already answered 202, so this is the only way the user
	// learns the delete didn't stick.
	DeleteError string `json:"deleteError,omitempty"`
}

func (s *Server) listProjects(w http.ResponseWriter, r *http.Request) {
	u, sp, err := s.who(r)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	statuses, err := s.orch.Adopt(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	out := []projectView{}
	for _, p := range sp.projects.List() {
		state := "absent"
		if st, ok := statuses[u.Username+"/"+p.ID]; ok {
			state = st.State
		}
		inFlight, delErr := s.deletingState(u.Username, p.ID)
		if inFlight {
			state = "deleting"
		}
		out = append(out, projectView{Project: p, State: state, DeleteError: delErr})
	}
	writeJSON(w, http.StatusOK, map[string]any{"projects": out})
}

type createProjectRequest struct {
	Name string `json:"name"`
	// GitInit is accepted for compatibility and ignored: every project is a
	// git repository (R13).
	GitInit bool `json:"gitInit"`
	// GitHub: "" or "none" = local repo only; "private"/"public" = also
	// create a GitHub repo via the user's gh credentials and push.
	GitHub string `json:"github"`
	// CloneURL: clone an existing repository as the workspace instead of
	// initializing an empty one. Mutually exclusive with GitHub.
	CloneURL string `json:"cloneUrl"`
}

// cloneURLRe accepts the common remote shapes and nothing that could read
// as a git option or shell metacharacter. The clone itself passes the URL
// as a bare argv element after "--".
var cloneURLRe = regexp.MustCompile(`^(https://|ssh://|git@)[\w.@:/~+-]+$`)

// redactURL strips any userinfo (a token embedded as https://user:tok@host)
// before the URL reaches a log line (R10).
func redactURL(u string) string {
	if at := strings.LastIndex(u, "@"); at != -1 {
		if slash := strings.Index(u, "//"); slash != -1 && slash+2 < at {
			return u[:slash+2] + "***@" + u[at+1:]
		}
	}
	return u
}

func (s *Server) createProject(w http.ResponseWriter, r *http.Request) {
	u, sp, err := s.who(r)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	var req createProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid JSON: %w", err))
		return
	}
	if err := project.ValidateName(req.Name); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if req.CloneURL != "" && !cloneURLRe.MatchString(req.CloneURL) {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid clone URL"))
		return
	}
	p, err := sp.projects.Add(project.Project{Name: req.Name, GitInit: true, CreatedAt: time.Now().UTC()})
	if err != nil {
		writeErr(w, http.StatusConflict, err)
		return
	}
	// Per-project workspace dir (ADR-027) — server-controlled path outside
	// the shared home, bind-mounted into the microVM at ~/projects/<dir>.
	workdir := s.cfg.ProjectWorkspace(u.Username, p.ID)
	if err := os.MkdirAll(workdir, 0o700); err != nil {
		_ = sp.projects.Remove(p.ID)
		writeErr(w, http.StatusInternalServerError, errors.New("cannot create workspace"))
		return
	}
	// The per-project home (ADR-031) — created here too so the Files "Home"
	// scope works before the first boot; the entrypoint seeds it on boot.
	if err := os.MkdirAll(s.cfg.ProjectHome(u.Username, p.ID), 0o700); err != nil {
		_ = sp.projects.Remove(p.ID)
		writeErr(w, http.StatusInternalServerError, errors.New("cannot create project home"))
		return
	}
	// R13: the workspace is a git repo from second zero. A clone fills the
	// (still empty) directory in the background boot instead.
	if req.CloneURL == "" {
		if err := gitrepo.EnsureRepo(r.Context(), workdir); err != nil {
			_ = sp.projects.Remove(p.ID)
			// gitrepo errors embed git's output, which can carry host paths
			// (its own doc says it must not reach a response verbatim).
			s.logger.Error("project git init failed", "project", p.Name, "error", err)
			writeErr(w, http.StatusInternalServerError, errors.New("could not initialize the project repository"))
			return
		}
	}

	// Respond immediately and boot the microVM in the background: a
	// seconds-long POST proved fragile in browsers (connection reuse during
	// the docker network churn it causes), and the terminal attach waits for
	// readiness anyway (per-project lock serializes with this boot).
	guest := sp.profile.GuestUser()
	// No runtime image configured = handler-level tests: skip the boot (it
	// would otherwise reach the host's real Docker daemon and race test
	// cleanup with its writes). config.Load always sets an image.
	if s.cfg.Image == "" {
		s.logger.Warn("no runtime image configured — skipping microVM boot", "project", p.Name)
		writeJSON(w, http.StatusCreated, projectView{Project: p, State: "creating"})
		return
	}
	go func() {
		defer func() { // outlives the request — net/http's recover doesn't cover it
			if rec := recover(); rec != nil {
				s.logger.Error("background project boot panicked", "project", p.Name, "panic", rec)
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		// A delete can land between the 201 and this boot. Booting anyway
		// could win the per-project lock ordering against Destroy and
		// resurrect a container+network for a record that no longer exists —
		// an orphan microVM invisible to the UI (audit finding; same guard as
		// ensureFor and startProject).
		if inFlight, _ := s.deletingState(u.Username, p.ID); inFlight {
			s.logger.Info("background boot skipped — delete in flight", "project", p.Name)
			return
		}
		containerID, err := s.orch.EnsureRunning(ctx, u.Username, p.ID,
			orchestrator.RunOpts{Name: p.Name, Dir: p.Dir, GuestUser: guest})
		if err != nil {
			s.logger.Error("background project boot failed", "project", p.Name, "error", err)
			// R13 must hold even if the microVM never booted: a clone can't
			// run without the guest, so leave an empty repo behind.
			if req.CloneURL != "" {
				if e := gitrepo.EnsureRepo(ctx, workdir); e != nil {
					s.logger.Error("R13 fallback init failed", "project", p.Name, "error", e)
				}
			}
			return
		}
		// Repo provisioning runs in-guest as the user so their gitconfig,
		// ssh key, and gh login apply. The bare `git init` case needs none
		// of that and already happened server-side before the 201.
		home := "/home/" + guest
		guestWorkdir := home + "/projects/" + p.Dir
		run := func(cmd ...string) (int, string, error) {
			return s.orch.Exec(ctx, containerID, orchestrator.ExecSpec{
				User:    guest,
				WorkDir: guestWorkdir,
				Env:     []string{"HOME=" + home, "USER=" + guest},
				Cmd:     cmd,
			})
		}
		switch {
		case req.CloneURL != "":
			// StrictHostKeyChecking=accept-new is deliberate TOFU: first
			// contact with a new host is trusted, later key changes are not.
			if code, out, err := run("git",
				"-c", "core.sshCommand=ssh -o StrictHostKeyChecking=accept-new",
				"clone", "--", req.CloneURL, "."); err != nil || code != 0 {
				s.logger.Warn("clone failed — falling back to empty repo",
					"project", p.Name, "url", redactURL(req.CloneURL), "exit", code, "output", out, "error", err)
				if err := gitrepo.EnsureRepo(ctx, workdir); err != nil {
					s.logger.Error("fallback git init failed", "project", p.Name, "error", err)
				}
			}
		case req.GitHub == "private" || req.GitHub == "public":
			// Needs the user's gh login (styrman-setup); logged, not fatal —
			// the local repo exists either way.
			if code, out, err := run("git", "commit", "--allow-empty", "-m", "Initialize styrman project"); err != nil || code != 0 {
				s.logger.Warn("initial commit failed", "project", p.Name, "exit", code, "output", out, "error", err)
			}
			vis := "--" + req.GitHub
			if code, out, err := run("bash", "-lc",
				"gh repo create "+p.Name+" "+vis+" --source=. --remote=origin --push"); err != nil || code != 0 {
				s.logger.Warn("github repo create failed (is gh signed in?)",
					"project", p.Name, "exit", code, "output", out, "error", err)
			}
		}
		// Pre-attach for port-preview while nothing depends on it (a fresh
		// attach can kill in-flight connections through this container).
		s.orch.EnsureSelfAttached(ctx, u.Username, p.ID)
	}()
	writeJSON(w, http.StatusCreated, projectView{Project: p, State: "creating"})
}

func (s *Server) getProject(w http.ResponseWriter, r *http.Request) {
	u, _, p, err := s.getProjectFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	st, err := s.orch.Status(r.Context(), u.Username, p.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	state := st.State
	inFlight, delErr := s.deletingState(u.Username, p.ID)
	if inFlight {
		state = "deleting"
	}
	writeJSON(w, http.StatusOK, projectView{Project: p, State: state, DeleteError: delErr})
}

// deleteProject responds 202 and destroys in the background: a Kata VM
// teardown can legitimately take minutes, far past any sane fetch timeout,
// and a dropped connection used to surface as "Failed to fetch" for a delete
// that then completed anyway. The UI polls the project list; the project
// shows state "deleting" until the record disappears.
func (s *Server) deleteProject(w http.ResponseWriter, r *http.Request) {
	u, sp, p, err := s.getProjectFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	key := u.Username + "/" + p.ID
	s.deletingMu.Lock()
	if s.deleting[key] {
		s.deletingMu.Unlock()
		w.WriteHeader(http.StatusAccepted) // already in flight — idempotent
		return
	}
	s.deleting[key] = true
	delete(s.deleteErrs, key) // a fresh attempt clears the previous failure
	s.deletingMu.Unlock()
	wipe := r.URL.Query().Get("wipe") == "true"
	go func() {
		defer func() {
			// This goroutine outlives the request and net/http's per-request
			// recover — a panic here would take down the whole server.
			if rec := recover(); rec != nil {
				s.logger.Error("project delete panicked", "project", p.Name, "panic", rec)
				s.deletingMu.Lock()
				s.deleteErrs[key] = "internal error during delete"
				s.deletingMu.Unlock()
			}
			s.deletingMu.Lock()
			delete(s.deleting, key)
			s.deletingMu.Unlock()
		}()
		// Detached context: nothing client-side can cancel a half-done
		// destroy and strand container-gone/record-present state.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		if err := s.orch.Destroy(ctx, u.Username, p.ID); err != nil {
			// Record stays; the project reappears in the list (flagged with
			// deleteError) and the user can retry.
			s.logger.Error("project destroy failed", "project", p.Name, "error", err)
			s.deletingMu.Lock()
			s.deleteErrs[key] = err.Error()
			s.deletingMu.Unlock()
			return
		}
		// wipe=true also removes the workspace, docker storage, transcripts,
		// and apt manifest — irreversible, so it is opt-in per request.
		if wipe {
			// Per-project storage (ADR-031/027) is server-controlled — the
			// whole per-project home (docker disk, transcripts, apt manifest,
			// dotfiles, any creds) AND the workspace. os.RemoveAll doesn't
			// follow symlinks out.
			for _, dir := range []string{
				s.cfg.ProjectHome(u.Username, p.ID),
				s.cfg.ProjectWorkspace(u.Username, p.ID),
			} {
				if err := os.RemoveAll(dir); err != nil {
					s.logger.Warn("wipe failed", "project", p.ID, "path", dir, "error", err)
				}
			}
		}
		if err := sp.projects.Remove(p.ID); err != nil {
			s.logger.Error("project record removal failed", "project", p.Name, "error", err)
			return
		}
		if err := sp.terms.DeleteProject(p.ID); err != nil {
			s.logger.Warn("terminal records cleanup failed", "project", p.Name, "error", err)
		}
		// S8: public links must not outlive the project they point at.
		if s.links != nil {
			if err := s.links.DeleteProject(p.ID); err != nil {
				s.logger.Warn("public-link cleanup failed", "project", p.Name, "error", err)
			}
		}
	}()
	w.WriteHeader(http.StatusAccepted)
}

// deletingState reports whether a background destroy is in flight for the
// project, plus the last destroy failure (empty if none), for the list/get
// views and the boot-path guards.
func (s *Server) deletingState(user, projectID string) (inFlight bool, lastErr string) {
	s.deletingMu.Lock()
	defer s.deletingMu.Unlock()
	key := user + "/" + projectID
	return s.deleting[key], s.deleteErrs[key]
}

func (s *Server) startProject(w http.ResponseWriter, r *http.Request) {
	u, sp, p, err := s.getProjectFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	// Same resurrection guard as ensureFor.
	if inFlight, _ := s.deletingState(u.Username, p.ID); inFlight {
		writeErr(w, http.StatusConflict, errors.New("project is being deleted"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	// Reuse sp from getProjectFor (N23: the old sp2, _ := s.space(...) ignored
	// the error and could nil-deref).
	if _, err := s.orch.EnsureRunning(ctx, u.Username, p.ID,
		orchestrator.RunOpts{Name: p.Name, Dir: p.Dir, GuestUser: sp.profile.GuestUser()}); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	go func() { // background pre-attach (see createProject)
		actx, acancel := context.WithTimeout(context.Background(), time.Minute)
		defer acancel()
		time.Sleep(time.Second) // let this response flush first
		s.orch.EnsureSelfAttached(actx, u.Username, p.ID)
	}()
	writeJSON(w, http.StatusOK, map[string]string{"state": "running"})
}

func (s *Server) stopProject(w http.ResponseWriter, r *http.Request) {
	u, _, p, err := s.getProjectFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := s.orch.Stop(ctx, u.Username, p.ID); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"state": "stopped"})
}

// terminalView is a durable terminal record merged with live tmux state.
type terminalView struct {
	conversation.Record
	Live     bool `json:"live"`
	Attached bool `json:"attached"`
	Bell     bool `json:"bell"`
}

func (s *Server) listTerminals(w http.ResponseWriter, r *http.Request) {
	u, sp, p, err := s.getProjectFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	live, err := s.tmux.Live(r.Context(), u.Username, p.ID, s.guestUserFor(u.Username))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	out := []terminalView{}
	for _, rec := range sp.terms.List(p.ID) {
		v := terminalView{Record: rec}
		if l, ok := live[rec.ID]; ok {
			v.Live = true
			v.Attached = l.Attached
			v.Bell = l.Bell
		}
		out = append(out, v)
	}
	writeJSON(w, http.StatusOK, map[string]any{"terminals": out})
}

func (s *Server) createTerminal(w http.ResponseWriter, r *http.Request) {
	_, sp, p, err := s.getProjectFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	var body struct {
		Title string `json:"title"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body) // empty body = default title
	rec, err := sp.terms.Create(p.ID, clampTitle(body.Title))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, terminalView{Record: rec})
}

// updateTerminal updates a terminal record: title, autostart command, or
// both (PATCH semantics — absent fields are untouched; command "" clears).
func (s *Server) updateTerminal(w http.ResponseWriter, r *http.Request) {
	_, sp, p, err := s.getProjectFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	var body struct {
		Title   *string `json:"title"`
		Command *string `json:"command"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if body.Title == nil && body.Command == nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("title or command required"))
		return
	}
	id := r.PathValue("term")
	if body.Title != nil {
		if *body.Title == "" {
			writeErr(w, http.StatusBadRequest, fmt.Errorf("title cannot be empty"))
			return
		}
		if err := sp.terms.Rename(p.ID, id, clampTitle(*body.Title)); err != nil {
			writeErr(w, http.StatusNotFound, err)
			return
		}
	}
	if body.Command != nil {
		// The command runs as the user, in their own VM — not a security
		// boundary, but keep it one sane line (chain with && or ;).
		cmd := strings.TrimSpace(*body.Command)
		if len(cmd) > 500 || strings.ContainsAny(cmd, "\n\r\x00") {
			writeErr(w, http.StatusBadRequest, fmt.Errorf("command must be one line, at most 500 characters"))
			return
		}
		if err := sp.terms.SetCommand(p.ID, id, cmd); err != nil {
			writeErr(w, http.StatusNotFound, err)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// pauseTerminal kills the tmux session but keeps the record: processes end,
// the transcript stays, and opening the terminal again starts a fresh shell
// under the same name. (While "live", closing the tab already keeps
// everything running — that's tmux — so pause is for reclaiming the session.)
func (s *Server) pauseTerminal(w http.ResponseWriter, r *http.Request) {
	u, sp, p, err := s.getProjectFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	id := r.PathValue("term")
	if _, ok := sp.terms.Get(p.ID, id); !ok {
		writeErr(w, http.StatusNotFound, fmt.Errorf("terminal %s not found", id))
		return
	}
	if err := s.tmux.KillSession(r.Context(), u.Username, p.ID, s.guestUserFor(u.Username), conversation.Prefix, id); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// deleteTerminal removes the record and kills its tmux session. The
// transcript in the shared home is left intact (ADR-016).
func (s *Server) deleteTerminal(w http.ResponseWriter, r *http.Request) {
	u, sp, p, err := s.getProjectFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	id := r.PathValue("term")
	if err := s.tmux.KillSession(r.Context(), u.Username, p.ID, s.guestUserFor(u.Username), conversation.Prefix, id); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if err := sp.terms.Delete(p.ID, id); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// deleteAllTerminals closes every terminal in the project.
func (s *Server) deleteAllTerminals(w http.ResponseWriter, r *http.Request) {
	u, sp, p, err := s.getProjectFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	for _, rec := range sp.terms.List(p.ID) {
		if err := s.tmux.KillSession(r.Context(), u.Username, p.ID, s.guestUserFor(u.Username), conversation.Prefix, rec.ID); err != nil {
			s.logger.Warn("kill session failed", "project", p.Name, "terminal", rec.ID, "error", err)
		}
		_ = sp.terms.Delete(p.ID, rec.ID)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	// Cheap daemon ping, not a full container Adopt — /api/health is exempt
	// from auth for the container's own probe, so it must not be an expensive
	// unauthenticated endpoint a shared-bridge VM could hammer.
	if err := s.orch.Ping(r.Context()); err != nil {
		writeErr(w, http.StatusServiceUnavailable, fmt.Errorf("docker unavailable: %w", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, err error) {
	if errors.Is(err, project.ErrNotFound) {
		status = http.StatusNotFound
	}
	writeJSON(w, status, map[string]any{"error": map[string]string{"message": err.Error()}})
}

// clampTitle bounds a terminal title (N25): titles land in conversations.json
// and every list response, so cap the length and strip control characters.
func clampTitle(t string) string {
	t = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, t)
	if len(t) > 120 {
		cut := 120
		for cut > 0 && !utf8.RuneStart(t[cut]) {
			cut-- // never split a multi-byte rune (audit nit)
		}
		t = t[:cut]
	}
	return t
}
