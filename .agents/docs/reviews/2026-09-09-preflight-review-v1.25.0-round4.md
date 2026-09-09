---
owner: reviewer
date: 2026-09-09
---
# Review — preflight v1.25.0 round 4 (two round-3 lows)   (2026-09-09)

Range reviewed: eccd1a0..worktree (`git diff HEAD -- internal/orchestrator/orchestrator.go`; eccd1a0 == HEAD, so the two hunks are the uncommitted working tree). Scope deliberately narrow: ONLY the `unmarkIfStillRunning` hunks (Stop/stopIfIdle) and the `bridgeModeMismatch` signature + doc comment. Nothing else in the working tree was reviewed. Round 3's third low (no unit tests for these paths) is recorded in BUG-030/031 and is not re-raised.

## Verdict: APPROVE WITH NITS

Both round-3 lows are closed, and both fixes are correct in the scenario each was raised for. The one thing worth putting on the record is that the fresh background context in `unmarkIfStillRunning` — which is the right call for BUG-030's own wedged-stop case — opens a narrow window the function's doc comment claims it forecloses: a stop whose transport failed *after* the daemon accepted the request can still inspect as `Running`, so the marker is cleared for a container that goes down seconds later. Low severity, narrow timing, and strictly better than the pre-fix behaviour (a permanently 4002-refused running project); not release-blocking.

## Findings

- **[low / confidence: medium]** `internal/orchestrator/orchestrator.go:1030-1046` — `unmarkIfStillRunning`'s doc claims that when a stop half-succeeds ("the daemon took the request, the transport returned an error") "the container IS down", so the marker is kept. That is not guaranteed at the instant of the inspect. `stopLocked` calls `ContainerStop(ctx, timeout=30)`; if the caller's deadline (2 min from `stopProject`, `lanRebuildTimeout` from the mode toggle) fires *while that request is in flight*, the client returns an error but the daemon continues the graceful stop, and the fresh-context inspect microseconds later legitimately reports `State.Running`. The marker is then deleted for a project that powers off seconds afterwards — and the next terminal auto-reconnect (`internal/terminal/bridge.go:123`) boots the microVM back, which is exactly the resurrection the marker exists to prevent. Reaching it needs the exec + `waitStopped` phase to have consumed almost the whole budget first, so it is narrow, but it is the scenario the comment names as handled. Concrete fix (post-freeze): require two consecutive `Running` samples ~2 s apart, or re-inspect once after `waitStopped(ictx, name, 5*time.Second)` returns, before `o.stopped.Delete`. Cheap and it does not change the fresh-context decision.
- **[low / confidence: high]** `internal/orchestrator/orchestrator.go:1043-1045` — the inspect error and the "not running" outcome are both swallowed silently. When the residual above fires the other way (inspect fails, marker stays), the operator sees a running project refusing every reconnect with close 4002 and nothing in the log connecting it to a failed stop — BUG-030's exact symptom, now rare instead of certain, but just as undiagnosable. One `o.logger.Warn("stop failed and the container could not be confirmed running — leaving the stopped marker", "project", project, "error", err)` on the non-clearing branch closes it. Not blocking.

## Answers to the two questions asked

**1. `unmarkIfStillRunning` on a fresh `context.Background()` + 5 s.**

