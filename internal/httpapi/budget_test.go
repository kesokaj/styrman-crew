package httpapi

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/kesokaj/styrman-crew/internal/identity"
	"github.com/kesokaj/styrman-crew/internal/membudget"
)

// A request that does not fit the memory budget waits, then gets 503 with
// Retry-After — while another tenant, inside its own share, is served.
func TestBudgetFullAnswers503ButAnotherTenantIsServed(t *testing.T) {
	defer func(d time.Duration) { budgetWait = d }(budgetWait)
	budgetWait = 100 * time.Millisecond
	n := newNetHarness(t)
	n.s.budget = membudget.New(8*mib, 4*mib)
	hold, err := n.s.budget.Acquire(context.Background(), identity.DeriveUsername(n.admin), 4*mib)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	w := doAs(t, n.h, n.admin, http.MethodGet, "/api/projects", "")
	if w.Code != http.StatusServiceUnavailable || w.Header().Get("Retry-After") != "1" {
		t.Fatalf("tenant at its share: got %d Retry-After=%q, want 503 and 1", w.Code, w.Header().Get("Retry-After"))
	}
	if waited := time.Since(start); waited < budgetWait {
		t.Fatalf("503 after %v, before the %v wait", waited, budgetWait)
	}
	if !strings.Contains(w.Body.String(), "server busy") {
		t.Fatalf("body %q", w.Body.String())
	}
	if w := doAs(t, n.h, n.member, http.MethodGet, "/api/projects", ""); w.Code != http.StatusOK {
		t.Fatalf("another tenant: got %d, want 200", w.Code)
	}
	hold()
	if w := doAs(t, n.h, n.admin, http.MethodGet, "/api/projects", ""); w.Code != http.StatusOK {
		t.Fatalf("after release: got %d, want 200", w.Code)
	}
}

// Every pattern the weight table prices is a registered route, matched as
// itself: a renamed route would otherwise silently fall to the default
// weight.
func TestEveryWeightedPatternIsARoute(t *testing.T) {
	n := newNetHarness(t)
	mux := n.s.newMux()
	wild := regexp.MustCompile(`\{[a-z]+\}`)
	for pattern := range routeWeight {
		method, p, _ := strings.Cut(pattern, " ")
		r := httptest.NewRequest(method, wild.ReplaceAllString(p, "x"), nil)
		if _, got := mux.Handler(r); got != pattern {
			t.Errorf("%s routes to %q", pattern, got)
		}
	}
	for _, ws := range []string{"GET /ws/term/{project}/{term}", "GET /ws/setup", "GET /ws/browser/{project}"} {
		if f := routeWeight[ws]; f == nil || f(httptest.NewRequest("GET", "/", nil)) != 0 {
			t.Errorf("%s must be uncharged", ws)
		}
	}
}

// A forged Content-Length cannot buy more than the body that can arrive,
// and a GET is priced with no body.
func TestBodyPricingIsCapped(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/api/projects/x/git/commit", strings.NewReader("{}"))
	r.ContentLength = 1 << 40
	if got := body(r); got != 1<<20 {
		t.Fatalf("forged length priced at %d, want the 1 MiB cap", got)
	}
	if got := body(httptest.NewRequest(http.MethodGet, "/api/projects/x/file", nil)); got != 0 {
		t.Fatalf("GET priced at %d", got)
	}
	if hasBody(httptest.NewRequest(http.MethodGet, "/api/projects/x/file", nil)) {
		t.Fatal("a GET must carry no read deadline")
	}
}

// One tenant holds at most half of a pool; another tenant still gets a slot.
func TestATenantHoldsAtMostHalfAPool(t *testing.T) {
	n := newNetHarness(t)
	as := func(user string) (*httptest.ResponseRecorder, *http.Request) {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		return httptest.NewRecorder(), identity.WithUser(r, identity.User{Username: user})
	}
	var rels []func()
	for i := range fileConcurrency / 2 {
		w, r := as("alice")
		rel := n.s.acquireSlot(w, r, n.s.fileSlots, "full")
		if rel == nil {
			t.Fatalf("slot %d refused: %s", i, w.Body.String())
		}
		rels = append(rels, rel)
	}
	w, r := as("alice")
	if rel := n.s.acquireSlot(w, r, n.s.fileSlots, "full"); rel != nil || w.Code != http.StatusTooManyRequests ||
		!strings.Contains(w.Body.String(), fmt.Sprintf("you already have %d", fileConcurrency/2)) {
		t.Fatalf("over the share: code %d body %q", w.Code, w.Body.String())
	}
	w, r = as("bob")
	rel := n.s.acquireSlot(w, r, n.s.fileSlots, "full")
	if rel == nil {
		t.Fatalf("another tenant refused: %s", w.Body.String())
	}
	rels = append(rels, rel)
	for _, rel := range rels {
		rel()
		rel() // idempotent
	}
	if len(n.s.fileSlots.ch) != 0 || len(n.s.fileSlots.held) != 0 {
		t.Fatalf("after release: %d slots, held %v", len(n.s.fileSlots.ch), n.s.fileSlots.held)
	}
}

