package httpapi

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
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

// The 5 MiB content guard in checkFile/formatFile is only reachable if
// budgetGuard's MaxBytesReader (csrfGuard's until v1.42.4) allows a body
// that large. It defaults to 1 MiB,
// which made those guards dead code and turned an oversize request into a
// confusing 400 instead of the intended 413.
//
// This drives the REAL middleware. An earlier version of this test asserted
// that three string literals ended with suffixes the test itself had written
// into them — deleting the "/check" case from server.go left it green (review
// finding: "theatre"). This one fails if that case is removed.
func TestCheckAndFormatGetTheLargeBodyLimit(t *testing.T) {
	// Echo how many bytes the handler could actually read.
	sink := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n, err := io.Copy(io.Discard, r.Body)
		if err != nil {
			http.Error(w, "too large", http.StatusRequestEntityTooLarge)
			return
		}
		fmt.Fprintf(w, "%d", n)
	})
	// The cap follows the matched route (bodyCap), so register the sink on
	// the real patterns behind the real budget middleware.
	mux := http.NewServeMux()
	for _, p := range []string{"POST /api/projects/{project}/check", "POST /api/projects/{project}/format",
		"PUT /api/projects/{project}/file", "POST /api/projects/{project}/terminals"} {
		mux.Handle(p, sink)
	}
	h := newNetHarness(t).s.budgetGuard(mux)

	// 2 MiB — over the 1 MiB default, under the 8 MiB file limit. This is a
	// realistic editor payload: a file that size opens and saves.
	body := make([]byte, 2<<20)
	for _, tc := range []struct {
		method, path string
		wantAllow    bool
	}{
		{http.MethodPost, "/api/projects/p1/check", true},
		{http.MethodPost, "/api/projects/p1/format", true},
		{http.MethodPut, "/api/projects/p1/file", true},        // control: already had the limit
		{http.MethodPost, "/api/projects/p1/terminals", false}, // control: small-JSON default
	} {
		req := httptest.NewRequest(tc.method, tc.path, bytes.NewReader(body))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		allowed := rec.Code == http.StatusOK && rec.Body.String() == fmt.Sprint(len(body))
		if allowed != tc.wantAllow {
			t.Errorf("%s: read-2MiB allowed=%v want=%v (status %d, body %q)",
				tc.path, allowed, tc.wantAllow, rec.Code, rec.Body.String())
		}
	}

	// And the content cap must sit inside that window to be reachable at all.
	if maxCheckBytes <= 1<<20 || maxCheckBytes > 8<<20 {
		t.Errorf("maxCheckBytes (%d) is unreachable: it must exceed the 1 MiB default and fit the 8 MiB route limit", maxCheckBytes)
	}
}

// tofu fmt -check prints bare file names (not formatted) or an `Error:` block
// (parse failure) — neither gcc-shaped; the tofu case turns each into one
// diagnostic so a misformatted .tf file is never shown as clean (review of
// the close-out's phase 3).
func TestParseCheckOutputTofu(t *testing.T) {
	if ds := parseCheckOutput("tofu", ""); len(ds) != 0 {
		t.Fatalf("clean: %v", ds)
	}
	ds := parseCheckOutput("tofu", "/home/x/.styrman/tmp/tmp.abc.tf\n")
	if len(ds) != 1 || ds[0].Severity != "warning" || ds[0].Line != 1 || !strings.Contains(ds[0].Message, "tofu fmt") {
		t.Fatalf("not formatted: %+v", ds)
	}
	ds = parseCheckOutput("tofu", "Error: Argument or block definition required\n\n  on tmp.tf line 3:\n   3: resource \"x\" {\n╷\n│ boom\n╵\n")
	if len(ds) != 1 || ds[0].Severity != "error" || !strings.Contains(ds[0].Message, "Argument or block") {
		t.Fatalf("parse error: %+v", ds)
	}
}

// The CHECK temp file lives OUTSIDE the workspace (carried-over review low,
// 2026-09-01): a regression back to `mktemp -p .` would put editor content
// in the project tree on every debounce tick. FORMAT is the deliberate
// opposite since the 2026-09-13 audit — formatters resolve config from the
// file's path — with its own guards (format_test.go); it must never fall
// back to ~/.styrman/tmp, where no project config is found.
func TestScriptsKeepTempFilesOutOfTheWorkspace(t *testing.T) {
	s := checkScript("go", 1, "true")
	if !strings.Contains(s, `mktemp -p "$D"`) || !strings.Contains(s, `D="$HOME/.styrman/tmp"`) {
		t.Errorf("check script does not use the ~/.styrman/tmp dir:\n%s", s)
	}
	if strings.Contains(s, "mktemp -p .") {
		t.Errorf("check script still creates its temp file in the workspace")
	}
	if f := formatScript("go", 1, "true"); strings.Contains(f, `.styrman/tmp`) {
		t.Errorf("format script fell back to ~/.styrman/tmp, where no project config is discovered:\n%s", f)
	}
	if s := checkScript("go", 1, "true"); strings.Contains(s, `${F}.err`) {
		t.Error("check trap names a file it never creates")
	}
	if s := formatScript("go", 1, "true"); strings.Contains(s, `${F}.out`) {
		t.Error("format trap names a file it never creates")
	}
}
