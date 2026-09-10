---
owner: security
date: 2026-09-10
---
# Security review — network mode, record-first (round 3)   (2026-09-10)

Range reviewed: `0378825..worktree` — the CURRENT uncommitted working tree
(`git diff`), dispatched as `.agents/scratch/network-record-first.diff`.

## Blobs this review was written against

Pinned with `git hash-object` before the read; `git diff` of the working tree
was byte-compared against the dispatched file and is **IDENTICAL**, so for the
first time in four rounds the target did not move under the review.

| file | blob | numstat |
|---|---|---|
| `internal/httpapi/network.go` | `418af58` | 121/101 |
| `internal/httpapi/network_test.go` | `28ac75a` | 295/67 |
| `internal/httpapi/server.go` | `614408e` | 20/9 |
| `internal/orchestrator/orchestrator.go` | `40b6549` | 129/24 |
| `.agents/scratch/network-record-first.diff` | `efd2ea3` | (the dispatched diff) |

`HEAD` = `0378825`. Integrity: all four are `Unicode text, UTF-8` and no
numstat column is `-` (BUG-007 clean); `network_test.go` goes 10 to **17**
`func Test` with two renamed (`...RollsBack` -> `...ChangesNothing`) and none
dropped in substance (BUG-005 clean); `git status --porcelain` shows only the
four dispatched paths plus five untracked review/testing artifacts, no
unexplained ` D` (BUG-006 clean).

## Verdict: FINDINGS

**The round-2 mechanism is genuinely deleted and every row of the round-2
interleaving table closes — and the same class of bug survives one level up,
in the handler's own decision inputs.** `RunOpts.Lan` is now a read evaluated
inside `ensureRunningLocked` under the project lock
(`orchestrator.go:163-190`, `:264-267`), all four production call sites pass
`lanOf` (`server.go:93-101`), and the whole PUT decision runs inside one
`WithProject` hold (`network.go:128`). A1-A5 and B1-B4 all close; the residual
set is now a strict subset of v1.25.0's on both branches. What did **not** get
the same treatment is `p.LanAccess` itself: the handler still compares the
request against the copy `getProjectFor` snapshotted *before* the lock
(`network.go:100`, read at `:154`), so a PUT that loses a race to another PUT
can take the "nothing to do" branch, write nothing, and answer `200` with the
mode the caller asked for while the record keeps the other one. On a stopped
project — the branch where `wrongBridge` is forced `false` and the live bridge
is therefore never consulted — that is a silently-dropped revocation.

All six directed checks were run; five come back clean, one (the three-minute
lock hold) comes back "unchanged in kind, wider in span by two Docker
inspects". Details below.

## Round-2 interleaving table, re-run

Symbols as in round 2: `t_r` the racing start's record read, `t_e` its lock
acquisition, `t_s` the PUT's `Status`, `t_p` the persist, `[t_lock, t_unlock]`
the hold. Direction throughout is the dangerous one: revoke LAN, record to
sandbox, the racer started life believing LAN.

The two structural facts that decide every row:

1. `t_r` no longer exists as a distinct moment. `lanOf` (`server.go:93`) is a
   closure over `(*userSpace, id)`; it is evaluated by `opts.lan()` at
   `orchestrator.go:266`, the first statement of `ensureRunningLocked`, which
   every caller reaches only with the project lock held (`EnsureRunning`
   `:192-196`, `lockedProject.EnsureRunning` `:258-260`). The record read and
   the bridge decision are the same critical section.
2. `t_s`, `t_p` and the stop/start are all inside one `WithProject` hold
   (`network.go:128-211`), so nothing can interleave *between* the PUT's own
   steps any more.

| # | Round-2 row | Status now | Faults to anything remaining |
|---|---|---|---|
| A1 | start wins the lock before the rebuild | **Closed** (was already fine). The start completes on the old bridge under the old record; the PUT then sees `running`, `ModeMismatch` true, stops, persists, rebuilds. | — |
| A2 | start arrives after a successful rebuild | **Closed.** It reads the NEW record via `lanOf`; the container is running on the new bridge, so `ensureRunningLocked`'s mode branch (`:319`, stopped-only) correctly no-ops. The stale value that made this row need an argument is gone. | — |
| A3 | rebuild failed at **stop** | **Closed.** Record untouched, container up on the old bridge, the late start reads the old record and no-ops. Consistent. | — |
| A4 | rebuild failed at **persist** | **Closed.** Record still LAN, container down and marked stopped; the late start reads LAN and brings it up on `lan-`. Record equals force; the 500 said exactly this. | — |
| A5 | rebuild failed at **start**, container gone, network already `sdb-` | **Closed — this was round 2's only two-fault fail-open.** The late start now reads the persisted record (sandbox) rather than its caller's stale `true`, so `create()`'s bridge comparison (`:959-969`) rebuilds toward `sdb-`, not `lan-`. | — |
| A6 | `waitReady` failed with the container up on `sdb-` | **Closed** (was already safe). | — |
| B1 | whole start fits between the PUT's `Status` and its persist | **Closed.** `Status` is now `pr.Status` inside the hold (`network.go:129-137`); the start cannot acquire the lock until the PUT has replied. | — |
| B2 | start straddles the whole handler | **Closed.** Same reason as B1 — and even the "before" half is harmless, because the value the start uses is read at `t_e`, not at `t_r`. | — |
| B3 | start holds the lock at `t_s` (inspect done, `ContainerStart` pending) | **Closed.** The PUT's `Status` cannot run at that moment; it is behind the same lock. | — |
| B4 | enable direction racing a stale `Lan:false` start | **Closed.** No stale `Lan` exists to race with. | — |

