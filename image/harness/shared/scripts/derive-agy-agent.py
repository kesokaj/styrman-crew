#!/usr/bin/env python3
"""Derive an Antigravity CLI agent .md from a Claude Code agent .md.

Claude Code is the source of truth (claude/agents/*.md). This script produces
the agy-cli flavor per the Antigravity custom-agent spec:
  - keeps portable frontmatter keys: name, description
  - adds  subagent: true / mainAgent: false  (invokable via invoke_subagent,
    never selectable as the primary agent)
  - maps Claude model aliases to agy model tiers (opus->pro, haiku->flash;
    sonnet and anything else -> omitted, i.e. agy's default "inherit")
  - drops Claude-only keys (tools, memory, skills, permissionMode, ...) —
    agy tool names differ and a misspelled tool can hang a subagent
  - rewrites ~/.claude/skills/ paths to ~/.gemini/config/skills/

Usage: derive-agy-agent.py <claude-agent.md> <output.md>
"""

import re
import sys
from typing import Optional

MODEL_MAP = {"opus": "pro", "haiku": "flash"}


def main() -> None:
    if len(sys.argv) != 3:
        sys.exit(__doc__)
    src, dst = sys.argv[1], sys.argv[2]

    with open(src, encoding="utf-8") as f:
        text = f.read()

    m = re.match(r"^---\n(.*?)\n---\n", text, re.S)
    if not m:
        sys.exit(f"{src}: no YAML frontmatter block found")
    frontmatter, body = m.group(1), text[m.end():]

    def field(key: str) -> Optional[str]:
        fm = re.search(rf"^{key}:\s*(.+)$", frontmatter, re.M)
        return fm.group(1).strip() if fm else None

    name, description = field("name"), field("description")
    if not name or not description:
        sys.exit(f"{src}: frontmatter must contain single-line name and description")

    out = [f"name: {name}", f"description: {description}",
           "subagent: true", "mainAgent: false"]
    tier = MODEL_MAP.get(field("model") or "")
    if tier:
        out.append(f"model: {tier}")

    body = body.replace("~/.claude/skills/", "~/.gemini/config/skills/")

    with open(dst, "w", encoding="utf-8") as f:
        f.write("---\n" + "\n".join(out) + "\n---\n" + body)


if __name__ == "__main__":
    main()
