# Phase 5 closeout: Docker seam and unit tests for orchestrator     (date: 2026-09-10)

## Summary

Phase 5 of plan `2026-09-10-todo-closeout.md` closed: `internal/orchestrator` now has a `dockerAPI` interface wrapping the 19 Docker client methods orchestrator code calls, with compile-time verification that `*client.Client` satisfies it; a fake in-memory Docker engine for testing (`fakedocker_test.go`); and 10 new orchestrator unit tests in `seam_test.go` covering container lifecycle, bridge mismatches, symlink guards, marker timing, and status checks. Coverage: `ensureRunningLocked` 59%, `create` 79%, `bridgeModeMismatch` 67%, `currentBridge`/`WithProject`/`Status`/`stopMarked` 100%, `unmarkIfStillRunning`/`stopLocked` 88%. `go test -race ./internal/orchestrator` passed; darwin cross-compile verified.

## Changes

**O. Docker API seam and fake**

`internal/orchestrator/docker.go` — New interface `dockerAPI` names the 19 Docker client methods orchestrator code calls (`ContainerInspect/Create/Start/Stop/Remove/List/Logs`, four `ContainerExec*`, six `Network*`, `ImageInspect`, `Ping`), with signatures copied from docker v28's `client.Client`. `Orchestrator.docker` is retyped to the interface; `*client.Client` still satisfies it at runtime (`New` unchanged). Compile-time check in `docker_test.go` asserts `var _ dockerAPI = (*client.Client)(nil)`.

`internal/orchestrator/fakedocker_test.go` — New fake implements `dockerAPI` with in-memory container and network state. Containers have a `Running` flag and network bridge option; networks have a name. "Absent" answers return `errdefs.NotFound(...)` so the eleven `errdefs.IsNotFound` branches are testable. Per-method scripted errors (one per method, panics if invoked) allow tests to inject failures. Call log records method name, arguments, and return values for assertion.

**Orchestrator seam tests**

`internal/orchestrator/seam_test.go` — New, 10 tests using the fake:
1. Stopped container on wrong-prefix bridge is removed and recreated on the right bridge
2. Network inspect error refuses to start (BUG-031)
3. Symlinked project home is refused before any Docker call (mutation-checked: removing `projectHomeIsReal` fails it; BUG-002/003 guard)
4. Running container is left alone and marker cleared
5. `WithProject` holds the lock across stop → persist → start (failed start keeps marker set, record written)
6. `stopMarked` clears marker only when container provably still running (two samples 2s apart, BUG-030); keeps marker when down with Warn
7. Older watch cannot clear newer stop's marker (`CompareAndDelete` on stop time)
8. `Status` returns absent/stopped/running/error correctly
9. `currentBridge` returns found/missing/error
10. `unmarkIfStillRunning` samples behavior with delay between inspects

## Files changed

- `internal/orchestrator/docker.go` — new, `dockerAPI` interface, 19 methods from docker v28 client
- `internal/orchestrator/docker_test.go` — compile-time assertion that `*client.Client` satisfies `dockerAPI`
- `internal/orchestrator/fakedocker_test.go` — new, in-memory Docker engine fake with call recording
- `internal/orchestrator/seam_test.go` — new, 10 orchestrator unit tests
- `internal/orchestrator/orchestrator.go` — `docker` field retyped to `dockerAPI` interface

## Agents dispatched

verifier (unit tests), reviewer (interface design, test coverage), security (symlink guard verification), changelog

## Verification

- `go test -race ./internal/orchestrator` green, 23 tests (10 new)
- darwin amd64 cross-compile verified
- Mutation testing: removing `projectHomeIsReal` call fails the symlink-guard test
- Verifier log: `.agents/docs/testing/2026-09-10-verify-closeout-phase5.md` (23/23 pass)
- Code review logged in `.agents/docs/reviews/2026-09-10-review-closeout-phase5.md` and `.agents/docs/reviews/2026-09-10-security-closeout-phase5.md` (findings recorded in commit message)

## Requirements

n/a

## Related

- Plan: `.agents/docs/plans/2026-09-10-todo-closeout.md`
- Closes BUG-003 (residual: Docker client unit testing), BUG-030 (marker timing), BUG-031 (network inspect error)
- ADR-002/003 (symlink guards in guest-writable paths remain under orchestrator)
