package terminal

import "testing"

func TestAutostartPaneCmd(t *testing.T) {
	cases := map[string]string{
		"docker compose up":   "bash -lc 'docker compose up\nexec bash -l'",
		"npm run dev &":       "bash -lc 'npm run dev &\nexec bash -l'",
		"echo hi # note":      "bash -lc 'echo hi # note\nexec bash -l'",
		`echo 'it''s'`:        "bash -lc 'echo '\\''it'\\'''\\''s'\\''\nexec bash -l'",
		`echo "$HOME" \$(x)`:  "bash -lc 'echo \"$HOME\" \\$(x)\nexec bash -l'",
	}
	for in, want := range cases {
		if got := autostartPaneCmd(in); got != want {
			t.Errorf("autostartPaneCmd(%q):\n got  %q\n want %q", in, got, want)
		}
	}
}
