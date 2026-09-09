---
owner: reviewer
date: 2026-09-09
---
# Review — preflight v1.25.0 round 2 (response to the 2026-09-09 review)   (2026-09-09)

Range reviewed: eccd1a0..worktree
Files judged: the uncommitted diff on `internal/httpapi/network.go`,
`internal/httpapi/network_test.go`, `internal/orchestrator/orchestrator.go`
(36/13, 52/1, 19/2 in `git diff --numstat` — all text, no binary lines, no
unexplained deletions; the deletions in `network.go` are the inlined rollback
now living in `rollbackMode`, and the one in `network_test.go` is the moved
`"strings"` import). Other dirty paths (`integration.sh`, `web/e2e/*`,
`web/src/lib/api.ts`, `.gitattributes`) were out of scope by dispatch.

## Verdict: APPROVE WITH NITS

All three claimed findings are genuinely closed, and closing them introduced
one residual, not a regression of the class it fixed. Finding 1's fail-open is
gone: a `Status` error now rolls the record back and answers 500 without
touching the container, and `!requested` is provably the in-force value on both
branches and in both directions. Finding 2's latch is cleared on the failure
branch. Finding 3 is clean (`gofmt -l ./internal ./cmd` is silent). The residual
is in the *new* orchestrator code: `stopLocked` returning an error does not
prove the container is still running, and on the paths where it stopped anyway
the unconditional `stopped.Delete` re-opens the auto-reconnect resurrection the
marker exists to prevent. That is low severity — a VM boot, no isolation or
record consequence — and worth fixing before the tag only if the fix is the
three lines below.

## Findings

- **[low / confidence: medium]** `internal/orchestrator/orchestrator.go:992-1002`
  and `:1082-1090` — `stopLocked` failing is treated as proof the container is
  still running; it is not, and on the paths where it is not, deleting the
  marker lets an open tab boot a project that is in fact stopped. Two reachable
  shapes: (a) partial stop — `ContainerStop` returns a transport error (daemon
  connection dropped, caller ctx cancelled after the request reached the daemon)
  while the daemon completes the stop; (b) a reaper tick racing a user Stop —
  `Adopt`'s snapshot said running, the user's Stop then succeeded and set the
  marker, `stopIfIdle` re-enters (`projectBusy` reads an exec failure on a
  stopped container as idle, `orchestrator.go:1161`), `stopLocked` errors for a
  transient daemon reason, and the reaper's `Delete` clears the marker the
  user's successful Stop set. In both cases the container ends up stopped with
  no marker, so the first automatic terminal reconnect passes the gate at
  `internal/terminal/bridge.go:123` and `EnsureRunning` boots the microVM —
  exactly what the "mark BEFORE stopping" comment was written to prevent. Fix:
  make the delete conditional on observed state rather than on the error, e.g.
  after `stopLocked` fails, `insp, ierr := o.docker.ContainerInspect(ctx, name)`
  and `Delete` only when `ierr == nil && insp.State != nil && insp.State.Running`
  — keep the marker when the state is unknown or stopped, which is the same
  "an unknown state is not a stopped state" rule the `network.go` half of this
  diff just adopted. Both call sites want the same helper.

- **[low / confidence: high]** `internal/orchestrator/orchestrator.go:992-1002`,
  `:1082-1090` — the new marker-clearing behaviour has no covering test. It is
  the whole point of the change and it is invisible to the suite: `o.docker` is
  a concrete `*client.Client` (`orchestrator.go:34`), so `Stop`'s failure branch
  cannot be driven, and `internal/httpapi`'s `fakeMode` has no `stopped` latch,
  which is precisely why the original finding 2 was invisible there. `go test`
  passing says nothing about these 19 lines. Either introduce the one-method
  seam the failure branch needs (the same move `modeRuntime` already made in
  `httpapi`), or record in the changelog that the branch is reasoned-only — but
  do not let a green suite stand as evidence for it.

- **[low / confidence: high]** `internal/httpapi/network.go:44-58` — the previous
  review's finding 4 is untouched and the refactor widened it: a concurrent
  project delete makes `SetLanAccess` return `project.ErrNotFound`, and
  `rollbackMode` logs ERROR "record disagrees with the running container" for a
  project that no longer exists. There are now two callers reaching that line
  instead of one. Fix as before: treat `errors.Is(rb, project.ErrNotFound)` as
  INFO "record already gone".

## Claimed findings, judged

