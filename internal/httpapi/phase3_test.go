package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestUpdateRuntime covers the admin runtime tunables endpoint: validation
// (400), success (200 + persisted), and the settings payload reflecting it.
func TestUpdateRuntime(t *testing.T) {
	h, dir := testRoutesWithDir(t)
	do := func(method, path, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
		return w
	}

	for _, bad := range []string{
		`{"dockerDiskGB": 2}`,
		`{"dockerDiskGB": 900}`,
		`{"idleTimeout": "3m"}`,
		`{"idleTimeout": "-1h"}`,
		`{"idleTimeout": "nonsense"}`,
	} {
		if w := do(http.MethodPatch, "/api/settings/runtime", bad); w.Code != http.StatusBadRequest {
			t.Errorf("PATCH %s = %d, want 400", bad, w.Code)
		}
	}

	w := do(http.MethodPatch, "/api/settings/runtime", `{"dockerDiskGB": 40, "idleTimeout": "2h"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("valid PATCH = %d: %s", w.Code, w.Body.String())
	}
	var got struct {
		DockerDiskGB int    `json:"dockerDiskGB"`
		IdleTimeout  string `json:"idleTimeout"`
	}
	json.Unmarshal(w.Body.Bytes(), &got)
	if got.DockerDiskGB != 40 || got.IdleTimeout != "2h0m0s" {
		t.Fatalf("applied = %+v", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "settings.json")); err != nil {
		t.Fatal("settings.json not persisted")
	}
	// The settings payload reflects the change and does NOT leak allowedUsers.
	w = do(http.MethodGet, "/api/settings", "")
	if !strings.Contains(w.Body.String(), `"dockerDiskGB":40`) {
		t.Errorf("settings payload missing new disk: %s", w.Body.String())
	}
	if strings.Contains(w.Body.String(), "allowedUsers") {
		t.Error("settings payload still leaks allowedUsers")
	}
}

// TestTerminalAutostart: PATCH sets/clears the command with validation, and
// the terminals listing carries it.
func TestTerminalAutostart(t *testing.T) {
	h := testRoutes(t)
	do := func(method, path, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
		return w
	}
	w := do(http.MethodPost, "/api/projects", `{"name":"auto"}`)
	var proj struct {
		ID string `json:"id"`
	}
	json.Unmarshal(w.Body.Bytes(), &proj)
	w = do(http.MethodPost, "/api/projects/"+proj.ID+"/terminals", `{}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("create terminal = %d: %s", w.Code, w.Body.String())
	}
	var term struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	json.Unmarshal(w.Body.Bytes(), &term)
	if term.Title == "" || strings.HasPrefix(term.Title, "Terminal ") {
		t.Errorf("expected generated friendly name, got %q", term.Title)
	}
	base := "/api/projects/" + proj.ID + "/terminals/" + term.ID

	// Invalid commands are refused.
	for _, bad := range []string{
		`{"command":"a\nb"}`,
		`{"command":"` + strings.Repeat("x", 600) + `"}`,
	} {
		if w := do(http.MethodPatch, base, bad); w.Code != http.StatusBadRequest {
			t.Errorf("PATCH %.30q = %d, want 400", bad, w.Code)
		}
	}
	// Empty body (neither field) is refused.
	if w := do(http.MethodPatch, base, `{}`); w.Code != http.StatusBadRequest {
		t.Errorf("empty PATCH = %d, want 400", w.Code)
	}

	if w := do(http.MethodPatch, base, `{"command":"docker compose up"}`); w.Code != http.StatusNoContent {
		t.Fatalf("set command = %d: %s", w.Code, w.Body.String())
	}
	w = do(http.MethodGet, "/api/projects/"+proj.ID+"/terminals", "")
	if !strings.Contains(w.Body.String(), `"command":"docker compose up"`) {
		t.Errorf("listing missing command: %s", w.Body.String())
	}
	// Clearing with "" while renaming in the same PATCH.
	if w := do(http.MethodPatch, base, `{"title":"svc","command":""}`); w.Code != http.StatusNoContent {
		t.Fatalf("clear+rename = %d", w.Code)
	}
	w = do(http.MethodGet, "/api/projects/"+proj.ID+"/terminals", "")
	if strings.Contains(w.Body.String(), `"command"`) || !strings.Contains(w.Body.String(), `"title":"svc"`) {
		t.Errorf("clear+rename not reflected: %s", w.Body.String())
	}
}

// TestHistorySearchRoute: the literal /history/search route wins over
// /history/{id}, is scoped to the project, and returns title-joined matches.
func TestHistorySearchRoute(t *testing.T) {
	h, dir := testRoutesWithDir(t)
	do := func(method, path, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
		return w
	}
	w := do(http.MethodPost, "/api/projects", `{"name":"hist"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("create = %d", w.Code)
	}
	var proj struct {
		ID string `json:"id"`
	}
	json.Unmarshal(w.Body.Bytes(), &proj)

	// Plant a transcript the way tmux pipe-pane would.
	// Transcripts live in the PER-PROJECT home (ADR-031).
	logs := filepath.Join(dir, "homes", "tester", "p", proj.ID, ".styrman", "logs", proj.ID)
	os.MkdirAll(logs, 0o755)
	os.WriteFile(filepath.Join(logs, "conv-9.log"), []byte("hello grep-target world\n"), 0o644)

	w = do(http.MethodGet, "/api/projects/"+proj.ID+"/history/search?q=grep-target", "")
	if w.Code != http.StatusOK {
		t.Fatalf("search = %d: %s", w.Code, w.Body.String())
	}
	var res struct {
		Matches []struct {
			Transcript string `json:"transcript"`
			Line       string `json:"line"`
		} `json:"matches"`
	}
	json.Unmarshal(w.Body.Bytes(), &res)
	if len(res.Matches) != 1 || res.Matches[0].Transcript != "conv-9" {
		t.Fatalf("matches = %+v", res.Matches)
	}
	// Overlong queries are refused.
	if w := do(http.MethodGet, "/api/projects/"+proj.ID+"/history/search?q="+strings.Repeat("a", 300), ""); w.Code != http.StatusBadRequest {
		t.Errorf("overlong query = %d, want 400", w.Code)
	}
}
