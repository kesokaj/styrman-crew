// Package updatestatus reads the one file in which the host's auto-updater
// records how its last tick ended (ADR-081), so the server can show admins a
// failing updater in Settings. BUG-084: the updater failed for ten days on a
// rejected token and nothing said so.
//
// The file is written by root (deploy/update/styrman-update, write_status)
// in a root-only directory that the compose file mounts read-only into the
// server. Everything in it is a closed set — codes and release tags, never
// text from git or from a remote — and this package re-validates every field
// rather than trusting the writer: what reaches the page is only what passes
// here.
package updatestatus

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"regexp"
	"time"
)

// StaleAfter is how old a status may be before it stops meaning anything.
// The timer ticks every 5 minutes and one tick may run for 45
// (TimeoutStartSec in styrman-update.service), so an hour without a write
// means the updater is not running at all — a disabled timer, or a refusal
// above the point where it starts recording.
const StaleAfter = 60 * time.Minute

// maxFile bounds the read: the real file is about 150 bytes.
const maxFile = 4 << 10

// Status is what the settings API hands an admin.
type Status struct {
	// State is "ok", "failing", "stale" (a valid file that stopped being
	// refreshed) or "unknown" (a file that is there and cannot be trusted:
	// unreadable, malformed, a field outside its set). Never "ok" by
	// default: an unreadable status reads "cannot confirm" (BUG-014).
	State     string `json:"state"`
	Code      string `json:"code,omitempty"`
	CheckedAt string `json:"checkedAt,omitempty"`
	Since     string `json:"since,omitempty"`
	Deployed  string `json:"deployed,omitempty"`
	Latest    string `json:"latest,omitempty"`
}

// file is the updater's own shape: exactly these six keys.
type file struct {
	Result    string `json:"result"`
	Code      string `json:"code"`
	CheckedAt string `json:"checkedAt"`
	Since     string `json:"since"`
	Deployed  string `json:"deployed"`
	Latest    string `json:"latest"`
}

var (
	tagRE = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+$`)

	// failingCodes mirrors write_status in deploy/update/styrman-update.
	failingCodes = map[string]bool{
		"fetch-auth": true, "fetch": true, "no-tags": true, "signature": true,
		"rolled-back": true, "deploy-failed": true, "latched": true, "error": true,
	}
)

var unknown = Status{State: "unknown"}

// Read returns ok=false when there is nothing to say: path is empty (the
// feature is off — a dev run, a suite that did not ask for it) or the file
// does not exist (auto-update was never set up on this host, or its updater
// predates the status file). Anything else that goes wrong is State
// "unknown", with no other field.
func Read(path string, now time.Time) (s Status, ok bool) {
	if path == "" {
		return Status{}, false
	}
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Status{}, false
		}
		return unknown, true
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, maxFile+1))
	if err != nil || len(raw) > maxFile {
		return unknown, true
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var in file
	if err := dec.Decode(&in); err != nil {
		return unknown, true
	}
	// Exactly one JSON value: trailing data is not the updater's file.
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return unknown, true
	}
	checked, err := time.Parse(time.RFC3339, in.CheckedAt)
	if err != nil {
		return unknown, true
	}
	switch in.Result {
	case "ok":
		if in.Code != "ok" || in.Since != "" {
			return unknown, true
		}
	case "failing":
		if !failingCodes[in.Code] {
			return unknown, true
		}
		if _, err := time.Parse(time.RFC3339, in.Since); err != nil {
			return unknown, true
		}
	default:
		return unknown, true
	}
	if (in.Deployed != "" && !tagRE.MatchString(in.Deployed)) || (in.Latest != "" && !tagRE.MatchString(in.Latest)) {
		return unknown, true
	}
	out := Status{State: in.Result, Code: in.Code, CheckedAt: in.CheckedAt, Since: in.Since, Deployed: in.Deployed, Latest: in.Latest}
	if now.Sub(checked) > StaleAfter {
		out.State = "stale"
	}
	return out, true
}
