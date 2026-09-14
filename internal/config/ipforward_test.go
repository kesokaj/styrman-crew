package config

import (
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
