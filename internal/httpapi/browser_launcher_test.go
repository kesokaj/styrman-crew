package httpapi

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kesokaj/styrman-crew/internal/config"
)

// launcherPath is the in-guest launcher as committed, reached from this
// package's directory (go test runs with the package dir as cwd).
const launcherPath = "../../image/configs/styrman-browser"

func launcherSource(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(launcherPath)
	if err != nil {
		t.Fatalf("read launcher: %v", err)
	}
	return string(b)
}

// The launcher and the server agree on the two ports by convention only —
// the script ships in the image, the constants in the server — and on how
// many forwards socat admits per project: at least twice the relays one
// user may hold, so a user with every pane open plus their discovery
// requests is never refused at the guest's door.
func TestLauncherPinsPortsAndCaps(t *testing.T) {
	src := launcherSource(t)
	pin := func(re string) int {
		m := regexp.MustCompile(re).FindStringSubmatch(src)
		if m == nil {
			t.Fatalf("launcher: no match for %s", re)
		}
		n, err := strconv.Atoi(m[1])
		if err != nil {
			t.Fatalf("launcher: %s = %q", re, m[1])
		}
		return n
	}
	if got := pin(`(?m)^PORT=(\d+)`); got != config.BrowserPort {
		t.Fatalf("launcher PORT=%d, server config.BrowserPort=%d", got, config.BrowserPort)
	}
	if got := pin(`(?m)^CHROME_PORT=(\d+)`); got != config.BrowserChromePort {
		t.Fatalf("launcher CHROME_PORT=%d, server config.BrowserChromePort=%d", got, config.BrowserChromePort)
	}
	if got := pin(`max-children=(\d+)`); got < 2*browserRelayPerUser {
		t.Fatalf("launcher max-children=%d < 2 × browserRelayPerUser (%d)", got, browserRelayPerUser)
	}
	// The MCP verb must not carry the start lock into the MCP server.
	if !strings.Contains(src, "exec chrome-devtools-mcp --browserUrl \"http://127.0.0.1:${CHROME_PORT}\" 9>&-") {
		t.Fatal("do_mcp execs without closing the start lock (fd 9)")
	}
}

// launcherFunctions evals the launcher's definitions (everything before its
// verb dispatch) in a bash whose HOME is a scratch dir, then runs body.
func launcherFunctions(t *testing.T, home, body string, env ...string) (string, error) {
	t.Helper()
	src := launcherSource(t)
	i := strings.Index(src, "\ncase \"${1:-}\" in")
	if i < 0 {
		t.Fatal("launcher: no verb dispatch found")
	}
	cmd := exec.Command("bash", "-c", src[:i]+"\n"+body)
	cmd.Env = append([]string{"HOME=" + home, "PATH=" + os.Getenv("PATH")}, env...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// peer_range's two answers for "no range": unset (an agent-started browser,
// loopback-only is fine — exit 0) and set-but-unusable (a server that could
// not learn its own address — exit 1, so the start fails instead of leaving
// a browser the pane can never reach).
func TestLauncherPeerRange(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the launcher's functions run in the Linux guest")
	}
	home := t.TempDir()
	cases := []struct {
		name string
		env  []string
		want string
		ok   bool
	}{
		{"server peer", []string{"STYRMAN_BROWSER_PEER=172.16.0.254"}, "172.16.0.254/32", true},
		{"server peer empty", []string{"STYRMAN_BROWSER_PEER="}, "", false},
		{"server peer not an ip", []string{"STYRMAN_BROWSER_PEER=bridge,fork"}, "", false},
		{"agent start, broker fallback", []string{"STYRMAN_CRED_BROKER=http://172.16.0.2:8091"}, "172.16.0.2/32", true},
		{"agent start, no broker", nil, "", true},
		{"agent start, broker not an ip", []string{"STYRMAN_CRED_BROKER=http://broker.local:8091"}, "", true},
	}
	for _, c := range cases {
		out, err := launcherFunctions(t, home, "peer_range", c.env...)
		if (err == nil) != c.ok || strings.TrimSpace(out) != c.want {
			t.Errorf("%s: out %q err %v, want %q ok=%v", c.name, out, err, c.want, c.ok)
		}
	}
}

// kill_group_file signals only OUR group: a pid file naming a live process
// with another name (a recycled pid after a VM restart) or a pid that no
// longer exists is dropped without a signal; a group we started goes down.
func TestLauncherKillGroupFile(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the launcher's functions run in the Linux guest")
	}
	if _, err := exec.LookPath("setsid"); err != nil {
		t.Skip("no setsid on this host")
	}
	home := t.TempDir()
	pidfile := filepath.Join(home, "pid")
	// A recycled pid: pid 1 is never chromium.
	if err := os.WriteFile(pidfile, []byte("1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := launcherFunctions(t, home, `kill_group_file "$HOME/pid" chrom`)
	if err != nil || !strings.Contains(out, "stale, not signalled") {
		t.Fatalf("recycled pid: out %q err %v", out, err)
	}
	if _, err := os.Stat(pidfile); !os.IsNotExist(err) {
		t.Fatal("stale pid file kept")
	}
	// A pid that does not exist (and a corrupt file) are dropped quietly.
	for _, content := range []string{"4000000000\n", "not-a-pid\n"} {
		if err := os.WriteFile(pidfile, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if out, err := launcherFunctions(t, home, `kill_group_file "$HOME/pid" chrom`); err != nil || out != "" {
			t.Fatalf("%q: out %q err %v", content, out, err)
		}
	}
	// A group we started (a session leader that exec'd sleep) is taken down.
	// Reap it: a child nobody waits for stays a zombie that pgrep still
	// lists and /proc still shows (the guest's launcher double-forks with
	// `setsid -f`, so init reaps there).
	leader := exec.Command("setsid", "bash", "-c", `echo $$ > "$1"; exec sleep 300`, "_", pidfile)
	if err := leader.Start(); err != nil {
		t.Fatal(err)
	}
	go func() { _ = leader.Wait() }()
	var pid string
	for i := 0; i < 50 && pid == ""; i++ {
		b, _ := os.ReadFile(pidfile)
		pid = strings.TrimSpace(string(b))
		time.Sleep(20 * time.Millisecond)
	}
	if pid == "" {
		t.Fatal("leader never wrote its pid")
	}
	if out, err := launcherFunctions(t, home, `kill_group_file "$HOME/pid" sleep`); err != nil {
		t.Fatalf("kill: out %q err %v", out, err)
	}
	for i := 0; i < 50; i++ {
		if _, err := os.Stat("/proc/" + pid); os.IsNotExist(err) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("pid %s still alive after kill_group_file", pid)
}
