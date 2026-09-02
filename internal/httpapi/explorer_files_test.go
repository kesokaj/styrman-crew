package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kesokaj/styrman-crew/internal/config"
	"github.com/kesokaj/styrman-crew/internal/orchestrator"
	"github.com/kesokaj/styrman-crew/internal/userstore"
)

// TestFileRoundTripProjectScope guards the save path against the prefix
// double-apply bug: readFile must report paths relative to the SCOPE (like
// listFiles), or saving a file with the server-returned path writes to
// projects/<dir>/projects/<dir>/… and fails.
func TestFileRoundTripProjectScope(t *testing.T) {
	h := testRoutes(t)
	do := func(method, path, body string) *httptest.ResponseRecorder {
		var rd *strings.Reader
		if body == "" {
			rd = strings.NewReader("")
		} else {
			rd = strings.NewReader(body)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(method, path, rd))
		return w
	}

	// Create a project (the background microVM boot fails without docker —
	// irrelevant here; the workspace dir + record exist after the 201).
	w := do(http.MethodPost, "/api/projects", `{"name":"demo"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("create project = %d: %s", w.Code, w.Body.String())
	}
	var proj struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &proj); err != nil || proj.ID == "" {
		t.Fatalf("bad create response: %s", w.Body.String())
	}
	base := "/api/projects/" + proj.ID

	// Create a file the way the SPA's "New file" does.
	if w := do(http.MethodPut, base+"/file?path=note.txt", `{"content":"v1"}`); w.Code != http.StatusNoContent {
		t.Fatalf("initial write = %d: %s", w.Code, w.Body.String())
	}

	// Read it back: the reported path must be scope-relative.
	w = do(http.MethodGet, base+"/file?path=note.txt", "")
	if w.Code != http.StatusOK {
		t.Fatalf("read = %d: %s", w.Code, w.Body.String())
	}
	var fc struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &fc); err != nil {
		t.Fatal(err)
	}
	if fc.Path != "note.txt" {
		t.Fatalf("readFile path = %q, want scope-relative \"note.txt\"", fc.Path)
	}

	// The round trip the editor performs: save using the RETURNED path.
	if w := do(http.MethodPut, base+"/file?path="+fc.Path, `{"content":"v2"}`); w.Code != http.StatusNoContent {
		t.Fatalf("round-trip save = %d: %s", w.Code, w.Body.String())
	}
	w = do(http.MethodGet, base+"/file?path=note.txt", "")
	_ = json.Unmarshal(w.Body.Bytes(), &fc)
	if fc.Content != "v2" {
		t.Fatalf("round-trip content = %q, want \"v2\"", fc.Content)
	}

	// New folder, rename into it, delete.
	if w := do(http.MethodPost, base+"/mkdir?path=docs", ""); w.Code != http.StatusNoContent {
		t.Fatalf("mkdir = %d: %s", w.Code, w.Body.String())
	}
	if w := do(http.MethodPost, base+"/file/rename?path=note.txt", `{"to":"docs/note.md"}`); w.Code != http.StatusNoContent {
		t.Fatalf("rename = %d: %s", w.Code, w.Body.String())
	}
	if w := do(http.MethodGet, base+"/file?path=docs/note.md", ""); w.Code != http.StatusOK {
		t.Fatalf("read after rename = %d", w.Code)
	}
	if w := do(http.MethodDelete, base+"/file?path=docs/note.md", ""); w.Code != http.StatusNoContent {
		t.Fatalf("delete = %d: %s", w.Code, w.Body.String())
	}
	if w := do(http.MethodGet, base+"/file?path=docs/note.md", ""); w.Code == http.StatusOK {
		t.Fatal("file still readable after delete")
	}

	// The scope root itself must be refused.
	if w := do(http.MethodDelete, base+"/file?path=", ""); w.Code != http.StatusBadRequest {
		t.Fatalf("delete scope root = %d, want 400", w.Code)
	}
	if w := do(http.MethodDelete, base+"/file?path=.", ""); w.Code != http.StatusBadRequest {
		t.Fatalf("delete \".\" = %d, want 400", w.Code)
	}

	// No-clobber: renaming onto an existing file must fail.
	do(http.MethodPut, base+"/file?path=a.txt", `{"content":"a"}`)
	do(http.MethodPut, base+"/file?path=b.txt", `{"content":"b"}`)
	if w := do(http.MethodPost, base+"/file/rename?path=a.txt", `{"to":"b.txt"}`); w.Code != http.StatusBadRequest {
		t.Fatalf("clobbering rename = %d, want 400", w.Code)
	}
	w = do(http.MethodGet, base+"/file?path=b.txt", "")
	_ = json.Unmarshal(w.Body.Bytes(), &fc)
	if fc.Content != "b" {
		t.Fatalf("clobbering rename overwrote destination: %q", fc.Content)
	}
}

// makeProject creates a project and returns its id (its workspace + home
// dirs exist after this — createProject makes both).
func makeProject(t *testing.T, h http.Handler, name string) string {
	t.Helper()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/projects", strings.NewReader(`{"name":"`+name+`"}`)))
	if w.Code != http.StatusCreated {
		t.Fatalf("create project %q = %d: %s", name, w.Code, w.Body.String())
	}
	var p struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil || p.ID == "" {
		t.Fatalf("bad create response: %s", w.Body.String())
	}
	return p.ID
}

// TestFileEndpointsManagedAndTraversal covers the destructive endpoints'
// guard rails: platform-managed home subtrees are refused, and traversal
// attempts stay confined. Home is per-project (ADR-031), so home scope is
// /api/projects/{id}/home.
func TestFileEndpointsManagedAndTraversal(t *testing.T) {
	h := testRoutes(t)
	pid := makeProject(t, h, "guarded")
	home := "/api/projects/" + pid + "/home"
	do := func(method, path, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
		return w
	}

	// Managed home subtrees: delete, rename (either side), AND the write
	// paths (audit finding: an unguarded PUT could O_TRUNC a running VM's
	// .dockerd disk) are all refused.
	for _, p := range []string{"projects", ".dockerd", ".styrman", ".ssh", "projects/x", ".dockerd/y.img"} {
		if w := do(http.MethodDelete, home+"/file?path="+p, ""); w.Code != http.StatusBadRequest {
			t.Errorf("home delete %q = %d, want 400", p, w.Code)
		}
		if w := do(http.MethodPost, home+"/file/rename?path="+p, `{"to":"elsewhere"}`); w.Code != http.StatusBadRequest {
			t.Errorf("home rename from %q = %d, want 400", p, w.Code)
		}
		if w := do(http.MethodPut, home+"/file?path="+p, `{"content":"x"}`); w.Code != http.StatusBadRequest {
			t.Errorf("home write %q = %d, want 400", p, w.Code)
		}
		if w := do(http.MethodPost, home+"/mkdir?path="+p, ""); w.Code != http.StatusBadRequest {
			t.Errorf("home mkdir %q = %d, want 400", p, w.Code)
		}
	}
	// Upload into a managed dir is refused too (multipart, part named x).
	{
		var mp strings.Builder
		mp.WriteString("--BOUND\r\nContent-Disposition: form-data; name=\"file\"; filename=\"x.img\"\r\n\r\ndata\r\n--BOUND--\r\n")
		req := httptest.NewRequest(http.MethodPost, home+"/upload?path=.dockerd", strings.NewReader(mp.String()))
		req.Header.Set("Content-Type", "multipart/form-data; boundary=BOUND")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("home upload into .dockerd = %d, want 400", w.Code)
		}
	}
	// The same names in PROJECT scope are ordinary paths — mkdir + write OK.
	if w := do(http.MethodPost, "/api/projects/"+pid+"/mkdir?path=.dockerd", ""); w.Code != http.StatusNoContent {
		t.Errorf("project-scope mkdir .dockerd = %d, want 204", w.Code)
	}
	if w := do(http.MethodPut, "/api/projects/"+pid+"/file?path=.dockerd/ok.txt", `{"content":"x"}`); w.Code != http.StatusNoContent {
		t.Errorf("project-scope write to .dockerd = %d, want 204", w.Code)
	}
	do(http.MethodPut, home+"/file?path=loose.txt", `{"content":"x"}`)
	if w := do(http.MethodPost, home+"/file/rename?path=loose.txt", `{"to":".ssh/id_ed25519"}`); w.Code != http.StatusBadRequest {
		t.Errorf("home rename INTO managed path = %d, want 400", w.Code)
	}
	// A non-managed home file deletes fine.
	if w := do(http.MethodDelete, home+"/file?path=loose.txt", ""); w.Code != http.StatusNoContent {
		t.Errorf("home delete of plain file = %d, want 204", w.Code)
	}

	// Dot-traversal is neutralized by path.Clean: these collapse to the scope
	// root, which is refused outright.
	for _, p := range []string{"../../..", "a/../../.."} {
		if w := do(http.MethodDelete, home+"/file?path="+p, ""); w.Code != http.StatusBadRequest {
			t.Errorf("traversal delete %q = %d, want 400", p, w.Code)
		}
	}
}

// TestFileDeleteSymlinkEscape: a guest-planted symlink must not let delete
// reach outside the home (os.Root confinement), and deleting the symlink
// itself must remove only the link.
func TestFileDeleteSymlinkEscape(t *testing.T) {
	h, dir := testRoutesWithDir(t)
	pid := makeProject(t, h, "symesc")
	homeRoute := "/api/projects/" + pid + "/home"
	do := func(method, path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader("")))
		return w
	}
	home := filepath.Join(dir, "homes", "tester", "p", pid) // ProjectHome layout
	outside := filepath.Join(dir, "outside")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("s"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(home, "esc")); err != nil {
		t.Fatal(err)
	}

	// Deleting THROUGH the symlink is refused by the confined root.
	if w := do(http.MethodDelete, homeRoute+"/file?path=esc/secret.txt"); w.Code != http.StatusBadRequest {
		t.Errorf("delete through symlink = %d, want 400", w.Code)
	}
	if _, err := os.Stat(filepath.Join(outside, "secret.txt")); err != nil {
		t.Fatal("outside file was deleted through a symlink")
	}
	// Deleting the symlink itself removes only the link.
	if w := do(http.MethodDelete, homeRoute+"/file?path=esc"); w.Code != http.StatusNoContent {
		t.Errorf("delete of symlink = %d, want 204", w.Code)
	}
	if _, err := os.Stat(filepath.Join(outside, "secret.txt")); err != nil {
		t.Fatal("deleting the symlink removed its target's contents")
	}
}

// testRoutesWithDir is testRoutes but exposes the state dir for tests that
// need to reach the filesystem behind the API.
func testRoutesWithDir(t *testing.T) (http.Handler, string) {
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
	return New(cfg, users, orch, logger).Routes(), dir
}

// A mid-stream upload failure must still report the parts that LANDED: the
// client evicts panes holding those files so a stale buffer cannot overwrite
// the upload on Save (code review 2026-09-02). Home scope: "a.txt" saves,
// ".ssh" is a managed name and is refused — the 400 body carries saved=[a.txt].
func TestUploadReportsSavedPartsOnFailure(t *testing.T) {
	h := testRoutes(t)
	pid := makeProject(t, h, "partial-upload")
	home := "/api/projects/" + pid + "/home"
	var mp strings.Builder
	mp.WriteString("--BOUND\r\nContent-Disposition: form-data; name=\"files\"; filename=\"a.txt\"\r\n\r\nfirst\r\n")
	mp.WriteString("--BOUND\r\nContent-Disposition: form-data; name=\"files\"; filename=\".ssh\"\r\n\r\nnope\r\n--BOUND--\r\n")
	req := httptest.NewRequest(http.MethodPost, home+"/upload?path=", strings.NewReader(mp.String()))
	req.Header.Set("Content-Type", "multipart/form-data; boundary=BOUND")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("partial upload = %d, want 400: %s", w.Code, w.Body.String())
	}
	var body struct {
		Saved []string          `json:"saved"`
		Error map[string]string `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("400 body is not JSON: %v: %s", err, w.Body.String())
	}
	if len(body.Saved) != 1 || body.Saved[0] != "a.txt" {
		t.Fatalf("saved = %v, want [a.txt] — the client must learn what landed", body.Saved)
	}
	if body.Error["message"] == "" {
		t.Fatal("error message missing alongside saved")
	}
	// The landed part really is on disk (so evicting its pane was right).
	if w := (func() *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, home+"/file?path=a.txt", nil))
		return w
	})(); w.Code != http.StatusOK {
		t.Fatalf("a.txt after partial upload = %d, want 200", w.Code)
	}

	// A successful upload reports the list; an upload where EVERY part is
	// skipped (no usable filename) must still report an EMPTY list, not null —
	// the client iterates it (a Go nil slice serialises as null).
	for _, tc := range []struct{ name, body, want string }{
		{"saved part", "--BOUND\r\nContent-Disposition: form-data; name=\"files\"; filename=\"b.txt\"\r\n\r\nsecond\r\n--BOUND--\r\n", `"saved":["b.txt"]`},
		{"nothing usable", "--BOUND\r\nContent-Disposition: form-data; name=\"files\"; filename=\"\"\r\n\r\nx\r\n--BOUND--\r\n", `"saved":[]`},
	} {
		req = httptest.NewRequest(http.MethodPost, home+"/upload?path=", strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "multipart/form-data; boundary=BOUND")
		w = httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), tc.want) {
			t.Fatalf("%s: upload = %d body %s, want 200 containing %s", tc.name, w.Code, w.Body.String(), tc.want)
		}
	}
}
