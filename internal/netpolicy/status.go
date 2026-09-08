package netpolicy

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// Status is root's report of what the firewall script applied on its last
// run — the key=value file described in the plan; unknown keys are ignored.
type Status struct {
	Schema    int
	Version   int
	AppliedAt time.Time
	LanDev    string
	LanCidrs  []string
	// Applied holds every project uuid whose (uuid, cidr) rules are ALL
	// present after the reconcile.
	Applied map[string]struct{}
	// Pending: uuid → reason ("no-bridge", "no-subnet").
	Pending map[string]string
	// Errors: uuid → text for per-project failures; non-project errors and
	// warn= lines land in Warnings.
	Errors    map[string]string
	ConfError string
	Warnings  []string
	Spool     int
	RunMS     int
	// ADR-066. What the host conf opted into, as ROOT read it — the server
	// reports these rather than assuming, because a script converged to
	// version 4 grants nothing until a human edits the conf.
	AllowHost bool
	// AllowPublishedPorts is only ever true when root also had a usable
	// IngressBridge; root skips the family otherwise.
	AllowPublishedPorts bool
	// IngressBridge is the bridge root EXCLUDED from the published-port
	// allowlist. Root cannot tell whether it is the right one without the
	// docker daemon, so it reports what it used and the server compares.
	IngressBridge string
	HostAddrs     []string
	PubSubnets    []string
}

// State is what the UI renders for one project.
type State string

const (
	StateOff         State = "off"
	StatePending     State = "pending"
	StateOn          State = "on"
	StateError       State = "error"
	StateUnavailable State = "unavailable"
)

// ParseStatus reads the status grammar. It requires schema=1 and the
// version/applied_at keys; everything else is optional.
func ParseStatus(r io.Reader) (*Status, error) {
	st := &Status{Applied: map[string]struct{}{}, Pending: map[string]string{}, Errors: map[string]string{}}
	sc := bufio.NewScanner(io.LimitReader(r, statusMaxBytes+1))
	seen := map[string]bool{}
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("line without '=': %q", clip(line))
		}
		seen[key] = true
		switch key {
		case "schema":
			st.Schema, _ = strconv.Atoi(val)
		case "version":
			st.Version, _ = strconv.Atoi(val)
		case "applied_at":
			secs, err := strconv.ParseInt(val, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("applied_at: %q", clip(val))
			}
			st.AppliedAt = time.Unix(secs, 0).UTC()
		case "lan_dev":
			st.LanDev = val
		case "lan_cidrs":
			st.LanCidrs = strings.Fields(val)
		case "lan_allow_host":
			st.AllowHost = val == "1"
		case "lan_allow_published_ports":
			st.AllowPublishedPorts = val == "1"
		case "ingress_bridge":
			st.IngressBridge = val
		case "lan_host_addrs":
			st.HostAddrs = strings.Fields(val)
		case "lan_pub_subnets":
			st.PubSubnets = strings.Fields(val)
		case "lan":
			for _, id := range strings.Fields(val) {
				if uuidRe.MatchString(id) {
					st.Applied[id] = struct{}{}
				}
			}
		case "pending":
			id, reason, _ := strings.Cut(val, ":")
			if uuidRe.MatchString(id) {
				st.Pending[id] = reason
			}
		case "error":
			id, text, _ := strings.Cut(val, ":")
			if uuidRe.MatchString(id) {
				if prev, ok := st.Errors[id]; ok {
					text = prev + "; " + text
				}
				st.Errors[id] = text
			} else {
				st.Warnings = append(st.Warnings, val)
			}
		case "conf_error":
			st.ConfError = val
		case "warn":
			st.Warnings = append(st.Warnings, val)
		case "spool":
			st.Spool, _ = strconv.Atoi(val)
		case "run_ms":
			st.RunMS, _ = strconv.Atoi(val)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if st.Schema != 1 {
		return nil, fmt.Errorf("unsupported schema %d", st.Schema)
	}
	if !seen["version"] || !seen["applied_at"] {
		return nil, fmt.Errorf("missing version or applied_at")
	}
	if st.LanCidrs == nil {
		st.LanCidrs = []string{}
	}
	if st.Warnings == nil {
		st.Warnings = []string{}
	}
	if st.HostAddrs == nil {
		st.HostAddrs = []string{}
	}
	if st.PubSubnets == nil {
		st.PubSubnets = []string{}
	}
	return st, nil
}

