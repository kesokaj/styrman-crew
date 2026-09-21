package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// deploy/gcp/vm.sh and deploy/gcp/install.sh are exercised the way
// deploy/styrman-ctl is (styrman_ctl_test.go): the real scripts, a fake
// gcloud/git/curl on PATH that record their argv and answer from FAKE_*
// environment knobs, HOME under t.TempDir(), and the --dry-run plan as the
// seam. No test here may reach a real gcloud, and a secret value planted in
// the env file must never appear in any output.

type gcpEnv struct {
	root, bin, calls, secrets string
	env                       []string
}

// gcpSetup plants the fakes and a complete, private secrets directory at the
// scripts' default location ($HOME/.config/styrman/gcp).
func gcpSetup(t *testing.T) gcpEnv {
	t.Helper()
	root := t.TempDir()
	bin := filepath.Join(root, "fakebin")
	calls := filepath.Join(root, "calls.log")
	secrets := filepath.Join(root, ".config", "styrman", "gcp")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(secrets, 0o700); err != nil {
		t.Fatal(err)
	}
	fake := func(name, body string) {
		script := "#!/usr/bin/env bash\necho \"" + name + " $*\" >> \"" + calls + "\"\n" + body + "\n"
		if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// describe answers print what the scripts' --format asks for: status and
	// external IP, tab-separated. FAKE_INSTANCE_RC=1 means "no such instance".
	fake("gcloud", `case "$*" in
  *"instances describe"*) [ "${FAKE_INSTANCE_RC:-1}" = 0 ] && printf '%s\t%s\n' "${FAKE_INSTANCE_STATUS:-RUNNING}" 203.0.113.5; exit "${FAKE_INSTANCE_RC:-1}" ;;
  *"addresses describe"*) [ "${FAKE_ADDRESS_RC:-1}" = 0 ] && echo 203.0.113.5; exit "${FAKE_ADDRESS_RC:-1}" ;;
  *"compute ssh"*)
    [ "${FAKE_SSH_RC:-0}" = 0 ] || exit "${FAKE_SSH_RC}"
    case "$*" in
      *"/dev/kvm"*) exit "${FAKE_KVM_RC:-0}" ;;
      *"styrman-crew/.git"*) exit "${FAKE_CLONE_EXISTS_RC:-1}" ;;
      *"styrman-ctl"*) exit "${FAKE_CTL_RC:-1}" ;;
    esac
    exit 0 ;;
esac
exit 0`)
	fake("git", `case "$*" in *get-url*) echo git@github.com:kesokaj/styrman-crew.git ;; *) echo v1.36.0 ;; esac`)
	fake("curl", `echo "302 https://auth.gcp.example.test/oauth2/google"`)
	for name, body := range map[string]string{
		"env":             "BASE_DOMAIN=gcp.example.test\nGOOGLE_CLIENT_ID=id.apps\nGOOGLE_CLIENT_SECRET=SENTINEL-abc\nCLOUDFLARE_API_TOKEN=SENTINEL-cf\nADMIN_EMAIL=me@example.test\nSTYRMAN_ALLOWED_EMAILS=me@example.test\nSTYRMAN_HOSTNAME=crew\n",
		"gh-token":        "SENTINEL-ghtoken\n",
		"allowed_signers": "me@example.test ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIExample\n",
	} {
		if err := os.WriteFile(filepath.Join(secrets, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	env := append(os.Environ(),
		"PATH="+bin+":"+os.Getenv("PATH"),
		"HOME="+root,
	)
	return gcpEnv{root: root, bin: bin, calls: calls, secrets: secrets, env: env}
}

// gcpRun runs one of the two scripts with extra environment and returns its
// combined output and exit code; any secret sentinel in the output fails the
// test at once, whatever the script was asked to do.
func gcpRun(t *testing.T, e gcpEnv, script string, extraEnv []string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(filepath.Join(repoRoot(t), "deploy", "gcp", script), args...)
	cmd.Env = append(append([]string{}, e.env...), extraEnv...)
	out, err := cmd.CombinedOutput()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("%s %v: %v\n%s", script, args, err, out)
	}
	if strings.Contains(string(out), "SENTINEL-") {
		t.Fatalf("%s %v printed a secret value:\n%s", script, args, out)
	}
	return string(out), code
}

func gcpCalls(t *testing.T, e gcpEnv) string {
	t.Helper()
	b, err := os.ReadFile(e.calls)
	if os.IsNotExist(err) {
		return ""
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestGCPVMCreateNeedsYesWithoutTerminal(t *testing.T) {
	e := gcpSetup(t)
	out, code := gcpRun(t, e, "vm.sh", nil, "--dry-run", "create")
	if code != 1 || !strings.Contains(out, "--yes") {
		t.Fatalf("create without a terminal and without --yes: code %d, want 1 naming --yes:\n%s", code, out)
	}
	if strings.Contains(out, "instances create") {
		t.Fatalf("a refused create still planned the instance:\n%s", out)
	}
}

func TestGCPVMCreatePlan(t *testing.T) {
	e := gcpSetup(t)
	out, code := gcpRun(t, e, "vm.sh", nil, "--dry-run", "--yes", "create")
	if code != 0 {
		t.Fatalf("dry-run create: code %d\n%s", code, out)
	}
	if n := strings.Count(out, "instances create"); n != 1 {
		t.Fatalf("want exactly one instances create line, got %d:\n%s", n, out)
	}
	if n := strings.Count(out, "addresses create"); n != 1 {
		t.Fatalf("want exactly one addresses create line, got %d:\n%s", n, out)
	}
	for _, flag := range []string{
		"--enable-nested-virtualization",
		"--no-service-account",
		"--no-scopes",
		"--tags=http,https",
		"--boot-disk-type=hyperdisk-balanced",
		"--machine-type=n4-standard-8",
		"--address=styrman-static",
		"--image-family=debian-13",
		"--zone=europe-north2-a",
		"--project=driven-eft-h4it",
		"block-project-ssh-keys=TRUE",
	} {
		if !strings.Contains(out, flag) {
			t.Errorf("plan lacks %s:\n%s", flag, out)
		}
	}
	if !strings.Contains(out, "would wait for ssh") {
		t.Errorf("dry run must skip the ssh wait and say so:\n%s", out)
	}
	// Existing address and instance: nothing is created, exit 0.
	out, code = gcpRun(t, e, "vm.sh", []string{"FAKE_INSTANCE_RC=0", "FAKE_ADDRESS_RC=0"}, "--dry-run", "--yes", "create")
	if code != 0 || strings.Contains(out, " create ") {
		t.Fatalf("create on an existing instance must be a no-op: code %d\n%s", code, out)
	}
	if !strings.Contains(out, "RUNNING") || !strings.Contains(out, "203.0.113.5") {
		t.Fatalf("the no-op path prints the state and IP:\n%s", out)
	}
}

func TestGCPVMDeleteRefusesWithoutConfirm(t *testing.T) {
	e := gcpSetup(t)
	out, code := gcpRun(t, e, "vm.sh", []string{"FAKE_INSTANCE_RC=0", "FAKE_ADDRESS_RC=0"}, "--dry-run", "delete")
	if code != 2 || strings.Contains(out, "delete --quiet") {
		t.Fatalf("delete without --confirm: code %d, want 2 and no delete in the plan:\n%s", code, out)
	}
	out, code = gcpRun(t, e, "vm.sh", []string{"FAKE_INSTANCE_RC=0", "FAKE_ADDRESS_RC=0"}, "--dry-run", "delete", "--confirm", "styrman")
	if code != 0 {
		t.Fatalf("delete --confirm styrman: code %d\n%s", code, out)
	}
	i, a := strings.Index(out, "instances delete"), strings.Index(out, "addresses delete")
	if i < 0 || a < 0 || i > a {
		t.Fatalf("instance delete must come before address delete:\n%s", out)
	}
	out, code = gcpRun(t, e, "vm.sh", nil, "--dry-run", "delete", "--confirm", "wrong")
	if code == 0 || strings.Contains(out, "delete --quiet") {
		t.Fatalf("delete --confirm wrong must refuse: code %d\n%s", code, out)
	}
}

func TestGCPVMRejectsBadNames(t *testing.T) {
	e := gcpSetup(t)
	out, code := gcpRun(t, e, "vm.sh", []string{"GCP_INSTANCE=x;rm"}, "--dry-run", "--yes", "create")
	if code != 1 || !strings.Contains(out, "GCP_INSTANCE") {
		t.Fatalf("bad instance name: code %d, want 1 naming GCP_INSTANCE:\n%s", code, out)
	}
	if c := gcpCalls(t, e); c != "" {
		t.Fatalf("a bad name reached a tool:\n%s", c)
	}
	out, code = gcpRun(t, e, "install.sh", []string{"FAKE_INSTANCE_RC=0"}, "--dry-run", "--version", "1.36.0", "--user", "a b")
	if code != 1 || !strings.Contains(out, "--user") {
		t.Fatalf("bad --user: code %d, want 1 naming --user:\n%s", code, out)
	}
	if c := gcpCalls(t, e); c != "" {
		t.Fatalf("a bad --user reached a tool:\n%s", c)
	}
}

func TestGCPInstallPlan(t *testing.T) {
	e := gcpSetup(t)
	out, code := gcpRun(t, e, "install.sh", []string{"FAKE_INSTANCE_RC=0"}, "--dry-run", "--version", "1.36.0", "--user", "kesokaj")
	if code != 0 {
		t.Fatalf("dry-run install: code %d\n%s", code, out)
	}
	for _, want := range []string{
		"compute scp",
		"apt-get update",
		"--branch v1.36.0",
		"styrman-ctl install --yes",
		"--version 1.36.0",
		"--user kesokaj",
		"rm -rf",
		"curl",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("plan lacks %q:\n%s", want, out)
		}
	}
	// The three secret files travel by scp, never by argument, through IAP.
	for _, f := range []string{"env", "gh-token", "allowed_signers"} {
		if !strings.Contains(out, filepath.Join(e.secrets, f)) {
			t.Errorf("plan does not scp %s:\n%s", f, out)
		}
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "compute scp") && !strings.Contains(line, "--tunnel-through-iap") {
			t.Errorf("scp does not tunnel through IAP: %s", line)
		}
	}
	// Every remote command is its own ssh exec: the install line must name
	// the staged token file itself, never a variable another call's env line
	// set (review 2026-09-21, high: GH_TOKEN was always empty on the VM).
	var install string
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "styrman-ctl install --yes") {
			install = line
		}
	}
	if install == "" {
		t.Fatalf("no styrman-ctl install line in the plan:\n%s", out)
	}
	// The token VALUE never enters an argv on the VM (sudo logs argv, and
	// /proc/<pid>/cmdline is world-readable): only the file path travels,
	// through --gh-token-file, and styrman-ctl exports it for host-setup.sh
	// itself (security review 2026-09-21, high).
	if !strings.Contains(install, `--gh-token-file "$HOME/.styrman-install/gh-token"`) || strings.Contains(install, "GH_TOKEN=") || strings.Contains(install, "STYRMAN_GH_TOKEN_FILE") {
		t.Fatalf("the install line must pass the token by path only: %s", install)
	}
	// The checked-out release tag is signature-verified against the staged
	// allowed_signers before anything runs from it.
	var verify string
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "verify-tag") {
			verify = line
		}
	}
	if !strings.Contains(verify, `verify-tag "v1.36.0"`) || !strings.Contains(verify, `gpg.ssh.allowedSignersFile="$HOME/.styrman-install/allowed_signers"`) {
		t.Fatalf("no tag verification against the staged signers in the plan:\n%s", out)
	}
	if strings.Index(out, "verify-tag") > strings.Index(out, "styrman-ctl install --yes") {
		t.Fatalf("verify-tag must run before styrman-ctl install:\n%s", out)
	}
}

