package httpapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/kesokaj/styrman/internal/files"
	"github.com/kesokaj/styrman/internal/history"
	"github.com/kesokaj/styrman/internal/identity"
)

// rootFor resolves the file-explorer root for a scope: a project's workspace
// or the requesting user's home.
func (s *Server) rootFor(r *http.Request) (string, error) {
	u, sp, err := s.who(r)
	if err != nil {
		return "", err
	}
	if id := r.PathValue("project"); id != "" {
		p, err := sp.projects.Get(id)
		if err != nil {
			return "", err
		}
		return filepath.Join(s.cfg.UserHome(u.Username), "projects", p.Dir), nil
	}
	return s.cfg.UserHome(u.Username), nil
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

// uploadFile accepts multipart form uploads (drag & drop) into ?path= (a
// directory); each part keeps its filename. Binary-safe, 512 MB cap via the
// csrf guard's MaxBytesReader.
func (s *Server) uploadFile(w http.ResponseWriter, r *http.Request) {
	root, err := s.rootFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	dir := r.URL.Query().Get("path")
	mr, err := r.MultipartReader()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	var saved []string
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		name := filepath.Base(part.FileName())
		if name == "" || name == "." || name == "/" {
			continue
		}
		full, err := files.Resolve(root, filepath.Join(dir, name))
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		f, err := os.Create(full)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		if _, err := io.Copy(f, part); err != nil {
			f.Close()
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		f.Close()
		saved = append(saved, name)
	}
	writeJSON(w, http.StatusOK, map[string]any{"saved": saved})
}

// downloadFile streams a raw file (any type) as an attachment.
func (s *Server) downloadFile(w http.ResponseWriter, r *http.Request) {
	root, err := s.rootFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	full, err := files.Resolve(root, r.URL.Query().Get("path"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	w.Header().Set("Content-Disposition", "attachment; filename=\""+filepath.Base(full)+"\"")
	http.ServeFile(w, r, full)
}

// listHistory returns the project's terminal transcripts.
func (s *Server) listHistory(w http.ResponseWriter, r *http.Request) {
	u, _, p, err := s.getProjectFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	items, err := history.List(s.cfg.UserHome(u.Username), p.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"transcripts": items})
}

func (s *Server) readHistory(w http.ResponseWriter, r *http.Request) {
	u, _, p, err := s.getProjectFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	max, _ := strconv.ParseInt(r.URL.Query().Get("bytes"), 10, 64)
	text, err := history.Read(s.cfg.UserHome(u.Username), p.ID, r.PathValue("id"), max)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": r.PathValue("id"), "text": text})
}

func (s *Server) projectDiff(w http.ResponseWriter, r *http.Request) {
	u, _, p, err := s.getProjectFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	d, err := history.GitDiff(filepath.Join(s.cfg.UserHome(u.Username), "projects", p.Dir))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

// settings reports everything the settings page shows.
func (s *Server) settings(w http.ResponseWriter, r *http.Request) {
	u, _, err := s.who(r)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	email := u.Email
	if email == "" {
		email = "(local operator)"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"version":       Version,
		"user":          email,
		"username":      u.Username,
		"isAdmin":       u.Admin,
		"authMode":      s.cfg.AuthMode,
		"allowedUsers":  s.users.List(),
		"homeDir":       s.cfg.UserHome(u.Username),
		"stateDir":      s.cfg.StateDir,
		"image":         s.cfg.Image,
		"runtime":       s.cfg.Runtime,
		"dockerDiskGB":  s.cfg.DockerDisk,
		"idleTimeout":   s.cfg.IdleTimeout.String(),
		"terminalUsername": s.guestUserFor(u.Username),
		"setupComplete": s.userSetupStatus(r.Context(), u.Username).Done,
	})
}

// addUser authorizes another Google account (admin only). The new user gets
// their own home and projects on first sign-in.
func (s *Server) addUser(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	var body struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Email == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("email required"))
		return
	}
	by := identity.FromContext(r.Context()).Email
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
	if !s.requireAdmin(w, r) {
		return
	}
	if err := s.users.Remove(r.PathValue("email")); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"allowedUsers": s.users.List()})
}
