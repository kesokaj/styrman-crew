package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/kesokaj/styrman-crew/internal/identity"
	"github.com/kesokaj/styrman-crew/internal/orchestrator"
	"github.com/kesokaj/styrman-crew/internal/project"
)

// Resources handler tests (ADR-083). The orchestrator's sizing rules
// (WantSizing) are the real ones — tested in internal/orchestrator — behind a
// fake host and a fake live container; the locked stop/persist/start order is
// driven through fakeRes, the same shape as network_test.go's fakeMode.

// The NUC's size: 16 CPUs, 63853 MiB (60 GiB settable by hand).
var resHost = orchestrator.Host{CPUs: 16, MemMiB: 63853}

type fakeSize struct {
	o       *orchestrator.Orchestrator // for the real WantSizing
	host    orchestrator.Host
	hostErr error
	res     *fakeRes // the live container, shared with the locked view
	liveErr error
}

func (f *fakeSize) Host(context.Context) (orchestrator.Host, error) { return f.host, f.hostErr }
func (f *fakeSize) LiveSizing(context.Context, string) (orchestrator.Live, bool, error) {
	if f.liveErr != nil {
		return orchestrator.Live{}, false, f.liveErr
	}
	return f.res.live, f.res.found, nil
}
func (f *fakeSize) WantSizing(h orchestrator.Host, r orchestrator.Resources, live *orchestrator.Live) orchestrator.Sizing {
	return f.o.WantSizing(h, r, live)
}

type fakeRes struct {
	size       *fakeSize
	live       orchestrator.Live
	found      bool
	stopErr    error
	ensureErr  error
	onStop     func()
	locks      int
	stops      int
	ensures    int
	ensuredFor []orchestrator.Resources // what each start read through RunOpts.Resources
}

func (f *fakeRes) WithProject(_, _ string, fn func(orchestrator.Project)) {
	f.locks++
	fn(f)
}
func (f *fakeRes) Status(context.Context) (orchestrator.Status, error) {
	if f.live.Running {
		return orchestrator.Status{State: "running"}, nil
	}
	return orchestrator.Status{State: "stopped"}, nil
}
func (f *fakeRes) ModeMismatch(context.Context, bool) (bool, error) { return false, nil }
func (f *fakeRes) Stop(context.Context) error {
	f.stops++
	if f.stopErr != nil {
		return f.stopErr
	}
	f.live.Running = false
	if f.onStop != nil {
		f.onStop()
	}
	return nil
}
func (f *fakeRes) EnsureRunning(_ context.Context, opts orchestrator.RunOpts) (string, error) {
	f.ensures++
	if opts.Resources == nil {
		return "", errors.New("RunOpts.Resources not set")
	}
	r, err := opts.Resources()
	if err != nil {
		return "", err
	}
	f.ensuredFor = append(f.ensuredFor, r)
	if f.ensureErr != nil {
		return "", f.ensureErr
	}
	f.live = orchestrator.Live{Sizing: f.size.WantSizing(f.size.host, r, &f.live), Running: true}
	return "c1", nil
}

// resHarness is netHarness with the fakes in place and the project's record
// at diskGB 20.
func resHarness(t *testing.T) (*netHarness, *fakeRes, string) {
	t.Helper()
	n := newNetHarness(t)
	fs := &fakeSize{o: n.s.orch, host: resHost}
	fr := &fakeRes{size: fs}
	fs.res = fr
	n.s.size, n.s.mode = fs, fr
	n.setRecord(t, 0, 0, 20)
	return n, fr, "/api/projects/" + n.pid + "/resources"
}

