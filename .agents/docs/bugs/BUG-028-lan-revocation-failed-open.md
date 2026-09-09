---
owner: bugs
date: 2026-09-09
id: BUG-028
status: fixed
area: internal/httpapi/network.go (ADR-067 network-mode PUT)
---
# BUG-028: a failed LAN revocation left LAN in force while every surface reported sandbox

## Symptom
Found by the security review (`reviews/2026-09-09-security-adr-067.md`,
finding 1, medium/high), not by a suite. No user report — the failure mode
is invisible by construction: everything that could have shown it read the
record, and the record was wrong.

## Repro
Put a running project in LAN mode. PUT `lanAccess:false`. Make `Stop` fail
(a wedged Kata shutdown, a Docker timeout). The API answers 500 — but the
record already says `lanAccess:false`, so the switch, the sidebar badge and
`GET /network` all say "sandboxed", while the container is still running on
its `lan-` bridge with full LAN and host reach. Nothing corrects it: the
bridge-mismatch heal in `EnsureRunning` fires for STOPPED containers only.
The state persists until someone stops and starts the project by hand.

## Root cause
`network.go` persisted the new mode FIRST, then stopped and rebuilt. That
order is right for the sandbox→LAN direction (a failure leaves the project
with less access than the record claims — fail closed) and wrong for the
revocation direction (more access than claimed — fail open). The handler
did not distinguish the two, and the error message — "saved, but the project
could not be stopped to apply it" — described the fail-open as if it were
merely pending.

The deeper cause is the deletion ADR-067 made deliberately: ADR-065/066 had a
reconcile ticker and a derived state that would eventually have noticed the
mismatch. ADR-067 removed both, on the reasoning that the bridge name IS the
state so there is nothing to derive. True for a container that restarts;
not true for one that stays up.

## Fix
When `Stop` fails, roll the record back to the previous mode before
returning the error, and say the mode was NOT changed. A failed rollback is
logged at ERROR — that state needs a human. The other branch (`Stop`
succeeded, restart failed) deliberately keeps the new record: the container
is down, nothing is in force, and the next start applies it.

Testing this needed the failure to be reachable: `internal/httpapi` gained a
`modeRuntime` seam (Status/Stop/EnsureRunning), set from the orchestrator in
`New()` the same way `guestRuntime` was in v1.22.0. Two tests appended to
`network_test.go`: `TestNetworkFailedRevocationRollsBackTheRecord`, which
was mutation-checked (a no-op rollback makes it fail), and
`TestNetworkFailedRestartKeepsTheRecord` for the branch that must not roll
back.

## Prevention
A state change with an apply step must decide, per DIRECTION, what the
record should say if the apply fails — and the answer is "what is actually
in force", never "what was requested". Where an earlier design's reconcile
loop is deleted as redundant, list what it would have caught and check each
item still has an owner; here one did not.

Related: BUG-014 (ADR-065's version of the same question: derive a
privilege's reported state from what is confirmed, never from a request),
BUG-022/BUG-023 (the same handler), BUG-003 (the missing orchestrator seam
that made this untestable until one was added).
