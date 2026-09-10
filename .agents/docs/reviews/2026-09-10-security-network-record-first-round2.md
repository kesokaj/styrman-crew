---
owner: security
date: 2026-09-10
---
# Security review — network mode, record-first (round 2)   (2026-09-10)

Range reviewed: `7def9f0..worktree` — the CURRENT uncommitted working tree
(`git diff`), dispatched as `.agents/scratch/network-record-first.diff`.

## Blobs this review was written against

Pinned with `git hash-object` at the start of the read. Re-diff whatever gets
tagged against these; the tree moved under three consecutive reviews.

| file | blob | numstat |
|---|---|---|
| `internal/httpapi/network.go` | `97b9f91` | 96/80 |
| `internal/httpapi/network_test.go` | `e52c6fa` | 251/65 |
| `internal/httpapi/server.go` | `b5bd390` | 2/2 |
| `internal/orchestrator/orchestrator.go` | `330f658` | 103/14 |
| `.agents/scratch/network-record-first.diff` | `58d6dc5` | (the dispatched diff) |

`HEAD` = `7def9f09b64b7ace03fb28f0569b03346c7f5b64`. Integrity: all four are
text (no `-` column in numstat, BUG-007 clean); `network_test.go` goes 10 to
**16** `func Test` with none dropped in substance (BUG-005 clean);
`git status --porcelain` shows only the four dispatched paths plus three
untracked review/testing artifacts, no unexplained ` D` (BUG-006 clean).

## Verdict: FINDINGS

**Round 1 finding 2 is fixed for the branch it was reported on and reopened,
narrower, on the branch beside it.** `Rebuild` holding the project lock across
stop, persist and start genuinely closes the interleaving round 1 named: a
stale start parked on that lock now resumes onto a container that is already
up on the new bridge, and `ensureRunningLocked`'s mode branch is stopped-only
so it leaves it alone. That path went from **zero-fault** in v1.25.0 to
**two-fault**. But `Rebuild` is only entered when `wrongBridge` is true. On
every other path the record write happens with **no project lock at all**,
after a `Status` read that also takes no lock — so a revocation on a STOPPED
project can be overtaken by a concurrent `POST /start` carrying the old
`RunOpts.Lan`, ending with the container on `lan-` and the record saying
sandbox, at **zero faults**. That interleaving is **not** a subset of what
v1.25.0 allowed: v1.25.0 persisted BEFORE it read `Status`, so a start landing
in that window was seen as running and rebuilt — an incidental self-heal this
order removes.

Round 1 finding 1 is **partly** fixed: the missing-network sub-case is now a
mismatch, the not-attached-to-the-project-network sub-case is not.

Lock ordering is **sound**: `netMu` is only ever taken as the inner lock
(`projLock -> netMu` in `Rebuild`'s persist; `netMu` alone, un-nested, in
`deleteProject`). No path takes `netMu` and then a project lock, so there is no
cycle. The invariant is undocumented, which is the third finding.

Authz ordering is **unchanged and correct** (404 before authz, admin-gate on
enable only, delete-in-flight 409 first).

## Per-branch adjudication — PUT /api/projects/{id}/network

"In force" = the bridge prefix the container's veth is actually on; a stopped
or absent container has nothing in force. The record write is the single
`SetLanAccess` inside `persist` (`network.go:145-149`), called either directly
(`:151`) or by `Rebuild` (`orchestrator.go:220`).

