package harness

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// State is the install manifest. It is the only thing uninstall trusts: a
// path not listed here is not sh-agent's to delete, whatever directory it
// sits in.
type State struct {
	Repo        string   `json:"repo"`
	Commit      string   `json:"commit"`
	InstalledAt string   `json:"installedAt"`
	Vendor      string   `json:"vendor,omitempty"`
	Files       []string `json:"files"` // relative to $HOME, sorted
}

func statePath(home string) string {
	return filepath.Join(home, ".config", "sh-agent", "state.json")
}

// LoadState reads the manifest. A missing file is not an error: it means
// nothing has been installed into this home yet.
func LoadState(home string) (State, bool, error) {
	data, err := os.ReadFile(statePath(home))
	if errors.Is(err, os.ErrNotExist) {
		return State{}, false, nil
	}
	if err != nil {
		return State{}, false, err
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return State{}, false, fmt.Errorf("parsing %s: %w", statePath(home), err)
	}
	// The manifest is a file under the user's home: its entries are paths
	// every reader (install's resync, uninstall, status) joins under the
	// install root and may Lstat or remove, so a non-local entry (`../x`, an
	// absolute path) is dropped HERE, once, never followed by any caller
	// (security review 2026-09-14).
	kept := s.Files[:0]
	for _, rel := range s.Files {
		if filepath.IsLocal(rel) {
			kept = append(kept, rel)
		}
	}
	s.Files = kept
	if s.Vendor == "" {
		s.Vendor = VendorAll
	}
	return s, true, nil
}

func SaveState(home string, s State) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(statePath(home), append(data, '\n'), 0o644)
}

func RemoveState(home string) error {
	p := statePath(home)
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	// The directory is sh-agent's own; drop it when empty.
	_ = os.Remove(filepath.Dir(p))
	return nil
}

// LastRetro returns when the learning loop last ran: the newest of the
// harness proposal files under ~/.config/sh-agent/proposals/ and the
// cross-project index ~/.config/sh-agent/retros.jsonl, which a project-level
// /retro or /distill appends to even when it proposes nothing for the harness.
func LastRetro(home string) (time.Time, bool) {
	paths, _ := filepath.Glob(filepath.Join(home, ".config", "sh-agent", "proposals", "*.md"))
	paths = append(paths, filepath.Join(home, ".config", "sh-agent", "retros.jsonl"))
	var newest time.Time
	for _, p := range paths {
		if info, err := os.Stat(p); err == nil && info.ModTime().After(newest) {
			newest = info.ModTime()
		}
	}
	return newest, !newest.IsZero()
}
