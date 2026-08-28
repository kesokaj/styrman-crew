package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"time"

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

// snapshot takes a best-effort automatic checkpoint (R13); failures are
// logged, never fatal — a snapshot must not block the action that triggered
// it. Runs on its own bounded context so a slow tree can't hang a caller.
func (s *Server) snapshot(user string, p project.Project, label string) {
	ws := s.workspacePath(user, p)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if _, _, err := gitrepo.Snapshot(ctx, ws, label); err != nil {
		s.logger.Warn("checkpoint failed", "project", p.ID, "label", label, "error", err)
	}
}

// RunCheckpointLoop snapshots every RUNNING project on a fixed cadence
// (operator Q6, 2026-08-28): tree-dedup in gitrepo makes an unchanged tree a
// cheap no-op, and the periodic snapshot also closes the idle-reaper gap
// (the reaper stops projects without checkpointing — with this loop the
// worst-case loss window is one interval). Manual and event-driven
// checkpoints are unchanged. Blocks until ctx is done.
func (s *Server) RunCheckpointLoop(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	// Last activity timestamp already snapshotted, per project — an idle
	// tree is skipped entirely (git add -A re-hashes the whole tree, so
	// even the dedup no-op isn't free at the 10+-VM scale target).
	snapped := map[string]time.Time{}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			statuses, err := s.orch.Adopt(ctx)
			if err != nil {
				s.logger.Warn("checkpoint loop: list failed", "error", err)
				continue
			}
			for k, st := range statuses {
				if st.State != "running" || ctx.Err() != nil {
					continue
				}
				// A container with a stale/unknown user label must not make
				// the server fabricate a home — only touch users that
				// already have state on disk (space() creates otherwise).
				if _, err := os.Stat(filepath.Join(s.cfg.UserStateDir(st.User), "projects.json")); err != nil {
					continue
				}
				act := s.orch.LastActivity(st.User, st.Project)
				if act.IsZero() || !act.After(snapped[k]) {
					continue // no terminal activity since the last snapshot
				}
				sp, err := s.space(st.User)
				if err != nil {
					continue
				}
				p, err := sp.projects.Get(st.Project)
				if err != nil {
					continue // adopted container without a record
				}
				s.snapshot(st.User, p, "periodic")
				snapped[k] = act
			}
		}
	}
}

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