| # | Branch | Record after | In force | Container | Verdict |
|---|---|---|---|---|---|
| 0 | **Status error** (`network.go:113-121`) | unchanged | whatever it was | unknown, presumed up | **PASS.** Nothing written, nothing stopped; the 500 says "NOT changed", true on both counts. |
| 1 | **Absent** (`:127` false) | new mode | nothing | none | **PASS.** `ModeMismatch` never called; the next `create()` builds the bridge from the record. |
| 2 | **Stopped** (`:127` false) | new mode | nothing | down | **PASS on the branch, racy beside it — finding 2.** `bridgeModeMismatch` heals it at the next start; but the persist here holds no project lock. |
| 3 | **ModeMismatch error** (`:129-135`) | unchanged | old mode | up | **PASS.** "I cannot tell" does not read as "nothing to do" and does not stop a container it may not need to stop. |
| 4 | **True no-op** (`:137`, record == request AND bridge agrees) | unchanged | request | up | **PASS.** |
| 5 | **Record behind a correct bridge** (`:137` false via `p.LanAccess != on`) | corrected to the bridge | request | up, untouched | **PASS on the branch.** Same missing lock as branch 2. |
| 6 | **Persist error on branch 5** (`:151-163`) | unchanged | request | up | **PASS, disclosed.** Pre-existing divergence, unchanged by the request; WARN plus a 500 that says the record keeps reporting the previous mode. |
| 7a | **Rebuild step "stop" fails** (`orchestrator.go:217-219` -> `network.go:179-183`) | unchanged | old mode | **up**, old bridge | **PASS.** `stopped` marker set before the stop and lifted again by `unmarkIfStillRunning` only when the container is provably still up (BUG-030 discipline preserved). Record equals force. |
| 7b | **Rebuild step "persist" fails** (`orchestrator.go:221-222` -> `network.go:184-192`) | old mode | nothing | **down**, marker SET | **PASS.** Over-claims against a container that has nothing — the safe direction. The marker keeps terminal reconnects from resurrecting it; the next explicit start applies the OLD record. The 500 says exactly that. |
| 7c | **Rebuild step "start" fails** (`orchestrator.go:224-226` -> `network.go:193-197`) | new mode | nothing | **down or removed**, marker SET | **PASS on the branch, the racy one — finding 2.** Record ahead of a container with nothing is safe in isolation; it is the one post-failure state a stale concurrent start can turn into a fail-open. |
| 7d | **Rebuild succeeds** (`network.go:200-207`) | new mode | new mode | up, new bridge | **PASS**, and `rebuilt:true` is now honest — round 1's "claims a rebuild that did not happen" is gone, because a stale start can no longer no-op inside the window. |
| 8 | **Running container NOT attached to its project network, network itself present and correctly named** | new mode | whatever bridge it IS on | up, untouched | **FAIL-OPEN — finding 1, residual.** `ModeMismatch` inspects the NETWORK, never the container. |
| 9 | **Running container, project network MISSING** (`orchestrator.go:876-885`) | new mode | none of ours | stopped and rebuilt | **PASS — round 1 finding 1's fix.** WARN plus rebuild instead of the old `(false, nil)`. |
| 10 | **Concurrent `POST /start` racing the PUT** | new mode (sandbox) | possibly LAN | up, on `lan-` | **FAIL-OPEN — finding 2.** Adjudicated per-interleaving below. |

Neither fail-open is cross-tenant: `getProjectFor` resolves through the
caller's own `userSpace`, so the project and the racer are the same principal.
What breaks is the operator's belief about which projects the host firewall's
`-i sdb-+` drops actually cover.

## The concurrent-start race, interleaving by interleaving

Timeline symbols: `t_r` the racing start's record read (`getProjectFor`,
`server.go:1044`, or `ensureFor`, `server.go:234` — both snapshot
`p.LanAccess` into `RunOpts.Lan`); `t_e` its `EnsureRunning` lock acquisition;
`t_s` the PUT's `Status` (`network.go:112`, **takes no lock** —
`orchestrator.go:147`); `t_p` the persist; `[t_lock, t_unlock]` `Rebuild`'s
single hold. Direction throughout is the dangerous one: revoke LAN, record to
sandbox, `RunOpts.Lan == true` in the racer.

**A. `wrongBridge` true (the Rebuild path) — closed as claimed.**

