package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/kesokaj/styrman-crew/internal/backup"
)

// Per-project backups (ADR-044): a tar.gz of the workspace + per-project home
// (minus the reproducible docker disk), stored server-side under
// StateDir/backups/<user>/<projectID>/. Backup works on a RUNNING project
// (crash-consistent, like any file-level backup); restore requires it
// stopped and holds the restoring guard so no boot path can race the
// extraction. This is the USER-level lifecycle layer — host-level snapshots
// of the whole state dir (TODO) remain the disaster-recovery layer.

// backupIDRe: timestamps like 20260830T105512Z — the only accepted id shape
// (it is joined into filesystem paths).
var backupIDRe = regexp.MustCompile(`^[0-9]{8}T[0-9]{6}Z$`)

// backupMeta is the sidecar record listed by the UI.
type backupMeta struct {
	ID          string    `json:"id"`
	ProjectID   string    `json:"projectId"`
	ProjectName string    `json:"projectName"`
	CreatedAt   time.Time `json:"createdAt"`
	SizeBytes   int64     `json:"sizeBytes"`
}

func (s *Server) restoringState(user, projectID string) bool {
	s.deletingMu.Lock()
	defer s.deletingMu.Unlock()
	return s.restoring[user+"/"+projectID]
}

// tryAcquire is a compare-and-set on one of the in-flight maps (audit
// 2026-08-30: a plain store let two concurrent restores both pass every
// check, share one quarantine dir, and destroy each other's rollback).
// Returns false when the slot is already held; release with the closure.
func (s *Server) tryAcquire(m map[string]bool, user, projectID string) (release func(), ok bool) {
	key := user + "/" + projectID
	s.deletingMu.Lock()
	defer s.deletingMu.Unlock()
	if m[key] {
		return nil, false
	}
	m[key] = true
	return func() {
		s.deletingMu.Lock()
		defer s.deletingMu.Unlock()
		delete(m, key)
	}, true
}

// createBackup archives the project now and responds with its metadata.
// Single-flighted per project (two same-second requests would compute the
// SAME timestamp id and interleave writes into one archive) and mutually
// exclusive with an in-flight restore/delete mutating the same tree.
func (s *Server) createBackup(w http.ResponseWriter, r *http.Request) {
	u, _, p, err := s.getProjectFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	release, ok := s.tryAcquire(s.backingUp, u.Username, p.ID)
	if !ok {
		writeErr(w, http.StatusConflict, errors.New("a backup of this project is already running"))
		return
	}
	defer release()
	if s.restoringState(u.Username, p.ID) {
		writeErr(w, http.StatusConflict, errors.New("project is being restored"))
		return
	}
	if inFlight, _ := s.deletingState(u.Username, p.ID); inFlight {
		writeErr(w, http.StatusConflict, errors.New("project is being deleted"))
		return
	}
	dir := s.cfg.ProjectBackups(u.Username, p.ID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		writeErr(w, http.StatusInternalServerError, errors.New("cannot create backup dir"))
		return
	}
	id := time.Now().UTC().Format("20060102T150405Z")
	// Unique temp name (CreateTemp) — belt to the single-flight's braces.
	f, err := os.CreateTemp(dir, id+".tmp-*")
	if err != nil {
		writeErr(w, http.StatusInternalServerError, errors.New("cannot create backup file"))
		return
	}
	tmp := f.Name()
	cerr := backup.Create(f, map[string]string{
		"workspace": s.cfg.ProjectWorkspace(u.Username, p.ID),
		"home":      s.cfg.ProjectHome(u.Username, p.ID),
	})
	if serr := f.Sync(); cerr == nil {
		cerr = serr
	}
	if clerr := f.Close(); cerr == nil {
		cerr = clerr
	}
	if cerr != nil {
		os.Remove(tmp)
		s.logger.Error("backup failed", "user", u.Username, "project", p.ID, "error", cerr)
		writeErr(w, http.StatusInternalServerError, errors.New("backup failed"))
		return
	}
	final := filepath.Join(dir, id+".tar.gz")
	if err := os.Rename(tmp, final); err != nil {
		os.Remove(tmp)
		writeErr(w, http.StatusInternalServerError, errors.New("backup failed"))
		return
	}
	fi, err := os.Stat(final)
	if err != nil {
		// Never dereference a nil FileInfo (audit: this was a panic path);
		// the archive exists — report it with an unknown size.
		s.logger.Warn("backup stat failed", "project", p.ID, "backup", id, "error", err)
	}
	var size int64
	if fi != nil {
		size = fi.Size()
	}
	meta := backupMeta{
		ID: id, ProjectID: p.ID, ProjectName: p.Name,
		CreatedAt: time.Now().UTC(), SizeBytes: size,
	}
	mb, _ := json.MarshalIndent(meta, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, id+".json"), append(mb, '\n'), 0o600); err != nil {
		s.logger.Warn("backup meta write failed", "project", p.ID, "error", err)
	}
	s.logger.Info("backup created", "user", u.Username, "project", p.ID, "backup", id, "bytes", meta.SizeBytes)
	writeJSON(w, http.StatusOK, meta)
}

