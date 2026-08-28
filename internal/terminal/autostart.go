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
	esc := strings.ReplaceAll(cmd, "'", `'\''`)
	return "bash -lc '" + esc + "\nexec bash -l'"
}
