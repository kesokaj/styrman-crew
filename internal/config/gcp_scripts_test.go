package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
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
  *"firewall-rules list"*) [ "${FAKE_FW_RC:-0}" = 0 ] || exit "${FAKE_FW_RC}"; printf '%s\n' "${FAKE_FW:-[]}"; exit 0 ;;
  *"addresses describe"*) [ "${FAKE_ADDRESS_RC:-1}" = 0 ] && echo 203.0.113.5; exit "${FAKE_ADDRESS_RC:-1}" ;;
  *"compute ssh"*)
    [ "${FAKE_SSH_RC:-0}" = 0 ] || exit "${FAKE_SSH_RC}"
    case "$*" in
      *"/dev/kvm"*) exit "${FAKE_KVM_RC:-0}" ;;
      *"styrman-crew/.git"*) exit "${FAKE_CLONE_EXISTS_RC:-1}" ;;
      *"for p in /opt/styrman"*) [ -n "${FAKE_CTL_PATH:-}" ] && echo "${FAKE_CTL_PATH}"; exit 0 ;;
      *"styrman-ctl status"*) exit "${FAKE_CTL_RC:-1}" ;;
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
	// A new session, so the child has NO controlling terminal: `go test` run
	// from a terminal would otherwise lend the script its tty, `has_tty`
	// would be true, and the scripts' `read … < /dev/tty` prompts would be
	// reached — two tests here passed for that reason without a tty and hung
	// the package with one (review 2026-09-22).
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
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
	// The optional keys styrman-ctl install does not carry are delivered
	// explicitly, or the host silently keeps its default (BUG-072).
	if !strings.Contains(out, "styrman-ctl edit STYRMAN_HOSTNAME=crew") {
		t.Errorf("the plan does not deliver STYRMAN_HOSTNAME to the host:\n%s", out)
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
	// No styrman-ctl anywhere on the host: not installed.
	if out, code := gcpRun(t, e, "vm.sh", []string{"FAKE_INSTANCE_RC=0"}, "status"); code != 1 || !strings.Contains(out, "not installed") {
		t.Fatalf("running, styrman not installed: code %d, want 1 naming not installed\n%s", code, out)
	}
	// Installed in the deploy clone (where it really lives — BUG-073: the
	// probe used to test /usr/local/bin, which nothing creates, so a serving
	// host reported "not installed"), and its status decides the exit code.
	for _, tc := range []struct {
		path, rc string
		want     int
	}{
		{"/opt/styrman/deploy/styrman-ctl", "0", 0},
		{"/opt/styrman/deploy/styrman-ctl", "1", 1},
		{"/home/kesokaj/styrman-crew/deploy/styrman-ctl", "0", 0},
	} {
		out, code := gcpRun(t, e, "vm.sh", []string{"FAKE_INSTANCE_RC=0", "FAKE_CTL_PATH=" + tc.path, "FAKE_CTL_RC=" + tc.rc}, "status")
		if code != tc.want {
			t.Fatalf("status with %s (ctl exit %s): code %d, want %d\n%s", tc.path, tc.rc, code, tc.want, out)
		}
		if strings.Contains(out, "not installed") {
			t.Fatalf("an installed host reported not installed:\n%s", out)
		}
	}
	// A path the host was not asked about never reaches a sudo command line.
	if out, code := gcpRun(t, e, "vm.sh", []string{"FAKE_INSTANCE_RC=0", "FAKE_CTL_PATH=/tmp/evil; rm -rf /"}, "status"); code == 0 || !strings.Contains(out, "unexpected styrman-ctl path") {
		t.Fatalf("an unexpected path was accepted: code %d\n%s", code, out)
	}
}