**Every row closes, and none of them is downgraded to "needs N faults" — they
are structurally unreachable, not merely unlikely.** The residual on both
branches is now a strict subset of v1.25.0's: v1.25.0 allowed the
record-vs-bridge divergence at zero faults on both, this code allows it on
neither.

The one interleaving that *remains* is not in the round-2 table at all,
because round 2 only modelled PUT-vs-start. It is PUT-vs-PUT — finding 1.

## The six directed checks

1. **No path takes `netMu` then a project lock — CONFIRMED, no deadlock.**
   `grep -rn netMu` gives exactly two live sites.
   `network.go:124-126` (`persist`) runs inside the `WithProject` closure, so
   the order there is `projLock -> netMu -> storeMu`; `netMu` is released by
   `defer` before `pr.Stop`/`pr.EnsureRunning`, so it is never held across a
   Docker call. `server.go:1017-1019` (`deleteProject`) wraps
   `sp.projects.Remove` only, and `s.orch.Destroy` — the one thing there that
   takes a project lock (`orchestrator.go:1327`) — has **returned** at
   `server.go:961` long before, so the two holds are sequential, not nested.
   `project.Store`'s methods (`Get`/`SetLanAccess`/`Remove`,
   `internal/project/store.go:223`, `:244`, `:255`) take only the store mutex
   and never call the orchestrator, so the innermost level cannot re-enter.
   No `netMu -> projLock` edge exists; no cycle.
2. **No locking orchestrator method is called for the same project inside the
   `WithProject` closure — CONFIRMED.** The four `Project` methods are all
   lock-free bodies: `lockedProject.Status` -> `o.Status`
   (`orchestrator.go:147`, **takes no lock** — confirmed as asked, it is a
   bare `ContainerInspect`); `lockedProject.ModeMismatch` -> `currentBridge`
   (`:904`, no lock); `lockedProject.Stop` -> `stopMarked` (`:1110`, the
   documented locked body); `lockedProject.EnsureRunning` ->
   `ensureRunningLocked` (`:264`, the documented locked body). The only other
   orchestrator call in the closure is `s.orch.EnsureSelfAttached`
   (`network.go:207`), and it is (a) in a new goroutine after a 1 s sleep and
   (b) lock-free anyway (`:616-673`). `lanOf` inside the closure touches the
   store mutex only.
3. **`lanOf` cannot return a stale value — CONFIRMED.** It closes over the
   `*userSpace` and the id string, not a `Project` struct, and calls
   `sp.projects.Get(id)` at evaluation time; `Get`
   (`internal/project/store.go:244-253`) reads the live map under the store
   mutex and returns a copy. `sp` itself is the per-user singleton from
   `s.space` (one `*project.Store` per user), so a write through the same
   `sp` is visible to the read. `TestLanOfReadsTheRecordAtCallTime`
   (`network_test.go:609`) asserts exactly this, including that a removed
   record is an error rather than a default mode.
4. **The nil-`Lan` default is fail-closed and no production caller passes nil
   — CONFIRMED, with one gap in test coverage.** `RunOpts.lan()`
   (`orchestrator.go:182-188`) returns `(false, nil)` = sandbox = the
   restrictive mode. `grep -rn "RunOpts{"` gives four production sites
   (`server.go:244`, `:871`, `:1070`, `network.go:195`) and all four pass
   `lanOf(sp, p.ID)`. A `Get` failure returns an error, and
   `ensureRunningLocked` refuses to start on it (`:266-268`) rather than
   defaulting — fail-closed twice over. Gap: nothing tests the nil branch;
   `fakeMode.EnsureRunning` (`network_test.go:321-323`) calls `opts.Lan()`
   directly (it cannot see the unexported helper) and would panic on nil, so
   the default is asserted nowhere. See the informational note below.
5. **Authz ordering unchanged — CONFIRMED.** `network.go:93-118`: body decode
   -> `getProjectFor` 404 (before any authorization signal) -> `on &&
   requireAdmin` (enable admin-only, disable never refused) -> `deletingState`
   409 -> only then the lock and any Docker call. `CanEnable` is still its own
   field (BUG-019). `git diff -- internal/httpapi/server.go` touches only the
   `modeRuntime` interface, `lanOf`, and three `RunOpts` literals — no route,
   no middleware, no guard moved.
6. **The three-minute hold — unchanged in kind, wider by two Docker inspects.**
   Round 2's `Rebuild` already held the project lock across stop+persist+start
   under the same `lanRebuildTimeout` (`network.go:35`). This version adds
   `Status` and `ModeMismatch` (two inspects, bounded by the same ctx) and the
   `writeJSON`/`writeErr` inside the hold. What blocks meanwhile is what
   blocked before: `Destroy` (`orchestrator.go:1327`), `Stop` (`:1103`) and
   the idle reaper's `stopIfIdle` (`:1204`), all on the same per-project
   mutex. Security-relevant consequences, both bounded and both pre-existing
   in kind: a delete already flagged in-flight waits up to 3 min before its
   `Destroy` runs (the flag is set first, so no *new* work starts meanwhile —
   `ensureFor`, `startProject` and this handler all check `deletingState`),
   and the reaper's tick is **serial** over `Adopt`'s map
   (`orchestrator.go:1414-1434`), so one project's 3-minute hold delays the
   idle-stop of every other project in that tick. Neither is a fail-open;
   both were true of `Rebuild`. Not raised as a finding, recorded so the next
   round does not re-derive it.

