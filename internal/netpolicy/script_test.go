package netpolicy

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"testing"
)

// repoRoot locates the module root from this file's position so the tests
// find deploy/firewall/ whatever the working directory is.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

// TestMinScriptVersionMatchesScript is the repo-level drift guard: the Go
// constant and the script's STYRMAN_FW_VERSION move together, or a release
// would ship a server that refuses the very script it deploys.
func TestMinScriptVersionMatchesScript(t *testing.T) {
	src, err := os.ReadFile(filepath.Join(repoRoot(t), "deploy", "firewall", "styrman-firewall.sh"))
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^STYRMAN_FW_VERSION=(\d+)$`).FindSubmatch(src)
	if m == nil {
		t.Fatal("deploy/firewall/styrman-firewall.sh has no plain STYRMAN_FW_VERSION=N line")
	}
	got, _ := strconv.Atoi(string(m[1]))
	if got != MinScriptVersion {
		t.Fatalf("script STYRMAN_FW_VERSION=%d but netpolicy.MinScriptVersion=%d — bump both together", got, MinScriptVersion)
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
