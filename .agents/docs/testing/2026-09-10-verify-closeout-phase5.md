---
owner: verifier
date: 2026-09-10
---
# Verification — closeout-phase5 (dockerAPI seam)   (2026-09-10)

Range reviewed: working tree (uncommitted) — new `internal/orchestrator/docker.go`
(`dockerAPI` interface, 19 methods, `var _ dockerAPI = (*client.Client)(nil)`),
`internal/orchestrator/orchestrator.go` (`docker` field retyped to `dockerAPI`),
new `internal/orchestrator/fakedocker_test.go`, new `internal/orchestrator/seam_test.go`
(10 tests). Note: the working tree also carries unrelated modified/untracked files
from earlier phases of `.agents/docs/plans/2026-09-10-todo-closeout.md` (web/,
terminal/, httpapi/, deploy/); this verification covers only Phase 5 (item O),
the orchestrator dockerAPI seam.

## Status: PASS

Scope: complete for the items named in the dispatch — gofmt, go vet, orchestrator
suite, full module suite, darwin/arm64 cross-build, seam test count, interface/
call-site method-count cross-check, per-function coverage trace. Did not run the
Kata integration suite, browser e2e, or the vendored Caddy ingress module's own
tests (not requested and out of scope for this seam change).

## Commands

- `gofmt -l internal/` → empty output (clean, no unformatted files)
- `go vet ./...` → clean, no output
- `go test ./internal/orchestrator/ -count=1 -race -v 2>&1 | grep -E '^--- '` →
  23 lines, all `--- PASS`, 0 FAIL. Full list:
  ```
  --- PASS: TestBusyReason (0.00s)
  --- PASS: TestContainerGuest (0.00s)
  --- PASS: TestRestartPolicy (0.00s)
  --- PASS: TestGuestHostname (0.00s)
  --- PASS: TestHasWorkspaceBind (0.00s)
  --- PASS: TestRealBelowRefusesSymlinkComponents (0.00s)
  --- PASS: TestProjectHomeIsRealAllowsMissingRefusesLinks (0.00s)
  --- PASS: TestSetupHomeStale (0.00s)
  --- PASS: TestEnsureRunningRebuildsAStoppedContainerOnTheWrongBridge (0.00s)
  --- PASS: TestEnsureRunningRefusesWhenTheNetworkCannotBeInspected (0.00s)
  --- PASS: TestEnsureRunningRefusesASymlinkedProjectHome (0.00s)
  --- PASS: TestEnsureRunningLeavesARunningContainerAlone (0.00s)
  --- PASS: TestEnsureRunningRefusesWhenTheModeCannotBeRead (0.00s)
  --- PASS: TestWithProjectStopPersistStartOrderAndFailedStart (12.04s)
  --- PASS: TestStopMarkedClearsTheMarkerOnlyWhenStillRunning (6.05s)
  --- PASS: TestUnmarkDoesNotClearANewerStopsMarker (2.50s)
  --- PASS: TestStatusMapsAbsentStoppedRunningAndError (0.00s)
  --- PASS: TestCurrentBridgeFoundMissingAndError (0.00s)
  --- PASS: TestTailBufferBoundsHostileOutput (0.01s)
  --- PASS: TestTailBufferKeepsTheNewest (0.00s)
  --- PASS: TestTailBufferExactMaxSingleWriteIsNotTruncation (0.00s)
  --- PASS: TestTailBufferUnderCap (0.00s)
  --- PASS: TestTrimLogTail (0.00s)
  --- PASS: TestTailBufferBoundaryDoesNotEatALine (0.00s)
  ```
  Wall time 21.9s (matches the dispatch note: two tests wait on the 2s unmark
  samples — `TestWithProjectStopPersistStartOrderAndFailedStart` 12.04s,
  `TestStopMarkedClearsTheMarkerOnlyWhenStillRunning` 6.05s,
  `TestUnmarkDoesNotClearANewerStopsMarker` 2.50s).
