package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime/debug"
	"strings"
	"syscall"
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
	want := "note       ~/.claude/CLAUDE.md is yours; move only rules meant for every machine into a rule file under rules/ in the harness repo — rules/ ships to every install and mirror, never credentials or personal or host-specific notes"
	if !strings.Contains(out.String(), want) {
		t.Errorf("status missing %q:\n%s", want, out.String())
	}
}

// Claude Code adds a Co-Authored-By trailer to every commit unless its own
// attribution setting is off; the context's Git rule alone did not stop it in
// the sandbox (three of three commits). install and status say so until the
// setting is there (settings.json is the operator's file: never written).
func TestAttributionNote(t *testing.T) {
	t.Setenv(harness.ChildEnv, "1")
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	// %s is the settings.json path under the scratch home: the note names the file it read.
	const body = `%s does not set "attribution": {"commit": ""}; Claude Code adds a Co-Authored-By trailer to every commit until it does (context § Git)`
	prefix := map[string]string{"install": "note: ", "status": "note       "}
	cases := []struct {
		name     string
		settings string // "" means no file; "DIR" makes settings.json a directory, unreadable as a file
		vendor   string // extra --vendor flag, "" for the default (all)
		want     string // the note body expected after the command's prefix; "" means no note line at all
	}{
		// install writes the key when it is absent (attributionSet), so the note is never printed for these two
		{"no settings file", "", "", ""},
		{"trailer switched off", `{"attribution": {"commit": "", "pr": ""}}` + "\n", "", ""},
		{"trailer switched off with false", `{"attribution": {"commit": false}}` + "\n", "", ""},
		{"custom trailer", `{"attribution": {"commit": "Co-Authored-By: me"}}` + "\n", "", body},
		{"other settings only", `{"permissions": {"allow": ["Bash(git:*)"]}}` + "\n", "", ""},
		{"antigravity only never notes", "", "agy", ""},
		{"malformed json is advisory, no echo", `{"attribution": ` + "\n", "", "%s is not valid JSON, so the attribution setting could not be checked; Claude Code adds"},
		{"a directory is advisory", "DIR", "", "%s is not a regular file, so the attribution setting could not be checked"},
		{"a fifo is advisory and never opened", "FIFO", "", "%s is not a regular file, so the attribution setting could not be checked"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			settingsPath := filepath.Join(home, harness.ClaudeRoot, "settings.json")
			if c.settings != "" {
				dir := filepath.Join(home, harness.ClaudeRoot)
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatal(err)
				}
				if c.settings == "DIR" {
					if err := os.Mkdir(filepath.Join(dir, "settings.json"), 0o755); err != nil {
						t.Fatal(err)
					}
				} else if c.settings == "FIFO" {
					// opening a FIFO with no writer blocks forever: the Stat guard must keep the command from opening it
					if err := syscall.Mkfifo(filepath.Join(dir, "settings.json"), 0o600); err != nil {
						t.Fatal(err)
					}
				} else if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(c.settings), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			for _, cmd := range []string{"install", "status"} {
				args := []string{cmd, "--repo", repo, "--home", home}
				if c.vendor != "" {
					args = append(args, "--vendor", c.vendor)
				}
				var out bytes.Buffer
				if err := run(args, &out); err != nil {
					t.Fatalf("%s: %v\n%s", cmd, err, out.String())
				}
				noted := strings.Contains(out.String(), prefix[cmd]+settingsPath)
				if c.want == "" && noted {
					t.Errorf("%s: unexpected settings.json note:\n%s", cmd, out.String())
				}
				want := prefix[cmd] + fmt.Sprintf(c.want, settingsPath)
				if c.want != "" && !strings.Contains(out.String(), want) {
					t.Errorf("%s: missing note %q:\n%s", cmd, want, out.String())
				}
				if strings.Contains(out.String(), "invalid character") || strings.Contains(out.String(), "unexpected end") {
					t.Errorf("%s: the note echoes the JSON error:\n%s", cmd, out.String())
				}
			}
		})
	}
}

