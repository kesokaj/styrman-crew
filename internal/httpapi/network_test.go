package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
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

// Network-mode handler tests (ADR-067). The tests above the fakeMode section
// run against the real orchestrator with no container behind the project, so
// Status reports "absent" and the handler takes its "nothing in force" path:
// the record is written and the rebuild is left to the next start. The
// fakeMode section below drives the stop/persist/rebuild order and its
// failure branches; the rebuild itself is covered by the orchestrator's
// bridge-name logic and by the live checks in testing/styrman/integration.sh.

type netHarness struct {
	h      http.Handler
	s      *Server
	state  string
	admin  string
	member string
	pid    string
}

func newNetHarness(t *testing.T) *netHarness {
	t.Helper()
	dir := t.TempDir()
	admin, member := "admin@gmail.com", "member@gmail.com"
	cfg := &config.Config{
		StateDir: dir, HomesDir: filepath.Join(dir, "homes"), Username: "tester",
		AuthMode: "header", IngressToken: "test-ingress-token", AllowedEmails: []string{admin},
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
	sp, err := s.space(identity.DeriveUsername(admin))
	if err != nil {
		t.Fatal(err)
	}
	p, err := sp.projects.Add(project.Project{Name: "lanproj", CreatedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	return &netHarness{h: s.Routes(), s: s, state: dir, admin: admin, member: member, pid: p.ID}
}

func decodeLan(t *testing.T, body string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		t.Fatalf("bad JSON %q: %v", body, err)
	}
	return m
}

// lanAccessOf reads the persisted record, which is the single source of truth
// for the mode — never the reply, which is what the request wanted.
func (n *netHarness) lanAccessOf(t *testing.T, email, id string) bool {
	t.Helper()
	sp, err := n.s.space(identity.DeriveUsername(email))
	if err != nil {
		t.Fatal(err)
	}
	p, err := sp.projects.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	return p.LanAccess
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
	// Another authorized user must see 404 on both verbs — the 404 comes
	// before any authorization signal, so another user's project is
	// indistinguishable from one that does not exist.
	if w := doAs(t, n.h, n.member, http.MethodGet, path, ""); w.Code != http.StatusNotFound {
		t.Errorf("member GET: %d, want 404", w.Code)
	}
	if w := doAs(t, n.h, n.member, http.MethodPut, path, `{"lanAccess":true}`); w.Code != http.StatusNotFound {
		t.Errorf("member PUT: %d, want 404", w.Code)
	}
	if n.lanAccessOf(t, n.admin, n.pid) {
		t.Fatal("a cross-user PUT changed the owner's record")
	}
}

// Enabling is admin-only; disabling is always allowed to the owner, because
// tightening must never be refused.
func TestNetworkEnableIsAdminOnlyDisableIsNot(t *testing.T) {
	n := newNetHarness(t)
	sp, err := n.s.space(identity.DeriveUsername(n.member))
	if err != nil {
		t.Fatal(err)
	}
	p, err := sp.projects.Add(project.Project{Name: "tenantproj", CreatedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/projects/" + p.ID + "/network"

	// The non-admin owner cannot enable.
	if w := doAs(t, n.h, n.member, http.MethodPut, path, `{"lanAccess":true}`); w.Code != http.StatusForbidden {
		t.Fatalf("member enable: %d %s, want 403", w.Code, w.Body.String())
	}
	if n.lanAccessOf(t, n.member, p.ID) {
		t.Fatal("a refused enable must not persist")
	}
	// An admin enables it for them...
	if w := doAs(t, n.h, n.admin, http.MethodPut, path, `{"lanAccess":true}`); w.Code != http.StatusNotFound {
		t.Fatalf("admin acting on another user's project must 404, got %d", w.Code)
	}
	// ...but there is no admin path into another user's store, so set it
	// directly and prove the OWNER can turn it back off.
	if err := sp.projects.SetLanAccess(p.ID, true); err != nil {
		t.Fatal(err)
	}
	if w := doAs(t, n.h, n.member, http.MethodPut, path, `{"lanAccess":false}`); w.Code != http.StatusOK {
		t.Fatalf("member disable: %d %s, want 200", w.Code, w.Body.String())
	}
	if n.lanAccessOf(t, n.member, p.ID) {
		t.Fatal("owner disable did not persist")
	}
}

// A PUT that changes nothing is a 200 that did not rebuild anything.
func TestNetworkNoOpPutDoesNotRebuild(t *testing.T) {
	n := newNetHarness(t)
	path := "/api/projects/" + n.pid + "/network"
	for i := 0; i < 3; i++ {
		w := doAs(t, n.h, n.admin, http.MethodPut, path, `{"lanAccess":false}`)
		if w.Code != http.StatusOK {
			t.Fatalf("no-op disable %d: %d", i, w.Code)
		}
		if m := decodeLan(t, w.Body.String()); m["rebuilt"] != false || m["lanAccess"] != false {
			t.Fatalf("no-op reply should be {lanAccess:false, rebuilt:false}, got %v", m)
		}
	}
}

// The mode round-trips through the record and both project views.
func TestNetworkModeRoundTrips(t *testing.T) {
	n := newNetHarness(t)
	path := "/api/projects/" + n.pid + "/network"

	if m := decodeLan(t, doAs(t, n.h, n.admin, http.MethodGet, path, "").Body.String()); m["lanAccess"] != false {
		t.Fatalf("a new project must default to sandbox, got %v", m)
	}
	if w := doAs(t, n.h, n.admin, http.MethodPut, path, `{"lanAccess":true}`); w.Code != http.StatusOK {
		t.Fatalf("enable: %d %s", w.Code, w.Body.String())
	}
	if !n.lanAccessOf(t, n.admin, n.pid) {
		t.Fatal("enable did not persist to the record")
	}
	if m := decodeLan(t, doAs(t, n.h, n.admin, http.MethodGet, path, "").Body.String()); m["lanAccess"] != true {
		t.Fatalf("GET after enable: %v", m)
	}
	// Both the single-project view and the list carry it, so the UI never has
	// to ask a second endpoint to render a badge.
	one := decodeLan(t, doAs(t, n.h, n.admin, http.MethodGet, "/api/projects/"+n.pid, "").Body.String())
	if one["lanAccess"] != true {
		t.Fatalf("project view is missing lanAccess: %v", one)
	}
	list := decodeLan(t, doAs(t, n.h, n.admin, http.MethodGet, "/api/projects", "").Body.String())
	ps, _ := list["projects"].([]any)
	if len(ps) != 1 {
		t.Fatalf("want 1 project, got %v", list)
	}
	if ps[0].(map[string]any)["lanAccess"] != true {
		t.Fatalf("list view is missing lanAccess: %v", ps[0])
	}
	// And back off again.
	if w := doAs(t, n.h, n.admin, http.MethodPut, path, `{"lanAccess":false}`); w.Code != http.StatusOK {
		t.Fatalf("disable: %d", w.Code)
	}
	if n.lanAccessOf(t, n.admin, n.pid) {
		t.Fatal("disable did not persist")
	}
}

// Concurrent PUTs must leave the record at one of the requested values, never
// torn — netMu plus the store's own lock.
func TestNetworkConcurrentPutsStayConsistent(t *testing.T) {
	n := newNetHarness(t)
	path := "/api/projects/" + n.pid + "/network"
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			body := `{"lanAccess":false}`
			if i%2 == 0 {
				body = `{"lanAccess":true}`
			}
			if w := doAs(t, n.h, n.admin, http.MethodPut, path, body); w.Code != http.StatusOK {
				t.Errorf("concurrent PUT %d: %d %s", i, w.Code, w.Body.String())
			}
		}(i)
	}
	wg.Wait()
	// Whatever won, the record must be readable and the GET must agree with it.
	rec := n.lanAccessOf(t, n.admin, n.pid)
	if m := decodeLan(t, doAs(t, n.h, n.admin, http.MethodGet, path, "").Body.String()); m["lanAccess"] != rec {
		t.Fatalf("GET %v disagrees with the record %v after concurrent PUTs", m["lanAccess"], rec)
	}
}

// A LAN-enabled project's record goes away with the project — there is no
// separate marker to leak, which is the point of ADR-067, but the record
// itself must still be removed under netMu.
func TestDeleteProjectRemovesTheRecord(t *testing.T) {
	n := newNetHarness(t)
	sp, err := n.s.space(identity.DeriveUsername(n.admin))
	if err != nil {
		t.Fatal(err)
	}
	if err := sp.projects.SetLanAccess(n.pid, true); err != nil {
		t.Fatal(err)
	}
	if _, err := sp.projects.Get(n.pid); err != nil {
		t.Fatalf("precondition: %v", err)
	}
	if err := sp.projects.Remove(n.pid); err != nil {
		t.Fatal(err)
	}
	if _, err := sp.projects.Get(n.pid); err == nil {
		t.Fatal("record survived removal")
	}
}

// ─── Fail-closed on a failed apply (security review 2026-09-09, findings 1–3) ───

// fakeMode drives the locked calls a mode change makes, so the FAILURE paths
// are reachable without Docker. It models the one fact the handler decides
// on: which bridge the container is on (bridgeLan), which only a successful
// start changes — and the start reads its mode through RunOpts.Lan, exactly
// as the orchestrator does, so a stale snapshot would show up here too.
type fakeMode struct {
	state         string // what Status reports
	bridgeLan     bool   // the LIVE bridge: true = lan-, false = sdb-
	statusErr     error  // a real inspect failure — NOT "absent", which Status maps to (absent, nil)
	mismatchErr   error  // a network-inspect failure (BUG-031's shape)
	stopErr       error
	ensureErr     error
	stopCalls     int
	ensureCalls   int
	mismatchCalls int
	ensureLan     []bool // the mode each start read through RunOpts.Lan
	onLock        func() // runs when the lock is taken, before the handler's decision
	onStop        func() // runs inside Stop, before it returns success
}

func (f *fakeMode) WithProject(_, _ string, fn func(orchestrator.Project)) {
	if f.onLock != nil {
		f.onLock()
	}
	fn(f)
}

func (f *fakeMode) Status(_ context.Context) (orchestrator.Status, error) {
	if f.statusErr != nil {
		return orchestrator.Status{}, f.statusErr
	}
	return orchestrator.Status{State: f.state, ContainerID: "c1"}, nil
}

func (f *fakeMode) ModeMismatch(_ context.Context, lan bool) (bool, error) {
	f.mismatchCalls++
	if f.mismatchErr != nil {
		return false, f.mismatchErr
	}
	return f.bridgeLan != lan, nil
}

func (f *fakeMode) Stop(_ context.Context) error {
	f.stopCalls++
	if f.stopErr != nil {
		return f.stopErr
	}
	f.state = "stopped"
	if f.onStop != nil {
		f.onStop()
	}
	return nil
}

func (f *fakeMode) EnsureRunning(_ context.Context, opts orchestrator.RunOpts) (string, error) {
	f.ensureCalls++
	lan, err := opts.Lan()
	if err != nil {
		return "", err
	}
	f.ensureLan = append(f.ensureLan, lan)
	if f.ensureErr != nil {
		return "", f.ensureErr
	}
	f.state, f.bridgeLan = "running", lan
	return "c1", nil
}

// lanRunning is a project already running in LAN mode with a record that
// agrees — the starting point of every revocation case.
func lanRunning(t *testing.T) (*netHarness, string) {
	t.Helper()
	n := newNetHarness(t)
	n.s.mode = &fakeMode{state: "running"}
	path := "/api/projects/" + n.pid + "/network"
	if w := doAs(t, n.h, n.admin, http.MethodPut, path, `{"lanAccess":true}`); w.Code != http.StatusOK {
		t.Fatalf("enable: want 200, got %d %s", w.Code, w.Body.String())
	}
	if !n.lanAccessOf(t, n.admin, n.pid) {
		t.Fatal("precondition: the project should be in LAN mode")
	}
	return n, path
}

// A revocation whose Stop fails must leave the record saying LAN, because LAN
// is still what is in force: the container never left its lan- bridge.
// Nothing reconciles a record that says otherwise — EnsureRunning's mismatch
// heal fires for STOPPED containers only — so the record is now written only
// AFTER the stop, and there is nothing to roll back (BUG-028).
func TestNetworkFailedRevocationChangesNothing(t *testing.T) {
	n, path := lanRunning(t)
	fm := &fakeMode{state: "running", bridgeLan: true, stopErr: errors.New("docker stop: timeout")}
	n.s.mode = fm
	w := doAs(t, n.h, n.admin, http.MethodPut, path, `{"lanAccess":false}`)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("failed revocation: want 500, got %d (%s)", w.Code, w.Body.String())
	}
	if fm.stopCalls != 1 || fm.ensureCalls != 0 {
		t.Fatalf("want stop=1 ensure=0, got stop=%d ensure=%d", fm.stopCalls, fm.ensureCalls)
	}
	// THE ASSERTION: the record must still say LAN. A false "sandboxed" here
	// is the fail-open.
	if !n.lanAccessOf(t, n.admin, n.pid) {
		t.Fatal("record says sandbox after a FAILED revocation — the project still has LAN access and every surface now under-reports it")
	}
	if !strings.Contains(w.Body.String(), "NOT changed") {
		t.Errorf("the error should say the mode was not changed, got %q", w.Body.String())
	}
}

// The other failure branch is different and must NOT lose the change: if Stop
// SUCCEEDED and only the restart failed, the container is down, so it has no
// access at all, and the record's new value is what the next start applies.
func TestNetworkFailedRestartKeepsTheRecord(t *testing.T) {
	n := newNetHarness(t)
	fm := &fakeMode{state: "running", ensureErr: errors.New("kata: boot failed")}
	n.s.mode = fm
	w := doAs(t, n.h, n.admin, http.MethodPut, "/api/projects/"+n.pid+"/network", `{"lanAccess":true}`)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", w.Code)
	}
	if fm.stopCalls != 1 || fm.ensureCalls != 1 {
		t.Fatalf("want stop=1 ensure=1, got stop=%d ensure=%d", fm.stopCalls, fm.ensureCalls)
	}
	if !n.lanAccessOf(t, n.admin, n.pid) {
		t.Fatal("a failed RESTART must keep the new record: the container is stopped, so nothing is in force, and the next start applies it")
	}
}

