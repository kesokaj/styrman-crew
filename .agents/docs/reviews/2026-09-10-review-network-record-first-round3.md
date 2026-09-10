---
owner: reviewer
date: 2026-09-10
---
# Review — network-record-first, round 3   (2026-09-10)

Range reviewed: uncommitted working-tree diff, `.agents/scratch/network-record-first.diff` (`internal/httpapi/network.go`, `internal/httpapi/network_test.go`, `internal/httpapi/server.go`, `internal/orchestrator/orchestrator.go`), against round 2's approved report.

## Verdict: APPROVE

This round replaces `Orchestrator.Rebuild` (one call holding the lock across stop→persist→start) with `WithProject`/`Project`/`lockedProject`, moving the whole decision — including the `p.LanAccess == on` no-op check, now compared against the LIVE bridge via the new `ModeMismatch` rather than the stale record — inside the handler's closure under one lock, and makes `RunOpts.Lan` a `func() (bool, error)` read at the moment `ensureRunningLocked` decides the bridge name instead of a value snapshotted by the caller. `go build`, `go vet`, `gofmt -l`, and `go test ./internal/httpapi/... ./internal/orchestrator/...` (plain and `-race`) are all clean. The closure writes exactly one response on every one of its seven exit paths (traced line by line: inspect-error, mismatch-error, no-op, persist-only success, persist-only failure, stop failure, persist-after-stop failure, ensure failure, full success) — no fallthrough, no double write. Every former `opts.Lan`/`p.LanAccess` call site (`server.go:244,871,1070`, `network.go:196`) now goes through `lanOf(sp, id)`; no leftover `Rebuild`/`RebuildError`/`Orchestrator.ModeMismatch` references anywhere in the tree. `ensureRunningLocked`'s `lan, err := opts.lan()` followed by `insp, err := o.docker.ContainerInspect(...)` is ordinary same-scope `err` reuse (both in the function body, not a nested block), not a shadowing bug — it compiles as intended and the `err` checked at each later `if err == nil` is the one just assigned. `go.mod` pins `go 1.25.0`, well past the 1.21 `max` builtin requirement used in the fake's `maxLocked` bookkeeping.

## Findings

No correctness or security findings. Two low-severity observations, neither blocking:

- **[low / confidence: high]** `internal/httpapi/network_test.go:279-293` (`fakeMode.locked`/`maxLocked`) — `maxLocked` is incremented on every `WithProject` call but never asserted anywhere in the test file (`grep maxLocked` finds only its declaration and the two writes). It is dead instrumentation: computed, never read. Either delete it or add the assertion it appears to exist for (e.g., that no test ever observes `maxLocked > 1`, which would catch a reentrant-lock regression). Not a functional bug — `locked` itself is fine as a sanity mechanism if a future test wants it — but as written it is unused code per the dead-code criterion.
- **[low / confidence: medium]** `internal/orchestrator/orchestrator.go:200-207` (`Project` interface godoc) — the comment says each method is "the locked body of the method of the same name on Orchestrator." True for `Status`, `Stop`, `EnsureRunning`, but `ModeMismatch` has no `Orchestrator.ModeMismatch` counterpart — it wraps `currentBridge` plus its own missing-network-is-a-mismatch policy, distinct from the existing `Orchestrator.bridgeModeMismatch` (different name, different signature, different missing-network answer). A reader relying on the doc's "same name" claim to find `ModeMismatch`'s logic on `Orchestrator` would not find it. Documentation-only; no behavior is affected.

## Criteria

1. **Dead code** — pass, with the one exception above (`fakeMode.maxLocked`, test-only, unused). `Rebuild`/`RebuildError`/`Orchestrator.ModeMismatch` are fully removed (`grep` confirms zero hits); `rollbackMode` stays removed from round 1.
2. **Flow tracing** — pass. Handler closure traced end to end (see verdict); `ensureRunningLocked` reads `opts.lan()` once under the lock and threads the resolved `lan` value through every later use (mismatch check, log line, `recreateMissingNetwork`, `create()`) — no remaining `opts.Lan` reference in that function. `WithProject` → `lockedProject.{Status,ModeMismatch,Stop,EnsureRunning}` → `o.{Status,stopMarked,ensureRunningLocked}`/`o.currentBridge`, none of which re-acquire the project lock (confirmed `Status` and `currentBridge` take no lock; `stopMarked`/`ensureRunningLocked` are documented as caller-holds-the-lock bodies) — no reentrant-lock deadlock. `lanOf` reads `sp.projects.Get(id)` fresh on every call, so a start queued behind the handler's lock sees the just-persisted value, not a value captured when the HTTP request began.
3. **Standards conformance** — pass. Errors wrapped with `%w` throughout; no discarded errors on any new path; `gofmt`/`go vet` clean.
4. **Directory layout** — pass. No new files; everything added stays beside the code it extends (`Project`/`lockedProject`/`currentBridge` beside `EnsureRunning`/`bridgeModeMismatch` in `orchestrator.go`; `lanOf` beside `modeRuntime` in `server.go`).
5. **Simplicity** — pass. `WithProject`/`Project` is a smaller surface than the deleted `Rebuild`/`RebuildError` machinery — the decision logic that used to live behind `Rebuild`'s single call now lives once, in the handler, which is where the persist-failure branching (three distinct failure messages) actually differs by caller intent. `RunOpts.Lan` becoming a closure is the minimum change needed to close the round-2 finding (a start racing a toggle must read the record after the toggle, not before); `lanOf` is a two-line helper, not a new abstraction layer.
6. **Decoupling** — pass. `modeRuntime` shrinks to exactly `WithProject`; `Project` is sized to what the handler calls and nothing more; `fakeMode` implements both seams with no reach-in to orchestrator internals; `persist` stays a plain closure owned by `httpapi`, so `lockedProject` never touches the record store.

## Not examined

- Live integration/e2e suites (`testing/styrman/integration.sh`, `web/e2e`) were not run — unit-test-and-code-reading only, as in round 2.
- No re-review of `internal/project/store_lan_test.go` / `SetLanAccess` internals beyond what round 2 already covered.
- No fresh security review beyond confirming the round-2 and round-1 findings stay fixed and no new lock-ordering or fail-open issue was introduced by `WithProject`/`Project`; a dedicated security pass was not re-run this round.
