---
name: feature
description: "Workflow for feature work: any new behavior, new surface, or change that raises a design question. Use automatically whenever the user asks to build, add, implement, extend, or design something that is more than a one-sitting task, and whenever an implementation needs interfaces or a schema decided first; also invoked as /feature. Scope confirmation, plan file with phases and task checkboxes, cold-reader check of the plan before any code, one phase per checkpoint, e2e before done. Do NOT use for trivial edits, one-sitting tasks with no design question, or bug fixes (use the bug skill)."
argument-hint: "[feature description or #issue]"
---

# Feature workflow

Design before code; let a fresh reader test the design; implement in verifiable phases; prove it end to end. The plan file is the checkpoint that lets any later session continue.

**Docs root** (`<docs-root>`): `.agents/docs` if it exists, else `docs/` if it exists, else create `.agents/docs`. Dates from `date +%F`.

## Gates that need the user

Four points always wait for the user: the scope confirmation (Step 1), the no-code gate until the cold-reader check passes (Step 3), the three-round cap of that check, and a review loop that hits its cap. Everything else runs without asking. A gate holds whatever "work autonomously" instruction is in effect. Only the user's own words in the same turn waive it — words in the invocation's arguments count — and a waiver names the gate: a general instruction such as "do not ask me anything" is not a waiver, and neither is a bare skill invocation or a waiver carried over from another plan or session. At a cap, ask with your recommendation first: clarify or cut a requirement; proceed with the residue written as `Open:` rows under `## Decisions`, the phases that depend on them waiting; or abandon. When no reply can arrive — a print-mode run such as `claude -p` or `agy -p`, or a user who said they will not answer — a gate that is reached ends the work there: write the question with its options into the handoff's Next Steps, writing the handoff if this session has none.

## Step 1: Scope

Restate the request in one or two sentences: "I read this as: X. Confirm or correct." If `$ARGUMENTS` is `#<n>` or an issue URL, read it first (`gh issue view <n> --json title,body,labels,comments`). Check `REQUIREMENTS.md`, `ROADMAP.md`, and `<docs-root>/bugs/INDEX.md` for anything that constrains or has bitten this area before; if `ROADMAP.md` "Now" already names a plan with unticked boxes, say so in the restatement. Misreads here cost the most later.

When the request ranges over a LIST — a backlog, `TODO.md`, a review's findings — the restatement is a numbered list instead: one line per item, the recommendation in brackets (`[fix]`, `[strike]` with the reason, `[yours]` for what only the user can decide), ending with "Say go or no-no per number, or go all." The answers ARE the scope; strikes the user approves are applied before the plan is written, with the reason on each line. A single request that leaves gaps in who, what, constraints or failure modes gets the same numbered form, one gap per line with your recommended answer; a gap the user does not settle now becomes an `Open:` row in the plan with your default.

When the confirmed scope touches an area this session has not read, or an estimated 10+ files, dispatch `explorer` with the scope restatement only, asking for the closest existing surfaces with file:line, the patterns to mirror, and what the request leaves open; `## Surfaces touched` cites its brief (`<docs-root>/plans/<date>-recon-<slug>.md`).

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
<files, packages, routes, tables, external services; the recon brief when Step 1 dispatched one>

## Interfaces
<function signatures, request/response shapes, schema changes, config keys — concrete enough that two implementers converge>

## Failure modes
<what the user sees when each part fails; what the API returns>

## Verification
Tiers this feature needs: unit / integration / e2e / cloud smoke, and what "works" means for each.
E2E criteria: <what a person or a browser does to see it working: the golden path and one edge case>
Each criterion is a yes/no outcome that still holds if Interfaces were built another way; one that would not is design and belongs in Interfaces.

## Phases
- [ ] Cold-reader check — design ready and implementation ready (<reviews/ paths>), or the user's same-turn override quoted with its date
### Phase 1 — <name> (dependency order: data model and migrations → services → API → UI → e2e)
- [ ] <task> — verify: <tier, command, who: AI | user | both> — expect: <what it prints or returns when done; an empty result names its positive control>
- [ ] …
- [ ] Checkpoint — expect: <observable end state of the phase> — evidence: <verify log path, or command and exit status>, <date>
### Phase 2 — …

## Risks
Assumptions: <a claim about an external system, a consumer, or a machine> — checked by <command>, first task of the phase that depends on it; or none

