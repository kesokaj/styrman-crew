package harness

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
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

	files, err := Plan(repo, self)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Install(repo, home, files, false, &out); err != nil {
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
	noSelf, err := Plan(repo, nil)
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
	files, err = Plan(repo, self)
	if err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := Install(repo, home, files, false, &out); err != nil {
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
	files, err := Plan(repo, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := Install(repo, home, files, false, &bytes.Buffer{}); err != nil {
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
	if _, ok := LastRetro(home); !ok {
		t.Fatal("proposal should be found")
	}
}
