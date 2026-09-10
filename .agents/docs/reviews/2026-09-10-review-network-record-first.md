---
owner: reviewer
date: 2026-09-10
---
# Review — network-record-first   (2026-09-10)

Range reviewed: uncommitted working-tree diff, `.agents/scratch/network-record-first.diff` (`internal/httpapi/network.go`, `internal/httpapi/network_test.go`, `internal/httpapi/server.go`, `internal/orchestrator/orchestrator.go`)

## Verdict: REQUEST CHANGES

The redesign correctly closes the three fail-opens named in its own comments (BUG-028/029/031: record-first-then-rollback, Status-error-as-stopped, mismatch-error-as-no). But the new self-heal branch it adds — comparing against the *live* bridge instead of the record so a re-PUT can repair drift — has one persist-failure sub-branch that reproduces the exact fail-open class the whole change exists to eliminate: the record can end up claiming **less** access than is actually in force, silently, with no distinguishing log. This is proven with a reproduction test below, not just argued. It is also untested — the diff adds a persist-failure test for the `wrongBridge==true` (Stop-then-persist-fails) case but not for the `wrongBridge==false` (bridge-already-matches-request, persist-fails) case, which is the one that fails open.

## Findings

- **[high / confidence: high]** `internal/httpapi/network.go:137-172` — When `wrongBridge` is `false` because the *live* bridge already agrees with the requested mode while the *record* still disagrees (the exact "record behind the bridge" drift this PR is designed to self-heal — reachable pre-existing state from any of BUG-028/029/031 before this fix, or manual intervention), `Stop` is skipped (correctly — nothing needs stopping) and the code goes straight to `SetLanAccess`. If that persist call fails, the function falls into the generic `else` branch at network.go:170 (`"cannot persist setting"`) instead of the `wrongBridge` branch's Warn-logged, explicitly-worded one at network.go:158-168. The record is left unchanged — i.e. still at its old, wrong value — while the container is untouched and still running on the bridge that disagrees with the record. Concretely: record says sandbox (`false`), live bridge is `lan-` (already granting LAN), admin PUTs `lanAccess:true` to fix the drift, persist fails (e.g. the temp-file rename in `project.Store.SetLanAccess` hits a permission or disk error) → 500 returned, but the record still reads `false` while the container is actually on `lan-`. Every surface that reads the record (switch, badge, `GET /network`) now under-reports live LAN access, indefinitely, with a plain "cannot persist setting" log line giving no signal that the system is in the dangerous state. I reproduced this directly against the harness (chmod'd the user's state dir to make `SetLanAccess` fail, `wrongBridge=false`, `bridgeLan=true`, record starts `false`): confirmed `record=false`, `bridgeLan=true` after the 500. This is a real, reachable single-fault path that violates the docstring's own claim ("no single failure can leave it claiming LESS access than the container has", network.go:68-70) and R12's isolation intent — not a new isolation bypass (Docker's own per-bridge DROP still holds), but a silent misrepresentation of what mode a project is actually in, exactly the class of bug BUG-028/029/031 were opened over. Fix: give the `wrongBridge==false` persist-failure path the same distinguishing Warn log the `wrongBridge==true` path gets, and/or fold the "bridge already correct, only the record needs fixing" persist failure into an explicit "still drifted" error message so the operator knows to retry rather than reading a generic transient-looking error.

- **[medium / confidence: high]** `internal/httpapi/network_test.go` — Missing test for the branch above. `TestNetworkPersistFailureAfterStopLeavesItDownInThePreviousMode` only covers `wrongBridge==true` (Stop already ran, container down). There is no test starting from `lanRunning`-style drift (`bridgeLan` already matching the request, record behind it) with `SetLanAccess` forced to fail — i.e. the sibling of `TestNetworkRecordBehindTheBridgeOnlyWritesTheRecord` but with a persist failure instead of success. Per this project's own convention (a verifier compares test count/coverage of changed behavior, not just exit status), the new self-heal-against-live-bridge behavior added by this diff has an untested failure mode, and it's the dangerous one.

## Criteria

