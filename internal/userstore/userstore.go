// Package userstore decides who may use styrman (R8). Ported (trimmed) from
// devbox's pkg/userstore: authorization is re-checked here even though the
// Caddy ingress already authenticated — the ingress proves identity, this
// store proves permission. Two sources: the STYRMAN_ALLOWED_EMAILS env
// (immutable, survives file loss) and users.json (managed, atomic writes).
package userstore

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kesokaj/styrman-crew/internal/identity"
)

// UserEntry is one stored user.
type UserEntry struct {
	Email   string    `json:"email"`
	AddedBy string    `json:"addedBy"`
	AddedAt time.Time `json:"addedAt"`
}

type usersFile struct {
	Users []UserEntry `json:"users"`
}

// Store is a mutex-guarded JSON-backed allowlist plus an env-derived set.
type Store struct {
	mu       sync.Mutex
	path     string
	envUsers map[string]bool
	users    map[string]UserEntry
}

// New loads the store. envEmails come from STYRMAN_ALLOWED_EMAILS and are
// never written to disk.
func New(path string, envEmails []string) (*Store, error) {
	s := &Store{path: path, envUsers: map[string]bool{}, users: map[string]UserEntry{}}
	for _, e := range envEmails {
		if e = normalize(e); e != "" {
			s.envUsers[e] = true
		}
	}
	if data, err := os.ReadFile(path); err == nil {
		var f usersFile
		if err := json.Unmarshal(data, &f); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		for _, u := range f.Users {
			if e := normalize(u.Email); e != "" {
				u.Email = e
				s.users[e] = u
			}
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	// Isolation keys on DeriveUsername (per-user homes, state, and brokered
	// credentials), so two authorized emails that slug to the same username
	// would share one workspace — see and delete each other's projects, files,
	// and secrets (H3). Refuse to start rather than silently merge them; the
	// operator curates a small allowlist, so this is a config error to fix.
	seen := map[string]string{}
	for _, e := range s.authorizedEmails() {
		slug := identity.DeriveUsername(e)
		if prev, ok := seen[slug]; ok && prev != e {
			return nil, fmt.Errorf("users %q and %q both map to username %q — pick distinct emails (per-user isolation keys on the username)", prev, e, slug)
		}
		seen[slug] = e
	}
	return s, nil
}

// authorizedEmails returns every authorized email (env + stored), deduped.
// Caller need not hold s.mu during New; use List() elsewhere.
func (s *Store) authorizedEmails() []string {
	set := map[string]bool{}
	for e := range s.envUsers {
		set[e] = true
	}
	for e := range s.users {
		set[e] = true
	}
	out := make([]string, 0, len(set))
	for e := range set {
		out = append(out, e)
	}
	return out
}

// slugCollision returns an already-authorized email (other than `email`) whose
// DeriveUsername slug equals email's, or "" if none. Caller holds s.mu.
func (s *Store) slugCollision(email string) string {
	slug := identity.DeriveUsername(email)
	for _, e := range s.authorizedEmails() {
		if e != email && identity.DeriveUsername(e) == slug {
			return e
		}
	}
	return ""
}

// IsAdmin reports whether email is an environment-provided administrator
// (STYRMAN_ALLOWED_EMAILS). Admins manage the user list; users added at
// runtime get their own workspace but no admin rights.
func (s *Store) IsAdmin(email string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.envUsers[normalize(email)]
}

// IsAuthorized reports whether email may use styrman.
func (s *Store) IsAuthorized(email string) bool {
	e := normalize(email)
	if e == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.envUsers[e] {
		return true
	}
	_, ok := s.users[e]
	return ok
}

// Add authorizes an email (persisted).
func (s *Store) Add(email, addedBy string) error {
	e := normalize(email)
	if e == "" || !strings.Contains(e, "@") {
		return fmt.Errorf("invalid email %q", email)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// Reject an email that would share a username slug with an already-authorized
	// one — it would collapse the two into a single shared workspace (H3).
	if c := s.slugCollision(e); c != "" {
		return fmt.Errorf("email %q maps to the same username %q as already-authorized %q — pick a different address", e, identity.DeriveUsername(e), c)
	}
	s.users[e] = UserEntry{Email: e, AddedBy: addedBy, AddedAt: time.Now().UTC()}
	return s.save()
}

// Remove revokes a stored email (env-derived users cannot be removed).
func (s *Store) Remove(email string) error {
	e := normalize(email)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.envUsers[e] {
		return fmt.Errorf("%s comes from STYRMAN_ALLOWED_EMAILS and cannot be removed here", e)
	}
	delete(s.users, e)
	return s.save()
}

// List returns all authorized emails (env-derived first).
func (s *Store) List() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.envUsers)+len(s.users))
	for e := range s.envUsers {
		out = append(out, e)
	}
	for e := range s.users {
		if !s.envUsers[e] {
			out = append(out, e)
		}
	}
	sort.Strings(out)
	return out
}

// save must be called with s.mu held.
func (s *Store) save() error {
	f := usersFile{Users: make([]UserEntry, 0, len(s.users))}
	for _, u := range s.users {
		f.Users = append(f.Users, u)
	}
	sort.Slice(f.Users, func(i, j int) bool { return f.Users[i].Email < f.Users[j].Email })
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	// Atomic temp+fsync+rename like every other store (audit finding): the
	// allowlist is the authorization source of truth (R8) — a power cut must
	// not be able to leave it truncated.
	tmp := s.path + ".tmp"
	tf, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := tf.Write(data); err != nil {
		tf.Close()
		return err
	}
	if err := tf.Sync(); err != nil {
		tf.Close()
		return err
	}
	if err := tf.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func normalize(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
