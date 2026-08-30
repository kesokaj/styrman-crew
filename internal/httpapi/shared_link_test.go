package httpapi

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kesokaj/styrman-crew/internal/config"
	"github.com/kesokaj/styrman-crew/internal/identity"
	"github.com/kesokaj/styrman-crew/internal/orchestrator"
	"github.com/kesokaj/styrman-crew/internal/project"
	"github.com/kesokaj/styrman-crew/internal/userstore"
)

// sharedHarness builds a header-auth server with one owner-owned project so
// requests can impersonate identities via X-Caddy-Email.
func sharedHarness(t *testing.T) (h http.Handler, projectID string) {
	t.Helper()
	dir := t.TempDir()
	cfg := &config.Config{
		StateDir: dir,
		HomesDir: filepath.Join(dir, "homes"),
		Username: "tester",
		AuthMode: "header",
		// Exercise the S7 ingress-origin defense on /shared too: every doAs
		// request carries the token; the no-token case is asserted explicitly.
		IngressToken: "test-ingress-token",
		// Minting a public link in header mode requires the public wildcard
		// host (v1.14.0 fail-loud) — give the fixture one.
		PreviewBase: "public.test",
	}
	logger := slog.New(slog.DiscardHandler)
	users, err := userstore.New(filepath.Join(dir, "users.json"), []string{"owner@gmail.com"})
	if err != nil {
		t.Fatal(err)
	}
	orch, err := orchestrator.New(cfg, logger)
	if err != nil {
		t.Fatal(err)
	}
	s := New(cfg, users, orch, logger)
	owner := identity.DeriveUsername("owner@gmail.com")
	sp, err := s.space(owner)
	if err != nil {
		t.Fatal(err)
	}
	p, err := sp.projects.Add(project.Project{Name: "demo"})
	if err != nil {
		t.Fatal(err)
	}
	return s.Routes(), p.ID
}