- *Correct here:* yes. Round 3's point holds exactly as stated — the caller's ctx expiring is the most likely cause of the failed stop (`stopProject` builds a 2-minute `context.WithTimeout(context.Background(), ...)`, so client disconnect cannot cancel it; only the deadline can), and an inspect on a dead ctx fails at once, keeping the marker stuck in BUG-030's own scenario. A derived ctx would have made the fix a no-op in the case it was written for.
- *5 s reasonable:* yes. `ContainerInspect` reads daemon state, not guest state, so a wedged Kata VM does not slow it; it is milliseconds against a healthy daemon. If the daemon itself is unresponsive the call times out and the marker is kept — the conservative direction, matching the function's stated bias.
- *Cancellation semantics broken anywhere:* no. Both call sites are on the error path of an operation that has already finished, under the per-project lock (`Stop` and `stopIfIdle` both hold it), so nothing downstream inherits the ctx and no goroutine outlives the call. The one place the ignored ctx could cost anything is server shutdown: the reaper's ctx is cancelled, `stopLocked` fails fast, and each `unmarkIfStillRunning` could burn up to 5 s. It costs nothing in practice — `RunIdleReaper` is launched as a bare `go orch.RunIdleReaper(ctx)` in `cmd/styrman/main.go:95` and shutdown does not wait on it, and the `stopped` map is in-memory only, so a marker cleared at exit has no successor to mislead. The `_ context.Context` parameter is kept unused deliberately and documented; `go vet ./internal/orchestrator/` is clean.

**2. `bridgeModeMismatch` doc comment vs body vs call site.** Matches exactly, on all three claims:

- "never guesses true" — the only `true` returns are the empty-bridge-name case (`orchestrator.go:851`, logged Warn) and the real comparison (`:854`). Neither is on an error path.
- "A missing network is NOT a mismatch" — `errdefs.IsNotFound(err)` → `(false, nil)` (`:832`). Verified this classifies correctly for `NetworkInspect` on docker v28.5.2: `errdefs.IsNotFound` is `var IsNotFound = cerrdefs.IsNotFound`, and the client maps a 404 through `httpErrorFromStatusCode` to `cerrdefs.ErrNotFound`, so a genuinely missing network takes the NotFound branch and `recreateMissingNetwork` still owns it on the start path below.
- "Any OTHER inspect error is RETURNED, and the caller REFUSES to start" — `(false, err)` at `:842`, and `EnsureRunning` (`:233-236`) returns `"network mode: cannot inspect the project network, refusing to start on an unverified bridge: %w"` before reaching `projectHomeIsReal`, the create switch, or any `ContainerStart`. The container stays down, as the comment says.
- Two scope facts the comment relies on and that hold: the block is guarded by `err == nil && (insp.State == nil || !insp.State.Running)`, so stopped-container-only (stated in the block comment above it at `:225-231`), and because that guard requires a *successful* `ContainerInspect` on the same daemon moments earlier, a non-NotFound `NetworkInspect` failure there is genuinely anomalous rather than a routine daemon hiccup — so the new refusal introduces no realistic new denial-of-start.

## Criteria

1. **Dead code** — pass. Nothing orphaned; `bridgeModeMismatch` has exactly one caller and both return values are consumed; the unused `_ context.Context` on `unmarkIfStillRunning` is intentional and documented at the call sites' reason.
2. **Flow tracing** — pass with the one low above. Both `bridgeModeMismatch` call sites (there is one) updated for the new two-value signature; the error is wrapped with `%w`; both `stopLocked` failure branches (`Stop`, `stopIfIdle`) go through the same helper and still return the original error unchanged, so `internal/httpapi/network.go:163`'s `rollbackMode` and `server.go:1078`'s 500 are unaffected. Marker mutation stays under the per-project lock at every site.
3. **Standards conformance** — pass. `go vet ./internal/orchestrator/` clean; errors wrapped with context, none discarded into `_`; comments say why, not what.
4. **Directory layout** — pass. Change confined to the file that owns the behaviour.
5. **Simplicity (DIY-except-security)** — pass. No new dependency, no new abstraction; one small unexported helper shared by the two branches that need it. No auth/crypto surface touched.
6. **Decoupling** — pass. `modeRuntime` in `internal/httpapi` is unaffected by the signature change (`bridgeModeMismatch` is unexported); no cross-package reach-in.

## Not examined

Everything outside the two hunks: `internal/httpapi/network.go` and `network_test.go`, `web/dist/client/_shell.html`, the docs/records changes in the same working tree, the `.gitattributes` addition, and the untracked e2e/integration edits from the session start. Round 3's unit-test gap was not re-assessed. No tests, builds, or suites were run beyond `go vet ./internal/orchestrator/`.
