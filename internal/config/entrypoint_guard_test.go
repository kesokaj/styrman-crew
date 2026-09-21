package config

import (
	"os/exec"
	"path/filepath"
	"testing"
)

// The entrypoint's never-downgrade guard, `harness_home_newer <home release>
// <image release>` in image/entrypoint.sh (ADR-074), extracted by its
// function boundaries and run under bash: true only when both are canonical
// vX.Y.Z and the home's is strictly newer. The "unknown" cases are the
// security review's (2026-09-21): an unreadable image binary must never win
// the comparison and silently keep an old home.
func TestEntrypointHarnessHomeNewer(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("no bash on this host")
	}
	entrypoint, err := filepath.Abs("../../image/entrypoint.sh")
	if err != nil {
		t.Fatal(err)
	}
	fn, err := exec.Command("sed", "-n", "/^harness_home_newer() {/,/^}/p", entrypoint).Output()
	if err != nil || len(fn) == 0 {
		t.Fatalf("harness_home_newer not found in %s: %v", entrypoint, err)
	}
	cases := []struct {
		home, image string
		newer       bool
	}{
		{"v9.9.9", "v1.6.0", true},
		{"v1.10.0", "v1.9.0", true}, // numeric, not lexical
		{"v1.6.0", "v1.6.0", false},
		{"v1.5.0", "v1.6.0", false},
		{"", "v1.6.0", false},
		{"v9.9.9", "unknown", false}, // an unreadable image binary never wins
		{"unknown", "v1.6.0", false},
		{"v01.6.0", "v1.6.0", false}, // non-canonical home release
		{"v9.9.9", "", false},
	}
	for _, c := range cases {
		cmd := exec.Command("bash", "-c", string(fn)+"\nharness_home_newer \"$1\" \"$2\"", "_", c.home, c.image)
		err := cmd.Run()
		got := err == nil
		if _, ok := err.(*exec.ExitError); err != nil && !ok {
			t.Fatalf("(%q,%q): run: %v", c.home, c.image, err)
		}
		if got != c.newer {
			t.Errorf("harness_home_newer(%q, %q) = %v, want %v", c.home, c.image, got, c.newer)
		}
	}
}
