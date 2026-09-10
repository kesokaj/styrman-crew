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

// setProjectNetwork applies a mode in ONE order for both directions, under
// the project's lock for the whole decision:
//
//	inspect → if the container is RUNNING on the wrong bridge: stop,
//	persist, start → else: persist.
//
// The record is written at the one moment nothing is in force — after the
// old bridge is gone and before the new one exists — so no single failure
// can leave it claiming LESS access than the container has. A failure before
// the write changes nothing and says so; a failure after it leaves the
// container down with the record ahead of it, which the next start applies.
// An earlier version wrote the record first and rolled it back on failure;
// three fail-opens landed on that rollback in one day (BUG-028/029/031), and
// each one was a branch where the record said "sandbox" over a container
// still on lan-. There is no rollback now because there is nothing to roll
// back.
//
// Whether anything needs doing is decided against the LIVE bridge name, not
// the record: a record that has drifted from the bridge is repaired by
// re-issuing the PUT (security review 2026-09-09, round 3 — the old
// record-based short-circuit answered 200 and touched nothing).
//
// The lock is held from the inspect to the reply, and every start reads the
// record under the same lock (RunOpts.Lan), so a start racing this change
// either completes first — and is then seen running on the wrong bridge and
// rebuilt — or waits, and then reads the record this change wrote. With the
// steps taken as separate calls, a start that read the old record before the
// change and ran between two of them brought the container up on the old
// bridge (security review 2026-09-10, round 2).
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
	ctx, cancel := context.WithTimeout(context.Background(), lanRebuildTimeout)
	defer cancel()
	// persist is the record write. netMu serializes it against
	// deleteProject's record removal; it is always the INNER lock (the
	// project lock is held here, and deleteProject takes netMu only after
	// its Destroy has returned), so the two cannot deadlock.
	persist := func() error {
		s.netMu.Lock()
		defer s.netMu.Unlock()
		return sp.projects.SetLanAccess(p.ID, on)
	}
	// A project deleted between the re-read below and the write is a 404,
	// not a persist failure with a message about a project that is gone.
	gone := func(err error) bool {
		if !errors.Is(err, project.ErrNotFound) {
			return false
		}
		writeErr(w, http.StatusNotFound, err)
		return true
	}
	s.mode.WithProject(u.Username, p.ID, func(pr orchestrator.Project) {
		// The record as of NOW, under the lock — not the copy getProjectFor
		// took before it. Two PUTs racing on a stopped project would
		// otherwise both see the same pre-change record, and the second
		// would call itself a no-op over a record the first had just changed
		// (security review 2026-09-10, round 3).
		rec, err := sp.projects.Get(p.ID)
		if err != nil {
			writeErr(w, http.StatusNotFound, err)
			return
		}
		st, err := pr.Status(ctx)
		if err != nil {
			// NOT the same as "stopped". Status maps a missing container to
			// ("absent", nil); a non-nil error is a real inspect failure, and
			// the container may well be running on the old bridge (BUG-029).
			// Nothing has been written, so nothing needs undoing.
			writeErr(w, http.StatusInternalServerError,
				fmt.Errorf("the project could not be inspected, so the mode was NOT changed: %w", err))
			return
		}
		// wrongBridge: a RUNNING container whose bridge disagrees with the
		// wanted mode — the only state in which anything is in force that
		// must not be. Stopped or absent, nothing is in force and the next
		// start applies the record, whatever the bridge says.
		wrongBridge := false
		if st.State == "running" {
			wrongBridge, err = pr.ModeMismatch(ctx, on)
			if err != nil {
				// Same shape as the inspect failure above, one layer down
				// (BUG-031): "I cannot tell" must never read as "nothing to do".
				writeErr(w, http.StatusInternalServerError,
					fmt.Errorf("the project network could not be inspected, so the mode was NOT changed: %w", err))
				return
			}
		}
		if rec.LanAccess == on && !wrongBridge {
			writeJSON(w, http.StatusOK, lanView{LanAccess: on, CanEnable: u.Admin})
			return
		}
		if !wrongBridge {
			if err := persist(); err != nil {
				if gone(err) {
					return
				}
				// The bridge already matched the request and only the record
				// was behind it. Nothing changed hands here — the record
				// under-claimed (or over-claimed) before this request and
				// still does — but say so, because the state does not repair
				// itself: the re-PUT is the fix and it is the thing that just
				// failed. Stop, then Start, applies the record as it stands.
				s.logger.Warn("network mode: the bridge already matches the request but the record could not be saved; it keeps reporting the previous mode",
					"project", p.ID, "requested", on, "recordSays", rec.LanAccess, "error", err)
				writeErr(w, http.StatusInternalServerError,
					fmt.Errorf("the project is already on the requested bridge but the setting could not be saved, so it keeps reporting the previous mode: %w", err))
				return
			}
			s.logger.Info("network mode changed", "by", u.Email, "project", p.ID,
				"projectName", p.Name, "lanAccess", on, "rebuilt", false)
			writeJSON(w, http.StatusOK, lanView{LanAccess: on, CanEnable: u.Admin})
			return
		}
		if err := pr.Stop(ctx); err != nil {
			// The container is still running on the OLD bridge and the
			// record still says so. True on both counts: nothing changed.
			writeErr(w, http.StatusInternalServerError,
				fmt.Errorf("the project could not be stopped, so the mode was NOT changed: %w", err))
			return
		}
		if err := persist(); err != nil {
			if gone(err) {
				return
			}
			// The container is DOWN and the record still names the old mode:
			// it over-claims against a container that has nothing, which is
			// the safe direction, and the next start applies the record as
			// it stands. Say what state the project is in.
			s.logger.Warn("network mode: project stopped but the setting could not be saved; it stays in its previous mode",
				"project", p.ID, "requested", on, "error", err)
			writeErr(w, http.StatusInternalServerError,
				fmt.Errorf("the project was stopped but the setting could not be saved, so the mode was NOT changed — start it to continue in the previous mode: %w", err))
			return
		}
		if _, err := pr.EnsureRunning(ctx, orchestrator.RunOpts{
			Name: rec.Name, Dir: rec.Dir, GuestUser: sp.profile.GuestUser(), Lan: lanOf(sp, p.ID),
		}); err != nil {
			// Container down, record ahead of it: the next start applies it.
			writeErr(w, http.StatusInternalServerError,
				fmt.Errorf("saved, but the project could not be restarted: %w — start it to apply the change", err))
			return
		}
		// An isolation change is worth a durable line whether or not it rebuilt.
		s.logger.Info("network mode changed", "by", u.Email, "project", p.ID,
			"projectName", p.Name, "lanAccess", on, "rebuilt", true)
		go func() { // background pre-attach, as createProject and startProject do
			time.Sleep(time.Second)
			s.orch.EnsureSelfAttached(u.Username, p.ID)
		}()
		writeJSON(w, http.StatusOK, lanView{LanAccess: on, CanEnable: u.Admin, Rebuilt: true})
	})
}
