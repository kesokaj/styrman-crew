package config

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// ADR-081: every real tick of deploy/update/styrman-update ends by recording
// its result in one small file the server shows to admins (BUG-084: the
// updater failed for ten days on a rejected token and nothing said so).
//
// These drive the SAME write_status and on_exit a real tick runs, through the
// gated hooks __write-status and __exit-status, as a non-root user whose own
// uid is the trust bar. What they do not reach is the tick itself — where the
// status is armed and which site sets which code; TestStyrmanUpdateStatusWiring
// pins that as text and says so.

type statusFile struct {
	Result    string `json:"result"`
	Code      string `json:"code"`
	CheckedAt string `json:"checkedAt"`
	Since     string `json:"since"`
	Deployed  string `json:"deployed"`
	Latest    string `json:"latest"`
}

func updaterScript(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("no bash on this host")
	}
	p, err := filepath.Abs("../../deploy/update/styrman-update")
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// runUpdaterHook runs one hook; gated chooses whether STYRMAN_UPDATE_TEST=1 is set.
func runUpdaterHook(t *testing.T, gated bool, args ...string) (int, string) {
	t.Helper()
	cmd := exec.Command("bash", append([]string{updaterScript(t)}, args...)...)
	cmd.Env = append(os.Environ(), "STYRMAN_UPDATE_STATUS_FILE=")
	if gated {
		cmd.Env = append(cmd.Env, "STYRMAN_UPDATE_TEST=1")
	}
	out, err := cmd.CombinedOutput()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("running the hook: %v", err)
	}
	return code, string(out)
}

func readStatus(t *testing.T, path string) statusFile {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("no status file: %v", err)
	}
	if n := strings.Count(string(raw), "\n"); n != 1 || !strings.HasSuffix(string(raw), "\n") {
		t.Fatalf("the status must be one line, got %q", raw)
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(raw, &keys); err != nil {
		t.Fatalf("not JSON: %v: %q", err, raw)
	}
	if len(keys) != 6 {
		t.Fatalf("want exactly six keys, got %d: %q", len(keys), raw)
	}
	var st statusFile
	if err := json.Unmarshal(raw, &st); err != nil {
		t.Fatal(err)
	}
	return st
}

// statusDir returns a directory that passes the updater's bar (the caller's,
// 0755) and the status path inside it.
func statusDir(t *testing.T) (dir, file string) {
	t.Helper()
	dir = filepath.Join(t.TempDir(), "status")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir, filepath.Join(dir, "updater.json")
}

func TestStyrmanUpdateStatusHooksAreGated(t *testing.T) {
	_, file := statusDir(t)
	for _, args := range [][]string{
		{"__write-status", file, "ok", "ok", "v1.2.3", "v1.2.3"},
		{"__exit-status", file, "0"},
	} {
		code, out := runUpdaterHook(t, false, args...)
		if code == 0 || !strings.Contains(out, "internal test hooks are disabled") {
			t.Errorf("%s without STYRMAN_UPDATE_TEST=1: exit %d, %q", args[0], code, out)
		}
		if _, err := os.Stat(file); err == nil {
			t.Fatalf("%s wrote a status without the gate", args[0])
		}
	}
}

