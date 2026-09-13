---
owner: bugs
date: 2026-09-13
id: BUG-050
status: fixed
area: image (launcher) test suite — TestLauncherKillGroupFile
---
# BUG-050: test asserted a killed process was gone while its own zombie still showed up

## Symptom
`TestLauncherKillGroupFile` failed even though the kill worked: the leader
process, started via `exec.Command.Start()` and never `Wait()`ed, became a
zombie that `pgrep -g` and `/proc` still listed as present.

## Repro
Run the test as originally written; observe the "process gone" assertion
fail despite the kill succeeding.

## Root cause
The test never reaped the child it started, so after the kill the process
existed in the process table as a zombie (state Z) — a state the assertion
did not distinguish from "still running".

## Fix
Added `go leader.Wait()` so the test process reaps its own child.

## Prevention
A test that asserts a process is gone must either reap the child it started
(`Wait()`) or explicitly check for/allow zombie state (Z) — otherwise the
test's own hygiene produces the exact symptom it's checking for.

## Related
Batch .agents/docs/plans/2026-09-13-browser-review-fixes.md.
