package httpapi

import (
	"net/http"
	"testing"
)

// deriveState's precedence: deleting > provisioning > the container's own
// state (TODO close-out 2026-09-10, item N).
func TestDeriveState(t *testing.T) {
	for _, c := range []struct {
		deleting, provisioning bool
		live, want             string
	}{
		{false, false, "absent", "absent"},
		{false, false, "running", "running"},
		{false, true, "absent", "provisioning"},
		{false, true, "running", "provisioning"},
		{true, false, "running", "deleting"},
		{true, true, "absent", "deleting"},
	} {
		if got := deriveState(c.deleting, c.provisioning, c.live); got != c.want {
			t.Errorf("deriveState(%v,%v,%q) = %q, want %q", c.deleting, c.provisioning, c.live, got, c.want)
		}
	}
}

// The views report "provisioning" while the entry is set — with no container
// at all, which is exactly the window — and the container's state once it
// is cleared. Set directly: the create path clears it on every return.
func TestProjectViewsReportProvisioning(t *testing.T) {
	n := newNetHarness(t)
	n.s.provisioning.Store(n.pid, struct{}{})
	one := decodeLan(t, doAs(t, n.h, n.admin, http.MethodGet, "/api/projects/"+n.pid, "").Body.String())
	if one["state"] != "provisioning" {
		t.Fatalf("GET state = %v, want provisioning", one["state"])
	}
	list := decodeLan(t, doAs(t, n.h, n.admin, http.MethodGet, "/api/projects", "").Body.String())
	ps, _ := list["projects"].([]any)
	if len(ps) != 1 || ps[0].(map[string]any)["state"] != "provisioning" {
		t.Fatalf("list state = %v, want provisioning", list)
	}
	n.s.provisioning.Delete(n.pid)
	one = decodeLan(t, doAs(t, n.h, n.admin, http.MethodGet, "/api/projects/"+n.pid, "").Body.String())
	if one["state"] != "absent" {
		t.Fatalf("after the window: state = %v, want absent", one["state"])
	}
}
