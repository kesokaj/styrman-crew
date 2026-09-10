---
owner: verifier
date: 2026-09-10
---
# Verification — network-record-first (setProjectNetwork rewrite)   (2026-09-10)

Range reviewed: working tree vs HEAD (uncommitted changes, not yet committed)
- internal/httpapi/network.go
- internal/httpapi/network_test.go
- internal/httpapi/server.go
- internal/orchestrator/orchestrator.go

## Status: PASS

Scope: gofmt, go vet, go test ./... -race (whole repo) ran and pass; caddy ingress module ran for completeness; diff binary-safety and test-count/name-disappearance checks ran; coverage traced by branch for `setProjectNetwork` and `Orchestrator.ModeMismatch`. Integration (`.agents/docs/testing/styrman/integration.sh`) and browser e2e (`web/e2e/run.sh`) NOT run — need Kata/minutes, out of scope for this dispatch.

## Commands

- `gofmt -l internal/ cmd/` → no output (PASS)
- `go vet ./...` → exit 0, no output (PASS)
- `go test ./... -count=1 -race` → all packages `ok`:
  `cmd/styrman`, `internal/config`, `internal/conversation`, `internal/credbroker`,
  `internal/credstore`, `internal/files`, `internal/gitrepo`, `internal/history`,
  `internal/httpapi`, `internal/identity`, `internal/links`, `internal/orchestrator`,
  `internal/project`, `internal/terminal`, `internal/userstore` (all `ok`);
  `cmd/wsprobe`, `web` — no test files. Exit 0.