## Findings

- **[medium / confidence: high]** `internal/httpapi/network.go:154` (the
  `p.LanAccess == on && !wrongBridge` no-op test) reading `p` snapshotted at
  `:100`, before the lock — **the handler fixed the stale record read on the
  start path and kept it on its own decision path, so one PUT can silently
  drop another's revocation and answer 200.** Scenario, all authorized, no
  failure anywhere, no Docker anomaly: project P is **stopped** and its record
  says sandbox. An admin issues PUT `{"lanAccess":true}` (call it E) and, a
  moment later, PUT `{"lanAccess":false}` (call it D) — a second tab, a retry,
  the UI switch double-clicked. D's `getProjectFor` runs first and snapshots
  `p.LanAccess == false`; E then takes the project lock, persists `true`, and
  replies. D now enters `WithProject`, `Status` reports `stopped`, so
  `wrongBridge` is left `false` at `:143` and `ModeMismatch` is never called —
  the live bridge, the one thing that is not stale, is deliberately not
  consulted for a stopped project. `:154` compares D's **stale** snapshot
  (`false`) against its request (`false`), calls it a no-op, writes nothing,
  and answers `200 {"lanAccess":false,"rebuilt":false}`. The record still says
  LAN. The next start — `startProject`, a terminal auto-reconnect through
  `ensureFor`, or the boot after a host reboot — reads that record through
  `lanOf` and brings the project up on `lan-<hash>`, which the root firewall's
  `-i sdb-+` jumps in INPUT and DOCKER-USER do not match: the project reaches
  the host (including its sshd) and the LAN, while the operator was told
  sandbox. This is the *same* defect class the diff just deleted one level
  down (a record value read outside the lock that serializes the writes), and
  it is the one input to the decision that did not get `lanOf`'s treatment.
  The reverse direction (stale `true`, request `true`, record actually
  `false`) under-claims and is safe. Not cross-tenant: `getProjectFor`
  resolves through the caller's own `userSpace`, so E and D are the same
  principal or the project's owner plus an admin; what breaks is the
  operator's belief about which projects `-i sdb-+` covers. Confidence is high
  on the code path (`p` is captured by the closure and is never re-read;
  `wrongBridge` is unconditionally `false` for a non-running container) and
  high on reachability — two PUTs on one project, no failure required.
  **Remediation, the shorter one:** re-read the record inside the closure and
  drop the outer snapshot from the decision — `cur, err := sp.projects.Get(p.ID)`
  as the first line of the `WithProject` body (a `Get` error is a 500, and it
  doubles as the delete-in-flight check that `:113` can only do before the
  lock), then compare `cur.LanAccess` at `:154` and log `cur.LanAccess` at
  `:167`. That is the same fix as `lanOf`, applied to the same kind of read,
  and it removes a variable rather than adding a guard. The narrow
  alternative — call `ModeMismatch` for stopped containers too — costs a
  Docker inspect on every no-op PUT and still leaves the record read stale.

- **[low / confidence: high]** `internal/httpapi/network.go:135`, `:149`,
  `:168`, `:180`, `:191`, `:199` — **the 500 bodies still wrap raw Docker and
  store errors back to the caller**, via `writeErr` ->
  `{"error":{"message": err.Error()}}` (`server.go:1272-1277`). Daemon socket
  paths, container/network names, bridge names and state-dir paths reach an
  authenticated tenant, and the revoke direction needs no admin, so a
  non-admin owner reaches all six. Carried unchanged from rounds 1-3; round
  2's Response section is silent on it, so it is neither fixed nor rebutted.
  Nothing here leaks another tenant's data or any secret — that is why it
  stays low. **Remediation** unchanged: log the cause with `s.logger`, return
  a fixed sentence.

- **[low / confidence: low]** `internal/httpapi/network.go:155`, `:174`,
  `:210` — **the HTTP response is written while the project lock is
  held.** `writeJSON` encodes into the `http.ResponseWriter`, and on the
  failure branches the body carries wrapped Docker error text. Today every
  body is far under `net/http`'s 4 KiB response buffer, so no write reaches
  the socket inside the hold and a slow-reading client cannot stall anything.
  It becomes real if a body ever grows past the buffer or someone adds a
  `Flush`: a client that stops reading would then pin one project's mutex —
  blocking that project's start, stop, delete and idle-stop — for as long as
  it likes, with no timeout, since the handler's `ctx` bounds the Docker calls
  and not the response write. Confidence is low precisely because it is
  latent, not live. **Remediation:** compute the reply inside the closure and
  write it after `WithProject` returns (a small named result struct), or at
  minimum a comment saying why the body must stay small.