// A slot holder whose body stops arriving gets 408 at the read deadline,
// not 400. A real server: the deadline lives on the connection.
func TestAStalledBodyGets408(t *testing.T) {
	defer func(d time.Duration) { slotReadTimeout = d }(slotReadTimeout)
	slotReadTimeout = 500 * time.Millisecond
	n := newNetHarness(t)
	srv := httptest.NewServer(n.h)
	defer srv.Close()
	conn, err := net.Dial("tcp", strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	fmt.Fprintf(conn, "PUT /api/projects/%s/file?path=a.txt HTTP/1.1\r\nHost: x\r\nContent-Type: application/json\r\nContent-Length: 100\r\n%s: %s\r\n%s: test-ingress-token\r\n\r\n{\"content\":",
		n.pid, HeaderEmail, n.admin, HeaderIngress)
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusRequestTimeout {
		t.Fatalf("got %d %s, want 408", resp.StatusCode, b)
	}
}

// Once the body is read, the read deadline must not outlive it: net/http's
// background read would hit it and cancel the request's context mid-handler
// (plan D21).
func TestTheBodyDeadlineEndsWithTheBody(t *testing.T) {
	defer func(d time.Duration) { slotReadTimeout = d }(slotReadTimeout)
	slotReadTimeout = 300 * time.Millisecond
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyDeadline(w, r)
		if _, err := io.ReadAll(r.Body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		time.Sleep(3 * slotReadTimeout)
		if err := r.Context().Err(); err != nil {
			http.Error(w, "context cancelled: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	resp, err := http.Post(srv.URL, "application/json", strings.NewReader(`{"a":1}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("got %d %s", resp.StatusCode, b)
	}
}

// Tunnels into a guest app (preview, link, shared, proxy) are never charged:
// a full budget must not 503 them.
func TestTunnelsAreNotCharged(t *testing.T) {
	defer func(d time.Duration) { budgetWait = d }(budgetWait)
	budgetWait = 50 * time.Millisecond
	n := newNetHarness(t)
	n.s.budget = membudget.New(1, 1)
	hold, err := n.s.budget.Acquire(context.Background(), "-", 1)
	if err != nil {
		t.Fatal(err)
	}
	defer hold()
	if w := doAs(t, n.h, "", http.MethodGet, "/link/nosuchtoken/", ""); w.Code == http.StatusServiceUnavailable {
		t.Fatalf("a link tunnel was charged: %d %s", w.Code, w.Body.String())
	}
	// Positive control: an API request of the same tenant is charged.
	if w := doAs(t, n.h, n.admin, http.MethodGet, "/api/version", ""); w.Code == http.StatusServiceUnavailable {
		t.Fatalf("/api/version is uncharged but got 503")
	}
}

// slowConn sends a request's head and the first part of its body, waits
// past the body deadline, then sends the rest, and returns the response.
func slowRequest(t *testing.T, srvURL, head string, body []byte, pause time.Duration) *http.Response {
	t.Helper()
	conn, err := net.Dial("tcp", strings.TrimPrefix(srvURL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	half := len(body) / 2
	fmt.Fprint(conn, head)
	_, _ = conn.Write(body[:half])
	time.Sleep(pause)
	_, _ = conn.Write(body[half:])
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

// The budget gives every charged body the read deadline — not only the
// slot holders — and exempts uploads, which may legitimately take longer.
func TestBodyDeadlineCoversBudgetedRoutesButNotUploads(t *testing.T) {
	defer func(d time.Duration) { slotReadTimeout = d }(slotReadTimeout)
	slotReadTimeout = 300 * time.Millisecond
	n := newNetHarness(t)
	srv := httptest.NewServer(n.h)
	defer srv.Close()
	auth := fmt.Sprintf("%s: %s\r\n%s: test-ingress-token\r\n", HeaderEmail, n.admin, HeaderIngress)

	// A small JSON route with no slot: the stalled body is cut at the
	// deadline (400 from the decode), not left to hang.
	body := []byte(`{"lanAccess":false}`)
	head := fmt.Sprintf("PUT /api/projects/%s/network HTTP/1.1\r\nHost: x\r\nContent-Type: application/json\r\nContent-Length: %d\r\n%s\r\n", n.pid, len(body), auth)
	if resp := slowRequest(t, srv.URL, head, body, time.Second); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("stalled network body: %d, want 400 at the deadline", resp.StatusCode)
	}

	// An upload slower than the deadline still lands.
	ws := n.s.cfg.ProjectWorkspace(identity.DeriveUsername(n.admin), n.pid)
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	var mp strings.Builder
	mp.WriteString("--BOUND\r\nContent-Disposition: form-data; name=\"file\"; filename=\"slow.txt\"\r\nContent-Type: text/plain\r\n\r\n")
	mp.WriteString(strings.Repeat("u", 4096))
	mp.WriteString("\r\n--BOUND--\r\n")
	up := []byte(mp.String())
	head = fmt.Sprintf("POST /api/projects/%s/upload?path= HTTP/1.1\r\nHost: x\r\nContent-Type: multipart/form-data; boundary=BOUND\r\nContent-Length: %d\r\n%s\r\n", n.pid, len(up), auth)
	resp := slowRequest(t, srv.URL, head, up, time.Second)
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("slow upload: %d %s", resp.StatusCode, b)
	}
	if got, err := os.ReadFile(filepath.Join(ws, "slow.txt")); err != nil || len(got) != 4096 {
		t.Fatalf("upload not saved whole: %d bytes, %v", len(got), err)
	}
}
