package httpapi

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kesokaj/styrman-crew/internal/config"
	"github.com/kesokaj/styrman-crew/internal/orchestrator"
	"github.com/kesokaj/styrman-crew/internal/userstore"
)

// testRoutes builds the handler with real stores in a temp dir; the docker
// client is constructed but never dialed by the paths under test.
func testRoutes(t *testing.T) http.Handler {
	t.Helper()
	dir := t.TempDir()
	cfg := &config.Config{StateDir: dir, HomesDir: filepath.Join(dir, "homes"), Username: "tester"}
	logger := slog.New(slog.DiscardHandler)
	users, err := userstore.New(filepath.Join(dir, "users.json"), nil)
	if err != nil {
		t.Fatal(err)
	}
	orch, err := orchestrator.New(cfg, logger)
	if err != nil {
		t.Fatal(err)
	}
	return New(cfg, users, orch, logger).Routes()
}

func TestRouting(t *testing.T) {
	h := testRoutes(t)
	get := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		return w
	}

	// Unknown API/WS paths must be JSON 404s, never the SPA fallback.
	for _, path := range []string{"/api/typo", "/ws/term/only-project", "/api/projects/x/nope"} {
		w := get(path)
		if w.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", path, w.Code)
		}
		if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "json") && w.Code == http.StatusNotFound {
			// mux's own 404 is text; ours is JSON — both acceptable as long
			// as it is NOT the SPA's HTML.
			if strings.Contains(w.Body.String(), "<title>") {
				t.Errorf("GET %s returned the SPA instead of an error", path)
			}
		}
	}

	// SPA: root and client-side routes serve index.html…
	for _, path := range []string{"/", "/setup", "/p/foo/1"} {
		w := get(path)
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "<title>crew</title>") {
			t.Errorf("GET %s: code=%d, SPA index expected", path, w.Code)
		}
	}

	// …but embedded directories are not listable.
	if w := get("/assets/"); strings.Contains(w.Body.String(), "<pre>") {
		t.Error("GET /assets/ exposes a directory listing")
	}

	// Cross-origin mutation is rejected.
	req := httptest.NewRequest(http.MethodPost, "/api/projects", strings.NewReader("{}"))
	req.Header.Set("Origin", "https://evil.example")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("cross-origin POST = %d, want 403", w.Code)
	}

	// Unknown project 404s before any docker interaction.
	if w := get("/api/projects/nope"); w.Code != http.StatusNotFound {
		t.Errorf("GET unknown project = %d, want 404", w.Code)
	}
}

func TestAuthGuard(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{StateDir: dir, HomesDir: filepath.Join(dir, "homes"), Username: "tester"}
	users, err := userstore.New(filepath.Join(dir, "users.json"), []string{"me@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	orch, err := orchestrator.New(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	srv := New(cfg, users, orch, nil)
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
	h := srv.authGuard("header", inner)

	cases := []struct {
		name, email, path string
		want              int
	}{
		{"no header", "", "/api/projects", http.StatusUnauthorized},
		{"unauthorized email", "evil@example.com", "/api/projects", http.StatusForbidden},
		{"authorized email", "me@example.com", "/api/projects", http.StatusOK},
		{"case-insensitive", "ME@Example.COM", "/api/projects", http.StatusOK},
		{"health always open", "", "/api/health", http.StatusOK},
	}
	for _, c := range cases {
		req := httptest.NewRequest(http.MethodGet, c.path, nil)
		if c.email != "" {
			req.Header.Set(HeaderEmail, c.email)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != c.want {
			t.Errorf("%s: got %d, want %d", c.name, w.Code, c.want)
		}
	}

	// Mode "none" bypasses entirely (acts as the default user, admin).
	h = srv.authGuard("none", inner)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/projects", nil))
	if w.Code != http.StatusOK {
		t.Errorf("mode none: got %d, want 200", w.Code)
	}
}

func TestRedactURL(t *testing.T) {
	cases := map[string]string{
		"https://user:ghp_secret@github.com/o/r.git": "https://***@github.com/o/r.git",
		"https://github.com/o/r.git":                 "https://github.com/o/r.git",
		"git@github.com:o/r.git":                     "git@github.com:o/r.git",
		"ssh://git@host/o/r":                         "ssh://***@host/o/r",
	}
	for in, want := range cases {
		if got := redactURL(in); got != want {
			t.Errorf("redactURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCloneURLValidation(t *testing.T) {
	ok := []string{
		"https://github.com/o/r.git",
		"git@github.com:o/r.git",
		"ssh://git@host:22/o/r",
		"https://user:tok@github.com/o/r.git",
	}
	bad := []string{
		"", "--upload-pack=evil", "file:///etc/passwd", "http://insecure/o/r",
		"https://host/o/r; rm -rf /", "https://host/o/r$(whoami)",
	}
	for _, u := range ok {
		if !cloneURLRe.MatchString(u) {
			t.Errorf("clone URL %q should be accepted", u)
		}
	}
	for _, u := range bad {
		if cloneURLRe.MatchString(u) {
			t.Errorf("clone URL %q should be rejected", u)
		}
	}
}
