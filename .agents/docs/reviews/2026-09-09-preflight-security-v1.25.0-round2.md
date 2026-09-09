---
owner: security
date: 2026-09-09
---
# Security review — preflight v1.25.0 round 2: the Status-error fail-open and the stopped marker   (2026-09-09)

Range reviewed: eccd1a0..worktree
(`git diff HEAD -- internal/httpapi/network.go internal/orchestrator/orchestrator.go internal/httpapi/network_test.go`)

**The working tree moved twice while this review was running.** I first read
`network.go` at blob `017e86e` and `orchestrator.go` at `cecedf5`; the file below is written
against the CURRENT contents, `network.go@01dac9a` and `orchestrator.go@9e8d3b4`. Three
things I had written up as findings were fixed live during the review (the `ErrNotFound`
rollback race, `bridgeModeMismatch`'s fail-open inspect error, and the unconditional marker
delete). A release gate that reviews a moving target verifies nothing durable — whatever is
tagged must be re-diffed against the reviewed blobs, or this review re-run on the frozen
tree. `deploy/verify-commit.sh` exists for exactly that reason.

Integrity checks on the diff: `--numstat` 45/13, 52/1, 60/11 — all text, no `-` column
(BUG-007 clean). `network_test.go` goes 8 -> 10 `func Test`, none removed (BUG-005 clean).
`go build ./...`, `go vet ./internal/...`, and `go test -count=1 ./internal/httpapi
./internal/orchestrator ./internal/project` all pass on the current tree.