// "No public port 22" is a property of the PROJECT: omitting the `ssh` tag
// hides the VM from a tag-targeted rule, but an untagged rule (a default
// VPC's `default-allow-ssh`) reaches it anyway. create refuses rather than
// claim a posture the project does not have (security review 2026-09-22).
func TestGCPVMCreateRefusesPublicSSH(t *testing.T) {
	open22 := `[{"name":"default-allow-ssh","direction":"INGRESS","sourceRanges":["0.0.0.0/0"],"allowed":[{"IPProtocol":"tcp","ports":["22"]}]}]`
	// Must-fail states: each of these reaches this instance on port 22.
	for _, fw := range []string{
		open22,
		`[{"name":"wide-range","direction":"INGRESS","sourceRanges":["0.0.0.0/0"],"allowed":[{"IPProtocol":"tcp","ports":["20-30"]}]}]`,
		`[{"name":"all-ports","direction":"INGRESS","sourceRanges":["0.0.0.0/0"],"allowed":[{"IPProtocol":"tcp"}]}]`,
		`[{"name":"our-tag","direction":"INGRESS","sourceRanges":["0.0.0.0/0"],"targetTags":["http"],"allowed":[{"IPProtocol":"all"}]}]`,
	} {
		e := gcpSetup(t)
		out, code := gcpRun(t, e, "vm.sh", []string{"FAKE_FW=" + fw}, "--dry-run", "--yes", "create")
		if code != 1 || !strings.Contains(out, "opens ssh to the internet") {
			t.Fatalf("exposed project accepted: code %d\n%s\nfw=%s", code, out, fw)
		}
		if strings.Contains(out, "instances create") || strings.Contains(out, "addresses create") {
			t.Fatalf("a refused create still planned resources:\n%s", out)
		}
	}
	// Must-pass states: none of these reaches this instance on port 22.
	for _, fw := range []string{
		`[]`,
		`[{"name":"other-tag-ssh","direction":"INGRESS","sourceRanges":["0.0.0.0/0"],"targetTags":["bastion"],"allowed":[{"IPProtocol":"tcp","ports":["22"]}]}]`,
		`[{"name":"web","direction":"INGRESS","sourceRanges":["0.0.0.0/0"],"allowed":[{"IPProtocol":"tcp","ports":["80","443"]}]}]`,
		`[{"name":"internal","direction":"INGRESS","sourceRanges":["10.0.0.0/8"],"allowed":[{"IPProtocol":"all"}]}]`,
		`[{"name":"disabled","direction":"INGRESS","disabled":true,"sourceRanges":["0.0.0.0/0"],"allowed":[{"IPProtocol":"tcp","ports":["22"]}]}]`,
		`[{"name":"egress","direction":"EGRESS","sourceRanges":["0.0.0.0/0"],"allowed":[{"IPProtocol":"tcp","ports":["22"]}]}]`,
	} {
		e := gcpSetup(t)
		out, code := gcpRun(t, e, "vm.sh", []string{"FAKE_FW=" + fw}, "--dry-run", "--yes", "create")
		if code != 0 || !strings.Contains(out, "instances create") {
			t.Fatalf("a project that does not expose ssh was refused: code %d\n%s\nfw=%s", code, out, fw)
		}
	}
	// Shapes round 2 of the review named: a numeric protocol and an IPv6
	// any-source both reach 22; a rule scoped to service accounts cannot
	// reach an instance created with --no-service-account.
	for _, tc := range []struct {
		fw     string
		refuse bool
	}{
		{`[{"name":"num-proto","direction":"INGRESS","sourceRanges":["0.0.0.0/0"],"allowed":[{"IPProtocol":"6","ports":["22"]}]}]`, true},
		{`[{"name":"v6-any","direction":"INGRESS","sourceRanges":["::/0"],"allowed":[{"IPProtocol":"tcp","ports":["22"]}]}]`, true},
		{`[{"name":"sa-scoped","direction":"INGRESS","sourceRanges":["0.0.0.0/0"],"targetServiceAccounts":["svc@example.iam.gserviceaccount.com"],"allowed":[{"IPProtocol":"tcp","ports":["22"]}]}]`, false},
	} {
		e := gcpSetup(t)
		out, code := gcpRun(t, e, "vm.sh", []string{"FAKE_FW=" + tc.fw}, "--dry-run", "--yes", "create")
		if tc.refuse && (code != 1 || !strings.Contains(out, "opens ssh to the internet")) {
			t.Fatalf("shape not caught: code %d\n%s\nfw=%s", code, out, tc.fw)
		}
		if !tc.refuse && (code != 0 || !strings.Contains(out, "instances create")) {
			t.Fatalf("shape wrongly refused: code %d\n%s\nfw=%s", code, out, tc.fw)
		}
	}
	// The check never passes quietly when it could not run (BUG-034's family).
	for _, tc := range []struct {
		env  []string
		want string
	}{
		{[]string{"FAKE_FW={not json"}, "could not run"},
		{[]string{"FAKE_FW_RC=1"}, "cannot list this project's firewall rules"},
	} {
		e := gcpSetup(t)
		out, code := gcpRun(t, e, "vm.sh", tc.env, "--dry-run", "--yes", "create")
		if code != 1 || !strings.Contains(out, tc.want) {
			t.Fatalf("an unrunnable check did not refuse (%v): code %d\n%s", tc.env, code, out)
		}
		if strings.Contains(out, "instances create") {
			t.Fatalf("an unrunnable check still planned a create:\n%s", out)
		}
	}
	// The operator can accept the exposure, and is told the check was skipped.
	e := gcpSetup(t)
	out, code := gcpRun(t, e, "vm.sh", []string{"FAKE_FW=" + open22, "GCP_ALLOW_PUBLIC_SSH=1"}, "--dry-run", "--yes", "create")
	if code != 0 || !strings.Contains(out, "instances create") || !strings.Contains(out, "GCP_ALLOW_PUBLIC_SSH=1") {
		t.Fatalf("the override did not proceed with a warning: code %d\n%s", code, out)
	}
}

