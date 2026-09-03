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
/handoff                     # Handoff saved to project <docs-root>/handoffs/
/handoff --global            # Handoff saved to ~/.config/sh-agent/handoffs/ (cross-project)
/handoff fix the auth bug    # Tailor the handoff toward the next session's focus
```

## Step 1: Scan the Session

Read the conversation transcript and identify:

1. **Goal** — What was the user trying to accomplish?
2. **What got done** — Completed changes, files modified, commands run
3. **What's still open** — Unfinished work, known issues, blocked items
4. **Key decisions** — Architecture choices, trade-offs made, rejected alternatives
5. **Corrections** — Only what the user actually corrected, each backed by a `~/.config/sh-agent/feedback.jsonl` entry (quote the user's words and the entry's timestamp). Anything the agent inferred is not a correction: it goes under `## Agent notes (unverified)`, never into Corrections — the Resume Protocol gives the newest handoff the win on conflict, so an inferred rule here silently outranks the global context
6. **Environment state** — Running processes, branches checked out, pending commits, dirty working tree

## Step 2: Identify Relevant Artifacts

Do NOT duplicate content that already exists elsewhere. Instead, reference these by path or URL (paths below assume the canonical `.agents/docs/` root; some repos use a bare `docs/` tree instead — use whichever exists):

| Artifact | Location |
|---|---|
| Changelog entries | `.agents/docs/changes/YYYY-MM-DD-*.md` |
| Architecture decisions | `.agents/docs/architecture/DECISIONS.md` |
| Specs | `.agents/docs/specs/<app>/` |
| Implementation plans | `.agents/docs/plans/` |
| Testing guides | `.agents/docs/testing/<app>/` |
| Deploy guides | `.agents/docs/deploy/{local,remote,gcp}.md` |
| Deferred work | `TODO.md` |
| Requirements | `REQUIREMENTS.md` |
| Retro proposals | `~/.config/sh-agent/proposals/` |
| Commits | Reference by short SHA |
| PRs / Issues | Reference by `#number` or URL |

List each relevant artifact with a one-line summary of what it contains and why the next agent should read it.

## Step 3: Bring the Trail Current — FIRST, before writing anything

The handoff records the remainder — it is never a substitute for rows already due:

1. Run `date +%F` and use that date everywhere below — never a date from memory.
2. If the session changed source files and no `changes/` entry covers those changes, dispatch the `changelog` agent **before** writing the handoff.
3. The main agent does **not** edit `TODO.md` or `ROADMAP.md` inline. `TODO.md` record updates ride in the `changelog` dispatch from step 2; when the plan shifted, dispatch the `documentation` agent for the `ROADMAP.md` "Now" update. If neither dispatch is warranted, the handoff **notes** the staleness instead of fixing it inline.
4. Only then write the handoff (Step 4).

## Step 4: Write the Handoff Document

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

- <the user's words, verbatim or near> — feedback log <timestamp> (required: no log entry, no correction)

## Agent notes (unverified)

- <inferences the agent made this session; the next agent may re-examine them>

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

## Step 5: Suggest Skills

Based on the session content, recommend skills the next agent should invoke. Only suggest skills that are genuinely relevant — don't list every skill you know about.

Common suggestions:

| If the session involved… | Suggest |
|---|---|
| Code changes without review | dispatch the `reviewer` agent (uses `review-standards`) |
| Go code | `golang` |
| Frontend / UI work | `frontend`, `typescript` |
| Infrastructure / deploy | `cloud-run-deploy`, `docker`, `terraform` |
| Shell scripts | `bash` |
| Corrections or friction | `feedback` (to log), then `/retro` (to analyze) |
| New patterns discovered | `/distill` |

## Step 6: Save the Document

### Project-local (default)

Resolve the docs root first: `.agents/docs` if it exists, else `docs/` if it exists, else create `.agents/docs`.

```bash
DOCS_ROOT=.agents/docs
[ -d "$DOCS_ROOT" ] || { [ -d docs ] && DOCS_ROOT=docs; } || true
mkdir -p "$DOCS_ROOT/handoffs"
# Save as: $DOCS_ROOT/handoffs/$(date +%F)-<topic>.md
```

Handoffs are dated and **immutable** — never overwrite a previous handoff; a new session gets a new file.

Reading side: the Resume Protocol in the global context tells every fresh session, on either vendor, to read the newest handoff.

### Global (`--global`)

For cross-project handoffs or when the work spans multiple repos:

```bash
mkdir -p ~/.config/sh-agent/handoffs
# Save as: ~/.config/sh-agent/handoffs/YYYY-MM-DD-<topic>.md
```

### Cold-reader check — before the handoff counts as finished

A handoff is done when a fresh reader can resume from it, not when it is saved. Run the Resume Protocol as that reader (yourself with the file only, or the `explorer` agent given the path and nothing else): AGENTS.md → TODO.md → this handoff → ROADMAP.md "Now" → the active plan's unticked boxes → the three newest `changes/` → `bugs/INDEX.md`. Ask for four things: the status and single next step as the trail states them, any named file that does not exist, any contradiction between handoff, ROADMAP, TODO, and plan, and anything still left to guess. Fix what comes back at the source (plan, ROADMAP, TODO via the dispatches above) and in the handoff itself while it is still this session's; then report.

## Step 7: Report to User

After saving, tell the user:

1. Where the handoff document was saved (absolute path)
2. How to use it: *"Start a new session and paste: `Read the handoff at <path> and continue the work.`"*
3. A 1-sentence summary of the handoff content

## Cross-References

- The `/retro` workflow captures corrections and proposes harness improvements
- The `/distill` workflow captures repeated patterns as reusable skills
- The `feedback` skill passively logs corrections to `~/.config/sh-agent/feedback.jsonl`
- The `changelog` subagent writes `.agents/docs/changes/` entries — reference these, don't duplicate them
