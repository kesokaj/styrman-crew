package httpapi

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/kesokaj/styrman-crew/internal/config"
	"github.com/kesokaj/styrman-crew/internal/orchestrator"
	"github.com/kesokaj/styrman-crew/internal/userstore"
)

// testRoutes builds the handler with real stores in a temp dir; the docker
// client is constructed but never dialed by the paths under test.
func testRoutes(t *testing.T) http.Handler {
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
	cfg := &config.Config{StateDir: dir, HomesDir: filepath.Join(dir, "homes"), Username: "tester", AuthMode: "none"}
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

// A clone URL and the guest command output that names it both reach the
// operator's logs and the project API, so redaction has to be exhaustive.
// This replaced a positional scan (redactURL) that was wrong twice over:
// it stopped at the first URL, and it anchored on the wrong "@".
func TestRedactSecrets(t *testing.T) {
	cases := map[string]string{
		"https://user:ghp_secret@github.com/o/r.git": "https://***@github.com/o/r.git",
		"ssh://git@host/o/r":                         "ssh://***@host/o/r",
		"ssh://git:key@host/o/r":                     "ssh://***@host/o/r",
		"fatal: unable to access 'https://user:ghp_secret@github.com/o/r.git/'": "fatal: unable to access 'https://***@github.com/o/r.git/'",
		// EVERY occurrence, not just the last one.
		"a https://u:t1@h/x b https://u:t2@h/y": "a https://***@h/x b https://***@h/y",
		// scp-style carries a USERNAME, not a secret — stays readable.
		"git@github.com:o/r.git":                         "git@github.com:o/r.git",
		"git@github.com: Permission denied (publickey).": "git@github.com: Permission denied (publickey).",
		// Nothing to do.
		"nothing to redact here":     "nothing to redact here",
		"https://github.com/o/r.git": "https://github.com/o/r.git",
		// An "@" in the PATH is not userinfo. The positional scan mangled
		// this into "https://***@v1.git" (review 2026-09-02).
		"https://github.com/o/r@v1.git": "https://github.com/o/r@v1.git",
		// git, curl and net/url all split userinfo at the LAST "@" of the
		// authority; anchoring on the first left the tail of a password
		// containing "@" in the clear (security review 2026-09-02).
		"https://user:AA@BB@github.com/o/r": "https://***@github.com/o/r",
	}
	for in, want := range cases {
		if got := redactSecrets(in); got != want {
			t.Errorf("redactSecrets(%q) = %q, want %q", in, got, want)
		}
	}
}

// guestErrMessage and clampTitle both render untrusted text into the UI. A
// control character or a bidi override there can make a line claim to say
// something it does not, and a blind byte slice can leave a broken rune.
func TestStripControlsAndClampBytes(t *testing.T) {
	strip := map[string]string{
		// Only the control BYTES go; the printable tail of an escape sequence
		// is ordinary text and stays visible, which is the point.
		"a\x1b[2Jb\x07c":      "a[2Jbc",
		"a\u0085b":            "ab",  // C1 (NEL)
		"a\u202eb":            "ab",  // right-to-left override
		"a\u2066b\u2069c":     "abc", // bidi isolates
		"a\u200bb\ufeffc":     "abc", // zero-width + BOM
		"a\u061cb":            "ab",  // arabic letter mark — the enumerated
		"a\u2028b\u2029c":     "abc", // version missed these three (review)
		"plain ascii is kept": "plain ascii is kept",
		"åäö are kept":        "åäö are kept",
	}
	for in, want := range strip {
		if got := stripControls(in); got != want {
			t.Errorf("stripControls(%q) = %q, want %q", in, got, want)
		}
	}

	// "aé" is 3 bytes: a cut at 2 lands inside "é" and must fall back to 1.
	if got := clampBytes("aé", 2); got != "a" {
		t.Errorf("clampBytes split a multi-byte rune: %q", got)
	}
	if got := clampBytes("aé", 3); got != "aé" {
		t.Errorf("clampBytes truncated an exactly-fitting string: %q", got)
	}
	if got := clampBytes("abc", 10); got != "abc" {
		t.Errorf("clampBytes truncated a short string: %q", got)
	}
	if !utf8.ValidString(clampBytes(strings.Repeat("é", 400), guestErrMaxBytes)) {
		t.Error("clampBytes produced invalid UTF-8")
	}
}

// A clone URL must never carry a secret: git writes the remote verbatim into
// .git/config on the host, where nothing redacts it, and nothing needs it —
// the broker already supplies the ssh key and answers git's credential helper
// per host. A bare username is not a secret and must keep working.
func TestCloneURLHasSecret(t *testing.T) {
	rejected := []string{
		"https://user:ghp_secret@github.com/o/r.git",
		"https://x-access-token:tok@github.com/o/r.git",
		"ssh://git:key@host/o/r",
		"https://user:@github.com/o/r.git", // empty password is still userinfo
	}
	allowed := []string{
		"https://github.com/o/r.git",
		"git@github.com:o/r.git", // scp-style: "git" is a username
		"ssh://git@host/o/r",     // standard SSH form, username only
		"https://user@github.com/o/r.git",
		"https://github.com/o/r@v1.git", // "@" in the PATH is not userinfo
	}
	for _, u := range rejected {
		if !cloneURLHasSecret(u) {
			t.Errorf("clone URL %q embeds a credential and must be refused", u)
		}
	}
	for _, u := range allowed {
		if cloneURLHasSecret(u) {
			t.Errorf("clone URL %q carries no secret and must be accepted", u)
		}
	}
	// Everything refused here must still be a shape cloneURLRe accepts —
	// otherwise the check is dead code behind the regex.
	for _, u := range rejected {
		if !cloneURLRe.MatchString(u) {
			t.Errorf("%q is rejected by cloneURLRe already; the secret check never runs for it", u)
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

// A failed clone used to be logged and nothing else — the user just got a
// silently empty project. guestErrMessage is what carries the reason back, so
// it must pick git's diagnosis line, redact any token riding in the URL, and
// stay bounded (the guest controls this text).
func TestGuestErrMessage(t *testing.T) {
	cases := []struct{ name, out, want string }{
		{
			name: "picks the fatal line over the banner",
			out:  "Cloning into '.'...\r\nfatal: could not read Username for 'https://github.com': No such device or address\n",
			want: "clone failed: fatal: could not read Username for 'https://github.com': No such device or address",
		},
		{
			name: "keeps the ssh diagnosis",
			out:  "Cloning into '.'...\ngit@github.com: Permission denied (publickey).\nfatal: Could not read from remote repository.\n",
			want: "clone failed: fatal: Could not read from remote repository.",
		},
		{
			name: "redacts a token embedded in the remote URL",
			out:  "fatal: unable to access 'https://user:ghp_secret@github.com/o/r.git/'\n",
			want: "clone failed: fatal: unable to access 'https://***@github.com/o/r.git/'",
		},
		{
			name: "redacts EVERY token on the line, not just the last",
			out:  "fatal: https://u:t1@h/x and https://u:t2@h/y both failed\n",
			want: "clone failed: fatal: https://***@h/x and https://***@h/y both failed",
		},
		{
			name: "no usable output degrades to the prefix alone",
			out:  "Cloning into '.'...\n\n",
			want: "clone failed",
		},
	}
	for _, c := range cases {
		if got := guestErrMessage("clone failed", c.out); got != c.want {
			t.Errorf("%s: guestErrMessage() = %q, want %q", c.name, got, c.want)
		}
	}

	// The guest is untrusted and root in its own VM: it controls both the
	// length and the bytes of this string.
	long := guestErrMessage("clone failed", "fatal: "+strings.Repeat("x", 5000))
	if len(long) > 320 {
		t.Errorf("guestErrMessage did not cap output: len = %d", len(long))
	}
	if got := guestErrMessage("clone failed", "fatal: a\x1b[2Jb\x07c"); got != "clone failed: fatal: a[2Jbc" {
		t.Errorf("control bytes not stripped: %q", got)
	}
}
