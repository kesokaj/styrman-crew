---
owner: main
date: 2026-09-09
---
# ADR-067 preflight — the fail-open had a sibling, and a stuck marker beside it

The Phase 6 gate was not the final gate. `/preflight` dispatched a fresh `reviewer` (Opus) over commits not yet covered (`5481ea2..eccd1a0`), verdict REQUEST CHANGES: 1 medium, 3 low (report: `reviews/2026-09-09-preflight-review-v1.25.0.md`). All findings were real defects; two became BUG-029 and BUG-030. This marks this repo's second time the same handler needed a second attempt: BUG-022 was the first.

## The findings

**Finding 1 (medium/high): the fail-open had a sibling — BUG-029.** BUG-028's fix closed the fail-open on the `Stop` branch (writing the record *after* the kill). But `Status` has its own branch: `if err != nil || st.State != "running"`. That treats a `Status` error (inspect failure) as "stopped", when the container may actually be running on the old `lan-` bridge and the real problem is that Docker inspect failed. A revocation there persisted `lanAccess:false`, replied 200, and left LAN in force with nothing to reconcile it.

Fixed: the error branch is now separate and takes the same rollback-plus-500 path as the stop-failed case. The rollback was factored into one helper `rollbackMode` used by both branches. New test `TestNetworkStatusErrorOnRevocationRollsBack`, mutation-checked (removing the rollback call makes it fail); test `TestNetworkAbsentContainerJustWritesTheRecord` confirms that "absent" keeps its old meaning. `network_test.go` was also the one file `gofmt -l` named in the repo (finding 3, low).

**Finding 2 (low/high): the stuck marker — BUG-030.** `Orchestrator.Stop` stores the `stopped` marker *before* calling `stopLocked` (deliberately: terminal auto-reconnects must see it before the kill), and never cleared it on failure. After a failed stop the project is running exactly as before, yet every reconnect is refused with close 4002 "project stopped" until someone clicks Start. The same shape existed in the idle reaper.

Fixed in `Stop` and the reaper: on `stopLocked` error, delete the marker and return the error. No unit test is possible (no fake Docker client; BUG-003's residual); recorded as such.

**Finding 4 (docs): stale ADR-065 in versioning.** `specs/styrman/versioning.md` still carried an "ADR-065 host-side files" paragraph describing the `.path` unit, the spool tmpfs, the marker dir and a UI that "reports LAN access as unavailable" — all deleted by ADR-067. Rewritten to the ADR-067 model, including the post-upgrade cleanup caveat (the leftover units survive an auto-update by design and are converged away by the next operator-run deploy).

## Verification

- `gofmt -l` clean outside `web/`.
- `go build ./...` and `go vet ./...` clean.
- `go test ./... -race`: 15 packages ok (194 tests, ingress module ok), httpapi race-clean.
- Mutation check performed on new test `TestNetworkStatusErrorOnRevocationRollsBack`: removing the rollback call makes it fail.
- Broader verification (integration.sh, e2e, etc.) in `testing/2026-09-09-verify-preflight-fix-v1.25.0.md`.

**Note on review cycles:** This is the second time a fix to the network mode handler needed correction on a second review pass. BUG-022 was the first. The gate exists because a single reviewer pass and a verification pass do not always catch everything that two independent review passes do. This entry is not a regression; it is the gate working.

## Addendum, same gate: a third fail-open, one layer down (BUG-031)

The `/preflight` security review of the committed range
(`reviews/2026-09-09-preflight-security-v1.25.0.md`) found the same class a
third time, below the handler: `bridgeModeMismatch` returned `false` — "no
mismatch" — on a network-inspect ERROR, so on a revocation `EnsureRunning`
would have started the just-stopped container on its old `lan-` bridge with
the record saying sandbox. It now returns `(bool, error)`; NotFound is still
"not a mismatch" (`recreateMissingNetwork` owns it), any other error refuses
the start — the container stays down, which is the safe state. No unit test
is possible (no fake Docker client, BUG-003); the handler tests above it
cover the branch that receives the error.

The same review noted that ADR-067's third amendment overclaimed "a failed
revocation now FAILS CLOSED" while only the `Stop` branch was closed. With
BUG-029 and BUG-031 fixed the claim is now true of every branch that can
leave a container running; the one branch that cannot be made to fail closed
— a failed rollback WRITE — logs at ERROR and needs a human. A correcting
note is appended to the amendment.

## Addendum: two of round 2's lows, taken

Round 2 (`reviews/2026-09-09-preflight-review-v1.25.0-round2.md`, approve
with nits) closed findings 1-3 and left three lows. Two were taken because
each was a correctness edge the fix itself had introduced:

- **The marker delete is now gated.** BUG-030's first draft cleared the
  "deliberately stopped" marker on any failed stop. A half-succeeded stop, or
  a reaper tick racing a user's Stop, leaves the project genuinely down — and
  an unconditional delete would let the next terminal reconnect boot it back.
  `unmarkIfStillRunning` re-inspects and clears only on `State.Running`; an
  inspect failure keeps the marker. BUG-030's record updated.
- **A rollback that fails with `ErrNotFound` logs Info, not Error.** The
  project was deleted mid-change; there is no record left to disagree with
  anything and the delete path removes the container. A race, not an
  incident.

The third low — no unit test for the orchestrator changes — stands, and is
recorded in BUG-030 and BUG-031. It is BUG-003's residual, again.

