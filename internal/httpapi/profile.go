package httpapi

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sync"

	"github.com/kesokaj/styrman-crew/internal/identity"
)

// profileStore holds per-user preferences — today just the in-guest
// ("terminal") username, chosen at first setup or in Settings. It defaults
// to the identity slug and applies to microVMs started after a change.
// GuestName is read on every terminal attach and written from Settings, so a
// mutex guards it (a torn string is memory-unsafe, not merely stale).
type profileStore struct {
	path      string
	def       string
	mu        sync.RWMutex
	GuestName string `json:"guestUser,omitempty"`
}

var guestNameRe = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)

// reservedNames are system accounts that already exist in the image at a
// non-1000 uid; picking one makes useradd a no-op and the guest run as the
// wrong uid on a home it can't write (the entrypoint also fails closed).
var reservedNames = map[string]bool{
	"root": true, "daemon": true, "bin": true, "sys": true, "sync": true,
	"games": true, "man": true, "lp": true, "mail": true, "news": true,
	"proxy": true, "www-data": true, "backup": true, "list": true,
	"nobody": true, "docker": true, "sshd": true, "systemd-network": true,
	"messagebus": true, "sudo": true, "adm": true, "tty": true, "disk": true,
}

func newProfileStore(path, defaultUser string) (*profileStore, error) {
	p := &profileStore{path: path, def: defaultUser}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return p, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(data, p); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if p.GuestName != "" && !guestNameRe.MatchString(p.GuestName) {
		p.GuestName = "" // hand-edited garbage must not reach useradd
	}
	return p, nil
}

// GuestUser returns the effective in-guest username.
func (p *profileStore) GuestUser() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.GuestName != "" {
		return p.GuestName
	}
	return p.def
}

// HasGuestUser reports whether a username was explicitly chosen.
func (p *profileStore) HasGuestUser() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.GuestName != ""
}

func (p *profileStore) setGuestUser(name string) error {
	if name != "" && !guestNameRe.MatchString(name) {
		return fmt.Errorf("invalid username %q: lowercase letters, digits, - and _, starting with a letter", name)
	}
	if reservedNames[name] {
		return fmt.Errorf("%q is a reserved system name — pick another", name)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	// Rollback-on-failure (audit finding): a failed write must not leave the
	// live process serving a guest username that disk doesn't have — same
	// discipline as config.SetOperator and the links store.
	old := p.GuestName
	p.GuestName = name
	if err := p.writeLocked(); err != nil {
		p.GuestName = old
		return err
	}
	return nil
}

// writeLocked persists the profile (atomic temp+fsync+rename); caller holds mu.
func (p *profileStore) writeLocked() error {
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	// Self-create the metadata dir (like credstore.atomicWrite): space()
	// normally makes it, but a cached store can outlive its directory if the
	// users tree is scrubbed under a running server (an operator reset that
	// forgets to restart) — the write must recover, not fail with ENOENT.
	// Warn when we actually had to recreate it: a silent 200 would otherwise
	// mask the more serious case where /var/lib/styrman itself was unmounted.
	dir := filepath.Dir(p.path)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		slog.Warn("metadata dir missing — recreating (users tree scrubbed under a running server?)", "path", dir)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp := p.path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil { // durability, like the other stores (N24)
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, p.path)
}

// updateProfile sets per-user preferences. Changing the terminal username
// takes effect for microVMs started afterwards (stop + start a project to
// apply); the home directory on disk is unaffected.
func (s *Server) updateProfile(w http.ResponseWriter, r *http.Request) {
	u := identity.FromContext(r.Context())
	sp, err := s.space(u.Username)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	var body struct {
		TerminalUsername string `json:"terminalUsername"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := sp.profile.setGuestUser(body.TerminalUsername); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"terminalUsername": sp.profile.GuestUser()})
}

// renameProject changes a project's friendly name (identity/UUID unchanged;
// the workspace directory stays where it is).
func (s *Server) renameProject(w http.ResponseWriter, r *http.Request) {
	_, sp, p, err := s.getProjectFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("name required"))
		return
	}
	if err := sp.projects.Rename(p.ID, body.Name); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// usernameChosen reports whether the user explicitly picked a terminal
// username (as opposed to the email-derived default) — a durable checkpoint
// so first-run setup never re-asks.
func (s *Server) usernameChosen(user string) bool {
	sp, err := s.space(user)
	if err != nil {
		return false
	}
	return sp.profile.HasGuestUser()
}
