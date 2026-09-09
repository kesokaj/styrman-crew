---
owner: reviewer
date: 2026-09-09
---
# Review — preflight v1.25.0 (ADR-067 fix round)   (2026-09-09)

Range reviewed: 5481ea2..eccd1a0

Source files judged: `internal/httpapi/network.go`, `internal/httpapi/network_test.go`,
`internal/httpapi/server.go`, `web/src/lib/api.ts`, `web/e2e/run.mjs`.
Docs under `.agents/docs` and the `web/dist` bundle rotation were out of scope by
dispatch, except that I verified the rotated bundle actually carries the new
timeout (see finding 4's note).

## Verdict: REQUEST CHANGES

BUG-028's fail-open is closed on the branch it was found on and left open on the
branch beside it. `setProjectNetwork` treats a `Status` **error** as "stopped or
absent", keeps the newly-written record and answers **200** — so a LAN→sandbox
revocation that hits a transient Docker inspect failure while the container is
running reports success, writes "sandbox" into the record, and leaves the
container on the `lan-` bridge forever, since `bridgeModeMismatch` heals stopped
containers only. That is the same mechanism, the same direction and the same
permanence BUG-028 describes, reached through `err != nil` instead of through
`Stop`. Everything else in the range is sound: the rollback itself is correct in
both directions, the lock discipline is right, the `modeRuntime` seam is
complete on the network path, and the two client timeouts genuinely pair with
the server constant they name.

## Findings

- **[medium / confidence: high]** `internal/httpapi/network.go:113-119` — the
  `Status` error branch is the fail-open BUG-028 just closed, one branch over.
  `if err != nil || st.State != "running"` collapses three different facts into
  one: "it is absent", "it is stopped", and "I could not find out". The first
  two justify keeping the record and returning 200 (the next start applies the
  mode). The third does not: `Orchestrator.Status`
  (`internal/orchestrator/orchestrator.go:147-160`) already maps not-found to
  `Status{State:"absent"}, nil`, so a non-nil error means a real inspect
  failure — daemon hiccup, or the shared 3-minute `lanRebuildTimeout` expiring —
  and the container may well be **running on the old bridge**. In that case a
  revocation persists `lanAccess:false`, replies `200 {"lanAccess":false}`, and
  nothing ever reconciles: the mismatch heal at
  `internal/orchestrator/orchestrator.go:232` is gated on
  `insp.State == nil || !insp.State.Running`, exactly the reasoning the new
  comment at `network.go:121-131` gives for the `Stop` branch. The switch, the
  badge and the API then all under-report a project that still has LAN reach —
  an R12-visible misreport. Fix: split the condition. Keep the current 200 for
  `err == nil && st.State != "running"`; on `err != nil`, take the same path the
  `Stop` failure now takes — roll the record back to `!on` under `netMu`, log,
  and 500 with "could not determine the project's state, so the mode was NOT
  changed". An unknown state is not a stopped state.

- **[low / confidence: high]** `internal/httpapi/network.go:120-147` — the
  rollback restores the record but not the orchestrator's deliberately-stopped
  latch, so a request the caller is told did nothing leaves the project looking
  stopped. `Orchestrator.Stop` sets `o.stopped.Store(key, true)` **before**
  attempting the stop (`internal/orchestrator/orchestrator.go:991`) and does not
  clear it on failure. After a failed apply the container is still running and
  the record is back to its old value, but `orch.Stopped()` now answers true, so
  `internal/terminal/bridge.go:123` closes every automatic terminal reconnect
  with 4002 "project stopped" and `bridge.go:407` labels every pane close the
  same way — on a project that is up and unchanged. Only a user-initiated Start
  (or a `?boot=1` connect) clears it. The old code's message ("saved, but the
  project could not be stopped") at least pointed the operator at Start; the new
  fail-closed framing says the mode was not changed, which reads as a clean
  no-op it is not. Fix: expose a way to clear the latch (e.g. an orchestrator
  `ClearStopped(user, project)`, or have `Stop` clear it when `stopLocked`
  returns an error and the container is still running) and call it beside the
  record rollback; or say so in the 500 body.

- **[low / confidence: high]** `internal/httpapi/network_test.go:3-13` — the file
  is not gofmt-clean: `"strings"` is inserted before `"path/filepath"` in the
  stdlib block, and `gofmt -l ./internal ./cmd` names this file (the only one in
  the repo). `go vet ./...` is clean and `go test ./...` passes, so this only
  fails a formatting gate, but it fails it on the release commit. Fix:
  `gofmt -w internal/httpapi/network_test.go`.

- **[low / confidence: medium]** `internal/httpapi/network.go:132-143` — a
  concurrent delete makes the rollback log a misleading ERROR. If the project is
  removed while `Stop` is in flight (the `deletingState` guard at line 84 is
  checked once, at request entry), `SetLanAccess` returns `project.ErrNotFound`
  and the handler logs at ERROR "rollback failed, record disagrees with the
  running container" for a project that no longer exists. No state corruption —
  `SetLanAccess` does not resurrect a missing record
  (`internal/project/store.go:226-229`) — but ERROR is the level this codebase
  reserves for "needs a human", and this one does not. Fix: treat
  `errors.Is(rollback, project.ErrNotFound)` as an INFO/DEBUG "record already
  gone".

## Not a finding, checked and clear

- **Rollback direction.** `SetLanAccess(p.ID, !on)` is correct in *both*
  directions, not just the revocation the comment dwells on. The no-op check at
  line 95 guarantees `p.LanAccess != on` at the moment of the forward write, and
  both writes are serialized by `netMu`, so `!on` is exactly the value that was
  in force. Enable-that-fails leaves "sandbox" (container still on `sdb-`);
  revoke-that-fails leaves "LAN" (container still on `lan-`). Both report what is
  in force, which is the invariant BUG-028 asks for.
- **Lock discipline.** `netMu` is taken for the two record writes and the
  rollback write only, never across `Status`/`Stop`/`EnsureRunning`; that matches
  its only other holder, `deleteProject` (`server.go:1006-1008`), and keeps the
  server-wide mutex off a three-minute path. `-race` on the package is clean.
- **Rollback vs. a concurrent PUT.** I walked the interleavings. A second PUT
  cannot complete a *rebuild* ahead of the rollback (both go through the
  orchestrator's per-project lock, which the failing `Stop` holds), and every
  ordering I could construct ends with the record agreeing with the bridge. The
  one residual is a second PUT that hits the *no-op* branch off a stale
  `getProjectFor` snapshot and gets a 200 for a value the rollback then reverts:
  the caller sees one wrong reply, the next GET corrects it, and the record and
  the container still agree. The stale-snapshot no-op check itself predates this
  range (unchanged lines) and is not a regression here.
- **`modeRuntime` completeness.** All three network-path calls are routed
  (`network.go:113,120,148`); nothing else on that path is on the concrete
  `s.orch` except `EnsureSelfAttached` at line 157, which is deliberately outside
  the seam and is a safe no-op under test (`cfg.SelfContainer == ""` returns
  early, `orchestrator.go:526`). The remaining `s.orch.EnsureRunning`/`Stop` call
  sites (`server.go:233,859,1058,1078`) are the start/stop and ensure paths, not
  the mode path, and correctly stay concrete — the interface is sized to its one
  consumer, which is what the codebase-design vocabulary asks for. The empty
  `if err == nil {}` block deleted at `server.go:233` was real dead code; good
  removal.
- **The two new tests.** They assert *behaviour* — the persisted record read
  back through the store (`lanAccessOf`), the response code, and the direction
  of the rollback — not the shape of the implementation, and
  `TestNetworkFailedRestartKeepsTheRecord` correctly pins the opposite
  requirement (a failed restart must NOT roll back, because a stopped container
  has no access and the record is what the next start applies). The two call
  counters are the one implementation-coupled assertion, and they earn it:
  `ensureCalls != 0` after a failed `Stop` is the concrete "we did not boot the
  thing back up" check. The fake's divergences from the real orchestrator (no
  per-project lock, no `stopped` latch, `Status` never errors) do not weaken what
  is asserted — except that the missing latch is why finding 2 is invisible to
  this test.
- **`api.ts` timeout.** `lanRebuildTimeout = 3 * time.Minute` does exist at
  `internal/httpapi/network.go:36` with that value, so 210 000 ms genuinely
  exceeds the server bound by 30 s. The comment's supporting claim is accurate
  too: start and stop are bounded at `2*time.Minute` server-side
  (`server.go:1054,1076`), which is why copying their 150 s was short. The signal
  reaches `fetch` — `request()` spreads `init` after its default, so the caller's
  `signal` wins (`api.ts:358-364`). The rotated `web/dist` bundle carries
  `AbortSignal.timeout(21e4)`, so the embedded SPA is built from this source and
  not stale.
- **`run.mjs`.** Both PUT timeouts raised to 210 000 ms, matching the client and
  clearing the server's 180 s; the checks stay at the end of the suite, so
  BUG-021 is not reintroduced.

## Criteria

1. **Dead code** — pass. The empty `if err == nil {}` at `server.go:233` was
   removed; nothing new is orphaned. `modeRuntime` has one production
   implementation and one test double, so it is not a speculative interface.
2. **Flow tracing** — fail (finding 1). Every changed call site is consistent —
   `s.mode` is assigned in `New` to the same `*Orchestrator` as `s.orch`, all
   three signatures match the concrete methods exactly, errors are wrapped with
   `%w` on every branch, and the 3-minute context is created with `defer cancel()`
   and threaded through all three calls. The break is one hop further out: the
   `err != nil` branch of `Status` hands a possibly-running container to a code
   path written for a stopped one.
3. **Standards conformance** — fail, minor (finding 3). `go vet ./...` clean,
   `go test ./...` green, `-race` green on `internal/httpapi`; `gofmt -l` names
   `network_test.go`. Errors wrapped, no discarded `err`, no floating promise in
   the TS. Per `.agents/rules/writing-files.md`, `git diff --numstat` shows
   `91  0` on `network_test.go` — appended, nothing deleted — and all five source
   files are text, no binary lines.
4. **Directory layout** — pass. Test beside its subject, the interface declared
   in the package that consumes it, no new files.
5. **Simplicity (DIY-except-security)** — pass. No new dependency; the seam is
   three methods sized to the one handler; no hand-rolled auth or crypto (the
   admin gate reuses `requireAdmin`).
6. **Decoupling** — pass. `modeRuntime` mirrors the existing `guestRuntime`
   convention, is consumer-declared and consumer-sized, and lets the failure
   paths be tested without Docker — which is the whole point of the change.

## Not examined

- The docs in the range (`DECISIONS.md`, `BUG-028`, the change/handoff/testing
  entries, `guide.md`, `remote.md`, `ROADMAP.md`, `AGENTS.md`) — excluded by
  dispatch; I read `BUG-028` only to check the fix against its prevention rule.
- The `web/dist` bundle beyond confirming the new timeout is present in it.
- The already-reviewed 44e81e0..5481ea2 ADR-067 diff, and the orchestrator's
  bridge-naming/firewall behaviour it introduced, except where finding 1 depends
  on `bridgeModeMismatch`'s stopped-only gate.
- The live paths: no Kata run, no `integration.sh`, no `web/e2e/run.sh` — the
  raised e2e timeouts are judged by reading only.
