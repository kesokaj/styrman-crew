---
owner: bugs
date: 2026-09-08
id: BUG-017
status: fixed
area: internal/netpolicy, cmd/styrman (per-project LAN access, ADR-065)
---
# BUG-017: four low-severity leaks/gaps in the LAN-access status/marker surface

## Symptom
Security review of the LAN-access server/UI half found four related lows,
none independently severe: (a) `Derive`'s per-project error/conf_error text
is returned verbatim to the project's own owner via `lanMessage`, and root's
error strings quote the operator's LAN CIDR (`internal/netpolicy/status.go:143,180`);
(b) the marker dir was `0o755` under a `0755` parent, so any local host uid
could enumerate which project UUIDs hold a standing LAN exception
(`cmd/styrman/main.go:39`, `internal/netpolicy/dir.go:89`); (c) `Derive` had
no guard against a status timestamped in the future, so clock skew made a
status permanently "fresh" (`status.go:138`); (d) `deleteProject`'s marker
removal ran outside `netMu`, letting a concurrent enable PUT recreate the
marker between removal and project-store removal
(`internal/httpapi/server.go:1018-1021`).

## Repro
Not reproduced live; found by code reading in
`.agents/docs/reviews/2026-09-08-security-lan-phase2-3.md` (lows, in order:
`status.go:143/180`, `main.go:39` + `dir.go:89`, `status.go:138`,
`server.go:1018-1021`).

## Root cause
Each is a boundary the surrounding design already enforced elsewhere but
missed here: (a) the invariant "a tenant never learns the operator's LAN
prefix" was enforced for the `host` object but not for per-project error
text passed through the same handler; (b) root already used `0640` for the
status file for exactly this reason, but the marker dir copied the parent's
default mode instead; (c) staleness was checked one direction (too old) but
not the other (from-the-future); (d) the marker removal was written as a
single-purpose cleanup step without noticing it needed the same lock the PUT
path already takes.

## Fix
Non-admin `lanMessage` now returns a fixed generic string instead of the
verbatim root text; marker dir created/kept `0o750`; `AppliedAt.After(now +
skew)` now also counts as stale; `deleteProject`'s marker removal moved
inside `s.netMu`. In the working tree per the review; lands with the Phase 2
checkpoint commit that follows this record.

## Prevention
When an invariant ("tenant never learns X") is enforced for one object
returned by a handler, grep every other field the same handler returns for
the same tenant before calling the invariant satisfied. A lock taken around
one mutation site (PUT) must be checked against every other mutation site of
the same state (delete) — search callers of the guarded state, not just the
handler being written.

## Related
`.agents/docs/reviews/2026-09-08-security-lan-phase2-3.md`, lows section;
BUG-014 (same package, fail-open reporting, same review).