| # | Interleaving | Outcome | Faults |
|---|---|---|---|
| A1 | `t_e < t_lock` — the start wins the lock | Start no-ops on a running container (or starts it on `lan-`); Rebuild then stops, persists and rebuilds on `sdb-`. Consistent. | — |
| A2 | `t_e > t_unlock`, Rebuild succeeded | Container is RUNNING on `sdb-`; `ensureRunningLocked`'s mode branch is gated on `!insp.State.Running` (`orchestrator.go:283-284`), so the stale `Lan:true` is never consulted. No-op. **The Response's claim verified.** | — |
| A3 | `t_e > t_unlock`, Rebuild failed at **stop** | Container still up on the old bridge, record unchanged. Start no-ops. Consistent. | 1 |
| A4 | `t_e > t_unlock`, Rebuild failed at **persist** | Record still says LAN, container down; the stale start brings it up on `lan-`. Record equals force — the revocation silently did not happen, but the 500 said so. | 1 |
| A5 | `t_e > t_unlock`, Rebuild failed at **start**, container removed or stopped, network already `sdb-` | Stale start with `Lan:true`: `create()` sees `sdb- != lan-<hash>` (`orchestrator.go:952-969`), REBUILDS the network as `lan-` and starts the container on it. Record says sandbox. **FAIL-OPEN.** | **2** (a failed rebuild-start plus a straddling read) |
| A6 | same, but the failure was inside `waitReady` with the container up on `sdb-` | Stale start sees running, no-ops. Safe. | 1 |

A5 is the only fail-open on this path, it needs two faults, and in v1.25.0 the
same end state was reachable with **zero**: v1.25.0 called `Stop` and
`EnsureRunning` as two separate orchestrator calls
(`git show v1.25.0:internal/httpapi/network.go`), leaving the project lock free
between them for exactly this start. **On this path the residual set is a
strict subset of v1.25.0's.**

**B. `wrongBridge` false (stopped, absent, or bridge-already-matches) — NOT a subset.**

Here `persist()` at `network.go:151` runs with no project lock, and the
`st.State == "running"` decision behind it came from a lock-free `Status`.

| # | Interleaving | Outcome | Faults |
|---|---|---|---|
| B1 | `t_s < t_r < t_e < t_p` — the whole racing start fits between the PUT's `Status` and its persist | PUT saw "stopped", so it takes the record-only path. The start reads the still-old record, takes the lock, `bridgeModeMismatch(lan-, want lan)` is false, `ContainerStart` on `lan-`. The PUT then writes sandbox over it and answers `200 {"rebuilt":false}`. **FAIL-OPEN.** | **0** |
| B2 | `t_r < t_s`, `t_e > t_p` — the start straddles the whole handler | Same end state. | **0** |
| B3 | the racing start holds the lock at `t_s` (inspect done, `ContainerStart` pending) | `Status` takes no lock, so it reports "stopped" for a container that is milliseconds from running. Same end state. | **0** |
| B4 | enable direction (`on == true`) racing a stale `Lan:false` start | Record says LAN, container comes up on `sdb-`. Record over-claims; safe direction. | 0 |

**B1 is not reachable in v1.25.0.** v1.25.0 persisted first and read `Status`
second, so (a) a start whose record read happened after the PUT began was not
stale at all, and (b) a start that completed before the `Status` read was seen
as `running` and was stopped and rebuilt — an accidental self-heal. v1.25.0's
residual on this path required `t_r < t_p < t_s < t_e`, i.e. the start had to
straddle the entire handler (B2 only). Putting `Status` before the persist adds
B1 and B3 and deletes the self-heal.

**Net:** the dangerous path (running container on `lan-`, admin revokes) is
much better — zero-fault to two-fault. The stopped/absent path is worse by two
interleavings, both still zero-fault. The overall answer to "is the residual a
subset" is **no**, though the part that got worse was already broken.

## Lock ordering — `netMu` against the project lock

Every `netMu` site (`grep -n netMu`):

- `internal/httpapi/network.go:146-147` — inside `persist`. On the `wrongBridge`
  path this closure is invoked by `Rebuild` (`orchestrator.go:221`) **while the
  project lock is held**: order `projLock -> netMu`. On the other path it is
  called directly at `:151` with no project lock held.