- `go test ./internal/httpapi -count=1 -v 2>&1 | grep -c '^--- PASS'` → **100**
- `grep -c '^func Test' internal/httpapi/network_test.go` → **15**
- `git show HEAD:internal/httpapi/network_test.go | grep -c '^func Test'` → **10**
  (matches the dispatch's expected 15 vs 10)
- `git diff --numstat` →
  ```
  77	76	internal/httpapi/network.go
  217	64	internal/httpapi/network_test.go
  2	1	internal/httpapi/server.go
  10	0	internal/orchestrator/orchestrator.go
  ```
  No `-` in either column on any line → not binary, BUG-007 clear. PASS.
- `(cd deploy/caddy/ingress && go test ./... -count=1)` → `ok github.com/spgo/devbox/caddy/ingress` (2.0s). Unrelated to this change, run for completeness. PASS.

### Test-name diff (HEAD vs working tree)

HEAD (10): `TestNetworkValidationAndIsolation`, `TestNetworkEnableIsAdminOnlyDisableIsNot`,
`TestNetworkNoOpPutDoesNotRebuild`, `TestNetworkModeRoundTrips`,
`TestNetworkConcurrentPutsStayConsistent`, `TestDeleteProjectRemovesTheRecord`,
`TestNetworkFailedRevocationRollsBackTheRecord`, `TestNetworkFailedRestartKeepsTheRecord`,
`TestNetworkStatusErrorOnRevocationRollsBack`, `TestNetworkAbsentContainerJustWritesTheRecord`.

Working tree (15): the same 6 unchanged tests, plus
`TestNetworkFailedRevocationChangesNothing` (renamed from `...RollsBackTheRecord`),
`TestNetworkStatusErrorChangesNothing` (renamed from `...RollsBack`),
`TestNetworkFailedRestartKeepsTheRecord` (unchanged),
`TestNetworkAbsentContainerJustWritesTheRecord` (unchanged), plus five new:
`TestNetworkMismatchErrorChangesNothing`, `TestNetworkRePutRepairsADriftedRecord`,
`TestNetworkRecordBehindTheBridgeOnlyWritesTheRecord`,
`TestNetworkNoOpWhenRecordAndBridgeAgree`,
`TestNetworkPersistFailureAfterStopLeavesItDownInThePreviousMode`.

All 10 HEAD tests are accounted for as either unchanged or clearly renamed (the two
`*RollsBack*`/`*RollsBackTheRecord*` names — `TestNetworkFailedRevocationRollsBackTheRecord`
and `TestNetworkStatusErrorOnRevocationRollsBack` — became `...ChangesNothing`, consistent
with the rollback mechanism's deletion described in network.go's new doc comment). No
OTHER HEAD test name disappeared. PASS — no BUG-005-style silent deletion.

Note: the dispatch text says "the four tests named `*RollsBack*`/`*RollsBackTheRecord*`
at HEAD" — only **two** HEAD test names actually match that pattern
(`grep -iE 'RollsBack'` on `git show HEAD:...` → 2 matches). Flagging the discrepancy for
the record; it does not change the PASS verdict since all 10 HEAD tests are traced above.

## Coverage of changed code — branch-by-branch trace of `setProjectNetwork`

| Branch in `internal/httpapi/network.go` | Covering test(s) |
|---|---|
| `Status` inspect error | `TestNetworkStatusErrorChangesNothing` |
| `ModeMismatch` error (wrong-bridge check) | `TestNetworkMismatchErrorChangesNothing` |
| `Stop` error (wrongBridge, revoking) | `TestNetworkFailedRevocationChangesNothing` |
| Persist (`SetLanAccess`) error after successful stop | `TestNetworkPersistFailureAfterStopLeavesItDownInThePreviousMode` |
| No-op: record and live bridge already agree | `TestNetworkNoOpWhenRecordAndBridgeAgree`, `TestNetworkNoOpPutDoesNotRebuild` |
| Drifted record (record behind bridge, wrongBridge false but record != on) repaired by re-PUT | `TestNetworkRePutRepairsADriftedRecord` |
| Record behind bridge, container stopped/absent — record-only write, no rebuild | `TestNetworkRecordBehindTheBridgeOnlyWritesTheRecord` |
| Absent container | `TestNetworkAbsentContainerJustWritesTheRecord` |
| `EnsureRunning` (rebuild) error after persist | `TestNetworkFailedRestartKeepsTheRecord` |
| Admin-gate on enable / non-admin disable allowed | `TestNetworkEnableIsAdminOnlyDisableIsNot` |
| Full round trip incl. rebuild success | `TestNetworkModeRoundTrips` |
| Concurrent PUTs (netMu serialization) | `TestNetworkConcurrentPutsStayConsistent` |
| Delete removes record | `TestDeleteProjectRemovesTheRecord` |
| Basic validation / isolation | `TestNetworkValidationAndIsolation` |

Every branch enumerated in the dispatch (status error, mismatch error, stop error,
persist error after stop, no-op with agreement, drifted record, record behind bridge,
absent, ensure error) has a named covering test above. PASS — full branch coverage of the
changed `setProjectNetwork` function.

## Coverage gap (reported, not fixed)

`Orchestrator.ModeMismatch` (new exported method, `internal/orchestrator/orchestrator.go`)
has **no unit test in `internal/orchestrator`**. It is exercised only indirectly, through
`internal/httpapi`'s `fakeMode.ModeMismatch` stand-in (`network_test.go:294`), which tests
the httpapi handler's use of the interface, not the orchestrator's real
`bridgeModeMismatch`/Docker-inspection logic behind it. This is because the orchestrator
package has no fake Docker client to unit-test against (known limitation — BUG-003
residual, tracked in TODO.md). Reporting this as the pre-existing gap it is; not fixing it
here per instructions.

## Environment

- OS: Linux 6.12.107+deb13-amd64 (container/dev host)
- Go: `go version` not re-captured this run; build/vet/test all succeeded against the
  repo's pinned toolchain via `go.mod`.
- Docker/Colima: not applicable — no container build requested for this verification;
  integration/e2e suites (which need Kata) were explicitly out of scope and not run.

---

# Round 2 — orchestrator.Rebuild/ensureRunningLocked/currentBridge (2026-09-10)

Range reviewed: working tree vs HEAD (uncommitted), re-run after further changes to
`internal/httpapi/network.go`, `internal/httpapi/network_test.go`, `internal/httpapi/server.go`,
`internal/orchestrator/orchestrator.go` (new `Rebuild`, `ensureRunningLocked`, `currentBridge`).

## Status: PASS

Scope: gofmt, go vet, go test ./... -race (whole repo), test-count check, numstat binary-safety
check, caddy ingress module test all ran and pass. Coverage trace confirms the dispatch's named
coverage gap in `internal/orchestrator` is real. Integration suite on real Kata is being run
separately by the main thread — not run here.

## Commands

- `gofmt -l internal/ cmd/` → no output. PASS.
- `go vet ./...` → exit 0, no output. PASS.
- `go test ./... -count=1 -race` → all packages `ok`: `cmd/styrman`, `internal/config`,
  `internal/conversation`, `internal/credbroker`, `internal/credstore`, `internal/files`,
  `internal/gitrepo`, `internal/history`, `internal/httpapi`, `internal/identity`,
  `internal/links`, `internal/orchestrator`, `internal/project`, `internal/terminal`,
  `internal/userstore`; `cmd/wsprobe`, `web` — no test files. PASS.
- `go test ./internal/httpapi -count=1 -v 2>&1 | grep -c '^--- PASS'` → **101**;
  `grep -c '^--- FAIL'` → **0**.
- `grep -c '^func Test' internal/httpapi/network_test.go` → **16**;
  `git show HEAD:internal/httpapi/network_test.go | grep -c '^func Test'` → **10**.
  Matches dispatch's expected 16 vs 10. The new test present:
  `TestNetworkPersistFailureOnDriftedRecordSaysWhatIsReported` (confirmed in the 16-name
  listing, alongside the same renames from round 1: `TestNetworkFailedRevocationChangesNothing`,
  `TestNetworkStatusErrorChangesNothing`, `TestNetworkMismatchErrorChangesNothing`, plus round-1's
  other new tests). PASS.
- `git diff --numstat` →
  ```
  96	80	internal/httpapi/network.go
  251	65	internal/httpapi/network_test.go
  2	2	internal/httpapi/server.go
  103	14	internal/orchestrator/orchestrator.go
  ```
  No `-` in either column on any line → not binary, BUG-007 clear. PASS.
- `(cd deploy/caddy/ingress && go test ./... -count=1)` → `ok github.com/spgo/devbox/caddy/ingress`
  (2.0s). Unrelated module, run for completeness. PASS.

## Coverage trace — new orchestrator code

`internal/orchestrator/orchestrator.go` gained (per this round's diff): `Rebuild` (line 209,
new exported method — holds the per-project lock across stop → persist(callback) → start, per
its doc comment referencing ADR-067 and a 2026-09-10 security-review finding 2, to close a
TOCTOU where a parked start could resurrect the container on the old bridge), `ensureRunningLocked`
(line 233, `EnsureRunning`'s body factored out so `Rebuild` can call it while already holding the
lock), `currentBridge` (line 893, reads the live Docker network a container is attached to), and
`ModeMismatch` (line 870, exported wrapper the httpapi package uses — this one already flagged
in round 1).

`grep -rl "Rebuild\|ensureRunningLocked\|currentBridge\|ModeMismatch" internal/orchestrator/*_test.go`
→ **no output** — none of the four names appears in any `_test.go` file in the package
(`busy_test.go`, `orchestrator_test.go`, `orchestrator_workspace_test.go`, `realbelow_test.go`,
`tailbuffer_test.go`). Confirmed by reading the four functions directly: `Rebuild` in particular
carries the security-relevant lock-ordering guarantee (stop-persist-start under one lock hold)
that this round's diff exists to add, and it has zero direct unit-test coverage in
`internal/orchestrator`.

## Coverage gaps (reported, not fixed)

- `Orchestrator.Rebuild` — NOT COVERED in `internal/orchestrator`. Exercised only indirectly
  through `internal/httpapi`'s fakes (e.g. `TestNetworkModeRoundTrips`,
  `TestNetworkFailedRestartKeepsTheRecord`), which test the httpapi handler's use of the
  interface, not `Rebuild`'s own lock-hold/step-ordering logic against a real or fake Docker
  client.
