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

// TestFileEndpointsManagedAndTraversal covers the destructive endpoints'
// guard rails: platform-managed home subtrees are refused, and traversal
// attempts stay confined.
func TestFileEndpointsManagedAndTraversal(t *testing.T) {
	h := testRoutes(t)
	do := func(method, path, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
		return w
	}
	// Touch the home so it exists (any home-scope request creates it lazily).
	do(http.MethodGet, "/api/home/files?path=", "")

	// Managed home subtrees: delete and rename (either side) are refused.
	for _, p := range []string{"projects", ".dockerd", ".styrman", ".ssh", "projects/x", ".dockerd/y.img"} {
		if w := do(http.MethodDelete, "/api/home/file?path="+p, ""); w.Code != http.StatusBadRequest {
			t.Errorf("home delete %q = %d, want 400", p, w.Code)
		}
		if w := do(http.MethodPost, "/api/home/file/rename?path="+p, `{"to":"elsewhere"}`); w.Code != http.StatusBadRequest {
			t.Errorf("home rename from %q = %d, want 400", p, w.Code)
		}
	}
	do(http.MethodPut, "/api/home/file?path=loose.txt", `{"content":"x"}`)
	if w := do(http.MethodPost, "/api/home/file/rename?path=loose.txt", `{"to":".ssh/id_ed25519"}`); w.Code != http.StatusBadRequest {
		t.Errorf("home rename INTO managed path = %d, want 400", w.Code)
	}
	// A non-managed home file deletes fine.
	if w := do(http.MethodDelete, "/api/home/file?path=loose.txt", ""); w.Code != http.StatusNoContent {
		t.Errorf("home delete of plain file = %d, want 204", w.Code)
	}

	// Dot-traversal is neutralized by path.Clean: these collapse to the scope
	// root, which is refused outright.
	for _, p := range []string{"../../..", "a/../../.."} {
		if w := do(http.MethodDelete, "/api/home/file?path="+p, ""); w.Code != http.StatusBadRequest {
			t.Errorf("traversal delete %q = %d, want 400", p, w.Code)
		}
	}
}

// TestFileDeleteSymlinkEscape: a guest-planted symlink must not let delete
// reach outside the home (os.Root confinement), and deleting the symlink
// itself must remove only the link.
func TestFileDeleteSymlinkEscape(t *testing.T) {
	h, dir := testRoutesWithDir(t)
	do := func(method, path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader("")))
		return w
	}
	do(http.MethodGet, "/api/home/files?path=") // create the home lazily
	home := filepath.Join(dir, "homes", "tester")
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
	if w := do(http.MethodDelete, "/api/home/file?path=esc/secret.txt"); w.Code != http.StatusBadRequest {
		t.Errorf("delete through symlink = %d, want 400", w.Code)
	}
	if _, err := os.Stat(filepath.Join(outside, "secret.txt")); err != nil {
		t.Fatal("outside file was deleted through a symlink")
	}
	// Deleting the symlink itself removes only the link.
	if w := do(http.MethodDelete, "/api/home/file?path=esc"); w.Code != http.StatusNoContent {
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
	return New(cfg, users, orch, logger).Routes(), dir
}
