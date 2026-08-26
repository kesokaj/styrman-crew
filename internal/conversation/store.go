package conversation

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"sort"
	"strconv"
	"sync"
	"time"
)

// Record is a durable terminal (ADR-016): it outlives tmux sessions,
// containers, and reboots, carrying the title shown in the sidebar.
// AgentSession is a stable UUID reserved for agent-session pinning
// (parked with ADR-014; see ADR-011 for what it was used for).
type Record struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	AgentSession string    `json:"agentSession,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
	LastOpened   time.Time `json:"lastOpened"`
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
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	for project, recs := range s.byProject {
		kept := recs[:0]
		for _, r := range recs {
			// Validate on load: ids flow into tmux session names and the
			// agentSession UUID into a shell string (bridge.paneCommand), so
			// a hand-edited file must not break those invariants.
			if ValidateID(r.ID) != nil {
				slog.Warn("skipping invalid terminal record", "project", project, "id", r.ID)
				continue
			}
			if r.AgentSession != "" && !sessionRe.MatchString(r.AgentSession) {
				slog.Warn("skipping terminal with invalid agentSession", "project", project, "id", r.ID)
				continue
			}
			kept = append(kept, r)
		}
		s.byProject[project] = kept
	}
	return s, nil
}

// sessionRe: lowercase UUIDv4 shape, as produced by newUUID.
var sessionRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// Create allocates the next numeric id for the project and persists a record.
func (s *Store) Create(project, title string) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	used := map[string]bool{}
	for _, r := range s.byProject[project] {
		used[r.ID] = true
	}
	id := ""
	for n := 1; ; n++ {
		id = strconv.Itoa(n)
		if !used[id] {
			break
		}
	}
	if title == "" {
		title = "Terminal " + id
	}
	rec := Record{
		ID:           id,
		Title:        title,
		AgentSession: newUUID(),
		CreatedAt:    time.Now().UTC(),
		LastOpened:   time.Now().UTC(),
	}
	s.byProject[project] = append(s.byProject[project], rec)
	if err := s.save(); err != nil {
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
			s.byProject[project][i].Title = title
			return s.save()
		}
	}
	return fmt.Errorf("terminal %s not found", id)
}

// Delete removes a record. The transcript in the shared home is left alone
// (ADR-016) — history outlives the terminal it came from.
func (s *Store) Delete(project, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	recs := s.byProject[project]
	for i, r := range recs {
		if r.ID == id {
			s.byProject[project] = append(recs[:i], recs[i+1:]...)
			return s.save()
		}
	}
	return nil
}

// DeleteProject removes all records for a project.
func (s *Store) DeleteProject(project string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.byProject[project]; !ok {
		return nil
	}
	delete(s.byProject, project)
	return s.save()
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

// newUUID returns a random v4 UUID (stdlib-only).
func newUUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("fallback-%d", time.Now().UnixNano())
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
