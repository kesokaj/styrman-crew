// Package links persists public, expiring "capability" URLs for port previews
// (plan: .agents/docs/plans/public-ttl-preview-links.md). A link is an opaque
// token → {project, owner, port, expiry}; anyone holding the token may open
// that one port with no login until it expires or is revoked. Stateful (not a
// signed blob) so links can be revoked and listed.
package links

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// Link is one public preview capability.
type Link struct {
	Token     string    `json:"token"` // 32 hex chars; also a valid DNS label
	Project   string    `json:"project"`
	Owner     string    `json:"owner"` // owner username slug (namespace ops)
	Port      int       `json:"port"`
	CreatedBy string    `json:"createdBy"`
	CreatedAt time.Time `json:"createdAt"`
	Expires   time.Time `json:"expires"`
}

// Expired reports whether the link is past its expiry at now.
func (l Link) Expired(now time.Time) bool { return !now.Before(l.Expires) }

// Store is a mutex-guarded JSON-file-backed link store, keyed by token.
type Store struct {
	mu    sync.Mutex
	path  string
	links map[string]Link
}

type storeFile struct {
	Links []Link `json:"links"`
}

// NewStore loads (or initializes) the store at path.
func NewStore(path string) (*Store, error) {
	s := &Store{path: path, links: map[string]Link{}}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	var f storeFile
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("parse links store: %w", err)
	}
	for _, l := range f.Links {
		s.links[l.Token] = l
	}
	return s, nil
}

func newToken() (string, error) {
	b := make([]byte, 16) // 128 bits
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// Create mints a link for {project, owner, port} valid for ttl from now.
func (s *Store) Create(project, owner string, port int, createdBy string, ttl time.Duration, now time.Time) (Link, error) {
	token, err := newToken()
	if err != nil {
		return Link{}, err
	}
	l := Link{
		Token: token, Project: project, Owner: owner, Port: port,
		CreatedBy: createdBy, CreatedAt: now, Expires: now.Add(ttl),
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked(now)
	s.links[token] = l
	if err := s.saveLocked(); err != nil {
		delete(s.links, token)
		return Link{}, err
	}
	return l, nil
}

// Get returns a live (non-expired) link by token.
func (s *Store) Get(token string, now time.Time) (Link, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.links[token]
	if !ok || l.Expired(now) {
		return Link{}, false
	}
	return l, true
}

// ListFor returns the project's live links, newest first.
func (s *Store) ListFor(project string, now time.Time) []Link {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Link
	for _, l := range s.links {
		if l.Project == project && !l.Expired(now) {
			out = append(out, l)
		}
	}
	// newest first (no time.Now sort key beyond CreatedAt)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].CreatedAt.After(out[j-1].CreatedAt); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// Delete revokes a link. Revoking an unknown/expired token is a no-op success.
func (s *Store) Delete(token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.links[token]; !ok {
		return nil
	}
	delete(s.links, token)
	return s.saveLocked()
}

// pruneLocked drops expired links (caller holds mu). Does not persist by itself.
func (s *Store) pruneLocked(now time.Time) {
	for t, l := range s.links {
		if l.Expired(now) {
			delete(s.links, t)
		}
	}
}

func (s *Store) saveLocked() error {
	f := storeFile{Links: make([]Link, 0, len(s.links))}
	for _, l := range s.links {
		f.Links = append(f.Links, l)
	}
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// ValidToken reports whether t has the exact shape Create produces (32 hex),
// so callers can cheap-reject garbage before a store lookup.
func ValidToken(t string) bool {
	if len(t) != 32 {
		return false
	}
	for _, c := range t {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
