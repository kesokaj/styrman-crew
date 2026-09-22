---
owner: bugs
date: 2026-09-22
id: BUG-078
status: fixed
area: web/e2e/run.mjs
---
# BUG-078: a failing e2e check left the explorer in the wrong place, cascading 8 more failures

## Symptom
While proving BUG-077's replacement check could fail (by suppressing the
truncation notice's render), the check left the file explorer at the
wrong path when it failed, and eight later checks that depend on the
explorer's position failed too: `passed=84 failed=9` for one cause.

## Repro
Deliberately suppress the truncation notice's render and run the
truncation check; pre-fix, the explorer is left navigated into the
5200-file directory and the 8 subsequent position-dependent checks fail
alongside it.

## Root cause
The check navigated the file explorer as part of its setup but had no
cleanup on the failure path, so a failure in this check left shared UI
state (the explorer's position) wrong for every check that ran after it.

## Fix
The check restores the explorer's position in a `finally`.

## Prevention
An e2e check that navigates restores its starting position in a
`finally`, or its single failure is reported as many — and this is only
visible if you make the check fail deliberately.

This rule graduated into `.agents/rules/e2e-checks-leave-the-suite-as-they-found-it.md` on 2026-09-22.

## Related
BUG-077 (the check this cascade was discovered while fixing). Same family
as BUG-021 (an e2e check placed before state-dependent checks cascaded 17
unrelated failures) — recurring pattern of e2e checks not isolating their
side effects from the checks that follow; three occurrences (BUG-021,
BUG-024, BUG-078) in `web/e2e/run.mjs`/`run.sh` suggest the isolation rule
belongs in the project's e2e-authoring convention (the `run.mjs:190`/
`:202-206` comment block, or a new dedicated comment) rather than being
rediscovered per-check.
