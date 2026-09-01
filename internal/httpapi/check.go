package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/kesokaj/styrman-crew/internal/orchestrator"
)

// Syntax checking runs the language's real CLI checker INSIDE the project VM,
// the same shape as formatting (format.go): the editor content is piped over
// stdin to a temp file in the workspace root (so checkers find the project's
// own config), and the guest reports which tool actually ran.
//
// Every checker has its OWN output format — there is no shared "gcc format"
// to lean on (measured 2026-09-01: gofmt and shellcheck are gcc-ish, hadolint
// speaks JSON, and bash/python/node/jq/yq each invent their own). So each tool
// gets a parser, all of them unit-tested against real captured output in
// check_test.go, with a raw-output fallback so a format drift degrades to a
// visible message rather than a silent pass.

// maxCheckBytes bounds the content we will check. Same limit as formatting.
const maxCheckBytes = maxFormatBytes

// checkDiagnostic is one problem, addressed to a line in the editor.
type checkDiagnostic struct {
	Line int `json:"line"`
	Col  int `json:"col,omitempty"`
	// Severity is "error", "warning" or "info" — CodeMirror's lint severities.
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

// checker is how one file type is checked in the guest.
type checker struct {
	// tool is the primary binary, reported to the UI so a missing checker can
	// say WHICH one is missing instead of failing anonymously.
	tool string
	// cmd is a shell fragment; $F is the temp file. It must print the tool it
	// actually used as `STYRMAN_TOOL <name>` so the fallback branches (e.g.
	// shellcheck → bash -n on an older image) select the right parser.
	cmd string
}

// safeSuffix reduces a caller-derived extension to characters that cannot mean
// anything to a shell. The suffix is interpolated into `mktemp --suffix=.%s`
// inside a `bash -c` script, and the Dockerfile branch of checkerFor matches on
// the BASENAME, so it accepts a file whose "extension" never passed the switch
// allowlist: "Dockerfile.$(id)" yielded ext=`$(id)` and executed it (review
// finding, verified reachable). Empty/oversized input falls back to "txt".
func safeSuffix(ext string) string {
	var b strings.Builder
	for _, r := range ext {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
		if b.Len() >= 16 {
			break
		}
	}
	if b.Len() == 0 {
		return "txt"
	}
	return b.String()
}

// hasShebang reports whether the content starts with "#!" — shellcheck infers
// the shell dialect from it, and only needs a default when it is absent.
func hasShebang(content string) bool { return strings.HasPrefix(content, "#!") }

// checkerFor maps a file extension (or basename, for extensionless types like
// Dockerfile) to its checker, or nil when we have none. Deliberately narrower
// than formatInPlace: a formatter that rewrites a file is useful everywhere,
// but a "syntax check" that cannot actually find syntax errors would be a lie.
func checkerFor(ext, base string, shebang bool) *checker {
	if strings.EqualFold(base, "Dockerfile") ||
		strings.EqualFold(base, "Containerfile") ||
		strings.HasPrefix(strings.ToLower(base), "dockerfile.") ||
		ext == "dockerfile" || ext == "containerfile" {
		return &checker{tool: "hadolint", cmd: `printf 'STYRMAN_TOOL hadolint\n'; hadolint -f json "$F"`}
	}
	switch ext {
	case "go":
		// gofmt -e reports parse errors on stderr and the formatted file on
		// stdout; only the errors matter here.
		return &checker{tool: "gofmt", cmd: `printf 'STYRMAN_TOOL gofmt\n'; gofmt -e "$F" >/dev/null`}
	case "sh", "bash":
		// shellcheck is the real linter; bash -n is the floor on an image
		// predating it (the guest reports which one ran).
		//
		// -s bash ONLY when the file has no shebang. shellcheck otherwise
		// reports SC2148 ("add a shebang") on every fragment, which made a
		// perfectly clean file light up the gutter. It is not passed
		// unconditionally because -s OVERRIDES a real shebang, which would
		// silently drop dialect findings — e.g. SC3014 (`==` is undefined in
		// POSIX sh) disappears from a `#!/bin/sh` script (measured).
		sc := `shellcheck -f gcc "$F"`
		if !shebang {
			sc = `shellcheck -f gcc -s bash "$F"`
		}
		return &checker{tool: "shellcheck", cmd: `if command -v shellcheck >/dev/null 2>&1; then printf 'STYRMAN_TOOL shellcheck\n'; ` + sc + `; else printf 'STYRMAN_TOOL bash\n'; bash -n "$F"; fi`}
	case "py":
		// PYTHONPYCACHEPREFIX redirects the .pyc out of the workspace.
		// PYTHONDONTWRITEBYTECODE does NOT work here — it governs the import
		// system, while py_compile's whole job is emitting bytecode, so it
		// still wrote __pycache__/ next to the temp file (measured).
		return &checker{tool: "python3", cmd: `printf 'STYRMAN_TOOL python\n'; PYTHONPYCACHEPREFIX=/tmp/styrman-pyc python3 -m py_compile "$F"`}
	case "js", "jsx", "mjs", "cjs":
		return &checker{tool: "node", cmd: `printf 'STYRMAN_TOOL node\n'; node --check "$F"`}
	case "json":
		return &checker{tool: "jq", cmd: `printf 'STYRMAN_TOOL jq\n'; jq empty "$F"`}
	case "yaml", "yml":
		return &checker{tool: "yq", cmd: `printf 'STYRMAN_TOOL yq\n'; yq e . "$F" >/dev/null`}
	case "tf", "tfvars", "hcl":
		return &checker{tool: "tofu", cmd: `printf 'STYRMAN_TOOL gcc\n'; tofu fmt -check "$F" >/dev/null`}
	}
	return nil
}

var (
	// `file:12:7: warning: message` — gofmt, shellcheck -f gcc, tofu.
	reGCC = regexp.MustCompile(`^[^:]*:(\d+):(\d+):\s*(?:(error|warning|note|style|info):\s*)?(.+)$`)
	// `a.sh: line 4: syntax error: unexpected end of file`
	reBashN = regexp.MustCompile(`^[^:]*:\s*line (\d+):\s*(.+)$`)
	// python: `  File "a.py", line 1` … later `SyntaxError: invalid syntax`
	rePyLine = regexp.MustCompile(`File "[^"]*", line (\d+)`)
	rePyErr  = regexp.MustCompile(`^\s*(\w*(?:Error|Warning)):\s*(.+)$`)
	// node --check: `/tmp/a.js:2` … later `SyntaxError: Unexpected end of input`
	reNodeLine = regexp.MustCompile(`^/?[^:\s]*:(\d+)$`)
	// jq: `jq: parse error: Expected another key-value pair at line 1, column 9`
	reJQ = regexp.MustCompile(`at line (\d+), column (\d+)`)
	// yq: `... go-yaml load error in scanner at L2.C4: mapping values ...`
	reYQ = regexp.MustCompile(`at L(\d+)\.C(\d+):\s*(.+)$`)
)

// hadolintJSON is hadolint's `-f json` record.
type hadolintJSON struct {
	Line    int    `json:"line"`
	Column  int    `json:"column"`
	Level   string `json:"level"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// normSeverity maps each tool's vocabulary onto CodeMirror's three levels.
// Anything unrecognised is an error: over-reporting a real problem is safer
// than silently downgrading it.
func normSeverity(s string) string {
	switch strings.ToLower(s) {
	case "warning", "warn", "style":
		return "warning"
	case "info", "note":
		return "info"
	default:
		return "error"
	}
}

// parseCheckOutput turns one tool's raw output into diagnostics. `tool` is the
// name the guest reported. Output that parses to nothing but is non-empty
// becomes a single line-1 diagnostic carrying the raw text, so a format change
// upstream surfaces as a visible (if crude) message instead of a false pass.
func parseCheckOutput(tool, out string) []checkDiagnostic {
	out = strings.TrimRight(out, "\n")
	if strings.TrimSpace(out) == "" {
		return nil
	}
	var ds []checkDiagnostic
	lines := strings.Split(out, "\n")

	switch tool {
	case "hadolint":
		var recs []hadolintJSON
		if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &recs); err == nil {
			for _, r := range recs {
				msg := r.Message
				if r.Code != "" {
					msg = fmt.Sprintf("%s: %s", r.Code, msg)
				}
				ds = append(ds, checkDiagnostic{
					Line: max(r.Line, 1), Col: r.Column,
					Severity: normSeverity(r.Level), Message: msg,
				})
			}
			return ds
		}
		// Not JSON: hadolint failed before producing any (e.g. not installed).

	case "gofmt", "shellcheck", "gcc":
		for _, l := range lines {
			if m := reGCC.FindStringSubmatch(l); m != nil {
				ds = append(ds, checkDiagnostic{
					Line: atoi(m[1], 1), Col: atoi(m[2], 0),
					Severity: normSeverity(m[3]), Message: strings.TrimSpace(m[4]),
				})
			}
		}
		if len(ds) > 0 {
			return ds
		}

	case "bash":
		for _, l := range lines {
			if m := reBashN.FindStringSubmatch(l); m != nil {
				ds = append(ds, checkDiagnostic{
					Line: atoi(m[1], 1), Severity: "error",
					Message: strings.TrimSpace(m[2]),
				})
			}
		}
		if len(ds) > 0 {
			return ds
		}

	case "python":
		line, msg := 1, ""
		for _, l := range lines {
			if m := rePyLine.FindStringSubmatch(l); m != nil {
				line = atoi(m[1], 1)
			}
			if m := rePyErr.FindStringSubmatch(l); m != nil {
				msg = m[1] + ": " + strings.TrimSpace(m[2])
			}
		}
		if msg != "" {
			return []checkDiagnostic{{Line: line, Severity: "error", Message: msg}}
		}

	case "node":
		line, msg := 1, ""
		for _, l := range lines {
			if m := reNodeLine.FindStringSubmatch(strings.TrimSpace(l)); m != nil {
				line = atoi(m[1], 1)
			}
			if m := rePyErr.FindStringSubmatch(l); m != nil { // same Name: msg shape
				msg = m[1] + ": " + strings.TrimSpace(m[2])
			}
		}
		if msg != "" {
			return []checkDiagnostic{{Line: line, Severity: "error", Message: msg}}
		}

	case "jq":
		for _, l := range lines {
			if m := reJQ.FindStringSubmatch(l); m != nil {
				return []checkDiagnostic{{
					Line: atoi(m[1], 1), Col: atoi(m[2], 0),
					Severity: "error",
					Message:  strings.TrimSpace(strings.TrimPrefix(l, "jq: ")),
				}}
			}
		}

	case "yq":
		for _, l := range lines {
			if m := reYQ.FindStringSubmatch(l); m != nil {
				return []checkDiagnostic{{
					Line: atoi(m[1], 1), Col: atoi(m[2], 0),
					Severity: "error", Message: strings.TrimSpace(m[3]),
				}}
			}
		}
	}

	// Fallback: something was reported that we could not place on a line.
	return []checkDiagnostic{{Line: 1, Severity: "error", Message: firstLines(out, 3)}}
}

func atoi(s string, def int) int {
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return def
}

func firstLines(s string, n int) string {
	parts := strings.Split(strings.TrimSpace(s), "\n")
	if len(parts) > n {
		parts = parts[:n]
	}
	return strings.TrimSpace(strings.Join(parts, "; "))
}

// missingTool spots "the checker isn't installed" so the UI can say so instead
// of showing the shell's error as if it were a syntax problem in the file.
//
// Anchored to the shell's OWN not-found message and only when it is the entire
// output. The previous version substring-matched "not found" anywhere in the
// tool's output and then discarded every diagnostic — so a checker reporting
// e.g. "import not found" rendered as CLEAN. A syntax checker that silently
// reports nothing is the one failure mode that matters here.
func missingTool(out string) bool {
	t := strings.TrimSpace(out)
	if t == "" || strings.Contains(t, "\n") {
		return false // real diagnostics, not a bare shell error
	}
	l := strings.ToLower(t)
	return strings.Contains(l, "command not found") ||
		strings.HasSuffix(l, "no such file or directory") &&
			strings.Contains(l, ":")
}

// checkFile syntax-checks the posted content in the project's guest VM.
//
// Unlike formatFile this never 4xx's on an unsupported file type: the editor
// calls it automatically for whatever is open, so "no checker for .txt" is a
// normal answer (`supported:false`), not a client error.
func (s *Server) checkFile(w http.ResponseWriter, r *http.Request) {
	_, cid, guest, ws, _, err := s.gitContext(r)
	if err != nil {
		if errors.Is(err, errVMNotRunning) {
			writeErr(w, http.StatusConflict, errors.New("start the project to check syntax"))
			return
		}
		writeErr(w, http.StatusNotFound, err)
		return
	}
	var body struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Path == "" {
		writeErr(w, http.StatusBadRequest, errors.New("path and content required"))
		return
	}
	if len(body.Content) > maxCheckBytes {
		writeErr(w, http.StatusRequestEntityTooLarge,
			fmt.Errorf("file too large to check (%d bytes, max %d)", len(body.Content), maxCheckBytes))
		return
	}

	base := filepath.Base(body.Path)
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(body.Path), "."))
	c := checkerFor(ext, base, hasShebang(body.Content))
	if c == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"supported": false, "diagnostics": []checkDiagnostic{},
		})
		return
	}

	// Same temp-file-over-stdin trick as formatting: no argv size limit and no
	// shell interpolation of the user's content. The suffix matters — several
	// checkers dispatch on the file extension.
	suffix := safeSuffix(ext)
	script := checkScript(suffix, len(body.Content), c.cmd)

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	_, out, err := s.orch.Exec(ctx, cid, orchestrator.ExecSpec{
		User:    guest,
		Env:     []string{"HOME=/home/" + guest, "USER=" + guest},
		WorkDir: ws,
		Cmd:     []string{"bash", "-c", script},
		Stdin:   []byte(body.Content),
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if strings.HasPrefix(out, "STYRMAN_ERR\n") {
		writeErr(w, http.StatusInternalServerError,
			errors.New(strings.TrimSpace(strings.TrimPrefix(out, "STYRMAN_ERR\n"))))
		return
	}

	// The guest echoes the tool it actually ran before the output block.
	tool, payload := c.tool, out
	if _, rest, ok := strings.Cut(out, "STYRMAN_OUT\n"); ok {
		payload = rest
	}
	if t, rest, ok := strings.Cut(payload, "\n"); ok && strings.HasPrefix(t, "STYRMAN_TOOL ") {
		tool = strings.TrimSpace(strings.TrimPrefix(t, "STYRMAN_TOOL "))
		payload = rest
	}

	if missingTool(payload) {
		writeJSON(w, http.StatusOK, map[string]any{
			"supported": true, "tool": c.tool, "unavailable": true,
			"diagnostics": []checkDiagnostic{},
		})
		return
	}

	ds := parseCheckOutput(tool, payload)
	if ds == nil {
		ds = []checkDiagnostic{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"supported": true, "tool": tool, "diagnostics": ds,
	})
}

// checkScript builds the guest script. Extracted so the stdin contract is
// unit-testable (check_test.go): it MUST read an exact byte count rather than
// waiting for stdin EOF — see the long comment in orchestrator.Exec.
func checkScript(suffix string, n int, cmd string) string {
	return fmt.Sprintf(`F=$(mktemp -p . --suffix=.%s) || { printf 'STYRMAN_ERR\ncannot create temp file'; exit 0; }
trap 'rm -f "$F" "${F}.err" "${F}.out"' EXIT
head -c %d > "$F"
[ "$(wc -c < "$F")" -eq %d ] || { printf 'STYRMAN_ERR\nshort read: editor content did not arrive intact'; exit 0; }
{ %s ; } > "${F}.out" 2>&1
printf 'STYRMAN_OUT\n'
cat "${F}.out"
rm -f "$F" "${F}.out"`, suffix, n, n, cmd)
}