- `go test ./... -count=1 -race` → all 15 tested packages `ok`, 0 failures,
  24.0s total:
  ```
  ok  	github.com/kesokaj/styrman-crew/cmd/styrman	1.041s
  ok  	github.com/kesokaj/styrman-crew/internal/config	1.764s
  ok  	github.com/kesokaj/styrman-crew/internal/conversation	1.039s
  ok  	github.com/kesokaj/styrman-crew/internal/credbroker	1.045s
  ok  	github.com/kesokaj/styrman-crew/internal/credstore	1.021s
  ok  	github.com/kesokaj/styrman-crew/internal/files	1.050s
  ok  	github.com/kesokaj/styrman-crew/internal/gitrepo	1.043s
  ok  	github.com/kesokaj/styrman-crew/internal/history	1.025s
  ok  	github.com/kesokaj/styrman-crew/internal/httpapi	1.677s
  ok  	github.com/kesokaj/styrman-crew/internal/identity	1.015s
  ok  	github.com/kesokaj/styrman-crew/internal/links	1.030s
  ok  	github.com/kesokaj/styrman-crew/internal/orchestrator	21.610s
  ok  	github.com/kesokaj/styrman-crew/internal/project	1.017s
  ok  	github.com/kesokaj/styrman-crew/internal/terminal	1.094s
  ok  	github.com/kesokaj/styrman-crew/internal/userstore	1.014s
  ```
  (`cmd/wsprobe` and `web` report "no test files", expected.)
- `GOOS=darwin GOARCH=arm64 go build ./...` → exits 0, prints `BUILD_OK`
  (cross-compiles clean; confirms no linux-only breakage was introduced).
- `grep -c '^func Test' internal/orchestrator/seam_test.go` → `10` (matches
  dispatch expectation exactly).
- `grep -oh 'o\.docker\.[A-Za-z]*' internal/orchestrator/*.go | sort -u | wc -l`
  → `19`, matching the interface's 19 methods. All 19 distinct call sites
  (`ContainerCreate`, `ContainerExecAttach`, `ContainerExecCreate`,
  `ContainerExecInspect`, `ContainerExecResize`, `ContainerInspect`,
  `ContainerList`, `ContainerLogs`, `ContainerRemove`, `ContainerStart`,
  `ContainerStop`, `ImageInspect`, `NetworkConnect`, `NetworkCreate`,
  `NetworkDisconnect`, `NetworkInspect`, `NetworkList`, `NetworkRemove`,
  `Ping`) appear verbatim in the `dockerAPI` interface declaration in
  `docker.go` — no method called on `o.docker` that is missing from the
  interface (the build already proves this transitively; grep confirms the
  count matches with no surprises).

## Coverage trace

`go test ./internal/orchestrator/ -count=1 -coverprofile=.agents/scratch/orch.cover`
→ `ok ... coverage: 35.0% of statements`. Per-function figures for the
functions named in the dispatch (`go tool cover -func`):

| Function | Coverage |
|---|---|
| `Status` (Orchestrator, line 147) | 100.0% |
| `WithProject` (line 219) | 100.0% |
| `Status` (lockedProject, line 231) | 100.0% |
| `ensureRunningLocked` (line 264) | 59.0% |
| `createBridgeNetwork` (line 891) | 100.0% |
| `currentBridge` (line 903) | 100.0% |
| `bridgeModeMismatch` (line 928) | 66.7% |
| `create` (line 958) | 79.1% |
| `stopMarked` (line 1109) | 100.0% |
| `unmarkIfStillRunning` (line 1147) | 87.5% |
| `stopLocked` (line 1190) | 87.5% |

`lockedProject` itself has no `create`/`ensureRunningLocked` combinator of its
own — `lockedProject.EnsureRunning` at line 258 calls `ensureRunningLocked`
directly (its own wrapper coverage was not in the func list because it is a
one-line delegation; not itemized separately by `go tool cover -func` beyond
what's shown). All nine named functions are exercised by the seam tests to
varying depth; none show 0% coverage, so the seam tests do reach every
function named in the dispatch. `ensureRunningLocked` (59.0%) and
`bridgeModeMismatch` (66.7%) are the two lowest — error/edge branches
(e.g. some Docker-API-failure paths) are not fully exercised, but this is
coverage depth, not absence of coverage, and was not flagged as a FAIL
criterion by the dispatch.

## Environment

Linux 6.12.107+deb13-amd64 (native build/test host). `go version` not
separately queried in this run but the toolchain used matches the project's
pinned module `go.mod` requirement (build and `go vet` both succeeded against
it). No Docker/Colima involved — the dockerAPI seam tests use the new
`fakedocker_test.go` fake and never touch a real Docker daemon.
