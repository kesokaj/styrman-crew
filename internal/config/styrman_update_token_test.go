package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// An empty or newline-only GitHub token file is refused by name, by both
// the updater (whose fetch helper and build secret read it) and styrman-ctl
// (install's host-setup and build secret, update --setup), instead of
// authenticating as nobody and failing later as "repository not found"
// (v1.42.3 gate report, low).
func TestEmptyTokenFilesAreRefused(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("no bash on this host")
	}
	script, err := filepath.Abs("../../deploy/update/styrman-update")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	check := func(content string) (int, string) {
		t.Helper()
		f := filepath.Join(dir, "gh-token")
		if err := os.WriteFile(f, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("bash", script, "__check-token-file", f)
		cmd.Env = append(os.Environ(), "STYRMAN_UPDATE_TEST=1")
		out, err := cmd.CombinedOutput()
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode(), string(out)
		} else if err != nil {
			t.Fatal(err)
		}
		return 0, string(out)
	}
	for _, empty := range []string{"", "\n", " \n\t\n"} {
		if code, out := check(empty); code == 0 || !strings.Contains(out, "is empty") {
			t.Errorf("updater accepted token file %q: %d %s", empty, code, out)
		}
	}
	if code, out := check("github_pat_x\n"); code != 0 {
		t.Fatalf("updater refused a real token: %d %s", code, out)
	}

	// styrman-ctl: install --gh-token-file and update --setup --gh-token-file.
	e := ctlSetup(t, true)
	tok := filepath.Join(e.root, "secrets", "tok")
	if err := os.WriteFile(tok, []byte("\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	envFile := filepath.Join(e.root, "checkout.env")
	if err := os.WriteFile(envFile, []byte("BASE_DOMAIN=example.test\nGOOGLE_CLIENT_ID=i\nGOOGLE_CLIENT_SECRET=s\nADMIN_EMAIL=a@example.test\nSTYRMAN_ALLOWED_EMAILS=a@example.test\nSTYRMAN_HOSTNAME=crew\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	e.env = append(e.env, "STYRMAN_ENV_FILE="+envFile, "SUDO_USER=tester")
	if out, code := ctl(t, e, "install", "--dry-run", "--yes", "--version", "1.2.3", "--gh-token-file", tok, "--skip-auto-update"); code == 0 || !strings.Contains(out, "is empty") {
		t.Errorf("install accepted an empty token file: %d %s", code, out)
	}
	if out, code := ctl(t, e, "update", "--setup", "--dry-run", "--yes", "--gh-token-file", tok, "--allowed-signers-file", filepath.Join(e.root, "secrets", "signers")); code == 0 || !strings.Contains(out, "is empty") {
		t.Errorf("update --setup accepted an empty token file: %d %s", code, out)
	}
}
