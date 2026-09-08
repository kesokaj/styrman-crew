---
owner: bugs
date: 2026-09-08
id: BUG-012
status: fixed
area: deploy/firewall (design: spool + units)
---
# BUG-012: uid-writable spool on /run, drained before base rules, could fill /run or drive an unthrottled root loop

## Symptom
Security review (`.agents/docs/reviews/2026-09-08-security-lan-phase1.md`,
finding 3, medium): the spool
(`deploy/firewall/styrman-firewall.sh:84-113`) was an unbounded uid-writable
directory on the root tmpfs `/run`, drained (84-113) *before* the R12 base
rules (115-176). Two shapes: (a) a lower-privileged uid could fill `/run`
via repeated `mktemp` until it aborted the whole host's `/run`-dependent
services; (b) `StartLimitIntervalSec=0` plus an unconditional revive at
`:402-406` let that same uid keep re-arming a root oneshot at run-duration
cadence, each pass issuing ~20 iptables/nft transactions contending with
dockerd's own.

## Repro
Not exploited live; mechanism reasoned from unit file settings and script
ordering by the security reviewer, not executed against a host.

## Root cause
The spool and its consuming units were designed for the reconcile logic
(correctness) without a resource/rate budget for the uid that controls the
spool's inputs, and were sequenced ahead of the base enforcement section
they were meant to extend, not gate.

## Fix
Dedicated 1 MiB / 1024-inode tmpfs for the spool (bounding fill); base
section moved first (host stays enforced regardless of spool state);
`StartLimit` set to 20/60s; revive backoff raised to 10 minutes.

## Prevention
Any directory a lower-privileged uid can write that a root unit consumes
needs an explicit size/entry cap and a rate limit on the root side before it
ships — "unbounded input, no rate limit, root consumer" is a resource-
exhaustion/DoS shape to check for on every privileged reconcile loop, not
just this one. Base/enforcement sections run before any section whose
timing a lower-privileged uid can influence.

## Related
Security review 2026-09-08; BUG-013 (same review, same script).
