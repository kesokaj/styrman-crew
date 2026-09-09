---
owner: bugs
date: 2026-09-09
id: BUG-030
status: fixed
area: internal/orchestrator/orchestrator.go (Stop, idle reaper)
---
# BUG-030: a failed stop left the "deliberately stopped" marker set on a running project

## Symptom
Found by the `/preflight` reviewer while reading BUG-028's rollback. After a
mode change whose `Stop` failed, the API said the mode was not changed and
the project was indeed still running — but every terminal auto-reconnect was
refused with close 4002 "project stopped" until someone pressed Start.

## Repro
Any `Orchestrator.Stop` (or idle-reaper stop) whose `stopLocked` returns an
error: a Kata shutdown that wedges, a Docker timeout. The container keeps
running; the UI's terminals cannot reconnect to it.

## Root cause
`Stop` stores `o.stopped[key]=true` BEFORE calling `stopLocked`, on purpose:
the stop kills every attached terminal exec, and their reconnects must
already see the marker or the first one boots the VM straight back up. The
ordering is right. What was missing is the branch where the stop never
happens — the marker stayed set, describing a state that did not exist. The
idle reaper had the identical shape.

## Fix
On `stopLocked` error, clear the marker **only if a re-inspect shows the
container still Running** (`unmarkIfStillRunning`), then return the error —
in `Stop` and in the reaper. Marking first is unchanged. The first draft
deleted unconditionally; the second review round pointed out that a stop can
half-succeed (daemon took the request, transport failed) or a reaper tick
can race a user's Stop, and then the project IS down and an unconditional
delete would let the next terminal reconnect boot it straight back — the
resurrection the marker exists to prevent. An inspect that fails keeps the
marker: the user can always press Start; a resurrected VM is a surprise. **No unit test**:
`internal/orchestrator` has no fake Docker client (BUG-003's residual), so
this is covered by reading and by the httpapi tests that exercise the
handler above it, not by a test of `Stop` failing.

## Prevention
A flag set optimistically ahead of an operation must be cleared on every
branch where the operation does not complete; "set before, for ordering" and
"clear on failure" are two halves of the same invariant. When a fix touches
a failure branch, look at what else that branch's caller latched before
calling.

Related: BUG-028/029 (the handler this surfaced in), BUG-003 (why it has no
test).