// readBackups lists one project-backups dir (newest first).
func readBackups(dir string) []backupMeta {
	out := []backupMeta{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return out
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var m backupMeta
		if json.Unmarshal(data, &m) != nil || !backupIDRe.MatchString(m.ID) {
			continue
		}
		// The archive itself must still exist (a half-deleted pair is not
		// restorable — don't list it).
		if _, err := os.Stat(filepath.Join(dir, m.ID+".tar.gz")); err != nil {
			continue
		}
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out
}

// listBackups returns one project's backups.
func (s *Server) listBackups(w http.ResponseWriter, r *http.Request) {
	u, _, p, err := s.getProjectFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"backups": readBackups(s.cfg.ProjectBackups(u.Username, p.ID)),
	})
}

// listAllBackups returns every backup the user owns (Settings overview) —
// including backups of projects that were since deleted (they remain
// restorable into a NEW project via create-from-backup).
func (s *Server) listAllBackups(w http.ResponseWriter, r *http.Request) {
	u, _, err := s.who(r)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	root := s.cfg.UserBackups(u.Username)
	all := []backupMeta{}
	entries, _ := os.ReadDir(root)
	for _, e := range entries {
		if e.IsDir() {
			all = append(all, readBackups(filepath.Join(root, e.Name()))...)
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].ID > all[j].ID })
	writeJSON(w, http.StatusOK, map[string]any{"backups": all})
}

// deleteBackup removes one archive + its meta. Addressed by raw ids under
// /api/settings (NOT getProjectFor): a backup may outlive its project — it
// stays listed and restorable into a new project — so deleting it must not
// require the project record to still exist. Both ids are regexp-validated
// before touching the filesystem; the path is user-scoped by construction.
func (s *Server) deleteBackup(w http.ResponseWriter, r *http.Request) {
	u, _, err := s.who(r)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	projectID, id := r.PathValue("project"), r.PathValue("backup")
	if !projectIDRe.MatchString(projectID) || !backupIDRe.MatchString(id) {
		writeErr(w, http.StatusBadRequest, errors.New("invalid backup reference"))
		return
	}
	dir := s.cfg.ProjectBackups(u.Username, projectID)
	if err := os.Remove(filepath.Join(dir, id+".tar.gz")); err != nil {
		if os.IsNotExist(err) {
			writeErr(w, http.StatusNotFound, errors.New("no such backup"))
			return
		}
		writeErr(w, http.StatusInternalServerError, errors.New("delete failed"))
		return
	}
	os.Remove(filepath.Join(dir, id+".json"))
	s.logger.Info("backup deleted", "user", u.Username, "project", projectID, "backup", id)
	w.WriteHeader(http.StatusNoContent)
}

