package terminal

import "testing"

func TestAutostartPaneCmd(t *testing.T) {
	cases := map[string]string{
		// NO automatic --continue rewrite (2026-08-30 review: all terminals
		// of a project share one cwd, so auto-resume collided across them).
		// Users opt in per terminal by setting the flag in their autostart.
		"claude":            "bash -lc 'claude\nexec bash -l'",
		"agy --continue":    "bash -lc 'agy --continue\nexec bash -l'",
		"claude -p hi":      "bash -lc 'claude -p hi\nexec bash -l'",
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
