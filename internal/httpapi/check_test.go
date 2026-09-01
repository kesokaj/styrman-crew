package httpapi

import (
	"strings"
	"testing"
)

// The fixtures below are REAL output captured from the tools in
// styrman-runtime:dev on 2026-09-01 (not invented), because every checker has
// its own format and guessing at them is exactly how a syntax checker ends up
// silently reporting nothing.
func TestParseCheckOutput(t *testing.T) {
	cases := []struct {
		name    string
		tool    string
		out     string
		wantLen int
		line    int
		col     int
		sev     string
		substr  string
	}{
		{
			name:    "gofmt parse error",
			tool:    "gofmt",
			out:     "a.go:4:1: expected operand, found '}'\na.go:4:3: expected ';', found 'EOF'\n",
			wantLen: 2, line: 4, col: 1, sev: "error", substr: "expected operand",
		},
		{
			name:    "shellcheck gcc format with severity",
			tool:    "shellcheck",
			out:     "b.sh:2:1: warning: foo appears unused. Verify use (or export if used externally). [SC2034]\nb.sh:3:6: note: Double quote to prevent globbing. [SC2086]\n",
			wantLen: 2, line: 2, col: 1, sev: "warning", substr: "appears unused",
		},
		{
			name:    "hadolint json",
			tool:    "hadolint",
			out:     `[{"code":"DL3006","column":1,"file":"Dockerfile","level":"warning","line":1,"message":"Always tag the version of an image explicitly"}]`,
			wantLen: 1, line: 1, col: 1, sev: "warning", substr: "DL3006: Always tag",
		},
		{
			name:    "bash -n syntax error",
			tool:    "bash",
			out:     "a.sh: line 4: syntax error: unexpected end of file\n",
			wantLen: 1, line: 4, col: 0, sev: "error", substr: "unexpected end of file",
		},
		{
			name:    "python py_compile",
			tool:    "python",
			out:     "  File \"a.py\", line 1\n    def f(:\n          ^\nSyntaxError: invalid syntax\n",
			wantLen: 1, line: 1, col: 0, sev: "error", substr: "SyntaxError: invalid syntax",
		},
		{
			name:    "node --check",
			tool:    "node",
			out:     "/tmp/a.js:2\n\n\n\nSyntaxError: Unexpected end of input\n",
			wantLen: 1, line: 2, col: 0, sev: "error", substr: "Unexpected end of input",
		},
		{
			name:    "jq parse error",
			tool:    "jq",
			out:     "jq: parse error: Expected another key-value pair at line 1, column 9\n",
			wantLen: 1, line: 1, col: 9, sev: "error", substr: "Expected another key-value pair",
		},
		{
			name:    "yq load error",
			tool:    "yq",
			out:     "Error: bad file 'a.yaml': go-yaml load error in scanner at L2.C4: mapping values are not allowed in this context\n",
			wantLen: 1, line: 2, col: 4, sev: "error", substr: "mapping values are not allowed",
		},
		{
			name:    "unparseable output still surfaces (never a silent pass)",
			tool:    "gofmt",
			out:     "something entirely unexpected happened\n",
			wantLen: 1, line: 1, col: 0, sev: "error", substr: "something entirely unexpected",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ds := parseCheckOutput(c.tool, c.out)
			if len(ds) != c.wantLen {
				t.Fatalf("got %d diagnostics, want %d: %+v", len(ds), c.wantLen, ds)
			}
			d := ds[0]
			if d.Line != c.line {
				t.Errorf("line = %d, want %d", d.Line, c.line)
			}
			if d.Col != c.col {
				t.Errorf("col = %d, want %d", d.Col, c.col)
			}
			if d.Severity != c.sev {
				t.Errorf("severity = %q, want %q", d.Severity, c.sev)
			}
			if !strings.Contains(d.Message, c.substr) {
				t.Errorf("message %q does not contain %q", d.Message, c.substr)
			}
		})
	}
}

