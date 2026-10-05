package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The server and the ingress must not be the OOM killer's first pick, and
// must not be unkillable either (ADR-085). Project VMs run at oom_score_adj
// -999 (cloud-hypervisor inherits it from containerd's shim) and, since
// ADR-083, are sized to the whole host. At the default 0 the platform died
// first (security review of v1.42.0); at -1000 (ADR-084, never released) a
// leak or a child git could hang the host (security review of v1.42.1). At
// -999 the tie with the VMs goes to the largest process, a VM, and the memory
// cap makes a leak inside either service an OOM kill of that container,
// which its restart policy recovers. This pins both in the compose file the
// auto-updater deploys.
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
	// Measured 2026-10-05 on the NUC: the server at about 9 MiB, Caddy at
	// about 40 MiB; the caps leave two orders of magnitude of headroom.
	memLimits := map[string]string{"caddy": "1g", "styrman": "4g"}
	for _, name := range []string{"caddy", "styrman"} {
		lines, ok := blocks[name]
		if !ok {
			t.Fatalf("service %q not found in docker-compose.yml", name)
		}
		has := func(want string) bool {
			for _, l := range lines {
				if l == want {
					return true
				}
			}
			return false
		}
		if !has("oom_score_adj: -999") {
			t.Errorf("service %q: want `oom_score_adj: -999`", name)
		}
		if !has("mem_limit: " + memLimits[name]) {
			t.Errorf("service %q: want `mem_limit: %s`", name, memLimits[name])
		}
		// The Go runtime's soft limit sits under the server's cap, so the GC
		// works hard before the kernel kills (security review of v1.42.1).
		if name == "styrman" && !strings.Contains(string(src), "      - GOMEMLIMIT=3GiB\n") {
			t.Errorf("service %q: want `GOMEMLIMIT=3GiB` in its environment", name)
		}
	}
}
