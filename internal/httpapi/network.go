package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/kesokaj/styrman-crew/internal/credstore"
	"github.com/kesokaj/styrman-crew/internal/files"
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
	// CredentialImpact names what LAN access costs this project, so the UI
	// need not encode the policy. Empty when nothing is withheld.
	CredentialImpact string `json:"credentialImpact,omitempty"`
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
	// ADR-066: what the host CONF opted into, as root read it. A script
	// converged to version 4 grants nothing until a human edits the conf, so
	// the UI must show "on, but the host does not include X" rather than a
	// switch that silently means less than it says.
	AllowHost           bool     `json:"allowHost"`
	AllowPublishedPorts bool     `json:"allowPublishedPorts"`
	HostAddrs           []string `json:"hostAddrs"`
	PubSubnets          []string `json:"pubSubnets"`
	IngressBridge       string   `json:"ingressBridge,omitempty"`
}

func (s *Server) hostFirewall(st *netpolicy.Status, now time.Time) hostFirewallView {
	v := hostFirewallView{RequiredVersion: netpolicy.MinScriptVersion, LanCidrs: []string{},
		Warnings: []string{}, HostAddrs: []string{}, PubSubnets: []string{}}
	v.Available, v.Reason = netpolicy.Availability(st, now)
	if st == nil {
		return v
	}
	v.AllowHost, v.AllowPublishedPorts = st.AllowHost, st.AllowPublishedPorts
	v.IngressBridge = st.IngressBridge
	v.HostAddrs = append(v.HostAddrs, st.HostAddrs...)
	v.PubSubnets = append(v.PubSubnets, st.PubSubnets...)
	if w := s.ingressBridgeMismatch(st); w != "" {
		v.Warnings = append(v.Warnings, w)
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

// ingressBridgeMismatch cross-checks the bridge root excluded from the
// published-port allowlist against the one this server's own ingress network
// actually uses. Root cannot make this check — a root firewall script must
// not depend on the Docker daemon — so it reports what it used and this is
// the other half.
//
// A wrong-but-valid value is the dangerous case: it passes root's shape and
// existence checks while leaving the REAL ingress inside the reachable set,
// the exact opposite of the key's purpose. An empty answer from Docker is "no
// opinion" and never a mismatch.
//
// Cached: this costs a NetworkInspect, and lanView is rendered per project in
// a listing.
func (s *Server) ingressBridgeMismatch(st *netpolicy.Status) string {
	if st == nil || !st.AllowPublishedPorts || st.IngressBridge == "" {
		return ""
	}
	s.ingressBrMu.Lock()
	defer s.ingressBrMu.Unlock()
	if time.Since(s.ingressBrAt) > time.Minute {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		s.ingressBr = s.orch.BridgeNameForNetwork(ctx, s.cfg.IngressNetwork)
		cancel()
		s.ingressBrAt = time.Now()
	}
	if s.ingressBr == "" || s.ingressBr == st.IngressBridge {
		return ""
	}
	return fmt.Sprintf("INGRESS_BRIDGE in the host conf is %s but the %s network uses %s — "+
		"published-port access may expose the ingress; fix /etc/styrman/firewall.conf on the server",
		st.IngressBridge, s.cfg.IngressNetwork, s.ingressBr)
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
	// The same latch the broker gates on, so the UI cannot disagree with what
	// is actually served. Not operator-sensitive — it describes the owner's
	// own project — so unlike Host it is not admin-gated.
	if p.LanEverEnabled || p.LanAccess {
		v.CredentialImpact = "ssh-key-withheld"
	}
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
		st := s.netpol.Status(now)
		if ok, reason := netpolicy.Availability(st, now); !ok {
			writeErr(w, http.StatusServiceUnavailable, fmt.Errorf("%s", reason))
			return
		}
		// A wrong-but-VALID INGRESS_BRIDGE passes root's shape and existence
		// checks while leaving the real ingress inside the published-port
		// allowlist — the opposite of that key's purpose. Root cannot detect
		// it without the Docker daemon; the server can, and it refuses rather
		// than merely mentioning it in an admin panel. An advisory nobody has
		// to read is not a control (security review, 2026-09-08).
		if reason := s.ingressBridgeMismatch(st); reason != "" {
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
	// Enabling means the broker stops serving this project host-reaching
	// credentials — but a copy may already sit in its home from an earlier
	// login shell, so remove that too. Best-effort and forward-looking: a
	// guest that copied the key elsewhere still has it, which is why ADR-066
	// says enabling LAN on an existing project means the key WAS exposed to
	// it. Never a reason to fail the request.
	if on {
		s.pruneHostReachingCreds(u.Username, p.ID)
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

// pruneHostReachingCreds deletes from a project's home the credential files
// the broker will no longer serve it. The paths come from the credential
// store, so this prunes exactly what the broker withholds rather than a
// hardcoded guess that could drift from it.
//
// Deletion goes through internal/files, which confines the path to the
// tenant's registered base (BUG-002/003): the project home sits under a tree
// the guest can write, so a component of ~/.ssh could be a symlink planted to
// aim this delete somewhere else.
func (s *Server) pruneHostReachingCreds(user, project string) {
	if s.creds == nil {
		return
	}
	fcs, err := s.creds.AllFiles(user)
	if err != nil {
		s.logger.Warn("lan access: cannot list credentials to prune", "user", user, "project", project, "error", err)
		return
	}
	home := s.cfg.ProjectHome(user, project)
	for _, c := range fcs {
		if !credstore.IsHostReaching(c.Path) {
			continue
		}
		rel := strings.TrimPrefix(strings.TrimPrefix(c.Path, "~/"), "$HOME/")
		if !files.Exists(home, rel) {
			continue
		}
		if err := files.Remove(home, rel); err != nil {
			s.logger.Warn("lan access: could not prune a materialized credential",
				"project", project, "path", rel, "error", err)
			continue
		}
		s.logger.Info("lan access: pruned a materialized host-reaching credential", "project", project, "path", rel)
	}
}

// LanExposed implements credbroker.LanChecker (ADR-066 part 4, as amended):
// has this project EVER had local-network access?
//
// It reads one boolean off the record and consults root's firewall status not
// at all. The earlier version asked "could this project reach the LAN right
// now?" — a union of the record, root's applied set, and cannot-confirm —
// because the key was meant to come back when LAN access was turned off.
// That question has no reliable answer: root's rule can outlive the record by
// up to a minute, and a failed revocation drops a project out of the applied
// set while its rule is still live, which handed the key to a project that
// still had LAN reach (release review round 3).
//
// Latching removes the question rather than answering it. Re-serving the key
// later would not un-expose it — the project already had it — so the live
// state was buying nothing and costing correctness.
func (s *Server) LanExposed(user, projectID string) (bool, error) {
	sp, err := s.space(user)
	if err != nil {
		return false, err
	}
	p, err := sp.projects.Get(projectID)
	if err != nil {
		return false, err
	}
	return p.LanEverEnabled || p.LanAccess, nil
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
