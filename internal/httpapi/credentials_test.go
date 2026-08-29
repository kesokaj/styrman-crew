package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kesokaj/styrman-crew/internal/config"
	"github.com/kesokaj/styrman-crew/internal/credstore"
	"github.com/kesokaj/styrman-crew/internal/orchestrator"
	"github.com/kesokaj/styrman-crew/internal/userstore"
)

// routesWithCreds builds the API with a credential store wired, so the
// broker-onboarding endpoints are enabled (the default test harness leaves the
// store nil → 503).
func routesWithCreds(t *testing.T) http.Handler {
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
	cs, err := credstore.New(filepath.Join(dir, "credentials"))
	if err != nil {
		t.Fatal(err)
	}
	srv := New(cfg, users, orch, logger)
	srv.SetCredStore(cs)
	return srv.Routes()
}

// TestCredentialsAPI: store a token, see it listed, delete it. Covers the
// value-free list, the manual-token endpoint, and delete. (Import-from-project
// needs a running container and is covered by the live integration test.)
func TestCredentialsAPI(t *testing.T) {
	h := routesWithCreds(t)
	do := func(method, path, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
		return w
	}

	// Empty to start.
	w := do(http.MethodGet, "/api/settings/credentials", "")
	if w.Code != http.StatusOK {
		t.Fatalf("list = %d: %s", w.Code, w.Body.String())
	}
	var list struct {
		Items []struct{ Kind, ID, Name string } `json:"items"`
	}
	json.Unmarshal(w.Body.Bytes(), &list)
	if len(list.Items) != 0 {
		t.Fatalf("fresh store not empty: %+v", list.Items)
	}

	// Bad key rejected.
	if w := do(http.MethodPost, "/api/settings/credentials/token", `{"key":"has space","value":"v"}`); w.Code != http.StatusBadRequest {
		t.Errorf("bad key = %d, want 400", w.Code)
	}
	// Missing value rejected.
	if w := do(http.MethodPost, "/api/settings/credentials/token", `{"key":"NPM_TOKEN"}`); w.Code != http.StatusBadRequest {
		t.Errorf("missing value = %d, want 400", w.Code)
	}

	// Store a valid token.
	if w := do(http.MethodPost, "/api/settings/credentials/token", `{"key":"NPM_TOKEN","value":"npm_secret"}`); w.Code != http.StatusOK {
		t.Fatalf("store token = %d: %s", w.Code, w.Body.String())
	}

	// It appears in the list — value-free.
	w = do(http.MethodGet, "/api/settings/credentials", "")
	json.Unmarshal(w.Body.Bytes(), &list)
	if len(list.Items) != 1 || list.Items[0].Kind != "env" || list.Items[0].Name != "NPM_TOKEN" {
		t.Fatalf("list after store = %+v", list.Items)
	}
	if strings.Contains(w.Body.String(), "npm_secret") {
		t.Error("list leaked the secret value")
	}

	// Delete it.
	if w := do(http.MethodDelete, "/api/settings/credentials/env/NPM_TOKEN", ""); w.Code != http.StatusNoContent {
		t.Fatalf("delete = %d", w.Code)
	}
	// Deleting again → 404.
	if w := do(http.MethodDelete, "/api/settings/credentials/env/NPM_TOKEN", ""); w.Code != http.StatusNotFound {
		t.Errorf("delete missing = %d, want 404", w.Code)
	}
}

// TestCredentialsDisabled: with no store wired (host-mode dev), the LIST
// endpoint returns an empty list with brokerEnabled:false (so the Settings
// page renders without a failed request), while the MUTATING endpoints report
// 503 (they genuinely can't work without a store) — never a panic or 500.
func TestCredentialsDisabled(t *testing.T) {
	h := testRoutes(t) // no credstore
	do := func(method, path, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
		return w
	}

	w := do(http.MethodGet, "/api/settings/credentials", "")
	if w.Code != http.StatusOK {
		t.Fatalf("no-store list = %d, want 200", w.Code)
	}
	var got struct {
		Items         []struct{ Kind, ID, Name string } `json:"items"`
		BrokerEnabled bool                              `json:"brokerEnabled"`
	}
	json.Unmarshal(w.Body.Bytes(), &got)
	if got.BrokerEnabled || len(got.Items) != 0 {
		t.Fatalf("no-store list body = %+v, want empty + brokerEnabled:false", got)
	}

	// Mutating endpoints still 503 without a store.
	if w := do(http.MethodPost, "/api/settings/credentials/token", `{"key":"X","value":"y"}`); w.Code != http.StatusServiceUnavailable {
		t.Errorf("no-store token = %d, want 503", w.Code)
	}
	if w := do(http.MethodPost, "/api/settings/credentials/import", `{"project":"p"}`); w.Code != http.StatusServiceUnavailable {
		t.Errorf("no-store import = %d, want 503", w.Code)
	}
}
