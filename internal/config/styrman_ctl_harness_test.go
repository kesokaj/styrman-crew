package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The harness channel's half of `styrman-ctl update --setup` (ADR-073), as a
// dry-run plan: the harness token staged and installed root 600, the content
// dir root 755, the styrman-harness clone at its fixed path (the literal
// /opt/styrman-harness in production; ctlSetup's seam points it into the
// scratch root) under the same scoped credential helper, the channel's own --check with every input
// named, and its timer — and, unattended without a token, the skip line that
// names the command for later, with none of the harness steps planned.
func TestStyrmanCtlUpdateSetupHarnessPlan(t *testing.T) {
	e := ctlSetup(t, true)
	gitFake := "#!/usr/bin/env bash\necho \"git $* GIT_ASKPASS=${GIT_ASKPASS:-} GIT_TERMINAL_PROMPT=${GIT_TERMINAL_PROMPT:-}\" >> \"" + e.calls + "\"\ncase \"$*\" in *get-url*) echo git@github.com:owner/repo.git ;; *rev-parse*) echo 4686792 ;; *) echo v1.27.1 ;; esac\n"
	if err := os.WriteFile(filepath.Join(e.bin, "git"), []byte(gitFake), 0o755); err != nil {
		t.Fatal(err)
	}
	tok := filepath.Join(e.root, "secrets", "tok")
	if err := os.WriteFile(tok, []byte("ghp_SECRETTOKEN\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	htok := filepath.Join(e.root, "secrets", "harness-tok")
	if err := os.WriteFile(htok, []byte("ghp_HARNESSSECRET\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	signers := filepath.Join(e.root, "secrets", "signers")
	if err := os.WriteFile(signers, []byte("me@example.test ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIExample\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	envFile := filepath.Join(e.root, "checkout.env")
	if err := os.WriteFile(envFile, []byte("BASE_DOMAIN=example.test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(e.root, "opt", "styrman", ".git")); err != nil {
		t.Fatal(err)
	}
	e.env = append(e.env, "STYRMAN_ENV_FILE="+envFile)
	harnessClone := filepath.Join(e.root, "opt", "styrman-harness") // the STYRMAN_HARNESS_REPO_DIR seam ctlSetup sets
	etc := filepath.Join(e.root, "etc", "styrman")
	content := filepath.Join(e.root, "var", "lib", "styrman", "harness")

	out, code := ctl(t, e, "update", "--setup", "--dry-run", "--yes", "--gh-token-file", tok, "--allowed-signers-file", signers, "--harness-token-file", htok)
	if code != 0 {
		t.Fatalf("update --setup with the harness token: %d\n%s", code, out)
	}
	if strings.Contains(out, "HARNESSSECRET") || strings.Contains(out, "SECRETTOKEN") {
		t.Fatalf("a token leaked into the plan:\n%s", out)
	}
	lines := planLines(out)
	want := []string{
		"systemctl start styrman-update.service", // the crew half comes first
		"install -m 600 -o root -g root <tmpdir>/gh-token-harness " + filepath.Join(etc, "gh-token-harness"),
		"install -d -m 755 -o root -g root " + content,
		"clone -- https://github.com/kesokaj/styrman-harness.git " + harnessClone,
		"chown -R root:root " + harnessClone,
		"styrman-update harness --check",
		"systemctl enable --now styrman-harness-update.timer",
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
	check := lines[indexOf(lines, "styrman-update harness --check")]
	for _, w := range []string{"STYRMAN_HARNESS_REPO_DIR=" + harnessClone, "STYRMAN_HARNESS_DIR=" + content, "STYRMAN_HARNESS_GH_TOKEN_FILE=" + filepath.Join(etc, "gh-token-harness"), "STYRMAN_UPDATE_STATE_DIR=" + filepath.Join(etc, "update")} {
		if !strings.Contains(check, w) {
			t.Fatalf("harness --check lacks %q: %s", w, check)
		}
	}
	clone := lines[indexOf(lines, "clone -- https://github.com/kesokaj/styrman-harness.git")]
	for _, w := range []string{"-c credential.helper= ", "credential.https://github.com.helper=", "gh-token-harness", "GIT_TERMINAL_PROMPT=0"} {
		if !strings.Contains(clone, w) {
			t.Fatalf("harness clone line lacks %q: %s", w, clone)
		}
	}
	if strings.Contains(clone, "gh-token ") || strings.Contains(clone, "gh-token\"") {
		t.Fatalf("the harness clone must read its own token, not the crew one: %s", clone)
	}
	// The snippet the helper's shell parses is constant: the token path is one
	// env word of the git call, never interpolated (a re-interpolated
	// `cat '<path>'` would pass every assertion above).
	if !strings.Contains(clone, `cat "${STYRMAN_HARNESS_GH_TOKEN_FILE}"`) || strings.Contains(clone, "cat '") || !strings.Contains(clone, "STYRMAN_HARNESS_GH_TOKEN_FILE="+filepath.Join(etc, "gh-token-harness")+" git") {
		t.Fatalf("the harness clone's helper must read the token path from its env word: %s", clone)
	}
	if strings.Contains(out, "=/opt/") || strings.Contains(out, " /opt/") {
		t.Fatalf("a production path leaked into a scratch-prefixed plan:\n%s", out)
	}

	// Unattended without a harness token: the crew half is planned, the
	// harness half is skipped with the command that does it later.
	out, code = ctl(t, e, "update", "--setup", "--dry-run", "--yes", "--gh-token-file", tok, "--allowed-signers-file", signers)
	if code != 0 {
		t.Fatalf("update --setup without the harness token: %d\n%s", code, out)
	}
	if !strings.Contains(out, "harness channel not set up (no token)") || !strings.Contains(out, "--harness-token-file") {
		t.Fatalf("the skip must name the later command:\n%s", out)
	}
	// The crew updater's own env still names the harness paths (the same
	// script serves both verbs); what must be absent is the harness TOKEN
	// install and the harness timer.
	if strings.Contains(out, "styrman-harness-update.timer") || strings.Contains(out, "<tmpdir>/gh-token-harness") || strings.Contains(out, "styrman-harness.git") {
		t.Fatalf("harness steps planned without a token:\n%s", out)
	}
}

// A latched failed harness tag is a stuck channel whatever installed under
// it: `status` is DEGRADED (exit 1) and names the tag; without the latch the
// same host reads OK.
func TestStyrmanCtlStatusLatchedHarnessTagIsDegraded(t *testing.T) {
	e := ctlSetup(t, true)
	update := filepath.Join(e.root, "etc", "styrman", "update")
	if err := os.MkdirAll(update, 0o755); err != nil {
		t.Fatal(err)
	}
	if out, code := ctl(t, e, "status", "--dry-run"); code != 0 || strings.Contains(out, "FAILED LATCHED") {
		t.Fatalf("without a latch: code %d\n%s", code, out)
	}
	if err := os.WriteFile(filepath.Join(update, "harness-failed-version"), []byte("v1.2.0"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, code := ctl(t, e, "status", "--dry-run"); code != 1 || !strings.Contains(out, "DEGRADED") || !strings.Contains(out, "FAILED LATCHED: v1.2.0") {
		t.Fatalf("with a latched tag: code %d\n%s", code, out)
	}
}

// The two controls on the harness origin, each in its must-fail state: a
// non-https transport and a two-line value (a per-line grep would have
// passed the second) are refused before any clone; and outside the test
// seam the override itself is refused as a production knob.
func TestStyrmanCtlHarnessOriginIsGated(t *testing.T) {
	e := ctlSetup(t, true)
	gitFake := "#!/usr/bin/env bash\necho \"git $*\" >> \"" + e.calls + "\"\ncase \"$*\" in *get-url*) echo git@github.com:owner/repo.git ;; *rev-parse*) echo 4686792 ;; *) echo v1.27.1 ;; esac\n"
	if err := os.WriteFile(filepath.Join(e.bin, "git"), []byte(gitFake), 0o755); err != nil {
		t.Fatal(err)
	}
	tok := filepath.Join(e.root, "secrets", "tok")
	htok := filepath.Join(e.root, "secrets", "harness-tok")
	signers := filepath.Join(e.root, "secrets", "signers")
	for f, c := range map[string]string{tok: "ghp_SECRETTOKEN\n", htok: "ghp_HARNESSSECRET\n", signers: "me@example.test ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIExample\n"} {
		if err := os.WriteFile(f, []byte(c), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	envFile := filepath.Join(e.root, "checkout.env")
	if err := os.WriteFile(envFile, []byte("BASE_DOMAIN=example.test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(e.root, "opt", "styrman", ".git")); err != nil {
		t.Fatal(err)
	}
	base := append([]string(nil), e.env...)
	for _, origin := range []string{"ext::sh -c id", "https://github.com/x/y\nhttps://github.com/kesokaj/styrman-harness.git", "file:///tmp/x", "https://evil.test/kesokaj/styrman-harness.git"} {
		e.env = append(append([]string(nil), base...), "STYRMAN_ENV_FILE="+envFile, "STYRMAN_HARNESS_ORIGIN="+origin)
		out, code := ctl(t, e, "update", "--setup", "--dry-run", "--yes", "--gh-token-file", tok, "--allowed-signers-file", signers, "--harness-token-file", htok)
		if code == 0 || !strings.Contains(out, "must be a plain https://github.com/ URL") {
			t.Fatalf("origin %q: code %d\n%s", origin, code, out)
		}
		if strings.Contains(out, "clone -- ") && strings.Contains(out, "styrman-harness") && strings.Contains(out, "clone -- "+strings.Split(origin, "\n")[0]) {
			t.Fatalf("origin %q reached a clone:\n%s", origin, out)
		}
	}
	// Outside the seam: only the documented knobs, plus the override.
	var prod []string
	for _, kv := range base {
		if strings.HasPrefix(kv, "PATH=") || strings.HasPrefix(kv, "STYRMAN_STATE_DIR=") || strings.HasPrefix(kv, "STYRMAN_DEPLOY_DIR=") || strings.HasPrefix(kv, "HOME=") {
			prod = append(prod, kv)
		}
	}
	e.env = append(prod, "STYRMAN_HARNESS_ORIGIN=https://github.com/kesokaj/styrman-harness.git")
	if out, code := ctl(t, e, "help"); code != 1 || !strings.Contains(out, "STYRMAN_HARNESS_ORIGIN is a test seam") {
		t.Fatalf("seam guard: code %d\n%s", code, out)
	}
	e.env = append(prod, "STYRMAN_HARNESS_REPO_DIR="+filepath.Join(e.root, "elsewhere"))
	if out, code := ctl(t, e, "help"); code != 1 || !strings.Contains(out, "STYRMAN_HARNESS_REPO_DIR is a test seam") {
		t.Fatalf("seam guard: code %d\n%s", code, out)
	}
}