- **[informational — round 2 finding 2 (`ModeMismatch` asks the network, not
  the container), rebuttal accepted with a residual]**
  `internal/orchestrator/orchestrator.go:235-253`. The rebuttal ("it takes
  host root to attach a container to another network, and the firewall matches
  on the bridge, which is a property of the network") is sound for this threat
  model: the guest's dockerd is a separate daemon inside the microVM and
  cannot reach the host's networks, so the actor who could produce the state
  can already do anything to the container. Recorded as closed by argument,
  not by code. Residual, unchanged and accepted: `ModeMismatch` answers about
  `config.NetworkName(project)`, so a *running* container attached elsewhere
  while its correctly-named network exists still reads as "no mismatch" and
  takes the record-only branch. The **missing-network** half of round 1
  finding 1 *is* fixed in code, and correctly inverted relative to the start
  path — `lockedProject.ModeMismatch` returns `(true, nil)` with a WARN at
  `:241-251`, while `bridgeModeMismatch` keeps `(false, nil)` at `:940-943`
  for the stopped-container caller that `recreateMissingNetwork` owns. Two
  callers, two right answers, both commented.

- **[informational — round 2 finding 3 (undocumented lock order), fixed as
  claimed]** `internal/httpapi/network.go:119-127` now states the invariant at
  the persist closure ("it is always the INNER lock... the two cannot
  deadlock"), and `orchestrator.go:217-218` states the other half ("fn must
  not call back into a locking method of Orchestrator for the same project").
  Between them the rule a future `netMu` site needs is written down. The
  `netMu` field comment (`server.go:129-133`) still only says "Never held
  across a rebuild", which remains **true** (the `defer` releases it before
  `pr.Stop`/`pr.EnsureRunning`) but is the weaker statement; a pointer from
  there to `network.go` already exists.

- **[informational]** BUG-030 discipline survives the refactor:
  `lockedProject.Stop` -> `stopMarked` (`orchestrator.go:1110-1126`) marks
  before stopping and lifts the marker only via `unmarkIfStillRunning`, and a
  successful `ensureRunningLocked` clears it at `:451`. So the two
  post-failure branches that leave the project down (persist failure at
  `network.go:184-193`, start failure at `:195-201`) leave it down **and marked**,
  which is what keeps a terminal auto-reconnect from resurrecting it into the
  mode the request was trying to leave.

- **[informational]** `internal/orchestrator/orchestrator.go:182-188` — the
  nil-`Lan` default has no test. `fakeMode.EnsureRunning`
  (`network_test.go:321-323`) calls `opts.Lan()` directly and would panic on
  nil rather than exercise `RunOpts.lan()`. The default is the fail-closed one
  and no caller passes nil today; a three-line orchestrator-package test
  (`RunOpts{}.lan()` returns `(false, nil)`) would keep the next caller that
  forgets the field from silently depending on an untested branch. New file or
  an append, per `.agents/rules/writing-files.md`.

- **[informational]** `internal/httpapi/network_test.go:275-288` —
  `fakeMode`'s counters (`locked`, `maxLocked`, `stopCalls`, `ensureCalls`,
  `mismatchCalls`, `ensureLan`) are mutated without synchronization, and
  `WithProject` does not actually lock. That is fine today —
  `TestNetworkConcurrentPutsStayConsistent` (`:220-241`) is the only
  concurrent test and it runs against the **real** orchestrator, not
  `fakeMode` — but the fake's `maxLocked` field advertises a mutual-exclusion
  assertion the fake cannot make. Note also that that test's only assertion is
  "the GET agrees with the record", which is exactly the property finding 1
  preserves while lying in the *reply*; it therefore cannot catch finding 1.
  A test for finding 1 needs two PUTs where the second's `getProjectFor`
  precedes the first's persist, asserting that a `200` whose body says
  `lanAccess:false` leaves a record saying `false`.

## Checked

- **Secrets** — none. Four `.go` files; no `.env*`, `*.pem`, `*.key`,
  `*.tfvars`, `*.tfstate` or credential JSON in the diff. New log lines carry
  project id, booleans, network/bridge names and error strings; the single
  email (`"by", u.Email`, `network.go:172`, `:204`) is the pre-existing audit
  line and is the operator's own identity.
- **Injection** — no new parsing boundary. External input is
  `{"lanAccess": bool}` decoded into a `*bool` with a nil check (400
  otherwise); the project id comes from the router and is resolved through the
  caller's own store. `ModeMismatch`/`currentBridge` receive only
  server-derived names (`config.NetworkName`, `config.BridgeNameFor`). Nothing
  in range reaches a shell, argv, SQL, template or filesystem path;
  `lanOf` passes an id that already indexed a map.
- **Authz/authn** — unchanged and correct; check 5 above has the ordering.
  Nothing hand-rolled: no auth, session, token or hashing code in range.
- **Crypto** — nothing in range. `BridgeNameFor`'s sha1 is a name derivation,
  untouched by this diff.
- **Dependencies** — none changed. `go.mod`, `go.sum`, the ingress module's
  pair, `web/package.json` and `web/package-lock.json` are all absent from the
  diff (`git diff --numstat` lists four `.go` files and nothing else), so the
  module graph is byte-identical to the tree round 1 read.
  `govulncheck`/`npm audit` — see "Not examined".
- **GCP surface** — not applicable; self-hosted, `.agents/docs/deploy/gcp.md`
  records it as out of scope. No IAM binding, service account,
  `--allow-unauthenticated`, Secret Manager reference or key file in range.
