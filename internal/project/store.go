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
	"path/filepath"
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
	CreatedAt time.Time `json:"createdAt"`
	// SharedPorts maps an exposed port → the Google emails allowed to open
	// THAT port's preview (and only that port — never the terminal, files, or
	// dashboard). Per-port so a share is least-privilege; an empty/absent entry
	// is owner-only. Emails are authenticated by the Google OAuth ingress.
	SharedPorts map[int][]string `json:"sharedPorts,omitempty"`
	// LanAccess is the operator-approved local-network exception (ADR-065):
	// while true the root firewall lets THIS project's microVM reach the
	// configured LAN prefixes. This record is the intent; the server projects
	// it as a marker file the root script reconciles, and reports the applied
	// state from the script's status, never from this field alone.
	// LanChangedAt is when LanAccess last flipped, so a change that root has
	// not applied can be reported as pending, then as an error.
	LanAccess    bool      `json:"lanAccess,omitempty"`
	LanChangedAt time.Time `json:"lanChangedAt,omitzero"`
	// LanEverEnabled LATCHES the first time LanAccess is turned on and is
	// never cleared (ADR-066 amendment). It is the whole of the credential
	// decision: a project that has ever been LAN-enabled is never served the
	// operator's SSH key again.
	//
	// Latching rather than tracking the live state is the point. Re-serving a
	// key after LAN access is turned off does not un-expose it — the project
	// had it — so the only thing the live state bought was a window in which
	// the answer could be WRONG: root's rule can outlive the record by up to
	// a minute, and a failed revocation drops the project out of the applied
	// set while its rule is still present. This field cannot be wrong.
	LanEverEnabled bool `json:"lanEverEnabled,omitempty"`
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
		// A corrupt store must not 500 every request for this user forever:
		// quarantine the unparseable file and start empty so the account works
		// again; the bad file is preserved as .corrupt for recovery (low finding).
		slog.Error("projects.json unparseable — quarantined to .corrupt, starting empty", "path", path, "error", err)
		_ = os.Rename(path, path+".corrupt")
		return s, nil
	}
	migrated := false
	var rejected []Project
	for _, p := range f.Projects {
		// Validate on load, not just on Add: everything downstream (paths,
		// container/network names, tmux -c, wipe's RemoveAll) trusts store
		// contents, so a hand-edited file must not break the invariant.
		if err := ValidateName(p.Name); err != nil {
			slog.Error("quarantining invalid project from store (preserved in .rejected)", "name", p.Name, "error", err)
			rejected = append(rejected, p)
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
			slog.Error("quarantining project with invalid id/dir (preserved in .rejected)", "name", p.Name)
			rejected = append(rejected, p)
			continue
		}
		// A hand-edited lanAccess:true without a timestamp: date it now so the
		// pending→error clock is defined (not persisted until the next save).
		if p.LanAccess && p.LanChangedAt.IsZero() {
			p.LanChangedAt = time.Now().UTC()
		}
		s.projects[p.ID] = p
	}
	// Preserve rejected records off to the side: save() rewrites the file from
	// valid records only, which would otherwise erase them from disk forever
	// (M21). Best-effort, loud on failure.
	if len(rejected) > 0 {
		if data, merr := json.MarshalIndent(storeFile{Projects: rejected}, "", "  "); merr == nil {
			if werr := os.WriteFile(s.path+".rejected", append(data, '\n'), 0o600); werr != nil {
				slog.Error("could not preserve rejected project records", "path", s.path+".rejected", "error", werr)
			}
		}
	}
	if migrated {
		s.mu.Lock()
		if err := s.save(); err != nil {
			// Loud, not silent (audit finding): if this write fails, the next
			// boot re-runs the migration and mints NEW UUIDs for the same
			// projects — orphaning containers, networks, and homes.
			slog.Error("project UUID migration save FAILED — next boot will re-migrate with new IDs",
				"path", path, "error", err)
		}
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
	if err := s.save(); err != nil {
		delete(s.projects, p.ID) // roll back so memory matches disk (M20)
		return Project{}, err
	}
	return p, nil
}

// Rename changes the friendly name (the workspace dir stays put).
func (s *Store) Rename(id, name string) error {
	if err := ValidateName(name); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	prev, ok := s.projects[id]
	if !ok {
		return ErrNotFound
	}
	p := prev
	p.Name = name
	s.projects[id] = p
	if err := s.save(); err != nil {
		s.projects[id] = prev // roll back (M20)
		return err
	}
	return nil
}

// SetPortShares replaces the shared-email list for one exposed port (emails
// already validated + normalized by the caller). An empty list clears it.
func (s *Store) SetPortShares(id string, port int, emails []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	prev, ok := s.projects[id]
	if !ok {
		return ErrNotFound
	}
	// Copy-on-write so the stored map isn't mutated in place (Get returns it).
	next := map[int][]string{}
	for k, v := range prev.SharedPorts {
		if k != port {
			next[k] = v
		}
	}
	if len(emails) > 0 {
		next[port] = emails
	}
	if len(next) == 0 {
		next = nil
	}
	p := prev
	p.SharedPorts = next
	s.projects[id] = p
	if err := s.save(); err != nil {
		s.projects[id] = prev // roll back (M20)
		return err
	}
	return nil
}

// SetLanAccess records the local-network intent (ADR-065). LanChangedAt
// moves only when the value actually changes, so a repeated PUT does not
// reset the pending clock. Copy-on-write with rollback on a failed save, the
// SetPortShares shape.
func (s *Store) SetLanAccess(id string, on bool, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	prev, ok := s.projects[id]
	if !ok {
		return ErrNotFound
	}
	if prev.LanAccess == on {
		return nil
	}
	return s.setLanLocked(id, prev, on, now.UTC())
}

// RestoreLanAccess puts back an EXACT earlier (value, timestamp) pair — the
// handler's rollback when the marker write fails after the record saved.
// Unlike SetLanAccess it never stamps "now": a rollback is not a change the
// user made, and stamping it would move the pending→error clock (review).
func (s *Store) RestoreLanAccess(id string, on bool, changedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	prev, ok := s.projects[id]
	if !ok {
		return ErrNotFound
	}
	if prev.LanAccess == on && prev.LanChangedAt.Equal(changedAt) {
		return nil
	}
	return s.setLanLocked(id, prev, on, changedAt)
}

// setLanLocked must be called with s.mu held.
func (s *Store) setLanLocked(id string, prev Project, on bool, changedAt time.Time) error {
	p := prev
	p.LanAccess = on
	p.LanChangedAt = changedAt
	// One-way. Set on the way ON and never cleared — including by
	// RestoreLanAccess, whose rollback means the MARKER write failed after the
	// record saved: the intent was recorded, the broker may already have
	// stopped serving, and un-latching would be the one direction that can
	// hand a key back by accident.
	if on {
		p.LanEverEnabled = true
	}
	s.projects[id] = p
	if err := s.save(); err != nil {
		s.projects[id] = prev // roll back (M20)
		return err
	}
	return nil
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
	prev, ok := s.projects[id]
	if !ok {
		return ErrNotFound
	}
	delete(s.projects, id)
	if err := s.save(); err != nil {
		s.projects[id] = prev // roll back (M20)
		return err
	}
	return nil
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
	// MkdirAll first (like credstore): the cached store can outlive its dir if
	// the users tree is scrubbed under a running server; warn when we actually
	// had to recreate it (a silent 200 would mask an unmounted state dir).
	dir := filepath.Dir(s.path)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		slog.Warn("metadata dir missing — recreating (users tree scrubbed under a running server?)", "path", dir)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
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
