package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kesokaj/styrman-crew/internal/config"
	"github.com/kesokaj/styrman-crew/internal/identity"
	"github.com/kesokaj/styrman-crew/internal/orchestrator"
	"github.com/kesokaj/styrman-crew/internal/project"
	"github.com/kesokaj/styrman-crew/internal/userstore"
)

// netHarness builds a header-mode server (so admin vs non-admin is real)
// with a scratch firewall run dir and one project owned by the admin.
type netHarness struct {
	h       http.Handler
	s       *Server
	state   string
	fw      string
	admin   string
	member  string
	pid     string
	markers string
}

func newNetHarness(t *testing.T) *netHarness {
	t.Helper()
	dir := t.TempDir()
	fw := filepath.Join(dir, "fw")
	if err := os.MkdirAll(filepath.Join(fw, "spool"), 0o700); err != nil {
		t.Fatal(err)
	}
	admin, member := "admin@gmail.com", "member@gmail.com"
	cfg := &config.Config{
		StateDir: dir, HomesDir: filepath.Join(dir, "homes"), Username: "tester",
		AuthMode: "header", IngressToken: "test-ingress-token", AllowedEmails: []string{admin},
		FirewallRunDir: fw,
	}
	logger := slog.New(slog.DiscardHandler)
	users, err := userstore.New(filepath.Join(dir, "users.json"), cfg.AllowedEmails)
	if err != nil {
		t.Fatal(err)
	}
	if err := users.Add(member, admin); err != nil {
		t.Fatal(err)
	}
	orch, err := orchestrator.New(cfg, logger)
	if err != nil {
		t.Fatal(err)
	}
	s := New(cfg, users, orch, logger)
	h := s.Routes()
	sp, err := s.space(identity.DeriveUsername(admin))
	if err != nil {
		t.Fatal(err)
	}
	p, err := sp.projects.Add(project.Project{Name: "lanproj", CreatedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	return &netHarness{h: h, s: s, state: dir, fw: fw, admin: admin, member: member, pid: p.ID,
		markers: filepath.Join(dir, "netpolicy", "lan")}
}

func (n *netHarness) writeStatus(t *testing.T, appliedAt time.Time, extra string) {
	t.Helper()
	text := "schema=1\nversion=3\napplied_at=" + strconv.FormatInt(appliedAt.Unix(), 10) +
		"\nlan_dev=enp0\nlan_cidrs=192.0.2.0/24\n" + extra + "spool=0\nrun_ms=5\n"
	if err := os.WriteFile(filepath.Join(n.fw, "status"), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	// Defeat the 2 s status cache between steps.
	n.s.netpol.Status(time.Now().Add(time.Hour))
}

func (n *netHarness) marker() bool {
	_, err := os.Lstat(filepath.Join(n.markers, n.pid))
	return err == nil
}

func (n *netHarness) spoolEntries(t *testing.T) int {
	t.Helper()
	des, err := os.ReadDir(filepath.Join(n.fw, "spool"))
	if err != nil {
		t.Fatal(err)
	}
	return len(des)
}

func decodeLan(t *testing.T, body string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		t.Fatalf("bad JSON %q: %v", body, err)
	}
	return m
}

func TestNetworkValidationAndIsolation(t *testing.T) {
	n := newNetHarness(t)
	path := "/api/projects/" + n.pid + "/network"
	for _, body := range []string{"", "{}", `{"lanAccess":"yes"}`, `{"lanAccess":null}`, "not json"} {
		if w := doAs(t, n.h, n.admin, http.MethodPut, path, body); w.Code != http.StatusBadRequest {
			t.Errorf("PUT %q: got %d, want 400", body, w.Code)
		}
	}
	unknown := "/api/projects/00000000-0000-4000-8000-000000000000/network"
	if w := doAs(t, n.h, n.admin, http.MethodGet, unknown, ""); w.Code != http.StatusNotFound {
		t.Errorf("GET unknown: %d", w.Code)
	}
	if w := doAs(t, n.h, n.admin, http.MethodPut, unknown, `{"lanAccess":true}`); w.Code != http.StatusNotFound {
		t.Errorf("PUT unknown: %d", w.Code)
	}
	// Another authorized user must see 404 on both verbs and leave no marker —
	// the 404 comes before any authorization signal.
	if w := doAs(t, n.h, n.member, http.MethodGet, path, ""); w.Code != http.StatusNotFound {
		t.Errorf("member GET: %d, want 404", w.Code)
	}
	if w := doAs(t, n.h, n.member, http.MethodPut, path, `{"lanAccess":true}`); w.Code != http.StatusNotFound {
		t.Errorf("member PUT: %d, want 404", w.Code)
	}
	if n.marker() {
		t.Fatal("cross-user PUT created a marker")
	}
}

func TestNetworkEnableGates(t *testing.T) {
	n := newNetHarness(t)
	path := "/api/projects/" + n.pid + "/network"

	// No status file: unavailable, enable refused, nothing written.
	w := doAs(t, n.h, n.admin, http.MethodGet, path, "")
	if w.Code != http.StatusOK || decodeLan(t, w.Body.String())["lanState"] != "off" {
		t.Fatalf("GET before enable: %d %s", w.Code, w.Body.String())
	}
	w = doAs(t, n.h, n.admin, http.MethodPut, path, `{"lanAccess":true}`)
	if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "not installed") {
		t.Fatalf("enable without host support: %d %s", w.Code, w.Body.String())
	}
	if n.marker() || n.spoolEntries(t) != 0 {
		t.Fatal("refused enable must write nothing")
	}
	// Disable is always accepted, even without host support.
	if w = doAs(t, n.h, n.admin, http.MethodPut, path, `{"lanAccess":false}`); w.Code != http.StatusOK {
		t.Fatalf("disable: %d %s", w.Code, w.Body.String())
	}

	// Outdated script: refused with the version pair.
	n.writeStatus(t, time.Now(), "")
	text, _ := os.ReadFile(filepath.Join(n.fw, "status"))
	os.WriteFile(filepath.Join(n.fw, "status"), []byte(strings.Replace(string(text), "version=3", "version=2", 1)), 0o644)
	n.s.netpol.Status(time.Now().Add(2 * time.Hour))
	w = doAs(t, n.h, n.admin, http.MethodPut, path, `{"lanAccess":true}`)
	if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "outdated (have 2, need 3)") {
		t.Fatalf("enable with an outdated script: %d %s", w.Code, w.Body.String())
	}

	// Fresh status: a non-admin owner cannot enable; the admin can.
	n.writeStatus(t, time.Now(), "")
	n.s.netpol.Status(time.Now().Add(3 * time.Hour))
	if w = doAs(t, n.h, n.member, http.MethodPut, path, `{"lanAccess":true}`); w.Code != http.StatusNotFound {
		t.Fatalf("member (not the owner) enable: %d", w.Code) // still 404: not their project
	}
	spoolBefore := n.spoolEntries(t) // the accepted disable above kicked once already
	w = doAs(t, n.h, n.admin, http.MethodPut, path, `{"lanAccess":true}`)
	if w.Code != http.StatusOK {
		t.Fatalf("enable: %d %s", w.Code, w.Body.String())
	}
	m := decodeLan(t, w.Body.String())
	if m["lanAccess"] != true || m["lanState"] != "pending" {
		t.Fatalf("enable reply must be pending, never on: %v", m)
	}
	if _, ok := m["host"]; !ok {
		t.Fatal("admin reply carries the host view")
	}
	if !n.marker() || n.spoolEntries(t) != spoolBefore+1 {
		t.Fatalf("enable must create the marker and exactly one spool entry (marker=%v spool=%d before=%d)", n.marker(), n.spoolEntries(t), spoolBefore)
	}
	// The list carries the state through the existing poll.
	w = doAs(t, n.h, n.admin, http.MethodGet, "/api/projects", "")
	if !strings.Contains(w.Body.String(), `"lanState":"pending"`) {
		t.Fatalf("list lacks lanState: %s", w.Body.String())
	}

	// Root reports it applied → on.
	n.writeStatus(t, time.Now(), "lan="+n.pid+"\n")
	n.s.netpol.Status(time.Now().Add(4 * time.Hour))
	w = doAs(t, n.h, n.admin, http.MethodGet, path, "")
	if decodeLan(t, w.Body.String())["lanState"] != "on" {
		t.Fatalf("applied → on: %s", w.Body.String())
	}
	// Root reports an error → error with the text.
	n.writeStatus(t, time.Now(), "error="+n.pid+":192.0.2.0/24:insert failed\n")
	n.s.netpol.Status(time.Now().Add(5 * time.Hour))
	w = doAs(t, n.h, n.admin, http.MethodGet, path, "")
	m = decodeLan(t, w.Body.String())
	if m["lanState"] != "error" || !strings.Contains(m["lanMessage"].(string), "insert failed") {
		t.Fatalf("error line → error: %v", m)
	}
	// Stale status while root last said applied: never "off".
	n.writeStatus(t, time.Now().Add(-10*time.Minute), "lan="+n.pid+"\n")
	n.s.netpol.Status(time.Now().Add(6 * time.Hour))
	doAs(t, n.h, n.admin, http.MethodPut, path, `{"lanAccess":false}`)
	w = doAs(t, n.h, n.admin, http.MethodGet, path, "")
	m = decodeLan(t, w.Body.String())
	if m["lanState"] != "error" || !strings.Contains(m["lanMessage"].(string), "cannot confirm closed") {
		t.Fatalf("stale+applied after disable must not read off: %v", m)
	}
	if n.marker() {
		t.Fatal("disable must remove the marker")
	}
}

