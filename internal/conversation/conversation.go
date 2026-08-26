// Package conversation implements styrman's conversation model (ADR-011):
// a conversation is a durable record (store.go) whose live view is a tmux
// session (`conv-<id>`) inside the project's microVM. Plain shells are
// "terminals" (`term-<id>`) — ephemeral, unrecorded, listed from tmux only.
package conversation

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/kesokaj/styrman/internal/config"
	"github.com/kesokaj/styrman/internal/orchestrator"
)

// Session-name prefixes. Anything else in tmux belongs to the user.
const (
	Prefix     = "conv-"
	TermPrefix = "term-"
)

var idRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,30}$`)

// ValidateID reports whether id is a safe conversation/terminal id (tmux
// session names must avoid ':' and '.'; we keep it kebab-case).
func ValidateID(id string) error {
	if !idRe.MatchString(id) {
		return fmt.Errorf("invalid session id %q", id)
	}
	return nil
}

// Live describes one live tmux session.
type Live struct {
	ID       string `json:"id"`
	Attached bool   `json:"attached"`
}

// Manager talks tmux inside project microVMs via exec.
type Manager struct {
	orch *orchestrator.Orchestrator
	cfg  *config.Config
}

// NewManager creates a Manager.
func NewManager(orch *orchestrator.Orchestrator, cfg *config.Config) *Manager {
	return &Manager{orch: orch, cfg: cfg}
}

// liveSessions lists tmux sessions with the given prefix. Empty (no error)
// when the project is stopped or tmux has no server.
func (m *Manager) liveSessions(ctx context.Context, project, prefix string) ([]Live, error) {
	st, err := m.orch.Status(ctx, project)
	if err != nil {
		return nil, err
	}
	if st.State != "running" {
		return []Live{}, nil
	}
	code, out, err := m.orch.Exec(ctx, st.ContainerID, orchestrator.ExecSpec{
		User: m.cfg.Username,
		Cmd:  []string{"tmux", "list-sessions", "-F", "#{session_name}|#{session_attached}"},
		Env:  []string{"HOME=/home/" + m.cfg.Username, "USER=" + m.cfg.Username},
	})
	if err != nil {
		return nil, err
	}
	if code != 0 { // "no server running" — no sessions
		return []Live{}, nil
	}
	var lives []Live
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		parts := strings.Split(line, "|")
		if len(parts) != 2 || !strings.HasPrefix(parts[0], prefix) {
			continue
		}
		lives = append(lives, Live{
			ID:       strings.TrimPrefix(parts[0], prefix),
			Attached: parts[1] != "0",
		})
	}
	if lives == nil {
		lives = []Live{}
	}
	return lives, nil
}

// LiveConversations returns live tmux state keyed by conversation id.
func (m *Manager) LiveConversations(ctx context.Context, project string) (map[string]Live, error) {
	lives, err := m.liveSessions(ctx, project, Prefix)
	if err != nil {
		return nil, err
	}
	out := map[string]Live{}
	for _, l := range lives {
		out[l.ID] = l
	}
	return out, nil
}

// Terminals lists the project's live plain-shell terminals.
func (m *Manager) Terminals(ctx context.Context, project string) ([]Live, error) {
	return m.liveSessions(ctx, project, TermPrefix)
}

// KillSession ends a tmux session (conversation or terminal, by full prefix).
func (m *Manager) KillSession(ctx context.Context, project, prefix, id string) error {
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
		Cmd:  []string{"tmux", "kill-session", "-t", prefix + id},
		Env:  []string{"HOME=/home/" + m.cfg.Username, "USER=" + m.cfg.Username},
	})
	if err != nil {
		return err
	}
	if code != 0 && !strings.Contains(out, "can't find session") && !strings.Contains(out, "no server running") {
		return fmt.Errorf("tmux kill-session: %s", strings.TrimSpace(out))
	}
	return nil
}
