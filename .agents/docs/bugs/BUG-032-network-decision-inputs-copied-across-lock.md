---
owner: bugs
date: 2026-09-10
id: BUG-032
status: fixed
area: internal/httpapi/network.go (PUT /api/projects/{id}/network)
---
# BUG-032: three rounds, each leaving one more copy of a lock-protected value read outside the lock

## Symptom
Found across three security reviews of the working-tree restructure that
removed the record-first-then-rollback design
(`reviews/2026-09-10-security-network-record-first.md` and its `-round2.md`,
`-round3.md`). Never shipped — v1.25.0 is the released version, this is
all working-tree. No production repro; each finding was a race a reviewer
traced by hand.

## Repro
Not reproduced live. Traced shapes, one per round:
- Round 1: stop, record write, and restart were three separate orchestrator
  calls with the per-project lock released between them; a `POST /start`
  parked on the lock during the stop, holding the OLD record, resumed
  between write and restart and started the container on the old `lan-`
  bridge while the record already said sandbox.
- Round 2: the record-only branch (stopped/absent container) persisted with
  no lock at all, and `RunOpts.Lan` was a caller-held snapshot taken before
  the lock — a start straddling the PUT lost the same race with zero
  container faults.
- Round 3: the no-op decision inside the now-locked closure still compared
  against `p.LanAccess`, a copy `getProjectFor` took BEFORE the lock; two
  concurrent PUTs (enable, then disable) on a stopped project could leave
  the record saying LAN under a 200 reply saying sandbox.

## Root cause
A value that the per-project lock protects (the network mode) was carried
INTO the locked decision as a copy taken before the lock was held — once as
three lock-released calls, once as a struct field (`RunOpts.Lan`), once as a
handler-local (`p`). Each round's fix closed the copy the previous round's
reviewer had found; each round's review found the next one, because closing
one copy does not audit the others.

## Fix
Working tree only. `Rebuild` became one atomic call under a single lock
hold; `RunOpts.Lan` became `func() (bool, error)` evaluated under the lock
by `ensureRunningLocked`; the handler's whole decision moved inside
`Orchestrator.WithProject`, and the no-op branch re-reads the record
(`sp.projects.Get`) as the first act under the lock instead of using the
pre-lock `p`. Test: `TestNetworkNoOpDecisionReadsTheRecordUnderTheLock`
(fake `onLock` hook plants the change at lock-acquisition time; mutation
checked). Also fixed in round 1: `ModeMismatch`'s shared "missing network is
not a mismatch" answer was correct for start but wrong for a running
container on the mode-change path (leaves it on no styrman bridge at all);
each caller now gets its own decision over a shared `currentBridge`.

## Prevention
When a decision moves under a lock, every INPUT to that decision must be
(re)read under the same lock — enumerate the inputs (record, container
state, bridge) and check each read site is inside the hold. A struct field
or parameter carrying a lock-protected value across the lock boundary is
the defect, however it was obtained; a reviewer closing one instance must
grep for the same shape (any pre-lock local later used inside the locked
block) rather than trusting the single fix. Second, narrower rule: a shared
helper whose "not found"/error default is a safe answer for ONE caller is
not automatically safe for another — name the default per caller, as
BUG-028/029/031 already established for `err != nil` defaults.

## Related
BUG-028, BUG-029, BUG-031 (the rollback design this replaced, same
revocation path, same reviewer-finds-the-next-copy pattern — the "trace
what the caller does with the default" rule from BUG-029/031 and this
record's "trace every read site of a lock-protected input" rule are close
enough to graduate together into `internal/orchestrator`'s AGENTS.md or a
`.agents/rules/` file). BUG-003 (no fake Docker client — why this path still
has no orchestrator-level unit test). BUG-030 (the `stopMarked` discipline
this design preserves).
