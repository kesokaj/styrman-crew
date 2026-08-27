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

// Prefix namespaces styrman's tmux sessions; anything else in tmux belongs
// to the user. Kept as "conv-" so sessions created before the rename
// (ADR-016) keep working.
const Prefix = "conv-"

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
	// Bell: the session rang the terminal bell while unattended — an agent
	// is likely waiting for input. Cleared by tmux when viewed.
	Bell bool `json:"bell"`
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
func (m *Manager) liveSessions(ctx context.Context, user, project, guest, prefix string) ([]Live, error) {
	st, err := m.orch.Status(ctx, user, project)
	if err != nil {
		return nil, err
	}
	if st.State != "running" {
		return []Live{}, nil
	}
	code, out, err := m.orch.Exec(ctx, st.ContainerID, orchestrator.ExecSpec{
		User: guest,
		// Trailing ':' pins exit 0 — a failing list-windows (e.g. the last
		// session dying between the two commands) must not discard a good
		// list-sessions result; the parser tolerates partial output.
		Cmd: []string{"sh", "-c",
			"tmux list-sessions -F '#{session_name}|#{session_attached}' 2>/dev/null; " +
				"tmux list-windows -a -F 'BELL|#{session_name}|#{window_bell_flag}' 2>/dev/null; :"},
		Env: []string{"HOME=/home/" + guest, "USER=" + guest},
	})
	if err != nil {
		return nil, err
	}
	if code != 0 { // "no server running" — no sessions
		return []Live{}, nil
	}
	return parseSessions(out, prefix), nil
}

// parseSessions turns the combined list-sessions + list-windows output into
// Live records. Session lines are "name|attached"; window lines are
// "BELL|name|flag" and only mark bells on sessions that also listed.
func parseSessions(out, prefix string) []Live {
	bells := map[string]bool{}
	var lives []Live
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		parts := strings.Split(line, "|")
		if len(parts) == 3 && parts[0] == "BELL" {
			if parts[2] != "0" {
				bells[parts[1]] = true
			}
			continue
		}
		if len(parts) != 2 || !strings.HasPrefix(parts[0], prefix) {
			continue
		}
		lives = append(lives, Live{
			ID:       strings.TrimPrefix(parts[0], prefix),
			Attached: parts[1] != "0",
		})
	}
	for i := range lives {
		lives[i].Bell = bells[prefix+lives[i].ID]
	}
	if lives == nil {
		lives = []Live{}
	}
	return lives
}

// Live returns live tmux state keyed by terminal id.
func (m *Manager) Live(ctx context.Context, user, project, guest string) (map[string]Live, error) {
	lives, err := m.liveSessions(ctx, user, project, guest, Prefix)
	if err != nil {
		return nil, err
	}
	out := map[string]Live{}
	for _, l := range lives {
		out[l.ID] = l
	}
	return out, nil
}

// KillSession ends a terminal's tmux session.
func (m *Manager) KillSession(ctx context.Context, user, project, guest, prefix, id string) error {
	if err := ValidateID(id); err != nil {
		return err
	}
	st, err := m.orch.Status(ctx, user, project)
	if err != nil {
		return err
	}
	if st.State != "running" {
		return nil
	}
	code, out, err := m.orch.Exec(ctx, st.ContainerID, orchestrator.ExecSpec{
		User: guest,
		Cmd:  []string{"tmux", "kill-session", "-t", prefix + id},
		Env:  []string{"HOME=/home/" + guest, "USER=" + guest},
	})
	if err != nil {
		return err
	}
	if code != 0 && !strings.Contains(out, "can't find session") && !strings.Contains(out, "no server running") {
		return fmt.Errorf("tmux kill-session: %s", strings.TrimSpace(out))
	}
	return nil
}