func (n *netHarness) record(t *testing.T) project.Project {
	t.Helper()
	sp, err := n.s.space(identity.DeriveUsername(n.admin))
	if err != nil {
		t.Fatal(err)
	}
	p, err := sp.projects.Get(n.pid)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func (n *netHarness) setRecord(t *testing.T, cpus, mem, disk int) {
	t.Helper()
	sp, err := n.s.space(identity.DeriveUsername(n.admin))
	if err != nil {
		t.Fatal(err)
	}
	if err := sp.projects.SetResources(n.pid, cpus, mem, disk); err != nil {
		t.Fatal(err)
	}
}

// runningAt plants a running container of the given size.
func (f *fakeRes) runningAt(cpus int64, memGiB int64, disk int) {
	f.found = true
	f.live = orchestrator.Live{Sizing: orchestrator.Sizing{NanoCPUs: cpus * 1e9, MemBytes: memGiB << 30, DiskGB: disk}, Running: true}
}

// The whole host, as WantSizing spells "unlimited" on resHost.
func (f *fakeRes) runningUnlimited(disk int) {
	f.found = true
	f.live = orchestrator.Live{Sizing: orchestrator.Sizing{NanoCPUs: 16e9, MemBytes: 61696 << 20, DiskGB: disk}, Running: true}
}

func decodeRes(t *testing.T, body string) resourcesReply {
	t.Helper()
	var r resourcesReply
	if err := json.Unmarshal([]byte(body), &r); err != nil {
		t.Fatalf("bad JSON %q: %v", body, err)
	}
	return r
}

func TestResourcesValidation(t *testing.T) {
	n, fr, path := resHarness(t)
	for _, tc := range []struct{ body, mentions string }{
		{"", "body"},
		{"not json", "body"},
		{`{"memoryGiB":0,"diskGB":20}`, "cpus"},
		{`{"cpus":0,"diskGB":20}`, "memoryGiB"},
		{`{"cpus":0,"memoryGiB":0}`, "diskGB"},
		{`{"cpus":"x","memoryGiB":0,"diskGB":20}`, "cpus"},
		{`{"cpus":1.5,"memoryGiB":0,"diskGB":20}`, "cpus"},
		{`{"cpus":17,"memoryGiB":0,"diskGB":20}`, "cpus"},
		{`{"cpus":-1,"memoryGiB":0,"diskGB":20}`, "cpus"},
		{`{"cpus":0,"memoryGiB":61,"diskGB":20}`, "memoryGiB"},
		{`{"cpus":0,"memoryGiB":0,"diskGB":19}`, "diskGB"},
		{`{"cpus":0,"memoryGiB":0,"diskGB":501}`, "diskGB"},
	} {
		w := doAs(t, n.h, n.admin, http.MethodPut, path, tc.body)
		if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), tc.mentions) {
			t.Errorf("PUT %q: got %d %q, want 400 naming %s", tc.body, w.Code, w.Body.String(), tc.mentions)
		}
	}
	if got := n.record(t); got.CPUs != 0 || got.MemoryGiB != 0 || got.DiskGB != 20 {
		t.Fatalf("a refused PUT changed the record: %+v", got)
	}
	if fr.stops+fr.ensures != 0 {
		t.Fatalf("a refused PUT touched the project: stops=%d ensures=%d", fr.stops, fr.ensures)
	}
}

func TestResourcesTinyHostRefusesAMemorySize(t *testing.T) {
	n, _, path := resHarness(t)
	n.s.size.(*fakeSize).host = orchestrator.Host{CPUs: 2, MemMiB: 2000}
	w := doAs(t, n.h, n.admin, http.MethodPut, path, `{"cpus":0,"memoryGiB":1,"diskGB":20}`)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "too small") {
		t.Fatalf("got %d %q", w.Code, w.Body.String())
	}
	w = doAs(t, n.h, n.admin, http.MethodGet, path, "")
	if got := decodeRes(t, w.Body.String()); got.Limits.MaxMemoryGiB != 0 {
		t.Fatalf("limits on a tiny host: %+v", got.Limits)
	}
}

