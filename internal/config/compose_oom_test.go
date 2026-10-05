package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The server and the ingress must never be the OOM killer's pick (ADR-084).
// Project VMs run at oom_score_adj -999 (cloud-hypervisor inherits it from
// containerd's shim) and, since ADR-083, are sized to the whole host, so a
// process at the default 0 is killed long before any VM: measured on the NUC
// 2026-10-05, styrman-server and Caddy sat at 0. Only -1000 ranks below the
// VMs. This pins the setting in the compose file the auto-updater deploys.
func TestComposeProtectsServerAndIngressFromTheOOMKiller(t *testing.T) {
	src, err := os.ReadFile(filepath.Join(repoRoot(t), "deploy", "docker-compose.yml"))
	if err != nil {
		t.Fatal(err)
	}
	// Each top-level service is a two-space-indented key; its settings are
	// indented four. Collect the four-space lines per service.
	blocks := map[string][]string{}
	svc := ""
	for _, line := range strings.Split(string(src), "\n") {
		switch {
		case strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "   ") && strings.HasSuffix(strings.TrimSpace(line), ":"):
			svc = strings.TrimSuffix(strings.TrimSpace(line), ":")
		case strings.HasPrefix(line, "    ") && !strings.HasPrefix(line, "     ") && svc != "":
			blocks[svc] = append(blocks[svc], strings.TrimSpace(line))
		case line != "" && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "#"):
			svc = "" // a new top-level key ends the services
		}
	}
	for _, name := range []string{"caddy", "styrman"} {
		lines, ok := blocks[name]
		if !ok {
			t.Fatalf("service %q not found in docker-compose.yml", name)
		}
		found := false
		for _, l := range lines {
			if l == "oom_score_adj: -1000" {
				found = true
			}
		}
		if !found {
			t.Errorf("service %q: want `oom_score_adj: -1000`", name)
		}
	}
}