func doAs(t *testing.T, h http.Handler, email, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	if email != "" {
		r.Header.Set(HeaderEmail, email)
	}
	r.Header.Set(HeaderIngress, "test-ingress-token")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

// TestSharedLinkLifecycle covers the private share link end to end at the
// handler level: minted by sharing, stable across edits, kind-locked to its
// route, access-checked per email, and dead once the share list empties.
func TestSharedLinkLifecycle(t *testing.T) {
	h, pid := sharedHarness(t)
	owner := "owner@gmail.com"
	sharesPath := "/api/projects/" + pid + "/ports/3000/shares"

	// Sharing a port mints a stable private link and returns its URL.
	w := doAs(t, h, owner, "PUT", sharesPath, `{"emails":["friend@gmail.com"]}`)
	if w.Code != 200 {
		t.Fatalf("set shares: %d %s", w.Code, w.Body.String())
	}
	url := jsonField(t, w.Body.Bytes(), "url")
	if !strings.HasPrefix(url, "/shared/") || !strings.HasSuffix(url, "/") {
		t.Fatalf("share url = %q, want path form /shared/<token>/", url)
	}
	token := strings.TrimSuffix(strings.TrimPrefix(url, "/shared/"), "/")

	// Editing the list must NOT rotate the URL.
	w = doAs(t, h, owner, "PUT", sharesPath, `{"emails":["friend@gmail.com","two@gmail.com"]}`)
	if got := jsonField(t, w.Body.Bytes(), "url"); got != url {
		t.Fatalf("share url rotated on edit: %q → %q", url, got)
	}
	// GET returns it too.
	w = doAs(t, h, owner, "GET", sharesPath, "")
	if got := jsonField(t, w.Body.Bytes(), "url"); got != url {
		t.Fatalf("GET shares url = %q, want %q", got, url)
	}

	// A shared guest reaches the route: past auth + share check, the stubbed
	// proxy fails upstream with exactly 502 — anything else means the handler
	// short-circuited differently than expected.
	for _, email := range []string{owner, "friend@gmail.com"} {
		w = doAs(t, h, email, "GET", url, "")
		if w.Code != 502 {
			t.Errorf("%s on %s: %d, want 502 (access granted, stub upstream)", email, url, w.Code)
		}
	}
	// S7: a request that bypasses the ingress (no X-Styrman-Ingress) is
	// refused even with a valid identity header + shared email.
	{
		r := httptest.NewRequest("GET", url, nil)
		r.Header.Set(HeaderEmail, "friend@gmail.com")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		if rec.Code != 403 {
			t.Errorf("no ingress token on %s: %d, want 403", url, rec.Code)
		}
	}
	// An unshared Google account is refused; anonymous is refused by authGuard.
	if w = doAs(t, h, "stranger@gmail.com", "GET", url, ""); w.Code != 403 {
		t.Errorf("stranger: %d, want 403", w.Code)
	}
	if w = doAs(t, h, "", "GET", url, ""); w.Code != 401 {
		t.Errorf("anonymous: %d, want 401", w.Code)
	}

	// KIND LOCK: the private token must be dead on the no-login /link route —
	// otherwise sharing would mint a public capability.
	if w = doAs(t, h, "", "GET", "/link/"+token+"/", ""); w.Code != 404 {
		t.Errorf("private token on /link: %d, want 404", w.Code)
	}

	// The owner cannot revoke it via the public-links API (it belongs to the
	// shares lifecycle), and it never shows in the public list.
	if w = doAs(t, h, owner, "DELETE", "/api/projects/"+pid+"/links/"+token, ""); w.Code != 404 {
		t.Errorf("DELETE private via links API: %d, want 404", w.Code)
	}
	w = doAs(t, h, owner, "GET", "/api/projects/"+pid+"/links", "")
	if strings.Contains(w.Body.String(), token) {
		t.Errorf("private token leaked into public links list")
	}

	// Emptying the share list kills the link entirely.
	doAs(t, h, owner, "PUT", sharesPath, `{"emails":[]}`)
	if w = doAs(t, h, "friend@gmail.com", "GET", url, ""); w.Code != 404 {
		t.Errorf("after unshare: %d, want 404", w.Code)
	}
}

// TestPublicTokenOnSharedRoute locks the other direction of the kind check: a
// public TTL token must not be honored on the email-gated route.
func TestPublicTokenOnSharedRoute(t *testing.T) {
	h, pid := sharedHarness(t)
	owner := "owner@gmail.com"
	w := doAs(t, h, owner, "POST", "/api/projects/"+pid+"/ports/3000/links", `{"ttl":"1h"}`)
	if w.Code != 200 {
		t.Fatalf("create public link: %d %s", w.Code, w.Body.String())
	}
	token := jsonField(t, w.Body.Bytes(), "token")
	if w = doAs(t, h, owner, "GET", "/shared/"+token+"/", ""); w.Code != 404 {
		t.Errorf("public token on /shared: %d, want 404", w.Code)
	}
}

// jsonField pulls a top-level string field out of a JSON body.
func jsonField(t *testing.T, body []byte, field string) string {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("bad JSON %s: %v", body, err)
	}
	v, _ := m[field].(string)
	return v
}

// TestProxySandboxHeader: the shared proxy applies the CSP sandbox exactly
// when asked — the header the main-origin path forms rely on (N22).
func TestProxySandboxHeader(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer backend.Close()
	hostPort := strings.TrimPrefix(backend.URL, "http://")
	host, portStr, _ := strings.Cut(hostPort, ":")
	var port int
	if _, err := fmt.Sscanf(portStr, "%d", &port); err != nil {
		t.Fatal(err)
	}
	s := &Server{}
	for _, sandbox := range []bool{true, false} {
		rec := httptest.NewRecorder()
		s.proxyFor(host, port, "", "", sandbox).ServeHTTP(rec, httptest.NewRequest("GET", "http://x/", nil))
		csp := rec.Header().Get("Content-Security-Policy")
		if sandbox && !strings.Contains(csp, "sandbox") {
			t.Errorf("sandbox=true: CSP missing (got %q)", csp)
		}
		if !sandbox && csp != "" {
			t.Errorf("sandbox=false: unexpected CSP %q", csp)
		}
	}
}
