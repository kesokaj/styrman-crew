package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kesokaj/styrman-crew/internal/conversation"
	"github.com/kesokaj/styrman-crew/internal/credstore"
	"github.com/kesokaj/styrman-crew/internal/files"
	"github.com/kesokaj/styrman-crew/internal/history"
	"github.com/kesokaj/styrman-crew/internal/identity"
	"github.com/kesokaj/styrman-crew/internal/orchestrator"
	"github.com/kesokaj/styrman-crew/internal/project"
	"github.com/kesokaj/styrman-crew/internal/updatestatus"
)

// scopeFor resolves the file-explorer scope for a project route. Both roots
// are real, server-created directories (os.Root confinement holds): the
// "home" route family (pattern contains "/home/") resolves to the project's
// OWN home (ADR-031); otherwise it's the project's workspace (ADR-027). Both
// are per-project since v1.4, so a guest symlink can't reach another
// project, let alone another tenant.
func (s *Server) scopeFor(r *http.Request) (root string, err error) {
	u, sp, err := s.who(r)
	if err != nil {
		return "", err
	}
	id := r.PathValue("project")
	if id == "" {
		return "", errors.New("project required")
	}
	p, perr := sp.projects.Get(id)
	if perr != nil {
		return "", perr
	}
	if strings.Contains(r.Pattern, "/home/") {
		return s.cfg.ProjectHome(u.Username, p.ID), nil
	}
	return s.cfg.ProjectWorkspace(u.Username, p.ID), nil
}

func (s *Server) listFiles(w http.ResponseWriter, r *http.Request) {
	root, err := s.scopeFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	// Paths in and out are scope-relative: since ADR-027 the scope root IS
	// the workspace (or home), so nothing needs prefix stripping anymore.
	lst, err := files.List(root, r.URL.Query().Get("path"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, errors.New("cannot list path"))
		return
	}
	// The formatter's transient temp file (format.go) lives beside the file
	// being formatted for a fraction of a second; never show it.
	lst.Entries = slices.DeleteFunc(lst.Entries, func(e files.Entry) bool { return strings.HasPrefix(e.Name, ".styrman-fmt-") })
	// Listing carries Truncated beside entries: the listing is capped since
	// 2026-09-22, and a cut directory must not read as a complete one.
	writeJSON(w, http.StatusOK, lst)
}

func (s *Server) readFile(w http.ResponseWriter, r *http.Request) {
	root, err := s.scopeFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	// The echoed path is scope-relative and round-trips into PUT unchanged
	// (regression-tested — a prefix mismatch here once broke every save).
	fc, err := files.Read(root, r.URL.Query().Get("path"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, errors.New("cannot read file"))
		return
	}
	writeJSON(w, http.StatusOK, fc)
}

// scopeRel normalizes a scope-relative ?path= and refuses the scope root
// itself — delete/rename must never target the workspace or home root.
func scopeRel(rel string) (string, error) {
	rel = strings.TrimPrefix(path.Clean("/"+strings.TrimSpace(rel)), "/")
	if rel == "" {
		return "", errors.New("path required")
	}
	return rel, nil
}

// managedHomeTarget refuses destructive operations on platform-managed
// subtrees in the HOME scope: ~/projects holds the workspace mount points,
// ~/.dockerd the loop-mounted docker disks of possibly-RUNNING microVMs,
// ~/.styrman the transcripts + apt manifests, ~/.ssh the set-up-once-ever
// identity (R3). Project scope is confined to its own workspace dir
// (ADR-027), where these names are ordinary user dirs — so the guard keys
// on the SCOPE, not the path shape.
//
// The name match alone is not a boundary, because os.Root follows symlinks
// that stay inside the root: with `ln -s .dockerd x`, deleting `x/disk.img`
// reached a managed subtree through an unmanaged name (probed 2026-09-22).
// Nothing legitimate reaches THROUGH a link in the home scope, so any path
// that traverses one is refused rather than resolved. Acting on the link
// ITSELF is not refused: removing or renaming a bad symlink is ordinary
// housekeeping, it touches only the link, and TestFileDeleteSymlinkEscape
// requires it.
func managedHomeTarget(root string, isHome bool, rel string) bool {
	if !isHome {
		return false
	}
	first, _, _ := strings.Cut(rel, "/")
	switch first {
	case "projects", ".dockerd", ".styrman", ".ssh":
		return true
	}
	return traversesALink(root, rel)
}