func TestStyrmanUpdateStatusWriteWritesTheSixValidatedFields(t *testing.T) {
	dir, file := statusDir(t)
	if code, out := runUpdaterHook(t, true, "__write-status", file, "failing", "fetch-auth", "v1.38.0", ""); code != 0 {
		t.Fatalf("exit %d: %s", code, out)
	}
	st := readStatus(t, file)
	if st.Result != "failing" || st.Code != "fetch-auth" || st.Deployed != "v1.38.0" || st.Latest != "" {
		t.Errorf("status = %+v", st)
	}
	if st.CheckedAt == "" || st.Since != st.CheckedAt {
		t.Errorf("the first failing write starts the streak at its own time: %+v", st)
	}
	if info, err := os.Stat(file); err != nil || info.Mode().Perm() != 0o644 {
		t.Errorf("the file must be 0644 so the server's uid can read it: %v %v", info, err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("a temp file was left behind: %v", entries)
	}
}

// Nothing outside the closed sets reaches the file: a code it does not know
// is "error", a tag that is not vX.Y.Z is empty — whatever the value was.
func TestStyrmanUpdateStatusWriteRefusesValuesOutsideItsSets(t *testing.T) {
	_, file := statusDir(t)
	runUpdaterHook(t, true, "__write-status", file, "failing", `x","injected":"y`, `v1.2.3"; rm -rf /`, "main")
	st := readStatus(t, file)
	if st.Result != "failing" || st.Code != "error" || st.Deployed != "" || st.Latest != "" {
		t.Errorf("status = %+v, want failing/error with both tags empty", st)
	}
	runUpdaterHook(t, true, "__write-status", file, "ok", "fetch", "v1.2.3", "v1.2.4")
	if st := readStatus(t, file); st.Result != "failing" || st.Code != "error" {
		t.Errorf("an ok with a failing code is not a result the writer knows: %+v", st)
	}
	runUpdaterHook(t, true, "__write-status", file, "maybe", "ok", "v1.2.3", "v1.2.4")
	if st := readStatus(t, file); st.Result != "failing" || st.Code != "error" || st.Deployed != "v1.2.3" || st.Latest != "v1.2.4" {
		t.Errorf("an unknown result: %+v", st)
	}
}

// `since` is the start of the failing streak: kept across failing ticks,
// cleared by an ok one, started afresh by the next failure.
func TestStyrmanUpdateStatusWriteCarriesSinceAcrossAFailingStreak(t *testing.T) {
	_, file := statusDir(t)
	planted := `{"result":"failing","code":"fetch","checkedAt":"2026-09-22T09:14:02Z","since":"2026-09-22T09:14:02Z","deployed":"v1.38.0","latest":""}` + "\n"
	if err := os.WriteFile(file, []byte(planted), 0o644); err != nil {
		t.Fatal(err)
	}
	runUpdaterHook(t, true, "__write-status", file, "failing", "fetch-auth", "v1.38.0", "")
	st := readStatus(t, file)
	if st.Since != "2026-09-22T09:14:02Z" || st.CheckedAt == "2026-09-22T09:14:02Z" {
		t.Fatalf("a second failing tick must keep the streak's start and move checkedAt: %+v", st)
	}
	runUpdaterHook(t, true, "__write-status", file, "ok", "ok", "v1.40.0", "v1.40.0")
	if st := readStatus(t, file); st.Result != "ok" || st.Since != "" {
		t.Fatalf("an ok tick clears since: %+v", st)
	}
	runUpdaterHook(t, true, "__write-status", file, "failing", "fetch", "v1.40.0", "")
	if st := readStatus(t, file); st.Since != st.CheckedAt {
		t.Fatalf("a failure after an ok tick starts a new streak: %+v", st)
	}
	// A `since` that is not a timestamp is not carried.
	bad := `{"result":"failing","code":"fetch","checkedAt":"x","since":"<script>","deployed":"","latest":""}` + "\n"
	if err := os.WriteFile(file, []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}
	runUpdaterHook(t, true, "__write-status", file, "failing", "fetch", "", "")
	if st := readStatus(t, file); st.Since != st.CheckedAt {
		t.Fatalf("a malformed since must not be carried: %+v", st)
	}
}

// The directory is created only under a parent that meets the bar, and a
// directory that does not meet it is refused — with nothing written and the
// tick's own exit status untouched (exit 0).
func TestStyrmanUpdateStatusWriteDirectoryTrust(t *testing.T) {
	// Missing, under a trusted parent: created 0755.
	parent := t.TempDir()
	if err := os.Chmod(parent, 0o755); err != nil { // t.TempDir is 0777 less the umask: 0775 under 002
		t.Fatal(err)
	}
	file := filepath.Join(parent, "status", "updater.json")
	if code, out := runUpdaterHook(t, true, "__write-status", file, "ok", "ok", "", ""); code != 0 {
		t.Fatalf("exit %d: %s", code, out)
	}
	if info, err := os.Stat(filepath.Dir(file)); err != nil || info.Mode().Perm() != 0o755 {
		t.Fatalf("the directory must be created 0755: %v %v", info, err)
	}
	readStatus(t, file)

	refused := func(name, file string) {
		t.Helper()
		code, out := runUpdaterHook(t, true, "__write-status", file, "ok", "ok", "", "")
		if code != 0 {
			t.Errorf("%s: a refused status must not fail the tick, exit %d", name, code)
		}
		if !strings.Contains(out, "WARN: status not written") {
			t.Errorf("%s: no WARN: %q", name, out)
		}
		if _, err := os.Lstat(file); err == nil {
			t.Errorf("%s: a status was written", name)
		}
	}

	// Missing, under a group-writable parent: not created.
	loose := filepath.Join(t.TempDir(), "loose")
	if err := os.Mkdir(loose, 0o775); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(loose, 0o775); err != nil { // past the umask
		t.Fatal(err)
	}
	refused("group-writable parent", filepath.Join(loose, "status", "updater.json"))
	if _, err := os.Lstat(filepath.Join(loose, "status")); err == nil {
		t.Error("the directory was created under a group-writable parent")
	}

	// A symlink where the directory should be.
	real := filepath.Join(t.TempDir(), "elsewhere")
	if err := os.Mkdir(real, 0o755); err != nil {
		t.Fatal(err)
	}
	linkParent := t.TempDir()
	if err := os.Symlink(real, filepath.Join(linkParent, "status")); err != nil {
		t.Fatal(err)
	}
	refused("symlinked directory", filepath.Join(linkParent, "status", "updater.json"))
	if entries, _ := os.ReadDir(real); len(entries) != 0 {
		t.Errorf("the write went through the symlink: %v", entries)
	}

	// A group-writable directory.
	gw := filepath.Join(t.TempDir(), "status")
	if err := os.Mkdir(gw, 0o775); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(gw, 0o775); err != nil {
		t.Fatal(err)
	}
	refused("group-writable directory", filepath.Join(gw, "updater.json"))

	// A file not named updater.json.
	_, good := statusDir(t)
	refused("another file name", filepath.Join(filepath.Dir(good), "other.json"))
}

// The exit decision: what a site said wins; otherwise the exit status.
func TestStyrmanUpdateStatusExitDecision(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantExit   int
		wantResult string
		wantCode   string
	}{
		{"exit 0 is ok", []string{"0"}, 0, "ok", "ok"},
		{"exit 1 is failing/error", []string{"1"}, 1, "failing", "error"},
		{"a latched exit is exit 0 and failing", []string{"0", "failing", "latched"}, 0, "failing", "latched"},
		{"a site's code on a failing exit", []string{"1", "failing", "fetch-auth"}, 1, "failing", "fetch-auth"},
	}
	for _, c := range cases {
		_, file := statusDir(t)
		code, out := runUpdaterHook(t, true, append([]string{"__exit-status", file}, c.args...)...)
		if code != c.wantExit {
			t.Errorf("%s: exit %d, want %d (%s)", c.name, code, c.wantExit, out)
		}
		if st := readStatus(t, file); st.Result != c.wantResult || st.Code != c.wantCode {
			t.Errorf("%s: status %+v, want %s/%s", c.name, st, c.wantResult, c.wantCode)
		}
	}
}

