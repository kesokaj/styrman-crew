package config

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// ADR-078: the harness's file install (`sh-agent install`, about 13 s on a
// disk-backed home since the v2 harness) runs AFTER /run/styrman/ready, in
// the background, and /run/styrman/harness-done is a per-boot marker.
//
// This pins the entrypoint's TEXT, nothing more: it is the cheap guard the
// release gate's Tier 1 can run. The evidence that the order holds in a
// booted microVM is Tier 2's "ready line precedes the agent harness
// deployed line" and its boot-budget check on a disk-backed state dir.
func TestEntrypointInstallsHarnessAfterReady(t *testing.T) {
	raw, err := os.ReadFile("../../image/entrypoint.sh")
	if err != nil {
		t.Fatal(err)
	}
	src := string(raw)

	// The per-boot markers are cleared together, before anything else can
	// write either of them.
	clear := "rm -f /run/styrman/ready /run/styrman/harness-done"
	clearAt := strings.Index(src, clear)
	if clearAt < 0 {
		t.Fatalf("entrypoint does not clear both markers with %q", clear)
	}
	if first := strings.Index(src, "touch /run/styrman/"); first >= 0 && first < clearAt {
		t.Errorf("a marker is written (offset %d) before the markers are cleared (offset %d)", first, clearAt)
	}

	// The install call exists exactly once, inside harness_finish.
	fn := regexp.MustCompile(`(?ms)^harness_finish\(\) \{\n.*?^\}\n`).FindStringIndex(src)
	if fn == nil {
		t.Fatal("harness_finish() not found as a top-level function")
	}
	const install = "/styrman/bin/sh-agent install --home"
	if n := strings.Count(src, install); n != 1 {
		t.Fatalf("%q appears %d times, want exactly 1", install, n)
	}
	if at := strings.Index(src, install); at < fn[0] || at > fn[1] {
		t.Errorf("%q (offset %d) is outside harness_finish (%d..%d): the install would run in front of ready", install, at, fn[0], fn[1])
	}
	body := src[fn[0]:fn[1]]
	if !strings.Contains(body, "touch /run/styrman/harness-done") {
		t.Error("harness_finish does not write /run/styrman/harness-done")
	}

	// harness_finish is started once, in the background, below the project
	// boot's ready marker (the LAST `touch /run/styrman/ready`; the first
	// belongs to minimal mode, which has no harness).
	call := regexp.MustCompile(`(?m)^harness_finish &$`).FindAllStringIndex(src, -1)
	if len(call) != 1 {
		t.Fatalf("`harness_finish &` appears %d times as a statement, want exactly 1", len(call))
	}
	readyAt := strings.LastIndex(src, "touch /run/styrman/ready\n")
	if readyAt < 0 {
		t.Fatal("no `touch /run/styrman/ready` in the entrypoint")
	}
	if call[0][0] < readyAt {
		t.Errorf("harness_finish is started (offset %d) before the ready marker is written (offset %d)", call[0][0], readyAt)
	}
}
