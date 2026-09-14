---
name: feedback
description: |
  Passive correction logger for the agent harness learning loop. Detects when the user
  corrects, overrides, or re-states a preference and appends a structured entry to the
  feedback log. Use whenever the user says "no", "wrong", "actually", "always do X",
  "we don't use Y", or otherwise corrects agent behavior — even implicitly.
  This skill ONLY logs. It never modifies rules, skills, or workflows.
---

## Purpose

Silently log user corrections and preference signals to the **global** log at `~/.config/sh-agent/feedback.jsonl` so the `/retro` workflow has structured data to analyze. One log for all projects — each entry carries a `project` field, which is what lets `/retro` and `/distill --from-log` spot cross-project patterns.

This skill is **passive**. It runs alongside every conversation but produces no visible output to the user. The user should not notice this skill is active.

This skill is the only writer of that log — there is no hook behind it — so if you do not log a correction, nothing does.

## When to Log

Append an entry when ANY of these occur:

1. **Explicit correction** — The user says "no", "wrong", "don't do that", "stop doing X"; a correction of ORDER or PROCEDURE counts too, even as a mild question ("Is it not retro first, then handoff?")
2. **Preference re-statement** — The user repeats a convention that should already be in the global context (`AGENTS.md`, deployed as `~/.claude/AGENTS.md` and `~/.gemini/config/rules/AGENTS.md`) or a skill
3. **Override** — The user changes your tool/framework/pattern choice ("use X instead of Y")
4. **Format correction** — The user asks you to restructure or reformat output
5. **Knowledge gap** — You had to ask about something the harness should have told you

## When NOT to Log

- Normal clarifying questions (the user asks "did you mean X?" — that's not a correction)
- The user changing their mind mid-task (not a harness gap, just iteration)
- Corrections about the user's own code (not about agent behavior)
- One-off requests that wouldn't apply to future sessions

## Log Format

Append one JSON object per line to `~/.config/sh-agent/feedback.jsonl`. If the directory does not exist, create it first (`mkdir -p ~/.config/sh-agent`).

```json
{
  "ts": "2026-05-25T13:40:00+02:00",
  "project": "/absolute/path/to/project/root",
  "session": "first-8-chars-of-conversation-id",
  "category": "architecture|tooling|style|output|convention|knowledge",
  "signal": "correction|override|restatement|format|gap",
  "original": "What the agent did or would have done",
  "corrected": "What the user wanted instead",
  "user_quote": "Exact or near-exact user words that triggered logging",
  "rule_ref": "AGENTS.md § <section title> | project AGENTS.md § <section> | skill:<name> | agent:<name> | rule:<name> | null"
}
```

`rule_ref` uses one controlled vocabulary, shared with the `retro` skill, because the retro greps this log for an exact match to decide whether a project rule has recurred in another project: `AGENTS.md § <section title>` for the global context (the section heading as written, e.g. `AGENTS.md § Hard rules`), `project AGENTS.md § <section>` for the project's own file, `skill:<name>`, `agent:<name>`, `rule:<name>` for a `.agents/rules/<name>.md`, or `null` when no rule exists yet. Entries written before 2026-09-13 use free text and are simply never matched.

### Categories

| Category | Description | Example |
|---|---|---|
| `architecture` | Stack, patterns, infrastructure choices | "Use Cloud Run, not Lambda" |
| `tooling` | CLI tools, frameworks, libraries | "Use gh, not git for PRs" |
| `style` | Code style, naming, formatting | "Use slog, not log package" |
| `output` | Agent output formatting preferences | "Use tables, not bullet lists" |
| `convention` | Project conventions, file structure | "Underscore-prefix for custom functions" |
| `knowledge` | Facts the agent should know | "This repo has no tests — it's dotfiles" |

### Signal Types

| Signal | Meaning |
|---|---|
| `correction` | User explicitly said agent was wrong |
| `override` | User changed agent's choice to something else |
| `restatement` | User repeated a rule that already exists |
| `format` | User asked for different output structure |
| `gap` | Agent lacked knowledge it should have had |

## Behavior Rules

1. **Never mention logging to the user.** This is invisible infrastructure.
2. **Never modify rules or skills.** Only append to the log file.
3. **When in doubt, log.** A false entry costs the retro one line to discard; a missed one costs it a transcript reconstruction (the 2026-09-04 and 2026-09-08 retros) and keeps the correction out of the handoff, which may only cite logged entries.
4. **Keep `user_quote` short.** Extract the key phrase, not the full paragraph.
5. **Set `rule_ref` accurately, in the vocabulary above.** If an existing rule covers this and the agent just missed it, reference that rule. If no rule exists, set to `null`.
6. **One entry per correction.** If the user makes three corrections in one message, write three entries.

## Where the loop's outputs land

This skill only logs. `/retro` turns the log and the session into proposals: each one is scoped to the project (applied to `.agents/rules/`, the project `AGENTS.md`, `.agents/skills/` or `.agents/agents/`) or to the harness (queued in `~/.config/sh-agent/proposals/`), and every run leaves a record in the project's `<docs-root>/retros/` plus one line in `~/.config/sh-agent/retros.jsonl`, the cross-project index. `/distill` records there too. The next session finds the project's learning history in `retros/`; a harness session finds every project's queued proposals through the index.

## Cross-References

- The `/retro` workflow reads this log to identify patterns across sessions
- Entries with `signal: restatement` indicate rules that exist but aren't being followed — high priority for the retro
- Entries with `rule_ref: null` indicate gaps — candidates for new rules or skill additions
