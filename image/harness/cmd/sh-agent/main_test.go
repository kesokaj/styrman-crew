package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/kesokaj/styrman-harness/internal/harness"
)

func TestReorder(t *testing.T) {
	cases := map[string][]string{
		// BUG-002: the flag package stops at the first positional argument.
		"adopt my-skill --home /x":          {"--home", "/x", "adopt", "my-skill"},
		"--vendor agy install":              {"--vendor", "agy", "install"},
		"adopt a --force":                   {"--force", "adopt", "a"},
		"adopt -- --looks-like-a-flag":      {"adopt", "--looks-like-a-flag"},
		"--repo r --home h adopt x --force": {"--repo", "r", "--home", "h", "--force", "adopt", "x"},
	}
	for in, want := range cases {
		if got := reorder(strings.Fields(in)); !reflect.DeepEqual(got, want) {
			t.Errorf("reorder(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestResolveVendor(t *testing.T) {
	home := t.TempDir()
	if v, err := resolveVendor("", home); err != nil || v != harness.VendorAll {
		t.Errorf("no flag, no manifest: %q, %v; want all", v, err)
	}
	if v, err := resolveVendor("agy", home); err != nil || v != harness.VendorAntigravity {
		t.Errorf("flag agy: %q, %v", v, err)
	}
	if _, err := resolveVendor("vscode", home); err == nil {
		t.Error("unknown vendor should fail")
	}
	if err := harness.SaveState(home, harness.State{Vendor: harness.VendorClaude}); err != nil {
		t.Fatal(err)
	}
	if v, err := resolveVendor("", home); err != nil || v != harness.VendorClaude {
		t.Errorf("manifest vendor: %q, %v; want claude", v, err)
	}
	if v, err := resolveVendor("all", home); err != nil || v != harness.VendorAll {
		t.Errorf("flag beats manifest: %q, %v", v, err)
	}
}

func TestShouldRebuild(t *testing.T) {
	cases := []struct{ goOnPath, child, own, want bool }{
		{true, false, true, true},
		{true, true, true, false},
		{false, false, true, false},
		{false, true, true, false},
		{true, false, false, false}, // a plain copy never rebuilds (BUG-015)
		{false, false, false, false},
	}
	for _, c := range cases {
		if got := shouldRebuild(c.goOnPath, c.child, c.own); got != c.want {
			t.Errorf("shouldRebuild(%v, %v, %v) = %v", c.goOnPath, c.child, c.own, got)
		}
	}
}

func TestBinaryOlder(t *testing.T) {
	cases := []struct {
		rev, commit string
		want        bool
	}{
		{"b64ffe886729", "b64ffe8", false},
		{"b64ffe8", "b64ffe886729", false},
		{"b64ffe886729+dirty", "b64ffe8", false},
		{"50ac771abcde", "b64ffe8", true},
		{"(devel)", "b64ffe8", true},
		{"unknown", "b64ffe8", true},
		{"", "b64ffe8", false},
		{"b64ffe886729", "", false},
	}
	for _, c := range cases {
		if got := binaryOlder(c.rev, c.commit); got != c.want {
			t.Errorf("binaryOlder(%q, %q) = %v", c.rev, c.commit, got)
		}
	}
}

func TestVersionFromInfo(t *testing.T) {
	stamp := func(rev, modified string) *debug.BuildInfo {
		return &debug.BuildInfo{
			Main:     debug.Module{Version: "(devel)"},
			Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: rev}, {Key: "vcs.modified", Value: modified}},
		}
	}
	cases := []struct {
		name string
		info *debug.BuildInfo
		want string
	}{
		{"clean", stamp("b64ffe886729abcdef0123456789abcdef012345", "false"), "b64ffe886729"},
		{"dirty", stamp("b64ffe886729abcdef0123456789abcdef012345", "true"), "b64ffe886729+dirty"},
		{"short revision kept whole", stamp("b64ffe8", "false"), "b64ffe8"},
		{"no revision", &debug.BuildInfo{Main: debug.Module{Version: "v1.2.3"}}, "v1.2.3"},
	}
	for _, c := range cases {
		if got := versionFromInfo(c.info); got != c.want {
			t.Errorf("%s: versionFromInfo = %q, want %q", c.name, got, c.want)
		}
	}
}

// TestRunInstallStatusUninstall drives the CLI end to end against this
// checkout and a scratch home. ChildEnv is set so install takes the plain
// path: under `go test` the running binary is the test binary, and a rebuild
// here would be a real `go build` of the repo for nothing.
func TestRunInstallStatusUninstall(t *testing.T) {
	t.Setenv(harness.ChildEnv, "1")
	home := t.TempDir()
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run([]string{"install", "--repo", repo, "--home", home}, &out); err != nil {
		t.Fatalf("install: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "files managed") {
		t.Errorf("install output:\n%s", out.String())
	}
	if _, err := os.Stat(filepath.Join(home, harness.BinPath)); err != nil {
		t.Errorf("binary not installed: %v", err)
	}
	out.Reset()
	if err := run([]string{"status", "--repo", repo, "--home", home}, &out); err != nil {
		t.Fatalf("status: %v\n%s", err, out.String())
	}
	for _, want := range []string{"binary     ", "drift      none"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("status missing %q:\n%s", want, out.String())
		}
	}
	// The binary line reads the installed file's build info: the test binary
	// was copied there, so the line must carry its version, never "unknown".
	if strings.Contains(out.String(), "binary     unknown") {
		t.Errorf("status could not read the installed binary's build info:\n%s", out.String())
	}
	out.Reset()
	if err := run([]string{"uninstall", "--home", home}, &out); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	// The vendor roots themselves are kept on purpose (pruneEmptyDirs stops
	// there); everything the manifest listed under them is gone.
	for _, rel := range []string{filepath.Join(harness.ClaudeRoot, "AGENTS.md"), filepath.Join(harness.ClaudeRoot, "skills"), filepath.Join(harness.AgyRoot, "agents"), harness.BinPath} {
		if _, err := os.Stat(filepath.Join(home, rel)); !os.IsNotExist(err) {
			t.Errorf("~/%s should be gone after uninstall, stat err = %v", rel, err)
		}
	}
	out.Reset()
	if err := run([]string{"status", "--repo", repo, "--home", home}, &out); err == nil {
		t.Error("status after uninstall should report exit 1")
	}
}

func TestKnownCheckout(t *testing.T) {
	home := t.TempDir()
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("SH_AGENT_REPO", "")
	if err := knownCheckout("", home, repo); err != nil {
		t.Errorf("first install (no manifest) must pass: %v", err)
	}
	if err := harness.SaveState(home, harness.State{Repo: repo}); err != nil {
		t.Fatal(err)
	}
	if err := knownCheckout("", home, repo); err != nil {
		t.Errorf("same checkout must pass: %v", err)
	}
	other := t.TempDir()
	if err := knownCheckout("", home, other); err == nil {
		t.Error("a checkout found only by the directory walk must be refused when another is installed")
	}
	if err := knownCheckout(other, home, other); err != nil {
		t.Errorf("explicit --repo must pass: %v", err)
	}
	t.Setenv("SH_AGENT_REPO", other)
	if err := knownCheckout("", home, other); err != nil {
		t.Errorf("$SH_AGENT_REPO must pass: %v", err)
	}
}

// TestDryRunNeverBuilds: with a toolchain present, no child guard and a
// checkout-shaped repo, a dry run must report the rebuild instead of
// performing it, and write nothing.
func TestDryRunNeverBuilds(t *testing.T) {
	t.Setenv(harness.ChildEnv, "")
	// A checkout-SHAPED repo of its own, not this checkout (`../..`): when
	// the test runs inside a downstream mirror — a plain copy with no .git,
	// which is what styrman-crew's image/harness is — `../..` is not a
	// checkout, the dry run rightly prints the plain-copy line, and the
	// assertion below failed for a shape the test never meant to cover
	// (styrman-crew BUG-058). IsCheckout asks only for a `.git` entry and
	// Inspect tolerates one git cannot read, so an empty directory is enough.
	repo := checkoutShaped(t)
	home := t.TempDir()
	var out bytes.Buffer
	if err := run([]string{"install", "--dry-run", "--repo", repo, "--home", home}, &out); err != nil {
		t.Fatalf("dry run: %v\n%s", err, out.String())
	}
	for _, want := range []string{"[dry-run] the binary would be rebuilt from", "[dry-run] "} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("dry-run output missing %q:\n%s", want, out.String())
		}
	}
	// Reinstall removes its build directory itself, so its absence proves
	// nothing; what a rebuild would leave is an installed binary and a
	// child's output, and a dry run must leave neither.
	for _, rel := range []string{harness.BinPath, ".claude", ".config"} {
		if _, err := os.Stat(filepath.Join(home, rel)); !os.IsNotExist(err) {
			t.Errorf("dry run wrote ~/%s: stat err = %v", rel, err)
		}
	}
	if strings.Contains(out.String(), "building sh-agent from") {
		t.Errorf("dry run ran a build:\n%s", out.String())
	}
}

