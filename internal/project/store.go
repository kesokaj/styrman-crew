// Package project persists project metadata that must outlive containers
// (ADR-007). The store is a single JSON file written atomically
// (tmp + rename), following devbox's userstore pattern.
package project

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"sort"
	"sync"
	"time"
)

// Project is one styrman project. Identity is the UUID (containers,
// networks, docker disks, transcripts all key on it); Name is a friendly,
// renamable label — the same name can be used again after deletion. Dir is
// the workspace directory under ~/projects, frozen at creation so renames
// never move files.
type Project struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Dir       string    `json:"dir"`
	Agent     string    `json:"agent,omitempty"` // legacy field
	CreatedAt time.Time `json:"createdAt"`
	GitInit   bool      `json:"gitInit"`
	Archived  bool      `json:"archived"`
	// SharedEmails may access this project's port previews (read-only, only
	// the previews — never the terminal, files, or the dashboard). Empty =
	// owner-only. Emails are authenticated by the Google OAuth ingress.
	SharedEmails []string `json:"sharedEmails,omitempty"`
}

// nameRe accepts short kebab-case slugs. First char a letter so names are
// valid hostnames, usernames-adjacent, and tmux-safe.
var nameRe = regexp.MustCompile(`^[a-z][a-z0-9-]{0,30}$`)

// ErrNotFound is returned when a project does not exist.
var ErrNotFound = errors.New("project not found")

// ValidateName reports whether name is an acceptable project slug.
func ValidateName(name string) error {
	if !nameRe.MatchString(name) {
		return fmt.Errorf("invalid project name %q: must match %s", name, nameRe.String())
	}
	return nil
}

type storeFile struct {
	Projects []Project `json:"projects"`
}

// Store is a mutex-guarded JSON-file-backed project store.
type Store struct {
	mu       sync.Mutex
	path     string
	projects map[string]Project // keyed by ID
}

// NewStore loads (or initializes) the store at path.
func NewStore(path string) (*Store, error) {
	s := &Store{path: path, projects: map[string]Project{}}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, err
	}
	var f storeFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	migrated := false
	for _, p := range f.Projects {
		// Validate on load, not just on Add: everything downstream (paths,
		// container/network names, tmux -c, wipe's RemoveAll) trusts store
		// contents, so a hand-edited file must not break the invariant.
		if err := ValidateName(p.Name); err != nil {
			slog.Warn("skipping invalid project in store", "name", p.Name, "error", err)
			continue
		}
		// Migration from name-keyed records: assign a UUID, freeze the dir.
		if p.ID == "" {
			p.ID = NewUUID()
			migrated = true
		}
		if p.Dir == "" {
			p.Dir = p.Name
		}
		if !uuidRe.MatchString(p.ID) || ValidateName(p.Dir) != nil {
			slog.Warn("skipping project with invalid id/dir", "name", p.Name)
			continue
		}
		s.projects[p.ID] = p
	}
	if migrated {
		s.mu.Lock()
		_ = s.save()
		s.mu.Unlock()
	}
	return s, nil
}

// uuidRe: lowercase UUIDv4 shape (NewUUID's output).
var uuidRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// Add registers a new project, assigning its UUID and freezing Dir = Name.
// The name must not collide with another *current* project (the workspace
// dir is name-based); reusing a name after deletion is fine — the UUID keeps
// every runtime artifact distinct.
func (s *Store) Add(p Project) (Project, error) {
	if err := ValidateName(p.Name); err != nil {
		return Project{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, other := range s.projects {
		if other.Name == p.Name {
			return Project{}, fmt.Errorf("a project named %q already exists", p.Name)
		}
	}
	p.ID = NewUUID()
	p.Dir = p.Name
	s.projects[p.ID] = p
	return p, s.save()
}

// Rename changes the friendly name (the workspace dir stays put).
func (s *Store) Rename(id, name string) error {
	if err := ValidateName(name); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.projects[id]
	if !ok {
		return ErrNotFound
	}
	p.Name = name
	s.projects[id] = p
	return s.save()
}

// SetShares replaces the project's shared-email list (already validated +
// normalized by the caller).
func (s *Store) SetShares(id string, emails []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.projects[id]
	if !ok {
		return ErrNotFound
	}
	p.SharedEmails = emails
	s.projects[id] = p
	return s.save()
}

// Get returns a project by ID.
func (s *Store) Get(id string) (Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.projects[id]
	if !ok {
		return Project{}, ErrNotFound
	}
	return p, nil
}

// Remove deletes a project's metadata by ID.
func (s *Store) Remove(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.projects[id]; !ok {
		return ErrNotFound
	}
	delete(s.projects, id)
	return s.save()
}

// SetArchived flips the archived flag.
func (s *Store) SetArchived(id string, archived bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.projects[id]
	if !ok {
		return ErrNotFound
	}
	p.Archived = archived
	s.projects[id] = p
	return s.save()
}

// List returns all projects, newest first.
func (s *Store) List() []Project {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Project, 0, len(s.projects))
	for _, p := range s.projects {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

// save must be called with s.mu held.
func (s *Store) save() error {
	f := storeFile{Projects: make([]Project, 0, len(s.projects))}
	for _, p := range s.projects {
		f.Projects = append(f.Projects, p)
	}
	sort.Slice(f.Projects, func(i, j int) bool { return f.Projects[i].ID < f.Projects[j].ID })
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	// Atomic + durable: fsync the temp file before rename so a power cut
	// can't leave a truncated projects.json — the only durable metadata (R7).
	tmp := s.path + ".tmp"
	f2, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := f2.Write(data); err != nil {
		f2.Close()
		return err
	}
	if err := f2.Sync(); err != nil {
		f2.Close()
		return err
	}
	if err := f2.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