// TEXT pins for what the hooks cannot reach — the tick itself needs root.
// NOT evidence that a real tick writes its status; a guard against the three
// ways the wiring was got wrong on paper before it was written (plan
// 2026-10-02, cold read round 1).
func TestStyrmanUpdateStatusWiring(t *testing.T) {
	raw, err := os.ReadFile(updaterScript(t))
	if err != nil {
		t.Fatal(err)
	}
	src := string(raw)
	at := func(needle string) int {
		t.Helper()
		if n := strings.Count(src, needle); n != 1 {
			t.Fatalf("%q appears %d times, want exactly 1", needle, n)
		}
		return strings.Index(src, needle)
	}
	// ONE exit trap for the tick, installed above the DOCKER_CONFIG case —
	// whose hand-run arm, the script's only trap before, production never takes.
	trap := at("\ntrap on_exit EXIT\n")
	dockerCase := at(`EXIT_RM_DIR="${DOCKER_CONFIG}"`)
	if trap > dockerCase {
		t.Error("the exit handler must be installed before the DOCKER_CONFIG case, for every mode")
	}
	if strings.Contains(src, `trap 'rm -rf "${DOCKER_CONFIG}"' EXIT`) {
		t.Error("the hand-run arm still sets its own trap: it would replace on_exit")
	}
	// Every variable on_exit reads is assigned above the trap (set -u: an
	// early refusal must not abort the handler), and so are both functions.
	for _, decl := range []string{
		`STATUS_FILE="${STYRMAN_UPDATE_STATUS_FILE:-}"`, "\nSTATUS_ARMED=0", "\nSTATUS_RESULT=\"\"",
		"\nSTATUS_CODE=\"\"", "\nSTATUS_DEPLOYED=\"\"", "\nSTATUS_LATEST=\"\"", "\nEXIT_RM_DIR=\"\"",
		"\nwrite_status() {", "\non_exit() {",
	} {
		if i := strings.Index(src, decl); i < 0 || i > trap {
			t.Errorf("%q must be assigned above the trap line", decl)
		}
	}
	// Armed only below the lock, and never under --check.
	lock := at("flock -n 9 ||")
	armed := at("    STATUS_ARMED=1\n")
	if armed < lock {
		t.Error("the status must be armed below the lock")
	}
	if !strings.Contains(src[lock:armed], `if [ "${CHECK_ONLY}" = false ]; then`) {
		t.Error("the status must not be armed under --check")
	}
	// Each site names its code before it fails.
	for _, site := range []string{
		"STATUS_CODE=fetch-auth ;;",
		"*) STATUS_CODE=fetch ;;",
		`{ STATUS_CODE=no-tags; fail "no stable vX.Y.Z tags found"; }`,
		`{ STATUS_CODE=signature; fail "refusing to deploy ${LATEST} (see above)"; }`,
		"STATUS_RESULT=failing; STATUS_CODE=latched\n    exit 0",
		`STATUS_CODE=rolled-back; STATUS_DEPLOYED="${PREVIOUS}"`,
	} {
		if !strings.Contains(src, site) {
			t.Errorf("site missing: %q", site)
		}
	}
	if n := strings.Count(src, "STATUS_CODE=deploy-failed\n"); n != 2 {
		t.Errorf("deploy-failed is set at %d sites, want 2 (no previous version; the rollback failed too)", n)
	}
}

