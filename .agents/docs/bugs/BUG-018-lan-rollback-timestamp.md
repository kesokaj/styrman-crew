---
owner: bugs
date: 2026-09-08
id: BUG-018
status: fixed
area: internal/httpapi/network.go, internal/project (per-project LAN access, ADR-065)
---
# BUG-018: PUT rollback after a failed marker write re-stamped LanChangedAt to the failed request's time

## Symptom
Reviewer finding (not security): the rollback after a failed `SetMarker`
called `sp.projects.SetLanAccess(p.ID, prev, now)` using the failed
request's `now`. Since the rollback is itself a value change (on back to
`prev`), it stamped `LanChangedAt` forward instead of restoring the true
prior timestamp — shifting `Derive`'s pending→error grace window
(`PendingGrace`, 90s) on a later read of the same still-unchanged value.

## Repro
Not reproduced live; found by code reading in
`.agents/docs/reviews/2026-09-08-review-lan-phase2-3.md` (finding 1,
`internal/httpapi/network.go:1028-1034`).

## Root cause
`SetLanAccess`'s existing "skip timestamp on unchanged value" rule does not
apply to a rollback, because the rollback IS a write of a new (reverted)
value from the store's point of view — the handler treated "restore the old
value" as equivalent to "set a new value with the caller's `now`", losing
the distinction between a real change and an undo.

## Fix
Added `project.RestoreLanAccess`, which reinstates the exact prior
`(LanAccess, LanChangedAt)` pair instead of re-stamping. In the working tree
per the review; lands with the Phase 2 checkpoint commit that follows this
record.

## Prevention
A rollback of a value+timestamp pair must restore both fields exactly, not
call the same "set" path used for forward changes with the current
request's clock — a dedicated restore function (or an explicit prior-
timestamp argument) is needed wherever a mutation can be undone.

## Related
`.agents/docs/reviews/2026-09-08-review-lan-phase2-3.md` finding 1.
