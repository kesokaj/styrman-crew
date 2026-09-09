package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/kesokaj/styrman-crew/internal/orchestrator"
	"github.com/kesokaj/styrman-crew/internal/project"
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

// rollbackMode puts the record back to the mode still in force after an
// apply failed with the container possibly still running on the old bridge.
// The point is that every surface reading the record — the switch, the badge,
// the API — must report what is actually in effect, never what was asked
// for: a record claiming "sandboxed" over a container still on a lan- bridge
// is a fail-open nothing would ever correct, since the mismatch heal in
// EnsureRunning fires for STOPPED containers only.
func (s *Server) rollbackMode(sp *userSpace, id string, requested bool, cause error, what string) {
	s.netMu.Lock()
	rb := sp.projects.SetLanAccess(id, !requested)
	s.netMu.Unlock()
	if errors.Is(rb, project.ErrNotFound) {
		// The project was deleted while its mode was changing. There is no
		// record left to disagree with anything; the delete path removes the
		// container too. A race, not an incident — do not page anyone.
		s.logger.Info("network mode: project deleted during the change; nothing to roll back",
			"project", id, "cause", cause)
		return
	}
	if rb != nil {
		// Worse: the record now disagrees with reality and we could not
		// correct it. ERROR — this one needs a human.
		s.logger.Error("network mode: rollback failed, record disagrees with the running container",
			"project", id, "recordSays", requested, "actual", !requested, "cause", cause, "error", rb)
		return
	}
	s.logger.Warn("network mode: apply failed, record rolled back",
		"project", id, "requested", requested, "stillInForce", !requested, "why", what, "cause", cause)
}

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
	st, err := s.mode.Status(ctx, u.Username, p.ID)
	if err != nil {
		// NOT the same as "stopped". Status maps a missing container to
		// ("absent", nil); a non-nil error is a real inspect failure, and the
		// container may well be running on the old bridge. Treating that as
		// "nothing to do" was the same fail-open as a failed Stop, one line
		// up: the record would claim the new mode while the old one stayed in
		// force, with nothing to reconcile them. Same remedy.
		s.rollbackMode(sp, p.ID, on, err, "could not inspect the project")
		writeErr(w, http.StatusInternalServerError,
			fmt.Errorf("the project could not be inspected, so the mode was NOT changed: %w", err))
		return
	}
	if st.State != "running" {
		// Stopped or absent: the record is the truth and EnsureRunning's
		// mismatch check applies it on the next start. Nothing to do.
		writeJSON(w, http.StatusOK, lanView{LanAccess: on, CanEnable: u.Admin})
		return
	}
	if err := s.mode.Stop(ctx, u.Username, p.ID); err != nil {
		// FAIL CLOSED on the reported state. The container is still running on
		// the OLD bridge, so the mode did not change — but the record already
		// says it did, and nothing would ever reconcile the two: the
		// mismatch heal in EnsureRunning fires for STOPPED containers only, so
		// a container left running on the wrong bridge stays there.
		//
		// Revoking (LAN -> sandbox) is the dangerous direction: the record
		// would claim "sandboxed" while the project still had LAN access, and
		// every surface that reads the record — the switch, the badge, the
		// API — would repeat that claim indefinitely. Put the record back so
		// what we report is what is actually in force.
		s.rollbackMode(sp, p.ID, on, err, "could not stop the project")
		writeErr(w, http.StatusInternalServerError,
			fmt.Errorf("the project could not be stopped, so the mode was NOT changed: %w", err))
		return
	}
	if _, err := s.mode.EnsureRunning(ctx, u.Username, p.ID, orchestrator.RunOpts{
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