// A Status ERROR is not "stopped" (BUG-029): Status maps a missing container
// to ("absent", nil), so a non-nil error is a real inspect failure and the
// container may be running on the old bridge. Nothing may be written.
func TestNetworkStatusErrorChangesNothing(t *testing.T) {
	n, path := lanRunning(t)
	fm := &fakeMode{statusErr: errors.New("docker inspect: connection refused")}
	n.s.mode = fm
	w := doAs(t, n.h, n.admin, http.MethodPut, path, `{"lanAccess":false}`)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d (%s)", w.Code, w.Body.String())
	}
	if fm.stopCalls != 0 || fm.ensureCalls != 0 {
		t.Fatalf("nothing may be stopped or rebuilt when the project cannot be inspected; stop=%d ensure=%d", fm.stopCalls, fm.ensureCalls)
	}
	if !n.lanAccessOf(t, n.admin, n.pid) {
		t.Fatal("record says sandbox after an uninspectable revocation — LAN may still be in force and nothing would ever correct it")
	}
	if !strings.Contains(w.Body.String(), "NOT changed") {
		t.Errorf("the error should say the mode was not changed, got %q", w.Body.String())
	}
}

// The same rule one layer down (BUG-031): when the NETWORK cannot be
// inspected, "I cannot tell whether the bridge is wrong" must not read as
// "nothing to do" — and must not stop a container it may not need to stop.
func TestNetworkMismatchErrorChangesNothing(t *testing.T) {
	n, path := lanRunning(t)
	fm := &fakeMode{state: "running", bridgeLan: true, mismatchErr: errors.New("network inspect: EOF")}
	n.s.mode = fm
	w := doAs(t, n.h, n.admin, http.MethodPut, path, `{"lanAccess":false}`)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d (%s)", w.Code, w.Body.String())
	}
	if fm.mismatchCalls != 1 || fm.stopCalls != 0 || fm.ensureCalls != 0 {
		t.Fatalf("want mismatch=1 stop=0 ensure=0, got %d/%d/%d", fm.mismatchCalls, fm.stopCalls, fm.ensureCalls)
	}
	if !n.lanAccessOf(t, n.admin, n.pid) {
		t.Fatal("the record changed although the bridge could not be verified")
	}
	if !strings.Contains(w.Body.String(), "NOT changed") {
		t.Errorf("the error should say the mode was not changed, got %q", w.Body.String())
	}
}

