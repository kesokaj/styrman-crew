package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"

	"github.com/kesokaj/styrman/internal/files"
	"github.com/kesokaj/styrman/internal/history"
)

// rootFor resolves the file-explorer root for a scope: a project's workspace
// or the shared home (which is where all credentials and agent state live,
// so it is browsable too — R3).
func (s *Server) rootFor(r *http.Request) (string, error) {
	if name := r.PathValue("project"); name != "" {
		if _, err := s.projects.Get(name); err != nil {
			return "", err
		}
		return filepath.Join(s.cfg.HomeDir, "projects", name), nil
	}
	return s.cfg.HomeDir, nil
}

func (s *Server) listFiles(w http.ResponseWriter, r *http.Request) {
	root, err := s.rootFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	entries, err := files.List(root, r.URL.Query().Get("path"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": entries})
}

func (s *Server) readFile(w http.ResponseWriter, r *http.Request) {
	root, err := s.rootFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	fc, err := files.Read(root, r.URL.Query().Get("path"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, fc)
}

func (s *Server) writeFile(w http.ResponseWriter, r *http.Request) {
	root, err := s.rootFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	var body struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := files.Write(root, r.URL.Query().Get("path"), body.Content); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// downloadFile streams a raw file (any type) as an attachment.
func (s *Server) downloadFile(w http.ResponseWriter, r *http.Request) {
	root, err := s.rootFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	rel := r.URL.Query().Get("path")
	full, err := files.Resolve(root, rel)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	w.Header().Set("Content-Disposition", "attachment; filename=\""+filepath.Base(full)+"\"")
	http.ServeFile(w, r, full)
}

// listHistory returns the project's terminal transcripts.
func (s *Server) listHistory(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("project")
	if _, err := s.projects.Get(name); err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	items, err := history.List(s.cfg.HomeDir, name)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"transcripts": items})
}

func (s *Server) readHistory(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("project")
	if _, err := s.projects.Get(name); err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	max, _ := strconv.ParseInt(r.URL.Query().Get("bytes"), 10, 64)
	text, err := history.Read(s.cfg.HomeDir, name, r.PathValue("id"), max)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": r.PathValue("id"), "text": text})
}

func (s *Server) projectDiff(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("project")
	if _, err := s.projects.Get(name); err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	d, err := history.GitDiff(filepath.Join(s.cfg.HomeDir, "projects", name))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

// settings reports everything the settings page shows: who you are, how the
// server is configured, and where state lives.
func (s *Server) settings(w http.ResponseWriter, r *http.Request) {
	email := r.Header.Get(HeaderEmail)
	if email == "" {
		email = "(no auth — local access)"
	}
	setupDone := fileExists(filepath.Join(s.cfg.HomeDir, setupDoneMarker))
	writeJSON(w, http.StatusOK, map[string]any{
		"version":       Version,
		"user":          email,
		"authMode":      s.cfg.AuthMode,
		"allowedUsers":  s.users.List(),
		"guestUser":     s.cfg.Username,
		"homeDir":       s.cfg.HomeDir,
		"stateDir":      s.cfg.StateDir,
		"image":         s.cfg.Image,
		"runtime":       s.cfg.Runtime,
		"dockerDiskGB":  s.cfg.DockerDisk,
		"idleTimeout":   s.cfg.IdleTimeout.String(),
		"setupComplete": setupDone,
	})
}

// addUser authorizes another Google account (settings page).
func (s *Server) addUser(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Email == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("email required"))
		return
	}
	by := r.Header.Get(HeaderEmail)
	if by == "" {
		by = "local"
	}
	if err := s.users.Add(body.Email, by); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"allowedUsers": s.users.List()})
}

func (s *Server) removeUser(w http.ResponseWriter, r *http.Request) {
	if err := s.users.Remove(r.PathValue("email")); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"allowedUsers": s.users.List()})
}
