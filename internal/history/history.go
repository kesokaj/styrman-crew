// Package history exposes what happened in a project: transcripts of every
// terminal session (captured by tmux pipe-pane inside the microVM, written
// into the project's own home so they outlive containers, ADR-031).
package history

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
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

// Transcript is one terminal session's log file.
type Transcript struct {
	ID       string `json:"id"` // tmux session name, e.g. "conv-1"
	Size     int64  `json:"size"`
	Modified string `json:"modified"`
}

// List returns the project's transcripts, most recently written first.
// homeDir is the confinement root; the logs dir is addressed relative to it.
func List(homeDir, project string) ([]Transcript, error) {
	// "No transcripts yet" (missing home/dir) is empty+nil; any OTHER failure
	// (permissions, I/O) is a real error — hiding it made a broken transcript
	// dir indistinguishable from an idle project (audit finding).
	rt, err := os.OpenRoot(homeDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []Transcript{}, nil
		}
		return nil, err
	}
	defer rt.Close()
	d, err := rt.Open(logsRel(project))
	if err != nil {
		// Missing dir — or a guest symlink escaping home, which os.Root
		// refuses with a non-NotExist error we also treat as "none".
		return []Transcript{}, nil
	}
	defer d.Close()
	// Bounded read: the transcript directory is guest-controlled (the microVM
	// writes its own session logs there), so an uncapped ReadDir(-1) over
	// millions of crafted entries could pin the server. Cap the listing.
	const maxTranscripts = 5000
	var des []os.DirEntry
	for len(des) < maxTranscripts {
		batch, rerr := d.ReadDir(512)
		des = append(des, batch...)
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return nil, rerr
		}
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

// Match is one search hit inside a transcript.
type Match struct {
	Transcript string `json:"transcript"` // e.g. "conv-1"
	Line       string `json:"line"`       // the matching line, ANSI-stripped, trimmed
}

// searchCaps bound the work: transcripts are unbounded raw PTY captures and
// the logs dir is guest-writable, so both file size AND file count are
// attacker-influenced from inside the microVM.
const (
	searchTailBytes    = 4 << 20 // scan at most the last 4 MiB per transcript
	searchMaxFiles     = 50      // newest-first; older transcripts are skipped
	searchMaxPerFile   = 20
	searchMaxTotal     = 200
	searchMaxLineRunes = 240
)

// Search scans the project's transcripts (newest first, capped) for the
// query, case-insensitive, and returns matching lines. Everything the agents
// and the operator typed or saw is in these logs, so this is full history
// search. ctx is checked between files so a disconnected client stops the
// scan.
func Search(ctx context.Context, homeDir, project, query string) ([]Match, error) {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return []Match{}, nil
	}
	transcripts, err := List(homeDir, project)
	if err != nil {
		return nil, err
	}
	if len(transcripts) > searchMaxFiles {
		transcripts = transcripts[:searchMaxFiles]
	}
	rt, err := os.OpenRoot(homeDir)
	if err != nil {
		return []Match{}, nil
	}
	defer rt.Close()
	out := []Match{}
	for _, t := range transcripts {
		if len(out) >= searchMaxTotal || ctx.Err() != nil {
			break
		}
		matches := searchOne(rt, path.Join(logsRel(project), t.ID+".log"), t.ID, q, searchMaxTotal-len(out))
		out = append(out, matches...)
	}
	return out, nil
}

// searchOne scans the tail of one transcript for q (already lowercased).
func searchOne(rt *os.Root, rel, id, q string, budget int) []Match {
	f, err := rt.Open(rel)
	if err != nil {
		return nil
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() {
		return nil
	}
	size := st.Size()
	if size > searchTailBytes {
		if _, err := f.Seek(size-searchTailBytes, 0); err != nil {
			return nil
		}
	}
	buf, err := io.ReadAll(io.LimitReader(f, searchTailBytes))
	if err != nil {
		return nil
	}
	text := ansiRe.ReplaceAllString(string(buf), "")
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	// One whole-file Contains check first: the common case is a transcript
	// with no hits, and per-line ToLower over every file was the dominant
	// cost (~144 ms per 4 MiB measured in review).
	if !strings.Contains(strings.ToLower(text), q) {
		return nil
	}
	max := min(budget, searchMaxPerFile)
	var out []Match
	for line := range strings.Lines(text) {
		if len(out) >= max {
			break
		}
		if !strings.Contains(strings.ToLower(line), q) {
			continue
		}
		clean := strings.TrimSpace(line)
		if r := []rune(clean); len(r) > searchMaxLineRunes {
			clean = string(r[:searchMaxLineRunes]) + "…"
		}
		if clean == "" {
			continue
		}
		out = append(out, Match{Transcript: id, Line: clean})
	}
	return out
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
	// io.ReadAll, not a single f.Read: one read(2) is not guaranteed to fill
	// the buffer (maxBytes reaches MiB), and a real I/O error must not be
	// presented as a silently short transcript (audit finding).
	buf, err := io.ReadAll(io.LimitReader(f, maxBytes))
	if err != nil {
		return "", err
	}
	text := ansiRe.ReplaceAllString(string(buf), "")
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return text, nil
}