// traversesALink reports whether any component of rel EXCEPT the last is a
// symlink. The depth matters: a guard that looked only at the first component
// was blind to `work/sub/link -> ../../.dockerd`, where the first component
// is a real directory and os.Root follows the link anyway (code review,
// 2026-09-22). The last component is deliberately not inspected — that is the
// link itself, and acting on it touches only the link.
//
// It answers "yes, treat as managed" whenever it cannot tell: this stands in
// front of a delete, and a guard whose own check errors must keep the data.
// A component that does not exist is the one exception — it is no subtree at
// all, and the operation behind this guard fails on its own.
//
// What this is NOT: a tenancy boundary. That is os.Root's, it is atomic, and
// it holds regardless of anything here. This lstat runs on a different handle
// from the one the operation later opens, so a guest can swap a real
// directory for a link in between and win the race — against its OWN home,
// as the same principal, which is the whole of what it gains (security
// review 2026-09-22). Closing the race would mean carrying one root handle
// from check to operation through every files.* verb; it is not worth that
// while the only thing on the other side of it is the tenant itself.
func traversesALink(root, rel string) bool {
	parts := strings.Split(rel, "/")
	if len(parts) < 2 {
		return false
	}
	rt, err := files.OpenRoot(root)
	if err != nil {
		return true
	}
	defer rt.Close()
	for i := 1; i < len(parts); i++ {
		prefix := strings.Join(parts[:i], "/")
		if prefix == "" || prefix == "." {
			continue
		}
		st, err := rt.Lstat(prefix)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return false
			}
			return true
		}
		if st.Mode()&fs.ModeSymlink != 0 {
			return true
		}
	}
	return false
}

