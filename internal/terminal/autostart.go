package terminal

import "strings"

// autostartPaneCmd wraps a terminal's autostart command so it runs once at
// session creation and then hands the pane to a login shell. The command is
// single-quote-escaped (backslash-quote form) and separated from the exec by a NEWLINE, not
// ";" — a command ending in "&", "|" or a "# comment" would turn ";" into a
// bash syntax error (or swallow the exec) and kill the pane instantly
// (review finding). Runs as the user in their own VM — quoting is for
// correctness, not a security boundary.
func autostartPaneCmd(cmd string) string {
	// Native agent resume is DELIBERATELY NOT auto-applied (2026-08-30
	// review, revising the v1.14.0 change): both CLIs key --continue on the
	// most recent conversation FOR THE CWD, and every terminal of a project
	// shares one cwd — an automatic rewrite made two terminals resume the
	// SAME conversation after a VM restart. Users who want resume set their
	// autostart to `claude --continue` / `agy --continue` themselves (both
	// CLIs start fresh, exit 0, when there is nothing to resume — verified
	// live), choosing per terminal instead of the platform guessing.
	esc := strings.ReplaceAll(cmd, "'", `'\''`)
	return "bash -lc '" + esc + "\nexec bash -l'"
}
