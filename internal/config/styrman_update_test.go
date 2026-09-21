package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The auto-updater's signature gate, run against fixtures through its test
// verb (`styrman-update __verify-tag <tag> <allowed_signers> <repo>`, gated
// by STYRMAN_UPDATE_TEST=1). Git dispatches the verifier by the LAST armor
// block in the tag object and knows four markers; the gate used to know two,
// so an SSH block followed by a PGP MESSAGE or SIGNED MESSAGE block passed
// the check while git verified with gpg/gpgsm (audit 2026-09-13). Tag
// objects are hand-built with git hash-object, placeholder armor only.
func TestStyrmanUpdateVerifyTagMarkers(t *testing.T) {
	for _, tool := range []string{"git", "ssh-keygen", "bash"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("no %s on this host", tool)
		}
	}
	script, err := filepath.Abs("../../deploy/update/styrman-update")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	env := append(os.Environ(), "HOME="+home, "GIT_CONFIG_GLOBAL="+filepath.Join(home, "gitconfig"), "GIT_CONFIG_NOSYSTEM=1")
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	if err := os.MkdirAll(repo, 0o700); err != nil {
		t.Fatal(err)
	}
	git("init", "-q", "-b", "main")
	git("-c", "user.name=t", "-c", "user.email=t@example.test", "commit", "-q", "--allow-empty", "-m", "base")
	head := git("rev-parse", "HEAD")
	// A scratch SSH key and allowed_signers naming it.
	key := filepath.Join(home, ".ssh", "id_ed25519")
	if out, err := exec.Command("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", key).CombinedOutput(); err != nil {
		t.Fatalf("ssh-keygen: %v\n%s", err, out)
	}
	pub, err := os.ReadFile(key + ".pub")
	if err != nil {
		t.Fatal(err)
	}
	signers := filepath.Join(root, "allowed_signers")
	if err := os.WriteFile(signers, []byte("t@example.test "+strings.TrimSpace(string(pub))+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// An honestly SSH-signed tag.
	git("-c", "user.name=t", "-c", "user.email=t@example.test", "-c", "gpg.format=ssh", "-c", "user.signingkey="+key+".pub", "tag", "-s", "v9.9.0", "-m", "styrman v9.9.0")
	// Fixture tag objects: an SSH SIGNATURE block (placeholder) followed by
	// another marker, the shapes the old grep passed.
	mk := func(name, trailing string) {
		t.Helper()
		obj := "object " + head + "\ntype commit\ntag " + name + "\ntagger t <t@example.test> 1700000000 +0000\n\nfixture\n-----BEGIN SSH SIGNATURE-----\nAAAA\n-----END SSH SIGNATURE-----\n"
		if trailing != "" {
			obj += "-----BEGIN " + trailing + "-----\nAAAA\n-----END " + trailing + "-----\n"
		}
		cmd := exec.Command("git", "hash-object", "-t", "tag", "-w", "--stdin")
		cmd.Dir = repo
		cmd.Env = env
		cmd.Stdin = strings.NewReader(obj)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("hash-object: %v\n%s", err, out)
		}
		git("update-ref", "refs/tags/"+name, strings.TrimSpace(string(out)))
	}
	mk("decoy-pgp-sig", "PGP SIGNATURE")
	mk("decoy-pgp-msg", "PGP MESSAGE")
	mk("decoy-signed-msg", "SIGNED MESSAGE")
	mk("decoy-ssh-only", "") // an SSH marker with no real signature: refused by verify-tag itself
	verify := func(tag, signersFile string) (int, string) {
		t.Helper()
		cmd := exec.Command("bash", script, "__verify-tag", tag, signersFile, repo)
		cmd.Env = append(env, "STYRMAN_UPDATE_TEST=1")
		out, err := cmd.CombinedOutput()
		code := 0
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else if err != nil {
			t.Fatalf("run: %v\n%s", err, out)
		}
		return code, string(out)
	}
	if code, out := verify("v9.9.0", signers); code != 0 || !strings.Contains(out, "signature OK") {
		t.Fatalf("honest tag: exit %d\n%s", code, out)
	}
	for _, decoy := range []string{"decoy-pgp-sig", "decoy-pgp-msg", "decoy-signed-msg", "decoy-ssh-only"} {
		if code, out := verify(decoy, signers); code == 0 || !strings.Contains(out, "REFUSED") {
			t.Fatalf("%s: exit %d, want a refusal\n%s", decoy, code, out)
		}
	}
	// An empty allowlist refuses even the honest tag.
	empty := filepath.Join(root, "empty_signers")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if code, out := verify("v9.9.0", empty); code == 0 {
		t.Fatalf("honest tag with an empty allowlist passed\n%s", out)
	}
	// The verb is refused without the test gate.
	cmd := exec.Command("bash", script, "__verify-tag", "v9.9.0", signers, repo)
	cmd.Env = env
	if out, err := cmd.CombinedOutput(); err == nil || !strings.Contains(string(out), "test hooks are disabled") {
		t.Fatalf("ungated verb: %v\n%s", err, out)
	}
}

