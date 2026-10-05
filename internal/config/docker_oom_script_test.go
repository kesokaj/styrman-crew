package config

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestDockerOOMScript runs deploy/lib/docker-oom-test.sh, the behavioural test
// of the dockerd OOM drop-in host-setup.sh writes (ADR-085), so `go test ./...`
// exercises the real shell function against a scratch dir.
func TestDockerOOMScript(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not found")
	}
	out, err := exec.Command("bash", filepath.Join(repoRoot(t), "deploy", "lib", "docker-oom-test.sh")).CombinedOutput()
	if err != nil {
		t.Fatalf("docker-oom-test.sh failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), " 0 failed") {
		t.Fatalf("docker-oom-test.sh exited 0 without its summary line:\n%s", out)
	}
}
