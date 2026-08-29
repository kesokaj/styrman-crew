package httpapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/kesokaj/styrman-crew/internal/orchestrator"
)

// Real formatting runs the language's actual CLI formatter INSIDE the project
// VM (where the toolchain lives) rather than shipping formatters to the browser.
// The editor content is round-tripped through a temp file: base64 in (no shell
// interpolation of user text), format in place, read back. A marker line
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
	u, _, p, err := s.getProjectFor(r)
	if err != nil {
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
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(body.Path), "."))
	cmd := formatInPlace(ext)
	if cmd == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("no formatter for .%s files", ext))
		return
	}
	st, serr := s.orch.Status(r.Context(), u.Username, p.ID)
	if serr != nil || st.State != "running" {
		writeErr(w, http.StatusConflict, errors.New("start the project to format"))
		return
	}
	guest := s.guestUserFor(u.Username)

	b64 := base64.StdEncoding.EncodeToString([]byte(body.Content))
	script := fmt.Sprintf(`F=$(mktemp --suffix=.%s) || { printf 'STYRMAN_ERR\ncannot create temp file'; exit 0; }
E="${F}.err"
printf %%s '%s' | base64 -d > "$F"
if %s >"$E" 2>&1; then printf 'STYRMAN_OK\n'; cat "$F"; else printf 'STYRMAN_ERR\n'; cat "$E"; fi
rm -f "$F" "$E"`, ext, b64, cmd)

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	_, out, err := s.orch.Exec(ctx, st.ContainerID, orchestrator.ExecSpec{
		User: guest,
		Env:  []string{"HOME=/home/" + guest, "USER=" + guest},
		Cmd:  []string{"bash", "-c", script},
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