// "absent" keeps its meaning: no container, the record is the truth, 200, and
// the next start applies it. Nothing is stopped and the bridge is not even
// asked about — nothing is in force.
func TestNetworkAbsentContainerJustWritesTheRecord(t *testing.T) {
	n := newNetHarness(t)
	fm := &fakeMode{state: "absent"}
	n.s.mode = fm
	w := doAs(t, n.h, n.admin, http.MethodPut, "/api/projects/"+n.pid+"/network", `{"lanAccess":true}`)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", w.Code, w.Body.String())
	}
	if fm.mismatchCalls != 0 || fm.stopCalls != 0 || fm.ensureCalls != 0 {
		t.Fatal("an absent container is nothing to inspect, stop or rebuild")
	}
	if !n.lanAccessOf(t, n.admin, n.pid) {
		t.Fatal("the record must carry the new mode for the next start to apply")
	}
	if m := decodeLan(t, w.Body.String()); m["rebuilt"] != false {
		t.Fatalf("nothing was rebuilt, got %v", m)
	}
}

// The security review's round-3 finding: the old no-op short-circuit trusted
// the record, so once the record and the bridge had drifted apart (a persist
// failure, a start racing a toggle) re-issuing the PUT answered 200 and
// touched nothing; the only recovery was Stop, then Start. The no-op decision
// now compares against the LIVE bridge, so the re-PUT IS the recovery.
func TestNetworkRePutRepairsADriftedRecord(t *testing.T) {
	n := newNetHarness(t)
	// The fail-open state itself: record says sandbox (the default), the
	// container is running on lan-.
	fm := &fakeMode{state: "running", bridgeLan: true}
	n.s.mode = fm
	w := doAs(t, n.h, n.admin, http.MethodPut, "/api/projects/"+n.pid+"/network", `{"lanAccess":false}`)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", w.Code, w.Body.String())
	}
	if fm.stopCalls != 1 || fm.ensureCalls != 1 {
		t.Fatalf("a re-PUT against a drifted record must rebuild: want stop=1 ensure=1, got %d/%d", fm.stopCalls, fm.ensureCalls)
	}
	if len(fm.ensureLan) != 1 || fm.ensureLan[0] {
		t.Fatalf("the rebuild must be asked for SANDBOX, got %v", fm.ensureLan)
	}
	if fm.bridgeLan {
		t.Fatal("the container is still on lan- after the repair")
	}
	if m := decodeLan(t, w.Body.String()); m["rebuilt"] != true || m["lanAccess"] != false {
		t.Fatalf("reply should be {lanAccess:false, rebuilt:true}, got %v", m)
	}
	if n.lanAccessOf(t, n.admin, n.pid) {
		t.Fatal("record should say sandbox")
	}
}

