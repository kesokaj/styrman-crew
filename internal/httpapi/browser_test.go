package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/kesokaj/styrman-crew/internal/config"
	"github.com/kesokaj/styrman-crew/internal/orchestrator"
	"github.com/kesokaj/styrman-crew/internal/userstore"
)

// Browser view (ADR-068) handler tests over the guestRuntime seam: the REST
// verbs, the port refusals, and the CDP relay against an httptest stand-in
// for the guest's DevTools server. No Docker, no microVM.

// browserReply scripts the launcher: `status` reports what the test says,
// `start` succeeds or fails with a log tail, `stop` succeeds.
func browserReply(running bool, startExit int, startOut string) func(cmd []string) (int, string, error) {
	return func(cmd []string) (int, string, error) {
		if len(cmd) != 2 || cmd[0] != "/styrman/bin/styrman-browser" {
			return 127, "unexpected command", nil
		}
		switch cmd[1] {
		case "status":
			if running {
				return 0, `{"running":true,"version":"Chrome/152.0.0.0"}` + "\n", nil
			}
			return 0, `{"running":false}` + "\n", nil
		case "start":
			return startExit, startOut, nil
		case "stop":
			return 0, "", nil
		}
		return 2, "usage", nil
	}
}

// browserTest is guestTest plus a scripted upstream resolver for the relay.
func browserTest(t *testing.T, f *fakeGuest, upstream func(ctx context.Context, user, project string) (string, bool, error)) (*Server, http.Handler, string) {
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
	s.guest = f
	s.browserUpstreamFn = upstream
	h := s.Routes()
	return s, h, makeProject(t, h, "browser-test")
}

func attached(host string) func(ctx context.Context, user, project string) (string, bool, error) {
	return func(context.Context, string, string) (string, bool, error) { return host, false, nil }
}

func TestBrowserStatusRoute(t *testing.T) {
	f := &fakeGuest{reply: browserReply(true, 0, "")}
	_, h, pid := browserTest(t, f, attached("10.0.0.2:9222"))
	w := do(t, h, http.MethodGet, "/api/projects/"+pid+"/browser", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}
	var st browserStatus
	if err := json.Unmarshal(w.Body.Bytes(), &st); err != nil || !st.Running || st.Version != "Chrome/152.0.0.0" {
		t.Fatalf("status body = %s (err %v)", w.Body.String(), err)
	}
	// The exec ran the launcher as the guest user, with HOME set: the
	// launcher derives every path from it.
	if len(f.calls) != 1 || f.calls[0][1] != "status" {
		t.Fatalf("calls = %v, want one status exec", f.calls)
	}
	// Status is asked about THIS user's project, never another tenant's.
	if len(f.statusFor) == 0 || f.statusFor[0] != [2]string{"tester", pid} {
		t.Fatalf("statusFor = %v", f.statusFor)
	}
	// A stopped project is "not running", not an error.
	f.state = "exited"
	w = do(t, h, http.MethodGet, "/api/projects/"+pid+"/browser", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"running":false`) {
		t.Fatalf("stopped project: %d %s", w.Code, w.Body.String())
	}
	// Unknown project: 404, like every owner-only route.
	if w := do(t, h, http.MethodGet, "/api/projects/00000000-0000-4000-8000-000000000000/browser", ""); w.Code != http.StatusNotFound {
		t.Fatalf("unknown project = %d", w.Code)
	}
}

func TestBrowserStartStopRoutes(t *testing.T) {
	f := &fakeGuest{reply: browserReply(true, 0, "")}
	_, h, pid := browserTest(t, f, attached("10.0.0.2:9222"))
	w := do(t, h, http.MethodPost, "/api/projects/"+pid+"/browser/start", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"running":true`) {
		t.Fatalf("start = %d %s", w.Code, w.Body.String())
	}
	if w := do(t, h, http.MethodPost, "/api/projects/"+pid+"/browser/stop", ""); w.Code != http.StatusNoContent {
		t.Fatalf("stop = %d %s", w.Code, w.Body.String())
	}
	// Not running: 409 on both verbs, and no exec was attempted.
	before := len(f.calls)
	f.state = "exited"
	if w := do(t, h, http.MethodPost, "/api/projects/"+pid+"/browser/start", ""); w.Code != http.StatusConflict {
		t.Fatalf("start on stopped = %d %s", w.Code, w.Body.String())
	}
	if w := do(t, h, http.MethodPost, "/api/projects/"+pid+"/browser/stop", ""); w.Code != http.StatusConflict {
		t.Fatalf("stop on stopped = %d %s", w.Code, w.Body.String())
	}
	if len(f.calls) != before {
		t.Fatalf("exec attempted against a stopped project: %v", f.calls[before:])
	}
	// A launch failure surfaces the launcher's tail, as 502.
	f.state = "running"
	f.reply = browserReply(false, 1, "chromium did not answer on :9222 within 15 s; last log lines:\nNo usable sandbox!")
	w = do(t, h, http.MethodPost, "/api/projects/"+pid+"/browser/start", "")
	if w.Code != http.StatusBadGateway || !strings.Contains(w.Body.String(), "No usable sandbox") {
		t.Fatalf("failed start = %d %s", w.Code, w.Body.String())
	}
}

