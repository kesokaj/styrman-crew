// Package project persists project metadata that must outlive containers
// (ADR-007). The store is a single JSON file written atomically
// (tmp + rename), following devbox's userstore pattern.
package project

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"sync"
	"time"

	"github.com/kesokaj/styrman/internal/config"
)

// Project is one styrman project.
type Project struct {
	Name      string    `json:"name"`
	Agent     string    `json:"agent"` // "claude" or "agy"
	CreatedAt time.Time `json:"createdAt"`
	GitInit   bool      `json:"gitInit"`
	Archived  bool      `json:"archived"`
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
	projects map[string]Project
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
	for _, p := range f.Projects {
		s.projects[p.Name] = p
	}
	return s, nil
}

// Add registers a new project. Fails if the name is taken or invalid.
func (s *Store) Add(p Project) error {
	if err := ValidateName(p.Name); err != nil {
		return err
	}
	if _, ok := config.Agents[p.Agent]; !ok {
		return fmt.Errorf("unknown agent %q", p.Agent)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.projects[p.Name]; exists {
		return fmt.Errorf("project %q already exists", p.Name)
	}
	s.projects[p.Name] = p
	return s.save()
}

// Get returns a project by name.
func (s *Store) Get(name string) (Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.projects[name]
	if !ok {
		return Project{}, ErrNotFound
	}
	return p, nil
}

// Remove deletes a project's metadata.
func (s *Store) Remove(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.projects[name]; !ok {
		return ErrNotFound
	}
	delete(s.projects, name)
	return s.save()
}

// SetArchived flips the archived flag.
func (s *Store) SetArchived(name string, archived bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.projects[name]
	if !ok {
		return ErrNotFound
	}
	p.Archived = archived
	s.projects[name] = p
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
	sort.Slice(f.Projects, func(i, j int) bool { return f.Projects[i].Name < f.Projects[j].Name })
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
