package harness

import (
	"fmt"
	"strings"
)

// modelMap translates Claude model aliases to Antigravity tiers. Anything
// unlisted (sonnet, fable) is omitted, which Antigravity reads as `inherit`.
var modelMap = map[string]string{"opus": "pro", "haiku": "flash"}

// toolMap translates Claude tool names to Antigravity ones. An unmapped tool
// is an error rather than a pass-through: Antigravity hangs a subagent whose
// tools list names something it does not recognise.
var toolMap = map[string]string{
	"Read":  "view_file",
	"Grep":  "grep_search",
	"Glob":  "find_by_name",
	"Bash":  "run_command",
	"Write": "write_to_file",
	"Edit":  "replace_file_content",
}

const (
	claudeSkillsDir = "~/.claude/skills/"
	agySkillsDir    = "~/.gemini/config/skills/"
)

// DeriveAgy renders the Antigravity `agent.md` flavour of a Claude Code agent.
// Claude-only keys (effort, maxTurns, isolation, permissionMode, skills)
// are dropped: Antigravity has no equivalent, and markdown agents inherit
// the global skills anyway.
func DeriveAgy(a Agent) ([]byte, error) {
	out := []string{
		"name: " + a.Name,
		"description: " + yamlQuote(a.Description),
		"subagent: true",
		"mainAgent: false",
	}
	if tier, ok := modelMap[a.Front["model"]]; ok {
		out = append(out, "model: "+tier)
	}
	if tools := a.Front["tools"]; tools != "" {
		var mapped []string
		for _, t := range SplitList(tools) {
			agy, ok := toolMap[t]
			if !ok {
				return nil, fmt.Errorf("agent %s: tool %q has no Antigravity mapping", a.Name, t)
			}
			mapped = append(mapped, agy)
		}
		out = append(out, "tools: ["+strings.Join(mapped, ", ")+"]")
	}
	body := strings.ReplaceAll(a.Body, claudeSkillsDir, agySkillsDir)
	return []byte("---\n" + strings.Join(out, "\n") + "\n---\n" + body), nil
}
