package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kesokaj/styrman-crew/internal/files"
	"github.com/kesokaj/styrman-crew/internal/history"
	"github.com/kesokaj/styrman-crew/internal/identity"
)

// scopeFor resolves the file-explorer scope. The root is ALWAYS the user's
// home (a real, server-created directory) so os.Root confinement holds; the
// prefix narrows it to a project workspace. A guest-planted symlink at
// projects/<dir> can therefore never escape the home (fixes the pre-os.Root
// cross-tenant read/write via a symlinked workspace root).
func (s *Server) scopeFor(r *http.Request) (root, prefix string, err error) {
	u, sp, err := s.who(r)
	if err != nil {
		return "", "", err
	}
	root = s.cfg.UserHome(u.Username)
	if id := r.PathValue("project"); id != "" {
		p, perr := sp.projects.Get(id)
		if perr != nil {
			return "", "", perr
		}
		prefix = path.Join("projects", p.Dir)
	}
	return root, prefix, nil
}

// scopedPath joins the request's ?path= onto the scope prefix.
func scopedPath(prefix, rel string) string {
	return path.Join(prefix, strings.TrimPrefix(path.Clean("/"+rel), "/"))
}

func (s *Server) listFiles(w http.ResponseWriter, r *http.Request) {
	root, prefix, err := s.scopeFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	entries, err := files.List(root, scopedPath(prefix, r.URL.Query().Get("path")))
	if err != nil {
		writeErr(w, http.StatusBadRequest, errors.New("cannot list path"))
		return
	}
	// Report paths relative to the SCOPE (what the SPA sends back as ?path=),
	// not the home root — else the project prefix is applied twice on drill-in.
	if prefix != "" {
		for i := range entries {
			entries[i].Path = strings.TrimPrefix(entries[i].Path, prefix+"/")
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": entries})
}

func (s *Server) readFile(w http.ResponseWriter, r *http.Request) {
	root, prefix, err := s.scopeFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	fc, err := files.Read(root, scopedPath(prefix, r.URL.Query().Get("path")))
	if err != nil {
		writeErr(w, http.StatusBadRequest, errors.New("cannot read file"))
		return
	}
	writeJSON(w, http.StatusOK, fc)
}

func (s *Server) writeFile(w http.ResponseWriter, r *http.Request) {
	root, prefix, err := s.scopeFor(r)
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
	if err := files.Write(root, scopedPath(prefix, r.URL.Query().Get("path")), body.Content); err != nil {
		writeErr(w, http.StatusBadRequest, errors.New("cannot write file"))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// uploadFile accepts multipart form uploads (drag & drop) into ?path= (a
// directory); each part keeps its filename. Binary-safe, 512 MB cap via the
// csrf guard's MaxBytesReader.
func (s *Server) uploadFile(w http.ResponseWriter, r *http.Request) {
	root, prefix, err := s.scopeFor(r)
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
		if err := files.SaveStream(root, scopedPath(prefix, path.Join(dir, name)), part); err != nil {
			writeErr(w, http.StatusInternalServerError, errors.New("cannot save upload"))
			return
		}
		saved = append(saved, name)
	}
	writeJSON(w, http.StatusOK, map[string]any{"saved": saved})
}

// downloadFile streams a raw file (any type) as an attachment.
func (s *Server) downloadFile(w http.ResponseWriter, r *http.Request) {
	root, prefix, err := s.scopeFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	rel := scopedPath(prefix, r.URL.Query().Get("path"))
	f, st, err := files.OpenForDownload(root, rel)
	if err != nil {
		writeErr(w, http.StatusBadRequest, errors.New("cannot open file"))
		return
	}
	defer f.Close()
	w.Header().Set("Content-Disposition",
		"attachment; filename=\""+strings.ReplaceAll(path.Base(rel), "\"", "")+"\"")
	http.ServeContent(w, r, path.Base(rel), st.ModTime(), f)
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
	// Resolve through the symlink-containment guard: a guest could point its
	// workspace at another user's home (same uid on the shared mount).
	ws, err := s.workspacePath(u.Username, p)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, errors.New("workspace unavailable"))
		return
	}
	d, err := history.GitDiff(ws)
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
		"version":          Version,
		"user":             email,
		"username":         u.Username,
		"isAdmin":          u.Admin,
		"authMode":         s.cfg.AuthMode,
		"allowedUsers":     s.users.List(),
		"homeDir":          s.cfg.UserHome(u.Username),
		"stateDir":         s.cfg.StateDir,
		"image":            s.cfg.Image,
		"runtime":          s.cfg.Runtime,
		"dockerDiskGB":     s.cfg.DockerDisk,
		"idleTimeout":      s.cfg.IdleTimeout.String(),
		"terminalUsername": s.guestUserFor(u.Username),
		// Whether the user explicitly picked a terminal username (vs. the
		// email-derived default) — the setup flow skips the picker if so.
		"terminalUsernameChosen": s.usernameChosen(u.Username),
		"setupComplete":          s.userSetupStatus(r.Context(), u.Username).Done,
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

// notification is one "a terminal wants you" entry.
type notification struct {
	ProjectID   string `json:"projectId"`
	ProjectName string `json:"projectName"`
	TerminalID  string `json:"terminalId"`
	Title       string `json:"title"`
}

// listNotifications scans the user's running projects for terminals that
// rang the bell while unattended (agents waiting for input). One tmux exec
// per running project per poll — bounded so a wedged microVM can't stall
// the endpoint past the UI's poll interval.
func (s *Server) listNotifications(w http.ResponseWriter, r *http.Request) {
	u, sp, err := s.who(r)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
	defer cancel()
	guest := sp.profile.GuestUser()
	out := []notification{}
	for _, p := range sp.projects.List() {
		st, err := s.orch.Status(ctx, u.Username, p.ID)
		if err != nil {
			s.logger.Warn("notifications: status failed", "project", p.ID, "error", err)
			continue
		}
		if st.State != "running" {
			continue
		}
		live, err := s.tmux.Live(ctx, u.Username, p.ID, guest)
		if err != nil {
			s.logger.Warn("notifications: tmux scan failed", "project", p.ID, "error", err)
			continue
		}
		for id, l := range live {
			if !l.Bell {
				continue
			}
			title := "Terminal " + id
			if rec, ok := sp.terms.Get(p.ID, id); ok {
				title = rec.Title
			}
			out = append(out, notification{
				ProjectID: p.ID, ProjectName: p.Name, TerminalID: id, Title: title,
			})
		}
	}
	// Map iteration order is random — keep the dropdown stable across polls.
	sort.Slice(out, func(i, j int) bool {
		if out[i].ProjectName != out[j].ProjectName {
			return out[i].ProjectName < out[j].ProjectName
		}
		return out[i].TerminalID < out[j].TerminalID
	})
	writeJSON(w, http.StatusOK, map[string]any{"notifications": out})
}
