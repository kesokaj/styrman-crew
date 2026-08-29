package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/kesokaj/styrman-crew/internal/gitrepo"
	"github.com/kesokaj/styrman-crew/internal/project"
)

// workspacePath is the host-side path of a project's working tree — since
// ADR-027 a dedicated per-project dir under StateDir/projects/<user>/<uuid>,
// bind-mounted into the microVM. The path is server-controlled end to end
// (its parent is never mounted into any guest), so the old home-containment
// guard no longer applies.
func (s *Server) workspacePath(user string, p project.Project) string {
	return s.cfg.ProjectWorkspace(user, p.ID)
}

// Automatic checkpoints (a periodic loop + snapshots on stop/pause/close) were
// removed 2026-08-29 at the operator's direction: backups now come from microVM
// volume snapshots, and the side panel is a real git client (git.go). The
// manual checkpoint list/create/restore endpoints below are retained for now.

func (s *Server) listCheckpoints(w http.ResponseWriter, r *http.Request) {
	u, _, p, err := s.getProjectFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	ws := s.workspacePath(u.Username, p)
	cps, err := gitrepo.List(r.Context(), ws)
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
	ws := s.workspacePath(u.Username, p)
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
	ws := s.workspacePath(u.Username, p)
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
