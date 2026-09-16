package harness

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Skill is one directory under skills/ holding a SKILL.md.
type Skill struct {
	Name        string
	Description string
	Dir         string // absolute, symlinks resolved
	SlashOnly   bool   // disable-model-invocation: the user types /name
}

// Agent is one Claude Code subagent definition under agents/.
type Agent struct {
	Name        string
	Description string
	Path        string
	Front       Frontmatter
	Body        string
	Raw         []byte
}

// Doc is a plain markdown customization — a command or a rule. Frontmatter
// is optional; without a description the first line of text stands in.
type Doc struct {
	Name        string
	Description string
	Path        string
	Raw         []byte
}

// Inventory is everything the repo contributes. Each field is a directory:
// drop a file into it and the next install deploys it — no code changes.
type Inventory struct {
	Skills   []Skill // skills/<name>/SKILL.md
	Agents   []Agent // agents/<name>.md
	Commands []Doc   // commands/<name>.md and workflows/<name>.md
	Rules    []Doc   // rules/<name>.md
}

// validName is the parse boundary for a frontmatter `name:` — the value
// `plan.go` joins into ~/.claude/skills/<name>/…, ~/.claude/agents/<name>.md
// and the agy mirrors. A name is ONE path segment: nothing that filepath.Join
// would carry out of the install root (`../x`, an absolute path), and no
// separator at all (`a/b` would nest a skill inside another's directory),
// and only the characters an identifier needs (`[A-Za-z0-9._-]`, leading
// alphanumeric): the same value is rendered verbatim into the consumer's
// AGENTS.md, so a backtick or a newline in it would inject prose into the
// global context (security review 2026-09-14).
// Refused with the file named, so an author sees it at `sh-agent install`
func identLike(name string) bool {
	for i, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case i > 0 && (r == '.' || r == '_' || r == '-'):
		default:
			return false
		}
	}
	return true
}

