// Package conversation maps styrman conversations onto tmux sessions inside a
// project's microVM (R6). A conversation exists iff its tmux session does; the
// terminal bridge creates sessions lazily with `tmux new -A`.
package conversation

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/kesokaj/styrman/internal/config"
	"github.com/kesokaj/styrman/internal/orchestrator"
)

// Prefix namespaces styrman's tmux sessions; other sessions the user starts
// manually are left alone.
const Prefix = "conv-"

var idRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,30}$`)

// ValidateID reports whether id is a safe conversation id (tmux session names
// must avoid ':' and '.'; we keep it kebab-case like project names).
func ValidateID(id string) error {
	if !idRe.MatchString(id) {
		return fmt.Errorf("invalid conversation id %q", id)
	}
	return nil
}

// Conversation is one tmux session in a project.
type Conversation struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"createdAt"`
	Attached  bool      `json:"attached"`
}

// Manager lists and kills conversations via exec.
type Manager struct {
	orch *orchestrator.Orchestrator
	cfg  *config.Config
}

// NewManager creates a Manager.
func NewManager(orch *orchestrator.Orchestrator, cfg *config.Config) *Manager {
	return &Manager{orch: orch, cfg: cfg}
}

// List returns the project's conversations (empty if the container is not
// running — stopped projects have no live tmux server).
func (m *Manager) List(ctx context.Context, project string) ([]Conversation, error) {
	st, err := m.orch.Status(ctx, project)
	if err != nil {
		return nil, err
	}
	if st.State != "running" {
		return []Conversation{}, nil
	}
	code, out, err := m.orch.Exec(ctx, st.ContainerID, orchestrator.ExecSpec{
		User: m.cfg.Username,
		Cmd:  []string{"tmux", "list-sessions", "-F", "#{session_name}|#{session_created}|#{session_attached}"},
		Env:  []string{"HOME=/home/" + m.cfg.Username, "USER=" + m.cfg.Username},
	})
	if err != nil {
		return nil, err
	}
	// tmux exits 1 with "no server running" when there are no sessions.
	if code != 0 {
		return []Conversation{}, nil
	}
	var convs []Conversation
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		parts := strings.Split(line, "|")
		if len(parts) != 3 || !strings.HasPrefix(parts[0], Prefix) {
			continue
		}
		created := time.Time{}
		if ts, err := strconv.ParseInt(parts[1], 10, 64); err == nil {
			created = time.Unix(ts, 0)
		}
		convs = append(convs, Conversation{
			ID:        strings.TrimPrefix(parts[0], Prefix),
			CreatedAt: created,
			Attached:  parts[2] != "0",
		})
	}
	if convs == nil {
		convs = []Conversation{}
	}
	return convs, nil
}

// Kill ends a conversation's tmux session (and whatever runs inside it).
func (m *Manager) Kill(ctx context.Context, project, id string) error {
	if err := ValidateID(id); err != nil {
		return err
	}
	st, err := m.orch.Status(ctx, project)
	if err != nil {
		return err
	}
	if st.State != "running" {
		return nil
	}
	code, out, err := m.orch.Exec(ctx, st.ContainerID, orchestrator.ExecSpec{
		User: m.cfg.Username,
		Cmd:  []string{"tmux", "kill-session", "-t", Prefix + id},
		Env:  []string{"HOME=/home/" + m.cfg.Username, "USER=" + m.cfg.Username},
	})
	if err != nil {
		return err
	}
	if code != 0 && !strings.Contains(out, "can't find session") {
		return fmt.Errorf("tmux kill-session: %s", strings.TrimSpace(out))
	}
	return nil
}
