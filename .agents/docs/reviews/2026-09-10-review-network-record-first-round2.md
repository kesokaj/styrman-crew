---
owner: reviewer
date: 2026-09-10
---
# Review — network-record-first, round 2   (2026-09-10)

Range reviewed: uncommitted working-tree diff, `.agents/scratch/network-record-first.diff` (`internal/httpapi/network.go`, `internal/httpapi/network_test.go`, `internal/httpapi/server.go`, `internal/orchestrator/orchestrator.go`), against round 1's report `.agents/docs/reviews/2026-09-10-review-network-record-first.md` and its "Response" section.

## Verdict: APPROVE

Round 1's high finding (persist failure on a drifted-but-already-correct bridge silently under-reporting) is fixed exactly as the response describes: the `!wrongBridge` persist-failure branch now Warns with `requested`/`recordSays` and the reply names the stale mode, and `TestNetworkPersistFailureOnDriftedRecordSaysWhatIsReported` reproduces and asserts it. This round's new surface — `Orchestrator.Rebuild` holding one lock across stop→persist→start, the `EnsureRunning`/`ensureRunningLocked` split, `ModeMismatch`/`currentBridge`, and the handler's `RebuildError.Step` switch — traces cleanly: the split is a byte-for-byte extraction (confirmed against the old body), `Rebuild` never re-enters `lockProject` (no deadlock), the `stopped` marker invariant from BUG-030 (set before stop, cleared only when the stop provably did not happen or a start fully succeeds) holds on every `Rebuild` exit path, and the step switch is total with each branch's message matching the state it describes. `go build`, `go vet`, and `go test ./internal/httpapi/... ./internal/orchestrator/...` (including `-race` on the network tests) are all clean.

## Findings

None at high or medium severity. One low-confidence observation, not filed as a finding because it is pre-existing and unchanged by this diff:

- **[low / confidence: medium]** `internal/orchestrator/orchestrator.go:405-415` (`ensureRunningLocked`) — if `ContainerStart` succeeds but the subsequent `waitReady` times out, the function returns an error without clearing `o.stopped`; the marker (set `true` by `Rebuild` before the stop) stays `true` even though the container may now actually be running. This is the same code as the pre-diff `EnsureRunning` body (the split moved it verbatim — confirmed by diffing the hunk against the extracted function), so it predates this round and is not introduced by `Rebuild`; flagged only for awareness since `Rebuild`'s docstring says the marker is "undone only when the stop provably did not happen" and this is a third case (start partially succeeded) the docstring doesn't name. Not a fail-open in the R12 sense — Docker's own bridge DROP still governs isolation — only a possible stale "stopped" WS-reconnect refusal until the next explicit Start.

## Criteria

1. **Dead code** — pass. `rollbackMode` stays fully removed (`grep -rn rollbackMode` across the tree: no hits); no unused imports (`project` package import removed from `network.go`, still legitimately used in `server.go`).
2. **Flow tracing** — pass.
   - `Rebuild`: `o.stopped.Store(true)` → `stopLocked` fails → `unmarkIfStillRunning` (clears only if container is provably still running) → `RebuildError{"stop"}`; handler's `"stop"` case says "NOT changed", matching (record untouched, container untouched).
   - `stopLocked` succeeds → `persist()` fails → marker stays `true` (correct: container is genuinely down) → `RebuildError{"persist"}`; handler Warns and replies "stopped ... NOT changed ... continue in the previous mode" — matches, record unchanged, container down.
   - `persist()` succeeds → `ensureRunningLocked` fails → marker stays `true` (see low observation above) → `RebuildError{"start"}` falls to the switch's `default`; handler replies "saved, but ... could not be restarted ... start it to apply the change" — matches (record has the NEW value, container down).
   - `ensureRunningLocked` succeeds → clears `o.stopped` at line 415 (the "successful start clears it" the `Rebuild` doc promises) → 200, `rebuilt:true`.
   - `ensureRunningLocked` itself: confirmed it never calls `lockProject`/`Lock` — the caller (`EnsureRunning` or `Rebuild`) holds the lock for its entire body — so no reentrant-lock deadlock.
   - `ModeMismatch`/`currentBridge` vs. the refactored `bridgeModeMismatch`: `currentBridge` factors out exactly the old `NetworkInspect` + `errdefs.IsNotFound` + `Options[...]` logic; `bridgeModeMismatch`'s NotFound→"not a mismatch" and real-error→propagate behavior is preserved verbatim through the new `found`/`err` return shape. `ModeMismatch`'s NotFound→"mismatch, rebuild" is a deliberately different answer for RUNNING containers only (documented, and the two call sites — `EnsureRunning`'s stopped-container path vs. the handler's running-container path — never share a code path that would apply the wrong rule).
   - Handler step switch (`network.go:196-219`): `"stop"`, `"persist"`, `default` (covers `"start"` and any unrecognized/unwrapped step) — total, and `errors.As` always succeeds for the real orchestrator and `fakeMode`, both of which only ever return `*RebuildError`.
   - `stopIfIdle`, `Stop`, `unmarkIfStillRunning`, `lockProject`, and the idle reaper are byte-unchanged by this diff; `Rebuild`'s marker handling reuses the same `stopLocked`/`unmarkIfStillRunning` calls under the same lock discipline, so no new interaction with the reaper beyond ordinary per-project lock contention (serialization, not deadlock).
3. **Standards conformance** — pass. Errors wrapped with `%w` or carried through `RebuildError.Unwrap`; no discarded errors; lock/netMu ordering is one-directional (project lock, then `netMu`, never the reverse — confirmed `deleteProject`'s `netMu` use has no orchestrator call, hence no project lock, while it is held).
4. **Directory layout** — pass. Everything stays in its existing package/file; `RebuildError` and `Rebuild` sit beside `EnsureRunning`/`stopLocked` in `orchestrator.go`; no new files.
5. **Simplicity** — pass. `Rebuild` is the minimum needed to close the TOCTOU the round-1 docstring names (a concurrent start slipping between a separately-called Stop and EnsureRunning); no new dependency; `ModeMismatch` and `currentBridge` are a one-method addition plus a shared helper, not a new abstraction layer.
6. **Decoupling** — pass. `modeRuntime` interface grows/shrinks to exactly what the handler now calls (`ModeMismatch`, `Rebuild` replacing `Stop`+`EnsureRunning`); `fakeMode` models the same seam with no reach-in to orchestrator internals; `persist` is passed as a plain closure, keeping the record-write concern in `httpapi` while `Rebuild` only sequences it — no layering violation.

## Not examined

- Live integration/e2e suites (`testing/styrman/integration.sh`, `web/e2e`) were not run — this review is unit-test-and-code-reading only, as scoped. `go test ./internal/httpapi/... ./internal/orchestrator/...` (plain and `-race`) was run and is clean.
- `internal/project/store_lan_test.go` / `SetLanAccess`'s own on-disk save semantics — read only as much as needed to trust the persist-failure tests' `os.Chmod` mechanism.
- No security review of orchestrator code outside `Rebuild`, `ensureRunningLocked`/`EnsureRunning`, `ModeMismatch`, `currentBridge`, `bridgeModeMismatch`, and the marker-discipline functions named in the dispatch.