func TestResourcesIsolationAndDeleting(t *testing.T) {
	n, _, path := resHarness(t)
	unknown := "/api/projects/00000000-0000-4000-8000-000000000000/resources"
	body := `{"cpus":0,"memoryGiB":0,"diskGB":20}`
	for _, c := range []struct{ email, method, path string }{
		{n.admin, http.MethodGet, unknown}, {n.admin, http.MethodPut, unknown},
		{n.member, http.MethodGet, path}, {n.member, http.MethodPut, path},
	} {
		b := ""
		if c.method == http.MethodPut {
			b = body
		}
		if w := doAs(t, n.h, c.email, c.method, c.path, b); w.Code != http.StatusNotFound {
			t.Errorf("%s %s as %s: %d, want 404", c.method, c.path, c.email, w.Code)
		}
	}
	n.s.deletingMu.Lock()
	n.s.deleting[identity.DeriveUsername(n.admin)+"/"+n.pid] = true
	n.s.deletingMu.Unlock()
	if w := doAs(t, n.h, n.admin, http.MethodPut, path, `{"cpus":2,"memoryGiB":0,"diskGB":20}`); w.Code != http.StatusConflict {
		t.Fatalf("PUT while deleting: %d, want 409", w.Code)
	}
}

func TestResourcesGet(t *testing.T) {
	n, fr, path := resHarness(t)
	fr.runningAt(1, 2, 20) // a project still running from before ADR-083
	w := doAs(t, n.h, n.admin, http.MethodGet, path, "")
	if w.Code != http.StatusOK {
		t.Fatalf("GET: %d %s", w.Code, w.Body.String())
	}
	got := decodeRes(t, w.Body.String())
	want := resourcesView{CPUs: 0, MemoryGiB: 0, DiskGB: 20, Pending: true,
		Limits: resourceLimits{MaxCPUs: 16, MaxMemoryGiB: 60, MinDiskGB: 20, MaxDiskGB: 500}}
	if got.resourcesView != want {
		t.Fatalf("GET = %+v, want %+v", got.resourcesView, want)
	}
	if fr.locks != 0 {
		t.Fatalf("GET took the project lock %d times; it must not wait behind a boot", fr.locks)
	}
	fr.runningUnlimited(20)
	if got := decodeRes(t, doAs(t, n.h, n.admin, http.MethodGet, path, "").Body.String()); got.Pending {
		t.Fatal("pending with the container at the record's size")
	}
	fr.live.Running = false
	fr.live.NanoCPUs = 1e9
	if got := decodeRes(t, doAs(t, n.h, n.admin, http.MethodGet, path, "").Body.String()); got.Pending {
		t.Fatal("pending for a stopped container (its next start applies the record)")
	}
	n.s.size.(*fakeSize).liveErr = errors.New("daemon gone")
	w = doAs(t, n.h, n.admin, http.MethodGet, path, "")
	if w.Code != http.StatusOK || decodeRes(t, w.Body.String()).Pending {
		t.Fatalf("an inspect error must cost only the hint: %d %s", w.Code, w.Body.String())
	}
	n.s.size.(*fakeSize).hostErr = errors.New("docker info failed")
	if w := doAs(t, n.h, n.admin, http.MethodGet, path, ""); w.Code != http.StatusServiceUnavailable {
		t.Fatalf("GET with no host size: %d, want 503", w.Code)
	}
}

// A record from before ADR-083 has diskGB 0: GET shows the container's size,
// and that is the floor a PUT may not go below.
func TestResourcesLegacyDiskIsTheContainers(t *testing.T) {
	n, fr, path := resHarness(t)
	n.setRecord(t, 0, 0, 0)
	fr.runningUnlimited(30)
	got := decodeRes(t, doAs(t, n.h, n.admin, http.MethodGet, path, "").Body.String())
	if got.DiskGB != 30 || got.Limits.MinDiskGB != 30 {
		t.Fatalf("legacy disk: %+v", got.resourcesView)
	}
	if w := doAs(t, n.h, n.admin, http.MethodPut, path, `{"cpus":0,"memoryGiB":0,"diskGB":25}`); w.Code != http.StatusBadRequest {
		t.Fatalf("shrinking below the container's disk: %d, want 400", w.Code)
	}
}