// install writes Claude Code's attribution setting when the key is absent —
// the one vendor setting the harness owns (R4, ADR-025): the file is created
// when missing, every other byte of an existing file stays, a present key of
// any value is left alone, and nothing about the file fails the install.
func TestAttributionSet(t *testing.T) {
	t.Setenv(harness.ChildEnv, "1")
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	const fragment = `"attribution": {"commit": "", "pr": ""}`
	pretty := "{\n  \"model\": \"claude-fable-5-1[1m]\",\n  \"theme\": \"dark\",\n  \"autoMode\": {\n    \"environment\": [\"**Repository visibility**: Not queryable here — assume private\"]\n  }\n}\n"
	run1 := func(t *testing.T, home string, args ...string) string {
		t.Helper()
		var out bytes.Buffer
		if err := run(append([]string{"install", "--repo", repo, "--home", home}, args...), &out); err != nil {
			t.Fatalf("install: %v\n%s", err, out.String())
		}
		return out.String()
	}
	write := func(t *testing.T, home, content string, mode os.FileMode) string {
		t.Helper()
		dir := filepath.Join(home, harness.ClaudeRoot)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		p := filepath.Join(dir, "settings.json")
		if err := os.WriteFile(p, []byte(content), mode); err != nil {
			t.Fatal(err)
		}
		return p
	}
	keys := func(t *testing.T, data []byte) map[string]json.RawMessage {
		t.Helper()
		var m map[string]json.RawMessage
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("not a JSON object: %v\n%s", err, data)
		}
		return m
	}

	t.Run("missing file is created with only the key", func(t *testing.T) {
		home := t.TempDir()
		out := run1(t, home)
		p := filepath.Join(home, harness.ClaudeRoot, "settings.json")
		if !strings.Contains(out, "settings: "+p+" created with "+fragment) {
			t.Errorf("missing settings line:\n%s", out)
		}
		if strings.Contains(out, "note: "+p) {
			t.Errorf("note printed although the key was just written:\n%s", out)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		m := keys(t, data)
		if len(m) != 1 || string(m["attribution"]) != `{"commit": "", "pr": ""}` {
			t.Errorf("file holds %s", data)
		}
		if again := run1(t, home); strings.Contains(again, "settings:") {
			t.Errorf("second install wrote again:\n%s", again)
		}
	})

	t.Run("other keys, formatting and mode stay", func(t *testing.T) {
		home := t.TempDir()
		p := write(t, home, pretty, 0o600)
		out := run1(t, home)
		if !strings.Contains(out, "settings: "+p+" — "+fragment+" added; other keys untouched") {
			t.Errorf("missing settings line:\n%s", out)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		// everything before the closing brace of the original object is untouched, byte for byte
		prefix := strings.TrimRight(strings.TrimSuffix(strings.TrimRight(pretty, "\n"), "}"), " \n")
		if !strings.HasPrefix(string(data), prefix) {
			t.Errorf("original bytes changed:\n%s", data)
		}
		before, after := keys(t, []byte(pretty)), keys(t, data)
		for k, v := range before {
			if string(after[k]) != string(v) {
				t.Errorf("key %q changed: %s -> %s", k, v, after[k])
			}
		}
		if len(after) != len(before)+1 || string(after["attribution"]) != `{"commit": "", "pr": ""}` {
			t.Errorf("attribution not added exactly once: %s", data)
		}
		if !strings.HasSuffix(string(data), "}\n") {
			t.Errorf("trailing newline lost: %q", data[len(data)-3:])
		}
		info, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Errorf("mode changed to %v", info.Mode().Perm())
		}
	})

	t.Run("crlf file keeps its line endings", func(t *testing.T) {
		home := t.TempDir()
		p := write(t, home, "{\r\n  \"model\": \"x\"\r\n}\r\n", 0o644)
		run1(t, home)
		data, _ := os.ReadFile(p)
		if strings.Count(string(data), "\n") != strings.Count(string(data), "\r\n") {
			t.Errorf("mixed line endings: %q", data)
		}
		if m := keys(t, data); string(m["model"]) != `"x"` || m["attribution"] == nil {
			t.Errorf("file holds %s", data)
		}
	})

	t.Run("empty object", func(t *testing.T) {
		home := t.TempDir()
		p := write(t, home, "{}\n", 0o644)
		run1(t, home)
		data, _ := os.ReadFile(p)
		m := keys(t, data)
		if len(m) != 1 || m["attribution"] == nil {
			t.Errorf("file holds %s", data)
		}
	})

	for _, c := range []struct{ name, content, line string }{
		{"custom value is left alone", `{"attribution": {"commit": "Co-Authored-By: me"}}` + "\n", "note: "},
		{"off value is left alone", `{"attribution": {"commit": ""}}` + "\n", ""},
		{"malformed is left alone", `{"attribution": ` + "\n", "note: "},
		{"not an object is left alone", `["x"]` + "\n", "note: "},
		{"null is left alone", "null\n", "note: "},
	} {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			p := write(t, home, c.content, 0o644)
			out := run1(t, home)
			data, _ := os.ReadFile(p)
			if string(data) != c.content {
				t.Errorf("file changed:\n%s", data)
			}
			if strings.Contains(out, "settings: "+p+" —") || strings.Contains(out, "created with") {
				t.Errorf("a write was reported:\n%s", out)
			}
			if c.line != "" && !strings.Contains(out, c.line+p) {
				t.Errorf("expected a %q line for %s:\n%s", c.line, p, out)
			}
			if c.line == "" && strings.Contains(out, p) {
				t.Errorf("unexpected line about the file:\n%s", out)
			}
		})
	}

	t.Run("a file over the read bound is left alone", func(t *testing.T) {
		home := t.TempDir()
		big := `{"pad": "` + strings.Repeat("x", 1<<20) + `"}` + "\n"
		p := write(t, home, big, 0o644)
		out := run1(t, home)
		if !strings.Contains(out, "settings: "+p+" is larger than 1 MiB") {
			t.Errorf("missing size line:\n%s", out)
		}
		data, _ := os.ReadFile(p)
		if string(data) != big {
			t.Errorf("file changed (len %d)", len(data))
		}
	})

	t.Run("a symlinked settings.json is never written through", func(t *testing.T) {
		home := t.TempDir()
		target := filepath.Join(t.TempDir(), "dotfiles-settings.json")
		if err := os.WriteFile(target, []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		dir := filepath.Join(home, harness.ClaudeRoot)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		p := filepath.Join(dir, "settings.json")
		if err := os.Symlink(target, p); err != nil {
			t.Fatal(err)
		}
		out := run1(t, home)
		if !strings.Contains(out, "settings: "+p+" is a symlink; add the key to its target by hand") {
			t.Errorf("missing symlink line:\n%s", out)
		}
		if info, err := os.Lstat(p); err != nil || info.Mode()&os.ModeSymlink == 0 {
			t.Errorf("the symlink was replaced: %v %v", info, err)
		}
		if data, _ := os.ReadFile(target); string(data) != "{}\n" {
			t.Errorf("the target was written: %s", data)
		}
	})

	t.Run("dry run writes nothing", func(t *testing.T) {
		home := t.TempDir()
		out := run1(t, home, "--dry-run")
		p := filepath.Join(home, harness.ClaudeRoot, "settings.json")
		if _, err := os.Stat(p); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("dry run created %s", p)
		}
		if !strings.Contains(out, "[dry-run] would create "+p) {
			t.Errorf("missing dry-run line:\n%s", out)
		}
	})

	t.Run("antigravity only writes nothing", func(t *testing.T) {
		home := t.TempDir()
		out := run1(t, home, "--vendor", "agy")
		if _, err := os.Stat(filepath.Join(home, harness.ClaudeRoot, "settings.json")); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("settings.json created for the agy vendor")
		}
		if strings.Contains(out, "settings:") {
			t.Errorf("settings line for the agy vendor:\n%s", out)
		}
	})

	t.Run("status alone keeps the note", func(t *testing.T) {
		home := t.TempDir()
		run1(t, home) // a full install writes the key …
		p := filepath.Join(home, harness.ClaudeRoot, "settings.json")
		if err := os.Remove(p); err != nil { // … and an operator can delete the file again
			t.Fatal(err)
		}
		var out bytes.Buffer
		if err := run([]string{"status", "--repo", repo, "--home", home}, &out); err != nil {
			t.Fatalf("status: %v\n%s", err, out.String())
		}
		if !strings.Contains(out.String(), "note       "+filepath.Join(home, harness.ClaudeRoot, "settings.json")+" does not set") {
			t.Errorf("status lost the note:\n%s", out.String())
		}
	})
}