// restoreBackup replaces the STOPPED project's workspace + home with the
// archive's content. The docker disk (~/.dockerd) is untouched — it is
// reproducible cache and never part of a backup.
func (s *Server) restoreBackup(w http.ResponseWriter, r *http.Request) {
	u, _, p, err := s.getProjectFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	id := r.PathValue("backup")
	if !backupIDRe.MatchString(id) {
		writeErr(w, http.StatusBadRequest, errors.New("invalid backup id"))
		return
	}
	arch := filepath.Join(s.cfg.ProjectBackups(u.Username, p.ID), id+".tar.gz")
	if _, err := os.Stat(arch); err != nil {
		writeErr(w, http.StatusNotFound, errors.New("no such backup"))
		return
	}
	// Acquire the restoring slot FIRST (compare-and-set): a second concurrent
	// restore 409s instead of sharing the quarantine dir and destroying this
	// one's rollback (audit 2026-08-30). Then rule out a concurrent delete or
	// backup of the same tree.
	release, ok := s.tryAcquire(s.restoring, u.Username, p.ID)
	if !ok {
		writeErr(w, http.StatusConflict, errors.New("a restore of this project is already running"))
		return
	}
	defer release()
	if inFlight, _ := s.deletingState(u.Username, p.ID); inFlight {
		writeErr(w, http.StatusConflict, errors.New("project is being deleted"))
		return
	}
	if s.backingUpState(u.Username, p.ID) {
		writeErr(w, http.StatusConflict, errors.New("a backup of this project is running — retry when it finishes"))
		return
	}
	// The wipe+extract runs under the orchestrator's PER-PROJECT LIFECYCLE
	// LOCK — the one EnsureRunning takes — with the stopped-check re-done
	// inside it. The restoring flag alone was a TOCTOU: a boot that had
	// already passed its flag check could mount the home mid-extract (audit
	// 2026-08-30). Boots arriving now either 409 on the flag or block on
	// this lock and then boot the fully-restored tree.
	err = s.orch.WithProjectLock(u.Username, p.ID, func() error {
		st, serr := s.orch.Status(r.Context(), u.Username, p.ID)
		if serr != nil {
			return serr
		}
		if st.State == "running" {
			return errRestoreRunning
		}
		return s.restoreInto(u.Username, p.ID, arch)
	})
	if err != nil {
		if errors.Is(err, errRestoreRunning) {
			writeErr(w, http.StatusConflict, errors.New("stop the project before restoring"))
			return
		}
		s.logger.Error("restore failed", "user", u.Username, "project", p.ID, "backup", id, "error", err)
		writeErr(w, http.StatusInternalServerError, fmt.Errorf("restore failed: %w", err))
		return
	}
	s.logger.Info("backup restored", "user", u.Username, "project", p.ID, "backup", id)
	writeJSON(w, http.StatusOK, map[string]string{"status": "restored", "backup": id})
}

// errRestoreRunning distinguishes the still-running 409 from real failures.
var errRestoreRunning = errors.New("project running")

func (s *Server) backingUpState(user, projectID string) bool {
	s.deletingMu.Lock()
	defer s.deletingMu.Unlock()
	return s.backingUp[user+"/"+projectID]
}

