package config

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestKataConfigScript runs deploy/lib/kata-config-test.sh, the behavioural
// test of the two Kata host settings host-setup.sh applies (BUG-089, ADR-083),
// so `go test ./...` exercises the real shell functions. The script works in
// its own mktemp scratch dir and touches no host path; it needs only bash and
// coreutils, so it never skips on Linux.
func TestKataConfigScript(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not found")
	}
	script := filepath.Join(repoRoot(t), "deploy", "lib", "kata-config-test.sh")
	out, err := exec.Command("bash", script).CombinedOutput()
	if err != nil {
		t.Fatalf("kata-config-test.sh failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), " 0 failed") {
		t.Fatalf("kata-config-test.sh exited 0 without its summary line:\n%s", out)
	}
}
