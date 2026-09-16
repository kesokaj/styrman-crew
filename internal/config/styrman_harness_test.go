package config

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The harness channel (ADR-073, plan 2026-09-16-harness-channel.md): the
// `harness` verb of deploy/update/styrman-update, run in test mode
// (STYRMAN_UPDATE_TEST=1, every path a scratch path, unelevated) against a
// scratch origin built from the mirror's own files and tagged with a scratch
// ed25519 key. The walk downward, the refusals, the latch, the downgrade
// rule, --check, --force and the export's shape are all asserted here; the
// real signer and the real host dirs are never touched. The export builds
// sh-agent inside the Go builder image image/Dockerfile pins and smokes the
// candidate inside the runtime image, so docker and both images must be
// present locally (they are on the verifier host; elsewhere the test skips).
func TestStyrmanHarnessChannel(t *testing.T) {
	for _, tool := range []string{"git", "ssh-keygen", "bash", "docker", "diff", "sha256sum"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("no %s on this host", tool)
		}
	}
	crew, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(crew, "deploy", "update", "styrman-update")
	mirror := filepath.Join(crew, "image", "harness")
	smokeImage := os.Getenv("STYRMAN_HARNESS_SMOKE_IMAGE")
	if smokeImage == "" {
		smokeImage = "styrman-runtime:test"
	}
	builder := builderPin(t, filepath.Join(crew, "image", "Dockerfile"))
	for _, img := range []string{smokeImage, builder} {
		if out, err := exec.Command("docker", "image", "inspect", "--format", "{{.Id}}", img).CombinedOutput(); err != nil {
			t.Skipf("image %s not present locally (%v): %s", img, err, strings.TrimSpace(string(out)))
		}
	}

	root := t.TempDir()
	origin := filepath.Join(root, "origin")
	clone := filepath.Join(root, "clone")
	state := filepath.Join(root, "state", "update") // nested, as ADR-052 asks
	content := filepath.Join(root, "harness")
	home := filepath.Join(root, "home")
	for _, d := range []string{origin, filepath.Join(home, ".ssh")} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	env := append(os.Environ(),
		"HOME="+home, "GIT_CONFIG_GLOBAL="+filepath.Join(home, "gitconfig"), "GIT_CONFIG_NOSYSTEM=1",
		"STYRMAN_UPDATE_STATE_DIR="+state,
		"STYRMAN_HARNESS_REPO_DIR="+clone,
		"STYRMAN_HARNESS_DIR="+content,
		"STYRMAN_DEPLOY_DIR="+crew,
		"STYRMAN_HARNESS_SMOKE_IMAGE="+smokeImage,
	)
	git := func(dir string, args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	// The origin: the mirror's files minus crew's own three, one commit.
	copyTree(t, mirror, origin, map[string]bool{"COMMIT": true, "SHA256SUMS": true, "VENDORED.md": true, ".agents": true})
	git(origin, "init", "-q", "-b", "main")
	git(origin, "add", "-A")
	git(origin, "-c", "user.name=t", "-c", "user.email=t@example.test", "commit", "-q", "-m", "base")
	commitA := git(origin, "rev-parse", "HEAD")
	// Two keys: the allowed one and a foreign one.
	keyAllowed := filepath.Join(home, ".ssh", "id_allowed")
	keyForeign := filepath.Join(home, ".ssh", "id_foreign")
	for _, k := range []string{keyAllowed, keyForeign} {
		if out, err := exec.Command("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", k).CombinedOutput(); err != nil {
			t.Fatalf("ssh-keygen: %v\n%s", err, out)
		}
	}
	pub, err := os.ReadFile(keyAllowed + ".pub")
	if err != nil {
		t.Fatal(err)
	}
	signers := filepath.Join(root, "allowed_signers")
	if err := os.WriteFile(signers, []byte("t@example.test "+strings.TrimSpace(string(pub))+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	env = append(env, "STYRMAN_ALLOWED_SIGNERS="+signers)
	sign := func(tag, key string) {
		t.Helper()
		git(origin, "-c", "user.name=t", "-c", "user.email=t@example.test", "-c", "gpg.format=ssh", "-c", "user.signingkey="+key+".pub", "tag", "-s", tag, "-m", "styrman-harness "+tag)
	}
	sign("v0.9.0", keyAllowed)
	sign("v1.0.0", keyAllowed)
	// Commit B carries a symlink inside the tree: a tag on it verifies but
	// must fail the export (no symlinks in a plain copy).
	if err := os.Symlink("/etc/passwd", filepath.Join(origin, "skills", "evil")); err != nil {
		t.Fatal(err)
	}
	git(origin, "add", "-A")
	git(origin, "-c", "user.name=t", "-c", "user.email=t@example.test", "commit", "-q", "-m", "a symlink")
	sign("v1.0.1", keyAllowed)
	git(origin, "tag", "v1.1.0")            // lightweight: refused
	sign("v1.2.0", keyForeign)              // not an allowed signer: refused
	git(root, "clone", "-q", origin, clone) // the channel's own clone; it fetches origin's tags

	run := func(args ...string) (int, string) {
		t.Helper()
		cmd := exec.Command("bash", append([]string{script, "harness"}, args...)...)
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
	readState := func(name string) string {
		b, _ := os.ReadFile(filepath.Join(state, name))
		return strings.TrimSpace(string(b))
	}
	current := filepath.Join(content, "current")
	target := func() string {
		t.Helper()
		got, err := os.Readlink(current)
		if err != nil {
			t.Fatalf("current is not a symlink: %v", err)
		}
		return got
	}

	// 1. The walk: v1.2.0 (foreign key) and v1.1.0 (lightweight) refused,
	// v1.0.1 verifies but fails the export (a symlink) and is latched, v1.0.0
	// is installed; v0.9.0 is never reached.
	code, out := run()
	if code != 0 {
		t.Fatalf("first run: exit %d\n%s", code, out)
	}
	for _, want := range []string{"REFUSED: v1.2.0", "REFUSED: v1.1.0", "REFUSED: v1.0.1", "v1.0.0 (" + commitA[:7] + ") is current"} {
		if !strings.Contains(out, want) {
			t.Fatalf("first run: missing %q\n%s", want, out)
		}
	}
	if strings.Contains(out, "v0.9.0") {
		t.Fatalf("first run must stop at v1.0.0, not reach v0.9.0\n%s", out)
	}
	if got := readState("harness-version"); got != "v1.0.0" {
		t.Fatalf("harness-version = %q, want v1.0.0", got)
	}
	if got := readState("harness-sha"); got != commitA {
		t.Fatalf("harness-sha = %q, want %s", got, commitA)
	}
	if got := readState("harness-failed-version"); got != "v1.0.1" {
		t.Fatalf("harness-failed-version = %q, want v1.0.1 (the symlinked tag is latched)", got)
	}
	if refused := readState("harness-refused"); strings.Count(refused, "\n")+1 != 3 {
		t.Fatalf("harness-refused should hold three lines (v1.2.0, v1.1.0, v1.0.1):\n%s", refused)
	}
	if got := target(); got != commitA {
		t.Fatalf("current -> %q, want the commit dir %s", got, commitA)
	}
	repo := filepath.Join(current, "repo")
	for _, f := range []string{"COMMIT", "TAG", "SHA256SUMS"} {
		if _, err := os.Stat(filepath.Join(repo, f)); err != nil {
			t.Fatalf("export lacks %s: %v", f, err)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(repo, "COMMIT")); strings.TrimSpace(string(b)) != commitA {
		t.Fatalf("repo/COMMIT = %q, want %s", strings.TrimSpace(string(b)), commitA)
	}
	if b, _ := os.ReadFile(filepath.Join(repo, "TAG")); strings.TrimSpace(string(b)) != "v1.0.0" {
		t.Fatalf("repo/TAG = %q, want v1.0.0", strings.TrimSpace(string(b)))
	}
	if _, err := os.Stat(filepath.Join(repo, ".git")); err == nil {
		t.Fatal("the export carries a .git — a plain copy must not")
	}
	sums := exec.Command("sha256sum", "-c", "--quiet", "SHA256SUMS")
	sums.Dir = repo
	if o, err := sums.CombinedOutput(); err != nil {
		t.Fatalf("SHA256SUMS does not verify: %v\n%s", err, o)
	}
	bin := filepath.Join(current, "bin", "sh-agent")
	if st, err := os.Stat(bin); err != nil || st.Mode()&0o111 == 0 {
		t.Fatalf("bin/sh-agent missing or not executable: %v", err)
	}
	_ = filepath.WalkDir(filepath.Join(content, commitA), func(p string, d fs.DirEntry, err error) error {
		if err == nil && d.Type()&fs.ModeSymlink != 0 {
			t.Fatalf("symlink inside the export: %s", p)
		}
		return nil
	})
	// Risk (d): the export equals the mirror up to crew's own files and TAG.
	d := exec.Command("diff", "-rq", "--exclude", ".agents", repo, mirror) // the refresh recipe's own exclusion
	dout, _ := d.CombinedOutput()                                          // exit 1 is expected: the four known lines
	lines := strings.Split(strings.TrimSpace(string(dout)), "\n")
	okLine := regexp.MustCompile(`^(Only in ` + regexp.QuoteMeta(mirror) + `: VENDORED\.md|Only in ` + regexp.QuoteMeta(repo) + `: TAG|Files .*/(COMMIT|SHA256SUMS) and .*/(COMMIT|SHA256SUMS) differ)$`)
	if len(lines) != 4 {
		t.Fatalf("diff -rq export mirror: want exactly 4 lines, got %d:\n%s", len(lines), dout)
	}
	for _, l := range lines {
		if !okLine.MatchString(l) {
			t.Fatalf("unexpected diff line: %s", l)
		}
	}

	// 2. A second run: nothing newer verifies; the latched tag is skipped.
	code, out = run()
	if code != 0 || !strings.Contains(out, "nothing newer than v1.0.0 verifies") || !strings.Contains(out, "v1.0.1") {
		t.Fatalf("second run: exit %d\n%s", code, out)
	}
	if got := target(); got != commitA {
		t.Fatalf("second run moved current to %q", got)
	}

	// 3. --check changes nothing.
	before, _ := os.Stat(filepath.Join(state, "harness-version"))
	code, out = run("--check")
	if code != 0 {
		t.Fatalf("--check: exit %d\n%s", code, out)
	}
	after, _ := os.Stat(filepath.Join(state, "harness-version"))
	if !before.ModTime().Equal(after.ModTime()) || target() != commitA {
		t.Fatalf("--check changed state\n%s", out)
	}

	// 4. The downgrade rule: with v1.3.0 recorded as installed (its tag
	// deleted upstream), the newest candidate v1.2.0 is below it — refused,
	// alarmed, nothing moves.
	if err := os.WriteFile(filepath.Join(state, "harness-version"), []byte("v1.3.0"), 0o644); err != nil {
		t.Fatal(err)
	}
	refusedBefore := readState("harness-refused")
	code, out = run()
	if code != 0 || !strings.Contains(out, "below the installed v1.3.0") {
		t.Fatalf("downgrade: exit %d\n%s", code, out)
	}
	if readState("harness-refused") == refusedBefore {
		t.Fatal("the downgrade refusal was not alarmed into harness-refused")
	}
	if got := readState("harness-version"); got != "v1.3.0" || target() != commitA {
		t.Fatalf("downgrade moved state: version %q, current %s", got, target())
	}
	if err := os.WriteFile(filepath.Join(state, "harness-version"), []byte("v1.0.0"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 5. --force re-exports the installed tag in place: current still names
	// the same commit dir, the previous content is kept beside it.
	code, out = run("--force")
	if code != 0 || !strings.Contains(out, "v1.0.0 ("+commitA[:7]+") is current") {
		t.Fatalf("--force: exit %d\n%s", code, out)
	}
	if got := target(); got != commitA {
		t.Fatalf("--force moved current to %q", got)
	}
	prev, _ := filepath.Glob(filepath.Join(content, commitA+".prev-*"))
	if len(prev) != 1 {
		t.Fatalf("--force must keep exactly one previous dir, got %v", prev)
	}

	// 6. Without the test gate the verb takes the production path, which
	// refuses to run unelevated.
	cmd := exec.Command("bash", script, "harness")
	cmd.Env = env
	if o, err := cmd.CombinedOutput(); err == nil || !strings.Contains(string(o), "must run as root") {
		t.Fatalf("ungated harness verb: %v\n%s", err, o)
	}
}

// builderPin reads the Go builder image image/Dockerfile pins for sh-agent,
// the way the script does (the harness-builder stage's FROM line).
func builderPin(t *testing.T, dockerfile string) string {
	t.Helper()
	b, err := os.ReadFile(dockerfile)
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^FROM (golang:[^ ]+) AS harness-builder$`).FindSubmatch(b)
	if m == nil {
		t.Fatal("no harness-builder FROM line in image/Dockerfile")
	}
	return string(m[1])
}

// copyTree copies src into dst (dirs 0755, files keep their mode), skipping
// the top-level names in skip.
func copyTree(t *testing.T, src, dst string, skip map[string]bool) {
	t.Helper()
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if rel == "." {
			return nil
		}
		if skip[rel] {
			return nil
		}
		out := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(out, 0o755)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(out, b, info.Mode().Perm())
	})
	if err != nil {
		t.Fatal(err)
	}
}