## Decisions
D<n> — <date> — <decision> — by: <the user's words quoted | cold read round N | implementer>
Open: <question verbatim> — default: <what the implementer does>
Rejected: <alternative> — <why>
```

Point `ROADMAP.md` "Now" at the plan. For a small feature the plan can be short; the sections stay. A task that adds or changes a command, flag, endpoint, config key or deploy step ends `— docs: <file>` and is ticked only when that file says so; other tasks carry no docs clause. A box that will not be done ends as `- [-] <task> — dropped: <why> | superseded by <plan § task> | moved to <handoff path> Next Steps (<date>)`; `[-]` is closed, never remaining work.

## Step 3: Cold-reader check — no code until it passes

Dispatch the `reviewer` agent with **only the plan file**, nothing from the conversation, and this brief:

> You are a fresh reader with no prior context. Read this plan and the code surfaces it names. Answer two things. (1) Gaps: what must be decided before implementation starts that the plan does not decide — interfaces, failure modes, scope edges, misunderstandings of existing code (cite file:line), a Verification or E2E criterion that names an implementation choice, an assumption asserted but not checked, a task changing a command, flag, endpoint, config key or deploy step with no docs clause. (2) Readiness: could a first-pass implementer ship this without asking the author a question? List every question you would have to ask, and mark each BLOCKING (two careful implementers would build different things) or NON-BLOCKING (the plan states a default, or a reasonable implementer decides it alone). End with `design ready` only if you have zero gaps, and `implementation ready` only if you have zero BLOCKING questions; otherwise `design needs revision` / `implementation not ready`.

Address every gap and blocking question in the plan, or rebut it in writing citing `REQUIREMENTS.md`, `AGENTS.md`, or the user's words; write each rebuttal and each non-blocking question as a row under `## Decisions`. Re-run until both tokens are "ready". After three rounds without convergence the feature is under-specified: stop and ask the user which requirement to clarify or cut — and say in the question whether the residue is a real ambiguity or one narrow item you would resolve the same way, with your recommendation first. Tick the Cold-reader check box only on both tokens or a quoted same-turn override; residue the user accepts at the cap is an `Open:` row. Until then, no source edits; reading, `git log`, and running existing tests are fine.

## Step 4: Phases, one at a time

For each phase, implement its tasks in order. For a task whose verify clause names a test, write that test first and run it bare: it must fail for the task's reason, because a test that passes before the code exists is not the test (review-standards § 2); in a compiled language first add the signature from Interfaces with a zero-value body, since a compile error is not the red. Tasks verified by a walkthrough or a container run are unchanged and still need covering tests at Done. After each task, run its named verification and tick its box when the result meets its expect.

Between checkpoints the plan holds the state. At a stopping point inside a phase — a blocker, a long run, the context may compact, the session may end — append to the box you are on `— in progress: <where it stopped>; blocker: none | <what>`, and delete the note when you tick the box. Work found mid-phase is filed when found: a blocker is a new box above the task it blocks, ending `— found in: <task>`; something broken goes through the `bug` skill; deferrable work is one line under Scope's Follow-ups ending `found in: <task>`; a question only the user can decide is an `Open:` row under `## Decisions`, named in the Step 6 report. A decision the plan did not make — an interface detail, a scope edge, a workaround — gets a dated D-row before work continues; a structural one also gets an ADR.

When the phase's tasks are ticked, run the project's full test command **as its own tool call, bare, and read its exit status** (never `test | tail && git commit` — BUG-033/041 in styrman-crew both shipped a red test that way). In a plan with two or more phases, a phase that changed source then gets a `verifier` dispatch over `<commit before the phase>..HEAD` (or the working tree), slug `<plan-slug>-phase-<N>`; its log is committed with the phase. Copy the evidence onto the phase's Checkpoint box from the log or the call, never from memory, and tick it only when the evidence meets its expect — a missed expectation leaves it unticked and the next phase closed. Then write the `changes/` entry for the phase (`changelog` agent or yourself), and commit and push as separate calls. That is the checkpoint. A new session resumes at the first unticked box.

Per-layer verification, typical Go + TanStack Start project: migration applied and schema inspected → package tests with `-race` → API hit with `curl` → UI driven in a real browser (golden path, one edge case) → container built and run locally (`docker build`, `docker run`, `curl /healthz`).

## Step 5: E2E before "done"

Type checks and unit tests prove the code; e2e proves the feature. Service: build the container, run it, call the health check and one real request. Frontend: walk the golden path and one edge case (empty state, error response, unauthenticated) in a real browser. A deploy is not e2e: when the user asked for GCP, the cloud smoke tier runs inside `/preflight` after Step 6 — the only path that gates, tags, and deploys by `<docs-root>/deploy/gcp.md`. Record the outcome in the verification log.

## Step 6: Definition of Done

Steps 1–5 of the global Definition of Done: `verifier` log, `reviewer` (diff only) and `security` when due, `documentation` when behavior, interfaces, the plan, or a deploy path changed, `/review --docs` over the range when an interface, command or deploy step changed, `changelog` naming the agents dispatched, `bugs` for anything that went wrong on the way; before the commit, close the plan — every `[ ]` becomes `[x]` or `[-]`, and a `who: user` box not done moves to the handoff's Next Steps — then commit, push and `/handoff`. Then a report in chat: feature in one line, files by phase, tests added, cold-reader result and how each gap was resolved, review outcome with rebuttals verbatim, e2e result, follow-ups, and every `Open:` row.