// --dry-run promises to run nothing; `ssh` opened a real session anyway.
func TestGCPVMSSHDryRunRunsNothing(t *testing.T) {
	e := gcpSetup(t)
	out, code := gcpRun(t, e, "vm.sh", nil, "--dry-run", "ssh", "--", "sudo styrman-ctl status")
	if code != 0 || !strings.Contains(out, "+ gcloud compute ssh") {
		t.Fatalf("dry-run ssh: code %d, want 0 and a planned command:\n%s", code, out)
	}
	if !strings.Contains(out, "--tunnel-through-iap") {
		t.Fatalf("the planned command does not tunnel through IAP:\n%s", out)
	}
	// The command is printed shell-quoted, so the line can be re-run as
	// printed; unquoting it must give back exactly what was asked for.
	_, cmdPart, found := strings.Cut(out, "--command=")
	if !found {
		t.Fatalf("no --command= in the planned line:\n%s", out)
	}
	if got := strings.ReplaceAll(strings.TrimSpace(cmdPart), `\ `, " "); got != "sudo styrman-ctl status" {
		t.Fatalf("the planned command is not the real one: %q\n%s", got, out)
	}
	if c := gcpCalls(t, e); strings.Contains(c, "compute ssh") {
		t.Fatalf("dry-run ssh actually connected:\n%s", c)
	}
}

// env_get and private_file live in deploy/gcp/lib.sh since 2026-09-22.
// deploy/gcp/install.sh used to carry its own forks: env_get's had lost the
// is_key guard and matched with `grep -E "^$2="`, interpolating the key into a
// regex, and private_file's had drifted from styrman-ctl's in two of five
// checks. These three tests hold the merge in place.

