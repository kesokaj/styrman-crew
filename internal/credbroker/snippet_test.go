package credbroker

import (
	"bufio"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

// The guest snippet that materializes brokered credentials
// (image/configs/styrman-credentials.sh) must use the broker address the
// server injects and nothing derived in the guest. Its /16 arithmetic pointed
// every login shell at a dead address once bridges became /24 (BUG-054);
// this pins the contract from the broker's side of it.
const snippetPath = "../../image/configs/styrman-credentials.sh"

func TestCredentialSnippetTrustsInjectedBrokerAddress(t *testing.T) {
	b, err := os.ReadFile(snippetPath)
	if err != nil {
		t.Fatalf("read snippet: %v", err)
	}
	src := string(b)
	// Code lines only: the comment explaining the old arithmetic may name it.
	forbidden := regexp.MustCompile(`255\.254|cut -d\. -f1,2|ip -o -4 addr`)
	sc := bufio.NewScanner(strings.NewReader(src))
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "#") {
			continue
		}
		if forbidden.MatchString(line) {
			t.Fatalf("snippet line %d re-derives the broker address in the guest: %q", n, line)
		}
	}
	for _, want := range []string{
		`"${STYRMAN_CRED_BROKER}/env"`,
		`"${STYRMAN_CRED_BROKER}/files"`,
		`.styrman/broker-unreachable`,
		`__sc_fail=1`,
	} {
		if !strings.Contains(src, want) {
			t.Fatalf("snippet lacks %q", want)
		}
	}
	// The MOTD reports the marker the snippet writes.
	motd, err := os.ReadFile("../../image/configs/profile-motd.sh")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(motd), `.styrman/broker-unreachable`) {
		t.Fatal("profile-motd.sh does not report the broker-unreachable marker")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("no bash on this host")
	}
	if out, err := exec.Command("bash", "-n", snippetPath).CombinedOutput(); err != nil {
		t.Fatalf("bash -n: %v\n%s", err, out)
	}
}

// The marker rule, run for real: with a broker URL nothing answers, one
// login-shell evaluation of the snippet writes the marker; with no URL (host
// mode) it removes a stale one. The snippet is sourced under a scratch HOME.
func TestCredentialSnippetMarker(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("no bash on this host")
	}
	for _, tool := range []string{"curl", "jq"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("no %s on this host", tool)
		}
	}
	home := t.TempDir()
	run := func(env ...string) {
		t.Helper()
		cmd := exec.Command("bash", "-c", ". "+snippetPath+"; exit 0")
		cmd.Env = append([]string{"HOME=" + home, "PATH=" + os.Getenv("PATH")}, env...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("snippet: %v\n%s", err, out)
		}
	}
	marker := home + "/.styrman/broker-unreachable"
	run("STYRMAN_CRED_BROKER=http://127.0.0.1:1")
	b, err := os.ReadFile(marker)
	if err != nil || !strings.Contains(string(b), "http://127.0.0.1:1") {
		t.Fatalf("marker after an unreachable broker: %q, %v", b, err)
	}
	run()
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("marker kept with no broker configured")
	}
}
