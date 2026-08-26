// Package history exposes what happened in a project: transcripts of every
// terminal session (captured by tmux pipe-pane inside the microVM, written
// into the shared home so they outlive containers) and the project's current
// git diff.
package history

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// LogsDir returns the transcript directory for a project, matching the path
// the in-guest tmux hook writes to (see image/entrypoint.sh).
func LogsDir(homeDir, project string) string {
	return filepath.Join(homeDir, ".styrman", "logs", project)
}

// Transcript is one terminal session's log file.
type Transcript struct {
	ID       string `json:"id"` // tmux session name, e.g. "conv-1"
	Size     int64  `json:"size"`
	Modified string `json:"modified"`
}

// List returns the project's transcripts, most recently written first.
func List(homeDir, project string) ([]Transcript, error) {
	dir := LogsDir(homeDir, project)
	des, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []Transcript{}, nil
		}
		return nil, err
	}
	var out []Transcript
	for _, de := range des {
		if de.IsDir() || !strings.HasSuffix(de.Name(), ".log") {
			continue
		}
		info, err := de.Info()
		if err != nil {
			continue
		}
		out = append(out, Transcript{
			ID:       strings.TrimSuffix(de.Name(), ".log"),
			Size:     info.Size(),
			Modified: info.ModTime().UTC().Format(time.RFC3339),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Modified > out[j].Modified })
	if out == nil {
		out = []Transcript{}
	}
	return out, nil
}

// ansiRe strips CSI/OSC escape sequences so a raw PTY capture reads as text.
var ansiRe = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(\x07|\x1b\\)|\x1b[()][A-B0-9]|\x1b[=>]|[\x00-\x08\x0b\x0c\x0e-\x1f]`)

// idRe keeps transcript ids to safe session-name shapes.
var idRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}$`)

// Read returns the tail of a transcript with escape sequences stripped.
// maxBytes 0 means 256 KiB.
func Read(homeDir, project, id string, maxBytes int64) (string, error) {
	if !idRe.MatchString(id) {
		return "", fmt.Errorf("invalid transcript id")
	}
	if maxBytes <= 0 {
		maxBytes = 256 << 10
	}
	path := filepath.Join(LogsDir(homeDir, project), id+".log")
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return "", err
	}
	if st.Size() > maxBytes {
		if _, err := f.Seek(st.Size()-maxBytes, 0); err != nil {
			return "", err
		}
	}
	buf := make([]byte, maxBytes)
	n, _ := f.Read(buf)
	text := ansiRe.ReplaceAllString(string(buf[:n]), "")
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return text, nil
}

// Diff describes the project's working-tree state.
type Diff struct {
	Repo   bool   `json:"repo"`
	Branch string `json:"branch,omitempty"`
	Status string `json:"status,omitempty"`
	Diff   string `json:"diff,omitempty"`
}

// GitDiff runs git in the project's workspace on the host (the shared home
// is mounted, so this works whether or not the microVM is running).
func GitDiff(workspace string) (*Diff, error) {
	if _, err := os.Stat(filepath.Join(workspace, ".git")); err != nil {
		return &Diff{Repo: false}, nil
	}
	run := func(args ...string) string {
		cmd := exec.Command("git", append([]string{"-C", workspace}, args...)...)
		// Containers see a foreign uid on some mounts; git refuses those
		// without this. The path is ours by construction.
		cmd.Env = append(os.Environ(), "GIT_CONFIG_COUNT=1",
			"GIT_CONFIG_KEY_0=safe.directory", "GIT_CONFIG_VALUE_0=*")
		out, _ := cmd.Output()
		return string(out)
	}
	d := &Diff{
		Repo:   true,
		Branch: strings.TrimSpace(run("rev-parse", "--abbrev-ref", "HEAD")),
		Status: run("status", "--short", "--branch"),
		Diff:   run("diff", "--stat", "HEAD"),
	}
	if full := run("diff", "HEAD"); full != "" {
		if len(full) > 256<<10 {
			full = full[:256<<10] + "\n… (truncated)"
		}
		d.Diff += "\n" + full
	}
	return d, nil
}
