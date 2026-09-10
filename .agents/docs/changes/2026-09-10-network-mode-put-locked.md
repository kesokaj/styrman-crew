---
owner: main
date: 2026-09-10
---
# Network-mode PUT now runs under the project lock — record-first restructure, live-bridge no-op

Date: 2026-09-10. The `PUT /api/projects/{id}/network` handler was restructured to hold the per-project lock for the entire decision: re-read the record, inspect the live bridge, decide whether to rebuild, persist, and start. This closes the residual fail-opens from three review rounds (BUG-028/029/031, all v1.25.0 preflight), fixes the toggle-vs-start race structurally, and makes a wrong record recoverable by re-PUT.

## The restructure

**Old path:** write the record first, trigger a rebuild, roll back on failure if needed. `rollbackMode` helper handled the rollback for both the `Stop` branch and the `Status` error branch. The entire operation was write-first.

**New path:** the whole decision runs inside `Orchestrator.WithProject` under the per-project lock (the same lock held by `Start` and `Stop`), against locked bodies (`Project` interface: `Status`, `ModeMismatch`, `Stop`, `EnsureRunning`). The handler (1) re-reads the record under the lock, (2) calls `ModeMismatch` to compare it against the live bridge, (3) decides whether to stop/rebuild/start, (4) persists the record only after the rebuild succeeds, (5) replies. `rollbackMode` is deleted because the record is written only when nothing remains in force — a failed rebuild leaves the running container and old record unchanged.

## No-op against a live bridge — closes security round-3 finding 1

**Before:** the no-op short-circuit (`if mode == currentMode ...`) checked against the RECORD's mode, so a drifted record (wrong prefix on the live bridge's name) would read as "agree, no-op, reply 200" while the mismatch guard outside the handler would eventually fix it. The finding: the gap in reasoning was that the finding itself described a state the handler could not observe.

**After:** `ModeMismatch` now checks whether the record agrees with the **live bridge** (the actual prefix it was created under, inspected from Docker). A missing network under a running container counts as a mismatch — the container is running on an old, now-deleted bridge. If `ModeMismatch` returns true on a RUNNING container, the handler stops it, persists the new record, and starts it on the new bridge. If the record and bridge already agree, it replies 200 with no work. A re-PUT on a drifted record now repairs it — closes the security finding.

## The toggle-vs-start race — closes TODO's second 🟡 item

**Before:** `RunOpts.Lan` was a `bool` snapshot of the record read at the API handler boundary, outside any lock. A concurrent `POST /start` could read the OLD record and bring the container up on the old bridge, while a `PUT /network` toggled it to sandbox and the record said so but the container was wrong.

**After:** `RunOpts.Lan` is now `func() (bool, error)`, evaluated by `ensureRunningLocked` under the project lock at the moment it decides the bridge name (`lanOf(sp, id)` at all four call sites; nil = sandbox). The name always reflects the record as of the start, read at the instant it is used. The same lock that serializes the PUT also serializes all starts — every interleaving is closed structurally. Security round 3 confirms the residual is a strict subset of v1.25.0's.

## Locked body splitting

Factored `Stop` into lock + `stopMarked`, and `EnsureRunning` into lock + `ensureRunningLocked`. Both now call `currentBridge` to inspect the running container's bridge prefix for the mismatch check, sharing the same logic.

## Tests and mutations

`network_test.go`: 10 → 18 `func Test`. Two rollback-named tests replaced (no rollback mechanism to test anymore); 8 new: `TestModeMismatchErrorNoLiveNetwork` (missing network under running container read as mismatch), `TestDriftedRecordRePUTRepairs` (re-PUT with the same mode repairs a drifted record), `TestRecordBehindBridge` (record wrong while bridge is correct), `TestNoOpDecisionOnAgreement` (same mode, bridge prefix matches, reply 200), `TestPersistFailureAfterStop` (stop succeeds, persist fails, no start), `TestPersistFailureOnDriftedRecord` (drifted record, persist fails before decision to start), `TestLanOfReadsAtCallTime` (`RunOpts.Lan` reads the record at the instant it is used, under the lock), `TestNoOpDecisionUnderTheLock` (the entire decision runs under the project lock). Mutation-checked: 6 mutations each killed exactly one intended test.

## Verification

- `go build ./...`, `go vet ./...` all packages and ingress module clean.
- `go test -race` all packages + ingress module: 194 tests + 3 ingress tests, all PASS, no race detectors.
- Web: check + lint + test all PASS.
- Integration suite on real Kata, 3 runs by main: `passed=84 failed=0 skipped=3` each, no fd/memory leaks.
- Security round 4 appended to round 3 report (`.agents/docs/reviews/2026-09-10-security-network-record-first.md`): residual low about handler writing reply while holding lock (deferred to the TODO-added 🟢 item below).

Verifier log: `.agents/docs/testing/2026-09-10-verify-network-record-first.md` (4 rounds total: initial + round-2 fixes + round-3 fixes + round-4 finalization).

## Real defects found and fixed

Working tree (all before the tag, never released):
- (Round 1 → fixed) Missing network under a running container read as "no mismatch" on a network inspect ERROR — moved the error branch to the fail-closed path.
- (Round 2 → fixed) The first restructure widened the toggle/start race: stop and persist were separate calls on the record-only path — restructured so both the record-read and the start run under the same lock.
- (Round 3 → fixed) The no-op decision compared against the pre-lock record copy, not the live bridge — `ModeMismatch` now inspects Docker before deciding.
- (Round 4 → fixed) The success log condition was too loose — corrected to log only after the full sequence.

## Agents dispatched

verifier × 4 (round 1 + re-verifications after rounds 2/3/4), reviewer × 3, security × 4 (rounds 1–3 + narrow round 4), documentation, changelog, bugs.

## Coverage gap reported

The new orchestrator lock-holder code (`ensureRunningLocked`, `stopMarked`) has no orchestrator-package unit test (BUG-003 residual — no fake Docker client). The integration suite exercises the paths end-to-end; unit isolation of these locked bodies remains deferred.