// checkoutShaped is plainCopy plus the one thing IsCheckout looks for: a
// `.git` entry. It is what the dry-run test needs — "this is a checkout"
// without being THIS checkout — and it is not a git repository, which
// Inspect tolerates (empty fields) and nothing on the install path needs.
func checkoutShaped(t *testing.T) string {
	t.Helper()
	dir := plainCopy(t)
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// plainCopy copies this checkout's deployable trees into a directory with
// no .git: what a downstream image ships (styrman-crew image/harness,
// argolis-devbox deploy/harness).
func plainCopy(t *testing.T) string {
	t.Helper()
	src, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	dst := t.TempDir()
	for _, top := range []string{"context", "skills", "agents", "commands", "rules"} {
		root := filepath.Join(src, top)
		err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(src, p)
			if d.IsDir() {
				return os.MkdirAll(filepath.Join(dst, rel), 0o755)
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(dst, rel), b, 0o644)
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if harness.IsCheckout(dst) {
		t.Fatal("fixture: the plain copy must not carry .git")
	}
	return dst
}

// TestPlainCopyNeverRebuilds: a mirror inside a project's image is built by
// that image from the same tree, is usually read-only to the installing
// user, and cannot be updated with git; install from it must deploy the
// running binary without a rebuild, without the no-toolchain warning, and
// without creating a scratch directory inside the mirror (BUG-015).
func TestPlainCopyNeverRebuilds(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not on PATH: the rebuild path is unreachable, nothing to prove")
	}
	t.Setenv(harness.ChildEnv, "")
	os.Unsetenv(harness.ChildEnv)
	copyDir := plainCopy(t)
	home := t.TempDir()

	var out bytes.Buffer
	if err := run([]string{"install", "--repo", copyDir, "--home", home}, &out); err != nil {
		t.Fatalf("install from a plain copy: %v\n%s", err, out.String())
	}
	for _, never := range []string{"building sh-agent from", "would be rebuilt", "warning: go is not on PATH"} {
		if strings.Contains(out.String(), never) {
			t.Errorf("install from a plain copy printed %q:\n%s", never, out.String())
		}
	}
	if _, err := os.Stat(filepath.Join(copyDir, ".agents")); !os.IsNotExist(err) {
		t.Errorf("install created %s inside the mirror (err=%v)", filepath.Join(copyDir, ".agents"), err)
	}
	if _, err := os.Stat(filepath.Join(home, harness.BinPath)); err != nil {
		t.Errorf("binary not installed from the plain copy: %v", err)
	}

	out.Reset()
	if err := run([]string{"install", "--dry-run", "--repo", copyDir, "--home", home}, &out); err != nil {
		t.Fatalf("dry run from a plain copy: %v\n%s", err, out.String())
	}
	if strings.Contains(out.String(), "would be rebuilt") {
		t.Errorf("dry run from a plain copy announced a rebuild:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "[dry-run] plain copy") {
		t.Errorf("dry run from a plain copy must say so as a dry run:\n%s", out.String())
	}
}

// A hand-written ~/.claude/CLAUDE.md survives install, and status names it
// with where its content belongs, so the one-line import can replace it (R2).
func TestStatusNamesForeignClaudeMD(t *testing.T) {
	t.Setenv(harness.ChildEnv, "1")
	home := t.TempDir()
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	claudeMD := filepath.Join(home, harness.ClaudeRoot, "CLAUDE.md")
	if err := os.MkdirAll(filepath.Dir(claudeMD), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(claudeMD, []byte("my own notes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run([]string{"install", "--repo", repo, "--home", home}, &out); err != nil {
		t.Fatalf("install: %v\n%s", err, out.String())
	}
	out.Reset()
	if err := run([]string{"status", "--repo", repo, "--home", home}, &out); err != nil {
		t.Fatalf("status: %v\n%s", err, out.String())
	}
	want := "note       ~/.claude/CLAUDE.md is yours; move its content into a rule file under rules/ in the harness repo"
	if !strings.Contains(out.String(), want) {
		t.Errorf("status missing %q:\n%s", want, out.String())
	}
}
