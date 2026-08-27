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
	"sort"
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

// MaxLivePerProject caps how many live links one project may hold, bounding
// both the file size and the O(N) rewrite cost.
const MaxLivePerProject = 20

// ErrTooManyLinks is returned when a project is at MaxLivePerProject.
var ErrTooManyLinks = fmt.Errorf("too many active links (max %d) — revoke one first", MaxLivePerProject)

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
	live := 0
	for _, e := range s.links {
		if e.Project == project {
			live++
		}
	}
	if live >= MaxLivePerProject {
		return Link{}, ErrTooManyLinks
	}
	s.links[token] = l
	if err := s.saveLocked(); err != nil {
		delete(s.links, token) // roll back so memory matches disk
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

// ListFor returns the project's live links, newest first. It also prunes any
// expired records it encounters (S4: pruning shouldn't depend on Create).
func (s *Store) ListFor(project string, now time.Time) []Link {
	s.mu.Lock()
	defer s.mu.Unlock()
	before := len(s.links)
	s.pruneLocked(now)
	if len(s.links) != before {
		_ = s.saveLocked() // best-effort: dropping expired rows is not critical
	}
	var out []Link
	for _, l := range s.links {
		if l.Project == project {
			out = append(out, l)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

// DeleteProject revokes every link belonging to a project (S8: link revocation
// must cascade when the project is deleted).
func (s *Store) DeleteProject(project string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	changed := false
	for t, l := range s.links {
		if l.Project == project {
			delete(s.links, t)
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return s.saveLocked()
}

// Delete revokes a link. Revoking an unknown/expired token is a no-op success.
func (s *Store) Delete(token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.links[token]
	if !ok {
		return nil
	}
	delete(s.links, token)
	if err := s.saveLocked(); err != nil {
		s.links[token] = l // B3: restore so memory matches disk on write failure
		return err
	}
	return nil
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
	sort.Slice(f.Links, func(i, j int) bool { return f.Links[i].Token < f.Links[j].Token })
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	// Atomic + durable (S5): fsync the temp file before rename so a power cut
	// just after a revoke can't lose the write and resurrect the link.
	tmp := s.path + ".tmp"
	fh, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := fh.Write(b); err != nil {
		fh.Close()
		return err
	}
	if err := fh.Sync(); err != nil {
		fh.Close()
		return err
	}
	if err := fh.Close(); err != nil {
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
