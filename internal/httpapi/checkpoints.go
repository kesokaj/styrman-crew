package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/kesokaj/styrman/internal/files"
	"github.com/kesokaj/styrman/internal/gitrepo"
	"github.com/kesokaj/styrman/internal/project"
)

// workspacePath is the host-side path of a project's working tree, resolved
// through the symlink-containment guard: the shared home is mounted into the
// server, and every guest runs as the same uid, so a project could symlink
// its workspace at another user's home. files.ContainedPath rejects any path
// whose real target escapes this user's home (git needs a real path, so this
// uses the path-containment guard rather than an os.Root handle).
func (s *Server) workspacePath(user string, p project.Project) (string, error) {
	return files.ContainedPath(s.cfg.UserHome(user), "projects/"+p.Dir)
}

// snapshot takes a best-effort automatic checkpoint (R13); failures are
// logged, never fatal — a snapshot must not block the action that triggered
// it. Runs on its own bounded context so a slow tree can't hang a caller.
func (s *Server) snapshot(user string, p project.Project, label string) {
	ws, err := s.workspacePath(user, p)
	if err != nil {
		s.logger.Warn("checkpoint: workspace path rejected", "project", p.ID, "error", err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if _, _, err := gitrepo.Snapshot(ctx, ws, label); err != nil {
		s.logger.Warn("checkpoint failed", "project", p.ID, "label", label, "error", err)
	}
}

func (s *Server) listCheckpoints(w http.ResponseWriter, r *http.Request) {
	u, _, p, err := s.getProjectFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	ws, err := s.workspacePath(u.Username, p)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, errors.New("workspace unavailable"))
		return
	}
	cps, err := gitrepo.List(ws)
	if err != nil {
		s.logger.Warn("list checkpoints failed", "project", p.ID, "error", err)
		writeErr(w, http.StatusInternalServerError, errors.New("could not list checkpoints"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"checkpoints": cps})
}

func (s *Server) createCheckpoint(w http.ResponseWriter, r *http.Request) {
	u, _, p, err := s.getProjectFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	var body struct {
		Label string `json:"label"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body) // empty or malformed body = default label
	if body.Label == "" {
		body.Label = "manual"
	}
	ws, err := s.workspacePath(u.Username, p)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, errors.New("workspace unavailable"))
		return
	}
	id, created, err := gitrepo.Snapshot(r.Context(), ws, body.Label)
	if err != nil {
		s.logger.Warn("create checkpoint failed", "project", p.ID, "error", err)
		writeErr(w, http.StatusInternalServerError, errors.New("could not create checkpoint"))
		return
	}
	status := http.StatusCreated
	if !created { // tree unchanged since the newest checkpoint
		status = http.StatusOK
	}
	writeJSON(w, status, map[string]any{"id": id, "created": created})
}

// restoreCheckpoint rewrites the working tree to the checkpoint's exact
// state. The current state is snapshotted first (safetyId in the response),
// so a restore can always be undone by restoring that.
func (s *Server) restoreCheckpoint(w http.ResponseWriter, r *http.Request) {
	u, _, p, err := s.getProjectFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	ws, err := s.workspacePath(u.Username, p)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, errors.New("workspace unavailable"))
		return
	}
	id := r.PathValue("id")
	safetyID, err := gitrepo.Restore(r.Context(), ws, id)
	switch {
	case errors.Is(err, gitrepo.ErrBadID):
		writeErr(w, http.StatusBadRequest, errors.New("invalid checkpoint id"))
		return
	case errors.Is(err, gitrepo.ErrNotFound):
		writeErr(w, http.StatusNotFound, errors.New("checkpoint not found"))
		return
	case err != nil:
		s.logger.Warn("restore checkpoint failed", "project", p.ID, "id", id, "error", err)
		writeErr(w, http.StatusInternalServerError, errors.New("could not restore checkpoint"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"restored": id, "safetyId": safetyID})
}