- `Orchestrator.ensureRunningLocked` — NOT COVERED directly; only reachable via `EnsureRunning`
  (its public wrapper) or via `Rebuild`, both untested at this layer.
- `Orchestrator.currentBridge` — NOT COVERED in `internal/orchestrator`.
- `Orchestrator.ModeMismatch` — NOT COVERED in `internal/orchestrator` (carried over from round 1).

All four gaps share the same root cause noted in round 1: `internal/orchestrator` has no fake
Docker client to unit-test against (BUG-003 residual, tracked in TODO.md). This round adds
`Rebuild`'s stop→persist→start lock-hold sequencing to the untested surface — the newest and,
per its own doc comment, most security-relevant of the four.

## Not examined

- Integration suite (`.agents/docs/testing/styrman/integration.sh`, real Kata) — explicitly
  deferred to the main thread per dispatch.
- Browser e2e (`web/e2e/run.sh`) — not requested this round.
- No source or test files were edited by this verification pass.

## Environment

- OS: Linux 6.12.107+deb13-amd64 (container/dev host), same as round 1.
- Docker/Colima: not applicable — no container build requested; integration suite needing Kata
  explicitly deferred elsewhere.

---

# Round 3 — WithProject/lockedProject/stopMarked/RunOpts.lan/currentBridge (2026-09-10)

Range reviewed: working tree vs HEAD (uncommitted), re-run after further changes to
`internal/httpapi/network.go`, `internal/httpapi/network_test.go`, `internal/httpapi/server.go`,
`internal/orchestrator/orchestrator.go`.

## Status: PASS

Scope: gofmt, go vet, go test ./... -race (whole repo), httpapi test-count check, numstat
binary-safety check, caddy ingress module test, web check/lint/test (incl. `lan.test.ts`
in isolation) all ran and pass. Coverage trace confirms the new orchestrator-package
surface named in the dispatch remains untested at that layer. Integration suite on real
Kata run separately by the main thread — not run here.

## Commands