// An unreachable VM is reported as exactly that — never as "not installed"
// (status) or "no /dev/kvm" (install), which is what the first bring-up
// printed when the operator's network dropped port 22.
func TestGCPSSHUnreachableIsNamed(t *testing.T) {
	e := gcpSetup(t)
	out, code := gcpRun(t, e, "vm.sh", []string{"FAKE_INSTANCE_RC=0", "FAKE_SSH_RC=255"}, "status")
	if code != 1 || !strings.Contains(out, "unreachable") || strings.Contains(out, "not installed") {
		t.Fatalf("status with ssh down: code %d, want 1 naming unreachable, not 'not installed':\n%s", code, out)
	}
	out, code = gcpRun(t, e, "install.sh", []string{"FAKE_INSTANCE_RC=0", "FAKE_SSH_RC=255"}, "--dry-run", "--version", "1.36.0")
	if code != 1 || !strings.Contains(out, "ssh to styrman failed") || strings.Contains(out, "/dev/kvm") {
		t.Fatalf("install with ssh down: code %d, want 1 naming ssh, not /dev/kvm:\n%s", code, out)
	}
	if !strings.Contains(out, "--troubleshoot") {
		t.Fatalf("the refusal names gcloud's troubleshoot command:\n%s", out)
	}
}

func TestGCPInstallCloneHelperShape(t *testing.T) {
	e := gcpSetup(t)
	out, code := gcpRun(t, e, "install.sh", []string{"FAKE_INSTANCE_RC=0"}, "--dry-run", "--version", "1.36.0", "--user", "kesokaj")
	if code != 0 {
		t.Fatalf("dry-run install: code %d\n%s", code, out)
	}
	var clone string
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, " clone ") {
			clone = line
		}
	}
	if clone == "" {
		t.Fatalf("no clone line in the plan:\n%s", out)
	}
	for _, want := range []string{
		"STYRMAN_GH_TOKEN_FILE=",
		"GIT_TERMINAL_PROMPT=0",
		"credential.helper=",
		"credential.https://github.com.helper=",
		`"${STYRMAN_GH_TOKEN_FILE}"`,
		"https://github.com/kesokaj/styrman-crew",
	} {
		if !strings.Contains(clone, want) {
			t.Errorf("clone line lacks %q: %s", want, clone)
		}
	}
	// The staged token path appears only on the env line, never inside the
	// helper snippet the helper's shell parses (styrman-ctl, security review
	// 2026-09-16).
	helper := clone[strings.Index(clone, "credential.https://github.com.helper="):]
	if strings.Contains(helper, ".styrman-install/gh-token") {
		t.Fatalf("token path interpolated into the helper snippet: %s", helper)
	}
}

