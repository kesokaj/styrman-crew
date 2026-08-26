// Package httpapi exposes styrman's REST + WebSocket API and serves the
// embedded UI. JSON follows the camelCase conventions from the json-api-style
// standard; errors are {"error":{"message":...}}.
package httpapi

import (
	"context"
	"embed"
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
	"time"

	"github.com/kesokaj/styrman/internal/config"
	"github.com/kesokaj/styrman/internal/conversation"
	"github.com/kesokaj/styrman/internal/orchestrator"
	"github.com/kesokaj/styrman/internal/project"
	"github.com/kesokaj/styrman/internal/terminal"
)

//go:embed devpage
var devPage embed.FS

// Version is stamped via -ldflags at build time.
var Version = "dev"

// Server wires the API handlers to their dependencies.
type Server struct {
	cfg      *config.Config
	projects *project.Store
	orch     *orchestrator.Orchestrator
	convs    *conversation.Manager
	bridge   *terminal.Bridge
	logger   *slog.Logger
}

// New creates the Server.
func New(cfg *config.Config, projects *project.Store, orch *orchestrator.Orchestrator, logger *slog.Logger) *Server {
	return &Server{
		cfg:      cfg,
		projects: projects,
		orch:     orch,
		convs:    conversation.NewManager(orch, cfg),
		bridge:   terminal.NewBridge(orch, projects, cfg, logger),
		logger:   logger,
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
	mux.HandleFunc("DELETE /api/projects/{project}/conversations/{conv}", s.deleteConversation)
	mux.Handle("GET /ws/term/{project}/{conv}", s.bridge)
	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("GET /api/version", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"version": Version})
	})

	sub, _ := fs.Sub(devPage, "devpage")
	mux.Handle("GET /dev/", http.StripPrefix("/dev/", http.FileServerFS(sub)))
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/dev/", http.StatusFound)
			return
		}
		http.NotFound(w, r)
	})
	return csrfGuard(mux)
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
	if err := s.orch.Destroy(r.Context(), name); err != nil {
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
	if err := s.orch.Stop(r.Context(), name); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"state": "stopped"})
}

func (s *Server) listConversations(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("project")
	if _, err := s.projects.Get(name); err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	convs, err := s.convs.List(r.Context(), name)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"conversations": convs})
}

// createConversation allocates the next free numeric id. The tmux session
// itself is created lazily on the first WebSocket attach.
func (s *Server) createConversation(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("project")
	if _, err := s.projects.Get(name); err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	convs, err := s.convs.List(r.Context(), name)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	used := map[string]bool{}
	for _, c := range convs {
		used[c.ID] = true
	}
	id := ""
	for n := 1; ; n++ {
		id = strconv.Itoa(n)
		if !used[id] {
			break
		}
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (s *Server) deleteConversation(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("project")
	if _, err := s.projects.Get(name); err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	if err := s.convs.Kill(r.Context(), name, r.PathValue("conv")); err != nil {
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