// The converse drift — record behind a bridge that is already right — needs
// no rebuild at all: the record is corrected and the running container is
// left alone.
func TestNetworkRecordBehindTheBridgeOnlyWritesTheRecord(t *testing.T) {
	n := newNetHarness(t)
	fm := &fakeMode{state: "running", bridgeLan: true} // record: sandbox
	n.s.mode = fm
	w := doAs(t, n.h, n.admin, http.MethodPut, "/api/projects/"+n.pid+"/network", `{"lanAccess":true}`)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", w.Code, w.Body.String())
	}
	if fm.stopCalls != 0 || fm.ensureCalls != 0 {
		t.Fatalf("the bridge already matches; nothing may be stopped or rebuilt, got stop=%d ensure=%d", fm.stopCalls, fm.ensureCalls)
	}
	if !n.lanAccessOf(t, n.admin, n.pid) {
		t.Fatal("the record was not corrected")
	}
	if m := decodeLan(t, w.Body.String()); m["rebuilt"] != false {
		t.Fatalf("nothing was rebuilt, got %v", m)
	}
}

// And a true no-op — record and bridge agree — still stops nothing and
// rebuilds nothing, on a RUNNING container.
func TestNetworkNoOpWhenRecordAndBridgeAgree(t *testing.T) {
	n, path := lanRunning(t)
	fm := &fakeMode{state: "running", bridgeLan: true}
	n.s.mode = fm
	w := doAs(t, n.h, n.admin, http.MethodPut, path, `{"lanAccess":true}`)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", w.Code, w.Body.String())
	}
	if fm.mismatchCalls != 1 || fm.stopCalls != 0 || fm.ensureCalls != 0 {
		t.Fatalf("want mismatch=1 stop=0 ensure=0, got %d/%d/%d", fm.mismatchCalls, fm.stopCalls, fm.ensureCalls)
	}
	if m := decodeLan(t, w.Body.String()); m["rebuilt"] != false {
		t.Fatalf("a no-op must not report a rebuild, got %v", m)
	}
}

