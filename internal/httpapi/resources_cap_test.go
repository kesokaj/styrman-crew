package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/kesokaj/styrman-crew/internal/config"
	"github.com/kesokaj/styrman-crew/internal/identity"
	"github.com/kesokaj/styrman-crew/internal/orchestrator"
	"github.com/kesokaj/styrman-crew/internal/project"
)

// On resHost (16 CPUs, 60 GiB settable) the default non-admin cap is half:
// 8 CPUs, 30 GiB.

var memberSeq int

// memberProject gives the harness's non-admin user a project at the given
// size and returns its resources path and the member's space.
func memberProject(t *testing.T, n *netHarness, cpus, mem int) (string, *userSpace, project.Project) {
	t.Helper()
	sp, err := n.s.space(identity.DeriveUsername(n.member))
	if err != nil {
		t.Fatal(err)
	}
	memberSeq++
	p, err := sp.projects.Add(project.Project{Name: fmt.Sprintf("memberproj%d", memberSeq), CreatedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if err := sp.projects.SetResources(p.ID, cpus, mem, 20); err != nil {
		t.Fatal(err)
	}
	p, _ = sp.projects.Get(p.ID)
	return "/api/projects/" + p.ID + "/resources", sp, p
}

func startSize(t *testing.T, n *netHarness, sp *userSpace, user string, p project.Project) orchestrator.Resources {
	t.Helper()
	r, err := n.s.runOptsFor(sp, user, p, "x").Resources()
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestANonAdminProjectStartsAtMostAtTheCap(t *testing.T) {
	n, _, _ := resHarness(t)
	member := identity.DeriveUsername(n.member)
	for _, c := range []struct{ cpus, mem, wantCPUs, wantMem int }{
		{0, 0, 8, 30},   // unlimited = up to the cap
		{12, 40, 8, 30}, // a record from before the cap is trimmed
		{4, 10, 4, 10},  // under the cap: as asked
	} {
		_, sp, p := memberProject(t, n, c.cpus, c.mem)
		if r := startSize(t, n, sp, member, p); r.CPUs != c.wantCPUs || r.MemoryGiB != c.wantMem || r.DiskGB != 20 {
			t.Errorf("record %d/%d starts at %+v, want %d/%d", c.cpus, c.mem, r, c.wantCPUs, c.wantMem)
		}
		if got, _ := sp.projects.Get(p.ID); got.CPUs != c.cpus || got.MemoryGiB != c.mem {
			t.Errorf("the record was rewritten: %d/%d", got.CPUs, got.MemoryGiB)
		}
	}
	// The admin is not capped.
	spA, _ := n.s.space(identity.DeriveUsername(n.admin))
	if r := startSize(t, n, spA, identity.DeriveUsername(n.admin), n.record(t)); r.CPUs != 0 || r.MemoryGiB != 0 {
		t.Errorf("admin start capped: %+v", r)
	}
	// A configured cap replaces half the host.
	six, twenty := 6, 20
	if err := n.s.cfg.UpdateOperator(config.OperatorUpdate{IdleTimeout: -1, UserMaxCPUs: &six, UserMaxMemoryGiB: &twenty}); err != nil {
		t.Fatal(err)
	}
	_, sp, p := memberProject(t, n, 0, 0)
	if r := startSize(t, n, sp, member, p); r.CPUs != 6 || r.MemoryGiB != 20 {
		t.Errorf("configured cap: started at %+v", r)
	}
	// A host read that fails fails the start: never a silent uncapped one.
	n.s.size.(*fakeSize).hostErr = errors.New("docker info failed")
	if _, err := n.s.runOptsFor(sp, member, p, "x").Resources(); err == nil {
		t.Error("a non-admin start ran without the host size")
	}
}

func TestANonAdminSeesTheCapInTheDialog(t *testing.T) {
	n, _, _ := resHarness(t)
	path, _, _ := memberProject(t, n, 0, 0)
	w := doAs(t, n.h, n.member, http.MethodGet, path, "")
	v := decodeRes(t, w.Body.String())
	if w.Code != http.StatusOK || v.CPUs != 0 || v.MemoryGiB != 0 || !v.Limits.Capped || v.Limits.MaxCPUs != 8 || v.Limits.MaxMemoryGiB != 30 {
		t.Fatalf("unlimited record: %d %s", w.Code, w.Body.String())
	}
	path, _, _ = memberProject(t, n, 12, 40)
	v = decodeRes(t, doAs(t, n.h, n.member, http.MethodGet, path, "").Body.String())
	if v.CPUs != 8 || v.MemoryGiB != 30 {
		t.Fatalf("over-cap record reads as %d/%d, want the cap 8/30", v.CPUs, v.MemoryGiB)
	}
	// The admin sees the host.
	v = decodeRes(t, doAs(t, n.h, n.admin, http.MethodGet, "/api/projects/"+n.pid+"/resources", "").Body.String())
	if v.Limits.Capped || v.Limits.MaxCPUs != 16 {
		t.Fatalf("admin limits %+v", v.Limits)
	}
}

func TestANonAdminCannotAskAboveTheCap(t *testing.T) {
	n, fr, _ := resHarness(t)
	path, sp, p := memberProject(t, n, 0, 0)
	for _, body := range []string{`{"cpus":9,"memoryGiB":0,"diskGB":20}`, `{"cpus":0,"memoryGiB":31,"diskGB":20}`} {
		if w := doAs(t, n.h, n.member, http.MethodPut, path, body); w.Code != http.StatusBadRequest {
			t.Errorf("PUT %s: %d %s, want 400", body, w.Code, w.Body.String())
		}
	}
	if w := doAs(t, n.h, n.member, http.MethodPut, path, `{"cpus":8,"memoryGiB":30,"diskGB":20}`); w.Code != http.StatusOK {
		t.Fatalf("PUT at the cap: %d %s", w.Code, w.Body.String())
	}
	// Running at the cap, then asking for unlimited: the start size is the
	// same, so nothing is rebuilt (plan D10).
	fr.runningAt(8, 30, 20)
	w := doAs(t, n.h, n.member, http.MethodPut, path, `{"cpus":0,"memoryGiB":0,"diskGB":20}`)
	if v := decodeRes(t, w.Body.String()); w.Code != http.StatusOK || v.Rebuilt || fr.stops != 0 {
		t.Fatalf("cap -> unlimited: %d rebuilt=%v stops=%d", w.Code, v.Rebuilt, fr.stops)
	}
	if got, _ := sp.projects.Get(p.ID); got.CPUs != 0 || got.MemoryGiB != 0 {
		t.Fatalf("record %d/%d, want 0/0 saved", got.CPUs, got.MemoryGiB)
	}
}

func TestOwnerIsAdminMirrorsTheAuthGuard(t *testing.T) {
	n := newNetHarness(t)
	admin, member := identity.DeriveUsername(n.admin), identity.DeriveUsername(n.member)
	if !n.s.ownerIsAdmin(admin) || n.s.ownerIsAdmin(member) || n.s.ownerIsAdmin("") {
		t.Fatal("header mode: env admin only")
	}
	n.s.cfg.AuthMode = "none" // every request on such a listener is admin
	if !n.s.ownerIsAdmin(member) {
		t.Fatal("AuthMode none: owner not admin")
	}
	n.s.cfg.AuthMode = "header"
	n.s.cfg.AllowedEmails, n.s.cfg.Username = nil, "devuser"
	n.s.cfg.DevAddr = "127.0.0.1:9"
	if !n.s.ownerIsAdmin("devuser") {
		t.Fatal("the dev listener's default user is admin there")
	}
}

func TestTheAdminSetsTheCapsInRuntimeSettings(t *testing.T) {
	n, _, _ := resHarness(t)
	const p = "/api/settings/runtime"
	if w := doAs(t, n.h, n.admin, http.MethodPatch, p, `{"dockerDiskGB":0,"idleTimeout":"","userMaxCpus":4,"userMaxMemoryGiB":12}`); w.Code != http.StatusOK {
		t.Fatalf("PATCH: %d %s", w.Code, w.Body.String())
	}
	if c, m := n.s.cfg.UserCaps(); c != 4 || m != 12 {
		t.Fatalf("caps %d/%d", c, m)
	}
	// Absent caps are kept; 0/"" keep disk and idle as before.
	if w := doAs(t, n.h, n.admin, http.MethodPatch, p, `{"dockerDiskGB":0,"idleTimeout":""}`); w.Code != http.StatusOK {
		t.Fatalf("PATCH keep: %d", w.Code)
	}
	if c, m := n.s.cfg.UserCaps(); c != 4 || m != 12 {
		t.Fatalf("absent caps changed: %d/%d", c, m)
	}
	for _, body := range []string{`{"userMaxCpus":17}`, `{"userMaxMemoryGiB":61}`, `{"userMaxCpus":-1}`} {
		if w := doAs(t, n.h, n.admin, http.MethodPatch, p, body); w.Code != http.StatusBadRequest {
			t.Errorf("PATCH %s: %d, want 400", body, w.Code)
		}
	}
	if w := doAs(t, n.h, n.member, http.MethodPatch, p, `{"userMaxCpus":16}`); w.Code != http.StatusForbidden {
		t.Errorf("member PATCH: %d, want 403", w.Code)
	}
	if w := doAs(t, n.h, n.member, http.MethodGet, "/api/settings", ""); !strings.Contains(w.Body.String(), `"userMaxCpus":4`) {
		t.Errorf("GET /api/settings lacks the cap: %s", w.Body.String())
	}
}