func TestBrowserStartFreshAttachIs503(t *testing.T) {
	f := &fakeGuest{reply: browserReply(true, 0, "")}
	fresh := func(context.Context, string, string) (string, bool, error) { return "10.0.0.2:9222", true, nil }
	_, h, pid := browserTest(t, f, fresh)
	w := do(t, h, http.MethodPost, "/api/projects/"+pid+"/browser/start", "")
	if w.Code != http.StatusServiceUnavailable || w.Header().Get("Retry-After") != "1" {
		t.Fatalf("fresh attach = %d, Retry-After %q", w.Code, w.Header().Get("Retry-After"))
	}
	if len(f.calls) != 0 {
		t.Fatalf("exec ran on a fresh attach: %v", f.calls)
	}
	// The relay handshake fails the same way, before any upgrade.
	w = do(t, h, http.MethodGet, "/ws/browser/"+pid, "")
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("relay on fresh attach = %d", w.Code)
	}
}

// The browser's ports never leave the owner's hands: not previewed, not
// shared, not linked — every one of those routes says so with a 400.
func TestBrowserPortRefusedOnPreviewSharesLinks(t *testing.T) {
	f := &fakeGuest{reply: browserReply(true, 0, "")}
	_, h, pid := browserTest(t, f, attached("10.0.0.2:9222"))
	for _, port := range []string{"9222", "9229"} {
		for _, c := range []struct{ method, path, body string }{
			{http.MethodGet, "/proxy/" + pid + "/" + port + "/", ""},
			{http.MethodGet, "/p/" + pid + "/t1/port/" + port + "/", ""},
			{http.MethodPut, "/api/projects/" + pid + "/ports/" + port + "/shares", `{"emails":[]}`},
			{http.MethodGet, "/api/projects/" + pid + "/ports/" + port + "/shares", ""},
			{http.MethodPost, "/api/projects/" + pid + "/ports/" + port + "/links", `{"ttl":"1h"}`},
		} {
			w := do(t, h, c.method, c.path, c.body)
			// The message names the port that was asked for (review nit:
			// 9229 used to be refused as "port 9222").
			if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "port "+port+" is the project's browser") {
				t.Errorf("%s %s = %d %s, want 400 browser refusal", c.method, c.path, w.Code, w.Body.String())
			}
		}
	}
	// An ordinary port is not caught by the refusal.
	if w := do(t, h, http.MethodGet, "/api/projects/"+pid+"/ports/3000/shares", ""); w.Code == http.StatusBadRequest {
		t.Fatalf("port 3000 refused: %s", w.Body.String())
	}
}