- **Static checks run here** — `go vet ./internal/httpapi/ ./internal/orchestrator/`
  clean; `go build ./...` clean; `go test ./internal/httpapi/ ./internal/orchestrator/
  ./internal/project/` all `ok`; `go test -race -count=1` on the two
  changed packages both `ok` (exit 0) — but note the only concurrent test
  drives the REAL orchestrator, so the detector's silence says nothing about
  `fakeMode`, and it exercises no PUT-vs-PUT ordering, so it cannot see
  finding 1.

## Not examined

- **Live behaviour.** No integration run, no Kata, no browser e2e, no compose
  deploy. Every row of the table above is reasoned from source plus the unit
  tests. No exploit was written and none should be inferred from this file.
- **`go test ./...` in full, and the vendored ingress module's suite**
  (`deploy/caddy/ingress`) — not run; out of the diff's blast radius.
- **`govulncheck` / `npm audit`** — not re-run. The dependency graph is
  unchanged by this diff, and the authoritative number for a release gate is
  `bash deploy/vulncheck.sh` (pinned builder image, both Go modules), which
  belongs to `/preflight`, not to a four-file source review.
- **The verifier's artifact**
  (`.agents/docs/testing/2026-09-10-verify-network-record-first.md`) — not
  read, deliberately, so this review does not inherit its conclusions.
- **`deploy/firewall/`** — untouched by this diff; its `-i sdb-+` rules are
  load-bearing for finding 1 but were read in an earlier round, not
  re-reviewed here.
- **`web/`** — the UI's handling of `rebuilt`, of the reply body's `lanAccess`
  (which finding 1 makes untrustworthy), and of the 500 texts was not diffed;
  `web/dist/client` is not in this range.
- **`EnsureSetup`'s network** (`orchestrator.go:676`) — the setup VM has its
  own lock key and no project record, so it is outside `RunOpts.Lan`'s reach;
  I confirmed it does not call `ensureRunningLocked` and looked no further.

## Response (main, 2026-09-10, round 3)

- **Finding 1 (medium) — fixed.** The closure re-reads the record with
  `sp.projects.Get` as its first act under the lock (`rec`), and the no-op
  and drift decisions compare against that, never the pre-lock copy.
  `TestNetworkNoOpDecisionReadsTheRecordUnderTheLock` plants a record change
  at lock time via the fake's `onLock` and asserts on the record; deleting
  the re-read makes it fail.
- **Finding 2 (low) — rebutted as pre-existing and uniform.** Every handler
  in this API returns the wrapped Docker/store error to the project's own
  owner or an admin (`startProject` returns `err` verbatim); v1.25.0's
  version of this handler did the same. Changing that is an API-wide
  decision, not this handler's.
- **Finding 3 (low/low) — accepted as latent.** The bodies are a few dozen
  bytes into a buffered writer; noted in TODO with the fix (collect the
  reply inside the closure, write after).

## Round 4 (narrow) — 2026-09-10

Scope: ONE change, re-checked against round-3 finding 1 only. Four directed
questions; nothing else in the diff was re-reviewed.

### The target moved DURING this review

`network.go` was edited between the first read and the second, in this
session, while I was reading it:

| moment | `network.go` blob | closure's first statement |
|---|---|---|
| first read (10:02) | `f046ddd` | `p, err := sp.projects.Get(p.ID)` — **shadowed** the outer `p` |
| re-pin (10:05) | `2d9ff8c` | `rec, err := sp.projects.Get(p.ID)` — the reviewed form |

Everything below is written against `2d9ff8c` and nothing else. The
dispatched diff `.agents/scratch/network-record-first.diff` (blob `520a61b`)
was byte-identical to `git diff` at 10:02 and is **STALE** at 10:05: the tree
has since grown four documentation files (`.agents/docs/bugs/INDEX.md`,
`specs/styrman/api.md`, `specs/styrman/guide.md`, `TODO.md`) and the
`p`→`rec` rename. No source file other than `network.go` moved
(`network_test.go` `14b523c`, `server.go` `614408e`,
`orchestrator.go` `640817b` — all unchanged across the two pins). Re-pin
before the next round rather than trusting the scratch diff.

Integrity of the re-pinned pair: `file` says `Unicode text, UTF-8` for both;
`git diff --numstat` gives `131/101` and `326/67`, no `-` column (BUG-007
clean). `network_test.go` goes 10 (`HEAD`) to **18** `func Test`, two renamed
(`TestNetworkFailedRevocationRollsBackTheRecord`→`...ChangesNothing`,
`TestNetworkStatusErrorOnRevocationRollsBack`→`TestNetworkStatusErrorChangesNothing`),
none dropped (BUG-005 clean). `git status --porcelain` shows the four `.go` paths
plus the four docs above plus untracked review artifacts — no unexplained
` D` (BUG-006 clean). `HEAD` is now `7def9f0` (was `0378825`); that commit is
docs-only, so the base is unchanged for these files.

### Verdict on the narrow re-check: the finding is CLOSED

Round-3 finding 1 is fixed as described, and the fix is the shorter one it
asked for: a variable removed from the decision, not a guard added. No new
finding at medium or above. Three low/informational notes, all about the
edges the fix newly touches, none of them a fail-open.

### (1) Two concurrent PUTs — closed, both orders, running and stopped

Closed, **completely**, and the reason is structural rather than
probabilistic. Three facts, each checked in source:

1. **`LanAccess` has exactly one writer.** `grep -rn SetLanAccess --include=*.go`
   (non-test) returns one call site: `network.go:126`, inside `persist`,
   which the closure calls only while `WithProject` holds the project lock.
   `project.Store.SetLanAccess` (`internal/project/store.go:223-241`) is the
   only mutator of the field.
2. **The read and that write are now in one critical section.**
   `WithProject` (`orchestrator.go:219-224`) takes `o.lockProject(key(user,
   project))` and holds it for the whole callback; `rec, err :=
   sp.projects.Get(p.ID)` (`network.go:134`) is the callback's first
   statement and `persist()` (`:169`, `:194`) its only write. Read →
   decide → write is therefore atomic with respect to any other PUT.
3. **Both PUTs land on the same lock and the same store.** The lock key is
   `key(u.Username, p.ID)`, and `getProjectFor` (`server.go:610-617`)
   resolves through `s.who(r)`'s own `userSpace` — an admin PUTting another
   user's project gets `ErrNotFound`/404, never that project's lock under a
   different key. `space()` (`server.go:263-296`) only ever *inserts* into
   `s.spaces` (no `delete(s.spaces` anywhere), so one `*project.Store` per
   user for the process's life — a second, divergent in-memory copy of the
   record cannot exist.

The four orders round 3 asked about, on a **stopped** project (the branch
where `wrongBridge` is forced `false` at `:153` and the live bridge is
deliberately never consulted, so the record is the only witness):

| order | before | E/D sequence | record after | replies |
|---|---|---|---|---|
| enable → disable | sandbox | E: `rec=false`, `on=true` → persist `true`, 200 · D: `rec=true`, `on=false` → **not** a no-op → persist `false`, 200 | **sandbox** | both truthful |
| disable → enable | LAN | D: `rec=true`, `on=false` → persist `false`, 200 · E: `rec=false`, `on=true` → persist `true`, 200 | **LAN** | both truthful |
| enable → disable | LAN | E: `rec=true`,`on=true` → no-op 200 · D: `rec=true` → persist `false` | **sandbox** | truthful |
| disable → enable | sandbox | D: no-op 200 · E: persist `true` | **LAN** | truthful |

Row 1 is exactly round-3 finding 1's scenario and it now ends with the
record at sandbox — the dangerous direction (a silently dropped revocation
that a later start would apply as `lan-<hash>`) is gone. Row 2, the reverse,
was already safe (it under-claimed) and stays safe: last writer wins and its
reply matches what it wrote. On a **running** project the same table holds
and is over-determined — `ModeMismatch` (`orchestrator.go:235-252`) consults
the live bridge, so even a hypothetical stale record could not produce a
wrong no-op there.

The residual is not PUT-vs-PUT: `deleteProject` removes the record under
`netMu` *without* the project lock (`server.go:1016-1019`), so a `Remove` can
still land between the closure's `Get` and its `persist`. `SetLanAccess` then
returns `ErrNotFound` and the handler answers 500 "already on the requested
bridge but the setting could not be saved". Wrong sentence, right direction —
the record is gone, so nothing claims LAN. Noted below, not a finding.

### (2) Every `p.` inside the closure, classified

