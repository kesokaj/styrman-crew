package harness

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, p, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A frontmatter `name:` becomes a path component under ~/.claude and the agy
// mirror (plan.go); the parse boundary refuses anything that is not one
// local segment, naming the file, so `../x` or `a/b` never reaches
// filepath.Join (styrman-crew's 2026-09-14 review of the mirror refresh).
func TestSkillAndAgentNamesMustBeOneLocalSegment(t *testing.T) {
	for _, tc := range []struct {
		name string
		bad  bool
	}{
		{"ok-name", false},
		{"../evil", true},
		{"a/b", true},
		{"/abs", true},
		{"..", true},
		{`a\b`, true},
		{"a b", true},
		{"a`b", true},
		{"café", true},
		{".hidden", true},
		{"v1.2_x-y", false},
	} {
		repo := t.TempDir()
		writeFile(t, filepath.Join(repo, "skills", "dir", "SKILL.md"),
			"---\nname: "+tc.name+"\ndescription: \"a skill\"\n---\nbody\n")
		writeFile(t, filepath.Join(repo, "agents", "file.md"),
			"---\nname: "+tc.name+"\ndescription: \"an agent\"\n---\nbody\n")
		_, serr := LoadSkills(repo)
		_, aerr := LoadAgents(repo)
		if tc.bad {
			if serr == nil || !strings.Contains(serr.Error(), "dir/SKILL.md") {
				t.Errorf("name %q: skills loader err = %v, want a refusal naming dir/SKILL.md", tc.name, serr)
			}
			if aerr == nil || !strings.Contains(aerr.Error(), "file.md") {
				t.Errorf("name %q: agents loader err = %v, want a refusal naming file.md", tc.name, aerr)
			}
			continue
		}
		if serr != nil || aerr != nil {
			t.Errorf("name %q: skills err = %v, agents err = %v, want none", tc.name, serr, aerr)
		}
	}
}

// The saved manifest lives under the user's home and is read back on the
// next install to remove what the new plan no longer contains. An entry
// that is not local to the install root is skipped, never followed: the
// resync removes a stale planned-less file inside the home and leaves a
// `../` entry's target untouched.
func TestResyncNeverFollowsANonLocalManifestEntry(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	stale := filepath.Join(home, ".claude", "agents", "old.md")
	writeFile(t, stale, "stale\n")
	outside := filepath.Join(root, "escape.txt")
	writeFile(t, outside, "must survive\n")
	if err := SaveState(home, State{Repo: "r", Files: []string{".claude/agents/old.md", "../escape.txt"}}); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Install("r", home, nil, "", false, &out); err != nil {
		t.Fatalf("install with an empty plan: %v\n%s", err, out.String())
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("stale planned-less file not removed: err = %v", err)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Errorf("the ../ manifest entry's target was touched: %v\n%s", err, out.String())
	}
}

// A manifest entry that is not local to the install root is dropped when
// the state is READ, so every reader — install's resync, uninstall, status —
// is guarded in one place (security review 2026-09-14).
func TestLoadStateDropsNonLocalManifestEntries(t *testing.T) {
	home := t.TempDir()
	if err := SaveState(home, State{Repo: "r", Files: []string{".claude/a.md", "../escape", "/etc/passwd", "ok/../../x"}}); err != nil {
		t.Fatal(err)
	}
	s, ok, err := LoadState(home)
	if err != nil || !ok {
		t.Fatalf("LoadState: ok=%v err=%v", ok, err)
	}
	if len(s.Files) != 1 || s.Files[0] != ".claude/a.md" {
		t.Fatalf("Files = %v, want only the local entry", s.Files)
	}
}

// A symlink inside a skill directory is refused by name, never followed:
// os.ReadFile would deploy the target's bytes under the link's mode, and a
// link's 0777 becomes a world-writable file in ~/.claude (security review
// 2026-09-14, high). The same refusal guards adopt's copy.
func TestSkillFilesAndAdoptCopiesRefuseSymlinks(t *testing.T) {
	repo := t.TempDir()
	dir := filepath.Join(repo, "skills", "s")
	writeFile(t, filepath.Join(repo, "context", "AGENTS.md"), "# ctx\n")
	writeFile(t, filepath.Join(dir, "SKILL.md"), "---\nname: s\ndescription: \"x\"\n---\nbody\n")
	outside := filepath.Join(t.TempDir(), "secret")
	writeFile(t, outside, "not for ~/.claude\n")
	if err := os.Symlink(outside, filepath.Join(dir, "link.md")); err != nil {
		t.Skip("symlinks unavailable here")
	}
	if _, err := Plan(repo, nil, ""); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("Plan err = %v, want a symlink refusal", err)
	}
	if err := copyTree(dir, t.TempDir()); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("copyTree err = %v, want a symlink refusal", err)
	}
}