- `internal/httpapi/server.go:1006-1008` — `deleteProject`, around
  `sp.projects.Remove` only. `s.orch.Destroy` (which takes the project lock at
  `orchestrator.go:1311`) is called at `server.go:950` and has **returned**
  before `netMu` is taken; the two holds are sequential, not nested.

No site takes `netMu` first and then a project lock, so there is no AB/BA
cycle and **no deadlock**. `project.Store.SetLanAccess`/`Remove`
(`internal/project/store.go:223`, `:255`) take only the store's own mutex and
never call into the orchestrator, so the innermost level cannot re-enter.

Two liveness notes, neither a deadlock: `netMu` is server-wide and is now
taken *under* a per-project lock across a `save()` (a file write) — a slow or
failing store write stalls every other project's mode change and every
delete's record removal for its duration. And `Rebuild` holds the project lock
for the whole three-minute `lanRebuildTimeout` budget, so a concurrent
`Stop`/`Destroy`/attach on that project blocks for up to that long; both are
bounded and both are the intended trade.

## Findings

- **[medium / confidence: high]** `internal/httpapi/network.go:150-151`
  (persist without the project lock) reached from `:112` (`Status` without the
  project lock) — **the record-only path writes the record outside the lock
  that serializes starts, so a revocation on a stopped project loses a race to
  a concurrent start at zero faults.** Scenario, all authorized, no failure
  anywhere: project P is stopped and its network is still `lan-`; an admin
  PUTs `{"lanAccess":false}`; the handler's `Status` reports "stopped" and it
  commits to the record-only branch. Before the persist lands, the owner
  clicks Start in another tab — `startProject` (`server.go:1044`, `:1059`)
  snapshots the still-old `p.LanAccess == true` into `RunOpts.Lan`, takes the
  project lock, `bridgeModeMismatch("lan-...", true)` is false, and the
  container starts on `lan-`. The PUT then writes sandbox and answers
  `200 {"lanAccess":false,"rebuilt":false}`. The project is on a bridge the
  root firewall's `-i sdb-+` jumps do not match — it reaches the host and the
  LAN — while the record, the badge, the switch and the API all say sandboxed,
  and nothing reconciles them until someone stops and starts it again. Two of
  the three interleavings (B1, B3 above) are new relative to v1.25.0, which
  persisted before it read `Status` and therefore saw and repaired that start.
  **Remediation — this is the second round of findings on the same mechanism
  (`RunOpts.Lan` as a caller-held snapshot of the record), so per AGENTS.md the
  answer is to delete the mechanism rather than defend it:** have
  `ensureRunningLocked` read `LanAccess` from the store itself, under the
  project lock it already holds, and drop `RunOpts.Lan` entirely
  (`orchestrator.go:162-173`). Every start then reconciles against the record
  as of the moment it is authoritative, `Rebuild`'s `opts` argument loses a
  field, and A5, B1, B2 and B3 all close at once — with less code than the
  current arrangement, not more. The narrow alternative (run the record-only
  persist under the project lock too, and re-read `Status` inside it) closes B
  but leaves A5 and keeps `RunOpts.Lan` alive for the next round.

- **[low / confidence: medium]** `internal/orchestrator/orchestrator.go:870-891`
  (`ModeMismatch`) — **round 1 finding 1 is closed for a missing network and
  open for a container attached to the wrong one.** The fix answers "is the
  project's NETWORK named for the wanted mode", never "is this CONTAINER on
  that network". If the project network exists and is correctly named `sdb-`
  but the running container has been attached elsewhere — `docker network
  disconnect` plus `connect` on the host daemon, a container created against
  another network, or a daemon-state anomaly — `currentBridge` returns
  `sdb-<hash>`, `ModeMismatch` returns false, and the handler takes the
  record-only branch (`network.go:150`): it writes `lanAccess:false`, answers
  `200 {"rebuilt":false}`, and leaves a container running on a bridge no
  styrman firewall rule matches. Severity is low and confidence medium only
  because reaching it needs host-level Docker access, which is already
  privileged — the guest's own dockerd is a separate daemon inside the microVM
  and cannot touch the host's networks. Note the same function already treats
  the *other* manual-intervention case (`cur == ""`, a Docker-auto-named
  bridge) as a mismatch at `:940-946`, for exactly the reason that applies
  here. **Remediation:** derive the bridge from the container —
  `ContainerInspect` then `NetworkSettings.Networks[config.NetworkName(project)]`
  — and treat "not attached to the project network" as a mismatch, the way
  `cur == ""` already is.

