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
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Link kinds. Public links are the anyone-with-the-URL capability (TTL is the
// only gate); private links are served on an OAuth-gated host and additionally
// checked against the port's shared-email list at serve time. An empty Kind is
// public (records predating the field).
const (
	KindPublic  = "public"
	KindPrivate = "private"
)

// Link is one preview capability (public TTL link or private share link).
type Link struct {
	Token     string    `json:"token"` // 32 hex chars; also a valid DNS label
	Kind      string    `json:"kind,omitempty"`
	Project   string    `json:"project"`
	Owner     string    `json:"owner"` // owner username slug (namespace ops)
	Port      int       `json:"port"`
	CreatedBy string    `json:"createdBy"`
	CreatedAt time.Time `json:"createdAt"`
	Expires   time.Time `json:"expires"`
}

// Private reports whether this is an email-gated share link.
func (l Link) Private() bool { return l.Kind == KindPrivate }

// Expired reports whether the link is past its expiry at now. The invariant
// keys on Kind, not on the timestamp: private links never expire (they live
// until the share is revoked), and a PUBLIC record without an expiry — a
// hand-edited store, a bug that skipped the TTL — is treated as expired
// rather than becoming an immortal public capability (fail-closed).
func (l Link) Expired(now time.Time) bool {
	if l.Private() {
		return false
	}
	if l.Expires.IsZero() {
		return true
	}
	return !now.Before(l.Expires)
}

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
		// Validate-on-load like the project/conversation stores: a
		// hand-edited record with a malformed token must not enter the map
		// (a short token would panic the token[:8] log slice downstream).
		if !ValidToken(l.Token) {
			continue
		}
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

// Create mints a public link for {project, owner, port} valid for ttl from now.
func (s *Store) Create(project, owner string, port int, createdBy string, ttl time.Duration, now time.Time) (Link, error) {
	token, err := newToken()
	if err != nil {
		return Link{}, err
	}
	l := Link{
		Token: token, Kind: KindPublic, Project: project, Owner: owner, Port: port,
		CreatedBy: createdBy, CreatedAt: now, Expires: now.Add(ttl),
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked(now)
	// The cap is per kind: a project's private links (one per shared port,
	// created implicitly by sharing) must not eat the public-link budget.
	live := 0
	for _, e := range s.links {
		if e.Project == project && !e.Private() {
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

// EnsurePrivate returns the project+port's private share link, minting it if
// absent (idempotent — sharing a port twice must not rotate its URL). Private
// links never expire; they are deleted when the port's share list empties.
// An existing link keeps its stored Owner/CreatedBy: project IDs are global
// UUIDs and ownership never moves, so a mismatch cannot occur in practice.
func (s *Store) EnsurePrivate(project, owner string, port int, createdBy string, now time.Time) (Link, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	live := 0
	for _, e := range s.links {
		if e.Project != project || !e.Private() {
			continue
		}
		if e.Port == port {
			return e, nil
		}
		live++
	}
	if live >= MaxLivePerProject {
		return Link{}, ErrTooManyLinks
	}
	token, err := newToken()
	if err != nil {
		return Link{}, err
	}
	l := Link{
		Token: token, Kind: KindPrivate, Project: project, Owner: owner, Port: port,
		CreatedBy: createdBy, CreatedAt: now,
	}
	s.links[token] = l
	if err := s.saveLocked(); err != nil {
		delete(s.links, token) // roll back so memory matches disk
		return Link{}, err
	}
	return l, nil
}

// GetPrivate returns the live private link for project+port, if any.
func (s *Store) GetPrivate(project string, port int) (Link, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.links {
		if e.Project == project && e.Port == port && e.Private() {
			return e, true
		}
	}
	return Link{}, false
}

// DeletePrivate removes the private link for project+port (no-op if absent).
func (s *Store) DeletePrivate(project string, port int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	removed := map[string]Link{}
	for t, e := range s.links {
		if e.Project == project && e.Port == port && e.Private() {
			removed[t] = e
			delete(s.links, t)
		}
	}
	if len(removed) == 0 {
		return nil
	}
	if err := s.saveLocked(); err != nil {
		// B3/S5: restore so memory matches disk — otherwise the token would
		// resurrect on restart while callers believe it's gone.
		for t, e := range removed {
			s.links[t] = e
		}
		return err
	}
	return nil
}

// ListPublicFor returns the project's live PUBLIC links, newest first —
// private share links never belong in the public-links listing, and keeping
// the filter here means no caller can forget it.
func (s *Store) ListPublicFor(project string, now time.Time) []Link {
	var out []Link
	for _, l := range s.ListFor(project, now) {
		if !l.Private() {
			out = append(out, l)
		}
	}
	return out
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
	removed := map[string]Link{}
	for t, l := range s.links {
		if l.Project == project {
			removed[t] = l
			delete(s.links, t)
		}
	}
	if len(removed) == 0 {
		return nil
	}
	if err := s.saveLocked(); err != nil {
		// B3 discipline (same as Delete/DeletePrivate): restore on a failed
		// save so the tokens don't resurrect on restart while callers
		// believe they're gone.
		for t, l := range removed {
			s.links[t] = l
		}
		return err
	}
	return nil
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
	if err := os.Rename(tmp, s.path); err != nil {
		return err
	}
	// fsync the parent directory too, so the rename itself survives a power cut
	// — otherwise the "a revoke cannot be lost" guarantee doesn't actually hold
	// (the temp file's fsync doesn't make the directory entry durable).
	if d, derr := os.Open(filepath.Dir(s.path)); derr == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
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
