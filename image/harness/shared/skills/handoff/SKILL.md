---
name: handoff
description: |
  Compact the current conversation into a handoff document for another agent to
  pick up. Use when the user says "handoff", "hand off", "wrap up for next session",
  "summarize for the next agent", or wants to capture session context before ending.
  Also use when context is getting long and the user wants to start fresh.
disable-model-invocation: true
argument-hint: "What will the next session focus on?"
---

# Session Handoff

Write a structured handoff document summarising the current conversation so a fresh agent can continue the work without re-reading the full chat history.

## Usage

```
/handoff                     # Handoff saved to project .agents/docs/handoffs/
/handoff --global            # Handoff saved to ~/.agents/handoffs/ (cross-project)
/handoff fix the auth bug    # Tailor the handoff toward the next session's focus
```

## Step 1: Scan the Session

Read the conversation transcript and identify:

1. **Goal** — What was the user trying to accomplish?
2. **What got done** — Completed changes, files modified, commands run
3. **What's still open** — Unfinished work, known issues, blocked items
4. **Key decisions** — Architecture choices, trade-offs made, rejected alternatives
5. **Corrections** — Anything the user corrected mid-session (the next agent should not repeat those mistakes)
6. **Environment state** — Running processes, branches checked out, pending commits, dirty working tree

## Step 2: Identify Relevant Artifacts

Do NOT duplicate content that already exists elsewhere. Instead, reference these by path or URL:

| Artifact | Location |
|---|---|
| Changelog entries | `.agents/docs/changes/YYYY-MM-DD-*.md` |
| Architecture decisions | `.agents/docs/architecture/DECISIONS.md` |
| Specs | `.agents/docs/specs/<app>/` |
| Implementation plans | `.agents/docs/plans/` |
| Testing guides | `.agents/docs/testing/<app>/` |
| Deferred work | `TODO.md` |
| Requirements | `REQUIREMENTS.md` |
| Retro proposals | `~/.agents/proposals/` |
| Commits | Reference by short SHA |
| PRs / Issues | Reference by `#number` or URL |

List each relevant artifact with a one-line summary of what it contains and why the next agent should read it.

## Step 3: Write the Handoff Document

Use this template:

```markdown
# Handoff — YYYY-MM-DD — <topic>

Session ID: <conversation-id>
Project: <absolute path to project root>
Branch: <current git branch>

## Context

<1-3 sentences: what was the user working on and why.>

## Completed

- <bullet list of what got done, with file paths>

## Open / In Progress

- <bullet list of unfinished work>
- <include any error messages or blockers>

## Key Decisions

- <decisions made during the session, with rationale>
- <rejected alternatives and why>

## Corrections

- <anything the user corrected — the next agent must not repeat these>

## Environment State

- Branch: <branch name>, <clean/dirty>
- Running processes: <any dev servers, watchers, etc.>
- Pending: <uncommitted changes, unstaged files>

## Artifacts to Read

- `<path>` — <one-line summary>

## Suggested Skills

The next agent should invoke these skills:

- `<skill-name>` — <why>

## Next Steps

<If the user provided arguments, tailor this section to what the next session
should focus on. Otherwise, infer from the open items above.>

1. <prioritized action item>
2. <prioritized action item>
```

### Writing Rules

- **Be concise.** The whole document should be under 150 lines. A fresh agent needs signal, not noise.
- **Use absolute paths.** The next agent may not share the same working directory.
- **Reference, don't repeat.** If a spec or plan exists, link to it — don't paste its contents.
- **Redact sensitive data.** Strip API keys, passwords, tokens, PII, and anything from `~/.zsh_secrets` or `.env` files. Replace with `<REDACTED>`.
- **Include the branch and commit state.** The next agent needs to know where the code stands.

## Step 4: Suggest Skills

Based on the session content, recommend skills the next agent should invoke. Only suggest skills that are genuinely relevant — don't list every skill you know about.

Common suggestions:

| If the session involved… | Suggest |
|---|---|
| Code changes without review | `code-review` |
| Go code | `golang` |
| Frontend / UI work | `frontend`, `typescript` |
| Infrastructure / deploy | `cloud-run-deploy`, `docker`, `terraform` |
| Shell scripts | `bash` |
| Corrections or friction | `feedback` (to log), then `/retro` (to analyze) |
| New patterns discovered | `/distill` |

## Step 5: Save the Document

### Project-local (default)

```bash
mkdir -p .agents/docs/handoffs
# Save as: .agents/docs/handoffs/YYYY-MM-DD-<topic>.md
```

### Global (`--global`)

For cross-project handoffs or when the work spans multiple repos:

```bash
mkdir -p ~/.agents/handoffs
# Save as: ~/.agents/handoffs/YYYY-MM-DD-<topic>.md
```

## Step 6: Report to User

After saving, tell the user:

1. Where the handoff document was saved (absolute path)
2. How to use it: *"Start a new session and paste: `Read the handoff at <path> and continue the work.`"*
3. A 1-sentence summary of the handoff content

## Cross-References

- The `/retro` workflow captures corrections and proposes harness improvements
- The `/distill` workflow captures repeated patterns as reusable skills
- The `feedback` skill passively logs corrections to `~/.agents/feedback/log.jsonl`
- The `changelog` subagent writes `.agents/docs/changes/` entries — reference these, don't duplicate them