// The last branch the old design could not close (security round 2, finding
// 1): the store write fails right after the stop. The record still names the
// old mode and the container is DOWN — it over-claims against a container
// that has nothing, the safe direction — and the caller is told exactly that
// state. Nothing is restarted: down is the one state with nothing in force.
func TestNetworkPersistFailureAfterStopLeavesItDownInThePreviousMode(t *testing.T) {
	n, path := lanRunning(t)
	fm := &fakeMode{state: "running", bridgeLan: true}
	n.s.mode = fm
	// The store saves by writing a temp file beside projects.json and
	// renaming it, so an unwritable directory fails the persist and nothing
	// else. Not meaningful as root, which ignores directory modes.
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions; the persist cannot be made to fail this way")
	}
	dir := n.s.cfg.UserStateDir(identity.DeriveUsername(n.admin))
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	w := doAs(t, n.h, n.admin, http.MethodPut, path, `{"lanAccess":false}`)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d (%s)", w.Code, w.Body.String())
	}
	if fm.stopCalls != 1 || fm.ensureCalls != 0 {
		t.Fatalf("want stop=1 ensure=0, got stop=%d ensure=%d", fm.stopCalls, fm.ensureCalls)
	}
	if fm.state != "stopped" {
		t.Fatalf("the container should be left down, got %q", fm.state)
	}
	if !n.lanAccessOf(t, n.admin, n.pid) {
		t.Fatal("the record changed although the persist failed")
	}
	if b := w.Body.String(); !strings.Contains(b, "NOT changed") || !strings.Contains(b, "stopped") {
		t.Errorf("the error should say the project was stopped and the mode not changed, got %q", b)
	}
}

