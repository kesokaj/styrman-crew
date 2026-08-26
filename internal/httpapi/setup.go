package httpapi

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// setupStatus reports which credentials already exist in the shared home.
// Probed host-side (the server owns the home), so it works with every
// microVM stopped. agy's keyring entries can't be inspected host-side without
// the password dance, so keyring presence is the best-effort signal there.
type setupStatus struct {
	GitIdentity bool `json:"gitIdentity"`
	SSHKey      bool `json:"sshKey"`
	GitHub      bool `json:"github"`
	Claude      bool `json:"claude"`
	Keyring     bool `json:"keyring"`
	Done        bool `json:"done"`      // operator clicked "done" (marker in the home)
	VMRunning   bool `json:"vmRunning"` // hidden setup microVM currently exists
}

const setupDoneMarker = ".styrman/setup-done"

func (s *Server) getSetupStatus(w http.ResponseWriter, r *http.Request) {
	h := s.cfg.HomeDir
	st := setupStatus{
		SSHKey:  fileExists(filepath.Join(h, ".ssh", "id_ed25519")),
		GitHub:  fileExists(filepath.Join(h, ".config", "gh", "hosts.yml")),
		Claude:  fileExists(filepath.Join(h, ".claude", ".credentials.json")),
		Keyring: fileExists(filepath.Join(h, ".local", "share", "keyrings", "login.keyring")),
		Done:    fileExists(filepath.Join(h, setupDoneMarker)),
	}
	if data, err := os.ReadFile(filepath.Join(h, ".gitconfig")); err == nil {
		st.GitIdentity = strings.Contains(string(data), "email")
	}
	if cst, err := s.orch.SetupStatus(r.Context()); err == nil {
		st.VMRunning = cst
	}
	writeJSON(w, http.StatusOK, st)
}

// startSetup boots the hidden onboarding microVM. The terminal itself is
// attached via GET /ws/setup.
func (s *Server) startSetup(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	if _, err := s.orch.EnsureSetup(ctx); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"state": "running"})
}

// finishSetup records completion in the shared home (so the UI banner stays
// gone across reinstalls of everything else) and tears the hidden VM down.
func (s *Server) finishSetup(w http.ResponseWriter, r *http.Request) {
	marker := filepath.Join(s.cfg.HomeDir, setupDoneMarker)
	if err := os.MkdirAll(filepath.Dir(marker), 0o755); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if err := os.WriteFile(marker, []byte(time.Now().UTC().Format(time.RFC3339)+"\n"), 0o644); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	dctx, dcancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer dcancel()
	if err := s.orch.DestroySetup(dctx); err != nil {
		s.logger.Warn("setup vm teardown failed", "error", err)
	}
	w.WriteHeader(http.StatusNoContent)
}

// discardSetup tears the hidden VM down without marking setup done.
func (s *Server) discardSetup(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := s.orch.DestroySetup(ctx); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