// Clean output must mean NO diagnostics — the pass case has to be real.
func TestParseCheckOutputClean(t *testing.T) {
	for _, tool := range []string{"gofmt", "shellcheck", "hadolint", "bash", "python", "node", "jq", "yq"} {
		if ds := parseCheckOutput(tool, ""); len(ds) != 0 {
			t.Errorf("%s: empty output produced %+v", tool, ds)
		}
		if ds := parseCheckOutput(tool, "\n  \n"); len(ds) != 0 {
			t.Errorf("%s: blank output produced %+v", tool, ds)
		}
	}
	// hadolint reports a clean Dockerfile as an empty JSON array.
	if ds := parseCheckOutput("hadolint", "[]"); len(ds) != 0 {
		t.Errorf("hadolint []: produced %+v", ds)
	}
}

func TestCheckerFor(t *testing.T) {
	cases := []struct{ ext, base, want string }{
		{"go", "main.go", "gofmt"},
		{"sh", "run.sh", "shellcheck"},
		{"py", "a.py", "python3"},
		{"js", "a.js", "node"},
		{"json", "a.json", "jq"},
		{"yaml", "a.yaml", "yq"},
		{"yml", "a.yml", "yq"},
		{"tf", "main.tf", "tofu"},
		// Dockerfiles are matched by BASENAME, like the editor's grammar
		// lookup — they have no extension.
		{"", "Dockerfile", "hadolint"},
		{"", "dockerfile", "hadolint"},
		{"dev", "Dockerfile.dev", "hadolint"},
		{"", "Containerfile", "hadolint"},
		// Nothing sensible to run.
		{"txt", "notes.txt", ""},
		{"", "README", ""},
	}
	for _, c := range cases {
		got := ""
		if ch := checkerFor(c.ext, c.base, false); ch != nil {
			got = ch.tool
		}
		if got != c.want {
			t.Errorf("checkerFor(%q,%q) = %q, want %q", c.ext, c.base, got, c.want)
		}
	}
}

// -s bash must be added ONLY when the file has no shebang: it overrides a real
// one, which would silently drop dialect-specific findings (SC3014 &c.).
func TestShellDialectFlag(t *testing.T) {
	noShebang := checkerFor("sh", "a.sh", false)
	withShebang := checkerFor("sh", "a.sh", true)
	if !strings.Contains(noShebang.cmd, "-s bash") {
		t.Error("a shebang-less script should get -s bash (else SC2148 fires on every clean fragment)")
	}
	if strings.Contains(withShebang.cmd, "-s bash") {
		t.Error("-s bash must NOT override an explicit shebang")
	}
	if !hasShebang("#!/bin/sh\necho hi") || hasShebang("echo hi") {
		t.Error("hasShebang misdetects")
	}
}

func TestMissingTool(t *testing.T) {
	if !missingTool("bash: line 1: hadolint: command not found") {
		t.Error("did not detect a missing tool")
	}
	// The failure mode this guards: missingTool DISCARDS every diagnostic, so a
	// false positive turns a broken file into a clean one. Real tool output
	// that merely contains "not found" must not trip it.
	for _, real := range []string{
		"a.go:1:1: expected declaration",
		"a.py:3:1: error: module 'x' not found [E0401]",
		"b.sh:2:1: warning: command not found is a common typo [SC1234]\nb.sh:3:1: error: real problem",
		"Dockerfile:1 DL3006 warning: base image not found in allowlist",
	} {
		if missingTool(real) {
			t.Errorf("false positive — diagnostics would be discarded for: %q", real)
		}
	}
}