`rec` is used at exactly two places: `:164` (the no-op decision) and `:177`
(the log's `recordSays`). The outer `p` — `getProjectFor`'s pre-lock copy —
survives at ten sites. Each one:

| line | use | class | stale-safe? |
|---|---|---|---|
| `:128` | `WithProject(u.Username, p.ID, …)` | identity (lock key) | yes — `ID` is immutable; it is the key the record was fetched by |
| `:134` | `sp.projects.Get(p.ID)` | identity (lookup key) | yes — same |
| `:177` | `"project", p.ID` | log | yes |
| `:182` | `"project", p.ID` | log | yes |
| `:183` | `"projectName", p.Name` | log (display) | cosmetic only — see below |
| `:200` | `"project", p.ID` | log | yes |
| `:206` | `Name: p.Name` | **state-changing** (`RunOpts`) | cosmetic only |
| `:206` | `Dir: p.Dir` | **state-changing** (`RunOpts`, workspace bind source) | yes — `Dir` is immutable |
| `:206` | `Lan: lanOf(sp, p.ID)` | identity only | yes — the mode itself is re-read under the lock inside `ensureRunningLocked` (`orchestrator.go:266`) |
| `:214-215`, `:218` | logs, `EnsureSelfAttached(u.Username, p.ID)` | identity | yes |

**No decision compares a pre-lock value.** The only two pre-lock values that
reach a state-changing call are `RunOpts.Name` and `RunOpts.Dir`:

- `Dir` is immutable — `Store.Rename` (`store.go:166-184`) copies the record
  and assigns `p.Name` only; nothing anywhere assigns `Dir` after `Add`. So
  the stale copy is bit-identical to a fresh read, and the workspace bind
  source cannot be redirected by a concurrent request. This is the one that
  would have mattered (it is a mount source), and it is safe by
  immutability, not by luck.
- `Name` is `RunOpts`'s "friendly name (label + hostname)"
  (`orchestrator.go:164`) and reaches `guestHostname(opts.Name)` (`:417`) and
  `config.LabelName` (`:430`), which `config.go:30` marks "informational" and
  whose only reader tests it for non-emptiness (`orchestrator.go:811`). A
  `PATCH` rename racing this PUT therefore leaves the rebuilt container with
  the previous hostname/label until its next rebuild. Cosmetic; no
  authorization, path, firewall rule or lookup depends on it.

Two more pre-lock values, for completeness: `on` (the request body,
correctly not re-read) and `u.Admin`, used at `:165`/`:184`/`:220` for the
`canEnable` reply field. `u` is the caller's identity, not project state, and
the enable gate `requireAdmin` already ran at `:108`; a demotion racing the
request would leave a UI hint one request stale. Pre-existing, unchanged by
this diff, not worth a finding.

### (3) The new 404 branch — correct, and strictly more honest than before

`rec, err := sp.projects.Get(p.ID)` → `writeErr(w, 404, err)` (`:134-138`).
Checked three ways:

- **The mapping is exact.** `Store.Get` (`store.go:244-253`) is a map lookup
  under the store mutex with no I/O; its only possible error is
  `ErrNotFound` (`store.go:47`, `errors.New("project not found")`). So 404
  cannot mask an I/O or permission failure, and the body carries no path,
  daemon socket or internal detail — unlike the six 500s in low finding 2.
- **The state it describes is real.** The one way to reach it is
  `deleteProject`'s `sp.projects.Remove` (`server.go:1018`) landing between
  the PUT's `deletingState` check (`:113`) and its acquisition of the project
  lock. `deleteProject` takes `netMu` only *after* `s.orch.Destroy` has
  returned (`server.go:961` → `:1016`), and `Destroy` is the thing that takes
  the project lock (`orchestrator.go:1327`) — so yes, the removal genuinely
  is not ordered behind this handler's lock, and by the time the closure
  runs the project really is gone. 404 is the truth, and it matches the
  pre-lock 404 at `:102` for the same condition.
- **It replaces something worse.** Without the re-read, that same interleaving
  produced either a `200 {"lanAccess":…}` for a project that no longer exists
  (the no-op branch, which reads only the stale copy) or a 500 from
  `SetLanAccess`'s `ErrNotFound` wearing the "already on the requested
  bridge" sentence. The 404 is the branch that describes the state.

No resurrection is introduced. The only `EnsureRunning` in the closure
(`:205`) is behind `wrongBridge`, which requires `st.State == "running"`; if
`Destroy` has already run, `Status` reports absent and the closure takes the
persist-only path. If instead the PUT wins the lock first, `Destroy` simply
runs after it and removes whatever was rebuilt.

One narrow imprecision, informational: `Store.Remove` rolls the record back
into the map if its `save()` fails (`store.go:262-268`). A `Get` landing
inside that window answers 404 for a project that then reappears. Nothing was
written and the mode is not misdescribed — only existence is, transiently,
and the client's retry succeeds. Not worth code.

### (4) Lock order — no new edge at all

The `Get` adds `projLock → storeMu`. That edge **already existed** before this
change, twice over: `persist` takes `projLock → netMu → storeMu`
(`network.go:123-127`), and `lanOf` (`server.go:93-101`) is evaluated as
`opts.lan()` inside `ensureRunningLocked` with the project lock held
(`orchestrator.go:266`), which is `projLock → storeMu` exactly. So the new
line is a second traversal of an existing edge, not a new one.

The full graph after the change: `projLock → netMu → storeMu`,
`projLock → storeMu`, `netMu → storeMu` (`deleteProject`). `storeMu` is a
**leaf** — every `project.Store` method (`Add`/`Rename`/`SetPortShares`/
`SetLanAccess`/`Get`/`Remove`/`List`) takes only `s.mu` and calls only
`save()`, which does file I/O and acquires nothing. No path takes `storeMu`
then anything, and round 3's finding of no `netMu → projLock` edge is
unaffected (nothing in this change touches `netMu`'s two sites). Acyclic;
no deadlock.

Cost, not correctness: `storeMu` is held across `save()`'s disk write in
`SetLanAccess`/`Remove`/`Add`, so the new `Get` can block a project-lock
holder for the duration of another project's JSON write. `persist` already
did exactly this from inside the same lock, so the hold's worst case is
unchanged in kind. Round 3's note on the three-minute hold stands as written.

### The test

`TestNetworkNoOpDecisionReadsTheRecordUnderTheLock`
(`network_test.go:637-663`) is load-bearing: `fakeMode.onLock`
(`:286`, `:289-292`) fires inside `WithProject` before `fn`, flipping the
record to LAN, and the assertion reads the record (`lanAccessOf`), not the
reply. Delete `:134` and the handler compares the pre-lock `false` against
`on=false`, takes the no-op branch, and the record stays `true` — the test
fails. It covers row 1 of the table above (the dangerous direction) on a
stopped project, which is the exact shape of finding 1.

Two gaps, informational: it does not cover the reverse order (row 2, the
under-claiming direction), and `fakeMode.WithProject` still does not actually
lock, so the fake models the *ordering* the lock produces rather than the
lock — a regression that removed `WithProject`'s `l.Lock()` in the
orchestrator would not be caught here. Round 3's note on `fakeMode`'s
unsynchronized counters and its unusable `maxLocked` field is unchanged.

### Findings (round 4)

- **[low / confidence: high]** `internal/httpapi/network.go:169`, `:194` —
  **a 500 that names the wrong cause when the project was deleted mid-PUT.**
  `deleteProject` removes the record under `netMu` without the project lock
  (`server.go:1016-1019`), so a `Remove` between the closure's `Get` and its
  `persist` makes `SetLanAccess` return `ErrNotFound`, which the handler
  reports as "the project is already on the requested bridge but the setting
  could not be saved" (or, past the stop, "start it to continue in the
  previous mode" for a project that can never start again). Not a fail-open —
  no record means nothing claims LAN, and the 409 at `:113` catches every
  delete that was already flagged when the request arrived. It is a
  **message** defect: the operator is told to retry or restart something that
  no longer exists. **Remediation:** in both `persist` failure branches,
  `errors.Is(err, project.ErrNotFound)` → 404 with the same sentence `:136`
  uses. Three lines, and it makes the closure's three not-found exits agree.
- **[low / confidence: medium]** `internal/httpapi/network.go:206` —
  **`RunOpts.Name` is still the pre-lock copy on the one path that rebuilds
  the container.** A `PATCH` rename committing between `getProjectFor` and
  the rebuild gives the new container the previous hostname
  (`guestHostname(opts.Name)`, `orchestrator.go:417`) and `styrman.name`
  label (`:430`), and nothing corrects it until the next rebuild. Purely
  cosmetic today — `config.go:30` calls the label informational and its only
  reader tests non-emptiness (`orchestrator.go:811`) — which is why this is
  low and not medium. It is flagged because it is the *last* pre-lock value
  feeding a state-changing call, and BUG-032's prevention rule ("when a
  decision moves under a lock, (re)read every one of its inputs under that
  same lock") reads as if it were already handled. **Remediation, one
  character each:** `Name: rec.Name, Dir: rec.Dir` at `:206` — `rec` is
  already in scope and is the fresher record — and `rec.Name` in the two log
  lines. Then the outer `p` is used for `p.ID` only, which is what makes the
  invariant checkable by grep.
- **[informational]** The comment at `:129-133` says "the copy `getProjectFor`
  took before it" and cites round 3, which is exactly right, but the outer
  `p` remains in scope for the whole closure with nine live uses. The
  previous revision of this same fix (blob `f046ddd`, an hour old) *shadowed*
  `p` instead, which silently made `:183`/`:206` fresh; the current revision
  makes them stale again. Both were defensible; neither is greppable. Naming
  the fields off `rec` everywhere (previous finding) removes the question.
- **[informational]** Round-3 findings 2 (raw Docker/store errors in 500
  bodies) and 3 (reply written under the project lock) are unchanged in this
  revision and were rebutted/accepted in the Response above. Nothing in
  round 4 alters either assessment. The new `:136` 404 does **not** join
  finding 2's set: `ErrNotFound`'s text is a fixed string with no path in it.

### Checked (round 4, narrow)

- **Secrets** — none. The one new line adds no logging; the new 404's body is
  the constant `"project not found"`.
- **Injection** — no new parsing boundary. `Get`'s argument is `p.ID`, a
  string that already indexed the same map in `getProjectFor`.
- **Authz/authn** — unchanged. The re-read is *after* the 404 / admin / 409
  gates (`:100-116`) and adds no authorization decision; it cannot reorder
  them because it lives inside the closure they all precede. Nothing
  hand-rolled in range.
- **Crypto** — nothing in range.
- **Dependencies** — none changed; the change is one statement using an
  existing method.
- **GCP surface** — not applicable (self-hosted).
- **Static checks** — `go vet ./internal/httpapi/ ./internal/orchestrator/`,
  `go build ./...`, and `go test -race -count=1` on
  `./internal/httpapi/ ./internal/orchestrator/ ./internal/project/`: see the
  line appended below this section. `govulncheck`/`npm audit` not re-run —
  unchanged dependency graph, and `bash deploy/vulncheck.sh` is
  `/preflight`'s job.

### Not examined (round 4)

- Everything outside the four questions. The rest of the diff
  (`orchestrator.go`, `server.go`, `network_test.go`'s other 17 tests) was
  reviewed in round 3 against blobs `40b6549`/`614408e`/`28ac75a`;
  `orchestrator.go` has since moved to `640817b` at the same `129/24`
  numstat, and I did **not** re-diff it — round 3's conclusions about it are
  inherited, not re-derived. `server.go` is byte-identical to round 3.
- The four documentation files the tree grew mid-review
  (`INDEX.md`, `api.md`, `guide.md`, `TODO.md`) — not reviewed. `api.md`'s new
  `PUT` row describes the handler's behaviour and is worth a documentation
  read, but it is not source.
- Live behaviour: no integration run, no Kata, no e2e. Every row above is
  reasoned from source plus the unit tests. No exploit was written and none
  should be inferred.

**Static-check result, this pin (`network.go` `2d9ff8c`, `network_test.go`
`14b523c`):** `go vet` on both changed packages exit 0; `go build ./...`
exit 0; `go test -race -count=1 ./internal/httpapi/ ./internal/orchestrator/
./internal/project/` all `ok` (1.652s / 1.027s / 1.013s). As in round 3, the
race detector's silence says nothing about the PUT-vs-PUT ordering above:
no test drives two concurrent PUTs through `fakeMode`, and `fakeMode`'s
`WithProject` does not lock. Blobs re-hashed after the run and unchanged.

## Response (main, 2026-09-10, round 4)

- **Both lows fixed.** A persist error that is `project.ErrNotFound` is a
  404 on either branch (`gone` helper), and the rebuild's `RunOpts` take
  `Name`/`Dir` from the record read under the lock (`rec`). `go test ./...
  -race` green after both.
