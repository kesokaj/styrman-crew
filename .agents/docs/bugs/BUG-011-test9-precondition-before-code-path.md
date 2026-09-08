---
owner: bugs
date: 2026-09-08
id: BUG-011
status: fixed
area: deploy/firewall (test.sh)
---
# BUG-011: test 9 planted its precondition before the run, so it passed regardless of the EXIT trap

## Symptom
Review (`.agents/docs/reviews/2026-09-08-review-lan-phase1.md`, finding 3):
`deploy/firewall/test.sh:127-135` test 9, named "spool drained on a FAILED
run (no re-trigger loop)", wrote its spool entry
(`${RUN}/spool/${U}.456`) *before* the failing run started. The script's
unconditional drain-first step consumed it regardless of whether the `on_exit`
EXIT trap worked at all — the assertion passed either way, so the test did
not prove what its name and the script's comment
(`styrman-firewall.sh:275-282`) claimed.

## Repro
Not a live bug in the script itself (drain-first already gives the safety
property checked); the defect is that the test could not have caught a
regression in the EXIT trap.

## Root cause
The precondition for the recovery path under test (an entry arriving mid-run,
after drain-first, before a failure) was set up before the run started
instead of during it, so the test exercised a different, already-safe code
path (drain-first) rather than the one it named.

## Fix
The failing command now plants the spool entry mid-run, inside the code path
actually under test.

## Prevention
A test of a recovery path (trap, retry, cleanup-on-failure) must make the
precondition arise *inside* the code path under test, not by pre-seeding
state before the run starts — otherwise an earlier, unrelated step in the
script can satisfy the assertion and the test proves nothing about the
mechanism it names.

## Related
BUG-010 (found during the same test-9 rework).