Blast radius followed: `internal/httpapi/network.go` (whole PUT handler),
`internal/httpapi/server.go` (`modeRuntime` seam, `getProjectFor`/`who`, `ensureFor`,
`startProject`, `stopProject`, `deleteProject`'s netMu window, `writeErr`),
`internal/orchestrator/orchestrator.go` (`Status`, `EnsureRunning` and its three recreate
branches, `bridgeModeMismatch`, `create`, `Stop`/`stopLocked`/`waitStopped`/`unmarkIfStillRunning`,
`stopIfIdle`, the idle reaper loop, `Stopped`, every `o.stopped` mutation),
`internal/terminal/bridge.go` (the only two `Stopped()` consumers),
`internal/project/store.go` `SetLanAccess`, `cmd/styrman/main.go:276` (the only other
`Stop` caller), `internal/httpapi/auth.go` `requireAdmin`.

## Verdict: FINDINGS (no branch of the PUT handler is blocking)

The Status-error fail-open is genuinely closed, and the `EnsureRunning` side is now closed
harder than asked: an inspect it cannot verify refuses to start the container at all.
Of the six branches, **none leaves the persisted record claiming LESS access than is in
force** except one doubly-faulted path — the rollback write itself failing (finding 1),
which needs the project store to fail *between* two writes and is logged at ERROR. I do not
call that a release blocker, but it is the last place the gate's own rule still bites, and
it is removable rather than defensible: on the tightening direction, persist the record only
after the rebuild succeeds (round 1's "cheapest correct variant"), which deletes the rollback
path from the dangerous direction instead of adding another failure handler to it.

Clearing `o.stopped` gives a non-admin **nothing**, in either the earlier unconditional form
or the current `unmarkIfStillRunning` form. The marker is not an authorization control: its
only two readers are in `internal/terminal/bridge.go`, an admin cannot stop another user's
project at all, and the owner can always boot their own project with `POST .../start` or
`?boot=1`. Full argument below.

## Per-branch adjudication — PUT /api/projects/{id}/network

All branches follow the initial `SetLanAccess(p.ID, on)` write at `network.go:127`.
"In force" = the bridge prefix the container is actually attached to (`sdb-` sandbox /
`lan-` LAN); a stopped or absent container has nothing in force.

| Branch | Persisted record | In force on the container | Verdict |
|---|---|---|---|
| **Status error** (`network.go:145`) | rolled back to the pre-request mode | unchanged — the container may still be running on the old bridge | **PASS.** Record == force. Nothing stopped, nothing rebuilt (`stopCalls==0 && ensureCalls==0`, asserted by the new test). The 500 says "NOT changed", which is true of the container. |
| **Absent** (`st.State=="absent"`, `network.go:157`) | requested mode | nothing (no container) | **PASS.** Record over-claims against nothing. The next `EnsureRunning` creates it, and `create()` rebuilds the network when the bridge name disagrees (`orchestrator.go:862`). |
| **Stopped** (same branch) | requested mode | nothing (container exists, not running) | **PASS in steady state.** The next start heals through `bridgeModeMismatch`, which now refuses to start on a bridge it cannot verify. One pre-existing caveat makes the branch's premise conditional — finding 2. |
| **Stop error** (`network.go:181`) | rolled back to the pre-request mode | old bridge, container still running | **PASS.** Record == force; this is round 1's finding, now shared through `rollbackMode`. The marker set by `Stop` is cleared again only if the container is provably still running, so the 4002 gate does not fire on a project that never stopped. |
| **EnsureRunning error** (`network.go:198`) | requested mode | nothing — `Stop` already succeeded | **PASS.** Record over-claims against nothing. The marker survives (only a *successful* `EnsureRunning` deletes it, `orchestrator.go:375`), so auto-reconnects are refused and an explicit start applies the mode. Two sub-cases both stay consistent: a failure after the container was recreated and started leaves it on the NEW bridge (= requested), and a failure from the new unverifiable-bridge check leaves it stopped. |
| **Rollback error** (`network.go:57-63`) | requested mode | old bridge, container still running | **FAIL-OPEN on revocation** — record says sandbox, LAN is in force, nothing reconciles it. Finding 1. Reachable only when the store write fails. |

## Findings

- **[medium / confidence: high]** `internal/httpapi/network.go:55-63` — **the rollback-failure
  branch is the one surviving fail-open.** `rollbackMode` writes
  `SetLanAccess(id, !requested)`; if that write fails with anything other than
  `project.ErrNotFound` it logs ERROR and returns, leaving the record at the *requested* mode
  while the old mode is still in force on a running container. On a revocation that is
  exactly round 1's finding 1: `GET .../network`, `projectView.LanAccess`, the switch and the
  badge all report "sandbox" for a project sitting on a `lan-` bridge, and nothing corrects
  it — `bridgeModeMismatch` is consulted for STOPPED containers only (`orchestrator.go:232`).
  Trigger: any project-store write failure between the two calls (ENOSPC, read-only remount,
  IO error). The first write having succeeded a second earlier does not make the second one
  safe: `Store.save()` rewrites the whole file. Remediation, shortest first — (a) for the
  tightening direction (`on == false`), persist the record only *after* the rebuild succeeds,
  which removes `rollbackMode` from the dangerous direction altogether; or (b) fail closed on
  the container rather than on the record — on a failed rollback, attempt a `Stop`, so that
  "nothing in force" makes the record true again.
  *(The `ErrNotFound`/deleteProject sub-case I raised earlier is already handled at INFO in
  the current tree, `network.go:49-54`. Correct.)*

- **[medium / confidence: medium]** `internal/httpapi/network.go:157-161` +
  `internal/httpapi/server.go:228-233` — **the "stopped, nothing to do" branch is not
  serialized against a concurrent start.** `RunOpts.Lan` is the *caller's snapshot* of
  `Project.LanAccess`, and the code says so: "It is the caller's copy of `Project.LanAccess`
  and it is the ONLY input to the bridge name" (`orchestrator.go:167-172`). `ensureFor` reads
  the record at `server.go:228` and only then calls `EnsureRunning`, which takes the
  per-project lock; the mode handler takes `netMu` for its record write and the orchestrator's
  project lock never. So a start whose record read preceded the PUT can bring the container up
  on the OLD bridge *after* `Status` observed "stopped" — the PUT answers 200 with the new
  mode persisted, and on a revocation the record then claims sandbox over a running `lan-`
  container, with no heal, because running containers are never re-checked. Not introduced by
  this diff, and the window is short (record read -> project lock -> home/image checks ->
  `ContainerStart`), which is why this is medium/medium rather than blocking; but it is the
  same fail-open family the gate exists for and it is the one branch whose PASS depends on
  timing. Remediation: resolve `LanAccess` from the store *inside* `EnsureRunning`, under the
  project lock — that removes the snapshot from four call sites and makes the lock the single
  serialization point — or have the mode handler hold the orchestrator's project lock across
  Status plus decision.

- **[low / confidence: high]** `internal/orchestrator/orchestrator.go:1027-1034` —
  **`unmarkIfStillRunning` inspects with the context that just failed.** The dominant way
  `stopLocked` returns an error is a context deadline (`ContainerStop` is given the handler's
  ctx, and `stopIfIdle` gets the reaper's). On that path `ContainerInspect(ctx, ...)` fails
  immediately with the same expired ctx, so `err != nil` and the marker is kept — the guard
  cannot answer in the case it was written for, and BUG-030's symptom (every reconnect
  refused with 4002 on a project running exactly as before) returns whenever the stop failed
  by timeout. Fail-closed, so not a security defect: keeping the marker only withholds
  automatic reconnects, and the owner can press Start. Remediation:
  `context.WithoutCancel(ctx)` plus a short timeout for the inspect, so the check can
  actually observe the container.

- **[low / confidence: high]** `internal/orchestrator/` — **neither security-relevant
  orchestrator change has a test.** `grep` finds no reference to `unmarkIfStillRunning` or
  `bridgeModeMismatch` in any `_test.go`, and the per-file `func Test` counts are identical
  at HEAD and in the worktree (1/3/1/3/6). The two new tests are both in `internal/httpapi`
  and both exercise the handler seam, not the orchestrator. The fail-closed
  "refusing to start on an unverified bridge" path and the marker guard are exactly the kind
  of branch that only a fake-Docker unit test reaches. Per the project's own Definition of
  Done, changed code without a covering test is a FAIL; flagging it here because these two
  branches are the security-load-bearing ones.

- **[low / confidence: high]** `internal/httpapi/network.go:154` — the 500 body wraps the raw
  Docker error ("the project could not be inspected ...: %w"), which can carry daemon socket
  paths and internal container names back to an authenticated tenant. Same shape as the
  pre-existing "could not be stopped" message. Log the cause; return a generic sentence.

- **[informational]** `orchestrator.go:815-822` — the doc comment above `bridgeModeMismatch`
  still opens with the old rationale ("It answers false on anything it cannot establish")
  before contradicting itself two lines later. The code is right; the comment's first clause
  is now false and will mislead the next reader of a security-critical default.

## Fixed during the review (verified in the current tree)

- `bridgeModeMismatch` no longer reads an inspect failure as "no mismatch": it returns
  `(false, nil)` for `IsNotFound` and `(false, err)` otherwise, and `EnsureRunning:233-236`
  turns that error into a refusal to start — "refusing to start on an unverified bridge".
  That closes the third fail-open in this path (BUG-031) in the correct direction: the
  container stays down, which is the safe state.
- The marker delete is now conditional on the container being provably still running
  (`unmarkIfStillRunning`), not unconditional, which removes the resurrect-by-reconnect
  window I raised against the earlier blob. See the low finding above for its residual.
- `rollbackMode` distinguishes `project.ErrNotFound` (a concurrent delete) at INFO from a
  real store failure at ERROR.

## Does clearing `o.stopped` on a failed stop weaken anything? No.

1. **It is not an authorization control.** The only readers of `Stopped()` are
   `internal/terminal/bridge.go:123` (a non-`boot=1` WS reconnect gets close 4002) and
   `bridge.go:407` (close-code selection). No handler, no proxy, no `EnsureRunning` path
   consults it. Nothing about *who* may do *what* depends on it.
2. **No admin ever stops a non-admin's project through this code.** `stopProject`
   (`server.go:1070`) resolves the project via `getProjectFor` -> `s.who(r)` -> the CALLER's
   own `userSpace`; there is no cross-user stop endpoint and no impersonation route. So "a
   project an admin meant stopped" exists only for the admin's own projects, or for the
   boot-time workspace migration (`cmd/styrman/main.go:276`), which runs before the server
   serves traffic, logs "project NOT migrated this boot", and retries next boot. There is no
   cross-tenant surface here to weaken.
3. **The owner already has every capability the marker withholds.** With the marker set the
   owner can still `POST /api/projects/{id}/start` or attach with `?boot=1`. Clearing it
   removes only the guard against AUTOMATIC reconnect boots. No state is reachable with the
   marker cleared that was unreachable with it set.
4. **The current form only clears it when nothing was stopped.** `unmarkIfStillRunning`
   deletes the marker solely when `ContainerInspect` reports `State.Running` — i.e. exactly
   when the project is running as before and "deliberately stopped" is a false claim. A
   container that is down, or that cannot be inspected, keeps its marker. So the resurrection
   path (stop errors client-side, completes daemon-side, marker gone, an open tab's automatic
   reconnect boots the VM) is closed in the current tree; it existed in the earlier blob and
   was confined to the tenant's own project and quota even then.
5. **Against the idle reaper it grants nothing new.** `stopIfIdle` already yields to a busy
   project by design (`projectBusy` blocks the stop on any running DinD container or
   non-shell foreground pane), so a tenant who wants to keep their VM alive has a supported
   way to do it; the marker is not what holds them to the idle policy.
6. **Intersection with the LAN change.** In the failed-`Stop` revocation branch the marker
   may now be cleared, so the tenant's terminals reconnect into a VM still on the `lan-`
   bridge. That grants nothing: the container keeps running on that bridge with or without
   the marker, the owner could reconnect with `boot=1` regardless, and `EnsureRunning` on a
   RUNNING container neither heals nor worsens the mode. The record is rolled back, so the
   admin sees LAN=on — which is the point of the fix.
7. **Direction of travel is a tightening.** Previously the marker could stand for a project
   that never stopped, so every reconnect to a *running* project was refused with "project
   stopped" — a false statement, on the very error path that tells the user nothing changed.
   The marker now means what its name says.

## Checked

- **Secrets:** none in the diff. `rollbackMode` and the new orchestrator code log project ID,
  booleans and error strings only — no credential material, no path material beyond the
  wrapped Docker error, no email beyond the pre-existing `"by", u.Email` audit line.
- **Injection:** no new parsing boundary. The only external input is `{"lanAccess": bool}`,
  decoded into `*bool` with an explicit nil check (400 otherwise); the project ID comes from
  `r.PathValue` and is resolved through the caller's own store before use. Nothing reaches a
  shell, SQL, template, or filesystem path.
- **Authz/authn:** unchanged and correct — 404 before any authorization signal
  (`getProjectFor` first, so another user's project is indistinguishable from a missing one),
  `requireAdmin` on enabling only (tightening is never refused), `deletingState` resurrection
  guard, and the new Status-error branch returns before any orchestrator call, so it cannot
  be used to probe another tenant. `rollbackMode` is unreachable without passing all of them.
- **Crypto:** nothing hand-rolled, nothing touched.
- **Dependencies:** one new *internal* import (`internal/project`, for `ErrNotFound`); no new
  third-party dependency. `govulncheck ./...` on the root module reports 31 called
  vulnerabilities, all in the **Go standard library at go1.25** (e.g. GO-2025-4007, quadratic
  x509 name-constraint checking, fixed in go1.25.3), reached through `crypto/x509` and
  `net/http` under the Docker client. None is introduced or made reachable by this diff, but
  the release builds on a toolchain with 31 known-called stdlib vulns: the fix is to bump the
  builder image's Go to >= 1.25.3 and re-run `bash deploy/vulncheck.sh`, which is the
  authoritative two-module run. I ran the host `govulncheck`, not that script.
- **GCP surface:** not applicable — self-hosted; `docs/deploy/gcp.md` records it as out of
  scope. No IAM, service account, or Secret Manager surface in the diff.

## Not examined

- The vendored Caddy ingress module (`deploy/caddy/ingress`) — untouched by this diff and not
  in its blast radius; its separate `go test ./...` and its `govulncheck` leg were not run.
- `deploy/vulncheck.sh` itself (the pinned-builder, two-module scan). I ran only the host
  `govulncheck ./...` on the root module.
- `deploy/firewall/` — ADR-067 Phases 2/3 deleted the root-mediated LAN machinery and none of
  it is reachable from these three files.
- The other files dirty in the worktree — `.agents/docs/specs/styrman/versioning.md`,
  `web/dist/client/_shell.html`, `DECISIONS.md`, `bugs/INDEX.md`, and the new BUG-029/030/031
  records — were outside the range I was asked to assess and were not reviewed as code.
- Live behaviour: no integration run (`.agents/docs/testing/styrman/integration.sh`), no Kata
  run, no browser e2e. Everything above is from reading the code and the unit tests.
- Findings 1, 2 and the `unmarkIfStillRunning` context issue are failure modes I reasoned
  about from the source, not reproduced. No exploit was written, and none should be inferred
  from this file.
