package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/kesokaj/styrman-crew/internal/orchestrator"
)

// Per-project network mode (ADR-067).
//
// The mode IS the bridge interface prefix, chosen by the server at
// `docker network create`:
//
//	SANDBOX  sdb-<hash>, matched by the root firewall's `-i sdb-+` jumps in
//	         INPUT and DOCKER-USER — internet and nothing else.
//	LAN      lan-<hash>, matched by no styrman rule — a normal Docker
//	         container, reaching whatever the host reaches.
//
// There is no root-side per-project state: no marker file, no spool, no
// status file, no version handshake, and so nothing that can be pending,
// stale, or unavailable. That is why this file is a fraction of what ADR-065
// and ADR-066 needed. Cross-project isolation is unaffected either way —
// Docker's own per-bridge `! -i <br> -o <br> -j DROP` enforces it.
//
// A bridge name is immutable, so applying a change REBUILDS the project's
// network and container. Home and workspace are bind mounts and survive; the
// ADR-028 apt manifest replays installed packages.

// lanRebuildTimeout bounds the synchronous stop+rebuild. startProject gives a
// plain start 2 minutes and a rebuild is strictly more work.
const lanRebuildTimeout = 3 * time.Minute

// lanView is GET /api/projects/{id}/network's body and PUT's reply.
type lanView struct {
	LanAccess bool `json:"lanAccess"`
	// CanEnable says whether THIS caller may turn LAN mode on. It is its own
	// field rather than something the UI infers from the presence of another
	// (BUG-019: "expose a capability the server already knows as its own
	// field; don't infer it from a state value reachable through more than
	// one cause"). Disabling never needs it — tightening is never refused.
	CanEnable bool `json:"canEnable"`
	// Rebuilt says whether this request rebuilt the project. False for a
	// no-op and for a stopped project, whose mode applies at its next start.
	Rebuilt bool `json:"rebuilt"`
}

func (s *Server) projectNetwork(w http.ResponseWriter, r *http.Request) {
	u, _, p, err := s.getProjectFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, lanView{LanAccess: p.LanAccess, CanEnable: u.Admin})
}

func (s *Server) setProjectNetwork(w http.ResponseWriter, r *http.Request) {
	var body struct {
		LanAccess *bool `json:"lanAccess"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.LanAccess == nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("lanAccess (boolean) required"))
		return
	}
	// 404 before any authorization signal: another user's project must look
	// exactly like a project that does not exist.
	u, sp, p, err := s.getProjectFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	on := *body.LanAccess
	// Enabling is admin-only; the owner may always disable, because
	// tightening must never be refused.
	if on && !s.requireAdmin(w, r) {
		return
	}
	// Same resurrection guard startProject uses: never rebuild a project that
	// is being deleted.
	if inFlight, _ := s.deletingState(u.Username, p.ID); inFlight {
		writeErr(w, http.StatusConflict, errors.New("project is being deleted"))
		return
	}

	// netMu serializes the record write against deleteProject's record
	// removal. Held for the WRITE only, never across the rebuild below: it is
	// a server-wide mutex, and a three-minute hold would block every other
	// project's mode change and deletion. The rebuild is already serialized
	// per project by the orchestrator's own lock.
	s.netMu.Lock()
	if p.LanAccess == on {
		s.netMu.Unlock()
		writeJSON(w, http.StatusOK, lanView{LanAccess: on, CanEnable: u.Admin})
		return
	}
	if err := sp.projects.SetLanAccess(p.ID, on); err != nil {
		s.netMu.Unlock()
		writeErr(w, http.StatusInternalServerError, fmt.Errorf("cannot persist setting: %w", err))
		return
	}
	s.netMu.Unlock()

	// An isolation change is worth a durable line whether or not it rebuilt.
	s.logger.Info("network mode changed", "by", u.Email, "project", p.ID,
		"projectName", p.Name, "lanAccess", on)

	ctx, cancel := context.WithTimeout(context.Background(), lanRebuildTimeout)
	defer cancel()
	st, err := s.orch.Status(ctx, u.Username, p.ID)
	if err != nil || st.State != "running" {
		// Stopped or absent: the record is the truth and EnsureRunning's
		// mismatch check applies it on the next start. Nothing to do.
		writeJSON(w, http.StatusOK, lanView{LanAccess: on, CanEnable: u.Admin})
		return
	}
	if err := s.orch.Stop(ctx, u.Username, p.ID); err != nil {
		writeErr(w, http.StatusInternalServerError,
			fmt.Errorf("saved, but the project could not be stopped to apply it: %w", err))
		return
	}
	if _, err := s.orch.EnsureRunning(ctx, u.Username, p.ID, orchestrator.RunOpts{
		Name: p.Name, Dir: p.Dir, GuestUser: sp.profile.GuestUser(), Lan: on,
	}); err != nil {
		writeErr(w, http.StatusInternalServerError,
			fmt.Errorf("saved, but the project could not be restarted: %w — start it to apply the change", err))
		return
	}
	go func() { // background pre-attach, as createProject and startProject do
		time.Sleep(time.Second)
		s.orch.EnsureSelfAttached(u.Username, p.ID)
	}()
	writeJSON(w, http.StatusOK, lanView{LanAccess: on, CanEnable: u.Admin, Rebuilt: true})
}
