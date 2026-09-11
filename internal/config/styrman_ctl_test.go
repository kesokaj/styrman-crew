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
		{"CADDY_ADMIN", "0.0.0.0:2019"}, {"STYRMAN_IMAGE", "styrman-runtime:dev"}, {"GOOGLE_CLIENT_SECRET", "has space"},
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
	if _, code := ctl(t, e, "__env-set", f, "lower", "x"); code == 0 {
		t.Fatal("non-key accepted")
	}
	out, code := ctl(t, e, "__env-set", f, "A", "9", "--dry-run")
	got, _ = os.ReadFile(f)
	if code != 0 || !strings.Contains(out, "+ mv -f") || !strings.Contains(string(got), "A=1\n") {
		t.Fatalf("dry-run env-set: %d %s\n%s", code, out, got)
	}
}

func TestStyrmanCtlGuardPath(t *testing.T) {
	e := ctlSetup(t, true)
	state := filepath.Join(e.root, "state")
	if out, code := ctl(t, e, "__guard-path", state); code != 0 {
		t.Fatalf("state dir refused: %s", out)
	}
	for _, bad := range []string{"/", "/tmp", filepath.Join(e.root, "state", ".."), filepath.Join(e.root, "elsewhere"), filepath.Join(e.root, "state/projects")} {
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
