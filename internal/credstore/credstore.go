// Package credstore is the credential broker's source of truth (ADR-030):
// one per-user store, server-owned, that projects never touch directly.
// Onboarding writes credentials here (extracted once from the tool, e.g.
// gh/agy keep theirs in the guest keyring); the broker reads and serves them
// so every project uses the current copy and a refresh propagates to all.
//
// Slice 1 covers git/GitHub tokens; the shape generalizes to the other
// tools' adapters. Files are 0600 under StateDir/credentials/<user>/.
package credstore

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

// ErrNotFound is returned when a credential has not been onboarded yet.
var ErrNotFound = errors.New("credential not found")

// GitCred is a git/GitHub credential for one host.
type GitCred struct {
	Host     string `json:"host"`     // e.g. "github.com"
	Username string `json:"username"` // e.g. "x-access-token" or the login
	Token    string `json:"token"`
}

// Store is the per-user credential store rooted at a base dir.
type Store struct {
	base string // StateDir/credentials
	mu   sync.RWMutex
}

// New roots a store at baseDir (created if missing).
func New(baseDir string) (*Store, error) {
	if err := os.MkdirAll(baseDir, 0o700); err != nil {
		return nil, err
	}
	return &Store{base: baseDir}, nil
}

func (s *Store) userDir(user string) string { return filepath.Join(s.base, user) }

// atomicWrite writes 0600 via temp+rename (never a torn credential file).
func atomicWrite(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

// PutGit stores (or replaces) the git credential for a user+host.
func (s *Store) PutGit(user string, c GitCred) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(filepath.Join(s.userDir(user), "git-"+safe(c.Host)+".json"), append(data, '\n'))
}

// GetGit returns the git credential for a user+host, or ErrNotFound.
func (s *Store) GetGit(user, host string) (GitCred, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	data, err := os.ReadFile(filepath.Join(s.userDir(user), "git-"+safe(host)+".json"))
	if err != nil {
		if os.IsNotExist(err) {
			return GitCred{}, ErrNotFound
		}
		return GitCred{}, err
	}
	var c GitCred
	if err := json.Unmarshal(data, &c); err != nil {
		return GitCred{}, err
	}
	return c, nil
}

// safe reduces a host to a filename-safe token (no path separators).
func safe(host string) string {
	out := make([]rune, 0, len(host))
	for _, r := range host {
		if r == '/' || r == '\\' || r == '.' || r == ':' {
			out = append(out, '_')
			continue
		}
		out = append(out, r)
	}
	if len(out) == 0 {
		return "default"
	}
	return string(out)
}
