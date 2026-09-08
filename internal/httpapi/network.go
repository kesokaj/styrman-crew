package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/kesokaj/styrman-crew/internal/netpolicy"
	"github.com/kesokaj/styrman-crew/internal/project"
)

// Per-project local-network access (ADR-065).
//
// The server cannot touch the host firewall: it records INTENT
// (Project.LanAccess), projects it as a marker file the root-owned firewall
// script reconciles, nudges that script through a spool, and reports the
// state root wrote back — never the state the request would like to be
// true. Enable is admin-only and refused (503) whenever root's support is
// absent, stale, outdated or unconfigured; disable is always accepted from
// the owner, because tightening must never be refused.

// kickThrottle bounds how often a boot path (a terminal reconnect, a start)
// may cost root a reconcile for the same project.
const kickThrottle = 10 * time.Second

// lanView is GET /api/projects/{id}/network's body and PUT's reply.
type lanView struct {
	LanAccess    bool            `json:"lanAccess"`
	LanState     netpolicy.State `json:"lanState"`
	LanMessage   string          `json:"lanMessage"`
	LanChangedAt *time.Time      `json:"lanChangedAt,omitempty"`
	// Host is the operator's view of the root half — admins only, so a
	// tenant never learns the operator's LAN prefix from a project they own.
	Host *hostFirewallView `json:"host,omitempty"`
}

type hostFirewallView struct {
	Available       bool       `json:"available"`
	Reason          string     `json:"reason"`
	Version         int        `json:"version"`
	RequiredVersion int        `json:"requiredVersion"`
	AppliedAt       *time.Time `json:"appliedAt,omitempty"`
	LanDev          string     `json:"lanDev"`
	LanCidrs        []string   `json:"lanCidrs"`
	LanProjects     int        `json:"lanProjects"`
	Warnings        []string   `json:"warnings"`
}

func (s *Server) hostFirewall(st *netpolicy.Status, now time.Time) hostFirewallView {
	v := hostFirewallView{RequiredVersion: netpolicy.MinScriptVersion, LanCidrs: []string{}, Warnings: []string{}}
	v.Available, v.Reason = netpolicy.Availability(st, now)
	if st == nil {
		return v
	}
	t := st.AppliedAt
	v.Version, v.AppliedAt, v.LanDev, v.LanProjects = st.Version, &t, st.LanDev, len(st.Applied)
	v.LanCidrs = append(v.LanCidrs, st.LanCidrs...)
	v.Warnings = append(v.Warnings, st.Warnings...)
	if st.ConfError != "" {
		v.Warnings = append(v.Warnings, "conf_error: "+st.ConfError)
	}
	return v
}

// lanFor derives one project's state from the record and root's status.
// Root's texts name the operator's LAN prefix (a conf_error quotes the CIDR,
// an error line carries the destination), so a non-admin owner gets the
// state with a generic message instead (security review).
func (s *Server) lanFor(p project.Project, st *netpolicy.Status, now time.Time, admin bool) (netpolicy.State, string) {
	state, msg := netpolicy.Derive(p.LanAccess, p.LanChangedAt, p.ID, st, now)
	if admin {
		return state, msg
	}
	switch state {
	case netpolicy.StateError:
		return state, "the host firewall reported a problem with this project's local-network access — ask the operator"
	case netpolicy.StateUnavailable:
		return state, "not available on this server — ask the operator"
	case netpolicy.StatePending:
		return state, msg // "applying…", "closing…", "applies when the project next starts": nothing to leak
	}
	return state, ""
}

func (s *Server) lanView(p project.Project, admin bool, now time.Time) lanView {
	st := s.netpol.Status(now)
	state, msg := s.lanFor(p, st, now, admin)
	v := lanView{LanAccess: p.LanAccess, LanState: state, LanMessage: msg}
	if !p.LanChangedAt.IsZero() {
		t := p.LanChangedAt
		v.LanChangedAt = &t
	}
	if admin {
		h := s.hostFirewall(st, now)
		v.Host = &h
	}
	return v
}

func (s *Server) projectNetwork(w http.ResponseWriter, r *http.Request) {
	u, _, p, err := s.getProjectFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, s.lanView(p, u.Admin, time.Now()))
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
	if on && !s.requireAdmin(w, r) {
		return
	}
	now := time.Now()
	// netMu serializes the store write + marker write + kick against other
	// PUTs, and — through the marker dir's own lock — against the periodic
	// Sync, so record and marker can never diverge silently.
	s.netMu.Lock()
	defer s.netMu.Unlock()
	if on {
		if ok, reason := netpolicy.Availability(s.netpol.Status(now), now); !ok {
			writeErr(w, http.StatusServiceUnavailable, fmt.Errorf("%s", reason))
			return
		}
	}
	prev, prevAt := p.LanAccess, p.LanChangedAt
	if err := sp.projects.SetLanAccess(p.ID, on, now); err != nil {
		writeErr(w, http.StatusInternalServerError, fmt.Errorf("cannot persist setting: %w", err))
		return
	}
	if err := s.netpol.SetMarker(p.ID, on); err != nil {
		// Restore the exact prior pair — a rollback is not a change the user
		// made, so it must not restart the pending→error clock (review).
		if rerr := sp.projects.RestoreLanAccess(p.ID, prev, prevAt); rerr != nil {
			s.logger.Error("lan access rollback failed — record and marker diverge until the next sync", "project", p.ID, "error", rerr)
		}
		writeErr(w, http.StatusInternalServerError, fmt.Errorf("saved, then rolled back: the host firewall could not be signalled: %w", err))
		return
	}
	// A real change kicks root at once; a PUT that changed nothing (the UI's
	// Retry, or a tenant looping disable) is throttled so nobody can drive
	// root's reconciler into its trigger limit (security review).
	var kicked bool
	if prev == on {
		kicked = s.netpol.KickThrottled(p.ID, kickThrottle)
	} else {
		kicked = s.netpol.Kick(p.ID)
	}
	// Audit: an isolation exception changing hands is worth a durable line.
	s.logger.Info("lan access changed", "by", u.Email, "project", p.ID, "projectName", p.Name, "lanAccess", on, "kicked", kicked)
	p, err = sp.projects.Get(p.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	v := s.lanView(p, u.Admin, now)
	if !kicked && v.LanState == netpolicy.StatePending {
		v.LanMessage = "queued for the host firewall's 60 s timer (the spool is unavailable)"
	}
	writeJSON(w, http.StatusOK, v)
}

// kickLAN nudges root after a project boots, and only when its record is on
// and root has not applied it yet — the rule needs the bridge's subnet, which
// exists only once the network does. Throttled so a reconnect storm never
// becomes a reconcile storm.
func (s *Server) kickLAN(user, projectID string) {
	sp, err := s.space(user)
	if err != nil {
		return
	}
	p, err := sp.projects.Get(projectID)
	if err != nil || !p.LanAccess {
		return
	}
	now := time.Now()
	if st := s.netpol.Status(now); st != nil {
		if _, applied := st.Applied[p.ID]; applied {
			return
		}
	}
	s.netpol.KickThrottled(p.ID, kickThrottle)
}
