// Package httpapi exposes styrman's REST + WebSocket API and serves the
// embedded UI. JSON follows the camelCase conventions from the json-api-style
// standard; errors are {"error":{"message":...}}.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kesokaj/styrman/internal/config"
	"github.com/kesokaj/styrman/internal/conversation"
	"github.com/kesokaj/styrman/internal/orchestrator"
	"github.com/kesokaj/styrman/internal/project"
	"github.com/kesokaj/styrman/internal/terminal"
	"github.com/kesokaj/styrman/internal/userstore"
	"github.com/kesokaj/styrman/web"
)

// Version is stamped via -ldflags at build time.
var Version = "dev"

// Server wires the API handlers to their dependencies.
type Server struct {
	cfg       *config.Config
	projects  *project.Store
	orch      *orchestrator.Orchestrator
	tmux      *conversation.Manager
	convStore *conversation.Store
	bridge    *terminal.Bridge
	users     *userstore.Store
	logger    *slog.Logger

	// termMu/termReserved close the terminal-id TOCTOU: ids are derived from
	// live tmux sessions, but the session only exists after the WS attaches,
	// so rapid "+ New" clicks would get the same id (review finding).
	// Reservations expire after a minute.
	termMu       sync.Mutex
	termReserved map[string]map[string]time.Time
}

// New creates the Server.
func New(cfg *config.Config, projects *project.Store, convStore *conversation.Store, users *userstore.Store, orch *orchestrator.Orchestrator, logger *slog.Logger) *Server {
	return &Server{
		cfg:       cfg,
		projects:  projects,
		orch:      orch,
		tmux:      conversation.NewManager(orch, cfg),
		convStore: convStore,
		bridge:    terminal.NewBridge(orch, projects, convStore, cfg, logger),
		users:     users,
		logger:    logger,
	}
}

// Routes returns the root handler.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/projects", s.listProjects)
	mux.HandleFunc("POST /api/projects", s.createProject)
	mux.HandleFunc("GET /api/projects/{project}", s.getProject)
	mux.HandleFunc("DELETE /api/projects/{project}", s.deleteProject)
	mux.HandleFunc("POST /api/projects/{project}/start", s.startProject)
	mux.HandleFunc("POST /api/projects/{project}/stop", s.stopProject)
	mux.HandleFunc("GET /api/projects/{project}/conversations", s.listConversations)
	mux.HandleFunc("POST /api/projects/{project}/conversations", s.createConversation)
	mux.HandleFunc("PATCH /api/projects/{project}/conversations/{conv}", s.renameConversation)
	mux.HandleFunc("DELETE /api/projects/{project}/conversations/{conv}", s.deleteConversation)
	mux.HandleFunc("GET /api/projects/{project}/terminals", s.listTerminals)
	mux.HandleFunc("POST /api/projects/{project}/terminals", s.createTerminal)
	mux.HandleFunc("DELETE /api/projects/{project}/terminals/{term}", s.deleteTerminal)
	mux.Handle("GET /ws/term/{project}/{conv}", s.bridge)
	mux.HandleFunc("GET /ws/setup", s.bridge.ServeSetup)
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
	// Unknown /api and /ws paths get a JSON 404 — the SPA fallback would
	// otherwise hand 200 HTML to API clients (review finding); directories
	// are excluded so the bundle can't be listing-browsed.
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
	return authGuard(s.cfg.AuthMode, s.users, csrfGuard(mux))
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
			r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		}
		next.ServeHTTP(w, r)
	})
}

type projectView struct {
	project.Project
	State string `json:"state"`
}

func (s *Server) listProjects(w http.ResponseWriter, r *http.Request) {
	statuses, err := s.orch.Adopt(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	out := []projectView{}
	for _, p := range s.projects.List() {
		state := "absent"
		if st, ok := statuses[p.Name]; ok {
			state = st.State
		}
		out = append(out, projectView{Project: p, State: state})
	}
	writeJSON(w, http.StatusOK, map[string]any{"projects": out})
}

type createProjectRequest struct {
	Name    string `json:"name"`
	Agent   string `json:"agent"`
	GitInit bool   `json:"gitInit"`
}

func (s *Server) createProject(w http.ResponseWriter, r *http.Request) {
	var req createProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid JSON: %w", err))
		return
	}
	p := project.Project{Name: req.Name, Agent: req.Agent, GitInit: req.GitInit, CreatedAt: time.Now().UTC()}

	// Workspace dir first: the server runs as the home's owner, so a plain
	// mkdir gets the right ownership for the in-guest user (same uid).
	workdir := filepath.Join(s.cfg.HomeDir, "projects", req.Name)
	if err := project.ValidateName(req.Name); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := os.MkdirAll(workdir, 0o755); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if err := s.projects.Add(p); err != nil {
		writeErr(w, http.StatusConflict, err)
		return
	}

	// Boot the microVM now so the first terminal attach is instant and
	// creation errors surface here rather than mid-connect. On failure, roll
	// the registration back so the name can be retried (review finding); the
	// workspace dir is left in place — it may hold nothing yet, and deleting
	// user data on an error path is worse than a leftover empty dir.
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	containerID, err := s.orch.EnsureRunning(ctx, req.Name)
	if err != nil {
		_ = s.orch.Destroy(context.Background(), req.Name)
		_ = s.projects.Remove(req.Name)
		writeErr(w, http.StatusInternalServerError, err)
		return
	}

	if req.GitInit {
		// git init inside the guest as the user, so the shared home's
		// gitconfig (identity, defaults) applies.
		home := "/home/" + s.cfg.Username
		code, out, err := s.orch.Exec(ctx, containerID, orchestrator.ExecSpec{
			User:    s.cfg.Username,
			WorkDir: home + "/projects/" + req.Name,
			Env:     []string{"HOME=" + home, "USER=" + s.cfg.Username},
			Cmd:     []string{"git", "init", "-b", "main"},
		})
		if err != nil || code != 0 {
			s.logger.Warn("git init failed", "project", req.Name, "exit", code, "output", out, "error", err)
		}
	}
	writeJSON(w, http.StatusCreated, projectView{Project: p, State: "running"})
}