- **[low / confidence: high]** `internal/httpapi/server.go:118-122` — **the
  `projLock -> netMu` ordering is now load-bearing and is documented nowhere.**
  The comment on `netMu` says what it serializes and that it is "Never held
  across a rebuild", which was the whole invariant when the persist ran at the
  top of the handler. Since this diff the persist is called *from inside*
  `Rebuild` while the project lock is held, so the real invariant is stronger:
  **never acquire a project lock while holding `netMu`.** Nothing violates it
  today (`deleteProject` releases `netMu` before anything else and calls
  `Destroy` beforehand), but the next `netMu` site that wants to stop or
  destroy a project while holding it deadlocks against any in-flight
  `Rebuild`, and the deadlock is a permanent hang of both requests plus every
  later mode change and delete, not a crash. **Remediation:** one sentence on
  the `netMu` field naming the order, and the same on `Rebuild`'s doc comment
  above `persist func() error` saying the callback must not take any lock that
  can outrank the project lock.

- **[low / confidence: high]** `internal/httpapi/network.go:119`, `:133`,
  `:161`, `:183`, `:192`, `:196` — **the 500 bodies still wrap raw Docker and
  store errors** back to an authenticated tenant (daemon socket paths, bridge
  and network names, state-dir paths); the revoke direction needs no admin, so
  a non-admin owner reaches all of them. Carried forward from rounds 1-3 at
  the same severity, with the new `RebuildError` prefix (`stop:`, `persist:`,
  `start:`) now also in the text. Nothing here leaks another tenant's data or
  any secret. **Remediation** unchanged: log the cause, return a fixed
  sentence.

- **[informational]** `internal/orchestrator/orchestrator.go:213-219` —
  `Rebuild` sets the `stopped` marker before the stop and undoes it only via
  `unmarkIfStillRunning`, matching `Stop`'s BUG-030 discipline exactly; a
  successful `ensureRunningLocked` clears it at `:415`. Branches 7b and 7c
  therefore leave the project down AND marked, so terminal auto-reconnects
  (`internal/terminal/bridge.go:123`, `:407`) will not resurrect it into the
  wrong mode. Correct, and worth recording because it is the part that makes
  7b/7c safe rather than merely unlucky.

- **[informational]** `internal/orchestrator/orchestrator.go:233` — the
  `ensureRunningLocked` extraction is a faithful body split: `EnsureRunning`
  is now lock plus delegate, with no behaviour moved across the boundary. The
  pre-ADR-027 workspace-bind recreate at `:245-259` is the one branch in it
  that is NOT stopped-only and can therefore rebuild a RUNNING container from
  a stale `RunOpts.Lan`; it is unreachable straight after a `Rebuild` (every
  `create()` adds the bind), so it does not widen finding 2 today, but it is
  the one place where A2's "a running container is left alone" is not
  literally true.

- **[informational]** `internal/httpapi/server.go:85-89` — the `modeRuntime`
  swap (`Stop`+`EnsureRunning` out, `ModeMismatch`+`Rebuild` in) is not a new
  injection point: `mode` is unexported, assigned once outside tests, and the
  interface only re-cuts the existing orchestrator surface. Passing `persist`
  as a `func() error` through the interface is the only new authority handed
  across, and its sole implementation is the closure two files away.

## Checked

- **Secrets** — none. Four `.go` files; no `.env*`, `*.pem`, `*.key`,
  `*.tfvars`, `*.tfstate` or credential JSON in the diff. New log lines carry
  project id, booleans, bridge/network names and error strings; the one email
  (`"by", u.Email`) is the pre-existing audit line.