// The sibling of the case above (review 2026-09-10, finding 1): the bridge
// already matches the request and only the record is behind it — the drift
// itself, which this PUT exists to repair — and the persist fails. The
// container is left alone (nothing to stop: it is where it should be), the
// record keeps its stale value, and the caller is told that the reported
// mode is the stale one, not merely that a write failed.
func TestNetworkPersistFailureOnDriftedRecordSaysWhatIsReported(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions; the persist cannot be made to fail this way")
	}
	n := newNetHarness(t)
	fm := &fakeMode{state: "running", bridgeLan: true} // record: sandbox, bridge: lan-
	n.s.mode = fm
	dir := n.s.cfg.UserStateDir(identity.DeriveUsername(n.admin))
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	w := doAs(t, n.h, n.admin, http.MethodPut, "/api/projects/"+n.pid+"/network", `{"lanAccess":true}`)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d (%s)", w.Code, w.Body.String())
	}
	if fm.stopCalls != 0 || fm.ensureCalls != 0 || fm.state != "running" {
		t.Fatalf("a container already on the requested bridge must be left running; stop=%d ensure=%d state=%q", fm.stopCalls, fm.ensureCalls, fm.state)
	}
	if n.lanAccessOf(t, n.admin, n.pid) {
		t.Fatal("the record changed although the persist failed")
	}
	if b := w.Body.String(); !strings.Contains(b, "already on the requested bridge") {
		t.Errorf("the error should say the project keeps reporting the previous mode, got %q", b)
	}
}

// RunOpts.Lan is a READ the orchestrator performs under the project lock,
// not a value the caller snapshotted when the request began: a start that
// waits on the lock while a mode change completes must see the record that
// change wrote (security review 2026-09-10, round 2).
func TestLanOfReadsTheRecordAtCallTime(t *testing.T) {
	n := newNetHarness(t)
	sp, err := n.s.space(identity.DeriveUsername(n.admin))
	if err != nil {
		t.Fatal(err)
	}
	read := lanOf(sp, n.pid)
	if lan, err := read(); err != nil || lan {
		t.Fatalf("before: want sandbox, got %v %v", lan, err)
	}
	if err := sp.projects.SetLanAccess(n.pid, true); err != nil {
		t.Fatal(err)
	}
	if lan, err := read(); err != nil || !lan {
		t.Fatalf("after the change: want LAN, got %v %v", lan, err)
	}
	if err := sp.projects.Remove(n.pid); err != nil {
		t.Fatal(err)
	}
	if _, err := read(); err == nil {
		t.Fatal("a deleted record must be an error, never a default mode")
	}
}