// Running at the requested size, record behind: persist only. A record-only
// difference never kills a session.
func TestResourcesPutRecordOnlyWhenTheContainerAlreadyMatches(t *testing.T) {
	n, fr, path := resHarness(t)
	n.setRecord(t, 4, 0, 20)
	fr.runningUnlimited(20)
	w := doAs(t, n.h, n.admin, http.MethodPut, path, `{"cpus":0,"memoryGiB":0,"diskGB":20}`)
	if w.Code != http.StatusOK || decodeRes(t, w.Body.String()).Rebuilt {
		t.Fatalf("got %d %s, want 200 rebuilt:false", w.Code, w.Body.String())
	}
	if fr.stops != 0 || fr.ensures != 0 {
		t.Fatalf("restarted a project already at the size: stops=%d ensures=%d", fr.stops, fr.ensures)
	}
	if got := n.record(t); got.CPUs != 0 {
		t.Fatalf("record not saved: %+v", got)
	}
}

// Running at another size with the record already equal to the request (the
// state every project is in after the upgrade): the PUT rebuilds it.
func TestResourcesPutRebuildsALiveMismatchEvenWithAnEqualRecord(t *testing.T) {
	n, fr, path := resHarness(t)
	fr.runningAt(1, 2, 20)
	w := doAs(t, n.h, n.admin, http.MethodPut, path, `{"cpus":0,"memoryGiB":0,"diskGB":20}`)
	if w.Code != http.StatusOK || !decodeRes(t, w.Body.String()).Rebuilt {
		t.Fatalf("got %d %s, want 200 rebuilt:true", w.Code, w.Body.String())
	}
	if fr.stops != 1 || fr.ensures != 1 {
		t.Fatalf("stops=%d ensures=%d, want 1 and 1", fr.stops, fr.ensures)
	}
}

func TestResourcesPutRunningChangeStopsPersistsStarts(t *testing.T) {
	n, fr, path := resHarness(t)
	fr.runningUnlimited(20)
	fr.onStop = func() {
		if got := n.record(t); got.CPUs != 0 {
			t.Errorf("record written before the stop finished: %+v", got)
		}
	}
	w := doAs(t, n.h, n.admin, http.MethodPut, path, `{"cpus":2,"memoryGiB":4,"diskGB":21}`)
	if w.Code != http.StatusOK || !decodeRes(t, w.Body.String()).Rebuilt {
		t.Fatalf("got %d %s", w.Code, w.Body.String())
	}
	want := orchestrator.Resources{CPUs: 2, MemoryGiB: 4, DiskGB: 21}
	if len(fr.ensuredFor) != 1 || fr.ensuredFor[0] != want {
		t.Fatalf("the restart read %+v through RunOpts.Resources, want %+v", fr.ensuredFor, want)
	}
	if got := n.record(t); got.CPUs != 2 || got.MemoryGiB != 4 || got.DiskGB != 21 {
		t.Fatalf("record: %+v", got)
	}
}

func TestResourcesPutStoppedRecordsOnly(t *testing.T) {
	n, fr, path := resHarness(t)
	fr.runningUnlimited(20)
	fr.live.Running = false
	w := doAs(t, n.h, n.admin, http.MethodPut, path, `{"cpus":2,"memoryGiB":0,"diskGB":20}`)
	if w.Code != http.StatusOK || decodeRes(t, w.Body.String()).Rebuilt {
		t.Fatalf("got %d %s, want 200 rebuilt:false", w.Code, w.Body.String())
	}
	if fr.stops != 0 || fr.ensures != 0 || n.record(t).CPUs != 2 {
		t.Fatalf("stops=%d ensures=%d record=%+v", fr.stops, fr.ensures, n.record(t))
	}
}

