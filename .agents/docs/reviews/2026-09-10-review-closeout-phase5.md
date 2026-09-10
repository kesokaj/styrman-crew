---
owner: reviewer
date: 2026-09-10
---
# Review — closeout phase 5 (dockerAPI seam, fake, orchestrator unit tests)   (2026-09-10)

Range reviewed: `.agents/scratch/phase5.diff` (already applied to the working tree, uncommitted) —
`internal/orchestrator/orchestrator.go` (one-line struct field change), new files
`internal/orchestrator/docker.go`, `internal/orchestrator/fakedocker_test.go`,
`internal/orchestrator/seam_test.go`.

## Verdict: APPROVE WITH NITS

The seam and fake are sound: the interface's 19 methods exactly match the 19 unique
`o.docker.*` call sites in the package (verified by the same grep the doc comment
cites), the compile-time assertion is against the real production type (`New()`
assigns `*client.Client` from `client.NewClientWithOpts` straight into the
`dockerAPI`-typed field), `container.ExecAttachOptions` is a type alias for
`ExecStartOptions` so the call site's literal compiles and behaves identically, and
`go build ./...`, `go vet ./internal/orchestrator/...` and
`go test ./internal/orchestrator/... -race -count=1` all pass (21.6s, no race
reported). The one real defect is a test that can't catch the exact regression class
it exists to catch (below); the rest are lower-severity gaps worth naming.

## Findings

- **[medium / confidence: high]** `internal/orchestrator/seam_test.go:105` —
  `if fd.callsTo("ContainerStart") != 0 && fd.callsTo("ContainerCreate") != 0` uses
  `&&`, not `||`. The intent (per the comment on line 106, "Docker must not be handed
  the home") is "neither call happened." As written the assertion only fires when
  *both* calls happened; a regression that calls `ContainerCreate` (handing Docker the
  symlinked home as a bind source — precisely the BUG-003-class failure
  `guest-writable-paths.md` exists to prevent) while `ContainerStart` stays at zero, or
  vice versa, would pass this test silently. Today's code is correct (`projectHomeIsReal`
  at `orchestrator.go:341` returns before the `switch` that reaches `create`/`ContainerStart`),
  so this is a test-only gap, not a live bug — but it is exactly the kind of
  discriminating-test defect the dispatch asked to check for, on the one test that
  guards a security-sensitive path. Fix: `||`.

- **[low / confidence: medium]** `internal/orchestrator/fakedocker_test.go` (`fakeContainer`
  fields `running`/`binds`/`netName`) — `ContainerStart`/`ContainerStop` mutate
  `c.running` on the pointer returned by `byName` *after* releasing `f.mu` (byName
  locks only for the map lookup). No current test races on this because
  `unmarkIfStillRunning`'s background goroutine (the one concurrent reader/writer
  path in the whole suite) is only exercised by tests that construct a fresh,
  non-overlapping `seamOrch` per case, so nothing else touches the same
  `fakeContainer` while that goroutine is in flight — `-race` is clean today.
  But the fake's own mutex doesn't actually protect per-container state, so a
  future test that runs a second operation against the same project while an
  `unmarkIfStillRunning` goroutine is still sampling (a scenario production code
  allows once `stopMarked`/`WithProject` releases the lock) would be a real,
  silent data race that this fake cannot currently be trusted to surface safely.
  Worth a `sync.Mutex` (or moving the fields under `f.mu`) before the fake is reused
  for a test that does overlap operations.

