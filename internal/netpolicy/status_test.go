package netpolicy

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

var now = time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)

func statusText(appliedAt time.Time, extra string) string {
	return "schema=1\nversion=3\napplied_at=" + strconv.FormatInt(appliedAt.Unix(), 10) +
		"\nlan_dev=enp0\nlan_cidrs=192.0.2.0/24\n" + extra + "spool=0\nrun_ms=7\n"
}

func parse(t *testing.T, text string) *Status {
	t.Helper()
	st, err := ParseStatus(strings.NewReader(text))
	if err != nil {
		t.Fatalf("ParseStatus: %v\n%s", err, text)
	}
	return st
}

func TestParseStatus(t *testing.T) {
	st := parse(t, statusText(now, "lan="+u1+" "+u2+"\npending="+u2+":no-bridge\nerror="+u1+":192.0.2.0/24:insert failed\nerror="+u1+":1 of 2 rules present\nerror=rule 3:delete failed\nwarn=LAN_DEV enp0 is a kernel bridge\nfuture_key=whatever\nconf_error=\n"))
	if st.Schema != 1 || st.Version != 3 || !st.AppliedAt.Equal(now) || st.LanDev != "enp0" || len(st.LanCidrs) != 1 {
		t.Fatalf("header: %+v", st)
	}
	if _, ok := st.Applied[u1]; !ok {
		t.Fatal("u1 applied")
	}
	if _, ok := st.Applied[u2]; !ok {
		t.Fatal("u2 applied")
	}
	if st.Pending[u2] != "no-bridge" {
		t.Fatalf("pending: %v", st.Pending)
	}
	if st.Errors[u1] != "192.0.2.0/24:insert failed; 1 of 2 rules present" {
		t.Fatalf("errors: %v", st.Errors)
	}
	if len(st.Warnings) != 2 || st.Warnings[0] != "rule 3:delete failed" {
		t.Fatalf("warnings: %v", st.Warnings)
	}
	if st.ConfError != "" || st.RunMS != 7 {
		t.Fatalf("tail: %+v", st)
	}
	// Non-uuid tokens in lan= are ignored, never trusted.
	st = parse(t, statusText(now, "lan=bogus ../x "+u1+"\n"))
	if len(st.Applied) != 1 {
		t.Fatalf("applied: %v", st.Applied)
	}
	for _, bad := range []string{"", "schema=2\nversion=3\napplied_at=1\n", "schema=1\nversion=3\n", "schema=1\napplied_at=1\n", "schema=1\nversion=3\napplied_at=soon\n", "schema=1\nversion=3\napplied_at=1\nno equals sign\n"} {
		if _, err := ParseStatus(strings.NewReader(bad)); err == nil {
			t.Errorf("ParseStatus(%q) should fail", bad)
		}
	}
}

func TestAvailability(t *testing.T) {
	cases := []struct {
		name   string
		st     *Status
		ok     bool
		reason string
	}{
		{"absent", nil, false, "not installed"},
		{"stale", parse(t, statusText(now.Add(-4*time.Minute), "")), false, "has not run for 4m"},
		{"future-dated (clock skew)", parse(t, statusText(now.Add(10*time.Minute), "")), false, "future"},
		{"outdated", parse(t, strings.Replace(statusText(now, ""), "version=3", "version=2", 1)), false, "outdated (have 2, need 3)"},
		{"conf error", parse(t, statusText(now, "conf_error=LAN_CIDRS: 10.0.0.0/8 prefix must be /16../30\n")), false, "config error: LAN_CIDRS"},
		{"no prefix", parse(t, strings.Replace(statusText(now, ""), "lan_cidrs=192.0.2.0/24", "lan_cidrs=", 1)), false, "no local-network prefix configured"},
		{"fresh", parse(t, statusText(now, "")), true, ""},
	}
	for _, c := range cases {
		ok, reason := Availability(c.st, now)
		if ok != c.ok || !strings.Contains(reason, c.reason) {
			t.Errorf("%s: ok=%v reason=%q, want ok=%v containing %q", c.name, ok, reason, c.ok, c.reason)
		}
	}
}

func TestDerive(t *testing.T) {
	fresh := parse(t, statusText(now, "lan="+u1+"\n"))
	freshNone := parse(t, statusText(now, ""))
	stale := parse(t, statusText(now.Add(-10*time.Minute), "lan="+u1+"\n"))
	noBridge := parse(t, statusText(now, "pending="+u2+":no-bridge\n"))
	withErr := parse(t, statusText(now, "error="+u2+":192.0.2.0/24:insert failed\n"))
	confErr := parse(t, statusText(now, "conf_error=LAN_DEV enp0 does not exist\n"))
	recent := now.Add(-10 * time.Second)
	old := now.Add(-5 * time.Minute)
	cases := []struct {
		name  string
		lan   bool
		at    time.Time
		uuid  string
		st    *Status
		state State
		msg   string
	}{
		{"absent, off", false, old, u1, nil, StateOff, ""},
		{"absent, on", true, old, u1, nil, StateError, "cannot confirm"},
		{"future-dated status, on, was applied", true, old, u1, parse(t, statusText(now.Add(10*time.Minute), "lan="+u1+"\n")), StateError, "cannot confirm closed"},
		{"future-dated status, on, never applied", true, old, u2, parse(t, statusText(now.Add(10*time.Minute), "")), StateUnavailable, "future"},
		{"on and applied", true, old, u1, fresh, StateOn, ""},
		{"off and not applied", false, old, u2, fresh, StateOff, ""},
		{"stale, was applied, record on", true, old, u1, stale, StateError, "cannot confirm closed"},
		{"stale, was applied, record off", false, recent, u1, stale, StateError, "cannot confirm closed"},
		{"stale, never applied, record on", true, old, u2, stale, StateUnavailable, "has not run"},
		{"stale, never applied, record off", false, old, u2, stale, StateOff, ""},
		{"per-project error wins", true, old, u2, withErr, StateError, "insert failed"},
		{"no bridge yet", true, old, u2, noBridge, StatePending, "next starts"},
		{"on, fresh mismatch", true, recent, u2, freshNone, StatePending, "applying"},
		{"on, old mismatch", true, old, u2, freshNone, StateError, "has not applied this change"},
		{"off, fresh mismatch", false, recent, u1, fresh, StatePending, "closing"},
		{"off, old mismatch", false, old, u1, fresh, StateError, "has not applied this change"},
		{"on, conf error", true, recent, u2, confErr, StateUnavailable, "config error"},
	}
	for _, c := range cases {
		state, msg := Derive(c.lan, c.at, c.uuid, c.st, now)
		if state != c.state || !strings.Contains(msg, c.msg) {
			t.Errorf("%s: got %s %q, want %s containing %q", c.name, state, msg, c.state, c.msg)
		}
	}
}