- **Injection** — no new parsing boundary. External input is
  `{"lanAccess": bool}` into a `*bool` with a nil check (400 otherwise); the id
  comes from the router and is resolved through the caller's own store.
  `ModeMismatch`/`currentBridge` take server-derived names only
  (`config.NetworkName`, `config.BridgeNameFor`). Nothing reaches a shell,
  argv, SQL, template or filesystem path.
- **Authz/authn** — unchanged and correct. 404-before-authz preserved
  (`network.go:92` before `:100`); enable admin-only, disable never refused;
  `deletingState` 409 first; `CanEnable` still its own field (BUG-019). The new
  `ModeMismatch` and `Rebuild` calls happen strictly after all three gates and
  only against the caller's own project. No route lost middleware; nothing
  hand-rolled.
- **Crypto** — nothing in range. No auth, session, JWT/OIDC, password or
  hashing code touched; `BridgeNameFor`'s sha1 is a name derivation, unchanged.
- **Dependencies** — none changed: `go.mod`, `go.sum`, the ingress module's
  pair, `web/package.json` and `web/package-lock.json` are all absent from the
  diff. `govulncheck`/`npm audit` — see "Not examined".
- **GCP surface** — not applicable; self-hosted, `docs/deploy/gcp.md` records
  it as out of scope. No IAM, service account, Secret Manager or key file in
  range.
- **Static checks run here** — `go vet ./internal/httpapi/ ./internal/orchestrator/`
  clean.

## Not examined

- **Live behaviour.** No integration run, no Kata, no browser e2e, no compose
  deploy. Every branch and interleaving above is reasoned from source plus the
  unit tests. No exploit was written and none should be inferred from this file.
- **`go test ./...` and the ingress module's suite** — not run in this round;
  the verifier's artifact
  (`.agents/docs/testing/2026-09-10-verify-network-record-first.md`) was not
  read either, deliberately, so this review does not inherit its conclusions.
- **`govulncheck` / `npm audit`** — not re-run; the module graph is
  byte-identical to the tree round 1 read. The authoritative number for a gate
  is `bash deploy/vulncheck.sh` (pinned builder, both modules).
- **`deploy/firewall/`** — untouched by this diff; its `-i sdb-+` rules are
  load-bearing for findings 1 and 2 but were read, not re-reviewed.
- **`web/`** — the UI's handling of `rebuilt` and of the new 500 texts was not
  diffed; `web/dist/client` is not in this range.
- **The reachability of finding 1's residual** — I did not attempt to produce a
  running container attached to a non-project network on this host; that is the
  whole of its medium confidence.

## Response (main, 2026-09-10, round 2)

- **Finding 1 (medium) and the subset question — fixed by deleting the
  mechanism, as recommended.** `RunOpts.Lan` is no longer a value but a
  read, `func() (bool, error)`, that `ensureRunningLocked` evaluates under
  the project lock at the moment it decides the bridge name; every start
  path passes `lanOf(sp, id)`, which reads the record. And the whole PUT
  decision — Status, ModeMismatch, stop, persist, start — now runs inside
  `Orchestrator.WithProject`, one hold of the same lock. So a start racing a
  revocation either completes first, and is then seen RUNNING on the wrong
  bridge and rebuilt, or waits on the lock and reads the record the change
  wrote. A5, B1, B2 and B3 close together; `Rebuild`/`RebuildError` and the
  Orchestrator-level `ModeMismatch` are gone. The handler keeps the policy
  (testable through the fake `Project`); the orchestrator provides the
  atomicity.
- **Finding 3 (low) — rebutted.** A container attached to a network other
  than its own while its correctly named network exists needs a manual
  `docker network connect`/`disconnect` on the host by root; the same actor
  can do anything to the container. The design's claim is about what styrman
  itself creates, and the firewall matches on the bridge interface, which is
  a property of the network, so the network is the right thing to ask.
- **Lock-ordering note (low) — documented** at the persist closure in
  `network.go`: netMu is always the inner lock.
