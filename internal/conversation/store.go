package conversation

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"
)

// Record is a durable terminal (ADR-016): it outlives tmux sessions,
// containers, and reboots, carrying the title shown in the sidebar.
// (The former AgentSession UUID was removed in v1.14.0: native agent resume
// via --continue superseded the parked ADR-011/014 pinning idea, and the
// field was written but never read.)
// Command is the optional autostart (operator Q8, 2026-08-28): it runs when
// the terminal's tmux SESSION is created — first open, and every re-open
// after a pause or VM restart — then drops to the login shell.
type Record struct {
	ID         string    `json:"id"`
	Title      string    `json:"title"`
	Command    string    `json:"command,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
	LastOpened time.Time `json:"lastOpened"`
}

// Store persists terminal records per project (single JSON file, atomic
// writes — the one addition to ADR-007's "no database" rule).
type Store struct {
	mu        sync.Mutex
	path      string
	byProject map[string][]Record
}

// NewStore loads (or initializes) the store.
func NewStore(path string) (*Store, error) {
	s := &Store{path: path, byProject: map[string][]Record{}}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(data, &s.byProject); err != nil {
		// A corrupt store must not 500 every request for this user forever:
		// quarantine the unparseable file and start empty; the bad file is kept
		// as .corrupt for recovery (low finding). Terminal records are the least
		// precious per-user state — tmux sessions in the VM are re-adopted.
		slog.Error("conversations.json unparseable — quarantined to .corrupt, starting empty", "path", path, "error", err)
		s.byProject = map[string][]Record{}
		_ = os.Rename(path, path+".corrupt")
		return s, nil
	}
	for project, recs := range s.byProject {
		kept := recs[:0]
		for _, r := range recs {
			// Validate on load: ids flow into tmux session names and the
			// autostart command into a shell string (autostartPaneCmd), so
			// a hand-edited file must not break those invariants.
			if ValidateID(r.ID) != nil {
				slog.Warn("skipping invalid terminal record", "project", project, "id", r.ID)
				continue
			}
			kept = append(kept, r)
		}
		s.byProject[project] = kept
	}
	return s, nil
}

// Create allocates the next numeric id for the project and persists a record.
func (s *Store) Create(project, title string) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	used := map[string]bool{}
	titles := map[string]bool{}
	for _, r := range s.byProject[project] {
		used[r.ID] = true
		titles[r.Title] = true
	}
	id := ""
	for n := 1; ; n++ {
		id = strconv.Itoa(n)
		if !used[id] {
			break
		}
	}
	if title == "" {
		title = friendlyName(titles, id)
	}
	rec := Record{
		ID:         id,
		Title:      title,
		CreatedAt:  time.Now().UTC(),
		LastOpened: time.Now().UTC(),
	}
	s.byProject[project] = append(s.byProject[project], rec)
	if err := s.save(); err != nil {
		recs := s.byProject[project] // roll back the append (M20)
		s.byProject[project] = recs[:len(recs)-1]
		return Record{}, err
	}
	return rec, nil
}

// Get returns one record.
func (s *Store) Get(project, id string) (Record, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.byProject[project] {
		if r.ID == id {
			return r, true
		}
	}
	return Record{}, false
}

// Touch updates LastOpened.
func (s *Store) Touch(project, id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, r := range s.byProject[project] {
		if r.ID == id {
			s.byProject[project][i].LastOpened = time.Now().UTC()
			if err := s.save(); err != nil {
				slog.Warn("terminal store save failed", "project", project, "id", id, "error", err)
			}
			return
		}
	}
}

// Rename sets a terminal's title.
func (s *Store) Rename(project, id, title string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, r := range s.byProject[project] {
		if r.ID == id {
			prev := r.Title
			s.byProject[project][i].Title = title
			if err := s.save(); err != nil {
				s.byProject[project][i].Title = prev // roll back (M20)
				return err
			}
			return nil
		}
	}
	return fmt.Errorf("terminal %s not found", id)
}

// SetCommand sets (or clears, with "") a terminal's autostart command.
func (s *Store) SetCommand(project, id, command string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, r := range s.byProject[project] {
		if r.ID == id {
			prev := r.Command
			s.byProject[project][i].Command = command
			if err := s.save(); err != nil {
				s.byProject[project][i].Command = prev // roll back (M20)
				return err
			}
			return nil
		}
	}
	return fmt.Errorf("terminal %s not found", id)
}

// Delete removes a record. The transcript in the project's home is left alone
// (ADR-016) — history outlives the terminal it came from.
func (s *Store) Delete(project, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	recs := s.byProject[project]
	for i, r := range recs {
		if r.ID == id {
			orig := append([]Record(nil), recs...) // copy for rollback (M20)
			s.byProject[project] = append(recs[:i], recs[i+1:]...)
			if err := s.save(); err != nil {
				s.byProject[project] = orig
				return err
			}
			return nil
		}
	}
	return nil
}

// DeleteProject removes all records for a project.
func (s *Store) DeleteProject(project string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	prev, ok := s.byProject[project]
	if !ok {
		return nil
	}
	delete(s.byProject, project)
	if err := s.save(); err != nil {
		s.byProject[project] = prev // roll back (M20)
		return err
	}
	return nil
}

// List returns the project's records, newest-opened first.
func (s *Store) List(project string) []Record {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := append([]Record(nil), s.byProject[project]...)
	sort.Slice(out, func(i, j int) bool { return out[i].LastOpened.After(out[j].LastOpened) })
	return out
}

// save must be called with s.mu held. Atomic + fsync'd like projects.json.
func (s *Store) save() error {
	data, err := json.MarshalIndent(s.byProject, "", "  ")
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
	tmp := s.path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

