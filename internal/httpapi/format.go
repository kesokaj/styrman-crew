package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path"
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
		// --ignore-path /dev/null: the temp file sits inside the project (for
		// config discovery), where a .prettierignore matching dotfiles would
		// make prettier skip it and the unchanged content come back as a
		// "success" (audit 2026-09-13). The other formatters format an
		// explicit path regardless of their excludes.
		return `prettier --write --ignore-path /dev/null "$F"`
	}
	return ""
}

// formatFile formats the posted content with the appropriate CLI formatter in
// the project's guest VM and returns the formatted text.
func (s *Server) formatFile(w http.ResponseWriter, r *http.Request) {
	release := s.fileSlot(w, r)
	if release == nil {
		return
	}
	defer release()
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
		if bodyTimedOut(err) {
			writeErr(w, http.StatusRequestTimeout, errors.New("request body took too long"))
			return
		}
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

	// The temp file goes BESIDE the edited file inside the workspace
	// (WorkDir=ws), because formatters resolve their config from the file's
	// own directory upward — a monorepo's web/.prettierrc as much as the
	// root's. Its directory travels as an env var, never interpolated into
	// the script, and the script confines it under $PWD. Content is read
	// from stdin, so no size limit and no shell interpolation. ext is already
	// allowlisted by formatInPlace's switch, but sanitize on the same path as
	// check.go so the two cannot drift apart (defense in depth).
	dir, err := formatDir(body.Path)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	script := formatScript(safeSuffix(ext), len(body.Content), cmd)

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	_, out, err := s.guest.Exec(ctx, cid, orchestrator.ExecSpec{
		User:    guest,
		Env:     []string{"HOME=/home/" + guest, "USER=" + guest, "STYRMAN_FMT_DIR=" + dir},
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

// formatDir is the workspace-relative directory of the file being formatted,
// or an error for a path that would leave the workspace. Cleaned server-side;
// the script re-confines it under $PWD anyway (belt and braces).
func formatDir(p string) (string, error) {
	p = strings.ReplaceAll(p, "\\", "/")
	if strings.HasPrefix(p, "/") {
		return "", errors.New("path must be workspace-relative")
	}
	// Clean RELATIVE to the workspace: a leading ".." survives cleaning and
	// is the escape; cleaning against "/" would swallow it.
	c := path.Clean(p)
	if c == ".." || strings.HasPrefix(c, "../") {
		return "", errors.New("path escapes the workspace")
	}
	return path.Dir(c), nil
}

// formatScript builds the guest script. Extracted so the stdin contract is
// unit-testable (format_test.go): it MUST read an exact byte count rather than
// waiting for stdin EOF — see the long comment in orchestrator.Exec.
//
// The temp file lives beside the edited file (STYRMAN_FMT_DIR under $PWD, the
// workspace) so the formatter finds the project's config — this deliberately
// reverses the 2026-09-01 move to ~/.styrman/tmp for FORMAT only (check keeps
// it: a debounce tick, and checkers take the path). What made the workspace
// temp a nuisance then is handled here: the name is a dotfile that
// .git/info/exclude hides from status and "Stage all", the file explorer
// drops it from listings, and a crash leftover is swept by the next format
// (audit 2026-09-13). The sweep is scoped by depth AND name — never `find
// "$D" -type f -delete` inside a workspace.
func formatScript(ext string, n int, cmd string) string {
	return fmt.Sprintf(`D="$PWD"
if [ -n "${STYRMAN_FMT_DIR:-}" ] && [ -d "$PWD/${STYRMAN_FMT_DIR}" ]; then
  case "$(realpath -m -- "$PWD/${STYRMAN_FMT_DIR}")" in "$PWD"|"$PWD"/*) D="$PWD/${STYRMAN_FMT_DIR}" ;; esac
fi
find "$D" -maxdepth 1 -name '.styrman-fmt-*' -mmin +60 -delete 2>/dev/null
if [ -d "$PWD/.git" ] && ! grep -qsxF -- '.styrman-fmt-*' "$PWD/.git/info/exclude"; then mkdir -p "$PWD/.git/info" && printf '%%s\n' '.styrman-fmt-*' >> "$PWD/.git/info/exclude"; fi 2>/dev/null
F=$(mktemp -p "$D" .styrman-fmt-XXXXXX --suffix=.%s) || { printf 'STYRMAN_ERR\ncannot create temp file'; exit 0; }
trap 'rm -f "$F" "${F}.err"' EXIT
E="${F}.err"
head -c %d > "$F"
[ "$(wc -c < "$F")" -eq %d ] || { printf 'STYRMAN_ERR\nshort read: editor content did not arrive intact'; exit 0; }
if %s >"$E" 2>&1; then printf 'STYRMAN_OK\n'; cat "$F"; else printf 'STYRMAN_ERR\n'; cat "$E"; fi
rm -f "$F" "$E"`, ext, n, n, cmd)
}