func clip(s string) string {
	if len(s) > 80 {
		return s[:80] + "…"
	}
	return s
}

// stale reports whether root's word is too old to act on — or from the
// future, which a clock jump can produce and which would otherwise never
// age out (security review).
func stale(st *Status, now time.Time) bool {
	age := now.Sub(st.AppliedAt)
	return age > StaleAfter || age < -StaleAfter
}

// Availability says whether an ENABLE may be accepted right now, and if not
// why — always root's word, never the request's.
func Availability(st *Status, now time.Time) (ok bool, reason string) {
	switch {
	case st == nil:
		return false, "host firewall support is not installed — run deploy/firewall/install.sh on the server"
	case stale(st, now):
		if st.AppliedAt.After(now) {
			return false, "host firewall status is dated in the future (clock skew?)"
		}
		return false, fmt.Sprintf("host firewall timer has not run for %dm", int(now.Sub(st.AppliedAt).Minutes()))
	case st.Version < MinScriptVersion:
		return false, fmt.Sprintf("host firewall script is outdated (have %d, need %d) — run deploy/firewall/install.sh on the server", st.Version, MinScriptVersion)
	case st.ConfError != "":
		return false, "host firewall config error: " + st.ConfError
	case len(st.LanCidrs) == 0:
		return false, "no local-network prefix configured (/etc/styrman/firewall.conf on the server)"
	}
	return true, ""
}

// Derive turns (record, root's status) into the state the UI renders.
// Rules, in order: no readable status at all with a record ON is an ERROR
// ("cannot confirm"), never "unavailable" — the rule may be live and root
// unreachable (bind gone, dir cleared), and reporting it as absent would be
// fail-open (security review); a stale status can only report what root
// last saw (a project it listed as applied is reported as an error, never as
// off); a per-project error wins; record and kernel agreeing is on/off; a
// bridge-less project is pending until it boots; a fresh mismatch is
// pending, an old one is an error.
func Derive(lan bool, changedAt time.Time, uuid string, st *Status, now time.Time) (State, string) {
	if st == nil {
		if lan {
			return StateError, "cannot confirm the host firewall's state — no readable status file, and the rule may still be present; check /run/styrman-firewall/status and journalctl -u styrman-firewall on the server"
		}
		return StateOff, ""
	}
	_, applied := st.Applied[uuid]
	if stale(st, now) {
		if applied {
			return StateError, fmt.Sprintf("cannot confirm closed — the host firewall has not run for %dm and the rule may still be present; journalctl -u styrman-firewall on the server", int(now.Sub(st.AppliedAt).Minutes()))
		}
		if lan {
			_, reason := Availability(st, now)
			return StateUnavailable, reason
		}
		return StateOff, ""
	}
	if text, ok := st.Errors[uuid]; ok {
		return StateError, "host firewall reported: " + text
	}
	if lan && applied {
		return StateOn, ""
	}
	if !lan && !applied {
		return StateOff, ""
	}
	if lan {
		if p, ok := st.Pending[uuid]; ok && (p == "no-bridge" || p == "no-subnet") {
			return StatePending, "applies when the project next starts"
		}
		if ok, reason := Availability(st, now); !ok {
			return StateUnavailable, reason
		}
	}
	if now.Sub(changedAt) < PendingGrace {
		if lan {
			return StatePending, "applying on the host firewall"
		}
		return StatePending, "closing on the host firewall"
	}
	return StateError, fmt.Sprintf("the host firewall has not applied this change for %ds — journalctl -u styrman-firewall on the server", int(now.Sub(changedAt).Seconds()))
}