func TestResourcesPutFailures(t *testing.T) {
	t.Run("inspect fails: nothing changes", func(t *testing.T) {
		n, fr, path := resHarness(t)
		fr.runningUnlimited(20)
		n.s.size.(*fakeSize).liveErr = errors.New("daemon gone")
		w := doAs(t, n.h, n.admin, http.MethodPut, path, `{"cpus":2,"memoryGiB":0,"diskGB":20}`)
		if w.Code != http.StatusInternalServerError || !strings.Contains(w.Body.String(), "NOT changed") {
			t.Fatalf("got %d %s", w.Code, w.Body.String())
		}
		if n.record(t).CPUs != 0 || fr.stops != 0 {
			t.Fatalf("record=%+v stops=%d", n.record(t), fr.stops)
		}
	})
	t.Run("stop fails: still running at the old size", func(t *testing.T) {
		n, fr, path := resHarness(t)
		fr.runningUnlimited(20)
		fr.stopErr = errors.New("stuck")
		w := doAs(t, n.h, n.admin, http.MethodPut, path, `{"cpus":2,"memoryGiB":0,"diskGB":20}`)
		if w.Code != http.StatusInternalServerError || !strings.Contains(w.Body.String(), "old size") {
			t.Fatalf("got %d %s", w.Code, w.Body.String())
		}
		if n.record(t).CPUs != 0 || fr.ensures != 0 {
			t.Fatalf("record=%+v ensures=%d", n.record(t), fr.ensures)
		}
	})
	t.Run("deleted while stopping: 404", func(t *testing.T) {
		n, fr, path := resHarness(t)
		fr.runningUnlimited(20)
		fr.onStop = func() {
			sp, _ := n.s.space(identity.DeriveUsername(n.admin))
			_ = sp.projects.Remove(n.pid)
		}
		if w := doAs(t, n.h, n.admin, http.MethodPut, path, `{"cpus":2,"memoryGiB":0,"diskGB":20}`); w.Code != http.StatusNotFound {
			t.Fatalf("got %d %s, want 404", w.Code, w.Body.String())
		}
		if fr.ensures != 0 {
			t.Fatal("restarted a deleted project")
		}
	})
	t.Run("restart fails: saved, applies on the next start", func(t *testing.T) {
		n, fr, path := resHarness(t)
		fr.runningUnlimited(20)
		fr.ensureErr = errors.New("kata said no")
		w := doAs(t, n.h, n.admin, http.MethodPut, path, `{"cpus":2,"memoryGiB":0,"diskGB":20}`)
		if w.Code != http.StatusInternalServerError || !strings.Contains(w.Body.String(), "next start") {
			t.Fatalf("got %d %s", w.Code, w.Body.String())
		}
		if n.record(t).CPUs != 2 {
			t.Fatalf("record=%+v, want the new size saved", n.record(t))
		}
	})
	t.Run("no host size: 503, nothing changes", func(t *testing.T) {
		n, fr, path := resHarness(t)
		n.s.size.(*fakeSize).hostErr = errors.New("docker info failed")
		if w := doAs(t, n.h, n.admin, http.MethodPut, path, `{"cpus":2,"memoryGiB":0,"diskGB":20}`); w.Code != http.StatusServiceUnavailable {
			t.Fatalf("got %d", w.Code)
		}
		if n.record(t).CPUs != 0 || fr.locks != 0 {
			t.Fatalf("record=%+v locks=%d", n.record(t), fr.locks)
		}
	})
}

// The four start paths build RunOpts through runOptsFor, which carries the
// size: what a start reads is the record as of the read, not a snapshot.
func TestRunOptsForCarriesTheRecordsSize(t *testing.T) {
	n, _, _ := resHarness(t)
	sp, err := n.s.space(identity.DeriveUsername(n.admin))
	if err != nil {
		t.Fatal(err)
	}
	opts := n.s.runOptsFor(sp, identity.DeriveUsername(n.admin), n.record(t), "x")
	n.setRecord(t, 3, 5, 40)
	r, err := opts.Resources()
	if err != nil || r != (orchestrator.Resources{CPUs: 3, MemoryGiB: 5, DiskGB: 40}) {
		t.Fatalf("Resources() = %+v, %v", r, err)
	}
	if opts.Lan == nil {
		t.Fatal("runOptsFor dropped the Lan read")
	}
}
