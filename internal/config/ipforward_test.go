package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// The sysctl file holds "0\n" or "1\n"; anything but an exact "0" is
// forwarding ON — the fail-closed reading, since the guard's job is to
// refuse a server that could route between project bridges (R12).
func TestIPForwardOffReadsTheSysctl(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		content string
		off     bool
	}{
		{"0\n", true},
		{"0", true},
		{"1\n", false},
		{"2\n", false},
		{"", false},
	} {
		p := filepath.Join(dir, "ip_forward")
		if err := os.WriteFile(p, []byte(tc.content), 0o644); err != nil {
			t.Fatal(err)
		}
		off, err := IPForwardOff(p)
		if err != nil {
			t.Fatalf("content %q: %v", tc.content, err)
		}
		if off != tc.off {
			t.Fatalf("content %q: off=%v, want %v", tc.content, off, tc.off)
		}
	}
}

// A missing or unreadable sysctl is an error, not a verdict: the caller
// decides (main warns and goes on — an exotic kernel must not take the
// service down over a file that is not there).
func TestIPForwardOffReportsAMissingFile(t *testing.T) {
	if _, err := IPForwardOff(filepath.Join(t.TempDir(), "absent")); err == nil {
		t.Fatal("want an error for a missing sysctl file")
	}
}

// The rule behind the R12 boot assertion, both branches. A missing sysctl
// file used to warn and continue; since 2026-09-22 it refuses like every
// other unreadable state, because a guard that measured nothing must not
// report "fine" (BUG-014/028/029/031's shape).
func TestIPForwardMustRefuse(t *testing.T) {
	missing := fmt.Errorf("read /proc/…: %w", fs.ErrNotExist)
	other := errors.New("permission denied")

	cases := []struct {
		name     string
		off      bool
		err      error
		override bool
		want     bool
	}{
		{"forwarding off, no error: run", true, nil, false, false},
		{"forwarding ON: refuse", false, nil, false, true},
		{"forwarding ON with override: run", false, nil, true, false},
		{"missing sysctl file: refuse", false, missing, false, true},
		{"missing sysctl file with override: run", false, missing, true, false},
		{"unreadable for any other reason: refuse", false, other, false, true},
		{"unreadable with override: run", false, other, true, false},
		// An error wins over a reading that looks fine: the bool is
		// meaningless when the read failed.
		{"error alongside off=true: refuse", true, other, false, true},
	}
	for _, c := range cases {
		if got := IPForwardMustRefuse(c.off, c.err, c.override); got != c.want {
			t.Errorf("%s: IPForwardMustRefuse(%v, %v, %v) = %v, want %v",
				c.name, c.off, c.err, c.override, got, c.want)
		}
	}
}
