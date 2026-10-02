package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kesokaj/styrman-crew/internal/config"
	"github.com/kesokaj/styrman-crew/internal/orchestrator"
	"github.com/kesokaj/styrman-crew/internal/userstore"
)

const updaterFailingFile = `{"result":"failing","code":"fetch-auth","checkedAt":"2026-10-02T17:58:00Z","since":"2026-09-22T09:14:02Z","deployed":"v1.38.0","latest":""}`

// The gate (ADR-081): the updater's status is for an admin, and only when
// there is a status to show.
func TestUpdaterForIsAdminOnly(t *testing.T) {
	now := time.Date(2026, 10, 2, 18, 0, 0, 0, time.UTC)
	p := filepath.Join(t.TempDir(), "updater.json")
	if err := os.WriteFile(p, []byte(updaterFailingFile), 0o644); err != nil {
		t.Fatal(err)
	}
	if st, ok := updaterFor(true, p, now); !ok || st.State != "failing" || st.Code != "fetch-auth" {
		t.Errorf("admin with a file: got %+v ok=%v, want failing/fetch-auth", st, ok)
	}
	if st, ok := updaterFor(false, p, now); ok || st.State != "" {
		t.Errorf("a non-admin must get nothing, got %+v ok=%v", st, ok)
	}
	if _, ok := updaterFor(true, "", now); ok {
		t.Error("admin with the feature off (no path) must get nothing")
	}
	if _, ok := updaterFor(true, filepath.Join(t.TempDir(), "updater.json"), now); ok {
		t.Error("admin on a host with no status file must get nothing")
	}
}

// Through the handler: the settings payload carries `updater` when the
// configured file exists, and has no such key otherwise (auth mode none
// makes the caller an admin).
func TestSettingsPayloadCarriesTheUpdaterStatus(t *testing.T) {
	settingsOf := func(statusFile string) map[string]json.RawMessage {
		t.Helper()
		dir := t.TempDir()
		cfg := &config.Config{StateDir: dir, HomesDir: filepath.Join(dir, "homes"), Username: "tester", AuthMode: "none", UpdateStatusFile: statusFile}
		logger := slog.New(slog.DiscardHandler)
		users, err := userstore.New(filepath.Join(dir, "users.json"), nil)
		if err != nil {
			t.Fatal(err)
		}
		orch, err := orchestrator.New(cfg, logger)
		if err != nil {
			t.Fatal(err)
		}
		w := httptest.NewRecorder()
		New(cfg, users, orch, logger).Routes().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/settings", nil))
		if w.Code != http.StatusOK {
			t.Fatalf("GET /api/settings = %d: %s", w.Code, w.Body.String())
		}
		var out map[string]json.RawMessage
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}

	if _, has := settingsOf("")["updater"]; has {
		t.Error("no status path configured: the payload must have no updater key")
	}
	missing := filepath.Join(t.TempDir(), "updater.json")
	if _, has := settingsOf(missing)["updater"]; has {
		t.Error("no status file on the host: the payload must have no updater key")
	}

	p := filepath.Join(t.TempDir(), "updater.json")
	// Written with the current time so the status is not stale.
	body := `{"result":"failing","code":"fetch-auth","checkedAt":"` + time.Now().UTC().Format(time.RFC3339) + `","since":"2026-09-22T09:14:02Z","deployed":"v1.38.0","latest":""}`
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	raw, has := settingsOf(p)["updater"]
	if !has {
		t.Fatal("a status file exists: the payload must carry updater")
	}
	var st struct{ State, Code, Since, Deployed string }
	if err := json.Unmarshal(raw, &st); err != nil {
		t.Fatal(err)
	}
	if st.State != "failing" || st.Code != "fetch-auth" || st.Since != "2026-09-22T09:14:02Z" || st.Deployed != "v1.38.0" {
		t.Errorf("updater = %+v", st)
	}

	if err := os.WriteFile(p, []byte("garbage"), 0o644); err != nil {
		t.Fatal(err)
	}
	raw = settingsOf(p)["updater"]
	if string(raw) != `{"state":"unknown"}` {
		t.Errorf("an untrustworthy file must read exactly unknown, got %s", raw)
	}
}
