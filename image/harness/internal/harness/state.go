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

// LastRetro returns when the newest /retro proposal was written. The retro
// skill writes to ~/.config/sh-agent/proposals/, so this is the harness's
// only view of whether the learning loop is being run.
func LastRetro(home string) (time.Time, bool) {
	paths, _ := filepath.Glob(filepath.Join(home, ".config", "sh-agent", "proposals", "*.md"))
	var newest time.Time
	for _, p := range paths {
		if info, err := os.Stat(p); err == nil && info.ModTime().After(newest) {
			newest = info.ModTime()
		}
	}
	return newest, !newest.IsZero()
}
