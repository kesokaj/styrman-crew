---
name: retro
description: Session retrospective — analyze corrections and propose harness improvements.
disable-model-invocation: true
---

# Session Retrospective

Analyze the current session to identify gaps in the agent harness and propose specific improvements.

## Usage

```
/retro              # Full retro: analyze → propose → write proposal file
/retro --dry-run    # Analysis only: show findings without writing proposals
```

## When to Suggest

At the end of any session where you detected **two or more** of the following:
- The user corrected your behavior or output
- The user re-stated a preference you should have already known
- The user overrode a tool/framework/pattern choice
- The user said something like "no, always do X" or "we don't use Y"
- You had to be told the same thing twice

Suggest it naturally: *"I noticed a few corrections this session. Want me to run `/retro` to capture those as harness improvements?"*

Do NOT suggest `/retro` if the session was smooth and correction-free.

## Step 1: Read the Session Transcript and Feedback Log

Read the current conversation transcript from the conversation logs directory, plus any entries in the global feedback log (`~/.agents/feedback/log.jsonl`) from this session — the capture hook and `feedback` skill both write there; deduplicate overlapping entries. Scan for:

1. **Explicit corrections** — User said "no", "wrong", "don't do that", "actually we…"
2. **Re-stated preferences** — User repeated a rule that already exists in the global standards (`shared/AGENTS.md`) or a skill
3. **Missing knowledge** — You didn't know something about the user's stack, tooling, or conventions
4. **Output format friction** — The user asked you to restructure, reformat, or re-present output
5. **Architecture/tooling mismatches** — You suggested the wrong tool, framework, or pattern

## Step 2: Classify Findings

For each finding, classify it into one of these categories:

Target files live in the harness repo (`devshell_prompt/harness/`); `shared/AGENTS.md` is the single source deployed as both `~/.claude/CLAUDE.md` and `~/.gemini/GEMINI.md`.

| Category | Target File |
|---|---|
| **Global rule gap** | `harness/shared/AGENTS.md` |
| **Skill knowledge gap** | `harness/shared/skills/<skill>/SKILL.md` or new skill |
| **Skill reference gap** | `harness/shared/skills/<skill>/references/<topic>.md` |
| **Output format preference** | `harness/shared/AGENTS.md` (§9 AI Session Preferences) |
| **Workflow improvement** | `harness/shared/skills/<workflow>/SKILL.md` |
| **New convention** | `harness/shared/AGENTS.md` (appropriate section) |

## Step 3: Draft Proposals

For each finding, draft a **specific, minimal change**:

- For AGENTS.md edits: show the exact diff (lines to add/modify)
- For skill edits: show the exact section and change
- For new files: show the full file content
- For each proposal, explain **why** — what went wrong and how this prevents it

### Proposal Quality Rules

- **Be specific.** "Add a rule about X" is not a proposal. Show the exact text.
- **Be minimal.** Don't rewrite entire sections. Target the smallest change that fixes the gap.
- **Don't over-generalize.** If the user corrected one specific thing, don't infer a broad philosophy change.
- **Respect existing structure.** Edits to AGENTS.md should follow the existing section numbering and style.
- **Check for duplicates.** Read the target file first — the rule might already exist and just need clarification.

## Step 4: Write the Proposal

If not `--dry-run`, write the proposals to the global proposals directory (works from any project — proposals are always about the global harness):

```
~/.agents/proposals/YYYY-MM-DD-retro.md
```

If the directory does not exist, create it first (`mkdir -p ~/.agents/proposals`).

Use this format:

```markdown
# Session Retrospective — YYYY-MM-DD

Session ID: <conversation-id>

## Summary

<1-2 sentence overview of what was found>

## Proposals

### Proposal 1: <short title>

**Category:** <from table above>
**Target:** `<file path>`
**Trigger:** <what happened in the session>

**Current behavior:** <what the agent did wrong>
**Expected behavior:** <what should have happened>

#### Proposed Change

\```diff
- <old line>
+ <new line>
\```

**Rationale:** <why this change prevents the issue>

---

### Proposal 2: ...

## Feedback Log Entries

<List any entries in ~/.agents/feedback/log.jsonl from this session (skill-written and auto-hook)>
```

## Step 5: Present to User

Show the user:
1. How many findings were identified
2. A summary table of proposals
3. Ask: *"Review the proposal file at `~/.agents/proposals/YYYY-MM-DD-retro.md`. Want me to apply any of these changes?"*

If the user approves specific proposals:
- Apply the changes to the target files in the harness repo
- Run `./harness/setup.sh --dry-run` to verify deployment would succeed
- Commit with message: `harness: apply retro proposals from YYYY-MM-DD`

If the user rejects proposals:
- Leave the proposal file in place (it's append-only documentation)
- Note rejections in the file with a `**Status: Rejected** — <reason>` annotation