1. **Dead code** — pass. `rollbackMode` and its imports (`internal/project`) are fully removed; `grep -rn rollbackMode` across the repo is empty; `go vet ./internal/httpapi/... ./internal/orchestrator/...` is clean.
2. **Flow tracing** — mostly pass, one gap (see finding above). Traced every branch of `setProjectNetwork` against what the record says vs. what is in force after that branch:
   - Status error → nothing written, nothing changed, consistent.
   - Running + ModeMismatch error → nothing written, nothing changed, consistent.
   - `p.LanAccess == on && !wrongBridge` → true no-op, 200, consistent.
   - `wrongBridge` true, Stop fails → nothing written, container still on old bridge (unchanged), record still old — consistent (matches actual).
   - `wrongBridge` true, Stop succeeds, persist fails → container down (nothing in force), record stays at old value — over-claims (safe direction), tested and logged distinctly.
   - `wrongBridge` true, Stop succeeds, persist succeeds, EnsureRunning fails → container down, record now says new value — over-claims for grant, safely under-provisions for revoke; matches "next start applies it" comment.
   - `wrongBridge` false (stopped/absent, or running-with-bridge-already-matching-request), persist succeeds → correct in both sub-cases.
   - `wrongBridge` false, persist fails → **under-claims when the drift being repaired was the dangerous direction (record says less than the live bridge grants)** — see high finding. Reproduced directly; not a hypothetical.
   `orchestrator.ModeMismatch` is a thin, correct wrapper over the existing `bridgeModeMismatch`; `modeRuntime`'s new `ModeMismatch` method is implemented by both the concrete orchestrator and `fakeMode`, so the interface and its only two implementers stay in sync — no stub left unwired.
3. **Standards conformance** — pass. Errors wrapped with `%w` throughout; no discarded errors; `netMu` held only for the write per its own documented rule; context/timeout used consistently for the three-minute synchronous rebuild.
4. **Directory layout** — pass. Handler/test stay in `internal/httpapi/`, orchestrator method in `internal/orchestrator/orchestrator.go` beside its private helper `bridgeModeMismatch`; nothing misplaced.
5. **Simplicity (DIY-except-security)** — pass. No new dependency; `ModeMismatch` is a one-line delegation to existing logic, not a new abstraction; the `modeRuntime` interface grows by exactly the one method the handler now needs, sized to the consumer as required by criterion 6.
6. **Decoupling** — pass. `modeRuntime` interface stays a small, consumer-defined seam; `fakeMode` in the test file models the same four calls with no reach-in to orchestrator internals; `Server`, `project.Store`, and `orchestrator.Orchestrator` boundaries are unchanged.

## Not examined

- `internal/project/store_lan_test.go` / `SetLanAccess`'s own rollback-on-save-failure behavior (read only enough to confirm it restores in-memory state on failure — did not review it end to end).
- The live integration/e2e suites (`testing/styrman/integration.sh`, `web/e2e`) were not run; this review is unit-test-and-code-reading only, as scoped.
- `EnsureRunning`'s own `bridgeModeMismatch` call path (orchestrator.go:233-244) for the STOPPED-container mismatch heal was read but not independently re-verified beyond what the existing code/comments already assert.
- No security review of unrelated `internal/orchestrator` code outside the two functions named in the dispatch.

## Response (main, 2026-09-10)

- **Finding 1 (high) — accepted as a reporting gap, severity rebutted.** The
  state it describes (record sandbox, container on `lan-`) exists BEFORE the
  request: it is the drift this PUT is the repair for, and a failed repair
  leaves it exactly as it was. No branch of the new handler CREATES a record
  that claims less than is in force; that was the bar, and it holds. What
  was missing is that the generic "cannot persist setting" said nothing about
  which mode the project keeps reporting. Fixed: the branch now logs at WARN
  with `requested`/`recordSays` and the reply says the project is already on
  the requested bridge but keeps reporting the previous mode. Stopping the
  container there was considered and rejected: a JSON write failure should
  not take a project down that is on the bridge the caller asked for.
- **Finding 2 (medium) — fixed.** `TestNetworkPersistFailureOnDriftedRecordSaysWhatIsReported`
  covers the branch: no stop, no rebuild, record unchanged, message names
  the stale report.
