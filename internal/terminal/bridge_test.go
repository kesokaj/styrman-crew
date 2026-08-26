package terminal

import (
	"strings"
	"testing"
)

func TestPaneCommand(t *testing.T) {
	uuid := "0d9cf1e2-3a4b-4c5d-8e6f-0123456789ab"
	tests := []struct {
		name, agent, cmd, session string
		want                      string
		wantContains              []string
	}{
		{
			name:  "claude with valid uuid resumes first",
			agent: "claude", cmd: "claude", session: uuid,
			wantContains: []string{"--resume " + uuid, "--session-id " + uuid, "||"},
		},
		{
			name:  "agy ignores session",
			agent: "agy", cmd: "agy", session: uuid,
			want: "bash -lc 'exec agy'",
		},
		{
			name:  "claude with empty session falls back to plain launch",
			agent: "claude", cmd: "claude", session: "",
			want: "bash -lc 'exec claude'",
		},
		{
			name:  "claude with malformed session (shell injection attempt) is not interpolated",
			agent: "claude", cmd: "claude", session: "'; rm -rf ~; '",
			want: "bash -lc 'exec claude'",
		},
		{
			name:  "uppercase uuid rejected (store generates lowercase)",
			agent: "claude", cmd: "claude", session: strings.ToUpper(uuid),
			want: "bash -lc 'exec claude'",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := paneCommand(tt.agent, tt.cmd, tt.session)
			if tt.want != "" && got != tt.want {
				t.Errorf("paneCommand() = %q, want %q", got, tt.want)
			}
			for _, sub := range tt.wantContains {
				if !strings.Contains(got, sub) {
					t.Errorf("paneCommand() = %q, missing %q", got, sub)
				}
			}
			// Nothing outside the generated UUID may reach the shell string
			// from the session parameter.
			if tt.session != uuid && tt.session != "" && strings.Contains(got, tt.session) {
				t.Errorf("unvalidated session leaked into command: %q", got)
			}
		})
	}
}