// The unit pins the path; the script has no default of its own (ADR-052).
func TestStyrmanUpdateStatusPathIsPinnedInTheUnitOnly(t *testing.T) {
	unit, err := os.ReadFile("../../deploy/update/styrman-update.service")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(unit), "\nEnvironment=STYRMAN_UPDATE_STATUS_FILE=/etc/styrman/status/updater.json\n") {
		t.Error("styrman-update.service must pin STYRMAN_UPDATE_STATUS_FILE")
	}
	script, err := os.ReadFile(updaterScript(t))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(script), `STATUS_FILE="${STYRMAN_UPDATE_STATUS_FILE:-}"`) {
		t.Error("the script must take the status path from the environment with no default")
	}
	// Unset, a tick-shaped exit writes nothing anywhere.
	dir := t.TempDir()
	cmd := exec.Command("bash", "-c", `STATUS_ARMED=1; source <(sed -n '/^log()/,/^# ALWAYS clear inherited/p' "$1"); TRUST_UID=$(id -u); on_exit`, "_", updaterScript(t))
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "STYRMAN_UPDATE_STATUS_FILE=")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("on_exit with no status path: %v: %s", err, out)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("something was written with the path unset: %v", entries)
	}
}

// The writer's path (the unit) and the reader's (the compose file: the
// server's environment and its read-only mount) are pinned separately, and
// nothing but this test holds them together.
func TestStyrmanUpdateStatusPathMatchesTheComposeMount(t *testing.T) {
	raw, err := os.ReadFile("../../deploy/docker-compose.yml")
	if err != nil {
		t.Fatal(err)
	}
	compose := string(raw)
	for _, line := range []string{
		"\n      - STYRMAN_UPDATE_STATUS_FILE=/etc/styrman/status/updater.json\n",
		"\n      - /etc/styrman/status:/etc/styrman/status:ro\n",
	} {
		if strings.Count(compose, line) != 1 {
			t.Errorf("docker-compose.yml must hold exactly one %q", strings.TrimSpace(line))
		}
	}
	// Read-only, and nothing wider: the server must never see /etc/styrman
	// itself (the updater's token and state live there).
	for _, l := range strings.Split(compose, "\n") {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "- /etc/styrman") && l != "- /etc/styrman/status:/etc/styrman/status:ro" {
			t.Errorf("unexpected mount of the updater's directory: %q", l)
		}
	}
}