// A non-admin OWNER sees the state but never root's texts, which name the
// operator's LAN prefix (security review).
func TestNetworkTenantMessagesAreGeneric(t *testing.T) {
	n := newNetHarness(t)
	sp, err := n.s.space(identity.DeriveUsername(n.member))
	if err != nil {
		t.Fatal(err)
	}
	p, err := sp.projects.Add(project.Project{Name: "tenantproj", CreatedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if err := sp.projects.SetLanAccess(p.ID, true, time.Now().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	n.writeStatus(t, time.Now(), "conf_error=LAN_CIDRS: 192.0.2.7/24 has host bits set\nerror="+p.ID+":192.0.2.0/24:insert failed\n")
	n.s.netpol.Status(time.Now().Add(time.Hour))
	w := doAs(t, n.h, n.member, http.MethodGet, "/api/projects/"+p.ID+"/network", "")
	if w.Code != http.StatusOK {
		t.Fatalf("owner GET: %d %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if strings.Contains(body, "192.0.2") || strings.Contains(body, "insert failed") || strings.Contains(body, `"host"`) {
		t.Fatalf("tenant reply leaks root's text or the host view: %s", body)
	}
	m := decodeLan(t, body)
	if m["lanState"] != "error" || !strings.Contains(m["lanMessage"].(string), "ask the operator") {
		t.Fatalf("tenant should see a generic error: %v", m)
	}
	w = doAs(t, n.h, n.member, http.MethodGet, "/api/projects", "")
	if strings.Contains(w.Body.String(), "192.0.2") {
		t.Fatalf("tenant list leaks the prefix: %s", w.Body.String())
	}
	// The admin sees root's words for their own project.
	n.writeStatus(t, time.Now(), "error="+n.pid+":192.0.2.0/24:insert failed\n")
	n.s.netpol.Status(time.Now().Add(2 * time.Hour))
	doAs(t, n.h, n.admin, http.MethodPut, "/api/projects/"+n.pid+"/network", `{"lanAccess":false}`)
	w = doAs(t, n.h, n.admin, http.MethodGet, "/api/projects/"+n.pid+"/network", "")
	if !strings.Contains(w.Body.String(), "insert failed") {
		t.Fatalf("admin should see root's text: %s", w.Body.String())
	}
}

// A PUT that changes nothing must not kick root on every call (a tenant can
// loop disable) — it is throttled; a real change kicks at once.
func TestNetworkNoOpPutIsThrottled(t *testing.T) {
	n := newNetHarness(t)
	n.writeStatus(t, time.Now(), "")
	path := "/api/projects/" + n.pid + "/network"
	for i := 0; i < 5; i++ {
		if w := doAs(t, n.h, n.admin, http.MethodPut, path, `{"lanAccess":false}`); w.Code != http.StatusOK {
			t.Fatalf("disable %d: %d", i, w.Code)
		}
	}
	if got := n.spoolEntries(t); got != 1 {
		t.Fatalf("five no-op disables should kick once, got %d spool entries", got)
	}
	if w := doAs(t, n.h, n.admin, http.MethodPut, path, `{"lanAccess":true}`); w.Code != http.StatusOK {
		t.Fatalf("enable: %d %s", w.Code, w.Body.String())
	}
	if got := n.spoolEntries(t); got != 2 {
		t.Fatalf("a real change must kick at once, got %d", got)
	}
}

func TestNetworkHostViewIsAdminOnly(t *testing.T) {
	n := newNetHarness(t)
	n.writeStatus(t, time.Now(), "")
	// The member owns nothing here, so use the settings endpoint, which every
	// user can read: hostFirewall must be present for the admin only.
	w := doAs(t, n.h, n.admin, http.MethodGet, "/api/settings", "")
	if !strings.Contains(w.Body.String(), `"hostFirewall"`) || !strings.Contains(w.Body.String(), "192.0.2.0/24") {
		t.Fatalf("admin settings lack hostFirewall: %s", w.Body.String())
	}
	w = doAs(t, n.h, n.member, http.MethodGet, "/api/settings", "")
	if strings.Contains(w.Body.String(), "hostFirewall") || strings.Contains(w.Body.String(), "192.0.2.0/24") {
		t.Fatalf("member settings leak the host view: %s", w.Body.String())
	}
}

// Two overlapping PUTs end consistent: the record and the marker agree.
func TestNetworkConcurrentPutsStayConsistent(t *testing.T) {
	n := newNetHarness(t)
	n.writeStatus(t, time.Now(), "")
	path := "/api/projects/" + n.pid + "/network"
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(on bool) {
			defer wg.Done()
			body := `{"lanAccess":false}`
			if on {
				body = `{"lanAccess":true}`
			}
			if w := doAs(t, n.h, n.admin, http.MethodPut, path, body); w.Code != http.StatusOK {
				t.Errorf("PUT %v: %d %s", on, w.Code, w.Body.String())
			}
		}(i%2 == 0)
	}
	wg.Wait()
	sp, _ := n.s.space(identity.DeriveUsername(n.admin))
	p, _ := sp.projects.Get(n.pid)
	if p.LanAccess != n.marker() {
		t.Fatalf("record %v but marker %v", p.LanAccess, n.marker())
	}
}

func TestDeleteProjectRemovesMarker(t *testing.T) {
	n := newNetHarness(t)
	n.writeStatus(t, time.Now(), "")
	path := "/api/projects/" + n.pid + "/network"
	if w := doAs(t, n.h, n.admin, http.MethodPut, path, `{"lanAccess":true}`); w.Code != http.StatusOK {
		t.Fatalf("enable: %d %s", w.Code, w.Body.String())
	}
	if !n.marker() {
		t.Fatal("marker missing after enable")
	}
	// Image is empty, so no container was ever created; Destroy finds nothing
	// and the record path runs to completion in the background.
	if w := doAs(t, n.h, n.admin, http.MethodDelete, "/api/projects/"+n.pid, ""); w.Code != http.StatusAccepted {
		t.Fatalf("delete: %d %s", w.Code, w.Body.String())
	}
	deadline := time.Now().Add(10 * time.Second)
	for n.marker() && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if n.marker() {
		t.Fatal("marker survived the project's deletion")
	}
}
