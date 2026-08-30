package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/kesokaj/styrman-crew/internal/credstore"
)

// Project-scoped env variables (ADR-043): like the user-wide "custom
// variables" in Settings, but delivered ONLY to this project's login shells —
// the broker merges them over the user-wide tier for this project's callers
// (project value wins on a shared key). Managed from the project SidePanel.
// Values apply to NEW login shells; running shells keep what they exported.

// listProjectEnv returns the project's variable KEYS (never the values — the
// list backs a UI table; values are fetched one at a time, audit-logged).
func (s *Server) listProjectEnv(w http.ResponseWriter, r *http.Request) {
	u, _, p, err := s.getProjectFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	if s.creds == nil {
		writeJSON(w, http.StatusOK, map[string]any{"brokerEnabled": false, "keys": []string{}})
		return
	}
	m, err := s.creds.ProjectEnv(u.Username, p.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, errors.New("cannot list project variables"))
		return
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	writeJSON(w, http.StatusOK, map[string]any{"brokerEnabled": true, "keys": keys})
}

// putProjectEnv stores (or replaces) one project-scoped variable.
func (s *Server) putProjectEnv(w http.ResponseWriter, r *http.Request) {
	u, _, p, err := s.getProjectFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	if s.creds == nil {
		writeErr(w, http.StatusServiceUnavailable, errors.New("credential broker disabled"))
		return
	}
	var body struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Key == "" || body.Value == "" {
		writeErr(w, http.StatusBadRequest, errors.New("key and value required"))
		return
	}
	if err := s.creds.PutProjectEnv(u.Username, p.ID, body.Key, body.Value); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	s.logger.Info("project env variable stored", "user", u.Username, "project", p.ID, "key", body.Key)
	writeJSON(w, http.StatusOK, map[string]string{"key": body.Key, "status": "stored"})
}

// projectEnvValue reveals one variable's value — the owner reading their own
// secret; logged like the Settings credential viewer.
func (s *Server) projectEnvValue(w http.ResponseWriter, r *http.Request) {
	u, _, p, err := s.getProjectFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	if s.creds == nil {
		writeErr(w, http.StatusServiceUnavailable, errors.New("credential broker disabled"))
		return
	}
	key := r.PathValue("key")
	m, err := s.creds.ProjectEnv(u.Username, p.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, errors.New("cannot read project variables"))
		return
	}
	v, ok := m[key]
	if !ok {
		writeErr(w, http.StatusNotFound, errors.New("no such variable"))
		return
	}
	s.logger.Info("project env variable read", "user", u.Username, "project", p.ID, "key", key)
	writeJSON(w, http.StatusOK, map[string]string{"key": key, "value": v})
}

// deleteProjectEnv removes one project-scoped variable.
func (s *Server) deleteProjectEnv(w http.ResponseWriter, r *http.Request) {
	u, _, p, err := s.getProjectFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	if s.creds == nil {
		writeErr(w, http.StatusServiceUnavailable, errors.New("credential broker disabled"))
		return
	}
	key := r.PathValue("key")
	if err := s.creds.DeleteProjectEnv(u.Username, p.ID, key); err != nil {
		if errors.Is(err, credstore.ErrNotFound) {
			writeErr(w, http.StatusNotFound, err)
			return
		}
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	s.logger.Info("project env variable removed", "user", u.Username, "project", p.ID, "key", key)
	w.WriteHeader(http.StatusNoContent)
}
