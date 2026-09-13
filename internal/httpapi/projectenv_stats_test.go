package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kesokaj/styrman-crew/internal/config"
	"github.com/kesokaj/styrman-crew/internal/credstore"
	"github.com/kesokaj/styrman-crew/internal/orchestrator"
	"github.com/kesokaj/styrman-crew/internal/userstore"
)

// Project-scoped env variables (ADR-043) and the stats snapshot had no
// handler tests at all (audit 2026-09-13: reachable routes at 0 % coverage).

// envTest is guestTest returning the Server too, so a credential store can
// be attached.
func envTest(t *testing.T) (*Server, http.Handler, string) {
	t.Helper()
	dir := t.TempDir()
	cfg := &config.Config{StateDir: dir, HomesDir: filepath.Join(dir, "homes"), Username: "tester", AuthMode: "none"}
	logger := slog.New(slog.DiscardHandler)
	users, err := userstore.New(filepath.Join(dir, "users.json"), nil)
	if err != nil {
		t.Fatal(err)
	}
	orch, err := orchestrator.New(cfg, logger)
	if err != nil {
		t.Fatal(err)
	}
	s := New(cfg, users, orch, logger)
	s.guest = &fakeGuest{}
	h := s.Routes()
	return s, h, makeProject(t, h, "env-test")
}

func TestProjectEnvRoutes(t *testing.T) {
	f := &fakeGuest{}
	h, pid := guestTest(t, f)
	base := "/api/projects/" + pid + "/env"
	// Broker disabled: the list says so, writes are 503.
	if w := do(t, h, http.MethodGet, base, ""); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"brokerEnabled":false`) {
		t.Fatalf("list without a broker = %d %s", w.Code, w.Body.String())
	}
	if w := do(t, h, http.MethodPut, base, `{"key":"FOO","value":"bar"}`); w.Code != http.StatusServiceUnavailable {
		t.Fatalf("put without a broker = %d", w.Code)
	}
	// With a store: put, list keys only, read one value, delete, 404 after.
	s, h, pid := envTest(t)
	base = "/api/projects/" + pid + "/env"
	cs, err := credstore.New(filepath.Join(t.TempDir(), "creds"))
	if err != nil {
		t.Fatal(err)
	}
	s.SetCredStore(cs)
	if w := do(t, h, http.MethodPut, base, `{"key":"FOO","value":"bar"}`); w.Code != http.StatusOK {
		t.Fatalf("put = %d %s", w.Code, w.Body.String())
	}
	if w := do(t, h, http.MethodPut, base, `{"key":"","value":"bar"}`); w.Code != http.StatusBadRequest {
		t.Fatalf("put without a key = %d", w.Code)
	}
	if w := do(t, h, http.MethodPut, base, `{"key":"bad key","value":"bar"}`); w.Code != http.StatusBadRequest {
		t.Fatalf("put with an invalid key = %d %s", w.Code, w.Body.String())
	}
	w := do(t, h, http.MethodGet, base, "")
	var list struct {
		BrokerEnabled bool     `json:"brokerEnabled"`
		Keys          []string `json:"keys"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil || !list.BrokerEnabled || len(list.Keys) != 1 || list.Keys[0] != "FOO" {
		t.Fatalf("list = %d %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "bar") {
		t.Fatal("the key list leaked a value")
	}
	if w := do(t, h, http.MethodGet, base+"/FOO", ""); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"value":"bar"`) {
		t.Fatalf("value = %d %s", w.Code, w.Body.String())
	}
	if w := do(t, h, http.MethodGet, base+"/NOPE", ""); w.Code != http.StatusNotFound {
		t.Fatalf("unknown value = %d", w.Code)
	}
	if w := do(t, h, http.MethodDelete, base+"/FOO", ""); w.Code != http.StatusNoContent {
		t.Fatalf("delete = %d %s", w.Code, w.Body.String())
	}
	if w := do(t, h, http.MethodDelete, base+"/FOO", ""); w.Code != http.StatusNotFound {
		t.Fatalf("delete again = %d", w.Code)
	}
	// Another tenant's project id is a 404, never a listing.
	if w := do(t, h, http.MethodGet, "/api/projects/00000000-0000-4000-8000-000000000000/env", ""); w.Code != http.StatusNotFound {
		t.Fatalf("foreign project = %d", w.Code)
	}
}

func TestProjectStatsRoute(t *testing.T) {
	f := &fakeGuest{reply: func(cmd []string) (int, string, error) {
		if len(cmd) == 3 && cmd[0] == "bash" && strings.Contains(cmd[2], "/proc/stat") {
			return 0, "cpu=12\ncpus=4\nload1=0.50\nmemtotal=2000\nmemused=500\ndisktotal=9000\ndiskused=1000\nprocs=42\nuptime=77\nhost=guest\n", nil
		}
		return 2, "usage", nil
	}}
	h, pid := guestTest(t, f)
	w := do(t, h, http.MethodGet, "/api/projects/"+pid+"/stats", "")
	if w.Code != http.StatusOK {
		t.Fatalf("stats = %d %s", w.Code, w.Body.String())
	}
	var v projectStatsView
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	if v.State != "running" || v.CPUPercent != 12 || v.CPUCount != 4 || v.Load1 != 0.5 || v.MemTotal != 2000 || v.MemUsed != 500 || v.DiskTotal != 9000 || v.DiskUsed != 1000 || v.Processes != 42 || v.Uptime != 77 || v.Host != "guest" {
		t.Fatalf("view = %+v", v)
	}
	// The sampler runs as the guest user in a NON-login shell (a login shell
	// would pull credentials on every poll).
	if len(f.calls) != 1 || f.calls[0][0] != "bash" || f.calls[0][1] != "-c" {
		t.Fatalf("calls = %v, want one bash -c exec", f.calls)
	}
	// A stopped project reports only its state, with no exec.
	f.state = "exited"
	before := len(f.calls)
	w = do(t, h, http.MethodGet, "/api/projects/"+pid+"/stats", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"state":"exited"`) || len(f.calls) != before {
		t.Fatalf("stopped stats = %d %s (execs %d)", w.Code, w.Body.String(), len(f.calls)-before)
	}
	// A failing sampler still answers 200 with the state and zeros.
	f.state = "running"
	f.reply = func([]string) (int, string, error) { return 1, "boom", nil }
	w = do(t, h, http.MethodGet, "/api/projects/"+pid+"/stats", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"state":"running"`) || !strings.Contains(w.Body.String(), `"cpuPercent":0`) {
		t.Fatalf("failed sampler = %d %s", w.Code, w.Body.String())
	}
}
