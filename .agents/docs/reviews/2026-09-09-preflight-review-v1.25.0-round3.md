---
owner: reviewer
date: 2026-09-09
---
# Review — preflight v1.25.0 round 3 (the four changes since round 2)   (2026-09-09)

Range reviewed: eccd1a0..worktree

Files judged: the uncommitted diff on `internal/httpapi/network.go` (45/13),
`internal/httpapi/network_test.go` (52/1 — byte-identical to what round 2 read;
the numstat is unchanged), `internal/orchestrator/orchestrator.go` (60/11, up
from round 2's 19/2). All three are text in `git diff --numstat`, no `-` column,
no deletion a reader has to explain: `network.go`'s 13 are the inlined rollback
now living in `rollbackMode`, `orchestrator.go`'s 11 are the old
`bridgeModeMismatch` signature, its doc paragraph, and the two bare
`return o.stopLocked(...)` lines. Other dirty paths (`integration.sh`,
`web/e2e/*`, `web/src/lib/api.ts`, `.gitattributes`) are out of scope by
dispatch. `gofmt -l ./internal ./cmd` silent, `go vet ./...` clean,
`go test ./internal/httpapi ./internal/orchestrator ./internal/project` green.

## Verdict: APPROVE WITH NITS

Nothing in this round is blocking and nothing regressed. The BUG-031 fix is
correct and fails closed in the direction that matters: on a non-NotFound
network-inspect error `EnsureRunning` returns before any `ContainerRemove`, so
the container the toggle handler just stopped stays stopped and intact, the
record keeps the requested mode, and the handler's own `EnsureRunning`-error
branch reports "saved, but the project could not be restarted … start it to
apply the change" — a stopped container has nothing in force, so record-equals-
requested is the honest answer there. Two of round 2's three lows are genuinely
closed; the third (no test for the orchestrator failure branches) stands and is
now disclosed in BUG-030, BUG-031 and the changelog, which is the alternative
round 2 offered. The one residual worth the operator's attention is that
`unmarkIfStillRunning` re-inspects with the SAME context whose expiry is the
most likely reason the stop failed, so in BUG-030's own headline repro the
marker is still left stuck — the default direction is right, the context is
not.

## Findings

- **[low / confidence: high]** `internal/orchestrator/orchestrator.go:1030-1035`
  — `unmarkIfStillRunning` passes the caller's `ctx` to `ContainerInspect`. The
  two most reachable causes of a `stopLocked` error both poison that context:
  `stopProject` gives `Stop` a 2-minute budget (`httpapi/server.go:1069`) and
  the toggle handler a 3-minute one (`network.go:141`), and a Kata guest that
  will not power off burns it inside `Exec` + `waitStopped` + `ContainerStop`,
  so the error is `context deadline exceeded` and the follow-up inspect fails
  instantly on the same expired context. The `err != nil` branch keeps the
  marker — which is the right default for an unknown state, but here the state
  is not really unknown and the container is very likely still running, i.e.
  exactly BUG-030's recorded repro ("a Kata shutdown that wedges, a Docker
  timeout") ends with terminals refused close 4002 for a running project, the
  symptom the fix was written for. The client-cancelled case is the same shape.
  Concrete fix, three lines: inspect on a fresh budget —
  `ictx, cancel := context.WithTimeout(context.Background(), 5*time.Second);
  defer cancel()` — and pass `ictx`. That leaves the conservative default in
  place for a genuinely unreachable daemon while letting the common
  deadline case be decided on evidence. Recovery today is a Start click
  (`EnsureRunning` clears the marker at `orchestrator.go:364` even when it
  finds the container already running), so this is an annoyance, not a
  correctness or isolation defect.

- **[low / confidence: high]** `internal/orchestrator/orchestrator.go:818-824`
  — the rewritten doc comment does not describe the code as it now is
  (dispatch question 4). Two defects. (a) A stranded clause from the old text:
  "any OTHER inspect error is RETURNED, never read as 'no mismatch' — it once
  was (BUG-031), **either, so a Docker hiccup can never destroy a project's
  container**". The "either" belonged to the deleted sentence "an inspect error
  is not a mismatch either"; as it stands the sentence is unparseable, and the
  claim it carries (true — errors never yield `true`) now hangs off the wrong
  clause. (b) "The cost of a false negative is that the toggle is applied on
  the next start instead of this one" is now true only of the NotFound branch;
  for every other error the cost is that **the start is refused**, and the doc
  comment never says so — the caller's behaviour (`EnsureRunning` returns
  "refusing to start on an unverified bridge", so a transient Docker fault
  makes a stopped project unstartable rather than startable-on-the-wrong-
  bridge) is the single most important thing a future reader of this signature
  needs. The in-body comments at :829-840 are accurate; it is the paragraph
  above the function that is stale. Fix: drop the "either" clause and replace
  the false-negative sentence with one naming the two outcomes — NotFound
  defers the toggle to the next start, any other error refuses the start.