func (s *Server) getProject(w http.ResponseWriter, r *http.Request) {
	p, err := s.projects.Get(r.PathValue("project"))
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	st, err := s.orch.Status(r.Context(), p.Name)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, projectView{Project: p, State: st.State})
}

func (s *Server) deleteProject(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("project")
	if _, err := s.projects.Get(name); err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	// Detached context: a client that gives up (tab closed, timeout) must not
	// cancel a half-done destroy and strand container-gone/record-present
	// state (found by the e2e suite).
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := s.orch.Destroy(ctx, name); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	// wipe=true also removes the workspace and the project's docker storage
	// (both the ext4 image and the vfs-fallback dir) — irreversible, so it is
	// opt-in per request. Store contents are validated on load, so name is a
	// safe slug here; failures are logged, not swallowed.
	if r.URL.Query().Get("wipe") == "true" {
		for _, path := range []string{
			filepath.Join(s.cfg.HomeDir, "projects", name),
			filepath.Join(s.cfg.HomeDir, ".dockerd", name+".img"),
			filepath.Join(s.cfg.HomeDir, ".dockerd", name),
		} {
			if err := os.RemoveAll(path); err != nil {
				s.logger.Warn("wipe failed", "project", name, "path", path, "error", err)
			}
		}
	}
	if err := s.projects.Remove(name); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if err := s.convStore.DeleteProject(name); err != nil {
		s.logger.Warn("conversation records cleanup failed", "project", name, "error", err)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) startProject(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("project")
	if _, err := s.projects.Get(name); err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	if _, err := s.orch.EnsureRunning(ctx, name); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"state": "running"})
}

func (s *Server) stopProject(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("project")
	if _, err := s.projects.Get(name); err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	// Detached for the same reason as deleteProject.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := s.orch.Stop(ctx, name); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"state": "stopped"})
}

// conversationView is a durable record merged with live tmux state.
type conversationView struct {
	conversation.Record
	Live     bool `json:"live"`
	Attached bool `json:"attached"`
}

// listConversations returns durable records (ADR-011) — present even for
// stopped projects — merged with live tmux state when running.
func (s *Server) listConversations(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("project")
	if _, err := s.projects.Get(name); err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	live, err := s.tmux.LiveConversations(r.Context(), name)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	out := []conversationView{}
	for _, rec := range s.convStore.List(name) {
		v := conversationView{Record: rec}
		if l, ok := live[rec.ID]; ok {
			v.Live = true
			v.Attached = l.Attached
		}
		out = append(out, v)
	}
	writeJSON(w, http.StatusOK, map[string]any{"conversations": out})
}

// createConversation persists a durable record. The tmux session is created
// lazily on the first WebSocket attach.
func (s *Server) createConversation(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("project")
	if _, err := s.projects.Get(name); err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	var body struct {
		Title string `json:"title"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body) // empty body = default title
	rec, err := s.convStore.Create(name, body.Title)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, conversationView{Record: rec})
}

func (s *Server) renameConversation(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("project")
	if _, err := s.projects.Get(name); err != nil {
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
	if err := s.convStore.Rename(name, r.PathValue("conv"), body.Title); err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// deleteConversation removes the record and kills its tmux session. The
// agent's transcript in the shared home is left intact (ADR-011).
func (s *Server) deleteConversation(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("project")
	if _, err := s.projects.Get(name); err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	convID := r.PathValue("conv")
	if err := s.tmux.KillSession(r.Context(), name, conversation.Prefix, convID); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if err := s.convStore.Delete(name, convID); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Terminals: ephemeral plain shells, listed straight from tmux.
func (s *Server) listTerminals(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("project")
	if _, err := s.projects.Get(name); err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	terms, err := s.tmux.Terminals(r.Context(), name)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"terminals": terms})
}

func (s *Server) createTerminal(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("project")
	if _, err := s.projects.Get(name); err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	terms, err := s.tmux.Terminals(r.Context(), name)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	s.termMu.Lock()
	defer s.termMu.Unlock()
	if s.termReserved == nil {
		s.termReserved = map[string]map[string]time.Time{}
	}
	if s.termReserved[name] == nil {
		s.termReserved[name] = map[string]time.Time{}
	}
	used := map[string]bool{}
	for _, t := range terms {
		used[t.ID] = true
	}
	now := time.Now()
	for id, exp := range s.termReserved[name] {
		if now.Before(exp) {
			used[id] = true
		} else {
			delete(s.termReserved[name], id)
		}
	}
	id := ""
	for n := 1; ; n++ {
		id = strconv.Itoa(n)
		if !used[id] {
			break
		}
	}
	s.termReserved[name][id] = now.Add(time.Minute)
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (s *Server) deleteTerminal(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("project")
	if _, err := s.projects.Get(name); err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	if err := s.tmux.KillSession(r.Context(), name, conversation.TermPrefix, r.PathValue("term")); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
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
