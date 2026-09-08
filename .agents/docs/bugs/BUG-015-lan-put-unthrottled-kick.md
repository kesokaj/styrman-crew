---
owner: bugs
date: 2026-09-08
id: BUG-015
status: fixed
area: internal/httpapi/network.go (per-project LAN access, ADR-065)
---
# BUG-015: no-op LAN PUT still fired the one unthrottled kick, letting any owner drive root's reconciler to its trigger limit

## Symptom
Security review: `PUT /api/projects/{id}/network` called `Kick`
unconditionally after `SetLanAccess`, even when the value was unchanged
(`SetLanAccess` itself no-ops on no change). Disable needs only the project
owner, no admin gate, no availability gate. Any authorized tenant looping
`{"lanAccess":false}` against their own project could drive root's
reconciler at the ceiling of `TriggerLimitBurst=20/60s`; once tripped, the
`.path` unit fails and event-driven reconciliation (including a genuine
disable by anyone else) stops for up to 10 minutes.

## Repro
Not reproduced live; found by code reading in
`.agents/docs/reviews/2026-09-08-security-lan-phase2-3.md` (finding 2,
`internal/httpapi/network.go:140`).

## Root cause
Every other kick site in the codebase used `KickThrottled(p.ID, 10s)`; the
PUT handler was written to kick at once so a genuine change reconciles
immediately, but the "unchanged value" branch was not distinguished from a
real change, so the fast/unthrottled path was reachable by the lowest-
privileged caller who can reach the route at all.

## Fix
`network.go:140`: a no-op PUT now uses the throttled kick
(`KickThrottled(p.ID, kickThrottle)`); only an actual change kicks at once.
In the working tree per the review; lands with the Phase 2 checkpoint commit
that follows this record.

## Prevention
Any request path that can make root do work must be rate-limited per
principal/object BEFORE it is exposed to the lowest-privileged caller who
can reach it. Audit who can reach the endpoint (owner, not just admin), not
who is expected to use it normally, when deciding whether a kick needs a
throttle.

## Related
`.agents/docs/reviews/2026-09-08-security-lan-phase2-3.md` finding 2;
BUG-016 (same reconciler/spool surface, availability angle);
BUG-012 (prior finding in deploy/firewall: unbounded/unthrottled root-facing
input from a lesser uid — same family, different layer).
