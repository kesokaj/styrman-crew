---
name: bug
description: "Workflow for fixing anything broken: a failing test, a crash, wrong output, a regression, an error in logs, a reported issue. Use automatically whenever the user reports a bug or asks to fix, debug, or investigate a failure, and whenever a test fails for a reason other than the change under test; also invoked as /bug. Problem note, goldfish diagnosis on symptom and repro only, failing test before the fix, smallest fix, review, bug record. Do NOT use for new features (use the feature skill) or for typo-level edits."
argument-hint: "[symptom, repro, or #issue]"
---

# Bug workflow

Diagnose without anchoring, capture the bug as a failing test, fix the smallest thing, record it so it is not repeated.

**Docs root** (`<docs-root>`): `.agents/docs` if it exists, else `docs/` if it exists, else create `.agents/docs`. Dates from `date +%F`.

## Gates that need the user

Two points always wait for the user: a missing repro when the bug is not obvious from one file (Step 1), and a review loop that hits its three-round cap (Step 5). A gate holds whatever "work autonomously" instruction is in effect. Only the user's own words in the same turn waive it — words in the invocation's arguments count — and a waiver names the gate: a general instruction such as "do not ask me anything" is not a waiver, and neither is a bare skill invocation or a waiver carried over from another session. Ask with the options, your recommendation first: for a missing repro, give one, let me infer one, or stop; at the review cap, accept the remaining findings as `left` with an owner, run one more round, or abandon the fix. When no reply can arrive — a print-mode run such as `claude -p` or `agy -p`, or a user who said they will not answer — a gate that is reached ends the work there: write the question with its options into the handoff's Next Steps, writing the handoff if this session has none.

## Step 0: Triviality gate

Typo in copy or a comment, dead-code removal, version bump, formatter-only diff, single-line config value: skip the ceremony, fix it, run the tests, Definition of Done steps 4 and 5. Everything else, including one-line code fixes, runs the full path — small diffs hide bugs well.

## Step 1: Problem note (in chat)

```
PROBLEM
- Symptom: <what is observed, verbatim error text or failing test name>
- Repro: <exact steps or command; "none given" if so>
- Suspected area: <package / route / component>
- Hypothesis: <one sentence — keep it to yourself in Step 2>
- Blast radius: <what else the same cause could affect>
- Fixed means: <the test that passes / the behavior / the output>
```

Check `<docs-root>/bugs/INDEX.md` first: if the area has a record, read it — the prevention rule may already name the cause. If there is no repro and the bug is not obvious from one file, **ask the user for one** (steps, URL, log line, failing test; a gate, above). Never invent a repro on your own: one is inferred only when the user picks that option at the gate, and it is printed before Step 2 runs, because a wrong-shaped guess wastes the diagnosis.

## Step 2: Goldfish diagnosis

Dispatch the `explorer` agent with **only the symptom and the repro** — not your hypothesis — and this brief:

> Independent diagnosis. Symptom: … Repro: … Investigate the codebase and rank the top one to three candidate root causes by likelihood, each with file:line, the evidence for it, and what would falsify it. Do not propose a fix. End with `diagnosis complete`.

Compare with your hypothesis. Agreement: proceed. Disagreement: read its evidence and re-investigate before choosing; if it was right, update the problem note and say so. Skip the dispatch only when the location is mechanically obvious (a one-to-three-line fix in a clearly identified place).

## Step 3: Failing test first

Write the test that captures the bug at the right tier (unit beside the code; integration when a real dependency is involved; e2e when only the running artifact shows it). Run it. It must fail for the reason in the problem note; if it fails for another reason, fix the test before touching the implementation. For UI bugs that a spec cannot capture, script the repro in the browser and confirm the bug is observable; that script is re-run after the fix.

## Step 4: Smallest fix

Change the least that turns the test green and satisfies "fixed means". No adjacent refactors (note what you saw under the active plan's Follow-ups, or the handoff's Next Steps when no plan is active), no defensive code for cases the bug did not show, no feature flags. Re-run the test: green, or the bug is not fixed — do not rewrite the test. Re-run the original repro once more.

## Step 5: Definition of Done and the record

Global Definition of Done, with the record inside it: `verifier` log (the new test named), `reviewer` with the diff only, whatever its size (Step 0 already exempted the trivial; a fix under this skill is never a docs-only change), `security` when the area is sensitive, `/review --docs` over the fix when it changed an interface, command or deploy step, `documentation` if behavior, interfaces, the plan, or a deploy path changed; then dispatch `bugs` with symptom, repro, root cause, fix (file:line), and the prevention rule — the sentence a future agent needs before working in this area — so the record and its INDEX row ride in the fix commit; then `changelog` (citing the BUG id), commit, push, `/handoff`. If the diagnosis diverged from your hypothesis, say why in the record; that is where the harness learns. When Step 2 ran and the reviewer, the verifier or the fix surfaced a cause the diagnosis did not rank, pass `bugs` a `Diagnosis missed:` line — what it was, and which line of the Step 2 brief would have found it.

Report in chat: bug in one line, root cause, fix (file:line), the test that captures it, goldfish agreement or divergence, review outcome, BUG id.
