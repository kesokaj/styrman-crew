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
	// Native agent resume (ADR-011, unparked v1.14.0): the autostart runs on
	// every session CREATION — including re-creations after a pause or VM
	// restart — so a bare agent command would start a FRESH conversation and
	// lose the agent's context each time. Both CLIs ship a --continue that
	// resumes the most recent conversation for this workspace; fall back to
	// a fresh start when there is nothing to resume (first open). Only the
	// EXACT bare command is rewritten — flagged/composed commands are the
	// user's own business.
	switch strings.TrimSpace(cmd) {
	case "claude":
		cmd = "claude --continue || claude"
	case "agy":
		cmd = "agy --continue || agy"
	}
	esc := strings.ReplaceAll(cmd, "'", `'\''`)
	return "bash -lc '" + esc + "\nexec bash -l'"
}