- `gofmt -l internal/ cmd/` → no output. PASS.
- `go vet ./...` → exit 0, no output. PASS.
- `go test ./... -count=1 -race` → all `ok`: `cmd/styrman`, `internal/config`,
  `internal/conversation`, `internal/credbroker`, `internal/credstore`, `internal/files`,
  `internal/gitrepo`, `internal/history`, `internal/httpapi`, `internal/identity`,
  `internal/links`, `internal/orchestrator`, `internal/project`, `internal/terminal`,
  `internal/userstore`; `cmd/wsprobe`, `web` — no test files. PASS.
- `go test ./internal/httpapi -count=1 -v 2>&1 | grep -c '^--- PASS'` → **102**;
  `grep -c '^--- FAIL'` → **0**.
- `grep -c '^func Test' internal/httpapi/network_test.go` → **17** (working tree) vs
  `git show HEAD:internal/httpapi/network_test.go | grep -c '^func Test'` → **10**.
  Matches dispatch's expected 17 vs 10 exactly. Both newly-named tests confirmed present:
  `TestNetworkPersistFailureOnDriftedRecordSaysWhatIsReported` (line 578) and
  `TestLanOfReadsTheRecordAtCallTime` (line 609). PASS.
- `git diff --numstat`:
  ```
  121	101	internal/httpapi/network.go
  295	67	internal/httpapi/network_test.go
  20	9	internal/httpapi/server.go
  129	24	internal/orchestrator/orchestrator.go
  ```
  No `-` in either column on any line → not binary, BUG-007 clear. PASS.
- `(cd deploy/caddy/ingress && go test ./... -count=1)` → `ok github.com/spgo/devbox/caddy/ingress`
  (2.0s). PASS.
- `(cd web && npm run check && npm run lint && npm test)` → `tsc -b` clean, `oxlint` clean,
  vitest: **9 test files, 127 tests, all passed**. Confirmed API reply shape unaffected: ran
  `web/src/lib/lan.test.ts` in isolation too (`npx vitest run src/lib/lan.test.ts`) →
  **1 file, 8 tests, all passed**. PASS.

## Coverage trace — new orchestrator code named by the dispatch

- `Orchestrator.WithProject` (orchestrator.go:219) — takes the per-project lock and hands
  the caller a `lockedProject`. `grep -rl "WithProject" internal/orchestrator/*_test.go` →
  no output. NOT COVERED in `internal/orchestrator`; only exercised via
  `internal/httpapi/network_test.go`'s `fakeMode.WithProject` stand-in (line 290), which
  tests the httpapi handler's use of the interface, not the real lock-then-callback body.
- `lockedProject.Status` / `.ModeMismatch` / `.Stop` / `.EnsureRunning` (orchestrator.go
  231–262) — thin wrappers invoked only through `WithProject`'s callback; same gap, no
  direct orchestrator-package test.
- `Orchestrator.stopMarked` (orchestrator.go:1110) — `grep -rl "stopMarked"
  internal/orchestrator/*_test.go` → no output. NOT COVERED.
- `RunOpts.lan` (orchestrator.go:182) — reads the project record at call time (per the
  dispatch's framing, this is the fix for the previously-flagged early-read TOCTOU).
  `grep -rl "RunOpts{" internal/orchestrator/*_test.go` → no output. NOT COVERED directly;
  only reachable through `EnsureRunning`/`ensureRunningLocked`, both also untested at this
  layer.
- `currentBridge` (orchestrator.go, ~line 893 in round 2's numbering) — `grep -rl
  "currentBridge" internal/orchestrator/*_test.go` → no output. NOT COVERED (carried over
  from round 2, code unchanged in that respect).
- The locked read at the top of `ensureRunningLocked` (orchestrator.go:264–268, `opts.lan()`
  called under the lock before any Docker call) — same function as round 2's
  `ensureRunningLocked` gap; still NOT COVERED, and this round's diff is specifically what
  moved the mode-read inside the locked region, so the new ordering guarantee itself has no
  direct test at this layer either.

All of the above share the same root cause as rounds 1 and 2: `internal/orchestrator` has no
fake Docker client to unit-test against (BUG-003 residual, tracked in TODO.md). The httpapi
fake (`fakeMode` in `network_test.go`) covers the handler side — the httpapi package's use of
the `Project`/`Mode` interfaces — but not the orchestrator's own lock-hold and record-read
ordering behind them. Reporting these as gaps, not fixing them, per instructions.

## Not examined

- Integration suite (`.agents/docs/testing/styrman/integration.sh`, real Kata) — run by the
  main thread per dispatch, not duplicated here.
- Browser e2e (`web/e2e/run.sh`) — not requested this round.
- No source or test files were edited by this verification pass.

## Environment

- OS: Linux 6.12.107+deb13-amd64 (container/dev host), same as rounds 1–2.
- Docker/Colima: not applicable — no container build requested; integration suite needing
  Kata explicitly deferred elsewhere.