// The no-op decision reads the record UNDER the lock, not the copy the
// handler took before it (security review 2026-09-10, round 3): a PUT that
// waited on the lock while another PUT changed the record must act on the
// record as changed. Here the project is stopped, so the live bridge is not
// consulted and only the record can tell a no-op from a change.
func TestNetworkNoOpDecisionReadsTheRecordUnderTheLock(t *testing.T) {
	n := newNetHarness(t)
	sp, err := n.s.space(identity.DeriveUsername(n.admin))
	if err != nil {
		t.Fatal(err)
	}
	fm := &fakeMode{state: "stopped"}
	// Between this request's record read and its lock, another request
	// enables LAN.
	fm.onLock = func() {
		if err := sp.projects.SetLanAccess(n.pid, true); err != nil {
			t.Fatal(err)
		}
	}
	n.s.mode = fm
	w := doAs(t, n.h, n.admin, http.MethodPut, "/api/projects/"+n.pid+"/network", `{"lanAccess":false}`)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", w.Code, w.Body.String())
	}
	// THE ASSERTION: the record must say sandbox. A handler comparing against
	// its pre-lock copy sees "sandbox == sandbox", calls this a no-op, and
	// leaves the record saying LAN under a reply that says sandbox.
	if n.lanAccessOf(t, n.admin, n.pid) {
		t.Fatal("record says LAN after a disable that answered 200 — the no-op decision used a stale copy of the record")
	}
}

// A project whose record disappears between the re-read under the lock and
// the persist (deleteProject removes the record under netMu, which the
// persist takes AFTER the project lock) is reported as gone — 404 — not as a
// persist failure with a sentence about a project's state (review
// 2026-09-10 of the release delta: the branch had no test). Both persist
// sites: after a stop, and on the record-only path.
func TestNetworkProjectDeletedMidPutIs404(t *testing.T) {
	n, path := lanRunning(t)
	sp, err := n.s.space(identity.DeriveUsername(n.admin))
	if err != nil {
		t.Fatal(err)
	}
	// Rebuild path: the delete lands while the container is being stopped.
	fm := &fakeMode{state: "running", bridgeLan: true}
	fm.onStop = func() {
		if err := sp.projects.Remove(n.pid); err != nil {
			t.Fatal(err)
		}
	}
	n.s.mode = fm
	w := doAs(t, n.h, n.admin, http.MethodPut, path, `{"lanAccess":false}`)
	if w.Code != http.StatusNotFound {
		t.Fatalf("deleted after the stop: want 404, got %d (%s)", w.Code, w.Body.String())
	}
	if fm.ensureCalls != 0 {
		t.Fatal("nothing may be started for a project that no longer exists")
	}

	// Record-only path: the record goes away after the handler's re-read
	// and before its persist — emulated from inside Status, which sits
	// between the two. Not reachable through deleteProject today (Destroy
	// holds the project lock before Remove), so this pins the reply shape,
	// not a live interleaving.
	n2 := newNetHarness(t)
	sp2, err := n2.s.space(identity.DeriveUsername(n2.admin))
	if err != nil {
		t.Fatal(err)
	}
	n2.s.mode = &statusHookMode{fakeMode: &fakeMode{state: "stopped"}, onStatus: func() {
		if err := sp2.projects.Remove(n2.pid); err != nil {
			t.Fatal(err)
		}
	}}
	w2 := doAs(t, n2.h, n2.admin, http.MethodPut, "/api/projects/"+n2.pid+"/network", `{"lanAccess":true}`)
	if w2.Code != http.StatusNotFound {
		t.Fatalf("deleted before the record-only persist: want 404, got %d (%s)", w2.Code, w2.Body.String())
	}
}

// statusHookMode is fakeMode with a hook inside Status.
type statusHookMode struct {
	*fakeMode
	onStatus func()
}

func (m *statusHookMode) WithProject(_, _ string, fn func(orchestrator.Project)) { fn(m) }

func (m *statusHookMode) Status(ctx context.Context) (orchestrator.Status, error) {
	m.onStatus()
	return m.fakeMode.Status(ctx)
}
