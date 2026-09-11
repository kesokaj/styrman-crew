package config

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// deploy/styrman-ctl is exercised the way deploy/firewall/test.sh is: the
// real script, every path pointed at a scratch prefix (ADR-052), and the
// tools it shells out to replaced by fakes on PATH that print canned answers
// and record their argv. The `--dry-run` plan and the `__` test hooks are
// the seams; nothing here may touch /var/lib/styrman or /etc/styrman, and a
// printed path under either fails the test.

type ctlEnv struct {
	root, bin, calls string
	env              []string
}

func ctlSetup(t *testing.T, healthy bool) ctlEnv {
	t.Helper()
	root := t.TempDir()
	bin := filepath.Join(root, "fakebin")
	calls := filepath.Join(root, "calls.log")
	for _, d := range []string{"fakebin", "state/projects/u/p1", "state/projects/u/p2", "etc/update", "units", "deploy/deploy", "deploy/.git", "kata/bin"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	fake := func(name, body string) {
		script := "#!/usr/bin/env bash\necho \"" + name + " $*\" >> \"" + calls + "\"\n" + body + "\n"
		if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	health := "unhealthy"
	if healthy {
		health = "healthy"
	}
	fake("docker", `case "$1 $2" in
  "inspect -f")
    case "$3" in
      *Health*) echo "`+health+`" ;;
      *Config.Image*) [ "$4" = styrman-server ] && echo styrman-server:1.27.1 || echo styrman-caddy:1.27.1 ;;
      *State.Running*) echo true ;;
      *Id*) echo abc123 ;;
    esac ;;
  "ps -q") printf 'c1\nc2\nc3\n' ;;
  "version --format") echo 29.7.2 ;;
esac`)
	fake("systemctl", `case "$1" in is-enabled) echo enabled ;; is-active) [ "$2" = styrman-update.service ] && echo inactive || echo active ;; esac`)
	fake("iptables", `case "$2" in STYRMAN-FWD) printf -- '-N STYRMAN-FWD\n-A STYRMAN-FWD -j DROP\n-A STYRMAN-FWD -j DROP\n' ;; STYRMAN-IN) printf -- '-N STYRMAN-IN\n-A STYRMAN-IN -j DROP\n' ;; esac`)
	fake("git", `case "$*" in *rev-parse*) echo 4686792 ;; *) echo v1.27.1 ;; esac`)
	fake("du", `printf '41G\tx\n'`)
	fake("hostname", `echo testhost`)
	fake("kata-runtime", `echo "kata-runtime  : 3.31.0"`)
	if err := os.Symlink(filepath.Join(bin, "kata-runtime"), filepath.Join(root, "kata", "bin", "kata-runtime")); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"units/styrman-update.service", "units/styrman-firewall.service"} {
		if err := os.WriteFile(filepath.Join(root, f), []byte("[Unit]\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "etc/update/deployed-version"), []byte("1.27.1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "etc/update/deployed-sha"), []byte("4686792\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "deploy/deploy/.env"), []byte("BASE_DOMAIN=example.test\nSTYRMAN_HOSTNAME=crew\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	env := append(os.Environ(),
		"STYRMAN_CTL_TEST=1",
		"PATH="+bin+":"+os.Getenv("PATH"),
		"STYRMAN_STATE_DIR="+filepath.Join(root, "state"),
		"STYRMAN_DEPLOY_DIR="+filepath.Join(root, "deploy"),
		"STYRMAN_ETC_DIR="+filepath.Join(root, "etc"),
		"STYRMAN_UNIT_DIR="+filepath.Join(root, "units"),
		"STYRMAN_BIN_DIR="+filepath.Join(root, "bin"),
		"STYRMAN_SBIN_DIR="+filepath.Join(root, "sbin"),
		"STYRMAN_TMPFILES_DIR="+filepath.Join(root, "tmpfiles"),
		"STYRMAN_KATA_DIR="+filepath.Join(root, "kata"),
	)
	return ctlEnv{root: root, bin: bin, calls: calls, env: env}
}

func ctl(t *testing.T, e ctlEnv, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(filepath.Join(repoRoot(t), "deploy", "styrman-ctl"), args...)
	cmd.Env = e.env
	out, err := cmd.CombinedOutput()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("%v: %v\n%s", args, err, out)
	}
	for _, bad := range []string{"/var/lib/styrman", "/etc/styrman", "/opt/styrman"} {
		if strings.Contains(string(out), bad) {
			t.Fatalf("%v printed a production path %q — the scratch prefix leaked:\n%s", args, bad, out)
		}
	}
	return string(out), code
}

func TestStyrmanCtlHelpAndUnknownVerb(t *testing.T) {
	e := ctlSetup(t, true)
	out, code := ctl(t, e, "help")
	if code != 0 || !strings.Contains(out, "status [--json]") {
		t.Fatalf("help: %d %s", code, out)
	}
	if out, code := ctl(t, e, "frobnicate"); code == 0 || !strings.Contains(out, "unknown verb") {
		t.Fatalf("unknown verb: %d %s", code, out)
	}
	if out, code := ctl(t, e, "uninstall", "--dry-run"); code == 0 || !strings.Contains(out, "not implemented") {
		t.Fatalf("uninstall stub: %d %s", code, out)
	}
}

func TestStyrmanCtlStatusJSON(t *testing.T) {
	e := ctlSetup(t, true)
	out, code := ctl(t, e, "status", "--json", "--dry-run")
	if code != 0 {
		t.Fatalf("status --json: %d %s", code, out)
	}
	var st struct {
		Host      string `json:"host"`
		Installed bool   `json:"installed"`
		Server    struct {
			Health string `json:"health"`
			Image  string `json:"image"`
			URL    string `json:"url"`
		} `json:"server"`
		Deployed struct{ Version, Sha, Failed string } `json:"deployed"`
		Updater  struct {
			CloneTag string `json:"clone_tag"`
		} `json:"updater"`
		Firewall struct {
			Service  string `json:"service"`
			FwdRules string `json:"fwd_rules"`
		} `json:"firewall"`
		Projects struct {
			Running int    `json:"running"`
			Dirs    int    `json:"dirs"`
			Disk    string `json:"disk"`
		} `json:"projects"`
		Exit int `json:"exit"`
	}
	if err := json.Unmarshal([]byte(out), &st); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	if st.Host != "testhost" || !st.Installed || st.Server.Health != "healthy" || st.Server.Image != "styrman-server:1.27.1" ||
		st.Server.URL != "https://crew.example.test" || st.Deployed.Version != "1.27.1" || st.Deployed.Sha != "4686792" ||
		st.Deployed.Failed != "" || st.Updater.CloneTag != "v1.27.1" || st.Firewall.Service != "enabled/active" ||
		st.Firewall.FwdRules != "2" || st.Projects.Running != 3 || st.Projects.Dirs != 2 || st.Projects.Disk != "41G" || st.Exit != 0 {
		t.Fatalf("status fields: %+v\n%s", st, out)
	}
	e2 := ctlSetup(t, false)
	if out, code := ctl(t, e2, "status", "--json", "--dry-run"); code != 1 || !strings.Contains(out, `"exit":1`) {
		t.Fatalf("degraded status: %d %s", code, out)
	}
	if err := os.WriteFile(filepath.Join(e.root, "etc/update/failed-version"), []byte("1.28.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, code := ctl(t, e, "status", "--dry-run"); code != 1 || !strings.Contains(out, "FAILED-VERSION LATCHED: 1.28.0") || !strings.Contains(out, "DEGRADED") {
		t.Fatalf("latched status: %d %s", code, out)
	}
}

func TestStyrmanCtlStatusNothingInstalled(t *testing.T) {
	e := ctlSetup(t, true)
	for _, f := range []string{"units/styrman-update.service", "units/styrman-firewall.service"} {
		if err := os.Remove(filepath.Join(e.root, f)); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(e.bin, "docker"), []byte("#!/usr/bin/env bash\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if out, code := ctl(t, e, "status", "--dry-run"); code != 2 || !strings.Contains(out, "nothing installed") {
		t.Fatalf("nothing installed: %d %s", code, out)
	}
}

func TestStyrmanCtlValidate(t *testing.T) {
	e := ctlSetup(t, true)
	good := [][2]string{
		{"BASE_DOMAIN", "styrman.xyz"}, {"STYRMAN_HOSTNAME", "crew"}, {"ADMIN_EMAIL", "a@b.co"},
		{"STYRMAN_ALLOWED_EMAILS", "a@b.co, c@d.org"}, {"SUPERADMINS", "${STYRMAN_ALLOWED_EMAILS}"},
		{"STYRMAN_IDLE_TIMEOUT", "8h"}, {"STYRMAN_TRANSCRIPT_TTL", "0"}, {"STYRMAN_TRANSCRIPT_TTL", "720h"},
		{"STYRMAN_BROKER_ADDR", ":8091"}, {"STYRMAN_BROKER_ADDR", ""}, {"DEFAULT_RPS", ""}, {"DEFAULT_RPS", "20"},
		{"CADDY_ADMIN", ""}, {"CADDY_ADMIN", "127.0.0.1:2019"}, {"CADDY_ADMIN", "[::1]:2019"}, {"STYRMAN_IMAGE", ""},
		{"GOOGLE_CLIENT_ID", "x.apps.googleusercontent.com"}, {"STYRMAN_PUBLIC_DOMAIN", ""}, {"AUTH_DOMAIN", "auth"},
	}
	for _, g := range good {
		if out, code := ctl(t, e, "__validate", g[0], g[1]); code != 0 {
			t.Errorf("valid %s=%q refused: %s", g[0], g[1], out)
		}
	}
	bad := [][2]string{
		{"BASE_DOMAIN", "not a host"}, {"BASE_DOMAIN", ""}, {"STYRMAN_HOSTNAME", "a.b"}, {"ADMIN_EMAIL", "nope"},
		{"STYRMAN_ALLOWED_EMAILS", ""}, {"STYRMAN_ALLOWED_EMAILS", "a@b.co,,"}, {"STYRMAN_IDLE_TIMEOUT", "8 hours"},
		{"STYRMAN_BROKER_ADDR", "8091"}, {"DEFAULT_RPS", "ten"}, {"CADDY_ADMIN", "localhost:2019"},
		{"CADDY_ADMIN", "0.0.0.0:2019"}, {"CADDY_ADMIN", "127.0.0.1:70000"}, {"CADDY_ADMIN", "127.0.0.999:2019"}, {"STYRMAN_IMAGE", "styrman-runtime:dev"}, {"GOOGLE_CLIENT_SECRET", "has space"},
		{"STYRMAN_UID", "1000"}, {"STYRMAN_JWT_KEY", "abc"}, {"NOT_A_KEY", "x"},
	}
	for _, b := range bad {
		if out, code := ctl(t, e, "__validate", b[0], b[1]); code == 0 {
			t.Errorf("invalid %s=%q accepted: %s", b[0], b[1], out)
		}
	}
}

func TestStyrmanCtlEnvSet(t *testing.T) {
	e := ctlSetup(t, true)
	f := filepath.Join(e.root, "test.env")
	if err := os.WriteFile(f, []byte("# comment\nA=1\nB=old\nB=older\nC=3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, code := ctl(t, e, "__env-set", f, "B", "new"); code != 0 {
		t.Fatalf("env-set: %s", out)
	}
	if out, code := ctl(t, e, "__env-set", f, "D", "4"); code != 0 {
		t.Fatalf("env-set append: %s", out)
	}
	got, _ := os.ReadFile(f)
	if string(got) != "# comment\nA=1\nB=new\nC=3\nD=4\n" {
		t.Fatalf("env after set:\n%s", got)
	}
	if st, _ := os.Stat(f); st.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o, want 600", st.Mode().Perm())
	}
	if out, code := ctl(t, e, "__env-get", f, "B"); code != 0 || strings.TrimSpace(out) != "new" {
		t.Fatalf("env-get: %d %q", code, out)
	}
	link := filepath.Join(e.root, "link.env")
	if err := os.Symlink(f, link); err != nil {
		t.Fatal(err)
	}
	if out, code := ctl(t, e, "__env-set", link, "A", "2"); code == 0 || !strings.Contains(out, "symlink") {
		t.Fatalf("symlink accepted: %d %s", code, out)
	}
	if out, code := ctl(t, e, "__env-set", f, "A", "x\nEVIL=1"); code == 0 || !strings.Contains(out, "newline") {
		t.Fatalf("newline accepted: %d %s", code, out)
	}
	// A LITERAL backslash-n: awk -v would have decoded it into a real newline
	// and smuggled a second assignment (security review, high).
	if out, code := ctl(t, e, "__env-set", f, "B", `x\nSUPERADMINS=evil@x.y`); code == 0 || !strings.Contains(out, "backslash") {
		t.Fatalf("backslash accepted: %d %s", code, out)
	}
	if got, _ := os.ReadFile(f); strings.Contains(string(got), "evil") || strings.Count(string(got), "SUPERADMINS") != 0 {
		t.Fatalf("a second assignment was smuggled in:\n%s", got)
	}
	// env_get takes the key as a literal, never as a program.
	if out, code := ctl(t, e, "__env-get", f, "A/,s/x/y/"); code == 0 || !strings.Contains(out, "not a key") {
		t.Fatalf("hostile key accepted by env_get: %d %s", code, out)
	}
	// The hooks are gated on the harness variable.
	e2 := ctlSetup(t, true)
	var without []string
	for _, kv := range e2.env {
		if !strings.HasPrefix(kv, "STYRMAN_CTL_TEST=") {
			without = append(without, kv)
		}
	}
	e2.env = without
	if out, code := ctl(t, e2, "__env-set", f, "A", "2"); code == 0 || !strings.Contains(out, "disabled") {
		t.Fatalf("hooks usable without the harness variable: %d %s", code, out)
	}
	if _, code := ctl(t, e, "__env-set", f, "lower", "x"); code == 0 {
		t.Fatal("non-key accepted")
	}
	out, code := ctl(t, e, "__env-set", f, "A", "9", "--dry-run")
	got, _ = os.ReadFile(f)
	if code != 0 || !strings.Contains(out, "+ env_set "+f+" A") || !strings.Contains(string(got), "A=1\n") {
		t.Fatalf("dry-run env-set: %d %s\n%s", code, out, got)
	}
	// And no temp file was left or even created beside it.
	if entries, _ := os.ReadDir(e.root); func() bool {
		for _, en := range entries {
			if strings.HasPrefix(en.Name(), ".env.") {
				return true
			}
		}
		return false
	}() {
		t.Fatal("dry-run env-set left a temp file")
	}
}

func TestStyrmanCtlGuardPath(t *testing.T) {
	e := ctlSetup(t, true)
	state := filepath.Join(e.root, "state")
	if out, code := ctl(t, e, "__guard-path", state); code != 0 {
		t.Fatalf("state dir refused: %s", out)
	}
	for _, bad := range []string{"/", "/tmp", filepath.Join(e.root, "state", ".."), filepath.Join(e.root, "elsewhere"), filepath.Join(e.root, "state/projects"), filepath.Join(e.root, "state") + "/", filepath.Join(e.root, "nonexistent")} {
		if out, code := ctl(t, e, "__guard-path", bad); code == 0 {
			t.Errorf("%s accepted: %s", bad, out)
		}
	}
	link := filepath.Join(e.root, "state-link")
	if err := os.Symlink(state, link); err != nil {
		t.Fatal(err)
	}
	e.env = append(e.env, "STYRMAN_STATE_DIR="+link)
	if out, code := ctl(t, e, "__guard-path", link); code == 0 || !strings.Contains(out, "symlink") {
		t.Fatalf("symlinked state accepted: %d %s", code, out)
	}
}

// ── Phase 2: install and update --setup, as dry-run plans ──────────────────

func planLines(out string) []string {
	var lines []string
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, "+ ") {
			lines = append(lines, l)
		}
	}
	return lines
}

func indexOf(lines []string, sub string) int {
	for i, l := range lines {
		if strings.Contains(l, sub) {
			return i
		}
	}
	return -1
}

func TestStyrmanCtlInstallPlan(t *testing.T) {
	e := ctlSetup(t, true)
	envFile := filepath.Join(e.root, "checkout.env")
	if err := os.WriteFile(envFile, []byte("BASE_DOMAIN=\nGOOGLE_CLIENT_ID=\nGOOGLE_CLIENT_SECRET=\nCLOUDFLARE_API_TOKEN=\nADMIN_EMAIL=\nSTYRMAN_ALLOWED_EMAILS=\nSTYRMAN_HOSTNAME=crew\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	answers := filepath.Join(e.root, "answers.env")
	if err := os.WriteFile(answers, []byte("BASE_DOMAIN=example.test\nGOOGLE_CLIENT_ID=id.apps\nGOOGLE_CLIENT_SECRET=s3cret\nCLOUDFLARE_API_TOKEN=cf\nADMIN_EMAIL=a@example.test\nSTYRMAN_ALLOWED_EMAILS=a@example.test,b@example.test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	e.env = append(e.env, "STYRMAN_ENV_FILE="+envFile, "SUDO_USER=tester")
	out, code := ctl(t, e, "install", "--dry-run", "--yes", "--version", "1.2.3", "--env-file", answers, "--skip-auto-update")
	if code != 0 {
		t.Fatalf("install --dry-run: %d\n%s", code, out)
	}
	lines := planLines(out)
	hs := indexOf(lines, "host-setup.sh --user tester")
	build := indexOf(lines, "docker build -t styrman-runtime:1.2.3 --build-arg STYRMAN_VERSION=1.2.3")
	up := indexOf(lines, "env STYRMAN_VERSION=1.2.3 docker compose -f")
	wait := indexOf(lines, "wait up to 120s")
	if hs < 0 || build < 0 || up < 0 || wait < 0 || !(hs < build && build < up && up < wait) {
		t.Fatalf("plan order wrong (host-setup %d, build %d, up %d, wait %d):\n%s", hs, build, up, wait, out)
	}
	if !strings.Contains(lines[up], "--env-file "+envFile) {
		t.Fatalf("compose must be pinned to the env file: %s", lines[up])
	}
	// The six must-fill keys were validated and written (dry-run: the moves are planned, the file untouched).
	if n := strings.Count(out, "+ env_set "); n != 6 {
		t.Fatalf("expected 6 planned .env writes, got %d:\n%s", n, out)
	}
	if got, _ := os.ReadFile(envFile); !strings.Contains(string(got), "BASE_DOMAIN=\n") {
		t.Fatalf("dry run wrote the env file:\n%s", got)
	}
	// Nothing ran: the fakes only saw status-style read calls, never host-setup or docker build.
	calls, _ := os.ReadFile(e.calls)
	if strings.Contains(string(calls), "docker build") || strings.Contains(string(calls), "compose") {
		t.Fatalf("dry run executed something:\n%s", calls)
	}
}

func TestStyrmanCtlInstallRefusals(t *testing.T) {
	e := ctlSetup(t, true)
	envFile := filepath.Join(e.root, "checkout.env")
	if err := os.WriteFile(envFile, []byte("BASE_DOMAIN=\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	base := append([]string{}, e.env...)
	// Unattended without a value for a must-fill key: dies naming the key.
	e.env = append(base, "STYRMAN_ENV_FILE="+envFile, "SUDO_USER=tester")
	if out, code := ctl(t, e, "install", "--dry-run", "--yes", "--version", "1.2.3", "--skip-auto-update"); code == 0 || !strings.Contains(out, "BASE_DOMAIN") {
		t.Fatalf("missing key accepted: %d %s", code, out)
	}
	// A bad version and a missing version (the fake git describes nothing usable).
	if out, code := ctl(t, e, "install", "--dry-run", "--yes", "--version", "v1", "--skip-auto-update"); code == 0 || !strings.Contains(out, "no release version") {
		t.Fatalf("bad version accepted: %d %s", code, out)
	}
	// No operator at all.
	e.env = append(base, "STYRMAN_ENV_FILE="+envFile, "SUDO_USER=")
	if out, code := ctl(t, e, "install", "--dry-run", "--yes", "--version", "1.2.3", "--skip-auto-update"); code == 0 || !strings.Contains(out, "--user") {
		t.Fatalf("no operator accepted: %d %s", code, out)
	}
	// From inside the updater's clone: refused. The checkout IS the repo, so
	// point the deploy dir at an ancestor of it.
	e.env = append(base, "STYRMAN_ENV_FILE="+envFile, "SUDO_USER=tester", "STYRMAN_DEPLOY_DIR="+filepath.Dir(repoRoot(t)))
	if out, code := ctl(t, e, "install", "--dry-run", "--yes", "--version", "1.2.3", "--skip-auto-update"); code == 0 || !strings.Contains(out, "auto-updater's clone") {
		t.Fatalf("install from the clone accepted: %d %s", code, out)
	}
}

func TestStyrmanCtlUpdateSetupPlan(t *testing.T) {
	e := ctlSetup(t, true)
	// A fake git that reports an ssh origin and records everything (askpass env included).
	gitFake := "#!/usr/bin/env bash\necho \"git $* GIT_ASKPASS=${GIT_ASKPASS:-} GIT_TERMINAL_PROMPT=${GIT_TERMINAL_PROMPT:-}\" >> \"" + e.calls + "\"\ncase \"$*\" in *get-url*) echo git@github.com:owner/repo.git ;; *rev-parse*) echo 4686792 ;; *) echo v1.27.1 ;; esac\n"
	if err := os.WriteFile(filepath.Join(e.bin, "git"), []byte(gitFake), 0o755); err != nil {
		t.Fatal(err)
	}
	tok := filepath.Join(e.root, "tok")
	if err := os.WriteFile(tok, []byte("ghp_SECRETTOKEN\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	signers := filepath.Join(e.root, "signers")
	if err := os.WriteFile(signers, []byte("me@example.test ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIExample\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	envFile := filepath.Join(e.root, "checkout.env")
	if err := os.WriteFile(envFile, []byte("BASE_DOMAIN=example.test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Fresh setup: no clone yet.
	if err := os.RemoveAll(filepath.Join(e.root, "deploy", ".git")); err != nil {
		t.Fatal(err)
	}
	e.env = append(e.env, "STYRMAN_ENV_FILE="+envFile)
	out, code := ctl(t, e, "update", "--setup", "--dry-run", "--yes", "--gh-token-file", tok, "--allowed-signers-file", signers)
	if code != 0 {
		t.Fatalf("update --setup --dry-run: %d\n%s", code, out)
	}
	if strings.Contains(out, "SECRETTOKEN") {
		t.Fatalf("the token leaked into the plan:\n%s", out)
	}
	lines := planLines(out)
	want := []string{
		"install -d -m 700 -o root -g root " + filepath.Join(e.root, "etc"),
		"install -m 600 -o root -g root", // gh-token from the temp file
		"git clone https://github.com/owner/repo.git " + filepath.Join(e.root, "deploy"),
		"chown -R root:root " + filepath.Join(e.root, "deploy"),
		"install -m 600 -o root -g root " + envFile + " " + filepath.Join(e.root, "deploy", "deploy", ".env"),
		"install -m 644 -o root -g root", // allowed_signers from the temp file
		"STYRMAN_UPDATE_STATE_DIR=" + filepath.Join(e.root, "etc", "update"),
		"systemctl enable --now styrman-update.timer",
		"systemctl start styrman-update.service",
	}
	last := -1
	for _, w := range want {
		i := indexOf(lines, w)
		if i < 0 {
			t.Fatalf("plan lacks %q:\n%s", w, out)
		}
		if i < last {
			t.Fatalf("plan out of order at %q:\n%s", w, out)
		}
		last = i
	}
	// The updater's every input is named — token and signers files included
	// (review finding: they default to /etc/styrman inside the updater).
	check := lines[indexOf(lines, "styrman-update --check")]
	for _, w := range []string{"STYRMAN_GH_TOKEN_FILE=" + filepath.Join(e.root, "etc", "gh-token"), "STYRMAN_ALLOWED_SIGNERS=" + filepath.Join(e.root, "etc", "allowed_signers"), "STYRMAN_DEPLOY_DIR=" + filepath.Join(e.root, "deploy")} {
		if !strings.Contains(check, w) {
			t.Fatalf("--check lacks %q: %s", w, check)
		}
	}
	// Dry run wrote no temp files with the secrets (the plan names <tmpdir>).
	if !strings.Contains(out, "<tmpdir>/gh-token") {
		t.Fatalf("dry run should plan with a placeholder temp dir:\n%s", out)
	}
	clone := lines[indexOf(lines, "git clone")]
	if !strings.Contains(clone, "GIT_ASKPASS=") || !strings.Contains(clone, "GIT_TERMINAL_PROMPT=0") {
		t.Fatalf("clone runs without the one-shot askpass: %s", clone)
	}
	if strings.Contains(out, "credential.helper") {
		t.Fatalf("a credential helper reached git config:\n%s", out)
	}
	// An existing clone is fetched, not re-cloned.
	if err := os.MkdirAll(filepath.Join(e.root, "deploy", ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	out, _ = ctl(t, e, "update", "--setup", "--dry-run", "--yes", "--gh-token-file", tok, "--allowed-signers-file", signers)
	if !strings.Contains(out, "fetch --tags") || strings.Contains(out, "git clone") {
		t.Fatalf("existing clone not fetched:\n%s", out)
	}
	// A malformed signers line is refused before anything is planned.
	if err := os.WriteFile(signers, []byte("not a signers line\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, code := ctl(t, e, "update", "--setup", "--dry-run", "--yes", "--gh-token-file", tok, "--allowed-signers-file", signers); code == 0 || !strings.Contains(out, "allowed_signers line") || len(planLines(out)) != 0 {
		t.Fatalf("bad signers accepted: %d %s", code, out)
	}
}

// ── Phase 3: edit and update, as dry-run plans ─────────────────────────────

func TestStyrmanCtlEditPlan(t *testing.T) {
	e := ctlSetup(t, true)
	envFile := filepath.Join(e.root, "checkout.env")
	if err := os.WriteFile(envFile, []byte("BASE_DOMAIN=example.test\nSTYRMAN_IDLE_TIMEOUT=8h\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	e.env = append(e.env, "STYRMAN_ENV_FILE="+envFile)
	cloneEnv := filepath.Join(e.root, "deploy", "deploy", ".env")
	out, code := ctl(t, e, "edit", "--dry-run", "STYRMAN_IDLE_TIMEOUT=9h", "DEFAULT_RPS=20")
	if code != 0 {
		t.Fatalf("edit --dry-run: %d\n%s", code, out)
	}
	lines := planLines(out)
	// Every key into BOTH copies, then one recreate from the clone at the deployed version, then the wait.
	for _, w := range []string{
		"+ env_set " + envFile + " STYRMAN_IDLE_TIMEOUT", "+ env_set " + cloneEnv + " STYRMAN_IDLE_TIMEOUT",
		"+ env_set " + envFile + " DEFAULT_RPS", "+ env_set " + cloneEnv + " DEFAULT_RPS",
	} {
		if indexOf(lines, w) < 0 {
			t.Fatalf("plan lacks %q:\n%s", w, out)
		}
	}
	up := indexOf(lines, "env STYRMAN_VERSION=1.27.1 docker compose -f "+filepath.Join(e.root, "deploy", "deploy", "docker-compose.yml")+" --env-file "+cloneEnv+" up -d")
	if up < 0 || strings.Contains(lines[up], "--build") || indexOf(lines, "wait up to 120s") < up {
		t.Fatalf("recreate line wrong or misplaced:\n%s", out)
	}
	if got, _ := os.ReadFile(envFile); !strings.Contains(string(got), "STYRMAN_IDLE_TIMEOUT=8h") {
		t.Fatalf("dry run wrote the env file:\n%s", got)
	}
	// --no-apply writes but never recreates.
	out, _ = ctl(t, e, "edit", "--dry-run", "--no-apply", "STYRMAN_IDLE_TIMEOUT=9h")
	if indexOf(planLines(out), "docker compose") >= 0 || !strings.Contains(out, "not applied") {
		t.Fatalf("--no-apply recreated:\n%s", out)
	}
	// Refusals happen before ANY write: a computed key, an unknown key, an invalid value.
	for _, bad := range []string{"STYRMAN_UID=1001", "NOT_A_KEY=1", "STYRMAN_IDLE_TIMEOUT=nine hours", "CADDY_ADMIN=localhost:2019"} {
		out, code := ctl(t, e, "edit", "--dry-run", "STYRMAN_HOSTNAME=crew", bad)
		if code == 0 || len(planLines(out)) != 0 {
			t.Fatalf("%s: code %d, plan %v", bad, code, planLines(out))
		}
	}
	// No terminal and no arguments: told to pass KEY=VALUE.
	if out, code := ctl(t, e, "edit", "--dry-run"); code == 0 || !strings.Contains(out, "KEY=VALUE") {
		t.Fatalf("edit without args: %d %s", code, out)
	}
	// Neither .env exists: run install first.
	e2 := ctlSetup(t, true)
	if err := os.Remove(filepath.Join(e2.root, "deploy", "deploy", ".env")); err != nil {
		t.Fatal(err)
	}
	e2.env = append(e2.env, "STYRMAN_ENV_FILE="+filepath.Join(e2.root, "missing.env"))
	if out, code := ctl(t, e2, "edit", "--dry-run", "STYRMAN_HOSTNAME=crew"); code == 0 || !strings.Contains(out, "run install first") {
		t.Fatalf("edit without any .env: %d %s", code, out)
	}
}

func TestStyrmanCtlUpdatePlan(t *testing.T) {
	e := ctlSetup(t, true)
	out, code := ctl(t, e, "update", "--dry-run")
	if code != 0 {
		t.Fatalf("update --dry-run: %d\n%s", code, out)
	}
	lines := planLines(out)
	start := indexOf(lines, "systemctl start styrman-update.service")
	journal := indexOf(lines, "journalctl -u styrman-update.service")
	if start < 0 || journal < start {
		t.Fatalf("update plan:\n%s", out)
	}
	out, _ = ctl(t, e, "update", "--redeploy", "--dry-run")
	force := indexOf(planLines(out), "STYRMAN_UPDATE_STATE_DIR="+filepath.Join(e.root, "etc", "update"))
	if force < 0 || !strings.Contains(planLines(out)[force], "styrman-update --force") || !strings.Contains(planLines(out)[force], "STYRMAN_GH_TOKEN_FILE=") || strings.Contains(out, "systemctl start styrman-update.service") {
		t.Fatalf("redeploy plan:\n%s", out)
	}
	// Timer not enabled → the --setup hint (only enforced outside dry-run, so
	// exercise it through the fake with dry-run off: need_root would refuse
	// first, which is the same "nothing runs" outcome we want to see).
	if err := os.WriteFile(filepath.Join(e.bin, "systemctl"), []byte("#!/usr/bin/env bash\ncase \"$1\" in is-enabled) echo disabled ;; is-active) echo inactive ;; esac\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if out, code := ctl(t, e, "update"); code == 0 || !strings.Contains(out, "run as root") {
		t.Fatalf("update as non-root: %d %s", code, out)
	}
}