// restoreInto replaces the project's workspace + home (keeping the docker
// disk) with the archive's content — TRANSACTIONALLY. The current state is
// quarantined by rename (never deleted first: an early version cleared the
// workspace, then failed on a root-owned bind-mount stub and left the project
// EMPTY); an extract failure renames everything back. Rename only needs write
// on the PARENT dir, so even root-owned stubs from pre-v1.11.1 dockerd move
// aside cleanly under the unprivileged server. On success the quarantine is
// removed — root-owned residue via the server container's own root
// (RootRemoveContents); if even that is unavailable the trash stays behind
// (server-side only, never mounted into any guest) and is retried by the
// next restore.
func (s *Server) restoreInto(user, projectID, archivePath string) error {
	ws := s.cfg.ProjectWorkspace(user, projectID)
	phome := s.cfg.ProjectHome(user, projectID)
	// Trash lives NEXT to the home (same server-owned parent, same fs so
	// rename works), never inside it (it must not be mounted into the guest).
	trash := phome + ".restore-trash"
	s.clearTrash(trash) // a previous failed run's residue — retry its removal
	trashWS := filepath.Join(trash, "workspace")
	trashHome := filepath.Join(trash, "home")
	if err := os.MkdirAll(trashHome, 0o700); err != nil {
		return fmt.Errorf("prepare quarantine: %w", err)
	}

	// Quarantine phase — renames, fully reversible. A plain rename fails for
	// root-owned DIRS (moving a dir to a new parent rewrites its "..", which
	// needs write ON the dir — dockerd stubs and guest-sudo files are root-
	// owned), so it escalates to a root `mv` via the server's own container.
	move := func(from, to string) error {
		if err := os.Rename(from, to); err == nil {
			return nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		return s.orch.RootRename(ctx, from, to)
	}
	moved := [][2]string{} // {from(original), to(trash)} for rollback
	rollback := func() {
		for i := len(moved) - 1; i >= 0; i-- {
			// Remove whatever the failed extract left at the original path,
			// then put the quarantined original back.
			os.RemoveAll(moved[i][0])
			if err := move(moved[i][1], moved[i][0]); err != nil {
				s.logger.Error("restore rollback rename failed", "from", moved[i][1], "to", moved[i][0], "error", err)
			}
		}
	}
	if _, err := os.Lstat(ws); err == nil {
		if err := move(ws, trashWS); err != nil {
			return fmt.Errorf("quarantine workspace: %w", err)
		}
		moved = append(moved, [2]string{ws, trashWS})
	}
	entries, err := os.ReadDir(phome)
	if err != nil && !os.IsNotExist(err) {
		rollback()
		return fmt.Errorf("read home: %w", err)
	}
	for _, e := range entries {
		if backup.Skip[e.Name()] {
			continue // keep ~/.dockerd — reproducible cache, never archived
		}
		from := filepath.Join(phome, e.Name())
		to := filepath.Join(trashHome, e.Name())
		if err := move(from, to); err != nil {
			rollback()
			return fmt.Errorf("quarantine home entry %s: %w", e.Name(), err)
		}
		moved = append(moved, [2]string{from, to})
	}

	// Extract phase.
	extract := func() error {
		if err := os.MkdirAll(ws, 0o700); err != nil {
			return err
		}
		f, err := os.Open(archivePath)
		if err != nil {
			return err
		}
		defer f.Close()
		return backup.Extract(f, map[string]string{"workspace": ws, "home": phome})
	}
	if err := extract(); err != nil {
		// Purge EVERYTHING the partial extract created before restoring the
		// originals — rollback alone only cleans paths that existed before,
		// so archive-only entries would otherwise survive as orphans mixed
		// into the rolled-back state (audit 2026-08-30). All extracted files
		// are server-written, so plain RemoveAll suffices.
		os.RemoveAll(ws)
		if entries, rerr := os.ReadDir(phome); rerr == nil {
			for _, e := range entries {
				if !backup.Skip[e.Name()] {
					os.RemoveAll(filepath.Join(phome, e.Name()))
				}
			}
		}
		rollback()
		return fmt.Errorf("extract: %w", err)
	}

	// Success — drop the quarantine (best-effort, escalating for root residue).
	s.clearTrash(trash)
	return nil
}

// clearTrash removes a restore-quarantine dir: plain RemoveAll first, then
// root inside the server's own container for stubborn root-owned entries.
// Best-effort by design — leftover trash is retried on the next restore.
func (s *Server) clearTrash(trash string) {
	if _, err := os.Lstat(trash); err != nil {
		return
	}
	if err := os.RemoveAll(trash); err == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := s.orch.RootRemoveContents(ctx, trash); err != nil {
		s.logger.Warn("restore trash kept (will retry next restore)", "path", trash, "error", err)
		return
	}
	if err := os.Remove(trash); err != nil {
		s.logger.Warn("restore trash dir kept", "path", trash, "error", err)
	}
}

// seedFromBackup populates a FRESHLY CREATED project from another project's
// backup (create-from-backup): extract the archive over the new dirs and copy
// the source project's scoped env variables. Called from createProject before
// the background boot; the new project's id differs, so nothing collides.
func (s *Server) seedFromBackup(ctx context.Context, user, newProjectID, srcProjectID, backupID string) error {
	_ = ctx
	if !backupIDRe.MatchString(backupID) {
		return errors.New("invalid backup id")
	}
	arch := filepath.Join(s.cfg.ProjectBackups(user, srcProjectID), backupID+".tar.gz")
	if _, err := os.Stat(arch); err != nil {
		return errors.New("no such backup")
	}
	if err := s.restoreInto(user, newProjectID, arch); err != nil {
		return err
	}
	// Project-scoped env vars belong to the environment being cloned.
	if s.creds != nil {
		if vars, err := s.creds.ProjectEnv(user, srcProjectID); err == nil {
			for k, v := range vars {
				if err := s.creds.PutProjectEnv(user, newProjectID, k, v); err != nil {
					s.logger.Warn("env copy failed during create-from-backup", "key", k, "error", err)
				}
			}
		}
	}
	return nil
}