- **[low / confidence: high]** `internal/orchestrator/orchestrator.go:1005-1035`,
  `:1111-1121`, `:825-848` — round 2's low #2 is unchanged and the surface it
  covers grew: three orchestrator branches (the two `unmarkIfStillRunning` call
  sites and `bridgeModeMismatch`'s error return) are invisible to the suite,
  because `o.docker` is a concrete `*client.Client` (`orchestrator.go:34`) and
  cannot be made to fail. One of them is now a SECURITY fix (BUG-031). The
  httpapi tests exercise the handler above it, not these lines: no test in the
  tree drives a `NetworkInspect` failure or a `stopLocked` failure. This is
  disclosed — BUG-030 and BUG-031 each say "No unit test", and the changelog's
  final section says the low stands — which is the second option round 2 named,
  so it is not a blocker for the tag; it belongs in the release notes as a
  known cover gap and in TODO.md against BUG-003's `dockerAPI` seam, which the
  2026-09-04 changelog already sized at ~166 lines / 19 methods.

## The dispatch's four questions, answered

1. **`bridgeModeMismatch` → `(bool, error)`; `EnsureRunning` refuses.**
   Correct, and the container is left safe. On the `merr` path
   (`orchestrator.go:233-236`) nothing has been touched: the `ContainerRemove`
   is inside the `mismatch` branch below, the earlier pre-ADR-027 and image-
   upgrade recreates already ran and left `err == errdefs.NotFound` only when
   they removed the container (in which case this branch is skipped, since it
   requires `err == nil`). So the container is stopped, intact, on its old
   bridge, with the record on the requested mode — nothing is in force, which
   is what makes record-equals-requested honest. NotFound is still excluded, so
   the `recreateMissingNetwork` fallback at :266 is untouched and a genuinely
   missing network still starts normally.
   **Other callers do now fail where they used to succeed**, and that is
   acceptable: `ensureFor` (`server.go:233`, terminal attach / boot-on-
   reconnect), `startProject` (`server.go:1058`), background project boot
   (`server.go:859`) and the toggle handler all reach the same branch, but only
   for a container that EXISTS and is NOT running — a brand-new project takes
   `errdefs.IsNotFound(err)` at :275 and never inspects the network. The new
   failure needs `ContainerInspect` to succeed while `NetworkInspect` fails
   with something other than 404, which is a narrow daemon fault, and the
   alternative on that fault is precisely BUG-031 (start on an unverified
   bridge). The user sees a 500 carrying "refusing to start on an unverified
   bridge" and can retry; the check is re-run on every start, so nothing
   latches.
2. **`unmarkIfStillRunning`.** The direction is right in both senses: a marker
   wrongly KEPT costs a Start click, a marker wrongly DELETED resurrects a VM
   the user or the reaper deliberately stopped (round 2's shapes (a) partial
   stop and (b) reaper racing a user Stop are both real — `projectBusy` reads
   an exec failure on a stopped container as idle, `orchestrator.go:1161`), so
   "unknown is not stopped" is the correct default and matches the rule the
   `network.go` half of this diff adopted. Both call sites use the one helper,
   both still mark before stopping, both return the original error unchanged.
   The reaper's ctx is the long-lived loop context (`orchestrator.go:1315`), so
   the re-inspect is meaningful there; the two HTTP call sites are the ones the
   first finding above is about.
3. **`rollbackMode` and `project.ErrNotFound`.** Correct. `SetLanAccess`
   returns the bare sentinel (`project/store.go:228`), so `errors.Is` matches;
   the branch logs Info with the cause and returns before the Error line, the
   Error line still fires for a real persistence failure (the one case that
   needs a human), and the Warn line still fires on a successful rollback. The
   ordering of the three branches is right — ErrNotFound is checked before the
   generic `rb != nil`. Both callers pass the same `on`/`!on` pair, and both
   still return 500 to the caller, which is right even for the deleted-project
   race: the client asked for a change that did not happen.
4. **The comments.** In-body comments in `bridgeModeMismatch` (:829-840),
   `Stop` (:1008-1014), `unmarkIfStillRunning` (:1022-1029), the reaper
   (:1115-1117) and both `network.go` branches describe the code accurately,
   including the BUG-030/BUG-031 attributions. The function-level doc paragraph
   above `bridgeModeMismatch` does not — see the second finding.

## Criteria

1. **Dead code** — pass. `rollbackMode` two callers, `unmarkIfStillRunning` two
   callers, `bridgeModeMismatch` one caller and no stale second signature; no
   commented-out block, no unreachable branch (the NotFound arm is reachable
   via `recreateMissingNetwork`'s own sweep case).
2. **Flow tracing** — pass. `bridgeModeMismatch`'s error is wrapped with `%w`
   at its one caller and surfaces through all four `EnsureRunning` callers as a
   500 or a logged boot failure; `stopLocked`'s error is returned unchanged
   after the helper; `rollbackMode` takes `netMu` for the write only, never
   across an orchestrator call; the toggle handler's `defer cancel()` still
   covers Status/Stop/EnsureRunning. The one hop that does not hold end-to-end
   is `unmarkIfStillRunning`'s reuse of an expired context (finding 1).
3. **Standards conformance** — pass. gofmt/vet/test green; errors wrapped or
   deliberately consumed with a comment; no discarded `err`; `errdefs.IsNotFound`
   is the same NotFound test the rest of the file uses on network inspects
   (:881), so the new branch cannot disagree with `recreateMissingNetwork`
   about what "missing" means. `.agents/rules/writing-files.md` checks done
   (numstat above; no unexplained deletions; both files text).
4. **Directory layout** — pass. No new files; the helper sits beside the two
   functions that call it; tests beside their subject.
5. **Simplicity (DIY-except-security)** — pass. No new dependency; both helpers
   remove duplication rather than adding a layer; the `(bool, error)` signature
   is the stdlib-idiomatic way to say "I could not tell". No auth/crypto code
   in range.
6. **Decoupling** — pass. `rollbackMode` is a `*Server` method sized to its two
   callers; `unmarkIfStillRunning` and `bridgeModeMismatch` are private and
   local to their siblings; the httpapi `modeRuntime` seam is unchanged. The
   missing seam is still `o.docker` (finding 3).

## Round 2's three lows

- **Low 1 (unconditional `stopped.Delete` on a failed stop) — CLOSED**, with
  finding 1 above as the residual on the same helper's context.
- **Low 2 (no covering test for the marker-clearing branch) — OPEN, disclosed.**
  No orchestrator test was added; `network_test.go` is unchanged since round 2.
  BUG-030, BUG-031 and the changelog all state it explicitly, which was round
  2's stated alternative.
- **Low 3 (`ErrNotFound` rollback logged at ERROR) — CLOSED.**

## Not examined

- Everything outside the three dispatched files, including the other dirty
  paths and `web/dist`. Docs were read only as evidence: BUG-028 through
  BUG-031, `bugs/INDEX.md`, `changes/2026-09-09-adr-067-preflight-fixes.md`,
  and round 2's report.
- The already-reviewed 5481ea2..eccd1a0 range, except where a finding depends
  on it (`EnsureRunning`'s recreate ladder, `stopLocked`, the reaper loop,
  `recreateMissingNetwork`, `project.Store.SetLanAccess`).
- Live paths: no Kata run, no `integration.sh`, no `web/e2e/run.sh`, no
  `-race` run of my own (the changelog records one). Finding 1's deadline shape
  is reasoned from the call sites' timeouts and the Docker SDK's context
  handling, not observed — the mechanism is certain, the frequency is not.
