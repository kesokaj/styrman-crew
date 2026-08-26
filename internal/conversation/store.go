package conversation

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strconv"
	"sync"
	"time"
)

// Record is a durable conversation (ADR-011): it outlives tmux sessions,
// containers, and reboots. For claude, AgentSession pins the Claude Code
// session UUID so reopening resumes the same thread with full history; agy
// keeps its own history and AgentSession stays empty until its resume
// mechanism is wired up.
type Record struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	AgentSession string    `json:"agentSession,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
	LastOpened   time.Time `json:"lastOpened"`
}

// Store persists conversation records per project (single JSON file, atomic
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
			if ValidateID(r.ID) == nil {
				kept = append(kept, r)
			} else {
				slog.Warn("skipping invalid conversation record", "project", project, "id", r.ID)
			}
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
		title = "Conversation " + id
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

// EnsureRecord returns the record for id, creating one (with that id) if it
// does not exist — keeps old clients and test probes working.
func (s *Store) EnsureRecord(project, id string) (Record, error) {
	if rec, ok := s.Get(project, id); ok {
		return rec, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.byProject[project] { // re-check under lock
		if r.ID == id {
			return r, nil
		}
	}
	rec := Record{
		ID:           id,
		Title:        "Conversation " + id,
		AgentSession: newUUID(),
		CreatedAt:    time.Now().UTC(),
		LastOpened:   time.Now().UTC(),
	}
	s.byProject[project] = append(s.byProject[project], rec)
	return rec, s.save()
}

// Touch updates LastOpened.
func (s *Store) Touch(project, id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, r := range s.byProject[project] {
		if r.ID == id {
			s.byProject[project][i].LastOpened = time.Now().UTC()
			_ = s.save()
			return
		}
	}
}

// Rename sets a conversation's title.
func (s *Store) Rename(project, id, title string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, r := range s.byProject[project] {
		if r.ID == id {
			s.byProject[project][i].Title = title
			return s.save()
		}
	}
	return fmt.Errorf("conversation %s not found", id)
}

// Delete removes a record. The agent's transcript files in the shared home
// are deliberately left alone (ADR-011).
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
