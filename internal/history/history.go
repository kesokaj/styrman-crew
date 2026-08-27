// Package history exposes what happened in a project: transcripts of every
// terminal session (captured by tmux pipe-pane inside the microVM, written
// into the shared home so they outlive containers) and the project's current
// git diff.
package history

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// logsRel is the transcript directory relative to the user home, matching
// the path the in-guest tmux hook writes to (see image/entrypoint.sh). It is
// addressed relative to the home so os.Root confinement refuses a
// guest-planted symlink under ~/.styrman/logs from escaping the home.
func logsRel(project string) string {
	return path.Join(".styrman", "logs", project)
}

// LogsDir returns the absolute transcript directory (for callers that only
// need the path, e.g. cleanup); reads/lists go through the confined helpers.
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
// homeDir is the confinement root; the logs dir is addressed relative to it.
func List(homeDir, project string) ([]Transcript, error) {
	rt, err := os.OpenRoot(homeDir)
	if err != nil {
		return []Transcript{}, nil
	}
	defer rt.Close()
	d, err := rt.Open(logsRel(project))
	if err != nil {
		return []Transcript{}, nil // missing dir (or a symlink escaping home) = empty
	}
	defer d.Close()
	des, err := d.ReadDir(-1)
	if err != nil {
		return []Transcript{}, nil
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
	rt, err := os.OpenRoot(homeDir)
	if err != nil {
		return "", err
	}
	defer rt.Close()
	// Confined open: a guest symlink under ~/.styrman/logs pointing outside
	// the home is refused, not followed.
	f, err := rt.Open(path.Join(logsRel(project), id+".log"))
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