// time rather than finding a file written beside their home.
func validName(kind, file, name string) error {
	switch {
	case name == "", name == ".", name == "..":
		return fmt.Errorf("%s %s: name %q is not a path segment", kind, file, name)
	case strings.ContainsAny(name, `/\`), !filepath.IsLocal(name), !identLike(name):
		return fmt.Errorf("%s %s: name %q must be a single local path segment", kind, file, name)
	}
	return nil
}

// readRegular reads a file the repo ships, refusing a link. checkManagedTree
// covers the managed directories and every entry in them; this guards the one
// read it cannot reach, skills/<name>/SKILL.md, because skills/<name> may
// itself be a link that LoadSkills resolves (BUG-036).
func readRegular(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("%s is a symlink — the repo ships regular files only", path)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file — the repo ships regular files only", path)
	}
	return os.ReadFile(path)
}

// LoadInventory reads every customization directory of the repo.
// checkManagedTree refuses a symbolic link anywhere sh-agent reads the repo's
// own bytes: each managed directory, and every entry directly inside the flat
// ones. os.ReadDir and filepath.Glob follow a directory link and os.ReadFile
// follows a file link, so either deploys bytes from outside the repo into
// every consumer home — and for context/ into the AGENTS.md both CLIs send to
// the model (BUG-036). One check at the boundary, so no reader can forget it.
func checkManagedTree(repo string) error {
	for _, dir := range []string{"context", "agents", "commands", "rules", "workflows", "skills"} {
		p := filepath.Join(repo, dir)
		info, err := os.Lstat(p)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%s is a symlink — the repo ships regular directories only", p)
		}
		if !info.IsDir() || dir == "skills" {
			// skills/<name> may be a link: `npx skills add -g` leaves one and
			// LoadSkills resolves it. A skill's own files are walked in
			// plan.go, and its SKILL.md is read with readRegular.
			continue
		}
		entries, err := os.ReadDir(p)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if e.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("%s is a symlink — the repo ships regular files only", filepath.Join(p, e.Name()))
			}
		}
	}
	return nil
}

func LoadInventory(repo string) (Inventory, error) {
	var inv Inventory
	if err := checkManagedTree(repo); err != nil {
		return inv, err
	}
	var err error
	if inv.Skills, err = LoadSkills(repo); err != nil {
		return inv, err
	}
	if inv.Agents, err = LoadAgents(repo); err != nil {
		return inv, err
	}
	if inv.Commands, err = LoadDocs(repo, "commands", "workflows"); err != nil {
		return inv, err
	}
	if inv.Rules, err = LoadDocs(repo, "rules"); err != nil {
		return inv, err
	}
	return inv, nil
}

// LoadSkills reads every skills/<name>/SKILL.md. A directory without a
// SKILL.md is ignored; one with a SKILL.md but no description is an error,
// because the description is the only thing either CLI shows the model.
func LoadSkills(repo string) ([]Skill, error) {
	root := filepath.Join(repo, "skills")
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", root, err)
	}
	var skills []Skill
	for _, e := range entries {
		dir, err := filepath.EvalSymlinks(filepath.Join(root, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("resolving skill %s: %w", e.Name(), err)
		}
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			continue
		}
		data, err := readRegular(filepath.Join(dir, "SKILL.md"))
		if err != nil {
			// No SKILL.md means the directory is not a skill. Any other
			// error, a symlinked SKILL.md above all, is refused out loud:
			// plan.go's walk only visits skills this loader returned, so a
			// silent skip would drop the refusal altogether (BUG-036).
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, err
		}
		front, _, err := Split(string(data))
		if err != nil {
			return nil, fmt.Errorf("skill %s: %w", e.Name(), err)
		}
		if err := Lint(front); err != nil {
			return nil, fmt.Errorf("skill %s: %w", e.Name(), err)
		}
		fm := Parse(front)
		name := fm["name"]
		if name == "" {
			name = e.Name()
		}
		if err := validName("skill", e.Name()+"/SKILL.md", name); err != nil {
			return nil, err
		}
		if fm["description"] == "" {
			return nil, fmt.Errorf("skill %s: SKILL.md has no description", e.Name())
		}
		skills = append(skills, Skill{
			Name:        name,
			Description: fm["description"],
			Dir:         dir,
			SlashOnly:   fm["disable-model-invocation"] == "true",
		})
	}
	sort.Slice(skills, func(i, j int) bool { return skills[i].Name < skills[j].Name })
	return skills, nil
}

// LoadAgents reads every agents/*.md in Claude Code's subagent format.
func LoadAgents(repo string) ([]Agent, error) {
	paths, err := filepath.Glob(filepath.Join(repo, "agents", "*.md"))
	if err != nil {
		return nil, err
	}
	var agents []Agent
	for _, p := range paths {
		raw, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		front, body, err := Split(string(raw))
		if err != nil {
			return nil, fmt.Errorf("agent %s: %w", filepath.Base(p), err)
		}
		if err := Lint(front); err != nil {
			return nil, fmt.Errorf("agent %s: %w", filepath.Base(p), err)
		}
		fm := Parse(front)
		name := fm["name"]
		if name == "" {
			name = strings.TrimSuffix(filepath.Base(p), ".md")
		}
		if err := validName("agent", filepath.Base(p), name); err != nil {
			return nil, err
		}
		if fm["description"] == "" {
			return nil, fmt.Errorf("agent %s: no description", name)
		}
		agents = append(agents, Agent{Name: name, Description: fm["description"], Path: p, Front: fm, Body: body, Raw: raw})
	}
	sort.Slice(agents, func(i, j int) bool { return agents[i].Name < agents[j].Name })
	return agents, nil
}

// LoadDocs reads every *.md directly under the named repo directories.
// Missing directories are fine; the same name in two of them is not.
func LoadDocs(repo string, dirs ...string) ([]Doc, error) {
	seen := map[string]string{}
	var docs []Doc
	for _, dir := range dirs {
		paths, err := filepath.Glob(filepath.Join(repo, dir, "*.md"))
		if err != nil {
			return nil, err
		}
		for _, p := range paths {
			raw, err := os.ReadFile(p)
			if err != nil {
				return nil, err
			}
			name := strings.TrimSuffix(filepath.Base(p), ".md")
			if prev, dup := seen[name]; dup {
				return nil, fmt.Errorf("%s and %s both define %q", prev, p, name)
			}
			seen[name] = p
			desc := ""
			text := string(raw)
			if front, body, err := Split(text); err == nil {
				if err := Lint(front); err != nil {
					return nil, fmt.Errorf("%s: %w", p, err)
				}
				desc = Parse(front)["description"]
				text = body
			}
			if desc == "" {
				desc = firstLine(text)
			}
			docs = append(docs, Doc{Name: name, Description: desc, Path: p, Raw: raw})
		}
	}
	sort.Slice(docs, func(i, j int) bool { return docs[i].Name < docs[j].Name })
	return docs, nil
}

func firstLine(s string) string {
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(strings.TrimLeft(l, "# ")); l != "" {
			return l
		}
	}
	return ""
}

// RenderInventory produces the markdown section appended to the deployed
// context so the model knows what is installed without listing directories.
func RenderInventory(inv Inventory) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Installed skills, agents, commands, and rules\n\n")
	fmt.Fprintf(&b, "Generated by `sh-agent` from the harness repo; `sh-agent list` prints the same.%s\n\n", slashNote(inv))
	fmt.Fprintf(&b, "### Skills (%d)\n\n| Skill | Invoke | Use for |\n|---|---|---|\n", len(inv.Skills))
	for _, s := range inv.Skills {
		invoke := "auto"
		if s.SlashOnly {
			invoke = "`/" + s.Name + "`"
		}
		fmt.Fprintf(&b, "| `%s` | %s | %s |\n", s.Name, invoke, cell(s.Description, 140))
	}
	fmt.Fprintf(&b, "\n### Subagents (%d)\n\n| Agent | Model | Does |\n|---|---|---|\n", len(inv.Agents))
	for _, a := range inv.Agents {
		model := a.Front["model"]
		if model == "" {
			model = "inherit"
		}
		fmt.Fprintf(&b, "| `%s` | %s | %s |\n", a.Name, model, cell(a.Description, 160))
	}
	if len(inv.Commands) > 0 {
		fmt.Fprintf(&b, "\n### Commands (%d)\n\n| Command | Does |\n|---|---|\n", len(inv.Commands))
		for _, c := range inv.Commands {
			fmt.Fprintf(&b, "| `/%s` | %s |\n", c.Name, cell(c.Description, 160))
		}
	}
	if len(inv.Rules) > 0 {
		fmt.Fprintf(&b, "\n### Rules (%d)\n\n| Rule | About |\n|---|---|\n", len(inv.Rules))
		for _, r := range inv.Rules {
			fmt.Fprintf(&b, "| `%s` | %s |\n", r.Name, cell(r.Description, 160))
		}
	}
	return b.String()
}

// RenderInventoryCompact is the form embedded in the deployed context: names
// only. Both CLIs inject every skill's description at session start anyway,
// so repeating descriptions there would spend tokens on what is already
// present; `sh-agent list` prints the full table.
func RenderInventoryCompact(inv Inventory) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Installed\n\nGenerated by `sh-agent`; `sh-agent list` prints the descriptions.%s\n\n", slashNote(inv))
	var skills []string
	for _, s := range inv.Skills {
		if s.SlashOnly {
			skills = append(skills, "`/"+s.Name+"`")
		} else {
			skills = append(skills, "`"+s.Name+"`")
		}
	}
	fmt.Fprintf(&b, "- Skills (%d): %s\n", len(inv.Skills), strings.Join(skills, ", "))
	var agents []string
	for _, a := range inv.Agents {
		model := a.Front["model"]
		if model == "" {
			model = "inherit"
		}
		agents = append(agents, "`"+a.Name+"` ("+model+")")
	}
	fmt.Fprintf(&b, "- Subagents (%d): %s\n", len(inv.Agents), strings.Join(agents, ", "))
	if len(inv.Commands) > 0 {
		var names []string
		for _, c := range inv.Commands {
			names = append(names, "`/"+c.Name+"`")
		}
		fmt.Fprintf(&b, "- Commands (%d): %s\n", len(inv.Commands), strings.Join(names, ", "))
	}
	if len(inv.Rules) > 0 {
		var names []string
		for _, r := range inv.Rules {
			names = append(names, "`"+r.Name+"`")
		}
		fmt.Fprintf(&b, "- Rules (%d): %s\n", len(inv.Rules), strings.Join(names, ", "))
	}
	return b.String()
}

// slashNote explains the `/name` marking, but only when some skill carries
// `disable-model-invocation`; with none, the sentence would describe nothing.
func slashNote(inv Inventory) string {
	for _, s := range inv.Skills {
		if s.SlashOnly {
			return " Skills marked `/name` are typed by the user; the rest load when their description matches the task."
		}
	}
	return " Every skill loads when its description matches the task and can also be typed as `/name`."
}

// cell shortens text for a table cell and escapes the column separator.
func cell(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > max {
		cut := strings.LastIndex(s[:max], " ")
		if cut < max/2 {
			cut = max
		}
		s = s[:cut] + "…"
	}
	return strings.ReplaceAll(s, "|", `\|`)
}