// styrman-ctl cannot source deploy/gcp/lib.sh — it is the general host tool
// and that lib is GCP-specific — so two copies of env_get exist on purpose.
// Two copies that already drifted once will drift again unless something
// fails when they do. Same shape as TestFirewallScriptMatchesBridgePrefixes.
func TestGCPLibEnvGetMatchesStyrmanCtl(t *testing.T) {
	root := repoRoot(t)
	read := func(rel string) string {
		b, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	ctl, lib := read("deploy/styrman-ctl"), read("deploy/gcp/lib.sh")

	// The literal-match awk program is the security-load-bearing line: the key
	// travels as a VARIABLE and is never interpolated into the program.
	const awkLine = `awk -v k="${key}" 'index($0, k "=") == 1 { v = substr($0, length(k) + 2) } END { if (v != "") print v }' "${file}"`
	for name, src := range map[string]string{"deploy/styrman-ctl": ctl, "deploy/gcp/lib.sh": lib} {
		if !strings.Contains(src, awkLine) {
			t.Errorf("%s no longer contains env_get's literal-match awk program; the two copies have drifted", name)
		}
		if !strings.Contains(src, `is_key() { [[ "$1" =~ ^[A-Z][A-Z0-9_]*$ ]]; }`) {
			t.Errorf("%s no longer contains the is_key guard", name)
		}
	}

	// private_file is the function whose drift motivated the merge, and the
	// two bodies are NOT textually identical -- lib.sh's is parameterised,
	// styrman-ctl's is not -- so they are compared by the checks they make.
	// Asserting only env_get here is what the comment in lib.sh and the
	// change entry both claimed to cover and did not (security review
	// 2026-09-22).
	for name, src := range map[string]string{"deploy/styrman-ctl": ctl, "deploy/gcp/lib.sh": lib} {
		for _, check := range []struct{ what, needle string }{
			{"symlink refusal", `[ -L "${f}" ] && die`},
			{"owner read", `owner="$(stat -c %U "${f}" 2>/dev/null || echo ?)"`},
			{"file mode read", `mode="$(stat -c %a "${f}" 2>/dev/null || echo 777)"`},
			{"file not group/other readable", `[ "$(( 8#${mode} & 8#077 ))" = 0 ]`},
			{"parent mode read", `pmode="$(stat -c %a "$(dirname -- "${f}")" 2>/dev/null || echo 777)"`},
		} {
			if !strings.Contains(src, check.needle) {
				t.Errorf("%s: private_file lost its %s; the two copies have drifted", name, check.what)
			}
		}
	}
	// The one check whose VALUE differs on purpose: styrman-ctl refuses a
	// parent intersecting 022 (not writable), because /etc/styrman is
	// legitimately 755; lib.sh defaults to 077 because ~/.config/styrman/gcp
	// exists only to hold secrets. If either changes, the reasoning in
	// lib.sh's comment and in changes/2026-09-22-review-remediation-phase-3.md
	// stops being true.
	if !strings.Contains(ctl, `[ "$(( 8#${pmode} & 8#022 ))" = 0 ]`) {
		t.Error("deploy/styrman-ctl's private_file no longer uses parent mask 022")
	}
	if !strings.Contains(lib, `parent_mask="${3:-077}"`) {
		t.Error("deploy/gcp/lib.sh's private_file no longer defaults to parent mask 077")
	}

	// And the fork must not come back.
	install := read("deploy/gcp/install.sh")
	for _, bad := range []string{"env_get() {", "private_file() {"} {
		if strings.Contains(install, bad) {
			t.Errorf("deploy/gcp/install.sh defines its own %s again; it must use lib.sh's", strings.TrimSuffix(bad, "() {"))
		}
	}
	if strings.Contains(install, `grep -E "^$2="`) {
		t.Error("deploy/gcp/install.sh interpolates a key into a grep regex again (the 2026-09-22 finding)")
	}
}

// The case the hardening was written for: a key containing a slash must be
// refused by name, never interpolated into the matcher. The fork would have
// passed it straight to `grep -E`.
func TestGCPLibEnvGetRefusesANonKey(t *testing.T) {
	e := gcpSetup(t)
	probe := filepath.Join(e.root, "probe.sh")
	body := `#!/usr/bin/env bash
set -euo pipefail
PROG=probe
. "` + filepath.Join(repoRoot(t), "deploy", "gcp", "lib.sh") + `"
env_get "$1" "$2"
`
	if err := os.WriteFile(probe, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	envFile := filepath.Join(e.root, "env")
	if err := os.WriteFile(envFile, []byte("BASE_DOMAIN=example.com\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, key := range []string{"a/b", "BASE_DOMAIN/x", "lower", "WITH-DASH", "$(id)", "*"} {
		out, err := exec.Command(probe, envFile, key).CombinedOutput()
		if err == nil {
			t.Errorf("env_get accepted %q as a key (output %q); is_key must refuse it", key, out)
		}
		if !strings.Contains(string(out), "not a key") {
			t.Errorf("env_get(%q) failed without naming the reason: %q", key, out)
		}
	}

	// A real key still works, so the guard is not simply refusing everything.
	out, err := exec.Command(probe, envFile, "BASE_DOMAIN").CombinedOutput()
	if err != nil {
		t.Fatalf("env_get on a valid key failed: %v (%q)", err, out)
	}
	if strings.TrimSpace(string(out)) != "example.com" {
		t.Errorf("env_get(BASE_DOMAIN) = %q, want example.com", out)
	}
}

// private_file's two parameters exist because styrman-ctl runs under sudo and
// these scripts do not, and because /etc/styrman is legitimately 755 while
// ~/.config/styrman/gcp exists only to hold secrets. Both defaults must stay
// what install.sh relied on before the merge: self, and 077.
func TestGCPLibPrivateFileDefaultsAndPolicies(t *testing.T) {
	e := gcpSetup(t)
	probe := filepath.Join(e.root, "pf.sh")
	body := `#!/usr/bin/env bash
set -euo pipefail
PROG=probe
. "` + filepath.Join(repoRoot(t), "deploy", "gcp", "lib.sh") + `"
private_file "$@"
echo OK
`
	if err := os.WriteFile(probe, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	mk := func(dirMode, fileMode os.FileMode) string {
		dir, err := os.MkdirTemp(e.root, "s")
		if err != nil {
			t.Fatal(err)
		}
		f := filepath.Join(dir, "secret")
		if err := os.WriteFile(f, []byte("x"), fileMode); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(f, fileMode); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(dir, dirMode); err != nil {
			t.Fatal(err)
		}
		return f
	}

	// Default: a 600 file in a 700 directory passes.
	if out, err := exec.Command(probe, mk(0o700, 0o600)).CombinedOutput(); err != nil {
		t.Errorf("private_file refused a 600 file in a 700 dir: %v (%q)", err, out)
	}
	// Default parent mask is 077, so a 755 parent is refused — what install.sh
	// did before the merge, and must keep doing.
	if out, err := exec.Command(probe, mk(0o755, 0o600)).CombinedOutput(); err == nil {
		t.Errorf("default private_file accepted a 755 parent; the default mask must stay 077 (%q)", out)
	}
	// With styrman-ctl's mask, the same 755 parent is fine: readable is not
	// writable, and /etc/styrman is legitimately 755.
	if out, err := exec.Command(probe, mk(0o755, 0o600), "self", "022").CombinedOutput(); err != nil {
		t.Errorf("private_file with mask 022 refused a 755 parent: %v (%q)", err, out)
	}
	// A group-writable parent is refused under either mask.
	if out, err := exec.Command(probe, mk(0o775, 0o600), "self", "022").CombinedOutput(); err == nil {
		t.Errorf("private_file accepted a group-writable parent under mask 022 (%q)", out)
	}
	// A group-readable FILE is refused regardless.
	if out, err := exec.Command(probe, mk(0o700, 0o640)).CombinedOutput(); err == nil {
		t.Errorf("private_file accepted a group-readable file (%q)", out)
	}
	// An unknown owner policy is an error, not a silent pass.
	if out, err := exec.Command(probe, mk(0o700, 0o600), "nonsense").CombinedOutput(); err == nil {
		t.Errorf("private_file accepted an unknown owner policy (%q)", out)
	}
	// A symlink is refused.
	real := mk(0o700, 0o600)
	link := filepath.Join(filepath.Dir(real), "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(probe, link).CombinedOutput(); err == nil {
		t.Errorf("private_file accepted a symlink (%q)", out)
	}
}