// The REAL early-refusal path, not a hook: as a non-root caller the script
// stops at "must run as root", right below the trap. The handler runs
// unarmed — nothing is written, even into a directory that would pass — and
// the exit status is fail's, unchanged.
func TestStyrmanUpdateStatusEarlyRefusalWritesNothing(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("runs the root refusal; meaningless as root")
	}
	dir, file := statusDir(t)
	cmd := exec.Command("bash", updaterScript(t), "--check")
	cmd.Env = append(os.Environ(), "STYRMAN_UPDATE_STATUS_FILE="+file)
	out, err := cmd.CombinedOutput()
	ee, ok := err.(*exec.ExitError)
	if !ok || ee.ExitCode() != 1 || !strings.Contains(string(out), "must run as root") {
		t.Fatalf("want the root refusal with exit 1, got %v: %s", err, out)
	}
	if strings.Contains(string(out), "unbound variable") {
		t.Fatalf("the handler hit an unset variable: %s", out)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("an unarmed exit wrote %v", entries)
	}
}

// The path itself, not only its last directory (security review of
// v1.40.0..5571b57, finding 2): absolute and canonical like the state dir —
// a relative path, a `..`, or a symlink ANYWHERE above the directory is
// refused, with nothing written through it and the tick's status untouched.
func TestStyrmanUpdateStatusWriteRefusesANonCanonicalPath(t *testing.T) {
	realParent := t.TempDir()
	if err := os.Chmod(realParent, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(realParent, "status"), 0o755); err != nil {
		t.Fatal(err)
	}
	linkRoot := t.TempDir()
	if err := os.Symlink(realParent, filepath.Join(linkRoot, "etc")); err != nil {
		t.Fatal(err)
	}
	for name, file := range map[string]string{
		"relative":         "status/updater.json",
		"dot-dot":          realParent + "/status/../status/updater.json", // not filepath.Join: it would clean the .. away
		"symlinked parent": filepath.Join(linkRoot, "etc", "status", "updater.json"),
	} {
		code, out := runUpdaterHook(t, true, "__write-status", file, "ok", "ok", "", "")
		if code != 0 || !strings.Contains(out, "WARN: status not written") {
			t.Errorf("%s: want a WARN and exit 0, got %d %q", name, code, out)
		}
	}
	if entries, _ := os.ReadDir(filepath.Join(realParent, "status")); len(entries) != 0 {
		t.Errorf("a status was written through a non-canonical path: %v", entries)
	}
}