- **Finding 1 (Status error treated as stopped) — CLOSED.** `network.go:135-146`
  splits the condition; the error branch calls `rollbackMode` and returns 500
  with "the project could not be inspected, so the mode was NOT changed: %w",
  wrapping the cause. `!requested` is provably in force on **both** branches and
  both directions: the no-op check at `network.go:113` guarantees
  `p.LanAccess != on` at the forward write, both writes are serialized by
  `netMu`, and on the `Status`-error branch **nothing has touched the
  container** — no `Stop`, no `EnsureRunning` (the test pins `stopCalls == 0 &&
  ensureCalls == 0`). So on an ENABLE (sandbox→LAN) that errors, the container
  is still on `sdb-` and the rollback to `false` reports what is in force; on a
  revocation it is still on `lan-` and the rollback to `true` does the same.
  I re-walked the concurrent-PUT interleavings (a second PUT completing a full
  rebuild before the rollback lands, in each direction) and every ordering ends
  with the record agreeing with the bridge; the one residual — a stale-snapshot
  no-op PUT getting a 200 the rollback then reverts — predates this range and is
  unchanged. The **"absent"/"stopped" path is behaviourally identical**: same
  `st.State != "running"` test, same 200 `lanView{LanAccess: on, CanEnable:
  u.Admin}` with `Rebuilt` false, same comment, no `Stop`/`EnsureRunning`, and
  `TestNetworkAbsentContainerJustWritesTheRecord` pins it. One consequence worth
  naming rather than filing: a revocation on a project whose container is
  absent-but-uninspectable is now *dropped* (record rolled back to LAN) where it
  used to be persisted. That is the correct trade — the caller is told 500 "NOT
  changed" and can retry, and guessing "absent" would be the fail-open again —
  but it means a tightening can fail on a daemon hiccup, which the operator must
  see and repeat.
- **Finding 2 (`o.stopped` latched before a failed stop) — CLOSED**, with the
  first finding above as the residual. `Stop` and `stopIfIdle` still mark before
  stopping (right, for the in-flight reconnects the comment describes) and now
  undo it when `stopLocked` errors. The dispatch's race does not exist in the
  direction asked: while the marker is set, a reconnect without `?boot=1` is
  *refused*, so it cannot boot the VM and then lose to the `Delete`; a
  `?boot=1` connect bypasses the marker but its `EnsureRunning` blocks on the
  same per-project lock `Stop` holds across Store→stopLocked→Delete, and it
  clears the marker itself on success (`orchestrator.go:359`) — so both
  orderings converge. The reaper's `projectBusy`/`Touch` path is unaffected: the
  busy branch returns at `orchestrator.go:1076` before the marker is ever
  stored, so `Touch` and the new `Delete` are on mutually exclusive branches.
  Other `Stop` callers are consistent with the new behaviour:
  `httpapi/server.go:1078` 500s and the project is still running,
  `cmd/styrman/main.go:276` aborts the migration and leaves it running.
- **Finding 3 (gofmt) — CLOSED.** `"path/filepath"` now precedes `"strings"`;
  `gofmt -l ./internal ./cmd` prints nothing, `go vet ./...` is clean, and
  `go test ./internal/httpapi ./internal/orchestrator` passes.
- **The fake's `statusErr` path matches the real `Status`.** `fakeMode.Status`
  returns `orchestrator.Status{}, err` — byte-for-byte the real
  `Orchestrator.Status`'s error return (`orchestrator.go:151`), which is reached
  only when `errdefs.IsNotFound` is false, i.e. a real inspect failure. The
  comment on the field states that invariant, so the double cannot drift into
  representing "absent" as an error. `state: "absent"` in the second test
  likewise matches the real mapping at `orchestrator.go:149`.

## Criteria

1. **Dead code** — pass. The inlined rollback was moved, not duplicated; no
   orphaned branch, no commented-out code, `rollbackMode` has two callers.
2. **Flow tracing** — pass with the residual above. Both new `network.go` exits
   wrap the cause with `%w`, the ctx with `defer cancel()` still covers all
   three calls, `rollbackMode` takes and releases `netMu` around the write only
   (never across an orchestrator call), and both orchestrator branches return
   the original error unchanged. The one hop that does not hold end-to-end is
   `stopLocked` error ⇒ "still running" (finding 1).
3. **Standards conformance** — pass. gofmt/vet/test green; errors wrapped; no
   discarded `err`; `.agents/rules/writing-files.md` checks done (numstat above,
   both files text, `network_test.go` appended with one moved import line).
4. **Directory layout** — pass. No new files; tests beside their subject.
5. **Simplicity (DIY-except-security)** — pass. No new dependency; the helper
   removes a duplicated block rather than adding a layer; no auth/crypto here.
6. **Decoupling** — pass. `rollbackMode` is a `*Server` method sized to its two
   callers; the orchestrator change is local to two sibling functions. The
   testability gap in finding 2 is the one seam the change wanted and did not
   get.

## Not examined

- Everything outside the three dispatched files: the other dirty paths
  (`.agents/docs/testing/styrman/integration.sh`, `web/e2e/run.mjs`,
  `web/e2e/run.sh`, `web/src/lib/api.ts`, the untracked `.gitattributes`), the
  `web/dist` bundle, and all docs (`BUG-028`/`BUG-029` read only to check the
  fix against their prevention rules).
- The already-reviewed 5481ea2..eccd1a0 range, except where a finding depends on
  it (`bridgeModeMismatch`'s stopped-only gate, `bridge.go`'s 4002 gate).
- Live paths: no Kata run, no `integration.sh`, no `web/e2e/run.sh`. The
  partial-stop shape in finding 1 is reasoned from the Docker SDK's error
  semantics, not observed — hence confidence medium.
