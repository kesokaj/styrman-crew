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
