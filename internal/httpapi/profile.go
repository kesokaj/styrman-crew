package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"

	"github.com/kesokaj/styrman/internal/identity"
)

// profileStore holds per-user preferences — today just the in-guest
// ("terminal") username, chosen at first setup or in Settings. It defaults
// to the identity slug and applies to microVMs started after a change.
type profileStore struct {
	path      string
	def       string
	GuestName string `json:"guestUser,omitempty"`
}

var guestNameRe = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)

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
	if p.GuestName != "" {
		return p.GuestName
	}
	return p.def
}

func (p *profileStore) setGuestUser(name string) error {
	if name != "" && !guestNameRe.MatchString(name) {
		return fmt.Errorf("invalid username %q: lowercase letters, digits, - and _, starting with a letter", name)
	}
	p.GuestName = name
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	tmp := p.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
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
	return sp.profile.GuestName != ""
}
