---
name: feature
description: "Workflow for feature work: any new behavior, new surface, or change that raises a design question. Use automatically whenever the user asks to build, add, implement, extend, or design something that is more than a one-sitting task, and whenever an implementation needs interfaces or a schema decided first; also invoked as /feature. Scope confirmation, plan file with phases and task checkboxes, cold-reader check of the plan before any code, one phase per checkpoint, e2e before done. Do NOT use for trivial edits, one-sitting tasks with no design question, or bug fixes (use the bug skill)."
argument-hint: "[feature description or #issue]"
---

# Feature workflow

Design before code; let a fresh reader test the design; implement in verifiable phases; prove it end to end. The plan file is the checkpoint that lets any later session continue.

**Docs root** (`<docs-root>`): `.agents/docs` if it exists, else `docs/` if it exists, else create `.agents/docs`. Dates from `date +%F`.

## Gates that need the user

Four points always wait for the user, whatever "work autonomously" instruction is in effect: the scope confirmation (Step 1), the no-code gate until the cold-reader check passes (Step 3), the three-round cap of that check, and a review loop that hits its cap. Everything else runs without asking. The user can waive a gate only in the same turn, explicitly.

## Step 1: Scope

Restate the request in one or two sentences: "I read this as: X. Confirm or correct." If `$ARGUMENTS` is `#<n>` or an issue URL, read it first (`gh issue view <n> --json title,body,labels,comments`). Check `REQUIREMENTS.md`, `ROADMAP.md`, and `docs/bugs/INDEX.md` for anything that constrains or has bitten this area before. Misreads here cost the most later.

## Step 2: Plan file — `<docs-root>/plans/<date>-<slug>.md`

```markdown
---
owner: main
date: YYYY-MM-DD
---
# Plan — <feature>   (<date>)

## Why
<user problem; cite the issue, conversation, or REQUIREMENTS item>

## Scope
In: … / Out: … / Follow-ups noted, not built: …

## Surfaces touched
<files, packages, routes, tables, external services>

## Interfaces
<function signatures, request/response shapes, schema changes, config keys — concrete enough that two implementers converge>

## Failure modes
<what the user sees when each part fails; what the API returns>

## Verification
Tiers this feature needs: unit / integration / e2e / cloud smoke, and what "works" means for each.
E2E criteria: <what a person or a browser does to see it working: the golden path and one edge case>

## Phases
### Phase 1 — <name> (dependency order: data model and migrations → services → API → UI → e2e)
- [ ] <task> — verify: <tier, command, who: AI | user | both>
- [ ] …
### Phase 2 — …

## Risks
```

Point `ROADMAP.md` "Now" at the plan. For a small feature the plan can be short; the sections stay.

## Step 3: Cold-reader check — no code until it passes

Dispatch the `reviewer` agent with **only the plan file**, nothing from the conversation, and this brief:

> You are a fresh reader with no prior context. Read this plan and the code surfaces it names. Answer two things. (1) Gaps: what must be decided before implementation starts that the plan does not decide — interfaces, failure modes, scope edges, misunderstandings of existing code (cite file:line). (2) Readiness: could a first-pass implementer ship this without asking the author a question? List every question you would have to ask. End with `design ready` only if you have zero gaps, and `implementation ready` only if you have zero questions; otherwise `design needs revision` / `implementation not ready`.

Address every gap and question in the plan, or rebut it in writing citing `REQUIREMENTS.md`, `AGENTS.md`, or the user's words. Re-run until both tokens are "ready". After three rounds without convergence the feature is under-specified: stop and ask the user which requirement to clarify or cut. Until then, no source edits; reading, `git log`, and running existing tests are fine.

## Step 4: Phases, one at a time

For each phase: implement its tasks in order; after each task run its named verification; when the phase is green, run the project's full test command, tick the boxes in the plan, write the `changes/` entry for the phase (`changelog` agent or yourself), commit, push. That is the checkpoint. A new session resumes at the first unticked box.

Per-layer verification, typical Go + TanStack Start project: migration applied and schema inspected → package tests with `-race` → API hit with `curl` → UI driven in a real browser (golden path, one edge case) → container built and run locally (`docker build`, `docker run`, `curl /healthz`).

## Step 5: E2E before "done"

Type checks and unit tests prove the code; e2e proves the feature. Service: build the container, run it, call the health check and one real request. Frontend: walk the golden path and one edge case (empty state, error response, unauthenticated) in a real browser. Deploy (only when the user asked for GCP): the live URL and the logs. Record the outcome in the verification log.

## Step 6: Definition of Done

Steps 1–5 of the global Definition of Done: `verifier` log, `reviewer` (diff only) and `security` when due, `documentation` when behavior or interfaces changed, `changelog` naming the agents dispatched, `bugs` for anything that went wrong on the way, commit and push. Then a report in chat: feature in one line, files by phase, tests added, cold-reader result and how each gap was resolved, review outcome with rebuttals verbatim, e2e result, follow-ups.
