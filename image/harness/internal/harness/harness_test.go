package harness

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// newRepo builds a minimal harness checkout: two skills, one agent, two
// context files. Tests never touch the real $HOME.
func newRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	write := func(rel, content string) {
		t.Helper()
		p := filepath.Join(repo, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("context/AGENTS.md", "# AGENTS.md\n\nBe honest.\n")
	write("context/dev-setup.md", "## Environment & Stack\n\n- GCP only.\n")
	write("skills/alpha/SKILL.md", "---\nname: alpha\ndescription: |\n  Alpha does things.\n  Use it for alpha work.\nmetadata:\n  version: \"1\"\n---\nbody\n")
	write("skills/alpha/references/notes.md", "notes\n")
	write("skills/beta/SKILL.md", "---\nname: beta\ndescription: \"Beta: slash only\"\ndisable-model-invocation: true\n---\nbody\n")
	write("skills/VENDORED.md", "not a skill\n")
	write("commands/deploy.md", "---\ndescription: Deploy the thing\n---\nSteps\n")
	write("rules/go-style.md", "# Go style\n\nUse slog.\n")
	write("agents/reviewer.md", "---\nname: reviewer\ndescription: Reviews diffs\ntools: Read, Grep, Bash\nmodel: opus\nskills: review-standards\neffort: high\n---\nLoad ~/.claude/skills/review-standards first.\n")
	return repo
}

func TestParseFrontmatter(t *testing.T) {
	front := "name: x\ndescription: |\n  line one\n  line two\nquoted: \"a: b\"\nmetadata:\n  version: \"1.2\"\n  author: me\ntools: [Read, Grep]\nskills:\n  - alpha\n  - beta\nplain: first\n  second\nempty:\n"
	fm := Parse(front)
	want := map[string]string{
		"name":        "x",
		"description": "line one line two",
		"quoted":      "a: b",
		"tools":       "[Read, Grep]",
		"skills":      "[alpha, beta]",
		"plain":       "first second",
		"empty":       "",
	}
	for k, v := range want {
		if fm[k] != v {
			t.Errorf("%s = %q, want %q", k, fm[k], v)
		}
	}
	if _, ok := fm["metadata"]; ok {
		t.Errorf("nested map should be skipped, got %q", fm["metadata"])
	}
	if got := SplitList("[Read, Grep]"); strings.Join(got, ",") != "Read,Grep" {
		t.Errorf("SplitList brackets = %v", got)
	}
	if got := SplitList("Read Grep"); strings.Join(got, ",") != "Read,Grep" {
		t.Errorf("SplitList spaces = %v", got)
	}
}

func TestLintRejectsUnquotedComment(t *testing.T) {
	// A plain scalar with " #" is a YAML comment start: strict parsers
	// truncate it or, inside a flow sequence, fail the whole file. The
	// Antigravity CLI dropped the `bug` and `feature` skills that way.
	bad := []string{
		"name: x\nargument-hint: [feature description or #issue]\n",
		"name: x\ndescription: fixes #1 and #2\n",
		"name: x\ndescription: #comment only\n",
		"name: x\ndescription: first line\n  wraps with #issue\n",
		"name: x\ndescription: value\t#tab-comment\n",
		"name: x\ndescription: \"quoted\" # trailing\n",
	}
	for _, front := range bad {
		if err := Lint(front); err == nil {
			t.Errorf("Lint accepted %q", front)
		} else if !strings.Contains(err.Error(), "#") {
			t.Errorf("error should name the marker: %v", err)
		}
	}
	good := []string{
		"name: x\nargument-hint: \"[feature description or #issue]\"\n",
		"name: x\nargument-hint: '[#issue]'\n",
		"name: x\ndescription: |\n  a #b\n  c #d\nmetadata:\n  tag: \"#1\"\n",
		"name: x\ntools: [Read, Grep]\ncolor: \"#ff0000\"\n",
	}
	for _, front := range good {
		if err := Lint(front); err != nil {
			t.Errorf("Lint rejected %q: %v", front, err)
		}
	}
}

func TestLoadSkillsRejectsUnquotedComment(t *testing.T) {
	repo := newRepo(t)
	p := filepath.Join(repo, "skills", "gamma", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("---\nname: gamma\ndescription: Gamma\nargument-hint: [topic or #issue]\n---\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadSkills(repo)
	if err == nil || !strings.Contains(err.Error(), "gamma") || !strings.Contains(err.Error(), "argument-hint") {
		t.Fatalf("LoadSkills should name the skill and the key, got %v", err)
	}
}

func TestSplitRejectsMissingFrontmatter(t *testing.T) {
	if _, _, err := Split("# no frontmatter\n"); err == nil {
		t.Fatal("expected an error")
	}
	if _, _, err := Split("---\nname: x\n"); err == nil {
		t.Fatal("expected an error for an unterminated block")
	}
}

func TestDeriveAgy(t *testing.T) {
	repo := newRepo(t)
	agents, err := LoadAgents(repo)
	if err != nil {
		t.Fatal(err)
	}
	out, err := DeriveAgy(agents[0])
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, want := range []string{
		"name: reviewer\n",
		"description: \"Reviews diffs\"\n",
		"subagent: true\n",
		"mainAgent: false\n",
		"model: pro\n",
		"tools: [view_file, grep_search, run_command]\n",
		"~/.gemini/config/skills/review-standards",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("derived agent missing %q:\n%s", want, s)
		}
	}
	for _, unwanted := range []string{"effort:", "skills:", "~/.claude/"} {
		if strings.Contains(s, unwanted) {
			t.Errorf("derived agent should not contain %q:\n%s", unwanted, s)
		}
	}

	bad := agents[0]
	bad.Front = Frontmatter{"tools": "Read, WebSearch"}
	if _, err := DeriveAgy(bad); err == nil {
		t.Fatal("unmapped tool should be an error")
	}
}

func TestInventoryAndRender(t *testing.T) {
	repo := newRepo(t)
	skills, err := LoadSkills(repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 2 || skills[0].Name != "alpha" || skills[1].Name != "beta" {
		t.Fatalf("skills = %+v", skills)
	}
	if !skills[1].SlashOnly || skills[0].SlashOnly {
		t.Errorf("SlashOnly wrong: %+v", skills)
	}
	inv, err := LoadInventory(repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(inv.Commands) != 1 || inv.Commands[0].Name != "deploy" || inv.Commands[0].Description != "Deploy the thing" {
		t.Errorf("commands = %+v", inv.Commands)
	}
	if len(inv.Rules) != 1 || inv.Rules[0].Description != "Go style" {
		t.Errorf("rules = %+v", inv.Rules)
	}
	ctx, err := RenderContext(repo, inv)
	if err != nil {
		t.Fatal(err)
	}
	s := string(ctx)
	if !strings.HasPrefix(s, Marker+"\n# AGENTS.md") {
		t.Errorf("context should start with the marker and AGENTS.md:\n%s", s[:80])
	}
	for _, want := range []string{"## Environment & Stack", "`alpha`", "`/beta`", "`reviewer` (opus)", "`/deploy`", "`go-style`"} {
		if !strings.Contains(s, want) {
			t.Errorf("context missing %q", want)
		}
	}
	if strings.Contains(s, "Alpha does things") {
		t.Errorf("deployed context should carry names only, not descriptions")
	}
	full := RenderInventory(inv)
	for _, want := range []string{"| `alpha` | auto |", "| `beta` | `/beta` |", "| `reviewer` | opus |", "| `/deploy` |", "| `go-style` |", "Beta: slash only"} {
		if !strings.Contains(full, want) {
			t.Errorf("full inventory missing %q", want)
		}
	}
}

func TestInstallStatusResyncUninstall(t *testing.T) {
	repo := newRepo(t)
	home := t.TempDir()
	self := []byte("#!/bin/sh\necho sh-agent\n")

	// A CLAUDE.md the operator wrote must survive an install.
	claudeMD := filepath.Join(home, ClaudeRoot, "CLAUDE.md")
	if err := os.MkdirAll(filepath.Dir(claudeMD), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(claudeMD, []byte("my own notes\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	files, err := Plan(repo, self, VendorAll)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Install(repo, home, files, VendorAll, false, &out); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{
		".claude/AGENTS.md",
		".claude/skills/alpha/SKILL.md",
		".claude/skills/alpha/references/notes.md",
		".claude/skills/beta/SKILL.md",
		".claude/agents/reviewer.md",
		".gemini/config/rules/AGENTS.md",
		".gemini/config/skills/alpha/SKILL.md",
		".gemini/config/agents/reviewer/agent.md",
		".claude/commands/deploy.md",
		".gemini/config/workflows/deploy.md",
		".claude/rules/go-style.md",
		".gemini/config/rules/go-style.md",
		".local/bin/sh-agent",
		".config/sh-agent/state.json",
	} {
		if _, err := os.Stat(filepath.Join(home, rel)); err != nil {
			t.Errorf("expected %s after install: %v", rel, err)
		}
	}
	if got, _ := os.ReadFile(claudeMD); string(got) != "my own notes\n" {
		t.Errorf("foreign CLAUDE.md was overwritten: %q", got)
	}
	if !strings.Contains(out.String(), "keep    ~/.claude/CLAUDE.md") {
		t.Errorf("install should report the kept file:\n%s", out.String())
	}
	if info, _ := os.Stat(filepath.Join(home, BinPath)); info.Mode().Perm()&0o100 == 0 {
		t.Errorf("binary should be executable, mode %v", info.Mode())
	}

	report, err := Status(home, files)
	if err != nil {
		t.Fatal(err)
	}
	if report.Drift() || report.Counts["foreign"] != 1 {
		t.Errorf("fresh install should be clean apart from the foreign CLAUDE.md: %+v", report.Counts)
	}
	// status and diff plan without the binary; the installed one is not stale.
	noSelf, err := Plan(repo, nil, VendorAll)
	if err != nil {
		t.Fatal(err)
	}
	if report, _ := Status(home, noSelf); report.Drift() {
		t.Errorf("installed binary must not count as stale: %+v", report.Counts)
	}

	// Edit a deployed file, delete another: both show up as drift.
	if err := os.WriteFile(filepath.Join(home, ".claude/skills/alpha/SKILL.md"), []byte("edited"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(home, ".gemini/config/agents/reviewer/agent.md")); err != nil {
		t.Fatal(err)
	}
	report, _ = Status(home, files)
	if report.Counts["differs"] != 1 || report.Counts["missing"] != 1 {
		t.Errorf("drift not detected: %+v", report.Counts)
	}

	// Remove a skill from the repo and reinstall: the deployed copies go away.
	if err := os.RemoveAll(filepath.Join(repo, "skills", "beta")); err != nil {
		t.Fatal(err)
	}
	files, err = Plan(repo, self, VendorAll)
	if err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := Install(repo, home, files, VendorAll, false, &out); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{".claude/skills/beta", ".gemini/config/skills/beta"} {
		if _, err := os.Stat(filepath.Join(home, rel)); err == nil {
			t.Errorf("stale %s should have been removed", rel)
		}
	}
	if !strings.Contains(out.String(), "2 removed") {
		t.Errorf("resync should report removals:\n%s", out.String())
	}
	report, _ = Status(home, files)
	if report.Drift() {
		t.Errorf("after resync there should be no drift: %+v", report.Counts)
	}

	// Dry-run changes nothing.
	if err := Uninstall(home, true, &out); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude/AGENTS.md")); err != nil {
		t.Errorf("dry-run uninstall removed a file")
	}

	if err := Uninstall(home, false, &out); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{".claude/AGENTS.md", ".claude/skills", ".claude/agents", ".gemini/config/skills", ".gemini/config/rules", ".local/bin/sh-agent", ".config/sh-agent"} {
		if _, err := os.Lstat(filepath.Join(home, rel)); err == nil {
			t.Errorf("%s should be gone after uninstall", rel)
		}
	}
	for _, rel := range []string{".claude", ".gemini/config", ".claude/CLAUDE.md"} {
		if _, err := os.Lstat(filepath.Join(home, rel)); err != nil {
			t.Errorf("%s must survive uninstall: %v", rel, err)
		}
	}
	if got, _ := os.ReadFile(claudeMD); string(got) != "my own notes\n" {
		t.Errorf("foreign CLAUDE.md touched by uninstall: %q", got)
	}
}

func TestInstallWritesOwnClaudeStub(t *testing.T) {
	repo := newRepo(t)
	home := t.TempDir()
	files, err := Plan(repo, nil, VendorAll)
	if err != nil {
		t.Fatal(err)
	}
	if err := Install(repo, home, files, VendorAll, false, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(home, ClaudeRoot, "CLAUDE.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !IsOurs(got) || !strings.Contains(string(got), "\n@AGENTS.md\n") {
		t.Errorf("stub = %q", got)
	}
}

func TestVendorSelection(t *testing.T) {
	// NormalizeVendor checks
	for _, in := range []string{"", "all", "ALL", " all "} {
		if got, err := NormalizeVendor(in); err != nil || got != VendorAll {
			t.Errorf("NormalizeVendor(%q) = %q, err %v", in, got, err)
		}
	}
	for _, in := range []string{"antigravity", "Antigravity", "agy", "gemini"} {
		if got, err := NormalizeVendor(in); err != nil || got != VendorAntigravity {
			t.Errorf("NormalizeVendor(%q) = %q, err %v", in, got, err)
		}
	}
	for _, in := range []string{"claude", "Claude", "claudecode"} {
		if got, err := NormalizeVendor(in); err != nil || got != VendorClaude {
			t.Errorf("NormalizeVendor(%q) = %q, err %v", in, got, err)
		}
	}
	if _, err := NormalizeVendor("invalid-vendor"); err == nil {
		t.Error("NormalizeVendor should reject unknown vendor")
	}

	repo := newRepo(t)
	self := []byte("#!/bin/sh\necho sh-agent\n")

	// Plan for antigravity only
	agyPlan, err := Plan(repo, self, VendorAntigravity)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range agyPlan {
		if strings.HasPrefix(f.Rel, ClaudeRoot) {
			t.Errorf("antigravity plan contains Claude file %s", f.Rel)
		}
	}
	foundAgyAgents := false
	for _, f := range agyPlan {
		if f.Rel == filepath.Join(AgyRoot, "rules", "AGENTS.md") {
			foundAgyAgents = true
		}
	}
	if !foundAgyAgents {
		t.Error("antigravity plan missing rules/AGENTS.md")
	}

	// Plan for claude only
	claudePlan, err := Plan(repo, self, VendorClaude)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range claudePlan {
		if strings.HasPrefix(f.Rel, AgyRoot) {
			t.Errorf("claude plan contains Antigravity file %s", f.Rel)
		}
	}

	// Install antigravity-only to clean home
	home := t.TempDir()
	var out bytes.Buffer
	if err := Install(repo, home, agyPlan, VendorAntigravity, false, &out); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, ClaudeRoot)); err == nil {
		t.Errorf(".claude should not exist after antigravity-only install")
	}
	if _, err := os.Stat(filepath.Join(home, AgyRoot, "rules", "AGENTS.md")); err != nil {
		t.Errorf("expected antigravity rules/AGENTS.md to exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, BinPath)); err != nil {
		t.Errorf("expected binary to be installed: %v", err)
	}

	// Verify state records vendor
	st, found, err := LoadState(home)
	if err != nil || !found {
		t.Fatalf("LoadState failed: %v, found: %v", err, found)
	}
	if st.Vendor != VendorAntigravity {
		t.Errorf("state vendor = %q, want %q", st.Vendor, VendorAntigravity)
	}

	// Status check with antigravity plan has no drift
	noSelfAgy, err := Plan(repo, nil, VendorAntigravity)
	if err != nil {
		t.Fatal(err)
	}
	report, err := Status(home, noSelfAgy)
	if err != nil {
		t.Fatal(err)
	}
	if report.Drift() {
		t.Errorf("antigravity status has drift: %+v", report.Counts)
	}

	// Switch from antigravity to all, then back to antigravity:
	// Verify resync removes Claude files when returning to antigravity
	allPlan, err := Plan(repo, self, VendorAll)
	if err != nil {
		t.Fatal(err)
	}
	if err := Install(repo, home, allPlan, VendorAll, false, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, ClaudeRoot, "AGENTS.md")); err != nil {
		t.Fatalf("expected .claude/AGENTS.md after installing all: %v", err)
	}

	// Now switch back to antigravity
	out.Reset()
	if err := Install(repo, home, agyPlan, VendorAntigravity, false, &out); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, ClaudeRoot, "AGENTS.md")); err == nil {
		t.Errorf(".claude/AGENTS.md should have been removed by resync when switching back to antigravity")
	}
	st, _, err = LoadState(home)
	if err != nil {
		t.Fatal(err)
	}
	if st.Vendor != VendorAntigravity {
		t.Errorf("state vendor = %q, want %q", st.Vendor, VendorAntigravity)
	}
}

func TestAdopt(t *testing.T) {
	repo := newRepo(t)
	home := t.TempDir()
	src := filepath.Join(home, ".agents", "skills", "gamma")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "SKILL.md"), []byte("---\nname: gamma\ndescription: g\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A symlink the way `npx skills add -g` leaves one for Claude Code.
	link := filepath.Join(home, ClaudeRoot, "skills", "gamma")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(src, link); err != nil {
		t.Fatal(err)
	}

	dest, err := Adopt(repo, home, "gamma", false)
	if err != nil {
		t.Fatal(err)
	}
	if dest != filepath.Join(repo, "skills", "gamma") {
		t.Errorf("dest = %s", dest)
	}
	if _, err := os.Stat(filepath.Join(dest, "SKILL.md")); err != nil {
		t.Error(err)
	}
	if _, err := Adopt(repo, home, "gamma", false); err == nil {
		t.Error("second adopt without --force should fail")
	}
	if _, err := Adopt(repo, home, "gamma", true); err != nil {
		t.Errorf("adopt --force: %v", err)
	}
	if _, err := Adopt(repo, home, "nope", false); err == nil {
		t.Error("unknown skill should fail")
	}
}

func TestFindRepo(t *testing.T) {
	repo := newRepo(t)
	home := t.TempDir()
	if _, err := FindRepo("", home); err == nil {
		t.Skip("working directory happens to be inside a harness repo")
	}
	got, err := FindRepo(repo, home)
	if err != nil || got != repo {
		t.Errorf("explicit repo: %s, %v", got, err)
	}
	if err := SaveState(home, State{Repo: repo}); err != nil {
		t.Fatal(err)
	}
	if got, err := FindRepo("", home); err != nil || got != repo {
		t.Errorf("repo from state: %s, %v", got, err)
	}
}

func TestBehindAndPull(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	base := t.TempDir()
	run := func(dir string, args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	origin := filepath.Join(base, "origin.git")
	run(base, "init", "--bare", "-q", "-b", "main", origin)

	a := filepath.Join(base, "a")
	run(base, "clone", "-q", origin, a)
	if err := os.WriteFile(filepath.Join(a, "f"), []byte("1"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(a, "add", "f")
	run(a, "commit", "-q", "-m", "one")
	run(a, "push", "-q", "-u", "origin", "main")

	b := filepath.Join(base, "b")
	run(base, "clone", "-q", origin, b)

	if n, err := Behind(b); err != nil || n != 0 {
		t.Fatalf("fresh clone behind = %d, %v", n, err)
	}

	if err := os.WriteFile(filepath.Join(a, "f"), []byte("2"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(a, "commit", "-q", "-am", "two")
	run(a, "push", "-q")

	if n, err := Behind(b); err != nil || n != 1 {
		t.Fatalf("behind = %d, %v; want 1", n, err)
	}

	if err := os.WriteFile(filepath.Join(b, "f"), []byte("dirty"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Pull(b, &bytes.Buffer{}); err == nil {
		t.Fatal("pull over a dirty tree must be refused")
	}
	run(b, "checkout", "--", "f")
	if err := Pull(b, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if n, _ := Behind(b); n != 0 {
		t.Errorf("after pull behind = %d", n)
	}
}

// TestNestedCopyIsNotACheckout: a harness-shaped tree inside another git
// repository must not inherit that repository's branch, commit, or dirty
// state, and git-driven update must refuse it.
func TestNestedCopyIsNotACheckout(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := newRepo(t)
	gitIn(t, root, "init", "-q")
	gitIn(t, root, "add", ".")
	gitIn(t, root, "commit", "-q", "-m", "fixture")

	nested := filepath.Join(root, "mirror")
	for _, rel := range []string{"context", "skills"} {
		if err := os.MkdirAll(filepath.Join(nested, rel), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(nested, "context", "AGENTS.md"), []byte("# copy\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !IsRepo(nested) {
		t.Fatal("fixture: nested copy should look like a harness")
	}

	if got := Inspect(nested); got != (RepoInfo{}) {
		t.Errorf("nested copy Inspect = %+v, want empty", got)
	}
	if _, err := Behind(nested); !errors.Is(err, errPlainCopy) {
		t.Errorf("nested copy Behind err = %v, want %v", err, errPlainCopy)
	}
	if err := Pull(nested, &bytes.Buffer{}); !errors.Is(err, errPlainCopy) {
		t.Errorf("nested copy Pull err = %v, want %v", err, errPlainCopy)
	}

	// An inherited GIT_DIR/GIT_WORK_TREE must not redirect the check to the
	// enclosing repository: the check looks for .git itself and never asks git.
	t.Setenv("GIT_DIR", filepath.Join(root, ".git"))
	t.Setenv("GIT_WORK_TREE", nested)
	if got := Inspect(nested); got != (RepoInfo{}) {
		t.Errorf("nested copy with GIT_DIR/GIT_WORK_TREE set: Inspect = %+v, want empty", got)
	}
	if _, err := Behind(nested); !errors.Is(err, errPlainCopy) {
		t.Errorf("nested copy with GIT_DIR set: Behind err = %v, want %v", err, errPlainCopy)
	}
	t.Setenv("GIT_DIR", "")
	t.Setenv("GIT_WORK_TREE", "")
	os.Unsetenv("GIT_DIR")
	os.Unsetenv("GIT_WORK_TREE")

	got := Inspect(root)
	if got.Branch == "" || got.Commit == "" || !got.Dirty {
		t.Errorf("repo root Inspect = %+v, want branch, commit, and dirty (the untracked mirror)", got)
	}
	if _, err := Behind(root); errors.Is(err, errPlainCopy) {
		t.Errorf("repo root Behind must not be refused as a plain copy: %v", err)
	}
}

func TestLastRetro(t *testing.T) {
	home := t.TempDir()
	if _, ok := LastRetro(home); ok {
		t.Fatal("no proposals yet")
	}
	dir := filepath.Join(home, ".config", "sh-agent", "proposals")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "2026-09-03-retro.md"), []byte("# retro\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	first, ok := LastRetro(home)
	if !ok {
		t.Fatal("proposal should be found")
	}
	// A project-level retro appends to the index without a proposal file;
	// that counts as a retro too, and its newer mtime wins.
	index := filepath.Join(home, ".config", "sh-agent", "retros.jsonl")
	if err := os.WriteFile(index, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	later := first.Add(48 * time.Hour)
	if err := os.Chtimes(index, later, later); err != nil {
		t.Fatal(err)
	}
	if got, ok := LastRetro(home); !ok || !got.Equal(later) {
		t.Fatalf("LastRetro = %v, %v; want %v", got, ok, later)
	}
}

func TestSlashNoteOnlyWhenSomeSkillIsSlashOnly(t *testing.T) {
	repo := newRepo(t)
	inv, err := LoadInventory(repo)
	if err != nil {
		t.Fatal(err)
	}
	const marked = "Skills marked `/name` are typed by the user"
	if !strings.Contains(RenderInventoryCompact(inv), marked) || !strings.Contains(RenderInventory(inv), marked) {
		t.Errorf("fixture has a slash-only skill; the note should be printed")
	}
	for i := range inv.Skills {
		inv.Skills[i].SlashOnly = false
	}
	for _, out := range []string{RenderInventoryCompact(inv), RenderInventory(inv)} {
		if strings.Contains(out, marked) {
			t.Errorf("no slash-only skill; the note must not be printed:\n%s", out)
		}
		if !strings.Contains(out, "can also be typed as `/name`") {
			t.Errorf("expected the all-model-invocable sentence:\n%s", out)
		}
	}
}

// gitIn runs git in dir and fails the test on error.
func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	outb, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, outb)
	}
	return strings.TrimSpace(string(outb))
}

func TestFindRepoRefusesNamedNonRepo(t *testing.T) {
	home := t.TempDir()
	bogus := filepath.Join(t.TempDir(), "not-a-checkout")
	if _, err := FindRepo(bogus, home); err == nil {
		t.Error("an explicit path that is not a checkout must be an error, not a fallback")
	}
	t.Setenv("SH_AGENT_REPO", bogus)
	if _, err := FindRepo("", home); err == nil {
		t.Error("$SH_AGENT_REPO pointing at a non-checkout must be an error, not a fallback")
	}
	t.Setenv("SH_AGENT_REPO", "")
	repo := newRepo(t)
	if got, err := FindRepo(repo, home); err != nil || got != repo {
		t.Errorf("explicit checkout: %q, %v", got, err)
	}
}

func TestInstallMarksDirtyTree(t *testing.T) {
	repo := newRepo(t)
	gitIn(t, repo, "init", "-q")
	gitIn(t, repo, "add", ".")
	gitIn(t, repo, "commit", "-q", "-m", "fixture")
	short := gitIn(t, repo, "rev-parse", "--short", "HEAD")
	home := t.TempDir()
	files, err := Plan(repo, nil, VendorAll)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Install(repo, home, files, VendorAll, false, &out); err != nil {
		t.Fatal(err)
	}
	st, _, err := LoadState(home)
	if err != nil {
		t.Fatal(err)
	}
	if st.Commit != short {
		t.Errorf("clean tree: commit = %q, want %q", st.Commit, short)
	}
	if err := os.WriteFile(filepath.Join(repo, "context", "AGENTS.md"), []byte("# AGENTS.md\n\nBe honest and brief.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	files, err = Plan(repo, nil, VendorAll)
	if err != nil {
		t.Fatal(err)
	}
	if err := Install(repo, home, files, VendorAll, false, &out); err != nil {
		t.Fatal(err)
	}
	st, _, err = LoadState(home)
	if err != nil {
		t.Fatal(err)
	}
	if st.Commit != short+"+dirty" {
		t.Errorf("dirty tree: commit = %q, want %q", st.Commit, short+"+dirty")
	}
}

// TestReinstallBuildsAndInstalls runs the real rebuild path against this
// checkout: go build into the repo's scratch space, then that build installs
// into a scratch home and copies itself there, stamped with the revision.
func TestReinstallBuildsAndInstalls(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not on PATH")
	}
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	var out bytes.Buffer
	if err := Reinstall(repo, []string{"--home", home}, &out); err != nil {
		t.Fatalf("Reinstall: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "files managed") {
		t.Errorf("child install output missing:\n%s", out.String())
	}
	bin := filepath.Join(home, BinPath)
	ver, err := exec.Command(bin, "version").Output()
	if err != nil {
		t.Fatalf("installed binary does not run: %v", err)
	}
	if v := strings.TrimSpace(string(ver)); strings.Contains(v, "(devel)") || strings.Contains(v, "unknown") {
		t.Errorf("installed binary is unstamped: %q", v)
	}
	builds, _ := filepath.Glob(filepath.Join(repo, ".agents", "scratch", "sh-agent-build-*"))
	if len(builds) != 0 {
		t.Errorf("build directories left behind: %v", builds)
	}
}

// TestNoTmpInHarnessProse guards the scratch rule at the source: the
// context, the agents, the skills, and the repo docs may mention /tmp only
// to forbid it. Three records (BUG-003, BUG-007, BUG-014) are the reason
// this is a test and not a reminder.
func TestNoTmpInHarnessProse(t *testing.T) {
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	roots := []string{"context", "agents", "README.md", "AGENTS.md"}
	skills, _ := filepath.Glob(filepath.Join(repo, "skills", "*", "SKILL.md"))
	for _, s := range skills {
		rel, _ := filepath.Rel(repo, s)
		roots = append(roots, rel)
	}
	allowed := func(line string) bool {
		l := strings.ToLower(line)
		return strings.Contains(l, "never /tmp") || strings.Contains(l, "never `/tmp`") ||
			strings.Contains(l, "not /tmp") || strings.Contains(l, "not `/tmp`") || strings.Contains(l, "instead of `/tmp`")
	}
	for _, root := range roots {
		err := filepath.WalkDir(filepath.Join(repo, root), func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".md") {
				return err
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for i, line := range strings.Split(string(data), "\n") {
				if strings.Contains(line, "/tmp") && !allowed(line) {
					rel, _ := filepath.Rel(repo, path)
					t.Errorf("%s:%d mentions /tmp outside a 'never /tmp' clause: %s", rel, i+1, strings.TrimSpace(line))
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
