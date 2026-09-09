---
owner: bugs
date: 2026-09-09
id: BUG-029
status: fixed
area: internal/httpapi/network.go (ADR-067 network-mode PUT)
---
# BUG-029: BUG-028's fix closed the fail-open on `Stop` and left its sibling on `Status`, one line above

## Symptom
Found by the `/preflight` reviewer on the release candidate, hours after
BUG-028 was fixed and its test mutation-checked. No suite caught it: the
branch only fires when `docker inspect` itself fails.

## Repro
Project running in LAN mode. PUT `lanAccess:false` while the Docker API is
unreachable or `inspect` errors. The handler answered **200** with the record
already set to sandbox, stopped nothing, rebuilt nothing — and the container
stayed on its `lan-` bridge with full LAN reach, reported everywhere as
sandboxed, with nothing to reconcile it (the mismatch heal fires for stopped
containers only).

## Root cause
    st, err := s.mode.Status(ctx, ...)
    if err != nil || st.State != "running" {   // "nothing to do"

`Status` maps a MISSING container to `("absent", nil)`. So a non-nil error is
never "the container is gone" — it is "I could not look", and the container
may be running. The condition folded a real inspect failure into the
harmless not-running case. BUG-028's fix, written the same afternoon, went
straight to the `Stop` branch below this line and did not read the line
above it.

## Fix
The error branch is separated from the not-running branch and takes the same
path a failed `Stop` does: roll the record back, log, reply 500 saying the
mode was NOT changed. The rollback is now one helper, `rollbackMode`, used
from both. Tests appended to `network_test.go`:
`TestNetworkStatusErrorOnRevocationRollsBack` (mutation-checked — deleting
the rollback call makes it fail) and
`TestNetworkAbsentContainerJustWritesTheRecord`, pinning that "absent" keeps
its old meaning, since telling a failure from an absence is the whole fix.

## Prevention
When a fix closes a fail-open on one branch of a handler, walk EVERY branch
above and below it with the same question — "what does the record say versus
what is in force?" — before calling the class closed. And an `err != nil`
folded into a state check by `||` deserves a second look every time: the two
are only interchangeable when the callee already maps absence to a state,
and then the error means something else.

Related: BUG-028 (same handler, same class, the branch below), BUG-022 (the
same handler's other second-attempt fix today).