// TestGuestScriptsNeverWaitForStdinEOF guards a DATA-LOSS regression.
//
// orchestrator.Exec deliberately never half-closes the exec stream: doing so
// tore down the whole bidirectional connection and truncated everything the
// guest printed after it finished reading stdin. That silently made /format
// return an EMPTY document for every request — and the editor pushes that
// straight into the buffer, so a Format+Save wiped the file.
//
// The invariant that keeps it fixed is that the guest reads an EXACT byte
// count. Nothing else in `go test` would notice if `cat >` came back, so this
// asserts it directly.
func TestGuestScriptsNeverWaitForStdinEOF(t *testing.T) {
	scripts := map[string]string{
		"check":  checkScript("sh", 42, `shellcheck -f gcc "$F"`),
		"format": formatScript("go", 42, `gofmt -w "$F"`),
	}
	for name, script := range scripts {
		if !strings.Contains(script, "head -c 42") {
			t.Errorf("%s: script must read an exact byte count (`head -c <len>`); got:\n%s", name, script)
		}
		// A bare `cat > "$F"` blocks until stdin EOF, which never comes.
		if strings.Contains(script, `cat > "$F"`) {
			t.Errorf("%s: script waits for stdin EOF (`cat > \"$F\"`) — it will hang or truncate:\n%s", name, script)
		}
	}
}

// TestSafeSuffixBlocksInjection guards a command-injection path: the suffix is
// interpolated into `mktemp --suffix=.%s` inside a `bash -c` script, and the
// Dockerfile branch matches on the basename, so it accepts an "extension" that
// never went through the switch allowlist. "Dockerfile.$(id)" used to reach the
// shell as live command substitution.
func TestSafeSuffixBlocksInjection(t *testing.T) {
	nasty := []string{
		"$(id)", "`id`", "; rm -rf /", "a b", "..", "/etc/passwd",
		"$(curl evil)", "a;b", "a|b", "a&b", "'", `"`, "\n",
	}
	for _, in := range nasty {
		got := safeSuffix(in)
		for _, r := range got {
			ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
			if !ok {
				t.Errorf("safeSuffix(%q) = %q — contains shell-significant %q", in, got, r)
			}
		}
		if got == "" {
			t.Errorf("safeSuffix(%q) returned empty", in)
		}
	}
	// The end-to-end shape: the generated script must not contain a live
	// substitution for a hostile filename.
	c := checkerFor("$(id)", "Dockerfile.$(id)", false)
	if c == nil {
		t.Fatal("expected the Dockerfile branch to match")
	}
	script := checkScript(safeSuffix("$(id)"), 10, c.cmd)
	if strings.Contains(script, "$(id)") {
		t.Errorf("injection survives into the script:\n%s", script)
	}
	// Ordinary extensions must survive untouched.
	for _, ok := range []string{"go", "sh", "json", "yaml", "tsx"} {
		if safeSuffix(ok) != ok {
			t.Errorf("safeSuffix(%q) = %q, want unchanged", ok, safeSuffix(ok))
		}
	}
}

// The 5 MiB guard in checkFile/formatFile is only reachable if the CSRF
// guard's MaxBytesReader allows a body that large. It defaults to 1 MiB, which
// made those guards dead code and turned an oversize request into a confusing
// 400 rather than the intended 413.
func TestCheckAndFormatGetTheLargeBodyLimit(t *testing.T) {
	for _, p := range []string{
		"/api/projects/x/check",
		"/api/projects/x/format",
		"/api/projects/x/file",
	} {
		if !strings.HasSuffix(p, "/check") &&
			!strings.HasSuffix(p, "/format") &&
			!strings.HasSuffix(p, "/file") {
			t.Fatalf("test is wrong: %s", p)
		}
	}
	// The content cap must be reachable under the raised body limit.
	if maxCheckBytes > 8<<20 {
		t.Errorf("maxCheckBytes (%d) exceeds the 8 MiB body limit — unreachable", maxCheckBytes)
	}
	if maxCheckBytes <= 1<<20 {
		t.Errorf("maxCheckBytes (%d) is below the default 1 MiB body cap — suspicious", maxCheckBytes)
	}
}
