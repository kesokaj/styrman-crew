package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
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

// Network-mode handler tests (ADR-067). There is no Docker in this harness, so
// s.orch.Status errors and the handler takes its "not running" path: the
// record is written and the rebuild is left to the next start. That is the
// behaviour under test here — the rebuild itself is covered by the
// orchestrator's bridge-name logic and by the live checks in
// testing/styrman/integration.sh.

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

// ─── Fail-closed on a failed apply (security review 2026-09-09, finding 1) ───

// fakeMode drives the three orchestrator calls a mode change makes, so the
// FAILURE paths are reachable without Docker. The concrete orchestrator cannot
// be made to fail Stop on demand, which is why this seam exists.
type fakeMode struct {
	state       string // what Status reports
	statusErr   error  // a real inspect failure — NOT "absent", which Status maps to (absent, nil)
	stopErr     error
	ensureErr   error
	stopCalls   int
	ensureCalls int
}

func (f *fakeMode) Status(_ context.Context, _, project string) (orchestrator.Status, error) {
	if f.statusErr != nil {
		return orchestrator.Status{}, f.statusErr
	}
	return orchestrator.Status{Project: project, State: f.state, ContainerID: "c1"}, nil
}

func (f *fakeMode) Stop(_ context.Context, _, _ string) error {
	f.stopCalls++
	return f.stopErr
}

func (f *fakeMode) EnsureRunning(_ context.Context, _, _ string, _ orchestrator.RunOpts) (string, error) {
	f.ensureCalls++
	return "c1", f.ensureErr
}

// A revocation whose apply fails must not leave the record claiming sandbox
// while the container keeps running on the lan- bridge. Nothing reconciles
// that: EnsureRunning's mismatch heal fires for STOPPED containers only, so a
// container left running on the wrong bridge stays there indefinitely, with
// the switch, the badge and the API all reporting the mode it does NOT have.
func TestNetworkFailedRevocationRollsBackTheRecord(t *testing.T) {
	n := newNetHarness(t)
	// Get it into LAN mode first, with a runtime that applies cleanly.
	n.s.mode = &fakeMode{state: "running"}
	if w := doAs(t, n.h, n.admin, http.MethodPut, "/api/projects/"+n.pid+"/network", `{"lanAccess":true}`); w.Code != http.StatusOK {
		t.Fatalf("enable: want 200, got %d", w.Code)
	}
	if !n.lanAccessOf(t, n.admin, n.pid) {
		t.Fatal("precondition: the project should be in LAN mode")
	}

	// Now revoke, with a Stop that fails: the container stays up, on lan-.
	fm := &fakeMode{state: "running", stopErr: errors.New("docker stop: timeout")}
	n.s.mode = fm
	w := doAs(t, n.h, n.admin, http.MethodPut, "/api/projects/"+n.pid+"/network", `{"lanAccess":false}`)
	code, body := w.Code, w.Body.String()
	if code != http.StatusInternalServerError {
		t.Fatalf("failed revocation: want 500, got %d (%s)", code, body)
	}
	if fm.stopCalls != 1 {
		t.Fatalf("Stop should have been attempted exactly once, got %d", fm.stopCalls)
	}
	if fm.ensureCalls != 0 {
		t.Fatal("EnsureRunning must not run after Stop failed")
	}
	// THE ASSERTION: the record must still say LAN, because LAN is still what
	// is in force. A false "sandboxed" here is the fail-open.
	if !n.lanAccessOf(t, n.admin, n.pid) {
		t.Fatal("record says sandbox after a FAILED revocation — the project still has LAN access and every surface now under-reports it")
	}
	// And the caller must be told the mode did not change, not that it did.
	if !strings.Contains(body, "NOT changed") {
		t.Errorf("the error should say the mode was not changed, got %q", body)
	}
}

// The other failure branch is different and must NOT roll back: if Stop
// SUCCEEDED and only the restart failed, the container is down, so it has no
// access at all and the record's new value is what the next start should
// apply. Rolling back there would silently discard the operator's change.
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

// The sibling of the Stop branch (preflight review, 2026-09-09): a Status
// ERROR is not "stopped". Status already maps a missing container to
// ("absent", nil), so a non-nil error is a real inspect failure and the
// container may be running on the old bridge. The first fix closed the
// fail-open on Stop and left this one, one line above it, open.
func TestNetworkStatusErrorOnRevocationRollsBack(t *testing.T) {
	n := newNetHarness(t)
	n.s.mode = &fakeMode{state: "running"}
	if w := doAs(t, n.h, n.admin, http.MethodPut, "/api/projects/"+n.pid+"/network", `{"lanAccess":true}`); w.Code != http.StatusOK {
		t.Fatalf("enable: want 200, got %d", w.Code)
	}
	fm := &fakeMode{statusErr: errors.New("docker inspect: connection refused")}
	n.s.mode = fm
	w := doAs(t, n.h, n.admin, http.MethodPut, "/api/projects/"+n.pid+"/network", `{"lanAccess":false}`)
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

// And "absent" must keep its old meaning: no container, the record is the
// truth, 200, and the next start applies it. Distinguishing an inspect
// FAILURE from a missing container is the whole point of the fix above.
func TestNetworkAbsentContainerJustWritesTheRecord(t *testing.T) {
	n := newNetHarness(t)
	fm := &fakeMode{state: "absent"}
	n.s.mode = fm
	w := doAs(t, n.h, n.admin, http.MethodPut, "/api/projects/"+n.pid+"/network", `{"lanAccess":true}`)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", w.Code, w.Body.String())
	}
	if fm.stopCalls != 0 || fm.ensureCalls != 0 {
		t.Fatal("an absent container is nothing to stop or rebuild")
	}
	if !n.lanAccessOf(t, n.admin, n.pid) {
		t.Fatal("the record must carry the new mode for the next start to apply")
	}
}
