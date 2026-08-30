package terminal

import "testing"

func TestAutostartPaneCmd(t *testing.T) {
	cases := map[string]string{
		// Bare agent commands resume natively on session re-creation (ADR-011
		// unparked, v1.14.0), falling back to a fresh start.
		"claude": "bash -lc 'claude --continue || claude\nexec bash -l'",
		"agy":    "bash -lc 'agy --continue || agy\nexec bash -l'",
		// Flagged/composed agent commands are left untouched.
		"claude -p hi": "bash -lc 'claude -p hi\nexec bash -l'",
		"docker compose up":  "bash -lc 'docker compose up\nexec bash -l'",
		"npm run dev &":      "bash -lc 'npm run dev &\nexec bash -l'",
		"echo hi # note":     "bash -lc 'echo hi # note\nexec bash -l'",
		`echo 'it''s'`:       "bash -lc 'echo '\\''it'\\'''\\''s'\\''\nexec bash -l'",
		`echo "$HOME" \$(x)`: "bash -lc 'echo \"$HOME\" \\$(x)\nexec bash -l'",
	}
	for in, want := range cases {
		if got := autostartPaneCmd(in); got != want {
			t.Errorf("autostartPaneCmd(%q):\n got  %q\n want %q", in, got, want)
		}
	}
}
