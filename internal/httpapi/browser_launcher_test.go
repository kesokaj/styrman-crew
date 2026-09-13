package httpapi

import (
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"syscall"
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

// port_inodes names the listening sockets on the launcher's port and
// owns_socket ties a pid to one of them through /proc/<pid>/fd; forwarder_up
// refuses a recorded pid that is not socat even when it IS the owner.
func TestLauncherPortOwnership(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the launcher's functions run in the Linux guest")
	}
	if _, err := exec.LookPath("ss"); err != nil {
		t.Skip("no ss on this host")
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)
	me := strconv.Itoa(os.Getpid())
	// This process's own view of the listener's inode.
	var ino string
	raw, err := ln.(*net.TCPListener).SyscallConn()
	if err != nil {
		t.Fatal(err)
	}
	if err := raw.Control(func(fd uintptr) {
		link, _ := os.Readlink("/proc/self/fd/" + strconv.Itoa(int(fd)))
		ino = strings.TrimSuffix(strings.TrimPrefix(link, "socket:["), "]")
	}); err != nil || ino == "" {
		t.Fatalf("listener inode: %q, %v", ino, err)
	}
	home := t.TempDir()
	out, err := launcherFunctions(t, home, "PORT="+port+"; port_inodes")
	if err != nil || !slices.Contains(strings.Fields(out), ino) {
		t.Fatalf("port_inodes = %q (err %v), want to include %s", out, err, ino)
	}
	if out, err := launcherFunctions(t, home, "owns_socket "+me+" "+ino); err != nil {
		t.Fatalf("owns_socket refused this process's own listener: %q %v", out, err)
	}
	if out, err := launcherFunctions(t, home, "owns_socket "+me+" 1"); err == nil {
		t.Fatalf("owns_socket accepted a socket this process does not hold: %q", out)
	}
	// This test binary owns the listener but is not socat: not our forwarder.
	dir := filepath.Join(home, ".styrman", "browser")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "socat.pid"), []byte(me+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := launcherFunctions(t, home, "PORT="+port+"; forwarder_up"); err == nil {
		t.Fatalf("forwarder_up accepted a non-socat owner: %q", out)
	}
	// A zero-padded pid is refused before /proc is consulted.
	if err := os.WriteFile(filepath.Join(dir, "socat.pid"), []byte("0"+me+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := launcherFunctions(t, home, "PORT="+port+"; forwarder_up"); err == nil {
		t.Fatalf("forwarder_up accepted a zero-padded pid: %q", out)
	}
	// The positive control: a process whose comm IS socat (this test binary
	// re-exec'd through a symlink of that name) holding a listener on the
	// port is our forwarder. Without this case a forwarder_up that always
	// fails would stay green (security review, round 4).
	ln.Close()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	fake := filepath.Join(home, "socat")
	if err := os.Symlink(exe, fake); err != nil {
		t.Fatal(err)
	}
	helper := exec.Command(fake, "-test.run=TestHelperListener")
	helper.Env = append(os.Environ(), "STYRMAN_HELPER_LISTEN="+port)
	helperOut, err := helper.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := helper.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = helper.Process.Kill(); _ = helper.Wait() })
	ready := make([]byte, 6)
	if _, err := io.ReadFull(helperOut, ready); err != nil || string(ready) != "ready\n" {
		t.Fatalf("helper listener never became ready: %q %v", ready, err)
	}
	hp := strconv.Itoa(helper.Process.Pid)
	if err := os.WriteFile(filepath.Join(dir, "socat.pid"), []byte(hp+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := launcherFunctions(t, home, "PORT="+port+"; forwarder_up"); err != nil {
		comm, _ := os.ReadFile("/proc/" + hp + "/comm")
		t.Fatalf("forwarder_up refused our own socat-named listener (pid %s, comm %q): %q %v", hp, comm, out, err)
	}
}

// TestHelperListener is not a test: re-exec'd by TestLauncherPortOwnership
// (through a symlink named socat) it listens on the given port, prints
// "ready" and waits to be killed.
func TestHelperListener(t *testing.T) {
	port := os.Getenv("STYRMAN_HELPER_LISTEN")
	if port == "" {
		t.Skip("helper process only")
	}
	ln, err := net.Listen("tcp", "127.0.0.1:"+port)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	fmt.Println("ready")
	select {}
}

// kill_group_file signals only OUR group: a pid file naming a live process
// with another name (a recycled pid after a VM restart) or a pid that no
// longer exists is dropped without a signal; a group we started goes down.
func TestLauncherKillGroupFile(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the launcher's functions run in the Linux guest")
	}
	home := t.TempDir()
	pidfile := filepath.Join(home, "pid")
	// startLeader starts a session leader that exec's sleep and returns ITS
	// pid from the process handle — never from the pid file, which is the
	// input under test (security review, round 2). The file is still
	// written by the leader for kill_group_file to read.
	startLeader := func() string {
		t.Helper()
		leader := exec.Command("bash", "-c", `echo $$ > "$1"; exec sleep 300`, "_", pidfile)
		leader.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		if err := leader.Start(); err != nil {
			t.Fatal(err)
		}
		// Reap it: a child nobody waits for stays a zombie that pgrep still
		// lists and /proc still shows (the guest's launcher double-forks
		// with `setsid -f`, so init reaps there).
		go func() { _ = leader.Wait() }()
		pid := strconv.Itoa(leader.Process.Pid)
		for i := 0; i < 50; i++ {
			if b, err := os.ReadFile(pidfile); err == nil && strings.TrimSpace(string(b)) == pid {
				return pid
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatalf("leader %s never wrote its pid file", pid)
		return pid
	}
	// A recycled pid: a live process with another name (here a sleep we
	// own, never pid 1 — a regressed check must not be able to signal the
	// machine) is dropped and left alone.
	stranger := startLeader()
	t.Cleanup(func() { _ = exec.Command("kill", "-9", "--", "-"+stranger).Run() })
	out, err := launcherFunctions(t, home, `kill_group_file "$HOME/pid" chrom`)
	if err != nil || !strings.Contains(out, "stale, not signalled") {
		t.Fatalf("recycled pid: out %q err %v", out, err)
	}
	if _, err := os.Stat(pidfile); !os.IsNotExist(err) {
		t.Fatal("stale pid file kept")
	}
	if _, err := os.Stat("/proc/" + stranger); err != nil {
		t.Fatalf("the stranger (pid %s) was signalled", stranger)
	}
	// Pids below 2 (0 = the caller's own group to pgrep/kill), a zero-padded
	// pid, a pid that does not exist, and a corrupt file are dropped quietly.
	for _, content := range []string{"0\n", "1\n", "0" + stranger + "\n", "4000000000\n", "not-a-pid\n"} {
		if err := os.WriteFile(pidfile, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if out, err := launcherFunctions(t, home, `kill_group_file "$HOME/pid" chrom`); err != nil || out != "" {
			t.Fatalf("%q: out %q err %v", content, out, err)
		}
	}
	// A group we started (a session leader that exec'd sleep) is taken down.
	pid := startLeader()
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
