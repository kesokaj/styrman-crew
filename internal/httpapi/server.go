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
	"strings"
	"sync"
	"time"

	"github.com/kesokaj/styrman/internal/config"
	"github.com/kesokaj/styrman/internal/conversation"
	"github.com/kesokaj/styrman/internal/identity"
	"github.com/kesokaj/styrman/internal/orchestrator"
	"github.com/kesokaj/styrman/internal/project"
	"github.com/kesokaj/styrman/internal/terminal"
	"github.com/kesokaj/styrman/internal/userstore"
	"github.com/kesokaj/styrman/web"
)

// Version is stamped via -ldflags at build time.
var Version = "dev"

// userSpace holds one user's metadata stores.
type userSpace struct {
	projects *project.Store
	terms    *conversation.Store
}

// Server wires the API handlers to their dependencies.
type Server struct {
	cfg    *config.Config
	orch   *orchestrator.Orchestrator
	tmux   *conversation.Manager
	bridge *terminal.Bridge
	users  *userstore.Store
	logger *slog.Logger

	spacesMu sync.Mutex
	spaces   map[string]*userSpace

	// proxyMu/proxies cache port-preview reverse proxies (see proxy.go).
	proxyMu sync.Mutex
	proxies map[string]*httputil.ReverseProxy
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
	s.bridge = terminal.NewBridge(orch, cfg, logger, s.termStoreFor)
	return s
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
	sp := &userSpace{projects: projects, terms: terms}
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
	// Settings + admin.
	mux.HandleFunc("GET /api/settings", s.settings)
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
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p != "" {
			if st, err := fs.Stat(dist, p); err == nil && !st.IsDir() {
				fileServer.ServeHTTP(w, r)
				return
			}
		}
		http.ServeFileFS(w, r, dist, "_shell.html")
	})
	// Auth outermost: identity/permission first, then CSRF, then routing.
	return s.authGuard(authMode, csrfGuard(mux))
}

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
		if st, ok := statuses[u.Username+"/"+p.Name]; ok {
			state = st.State
		}
		out = append(out, projectView{Project: p, State: state})
	}
	writeJSON(w, http.StatusOK, map[string]any{"projects": out})
}

type createProjectRequest struct {
	Name    string `json:"name"`
	GitInit bool   `json:"gitInit"`
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
	p := project.Project{Name: req.Name, GitInit: req.GitInit, CreatedAt: time.Now().UTC()}

	workdir := filepath.Join(s.cfg.UserHome(u.Username), "projects", req.Name)
	if err := os.MkdirAll(workdir, 0o755); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if err := sp.projects.Add(p); err != nil {
		writeErr(w, http.StatusConflict, err)
		return
	}

	// Boot the microVM now so the first terminal attach is instant and
	// creation errors surface here rather than mid-connect. On failure, roll
	// the registration back so the name can be retried; the workspace dir is
	// left in place — deleting user data on an error path is worse than a
	// leftover empty dir.
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	containerID, err := s.orch.EnsureRunning(ctx, u.Username, req.Name)
	if err != nil {
		_ = s.orch.Destroy(context.Background(), u.Username, req.Name)
		_ = sp.projects.Remove(req.Name)
		writeErr(w, http.StatusInternalServerError, err)
		return
	}

	if req.GitInit {
		// git init inside the guest as the user, so their gitconfig applies.
		home := "/home/" + u.Username
		code, out, err := s.orch.Exec(ctx, containerID, orchestrator.ExecSpec{
			User:    u.Username,
			WorkDir: home + "/projects/" + req.Name,
			Env:     []string{"HOME=" + home, "USER=" + u.Username},
			Cmd:     []string{"git", "init", "-b", "main"},
		})
		if err != nil || code != 0 {
			s.logger.Warn("git init failed", "project", req.Name, "exit", code, "output", out, "error", err)
		}
	}
	writeJSON(w, http.StatusCreated, projectView{Project: p, State: "running"})
}

func (s *Server) getProject(w http.ResponseWriter, r *http.Request) {
	u, _, p, err := s.getProjectFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	st, err := s.orch.Status(r.Context(), u.Username, p.Name)
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
	name := p.Name
	// Detached context: a client that gives up must not cancel a half-done
	// destroy and strand container-gone/record-present state.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := s.orch.Destroy(ctx, u.Username, name); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	// wipe=true also removes the workspace, docker storage, and transcripts
	// — irreversible, so it is opt-in per request.
	if r.URL.Query().Get("wipe") == "true" {
		home := s.cfg.UserHome(u.Username)
		for _, path := range []string{
			filepath.Join(home, "projects", name),
			filepath.Join(home, ".dockerd", name+".img"),
			filepath.Join(home, ".dockerd", name),
			filepath.Join(home, ".styrman", "logs", name),
		} {
			if err := os.RemoveAll(path); err != nil {
				s.logger.Warn("wipe failed", "project", name, "path", path, "error", err)
			}
		}
	}
	if err := sp.projects.Remove(name); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if err := sp.terms.DeleteProject(name); err != nil {
		s.logger.Warn("terminal records cleanup failed", "project", name, "error", err)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) startProject(w http.ResponseWriter, r *http.Request) {
	u, _, p, err := s.getProjectFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	if _, err := s.orch.EnsureRunning(ctx, u.Username, p.Name); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
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
	if err := s.orch.Stop(ctx, u.Username, p.Name); err != nil {
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
}

func (s *Server) listTerminals(w http.ResponseWriter, r *http.Request) {
	u, sp, p, err := s.getProjectFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	live, err := s.tmux.Live(r.Context(), u.Username, p.Name)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	out := []terminalView{}
	for _, rec := range sp.terms.List(p.Name) {
		v := terminalView{Record: rec}
		if l, ok := live[rec.ID]; ok {
			v.Live = true
			v.Attached = l.Attached
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
	rec, err := sp.terms.Create(p.Name, body.Title)
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
	if err := sp.terms.Rename(p.Name, r.PathValue("term"), body.Title); err != nil {
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
	if _, ok := sp.terms.Get(p.Name, id); !ok {
		writeErr(w, http.StatusNotFound, fmt.Errorf("terminal %s not found", id))
		return
	}
	if err := s.tmux.KillSession(r.Context(), u.Username, p.Name, conversation.Prefix, id); err != nil {
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
	if err := s.tmux.KillSession(r.Context(), u.Username, p.Name, conversation.Prefix, id); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if err := sp.terms.Delete(p.Name, id); err != nil {
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
	for _, rec := range sp.terms.List(p.Name) {
		if err := s.tmux.KillSession(r.Context(), u.Username, p.Name, conversation.Prefix, rec.ID); err != nil {
			s.logger.Warn("kill session failed", "project", p.Name, "terminal", rec.ID, "error", err)
		}
		_ = sp.terms.Delete(p.Name, rec.ID)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	if _, err := s.orch.Adopt(r.Context()); err != nil {
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
