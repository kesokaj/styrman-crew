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
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, err
	}
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
	return s, nil
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
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func normalize(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
