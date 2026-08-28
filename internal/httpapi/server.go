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

	"github.com/kesokaj/styrman-crew/internal/config"
	"github.com/kesokaj/styrman-crew/internal/conversation"
	"github.com/kesokaj/styrman-crew/internal/files"
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
}

// New creates the Server.
func New(cfg *config.Config, users *userstore.Store, orch *orchestrator.Orchestrator, logger *slog.Logger) *Server {
	s := &Server{
		cfg:    cfg,
		orch:   orch,
		tmux:   conversation.NewManager(orch, cfg),
		users:  users,
		logger: logger,
		spaces: map[string]*userSpace{},
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

// ensureFor lets the WS bridge boot a project it only knows by user+UUID.
func (s *Server) ensureFor(ctx context.Context, user, projectID string) (containerID, guestUser, dir string, err error) {
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
	if err := s.ensureHome(user); err != nil {
		return nil, err
	}
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

// ensureHome creates a user's home skeleton on first sight. SSH keys and
// agent credentials come from styrman-setup inside the guest, not here.
func (s *Server) ensureHome(user string) error {
	home := s.cfg.UserHome(user)
	if _, err := os.Stat(home); err == nil {
		return nil
	}
	s.logger.Info("creating home for new user", "user", user, "home", home)
	for _, d := range []string{
		"projects", ".ssh", ".styrman", ".config", ".cache",
		filepath.Join(".local", "bin"), filepath.Join(".local", "share", "keyrings"),
	} {
		if err := os.MkdirAll(filepath.Join(home, d), 0o755); err != nil {
			return err
		}
	}
	os.Chmod(home, 0o700)
	os.Chmod(filepath.Join(home, ".ssh"), 0o700)
	writeOnce := func(name, content string, mode os.FileMode) {
		p := filepath.Join(home, name)
		if _, err := os.Stat(p); os.IsNotExist(err) {
			_ = os.WriteFile(p, []byte(content), mode)
		}
	}
	writeOnce(".bashrc", "# styrman home\ncase $- in *i*) ;; *) return ;; esac\n[ -f ~/.bash_aliases ] && . ~/.bash_aliases\n[ -f ~/.bash_exports ] && . ~/.bash_exports\n[ -f ~/.bash_secrets ] && . ~/.bash_secrets\nexport PATH=\"$HOME/.local/bin:$HOME/go/bin:$PATH\"\nPS1=\"\\[\\e[36m\\]\\u@\\h\\[\\e[0m\\]:\\[\\e[34m\\]\\w\\[\\e[0m\\]\\$ \"\n", 0o644)
	writeOnce(".profile", "[ -n \"$BASH_VERSION\" ] && [ -f \"$HOME/.bashrc\" ] && . \"$HOME/.bashrc\"\n", 0o644)
	writeOnce(".bash_aliases", "alias ll=\"ls -la\"\n", 0o644)
	writeOnce(".bash_exports", "export EDITOR=vim\n", 0o644)
	writeOnce(".bash_secrets", "# per-user secrets (never overwritten)\n", 0o600)
	return nil
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
	// Terminals: durable records, one tmux session each (ADR-016).
	mux.HandleFunc("GET /api/projects/{project}/terminals", s.listTerminals)
	mux.HandleFunc("POST /api/projects/{project}/terminals", s.createTerminal)
	mux.HandleFunc("DELETE /api/projects/{project}/terminals", s.deleteAllTerminals)
	mux.HandleFunc("PATCH /api/projects/{project}/terminals/{term}", s.renameTerminal)
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
	mux.HandleFunc("POST /api/projects/{project}/upload", s.uploadFile)
	mux.HandleFunc("GET /api/projects/{project}/download", s.downloadFile)
	mux.HandleFunc("GET /api/home/files", s.listFiles)
	mux.HandleFunc("GET /api/home/file", s.readFile)
	mux.HandleFunc("PUT /api/home/file", s.writeFile)
	mux.HandleFunc("POST /api/home/upload", s.uploadFile)
	mux.HandleFunc("GET /api/home/download", s.downloadFile)
	// What happened: terminal transcripts + working-tree diff.
	mux.HandleFunc("GET /api/projects/{project}/history", s.listHistory)
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
// relative asset paths resolve under the prefix.
func redirectSlash(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, r.URL.Path+"/", http.StatusMovedPermanently)
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
			// Uploads carry real payloads; everything else is small JSON.
			limit := int64(1 << 20)
			if strings.HasSuffix(r.URL.Path, "/upload") {
				limit = 512 << 20
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
		out = append(out, projectView{Project: p, State: state})
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
	// Confined create: refuses a symlinked ~/projects escaping the home.
	home := s.cfg.UserHome(u.Username)
	if err := files.MkdirAll(home, "projects/"+p.Dir); err != nil {
		_ = sp.projects.Remove(p.ID)
		writeErr(w, http.StatusInternalServerError, errors.New("cannot create workspace"))
		return
	}
	workdir, err := files.ContainedPath(home, "projects/"+p.Dir)
	if err != nil {
		_ = sp.projects.Remove(p.ID)
		writeErr(w, http.StatusInternalServerError, errors.New("workspace unavailable"))
		return
	}
	// R13: the workspace is a git repo from second zero. A clone fills the
	// (still empty) directory in the background boot instead.
	if req.CloneURL == "" {
		if err := gitrepo.EnsureRepo(workdir); err != nil {
			_ = sp.projects.Remove(p.ID)
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
	}

	// Respond immediately and boot the microVM in the background: a
	// seconds-long POST proved fragile in browsers (connection reuse during
	// the docker network churn it causes), and the terminal attach waits for
	// readiness anyway (per-project lock serializes with this boot).
	guest := sp.profile.GuestUser()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		containerID, err := s.orch.EnsureRunning(ctx, u.Username, p.ID,
			orchestrator.RunOpts{Name: p.Name, Dir: p.Dir, GuestUser: guest})
		if err != nil {
			s.logger.Error("background project boot failed", "project", p.Name, "error", err)
			// R13 must hold even if the microVM never booted: a clone can't
			// run without the guest, so leave an empty repo behind.
			if req.CloneURL != "" {
				if e := gitrepo.EnsureRepo(workdir); e != nil {
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
				if err := gitrepo.EnsureRepo(workdir); err != nil {
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
	writeJSON(w, http.StatusOK, projectView{Project: p, State: st.State})
}

func (s *Server) deleteProject(w http.ResponseWriter, r *http.Request) {
	u, sp, p, err := s.getProjectFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	// Detached context: a client that gives up must not cancel a half-done
	// destroy and strand container-gone/record-present state.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := s.orch.Destroy(ctx, u.Username, p.ID); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	// wipe=true also removes the workspace, docker storage, and transcripts
	// — irreversible, so it is opt-in per request.
	if r.URL.Query().Get("wipe") == "true" {
		home := s.cfg.UserHome(u.Username)
		// Confined removal: a guest that symlinked ~/projects at another
		// tenant's home must not make this RemoveAll recurse into it.
		for _, rel := range []string{
			"projects/" + p.Dir,
			".dockerd/" + p.ID + ".img",
			".dockerd/" + p.ID,
			".styrman/logs/" + p.ID,
		} {
			if err := files.RemoveAll(home, rel); err != nil {
				s.logger.Warn("wipe failed", "project", p.ID, "path", rel, "error", err)
			}
		}
	}
	if err := sp.projects.Remove(p.ID); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
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
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) startProject(w http.ResponseWriter, r *http.Request) {
	u, sp, p, err := s.getProjectFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
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
	// Snapshot after the container is down: the tree is quiescent (no agent
	// mid-write) and the request isn't blocked on git.
	go s.snapshot(u.Username, p, "project stopped")
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

func (s *Server) renameTerminal(w http.ResponseWriter, r *http.Request) {
	_, sp, p, err := s.getProjectFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	var body struct {
		Title string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Title == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("title required"))
		return
	}
	if err := sp.terms.Rename(p.ID, r.PathValue("term"), clampTitle(body.Title)); err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
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
	go s.snapshot(u.Username, p, "terminal "+id+" paused")
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
	go s.snapshot(u.Username, p, "terminal "+id+" closed")
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
	go s.snapshot(u.Username, p, "all terminals closed")
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
		t = t[:120]
	}
	return t
}