// fakeDevTools stands in for the guest's DevTools server: /json/version
// announces a browser endpoint (with a host the relay must NOT trust), and
// the endpoint pushes one event, echoes every message, and closes on "bye".
func fakeDevTools(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/json/version", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"Browser":"Chrome/152.0.0.0","webSocketDebuggerUrl":"ws://127.0.0.1:1/devtools/browser/abc"}`))
	})
	mux.HandleFunc("/devtools/browser/abc", func(w http.ResponseWriter, r *http.Request) {
		// Chromium's DevTools server admits an upgrade with NO Origin and
		// refuses every Origin it sees (the launcher sets no
		// --remote-allow-origins). The relay must therefore never send one —
		// this fake refuses exactly as Chromium does, so a relay that started
		// sending an Origin fails this test rather than production.
		if r.Header.Get("Origin") != "" {
			http.Error(w, "origin not allowed", http.StatusForbidden)
			return
		}
		c, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			return
		}
		ctx := r.Context()
		_ = c.Write(ctx, websocket.MessageText, []byte(`{"method":"Target.targetCreated","params":{}}`))
		for {
			_, data, err := c.Read(ctx)
			if err != nil {
				return
			}
			if string(data) == `{"id":99,"method":"bye"}` {
				c.Close(websocket.StatusNormalClosure, "chromium gone")
				return
			}
			_ = c.Write(ctx, websocket.MessageText, append([]byte(`{"echo":`), append(data, '}')...))
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func dialRelay(t *testing.T, h http.Handler, pid string) (*websocket.Conn, *httptest.Server) {
	t.Helper()
	front := httptest.NewServer(h)
	t.Cleanup(front.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	c, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(front.URL, "http")+"/ws/browser/"+pid, nil)
	if err != nil {
		t.Fatalf("dial relay: %v", err)
	}
	return c, front
}

func TestBrowserRelayPumpsBothWays(t *testing.T) {
	dev := fakeDevTools(t)
	f := &fakeGuest{reply: browserReply(true, 0, "")}
	_, h, pid := browserTest(t, f, attached(strings.TrimPrefix(dev.URL, "http://")))
	c, _ := dialRelay(t, h, pid)
	defer c.Close(websocket.StatusNormalClosure, "")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// The upstream's unsolicited event reaches the pane untouched.
	_, ev, err := c.Read(ctx)
	if err != nil || !strings.Contains(string(ev), "Target.targetCreated") {
		t.Fatalf("first frame = %q, %v", ev, err)
	}
	// The relay started the browser before dialing (start, then status).
	if got := f.matching("start"); len(got) != 1 {
		t.Fatalf("start execs = %v", got)
	}
	if err := c.Write(ctx, websocket.MessageText, []byte(`{"id":1,"method":"Target.getTargets"}`)); err != nil {
		t.Fatal(err)
	}
	_, reply, err := c.Read(ctx)
	if err != nil || string(reply) != `{"echo":{"id":1,"method":"Target.getTargets"}}` {
		t.Fatalf("echo = %q, %v", reply, err)
	}
	// The guest side going away closes the pane with 1011 and a reason the
	// pane can show — not a silent EOF.
	if err := c.Write(ctx, websocket.MessageText, []byte(`{"id":99,"method":"bye"}`)); err != nil {
		t.Fatal(err)
	}
	_, _, err = c.Read(ctx)
	if websocket.CloseStatus(err) != websocket.StatusInternalError {
		t.Fatalf("after upstream close: err = %v, want close 1011", err)
	}
}

func TestBrowserRelayCloseCodes(t *testing.T) {
	// Project not running (no guest IP): accepted, then 4002.
	f := &fakeGuest{reply: browserReply(true, 0, ""), state: "exited"}
	_, h, pid := browserTest(t, f, attached(""))
	c, _ := dialRelay(t, h, pid)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, _, err := c.Read(ctx); websocket.CloseStatus(err) != websocket.StatusCode(config.CloseProjectStopped) {
		t.Fatalf("stopped project: err = %v, want close %d", err, config.CloseProjectStopped)
	}
	// Launch failure: accepted, then 4004 carrying the launcher's words.
	f2 := &fakeGuest{reply: browserReply(false, 1, "No usable sandbox!")}
	_, h2, pid2 := browserTest(t, f2, attached("10.0.0.2:9222"))
	c2, _ := dialRelay(t, h2, pid2)
	_, _, err := c2.Read(ctx)
	var ce websocket.CloseError
	if !asCloseError(err, &ce) || ce.Code != websocket.StatusCode(config.CloseBrowserUnavailable) || !strings.Contains(ce.Reason, "No usable sandbox") {
		t.Fatalf("failed launch: err = %v, want close %d with the tail", err, config.CloseBrowserUnavailable)
	}
}

func asCloseError(err error, ce *websocket.CloseError) bool {
	if err == nil {
		return false
	}
	var c websocket.CloseError
	if errorsAs(err, &c) {
		*ce = c
		return true
	}
	return false
}

func TestCloseReasonFitsAndKeepsRunes(t *testing.T) {
	long := strings.Repeat("é", 100) // 200 bytes
	r := closeReason("browser did not start: " + long)
	if len(r) > 120 {
		t.Fatalf("reason %d bytes", len(r))
	}
	if !strings.HasPrefix(r, "browser did not start: ") || strings.HasSuffix(r, "\xc3") {
		t.Fatalf("reason cut mid-rune or lost its head: %q", r)
	}
	if tail("abc", 5) != "abc" || tail(strings.Repeat("x", 10)+"END", 3) != "…END" {
		t.Fatal("tail keeps the wrong end")
	}
}

func errorsAs(err error, target *websocket.CloseError) bool {
	for err != nil {
		if ce, ok := err.(websocket.CloseError); ok {
			*target = ce
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}

// A link or share record for the browser's port that predates the mint's
// refusal must not serve: the public link route is authGuard-exempt, so it
// would hand out no-login DevTools control (security review, round 2).
func TestBrowserPortLinkAndShareRecordsDoNotServe(t *testing.T) {
	f := &fakeGuest{reply: browserReply(true, 0, "")}
	s, h, pid := browserTest(t, f, attached("10.0.0.2:9222"))
	for _, port := range []int{config.BrowserPort, config.BrowserChromePort} {
		l, err := s.links.Create(pid, "tester", port, "tester@example.com", time.Hour, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if w := do(t, h, http.MethodGet, "/link/"+l.Token+"/", ""); w.Code != http.StatusNotFound {
			t.Fatalf("stale public link for %d served: %d %s", port, w.Code, w.Body.String())
		}
		sp, err := s.space("tester")
		if err != nil {
			t.Fatal(err)
		}
		if err := sp.projects.SetPortShares(pid, port, []string{"guest@example.com"}); err != nil {
			t.Fatal(err)
		}
		pl, err := s.links.EnsurePrivate(pid, "tester", port, "tester@example.com", time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if w := do(t, h, http.MethodGet, "/shared/"+pl.Token+"/", ""); w.Code != http.StatusNotFound {
			t.Fatalf("stale private share for %d served: %d %s", port, w.Code, w.Body.String())
		}
	}
	// A normal port's records still serve (well, reach the proxy: 502 from a
	// guest that is not there, not 404).
	l, _ := s.links.Create(pid, "tester", 3000, "tester@example.com", time.Hour, time.Now())
	if w := do(t, h, http.MethodGet, "/link/"+l.Token+"/", ""); w.Code == http.StatusNotFound {
		t.Fatalf("a normal port's link 404s: %s", w.Body.String())
	}
}

// The guest chooses what answers on its port; a redirect there must not steer
// this server (which sits on every project bridge) anywhere else.
func TestBrowserRelayFollowsNoRedirect(t *testing.T) {
	victim := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("the relay followed a redirect to %s", r.URL)
	}))
	defer victim.Close()
	mux := http.NewServeMux()
	mux.HandleFunc("/json/version", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, victim.URL+"/json/version", http.StatusFound)
	})
	dev := httptest.NewServer(mux)
	defer dev.Close()
	f := &fakeGuest{reply: browserReply(true, 0, "")}
	_, h, pid := browserTest(t, f, attached(strings.TrimPrefix(dev.URL, "http://")))
	c, _ := dialRelay(t, h, pid)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, _, err := c.Read(ctx); websocket.CloseStatus(err) != websocket.StatusCode(config.CloseBrowserUnavailable) {
		t.Fatalf("redirected discovery: err = %v, want close %d", err, config.CloseBrowserUnavailable)
	}
	// Same for the WebSocket dial itself.
	mux2 := http.NewServeMux()
	mux2.HandleFunc("/json/version", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"webSocketDebuggerUrl":"ws://x/devtools/browser/abc"}`))
	})
	mux2.HandleFunc("/devtools/browser/abc", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, victim.URL+"/devtools/browser/abc", http.StatusMovedPermanently)
	})
	dev2 := httptest.NewServer(mux2)
	defer dev2.Close()
	_, h2, pid2 := browserTest(t, &fakeGuest{reply: browserReply(true, 0, "")}, attached(strings.TrimPrefix(dev2.URL, "http://")))
	c2, _ := dialRelay(t, h2, pid2)
	if _, _, err := c2.Read(ctx); websocket.CloseStatus(err) != websocket.StatusCode(config.CloseBrowserUnavailable) {
		t.Fatalf("redirected dial: err = %v, want close %d", err, config.CloseBrowserUnavailable)
	}
}

// Origin is checked before any side effect, and the relay count is capped.
func TestBrowserRelayOriginAndCap(t *testing.T) {
	f := &fakeGuest{reply: browserReply(true, 0, "")}
	s, h, pid := browserTest(t, f, attached("10.0.0.2:9222"))
	r := httptest.NewRequest(http.MethodGet, "/ws/browser/"+pid, nil)
	r.Header.Set("Origin", "http://evil.example")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden || len(f.calls) != 0 {
		t.Fatalf("cross-origin: %d, execs %v", w.Code, f.calls)
	}
	for i := 0; i < browserRelayPerUser; i++ {
		if ok, _ := s.browserRelays.acquire("tester"); !ok {
			t.Fatalf("slot %d refused below the per-user cap", i)
		}
	}
	if w := do(t, h, http.MethodGet, "/ws/browser/"+pid, ""); w.Code != http.StatusServiceUnavailable || len(f.calls) != 0 {
		t.Fatalf("over the per-user cap: %d, execs %v", w.Code, f.calls)
	}
	// Another user is not locked out by this one's panes.
	if ok, why := s.browserRelays.acquire("someone-else"); !ok {
		t.Fatalf("another user refused by the %s cap", why)
	}
	s.browserRelays.release("someone-else")
	for i := 0; i < browserRelayPerUser; i++ {
		s.browserRelays.release("tester")
	}
	if ok, _ := s.browserRelays.acquire("tester"); !ok {
		t.Fatal("slot not freed after release")
	}
	s.browserRelays.release("tester")
}

// The launcher learns the server's own bridge address on every start and
// nothing else — the forwarder admits exactly that.
func TestBrowserStartPassesPeer(t *testing.T) {
	var env []string
	f := &fakeGuest{}
	f.reply = browserReply(true, 0, "")
	s, h, pid := browserTest(t, f, attached("10.0.0.2:9222"))
	s.browserPeerFn = func(context.Context, string) string { return "172.16.0.254" }
	s.guest = &envRecordingGuest{fakeGuest: f, envs: &env}
	if w := do(t, h, http.MethodPost, "/api/projects/"+pid+"/browser/start", ""); w.Code != http.StatusOK {
		t.Fatalf("start = %d %s", w.Code, w.Body.String())
	}
	if !hasWord(env, "STYRMAN_BROWSER_PEER=172.16.0.254") {
		t.Fatalf("start exec env = %v, want the peer", env)
	}
	// An unknown peer is passed as an EMPTY value, never omitted: the
	// launcher reads "set but empty" as "no forwarder" and only an unset
	// variable falls back to the broker's host.
	env = nil
	s.browserPeerFn = func(context.Context, string) string { return "" }
	if w := do(t, h, http.MethodPost, "/api/projects/"+pid+"/browser/start", ""); w.Code != http.StatusOK {
		t.Fatalf("start = %d %s", w.Code, w.Body.String())
	}
	if !hasWord(env, "STYRMAN_BROWSER_PEER=") {
		t.Fatalf("start exec env = %v, want an explicit empty peer", env)
	}
}

type envRecordingGuest struct {
	*fakeGuest
	envs *[]string
}

func (g *envRecordingGuest) Exec(ctx context.Context, id string, spec orchestrator.ExecSpec) (int, string, error) {
	if len(spec.Cmd) == 2 && spec.Cmd[1] == "start" {
		*g.envs = append(*g.envs, spec.Env...)
	}
	return g.fakeGuest.Exec(ctx, id, spec)
}