// The retired harness channel (ADR-073 → ADR-074): a host that still carries
// the styrman-harness-update unit pair and the channel's state files sheds
// them on every updater tick, through `retire_harness_units`, run here via
// its test verb against scratch dirs with a fake systemctl first on PATH.
// Timer first, then the service, then one daemon-reload; a second run finds
// nothing and calls systemctl not at all; the hook is refused outside the
// test gate.
func TestStyrmanUpdateRetiresHarnessUnits(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("no bash on this host")
	}
	script, err := filepath.Abs("../../deploy/update/styrman-update")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	unitDir := filepath.Join(root, "units")
	stateDir := filepath.Join(root, "state")
	binDir := filepath.Join(root, "bin")
	calls := filepath.Join(root, "systemctl.calls")
	for _, d := range []string{unitDir, stateDir, binDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	fake := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"" + calls + "\"\n"
	if err := os.WriteFile(filepath.Join(binDir, "systemctl"), []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	plant := func() {
		t.Helper()
		for _, f := range []string{"styrman-harness-update.service", "styrman-harness-update.timer"} {
			if err := os.WriteFile(filepath.Join(unitDir, f), []byte("[Unit]\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		for _, f := range []string{"harness-version", "harness-sha", "harness-failed-version", "harness-refused"} {
			if err := os.WriteFile(filepath.Join(stateDir, f), []byte("x"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	runArgs := func(gated bool, extra ...string) (int, string) {
		t.Helper()
		cmd := exec.Command("bash", append([]string{script, "__retire-harness-units", unitDir, stateDir}, extra...)...)
		cmd.Env = append(os.Environ(), "PATH="+binDir+":"+os.Getenv("PATH"))
		if gated {
			cmd.Env = append(cmd.Env, "STYRMAN_UPDATE_TEST=1")
		}
		out, err := cmd.CombinedOutput()
		code := 0
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else if err != nil {
			t.Fatalf("run: %v\n%s", err, out)
		}
		return code, string(out)
	}
	run := func(gated bool) (int, string) { t.Helper(); return runArgs(gated) }

	plant()
	if code, out := run(false); code == 0 || !strings.Contains(out, "test hooks are disabled") {
		t.Fatalf("hook usable without STYRMAN_UPDATE_TEST=1: %d\n%s", code, out)
	}
	if _, err := os.Stat(filepath.Join(unitDir, "styrman-harness-update.timer")); err != nil {
		t.Fatalf("the refused hook still removed a unit: %v", err)
	}

	if code, out := run(true); code != 0 {
		t.Fatalf("retire: %d\n%s", code, out)
	}
	for _, f := range []string{"styrman-harness-update.service", "styrman-harness-update.timer"} {
		if _, err := os.Stat(filepath.Join(unitDir, f)); !os.IsNotExist(err) {
			t.Fatalf("%s still present after retire (err=%v)", f, err)
		}
	}
	for _, f := range []string{"harness-version", "harness-sha", "harness-failed-version", "harness-refused"} {
		if _, err := os.Stat(filepath.Join(stateDir, f)); !os.IsNotExist(err) {
			t.Fatalf("state file %s still present after retire (err=%v)", f, err)
		}
	}
	got, err := os.ReadFile(calls)
	if err != nil {
		t.Fatalf("the fake systemctl was never called: %v", err)
	}
	want := "disable --now styrman-harness-update.timer\ndisable --now styrman-harness-update.service\ndaemon-reload\n"
	if string(got) != want {
		t.Fatalf("systemctl calls:\n%s\nwant:\n%s", got, want)
	}

	// Idempotent: nothing left to do, and systemctl is not touched.
	if err := os.Remove(calls); err != nil {
		t.Fatal(err)
	}
	if code, out := run(true); code != 0 {
		t.Fatalf("second retire: %d\n%s", code, out)
	}
	if _, err := os.Stat(calls); !os.IsNotExist(err) {
		t.Fatalf("second run called systemctl (err=%v)", err)
	}

	// --check: reports and changes nothing — units, state files, systemctl.
	plant()
	if err := os.Remove(calls); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if code, out := runArgs(true, "--check"); code != 0 || !strings.Contains(out, "would remove") {
		t.Fatalf("--check: %d\n%s", code, out)
	}
	for _, f := range []string{"styrman-harness-update.service", "styrman-harness-update.timer"} {
		if _, err := os.Stat(filepath.Join(unitDir, f)); err != nil {
			t.Fatalf("--check removed %s: %v", f, err)
		}
	}
	for _, f := range []string{"harness-version", "harness-sha", "harness-failed-version", "harness-refused"} {
		if _, err := os.Stat(filepath.Join(stateDir, f)); err != nil {
			t.Fatalf("--check removed state file %s: %v", f, err)
		}
	}
	if _, err := os.Stat(calls); !os.IsNotExist(err) {
		t.Fatalf("--check called systemctl (err=%v)", err)
	}

	// An untrusted unit dir (group/world-writable) is refused: WARN, nothing removed.
	if err := os.Chmod(unitDir, 0o777); err != nil {
		t.Fatal(err)
	}
	if code, out := run(true); code != 0 || !strings.Contains(out, "harness-retire skipped") {
		t.Fatalf("untrusted unit dir: %d\n%s", code, out)
	}
	for _, f := range []string{"styrman-harness-update.service", "styrman-harness-update.timer"} {
		if _, err := os.Stat(filepath.Join(unitDir, f)); err != nil {
			t.Fatalf("untrusted unit dir still lost %s: %v", f, err)
		}
	}
	if _, err := os.Stat(calls); !os.IsNotExist(err) {
		t.Fatalf("untrusted unit dir called systemctl (err=%v)", err)
	}
}