func TestGCPInstallRefusesLooseEnvFile(t *testing.T) {
	e := gcpSetup(t)
	envPath := filepath.Join(e.secrets, "env")
	if err := os.Chmod(envPath, 0o644); err != nil {
		t.Fatal(err)
	}
	out, code := gcpRun(t, e, "install.sh", []string{"FAKE_INSTANCE_RC=0"}, "--dry-run", "--version", "1.36.0")
	if code != 1 || !strings.Contains(out, "chmod 600") {
		t.Fatalf("world-readable env accepted: code %d\n%s", code, out)
	}
	if err := os.Chmod(envPath, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(envPath, []byte("BASE_DOMAIN=gcp.example.test\nGOOGLE_CLIENT_ID=id\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, code = gcpRun(t, e, "install.sh", []string{"FAKE_INSTANCE_RC=0"}, "--dry-run", "--version", "1.36.0")
	if code != 1 || !strings.Contains(out, "GOOGLE_CLIENT_SECRET") {
		t.Fatalf("missing key accepted: code %d, want 1 naming GOOGLE_CLIENT_SECRET\n%s", code, out)
	}
	if c := gcpCalls(t, e); c != "" {
		t.Fatalf("a refused env file still reached a tool:\n%s", c)
	}
	e = gcpSetup(t)
	tok := filepath.Join(e.secrets, "gh-token")
	if err := os.Remove(tok); err != nil {
		t.Fatal(err)
	}
	out, code = gcpRun(t, e, "install.sh", []string{"FAKE_INSTANCE_RC=0"}, "--dry-run", "--version", "1.36.0")
	if code != 1 || !strings.Contains(out, tok) {
		t.Fatalf("missing gh-token accepted: code %d, want 1 naming %s\n%s", code, tok, out)
	}
}

func TestGCPInstallNeedsRunningVM(t *testing.T) {
	e := gcpSetup(t)
	out, code := gcpRun(t, e, "install.sh", []string{"FAKE_INSTANCE_RC=0", "FAKE_INSTANCE_STATUS=TERMINATED"}, "--dry-run", "--version", "1.36.0")
	if code != 1 || !strings.Contains(out, "vm.sh start") {
		t.Fatalf("stopped VM: code %d, want 1 naming vm.sh start:\n%s", code, out)
	}
	out, code = gcpRun(t, e, "install.sh", []string{"FAKE_INSTANCE_RC=1"}, "--dry-run", "--version", "1.36.0")
	if code != 1 || !strings.Contains(out, "vm.sh create") {
		t.Fatalf("no VM: code %d, want 1 naming vm.sh create:\n%s", code, out)
	}
	if c := gcpCalls(t, e); strings.Contains(c, "scp") {
		t.Fatalf("a refused install still staged files:\n%s", c)
	}
}

func TestGCPInstallNeedsKVM(t *testing.T) {
	e := gcpSetup(t)
	out, code := gcpRun(t, e, "install.sh", []string{"FAKE_INSTANCE_RC=0", "FAKE_KVM_RC=1"}, "--dry-run", "--version", "1.36.0")
	if code != 1 || !strings.Contains(out, "/dev/kvm") || !strings.Contains(out, "GCP_MACHINE_TYPE") {
		t.Fatalf("no /dev/kvm: code %d, want 1 naming /dev/kvm and GCP_MACHINE_TYPE:\n%s", code, out)
	}
	if strings.Contains(out, "compute scp") || strings.Contains(out, " clone ") {
		t.Fatalf("a VM without KVM still had files staged or a clone planned:\n%s", out)
	}
}

func TestGCPVMStatusExitCodes(t *testing.T) {
	e := gcpSetup(t)
	if out, code := gcpRun(t, e, "vm.sh", []string{"FAKE_INSTANCE_RC=1"}, "status"); code != 2 {
		t.Fatalf("no instance: code %d, want 2\n%s", code, out)
	}
	if out, code := gcpRun(t, e, "vm.sh", []string{"FAKE_INSTANCE_RC=0", "FAKE_INSTANCE_STATUS=TERMINATED"}, "status"); code != 3 {
		t.Fatalf("stopped instance: code %d, want 3\n%s", code, out)
	}
	if out, code := gcpRun(t, e, "vm.sh", []string{"FAKE_INSTANCE_RC=0", "FAKE_CTL_RC=1"}, "status"); code != 1 || !strings.Contains(out, "not installed") {
		t.Fatalf("running, styrman not installed: code %d, want 1 naming not installed\n%s", code, out)
	}
	if out, code := gcpRun(t, e, "vm.sh", []string{"FAKE_INSTANCE_RC=0", "FAKE_CTL_RC=0"}, "status"); code != 0 {
		t.Fatalf("running and healthy: code %d, want 0\n%s", code, out)
	}
}
