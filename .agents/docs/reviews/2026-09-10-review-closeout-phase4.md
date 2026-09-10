---
owner: reviewer
date: 2026-09-10
---
# Review — closeout phase 4 (item I: terminal re-authorization)   (2026-09-10)

Range reviewed: `.agents/scratch/phase4.diff` (Phase 4 of `.agents/docs/plans/2026-09-10-todo-closeout.md`, item I — terminal
revocation watch), read alongside `internal/terminal/bridge.go` in full for context. The diff also carries unrelated Phase-4
items (project `provisioning` state in `internal/httpapi/server.go`, an `AbortSignal.any` fix and `CHECKABLE_EXTENSIONS` in
`web/src/lib/api.ts`) which were skimmed but not exhaustively checked against the plan — see "Not examined".

## Verdict: APPROVE

Nothing broken found. All seven checklist items were traced end to end and confirmed correct; the two places most likely to
hide a bug (concurrent `conn.Close`, and whether an unauthorized "header" guest can even reach the bridge) both check out
against the actual library source and `authGuard` respectively. Build (`go build ./...`) and the relevant test packages
(`go test ./internal/terminal/... ./internal/httpapi/...`) pass.

## Checklist findings (no defects)

1. **Watch goroutine lifecycle** — `wg.Add(1)` at `bridge.go:373` precedes `go func(){...}()` at 374; `revocationWatch`
   (bridge.go:75-92) always returns on `hctx.Done()` via `select`, so `wg.Wait()` (bridge.go:478) cannot hang even if
   `stillAuthorized` never fires — confirmed by reading the whole function, not just the ticker branch.
   `userstore.Store.IsAuthorized` (internal/userstore/userstore.go:117) is a plain mutex-guarded map lookup, so the `check()`
   call inside the `select` case cannot block indefinitely and stall the loop's observation of `ctx.Done()`.
2. **`conn.Close` concurrency** — verified against `github.com/coder/websocket@v1.8.15/close.go:99-120`: `Close` CAS-guards
   via `c.casClosing()`; a second/concurrent call returns `net.ErrClosed` after waiting on the first call's teardown
   goroutines, never panics or double-frees. Both call sites (`bridge.go:383` inside the watch, and `bridge.go:476` at the
   end of `serve`) discard the return value, so the harmless second call is exactly that — harmless, not a bug.
3. **`session.email` empty under `AuthMode=none`** — `stillAuthorized` is `nil` in that mode (`server.go:221-223`), so
   `revocationWatch`'s nil-check short-circuit means `s.email` is never consulted; correct.
   **`AuthMode=header` with an unauthorized ("preview-only") caller** — traced `authGuard`
   (`internal/httpapi/auth.go:64-78`): an unauthorized header identity only reaches `next.ServeHTTP` for
   `isPreviewPath`/`isSharedPath` (`/proxy/…`, `/p/…/port/…`, `/shared/…`); `/ws/term/{project}/{term}` and `/ws/setup`
   (`server.go:370-371`) match neither, so such a caller gets 403 before the bridge is ever reached. `s.email` is
   therefore always non-empty whenever `stillAuthorized != nil`.
4. **First check after one full interval (60 s)** — matches the plan's "within a minute" language; not a defect.
5. **Frontend wiring** — `CLOSE_REVOKED = 4003` exported (`web/src/lib/api.ts`), no collision with 4000/4001/4002
   (`internal/config/config.go:58-71`); imported and handled in `TerminalPane.tsx` at the same guard level as the existing
   4001/4002 handlers (`closedForGood` check first, `setState("revoked")`, `closedForGood = true`, return before the
   transient-retry branch), with a distinct user-facing message. Consistent with the established pattern.
6. **`reauth_test.go` discrimination** — traced each of the three tests against a hypothetically-broken
   `revocationWatch`: a watch that fires on the first tick would fail
   `TestRevocationWatchFiresOnceWhenAuthorizationIsLost`'s `calls.Load() != 3` assertion (expects two `true` then one
   `false`); a watch that never returns after `check()` turns false would hit the test's own 2 s timeout via
   `t.Fatal`. The nil-check and always-authorized tests are similarly exact (elapsed-time and fire-count bounds). The
   suite genuinely discriminates, not just exercises the happy path.
7. **`NewBridge`'s new 7th parameter** — one production caller (`server.go:225`), updated in the same diff; `grep -rn
   "NewBridge("` finds no other call site, and `grep -rln "Bridge{"` finds no test constructing the struct literal
   directly (would have needed the new fields too). `go build ./...` and `go test ./internal/terminal/...` both pass.

## Criteria

1. Dead code — pass. No orphaned code introduced; `revocationWatch` is exercised by both production code and its own unit tests.
2. Flow tracing — pass. Traced `ServeHTTP`/`ServeSetup` → `session.email` → `serve` → watch goroutine → `revoked` atomic →
   close-decision path, and the constructor call → its one caller. Context/cancellation flow (`hctx`/`hcancel`) is
   consistent; no leaked goroutine or resource on any exit path.
3. Standards conformance — pass. Idiomatic Go (atomic bool, sync.WaitGroup, table-style unit tests); TS side matches the
   existing close-code handling pattern in `TerminalPane.tsx`.
4. Directory layout — pass. New test lands beside the code it covers (`internal/terminal/reauth_test.go`), not a parallel tree.
5. Simplicity — pass. `revocationWatch` is a small, dependency-free helper; no new library, no speculative abstraction. The
   security-relevant piece (authorization gating) reuses the existing `userstore.IsAuthorized`, not a hand-rolled check.
6. Decoupling — pass. `revocationWatch` takes plain `func() bool`/`func()` callbacks, is unit-testable without a
   WebSocket, and the `stillAuthorized` wiring is injected from `httpapi` rather than the `terminal` package reaching
   into `userstore` directly.

## Not examined

- The `provisioning sync.Map` / `deriveState` change in `internal/httpapi/server.go` (project list/get state derivation,
  presumably a separate Phase-4 item) and the `AbortSignal.any` / `CHECKABLE_EXTENSIONS` changes in `web/src/lib/api.ts`
  were read but not traced end to end or checked against their own plan items — the dispatch's checklist was entirely
  about the terminal revocation feature (item I), so review effort went there.
- Did not run `web/e2e/run.sh` or the integration script (`.agents/docs/testing/styrman/integration.sh`); relied on
  `go build ./...` and `go test ./internal/terminal/... ./internal/httpapi/...` (both green) plus static tracing against
  library source for the concurrency/auth-guard questions.
- Did not check `check.go`/`format.go`, which `git status` shows modified in the working tree but which are not part of
  `phase4.diff` — out of scope for this review.
