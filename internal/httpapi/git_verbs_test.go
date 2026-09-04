package httpapi

import "testing"

// Validation for the strings this package turns into git argv.
//
// `gitExec` builds a []string and never a shell string, so quoting is not the
// exposure — option injection (a leading "-" read as a flag) and refspec
// traversal are. This is the one piece of that surface that is a pure function
// and therefore unit-testable; the handlers themselves are covered by tier-2
// integration against a real VM, because `Server.orch` is a concrete
// *orchestrator.Orchestrator with no exec seam to fake (the same gap already
// recorded for checkFile/formatFile in TODO.md and as BUG-003's residual).
func TestValidCommitHash(t *testing.T) {
	valid := []string{
		"abc1234", // the 7-char abbreviation git prints
		"ABC1234", // uppercase hex is a real object name
		"0123456789abcdef0123456789abcdef01234567", // full 40
		"deadbeef",
	}
	for _, h := range valid {
		if !validCommitHash(h) {
			t.Errorf("validCommitHash(%q) = false, want true", h)
		}
	}

	invalid := []struct{ in, why string }{
		{"", "empty"},
		{"abc123", "6 chars, shorter than git's own abbreviation"},
		{"0123456789abcdef0123456789abcdef012345678", "41 chars, longer than a sha1"},
		{"--output=/tmp/x", "leading dash would be read as an option, not a revision"},
		{"-abc1234", "leading dash"},
		{"abc123g", "g is not hex"},
		{"HEAD", "a ref name, not a hash"},
		{"HEAD@{1}", "reflog syntax"},
		{"abc1234^", "parent syntax"},
		{"abc1234..def5678", "range syntax"},
		{"abc1234 --output=x", "embedded space and flag"},
		{"../../etc/passwd", "path traversal"},
		{"abc1234\nrm -rf /", "embedded newline"},
		{"abc1234;id", "shell metacharacter"},
		{"abc1234\x00", "embedded NUL"},
	}
	for _, c := range invalid {
		if validCommitHash(c.in) {
			t.Errorf("validCommitHash(%q) = true, want false (%s)", c.in, c.why)
		}
	}
}
