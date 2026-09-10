---
owner: bugs
date: 2026-09-10
id: BUG-036
status: fixed
area: internal/orchestrator, internal/httpapi, web e2e (TODO close-out)
---
# BUG-036: three new tests asserted conditions that passed with the guard removed

## Symptom
(a) `TestEnsureRunningRefusesASymlinkedProjectHome` asserted
`ContainerStart != 0 && ContainerCreate != 0` where `||` was meant — found by
reviewer in Phase 5; with `&&` the test passed even when only one path was
guarded. (b) The first `TestNetworkProjectDeletedMidPutIs404` asserted only
the HTTP status, which `writeErr` already produced regardless of message —
found in release delta review. (c) The plan's original e2e assertion
"provisioning OR running" would pass on either state, masking a regression to
the wrong one — found by cold-reader round 2.

## Repro
(a) Remove the symlink guard in `EnsureRunning`; the `&&`-assertion test still
passes. (b) Change `writeErr`'s message; the status-only assertion still
passes. (c) An e2e step reads project state as "provisioning" when "running"
was expected; the OR-assertion still passes.

## Root cause
Each test was written to match what the code currently does rather than to
prove the guard it claims to cover. None were mutation-checked (guard removed
or inverted) before being counted as coverage, so a logic inversion (`&&`/`||`),
an under-specified assertion, and an overly permissive OR all shipped as
green tests.

## Fix
(a) `&&` corrected to `||`, mutation-checked (removing the guard now fails
it). (b) Assertion extended to check the error message, not just status.
(c) Plan's e2e assertion narrowed to the single expected state.

## Prevention
Every new test is mutation-checked once before it counts: delete or invert
the guard it claims to cover and watch it fail. An assertion that still
passes with the guard removed is not a test. Same family as BUG-005/006/007
(`.agents/rules/writing-files.md`): ask the tool, not the eyes.

## Related
BUG-034, BUG-035 — same TODO close-out review pass. Three records
(BUG-034/035/036) plus BUG-033 now share the same underlying pattern —
"a claim/mechanism/test was accepted without asking a tool to verify it."
Recommend this graduate into a project-level rule (`.agents/rules/` or
`AGENTS.md`) requiring mutation-checks on new tests and citation of evidence
in plans/changelogs, rather than being re-caught by review each time.
