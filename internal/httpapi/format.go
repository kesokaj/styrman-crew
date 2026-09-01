package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/kesokaj/styrman-crew/internal/orchestrator"
)

// maxFormatBytes bounds the editor content we'll format — source files are tiny;
// anything larger is rejected with 413 rather than silently mishandled.
const maxFormatBytes = 5 << 20

// Real formatting runs the language's actual CLI formatter INSIDE the project
// VM (where the toolchain lives) rather than shipping formatters to the browser.
// The editor content is piped in over the command's stdin (no argv size limit,
// no shell interpolation of user text) to a temp file created in the workspace
// root so formatters discover the project's own config; a marker line
// distinguishes success from the formatter's own error output (docker exec
// merges stdout+stderr).

// formatInPlace returns the in-place format command for a file extension, with
// $F standing for the temp file, or "" when no formatter is wired. The tool may
// still be absent in the project (→ "command not found", surfaced to the user).
func formatInPlace(ext string) string {
	switch ext {
	case "go":
		return `gofmt -w "$F"`
	case "tf", "tfvars", "hcl":
		return `terraform fmt "$F" 2>/dev/null || tofu fmt "$F"`
	case "rs":
		return `rustfmt "$F"`
	case "py":
		return `black -q "$F" 2>/dev/null || ruff format -q "$F"`
	case "sh", "bash":
		return `shfmt -w "$F"`
	case "js", "jsx", "ts", "tsx", "mjs", "cjs",
		"json", "css", "scss", "less",
		"html", "htm", "md", "markdown",
		"yaml", "yml", "vue", "graphql":
		return `prettier --write "$F"`
	}
	return ""
}

// formatFile formats the posted content with the appropriate CLI formatter in
// the project's guest VM and returns the formatted text.
func (s *Server) formatFile(w http.ResponseWriter, r *http.Request) {
	_, cid, guest, ws, _, err := s.gitContext(r)
	if err != nil {
		if errors.Is(err, errVMNotRunning) {
			writeErr(w, http.StatusConflict, errors.New("start the project to format"))
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
	if len(body.Content) > maxFormatBytes {
		writeErr(w, http.StatusRequestEntityTooLarge,
			fmt.Errorf("file too large to format (%d bytes, max %d)", len(body.Content), maxFormatBytes))
		return
	}
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(body.Path), "."))
	cmd := formatInPlace(ext)
	if cmd == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("no formatter for .%s files", ext))
		return
	}

	// Temp file in the workspace (WorkDir=ws) so formatters discover repo config;
	// content is read from stdin, so no size limit and no shell interpolation.
	// ext is already allowlisted by formatInPlace's switch, but sanitize on the
	// same path as check.go so the two cannot drift apart (defense in depth).
	script := formatScript(safeSuffix(ext), len(body.Content), cmd)

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
	if formatted, ok := strings.CutPrefix(out, "STYRMAN_OK\n"); ok {
		writeJSON(w, http.StatusOK, map[string]string{"content": formatted})
		return
	}
	msg := strings.TrimSpace(strings.TrimPrefix(out, "STYRMAN_ERR\n"))
	if msg == "" {
		msg = "formatter failed"
	}
	// 422: the request was fine but the formatter rejected it (syntax error) or
	// isn't installed — surface its own message so the user knows which.
	writeErr(w, http.StatusUnprocessableEntity, errors.New(firstLine(msg)))
}

// formatScript builds the guest script. Extracted so the stdin contract is
// unit-testable (format_test.go): it MUST read an exact byte count rather than
// waiting for stdin EOF — see the long comment in orchestrator.Exec.
func formatScript(ext string, n int, cmd string) string {
	return fmt.Sprintf(`F=$(mktemp -p . --suffix=.%s) || { printf 'STYRMAN_ERR\ncannot create temp file'; exit 0; }
trap 'rm -f "$F" "${F}.err" "${F}.out"' EXIT
E="${F}.err"
head -c %d > "$F"
[ "$(wc -c < "$F")" -eq %d ] || { printf 'STYRMAN_ERR\nshort read: editor content did not arrive intact'; exit 0; }
if %s >"$E" 2>&1; then printf 'STYRMAN_OK\n'; cat "$F"; else printf 'STYRMAN_ERR\n'; cat "$E"; fi
rm -f "$F" "$E"`, ext, n, n, cmd)
}