func (s *Server) deleteFile(w http.ResponseWriter, r *http.Request) {
	root, err := s.scopeFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	rel, err := scopeRel(r.URL.Query().Get("path"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if managedHomeTarget(root, strings.Contains(r.Pattern, "/home/"), rel) {
		writeErr(w, http.StatusBadRequest, errors.New("path is managed by styrman"))
		return
	}
	if err := files.RemoveAll(root, rel); err != nil {
		s.logger.Warn("file delete failed", "project", r.PathValue("project"), "path", rel, "error", err)
		writeErr(w, http.StatusBadRequest, errors.New("cannot delete path"))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) renameFile(w http.ResponseWriter, r *http.Request) {
	root, err := s.scopeFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	from, err := scopeRel(r.URL.Query().Get("path"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	var body struct {
		To string `json:"to"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	to, err := scopeRel(body.To)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	isHome := strings.Contains(r.Pattern, "/home/")
	if managedHomeTarget(root, isHome, from) || managedHomeTarget(root, isHome, to) {
		writeErr(w, http.StatusBadRequest, errors.New("path is managed by styrman"))
		return
	}
	if err := files.Rename(root, from, to); err != nil {
		s.logger.Warn("file rename failed", "project", r.PathValue("project"), "from", from, "to", to, "error", err)
		msg := "cannot rename path"
		if strings.Contains(err.Error(), "destination already exists") {
			msg = "destination already exists" // user-actionable, worth surfacing
		}
		writeErr(w, http.StatusBadRequest, errors.New(msg))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) makeDir(w http.ResponseWriter, r *http.Request) {
	root, err := s.scopeFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	rel, err := scopeRel(r.URL.Query().Get("path"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	// Same managed-subtree guard as delete/rename (audit finding: the write
	// paths were unguarded, letting a home-scope write clobber e.g. the
	// loop-mounted docker disk of a RUNNING microVM).
	if managedHomeTarget(root, strings.Contains(r.Pattern, "/home/"), rel) {
		writeErr(w, http.StatusBadRequest, errors.New("path is managed by styrman"))
		return
	}
	if err := files.MkdirAll(root, rel); err != nil {
		s.logger.Warn("mkdir failed", "project", r.PathValue("project"), "path", rel, "error", err)
		writeErr(w, http.StatusBadRequest, errors.New("cannot create folder"))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) writeFile(w http.ResponseWriter, r *http.Request) {
	root, err := s.scopeFor(r)
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
	rel, err := scopeRel(r.URL.Query().Get("path"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	// Managed-subtree guard on the write path too (audit finding): without
	// it, PUT home/file?path=.dockerd/<uuid>.img O_TRUNCs the loop-mounted
	// docker disk of a possibly-running microVM.
	if managedHomeTarget(root, strings.Contains(r.Pattern, "/home/"), rel) {
		writeErr(w, http.StatusBadRequest, errors.New("path is managed by styrman"))
		return
	}
	// ?create=true is create-only: the kernel refuses if the name is taken.
	// The UI's own "name already exists" check is client-side and TOCTOU
	// against the last-loaded listing, so without this a raw API caller — or
	// two people picking the same name at once — could truncate an existing
	// file through this endpoint (v1.17.2 review).
	if r.URL.Query().Get("create") == "true" {
		if err := files.Create(root, rel, body.Content); err != nil {
			if errors.Is(err, files.ErrExists) {
				writeErr(w, http.StatusConflict, errors.New("a file with that name already exists"))
				return
			}
			writeErr(w, http.StatusBadRequest, errors.New("cannot create file"))
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err := files.Write(root, rel, body.Content); err != nil {
		writeErr(w, http.StatusBadRequest, errors.New("cannot write file"))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// uploadFile accepts multipart form uploads (drag & drop) into ?path= (a
// directory); each part keeps its filename. Binary-safe, 512 MB cap via the
// csrf guard's MaxBytesReader.
func (s *Server) uploadFile(w http.ResponseWriter, r *http.Request) {
	root, err := s.scopeFor(r)
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
	// Non-nil from the start: the client iterates it, and it is reported on
	// EVERY exit — a mid-stream failure must tell the client which earlier
	// parts already landed, so panes holding those files get evicted and a
	// stale buffer cannot overwrite the upload on Save (code review 2026-09-02).
	saved := []string{}
	// uploadErr mirrors writeErr's JSON shape (minus its ErrNotFound remap —
	// every error passed here is a static message) plus the parts saved so far.
	uploadErr := func(status int, err error) {
		writeJSON(w, status, map[string]any{
			"error": map[string]string{"message": err.Error()},
			"saved": saved,
		})
	}
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			// The branch an oversized drop actually hits (the 512 MiB cap trips
			// inside the boundary scan between parts) — earlier parts landed,
			// so this exit reports them too (code review round 4).
			uploadErr(http.StatusBadRequest, err)
			return
		}
		name := filepath.Base(part.FileName())
		if name == "" || name == "." || name == "/" {
			continue
		}
		target := strings.TrimPrefix(path.Clean("/"+path.Join(dir, name)), "/")
		// Managed-subtree guard per part (audit finding): an upload into
		// ~/.dockerd or ~/.ssh must be refused like delete/rename/write.
		if managedHomeTarget(root, strings.Contains(r.Pattern, "/home/"), target) {
			uploadErr(http.StatusBadRequest, errors.New("path is managed by styrman"))
			return
		}
		if err := files.SaveStream(root, target, part); err != nil {
			uploadErr(http.StatusInternalServerError, errors.New("cannot save upload"))
			return
		}
		saved = append(saved, name)
	}
	writeJSON(w, http.StatusOK, map[string]any{"saved": saved})
}

// downloadFile streams a raw file (any type) as an attachment.
func (s *Server) downloadFile(w http.ResponseWriter, r *http.Request) {
	root, err := s.scopeFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	rel := strings.TrimPrefix(path.Clean("/"+r.URL.Query().Get("path")), "/")
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

// transcriptView joins a transcript file with its terminal's friendly title
// (the file is named after the tmux session, conv-<terminal id>).
type transcriptView struct {
	history.Transcript
	Title string `json:"title,omitempty"`
}

// transcriptTitles maps transcript id (conv-<n>) → the terminal's friendly
// title, for terminals whose record still exists (deleted ones keep only
// the raw id — the transcript deliberately outlives the record).
func (s *Server) transcriptTitles(sp *userSpace, projectID string) map[string]string {
	titles := map[string]string{}
	for _, rec := range sp.terms.List(projectID) {
		titles[conversation.Prefix+rec.ID] = rec.Title
	}
	return titles
}

// listHistory returns the project's terminal transcripts.
func (s *Server) listHistory(w http.ResponseWriter, r *http.Request) {
	u, sp, p, err := s.getProjectFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	items, err := history.List(s.cfg.ProjectHome(u.Username, p.ID), p.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	titles := s.transcriptTitles(sp, p.ID)
	out := make([]transcriptView, 0, len(items))
	for _, t := range items {
		out = append(out, transcriptView{Transcript: t, Title: titles[t.ID]})
	}
	writeJSON(w, http.StatusOK, map[string]any{"transcripts": out})
}

// searchConcurrency is how many history searches the whole server runs at
// once. A search holds several MiB per transcript (the 4 MiB tail and its
// cleaned, lowered copies); under the server's 4g memory cap (ADR-085),
// hundreds in flight against guest-planted transcripts would OOM-kill the
// server for every tenant (security review of v1.42.1, finding 1).
const searchConcurrency = 4

// searchHistory greps all of the project's transcripts (agent sessions
// included — they're raw PTY captures) for ?q=, case-insensitive. Past
// searchConcurrency searches in flight it answers 429 rather than queue: a
// queue would hold the requests, and their memory, anyway.
func (s *Server) searchHistory(w http.ResponseWriter, r *http.Request) {
	u, sp, p, err := s.getProjectFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	q := r.URL.Query().Get("q")
	if len(q) > 256 {
		writeErr(w, http.StatusBadRequest, errors.New("query too long"))
		return
	}
	select {
	case s.searchSlots <- struct{}{}:
		defer func() { <-s.searchSlots }()
	default:
		writeErr(w, http.StatusTooManyRequests, errors.New("too many history searches in progress — try again in a moment"))
		return
	}
	matches, err := history.Search(r.Context(), s.cfg.ProjectHome(u.Username, p.ID), p.ID, q)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	titles := s.transcriptTitles(sp, p.ID)
	type matchView struct {
		history.Match
		Title string `json:"title,omitempty"`
	}
	out := make([]matchView, 0, len(matches))
	for _, m := range matches {
		out = append(out, matchView{Match: m, Title: titles[m.Transcript]})
	}
	writeJSON(w, http.StatusOK, map[string]any{"matches": out})
}

func (s *Server) readHistory(w http.ResponseWriter, r *http.Request) {
	u, _, p, err := s.getProjectFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	max, _ := strconv.ParseInt(r.URL.Query().Get("bytes"), 10, 64)
	// Clamp: history.Read allocates maxBytes up front, so an unbounded
	// ?bytes= would attempt an arbitrary-size allocation. Negative falls
	// back to history.Read's 256 KiB default.
	if max < 0 {
		max = 0
	} else if max > 4<<20 {
		max = 4 << 20
	}
	text, err := history.Read(s.cfg.ProjectHome(u.Username, p.ID), p.ID, r.PathValue("id"), max)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": r.PathValue("id"), "text": text})
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
	// allowedUsers is deliberately NOT in this payload anymore: the runtime
	// user-management UI was removed (operator Q4, 2026-08-28) — access is
	// managed via STYRMAN_ALLOWED_EMAILS plus the admin-only /settings/users
	// endpoints, and the list shouldn't leak to non-admin sessions.
	resp := map[string]any{
		"version":          Version,
		"user":             email,
		"username":         u.Username,
		"isAdmin":          u.Admin,
		"authMode":         s.cfg.AuthMode,
		"homeDir":          s.cfg.UserHome(u.Username),
		"stateDir":         s.cfg.StateDir,
		"image":            s.cfg.Image,
		"runtime":          s.cfg.Runtime,
		"dockerDiskGB":     s.cfg.DockerDiskGB(),
		"idleTimeout":      s.cfg.IdleTimeoutD().String(),
		"terminalUsername": s.guestUserFor(u.Username),
		// Whether the user explicitly picked a terminal username (vs. the
		// email-derived default) — the setup flow skips the picker if so.
		"terminalUsernameChosen": s.usernameChosen(u.Username),
		"setupComplete":          s.userSetupStatus(r.Context(), u.Username).Done,
	}
	if st, ok := updaterFor(u.Admin, s.cfg.UpdateStatusFile, time.Now()); ok {
		resp["updater"] = st
	}
	writeJSON(w, http.StatusOK, resp)
}

// updaterFor is the auto-updater's last recorded result, for an ADMIN only
// (ADR-081): which release the host runs and why its updater is failing is
// operator information. ok is false — no key in the payload at all — for
// everyone else, when the feature is off (no path), and when the host has no
// status file. Pure but for the read, so the gate is tested without a
// second identity.
func updaterFor(admin bool, path string, now time.Time) (updatestatus.Status, bool) {
	if !admin {
		return updatestatus.Status{}, false
	}
	return updatestatus.Read(path, now)
}

// updateRuntime changes the operator-tunable settings (admin only): idle
// timeout applies to the reaper immediately; docker disk applies to newly
// created project VMs. Persisted in StateDir/settings.json.
func (s *Server) updateRuntime(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	var body struct {
		DockerDiskGB int    `json:"dockerDiskGB"`
		IdleTimeout  string `json:"idleTimeout"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	// Validation → 400 here; anything SetOperator returns after this is a
	// persistence failure → 500 (disk full is not the client's fault).
	if body.DockerDiskGB != 0 && (body.DockerDiskGB < 5 || body.DockerDiskGB > 500) {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("docker disk must be 5-500 GB"))
		return
	}
	idle := time.Duration(-1) // keep
	if body.IdleTimeout != "" {
		d, err := time.ParseDuration(body.IdleTimeout)
		if err != nil || d < 0 || (d > 0 && d < 10*time.Minute) {
			writeErr(w, http.StatusBadRequest, fmt.Errorf("idle timeout must be 0 (never) or at least 10m"))
			return
		}
		idle = d
	}
	if err := s.cfg.SetOperator(body.DockerDiskGB, idle); err != nil {
		s.logger.Error("runtime settings persist failed", "error", err)
		writeErr(w, http.StatusInternalServerError, errors.New("cannot persist settings"))
		return
	}
	s.logger.Info("runtime settings changed",
		"by", identity.FromContext(r.Context()).Email,
		"dockerDiskGB", s.cfg.DockerDiskGB(),
		"idleTimeout", s.cfg.IdleTimeoutD().String())
	writeJSON(w, http.StatusOK, map[string]any{
		"dockerDiskGB": s.cfg.DockerDiskGB(),
		"idleTimeout":  s.cfg.IdleTimeoutD().String(),
	})
}

// hostnameRe bounds an onboarding host to safe characters (it reaches a
// login-shell command). Letters, digits, dot, hyphen only.
var hostnameRe = regexp.MustCompile(`^[A-Za-z0-9.-]+$`)

// importGitCredential onboards the caller's CURRENT GitHub token into the
// credential broker store (ADR-030) by running `gh auth token` inside one of
// the user's RUNNING projects and storing the result once; thereafter the
// broker serves it to every project. Body: {"project": "<id>"}
// names the running project to extract from; "host" optional (default
// github.com).
func (s *Server) importGitCredential(w http.ResponseWriter, r *http.Request) {
	u, sp, err := s.who(r)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if s.creds == nil {
		writeErr(w, http.StatusServiceUnavailable, errors.New("credential broker disabled"))
		return
	}
	var body struct {
		Project string `json:"project"`
		Host    string `json:"host"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Project == "" {
		writeErr(w, http.StatusBadRequest, errors.New("project required"))
		return
	}
	if body.Host == "" {
		body.Host = "github.com"
	}
	// The host is interpolated into a login-shell command below (the login
	// shell materializes gh's broker credentials via /etc/profile.d,
	// ADR-032), so validate it strictly.
	if !hostnameRe.MatchString(body.Host) {
		writeErr(w, http.StatusBadRequest, errors.New("invalid host"))
		return
	}
	p, err := sp.projects.Get(body.Project)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	st, err := s.guest.Status(r.Context(), u.Username, p.ID)
	if err != nil || st.State != "running" {
		writeErr(w, http.StatusConflict, errors.New("name a RUNNING project to import from"))
		return
	}
	guest := s.guestUserFor(u.Username)
	// Login shell so gh's config/PATH are set up; skip the broker cred-pull so
	// we read the project's REAL gh token, not the broker's own copy (H6).
	code, out, err := s.guest.Exec(r.Context(), st.ContainerID, orchestrator.ExecSpec{
		User: guest,
		Env:  []string{"HOME=/home/" + guest, "USER=" + guest, "STYRMAN_SKIP_CRED_PULL=1"},
		Cmd:  []string{"bash", "-lc", "gh auth token --hostname " + body.Host + " 2>/dev/null"},
	})
	token := strings.TrimSpace(out)
	if err != nil || code != 0 || token == "" {
		writeErr(w, http.StatusBadRequest, errors.New("could not read a gh token in that project — is gh signed in?"))
		return
	}
	// gh HTTPS auth uses any username with the token as the password;
	// "x-access-token" is the conventional placeholder.
	if err := s.creds.PutGit(u.Username, credstore.GitCred{Host: body.Host, Username: "x-access-token", Token: token}); err != nil {
		writeErr(w, http.StatusInternalServerError, errors.New("could not store credential"))
		return
	}
	s.logger.Info("git credential imported to broker store", "user", u.Username, "host", body.Host)
	writeJSON(w, http.StatusOK, map[string]string{"host": body.Host, "status": "stored"})
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
	// 5s memo per user: several UI surfaces poll this, and each scan execs
	// into every running microVM (v1.14.0 polish).
	if v, ok := s.notifCache.Load(u.Username); ok {
		if c := v.(notifCacheEntry); time.Since(c.at) < 5*time.Second {
			writeJSON(w, http.StatusOK, map[string]any{"notifications": c.items})
			return
		}
	}
	out := []notification{}
	// Concurrent fan-out (one goroutine per project, LiveIn skips the
	// redundant per-scan Status) instead of the old sequential exec chain.
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, p := range sp.projects.List() {
		wg.Add(1)
		go func(p project.Project) {
			defer wg.Done()
			st, err := s.guest.Status(ctx, u.Username, p.ID)
			if err != nil {
				s.logger.Warn("notifications: status failed", "project", p.ID, "error", err)
				return
			}
			if st.State != "running" {
				return
			}
			live, err := s.tmux.LiveIn(ctx, st.ContainerID, guest)
			if err != nil {
				s.logger.Warn("notifications: tmux scan failed", "project", p.ID, "error", err)
				return
			}
			mu.Lock()
			defer mu.Unlock()
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
		}(p)
	}
	wg.Wait()
	// Map iteration order is random — keep the dropdown stable across polls.
	sort.Slice(out, func(i, j int) bool {
		if out[i].ProjectName != out[j].ProjectName {
			return out[i].ProjectName < out[j].ProjectName
		}
		return out[i].TerminalID < out[j].TerminalID
	})
	s.notifCache.Store(u.Username, notifCacheEntry{at: time.Now(), items: out})
	writeJSON(w, http.StatusOK, map[string]any{"notifications": out})
}
