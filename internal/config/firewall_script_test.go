package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The root firewall script's behavioural test lives here, in the package that
// owns BridgeNameFor, because the script's entire job is matching the `sdb-`
// prefix that function produces (ADR-067). It used to live in
// internal/netpolicy, which ADR-067 deleted; moving it rather than dropping it
// keeps `go test ./...` exercising the real script.

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

// TestFirewallScriptMatchesBridgePrefixes is the repo-level drift guard that
// replaced the old STYRMAN_FW_VERSION handshake: nothing gates on a version
// any more, but the script and BridgeNameFor must still agree about which
// prefix is confined. If BridgeNameFor's sandbox prefix ever changes without
// the script's wildcard changing with it, every project silently loses its
// isolation — so assert the two literally.
func TestFirewallScriptMatchesBridgePrefixes(t *testing.T) {
	src, err := os.ReadFile(filepath.Join(repoRoot(t), "deploy", "firewall", "styrman-firewall.sh"))
	if err != nil {
		t.Fatal(err)
	}
	script := string(src)

	sandbox := BridgeNameFor(NetworkName("3e6a772c-b177-4cfc-b209-b14db514bac2"), false)
	wildcard := sandbox[:4] + "+" // sdb-+
	for _, want := range []string{
		"ensure_jump  INPUT " + wildcard + " STYRMAN-IN",
		"ensure_jump  DOCKER-USER " + wildcard + " STYRMAN-FWD",
		// A missing xt_physdev blocks the broker for every sandbox project:
		// it must go through warn() into the exit path, not a bare echo the
		// unit reads as success (audit 2026-09-13).
		`--physdev-is-bridged -j RETURN 2>/dev/null \
    || warn "xt_physdev unavailable`,
	} {
		if !strings.Contains(script, want) {
			t.Errorf("styrman-firewall.sh does not carry %q — sandbox projects would not be confined", want)
		}
	}

	// And no RULE may name the LAN prefix: the whole design is that a LAN
	// project is matched by nothing. Comments are excluded deliberately — the
	// script's header explains the prefix at length, and asserting on prose
	// would make this test fail for a documentation edit.
	lan := BridgeNameFor(NetworkName("3e6a772c-b177-4cfc-b209-b14db514bac2"), true)
	for i, line := range strings.Split(script, "\n") {
		code, _, _ := strings.Cut(line, "#")
		if strings.Contains(code, lan[:4]) {
			t.Errorf("styrman-firewall.sh:%d names the LAN bridge prefix %q in code — a LAN project must be matched by no rule: %s",
				i+1, lan[:4], strings.TrimSpace(line))
		}
	}
}

// TestFirewallScriptInNetns runs deploy/firewall/test.sh — the real firewall
// script against scratch chains — inside an unprivileged user+net+mount
// namespace with its own sysfs (a plain `unshare -Urn` still shows the HOST's
// /sys/class/net). It skips, loudly, where that is unavailable; the verifier
// must report the skip rather than count it as coverage. When passwordless
// sudo is available the host's STYRMAN-FWD chain is compared before and
// after, proving the run never touched it.
func TestFirewallScriptInNetns(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux only")
	}
	if _, err := exec.LookPath("unshare"); err != nil {
		t.Skip("unshare not installed")
	}
	if _, err := os.Stat("/usr/sbin/iptables"); err != nil {
		t.Skip("/usr/sbin/iptables not installed")
	}
	if err := exec.Command("unshare", "-Urnm", "sh", "-c", "mount -t sysfs sysfs /sys").Run(); err != nil {
		t.Skipf("unprivileged user+net+mount namespace with a private sysfs unavailable here: %v", err)
	}
	hostChain := func() (string, bool) {
		if exec.Command("sudo", "-n", "true").Run() != nil {
			return "", false
		}
		out, err := exec.Command("sudo", "-n", "iptables", "-S", "STYRMAN-FWD").CombinedOutput()
		if err != nil {
			return "", false
		}
		return string(out), true
	}
	before, canCompare := hostChain()
	if !canCompare {
		t.Log("passwordless sudo unavailable: the host-chain-unchanged assertion is skipped")
	}

	script := filepath.Join(repoRoot(t), "deploy", "firewall", "test.sh")
	cmd := exec.Command("unshare", "-Urnm", "bash", script)
	cmd.Env = append(os.Environ(), "PATH=/usr/sbin:/sbin:"+os.Getenv("PATH"))
	out, err := cmd.CombinedOutput()
	t.Logf("deploy/firewall/test.sh:\n%s", out)
	if err != nil {
		t.Fatalf("deploy/firewall/test.sh failed: %v", err)
	}
	if canCompare {
		after, _ := hostChain()
		if before != after {
			t.Fatalf("host STYRMAN-FWD chain changed during the namespaced test:\nbefore:\n%s\nafter:\n%s", before, after)
		}
	}
}