- **[informational, not a finding]** `fail map[string]error` is scripted per-method
  for *every* call, with no built-in "fail once then succeed" support (confirmed:
  none of the ten tests need it, and none of the diff's methods reset the map mid-test).
  A test wanting that shape today would have to mutate `fd.fail[method]` directly
  between calls from the test goroutine — safe only as long as no concurrent
  goroutine (e.g. `unmarkIfStillRunning`) is calling `record()` on the same method at
  the same time, since that direct assignment bypasses `f.mu`. None of the current
  tests do this, so it's not a live bug, but it's a real limitation if Phase 6+ adds
  a retry-path test (e.g. `recreateMissingNetwork`'s start-fails-then-recreate-then-
  retry flow in `ensureRunningLocked`, which today has no test at all).

- **[informational, not a finding]** `TestStopMarkedClearsTheMarkerOnlyWhenStillRunning`
  polls up to 4s against a goroutine whose own two-sample wait is hardcoded to ~2s
  — a 2x margin against a fully deterministic, in-memory fake (no real I/O), so not
  meaningfully flake-prone in practice, but it does make this one test ~6s of the
  ~21.6s suite (plus `TestUnmarkDoesNotClearANewerStopsMarker`'s fixed 2.5s sleep and
  `TestWithProjectStopPersistStartOrderAndFailedStart`'s ~12s from the same
  two-sample wait triggered by the scripted `ContainerStart` failure path... actually
  that one doesn't trigger unmark — its 12s is `waitStopped`'s poll inside `stopLocked`
  during `pr.Stop()`, unrelated to this diff's fake and not a flake risk, just slow).

- `TestUnmarkDoesNotClearANewerStopsMarker` (`seam_test.go:547`) does test the
  `CompareAndDelete`-on-`time.Time` behavior, not a superficial check: the stored
  marker (`newer`) and the value passed to `unmarkIfStillRunning` (`old`) are
  different `time.Time` values, so `CompareAndDelete(k, old)` correctly no-ops; a
  naive unconditional `Delete` would clear the marker and fail this test. Confirmed
  by reading `unmarkIfStillRunning` (`orchestrator.go:1173`) — no gap here.

- `TestEnsureRunningLeavesARunningContainerAlone` (`seam_test.go:453`) is
  discriminating: a version that removed/rebuilt a *running* container on a bridge
  mismatch (rather than deferring to the toggle handler, per `orchestrator.go:319`'s
  "STOPPED containers only") would trip `ContainerRemove`/`ContainerStop` != 0 and
  fail. Confirmed correct.

- `ContainerExecAttach`'s `net.Pipe` with the peer closed immediately: verified this
  produces a clean `io.EOF` for `stdcopy.StdCopy` (not an error) by running the suite
  — every test that reaches `waitReady`/`Exec` (all the `EnsureRunning`-path tests)
  passes in ~0s with `execExit` defaulting to 0, confirming `Exec` reaches
  `ContainerExecInspect` and returns exit code 0 as intended.

- `ensureRunningLocked`'s symlink guard (`orchestrator.go:341`, `projectHomeIsReal`)
  runs before any Docker call that could use the project home as a bind source or
  mount target (the actual guest-writable-paths concern), but **not** before every
  Docker call: on the code path the symlink test exercises, `ContainerInspect`,
  `ImageInspect`, and `NetworkInspect` (via `bridgeModeMismatch`) all run first,
  since none of them reference the home path. This matches the code's own comment
  ("hands the project home to Docker as a bind SOURCE") and is not a violation of
  the rule, but note it precisely since the dispatch's framing ("before any Docker
  call") is stricter than what the code (correctly) does.

## Not examined

- `internal/orchestrator/orchestrator_test.go`, `orchestrator_workspace_test.go`,
  `realbelow_test.go`, `busy_test.go`, `tailbuffer_test.go` — pre-existing test
  files, not part of this diff, not re-reviewed.
- Legacy migration branches in `ensureRunningLocked` (pre-ADR-027 workspace-bind
  recreate, image-upgrade recreate, `recreateMissingNetwork` defense-in-depth retry)
  have no coverage in this diff's ten tests — not a defect in what was written (the
  diff's stated scope is the seam + first tests, not full coverage), but worth
  naming as a gap for a future phase.
- Did not review `internal/httpapi`'s `guestRuntime`/`modeRuntime` seams referenced
  in `docker.go`'s comment for consistency of shape — out of this diff's scope.
- Did not run `bash deploy/vulncheck.sh` or the full `go test ./...` / integration
  suite — scoped this review to `internal/orchestrator` per the dispatch.

## Criteria

1. Dead code — pass: no unused exports, no commented-out code; the fake and seam
   file are both fully exercised by the new tests.
2. Flow tracing — pass, with the one test-assertion gap noted above (`&&` vs `||`);
   errors from every new method propagate through existing `fmt.Errorf(...: %w...)`
   wrapping unchanged by this diff.
3. Standards conformance — pass; matches the `golang` skill's stdlib-first
   interface-at-the-seam pattern, mirrors the project's own `guestRuntime`/
   `modeRuntime` precedent per the doc comment.
4. Directory layout — pass: `docker.go` and the two `_test.go` files sit beside
   `orchestrator.go` in `internal/orchestrator/`, matching the "tests beside the code"
   rule.
5. Simplicity — pass: no new dependency, a minimal interface sized to the call
   sites, a plain in-memory fake instead of a mocking framework.
6. Decoupling — pass: `dockerAPI` is declared where it's consumed (orchestrator
   package), sized to its own call sites, not re-exported.

## Response (main, 2026-09-10)

- **Finding 1 (medium) — fixed.** `||`, as meant; and mutation-checked:
  with the `projectHomeIsReal` call removed from `ensureRunningLocked` the
  test fails, with it present it passes.
- **Finding 2 (low) — fixed.** The fake mutates and reads `fakeContainer`
  fields under `f.mu` (`ContainerStart`/`Stop`/`Inspect`), and the two
  tests that flip `running` by hand take the lock too.
- **Finding 3 (informational) — noted** in the fake's doc comment as the
  next need (a once-only failure script) when a retry path gets a test.
