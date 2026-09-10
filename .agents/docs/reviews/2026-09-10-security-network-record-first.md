---
owner: security
date: 2026-09-10
---
# Security review — network mode, record written last   (2026-09-10)

Range reviewed: `0378825..worktree` — `git diff` of
`internal/httpapi/network.go`, `internal/httpapi/network_test.go`,
`internal/httpapi/server.go`, `internal/orchestrator/orchestrator.go`
(dispatched as `.agents/scratch/network-record-first.diff`).

## The tree moved under me again — pin these blobs

Third review in a row to report this. `internal/httpapi/network.go` changed
during the review (`f07d0fc` -> `6345d71`, numstat 77/76 -> 85/75), and the
dispatched diff file itself was rewritten with it (`.agents/scratch/network-record-first.diff`,
md5 `04c72900...`, blob `75e60f3`). The new revision adds a distinct
log+message for the persist failure on the drift branch — an improvement, and
also proof the target is not frozen. **This review is written against:**

| file | blob | numstat |
|---|---|---|
| `internal/httpapi/network.go` | `6345d71` | **85/75** |
| `internal/httpapi/network_test.go` | `a6db07d` | 250/64 (the dispatched diff header says 217/64 — superseded) |
| `internal/httpapi/server.go` | `3df91e9` | 2/1 |
| `internal/orchestrator/orchestrator.go` | `42e6a95` | 10/0 |

Whatever is tagged must be re-diffed against those four hashes
(`deploy/verify-commit.sh`). Integrity: all four are text (no `-` in either
numstat column, BUG-007 clean); `network_test.go` goes 10 -> 16 `func Test`
with two renamed and none dropped in substance (BUG-005 clean);
`git status --porcelain` shows only the four dispatched paths, no unexplained
` D` (BUG-006 clean).

## Verdict: FINDINGS

**The two findings this diff set out to close are genuinely closed, and the
new order is the right one.** Round 3 finding 1 (the record-trusting no-op
short-circuit) is fixed at `network.go:137` — the no-op now requires the LIVE
bridge to agree, so re-issuing the PUT IS the repair, with a test
(`TestNetworkRePutRepairsADriftedRecord`). Round 2 finding 1 (the persist
failure between two writes) is fixed by deleting the second write: the record
is now written once, after the stop, and every pre-write failure changes
nothing. `rollbackMode` is gone, and with it the whole class BUG-028/029/031
lived in.

What is new: **the drift check asks the wrong object.** `ModeMismatch`
inspects the project's NETWORK and answers `(false, nil)` when that network
does not exist — a correct answer for its original caller (a stopped
container, where the next start builds the bridge under the right name) and an
inverted one for the new caller, where `false` means "leave the running
container alone". A running container that is not on its project network is on
no `sdb-` bridge, matches none of the root firewall's `-i sdb-+` drops, and a
revocation now writes `lanAccess:false` over it and answers 200. The old code
could not reach that state: it stopped and rebuilt every running container
unconditionally.

Second, out-of-scope but asked for: round 2's finding 2 (a concurrent start
racing the toggle) is **worse**, not unchanged — see finding 2.

## Per-branch adjudication — PUT /api/projects/{id}/network

"In force" = the bridge prefix the container's veth is actually attached to; a
stopped or absent container has nothing in force. Record write is the single
`SetLanAccess` at `network.go:156`.

| # | Branch | Record after | In force | Container | Verdict |
|---|---|---|---|---|---|
| 0 | **Status error** (`:113-121`) | unchanged | whatever it was | unknown, presumed up | **PASS.** Nothing written, nothing stopped; 500 says "NOT changed", true of both. BUG-029 closed at the source. |
| 1 | **Absent** (`:127` false) | new mode | nothing | none | **PASS.** `ModeMismatch` not called at all; `create()` builds the bridge under the record. Asserted (`mismatchCalls == 0`). |
| 2 | **Stopped** (`:127` false) | new mode | nothing | down | **PASS.** Next `EnsureRunning` heals via `bridgeModeMismatch` (stopped-container branch, `orchestrator.go:229-243`). Note `Status` maps *every* non-`Running` state — `created`, `exited`, `restarting`, `removing` — to "stopped"; only `paused` reports Running=true, and that is the one non-running state where something IS attached, handled as running. Correct. |
| 3 | **Mismatch (network-inspect) error** (`:128-135`) | unchanged | old mode | up | **PASS — BUG-031's shape, closed.** `stop=0 ensure=0` asserted. "I cannot tell" does not read as "nothing to do", and it does not stop a container it may not need to stop. |
| 4 | **True no-op** (`:137`, record == request AND bridge agrees) | unchanged | request | up | **PASS.** One `NetworkInspect` per PUT is the price; see finding 3. |
| 5 | **Record behind a correct bridge** (`:137` false via `p.LanAccess != on`, `wrongBridge` false) | corrected to the bridge | request | up, untouched | **PASS.** Record moves to match reality in both directions; for `on=true` the move is admin-gated at `:100`. |
| 6 | **Drifted record, wrong bridge** (`wrongBridge` true, record already `on`) | rewritten `on` | rebuilt to `on` | stopped then rebuilt | **PASS — this is round 3 finding 1's fix.** The re-PUT is the recovery. |
| 7 | **Stop error** (`:142-148`) | unchanged | old mode | up on the old bridge | **PASS — BUG-028's branch, now structurally impossible to get wrong.** Record ≥ force because the record was never touched. |
| 8 | **Persist error after a successful stop** (`:158-169`) | old mode | nothing | **down** | **PASS.** Over-claims against a container that has nothing; next start applies the old mode; the 500 says exactly that. This is round 2 finding 1, closed. Honest residual: a revocation that lands here silently does not happen — the project comes back on `lan-` — but the record says `lan-` too, so no surface lies. |
| 9 | **Persist error, bridge already matched** (`:170-180`) | unchanged (may under-claim) | request | up | **PASS, disclosed.** Pre-existing divergence, unchanged by the request; the 500 and the WARN both say the record keeps reporting the previous mode. |
| 10 | **EnsureRunning error** (`:189-195`) | new mode | nothing | down | **PASS.** Record ahead of a container with nothing; next start applies it. |
| 11 | **Running container NOT on its project network** (`ModeMismatch` -> NotFound -> `(false,nil)`, `orchestrator.go:819` -> `:838-842`) | **new mode (sandbox)** | **whatever bridge it is on — not `sdb-`** | up, untouched | **FAIL-OPEN — finding 1.** New in this diff: branch 5's "the bridge already agrees" is inferred from an object the container may not be attached to. |
| 12 | **Concurrent start racing the toggle** (not a failure branch) | new mode (sandbox) | **LAN** | up, on `lan-` | **FAIL-OPEN, widened — finding 2.** Round 2 finding 2, not counted as new; the window grew from ~instant to the whole stop, and the reply now asserts `rebuilt:true`. |

Branches 11 and 12 are the two where the record can end up claiming less
access than is in force. Neither is a cross-tenant escalation: `getProjectFor`
resolves through the caller's OWN `userSpace`, so a LAN project is always the
toggling admin's own project and the racer in branch 12 is the same principal.
The property that breaks is the operator's belief about the host firewall, not
another tenant's boundary.

## Findings

- **[medium / confidence: medium]** `internal/orchestrator/orchestrator.go:838-842`
  reached from `internal/httpapi/network.go:127-140` — **`ModeMismatch`'s
  "missing network is not a mismatch" is correct for its old caller and
  inverted for its new one.** `bridgeModeMismatch` returns `(false, nil)` on
  `errdefs.IsNotFound`, and its own comment gives the reason: "recreateMissingNetwork
  owns that case and builds the bridge under the right name anyway" — an
  argument about the *next start*, i.e. about a container that is DOWN. The
  new caller consults it for a container that is UP and reads `false` as "the
  running container is on the right bridge, leave it". A running project
  container that is not attached to `config.NetworkName(project)` — the case
  `recreateMissingNetwork` exists for ("an older buggy sweep removed it"),
  a manual `docker network disconnect`, or any attachment to another bridge —
  is on no `sdb-` interface, so the root firewall's `-i sdb-+` jumps in INPUT
  and DOCKER-USER match none of its traffic and it reaches the host and the
  LAN. The handler then takes branch 5, writes `lanAccess:false`, answers
  `200 {"rebuilt":false}`, and leaves it running. Before this diff a running
  container on a revocation was stopped and rebuilt unconditionally, so this
  state was unreachable. Note the inconsistency inside the same function: the
  other "manual intervention" case, a bridge Docker auto-named (`cur == ""`),
  IS treated as a mismatch and rebuilt, loudly, for exactly the reason that
  applies here — "an unnamed bridge matches no firewall rule, so the project
  is silently unconfined". Confidence is medium only on how a running
  container reaches that state (Docker refuses `network rm` while an endpoint
  is active, so it needs the sweep bug, a disconnect, or a daemon-state
  anomaly), high on the code path. **Remediation:** for the running-container
  decision, derive the bridge from the CONTAINER (`ContainerInspect` ->
  `NetworkSettings.Networks[config.NetworkName(project)]`, then the network's
  `com.docker.network.bridge.name`), and treat "not attached to the project
  network" as a mismatch — rebuild — the same way `cur == ""` already is.
  Keep `bridgeModeMismatch`'s NotFound answer as it is for the start path; the
  two callers are asking different questions and should not share the return.

- **[medium / confidence: high]** `internal/httpapi/network.go:141-157` —
  **moving the record write to after the stop widens round 2's finding 2 from
  a scheduling coincidence into an open window the length of a Kata stop, and
  the racing start is by construction the next holder of the lock it is
  waiting on.** Not counted as a new finding (it is the out-of-scope item),
  but the dispatch asked better/worse/unchanged and the answer is worse.
  Concretely: `startProject` (`server.go:1059-1060`) and the auto-boot path
  (`server.go:234`) both pass `Lan: p.LanAccess` from a snapshot taken at
  `getProjectFor`. Under the old order the record was written in the first
  microseconds of the handler, so only a start that had *already* read it was
  stale. Now the write happens after `Status` + `ModeMismatch` + the whole
  `Stop`; every start that reads the record during that span is stale, and
  `Stop` holds the orchestrator's per-project lock for its duration
  (`orchestrator.go:1012-1015`), so a stale start is parked on precisely the
  lock the toggle is about to release — it runs BEFORE the toggle's own
  `EnsureRunning`, sees a stopped container, finds no mismatch against the
  still-`lan-` network for its stale `Lan:true`, and starts it on `lan-`. The
  toggle's `EnsureRunning(Lan:false)` then finds the container RUNNING, skips
  the mode branch entirely (`orchestrator.go:230`, stopped-only), does
  `waitReady`, returns nil — and the handler answers
  `200 {"lanAccess":false,"rebuilt":true}`, a claim that a rebuild happened
  when none did. That last part is a new inaccuracy this diff adds on top of
  the old race. **Remediation** (either is enough, both are shorter than the
  status quo): have `EnsureRunning` re-read `LanAccess` from the store under
  the project lock instead of trusting `RunOpts.Lan` — every start then
  reconciles and `RunOpts.Lan` can be deleted; or serialize the whole toggle
  per project on the same lock the orchestrator already has. Until then, do
  not report `rebuilt:true` on an `EnsureRunning` that did not rebuild —
  `ModeMismatch` after the ensure would tell you, and it is one call.

- **[low / confidence: high]** `internal/httpapi/network.go:110-136` — **a
  no-op PUT is no longer free: it now costs two calls on the shared host
  Docker daemon**, `ContainerInspect` (via `Status`) and `NetworkInspect` (via
  `ModeMismatch`), where the old handler short-circuited on the in-memory
  record before touching Docker at all. Any authenticated tenant can loop
  `PUT {"lanAccess":false}` against their own project — always authorized,
  tightening is never refused — and each request lands two calls on the daemon
  that every other tenant's terminals, execs and project starts depend on. I
  found no rate limiter in front of the API (`internal/httpapi`), and each
  request also allocates a 3-minute context and a goroutine-per-request with
  no concurrency cap. Availability only, same-host blast radius, no data
  reachable that was not. **Remediation:** cheapest is to keep the record
  short-circuit as a *pre-filter* only when the caller is not asking for a
  repair — but that reintroduces round 3's finding. Better: a small per-user
  rate limit on the mutating project routes, which this route is not the only
  candidate for.

- **[low / confidence: high]** `internal/httpapi/network.go:133`, `:167`,
  `:179` — **the 500 bodies wrap raw Docker and store errors**, which carry
  daemon socket paths, host network names (`sdb-<uuid>`) and state-dir paths
  back to an authenticated tenant; the revoke direction needs no admin, so a
  non-admin owner reaches all three. This is the same class round 3 recorded
  as low and round 2 before it, with one genuinely new source (the
  `NetworkInspect` error at `:133`) and two reworded ones (the store error was
  already exposed as `cannot persist setting: %w`). Nothing here leaks another
  tenant's data or any secret. **Remediation** unchanged: log the cause, return
  a fixed sentence.

- **[informational]** `internal/httpapi/network.go:182-184` — the audit line
  "network mode changed" now sits AFTER the record write instead of before the
  apply, which fixes round 3's informational finding: the branches that change
  nothing (0, 3, 7) no longer emit it. Branch 10 still logs "changed" and then
  fails the restart, but the record genuinely did change there, so the line is
  true. Improvement, recorded so it is not re-raised.

- **[informational]** `internal/httpapi/network.go:158-181` — **the deleted-mid-PUT
  case lost its dedicated branch.** `rollbackMode` had an explicit
  `errors.Is(rb, project.ErrNotFound)` arm that logged INFO ("project deleted
  during the change; nothing to roll back"). Now a project removed between the
  `deletingState` check at `:105` and the write at `:156` makes `SetLanAccess`
  return `ErrNotFound`, which falls into one of the two persist-failure arms
  and produces a WARN plus a 500 that talks about a setting that could not be
  saved. Noise, not a fail-open — `SetLanAccess` cannot resurrect a removed
  record, and the delete path removes the container. The resurrection risk that
  does exist (the toggle's `EnsureRunning` at `:189` recreating a container for
  a project whose delete started after `:105`) is unchanged by this diff, in
  either direction.

- **[informational]** `internal/httpapi/server.go:100`, `:181` — **the
  `modeRuntime` seam is not a new injection point.** `mode` is an unexported
  field, assigned exactly once outside tests (`mode: orch` in the constructor),
  and the interface only widens the existing orchestrator surface by one
  read-only method. No config, env or request can substitute it. Every
  `n.s.mode = ...` is in `network_test.go`, same package.

- **[informational]** `internal/httpapi/network_test.go:40-60` — **the
  non-`fakeMode` tests in this file need a reachable Docker daemon to mean what
  their comment says.** `newNetHarness` wires the REAL orchestrator, so the
  "absent" path they document depends on `ContainerInspect` returning NotFound;
  on a host without a daemon `Status` returns a connection error and those
  cases take the new `:113-121` 500 branch instead. They pass here (Docker
  29.7.2 reachable) and would FAIL, not silently pass wrong, elsewhere — so it
  is a portability note, not a coverage hole. The security-load-bearing
  branches all run against `fakeMode` and need nothing.

## Checked

- **Secrets** — none. `git diff --name-only` is four `.go` files; no `.env*`,
  `*.pem`, `*.key`, `*.tfvars`, `*.tfstate` or credential JSON. Grep of the
  diff for key/secret/token/password patterns: zero hits. New log lines carry
  project id, booleans, `recordSays`, and error strings; the one email
  (`"by", u.Email`) is the pre-existing audit line, unchanged.
- **Injection** — no new parsing boundary. External input is
  `{"lanAccess": bool}` into a `*bool` with an explicit nil check (400
  otherwise); the id comes from the router and is resolved through the
  caller's own store before use. `ModeMismatch` takes no user text —
  `config.NetworkName(project)` and `config.BridgeNameFor` are server-derived
  (`"sdb-" + uuid`, sha1 -> 11 hex). Nothing reaches a shell, argv, SQL,
  template or filesystem path.
- **Authz/authn** — unchanged and correct. 404-before-authz preserved
  (`getProjectFor` at `:92`, before `requireAdmin` at `:100`); enable is
  admin-only, disable is never refused; `deletingState` 409 resurrection guard
  still first; `CanEnable` still its own server field (BUG-019). The new
  `ModeMismatch` call happens strictly after all three gates and only on the
  caller's own project, so it is not a cross-tenant probe. No route lost
  middleware; nothing hand-rolled.
- **Crypto** — nothing in range. No auth, session, JWT/OIDC, password or
  hashing code touched; `BridgeNameFor`'s sha1 is a name derivation, not a
  security primitive, and is unchanged by this diff.
- **Dependencies** — none changed: `go.mod`, `go.sum`,
  `deploy/caddy/ingress/go.{mod,sum}`, `web/package.json`,
  `web/package-lock.json` all absent from the diff; one import was REMOVED
  (`internal/project`). `govulncheck`/`npm audit` — see "Not examined".
- **GCP surface** — not applicable; self-hosted, `docs/deploy/gcp.md` records
  it as out of scope. No IAM, service account, Secret Manager or key file in
  range.

## Not examined

- **Live behaviour.** No integration run, no Kata, no browser e2e, no compose
  deploy. Every branch above is reasoned from source plus the unit tests. No
  exploit was written and none should be inferred from this file.
- **The reachability of finding 1's precondition** — I did not measure whether
  a running project container can end up off its project network on this host;
  that is the whole of its medium confidence.
- **`govulncheck` / `npm audit`** — not re-run in this round; the module graph
  is byte-identical to the tree round 3 read, and its result stands (31 called
  vulnerabilities, all Go stdlib at the local toolchain, none introduced here;
  `web/`'s 1 high, js-yaml GHSA-2883-xcg3-v3hh, unchanged). The authoritative
  number for a gate is `bash deploy/vulncheck.sh` (pinned builder, both
  modules).
- **The vendored Caddy ingress module** and **`deploy/firewall/`** — untouched
  by this diff and not in its blast radius; the firewall's `-i sdb-+` rules
  are load-bearing for finding 1 but were read, not re-reviewed.
- **`web/`** — the UI's reading of the new `rebuilt` field was not diffed;
  `web/dist/client` is not in this range.

## Response (main, 2026-09-10, round 1)

- **Finding 1 (medium) — fixed.** `Orchestrator.ModeMismatch` no longer
  shares the start path's "missing network is not a mismatch" answer: asked
  about a RUNNING container, a missing project network is a mismatch (WARN +
  rebuild). Both callers read through one `currentBridge`, which returns
  `found=false` for NotFound so each decides for itself.
- **Finding 2 (medium) — fixed, and the mechanism is now narrower than
  before this change, not wider.** The stop, the record write and the restart
  run under ONE hold of the per-project lock (`Orchestrator.Rebuild`, taking
  the persist as a closure). A start parked on that lock resumes only once
  the container is up on the new bridge, where EnsureRunning leaves a running
  container alone — so the "parked stale start" case is closed in both the
  old and the widened window. What remains is a stale start that reaches a
  container left DOWN by a failed restart (two faults: a failed start plus a
  read that straddled the change); that is TODO.md's second 🟡 item, now
  restated at that size. `RebuildError.Step` tells the handler which step
  failed so the reply still says exactly what state the project is in.
- **Finding 3 (low) — rebutted.** Two inspects on a no-op PUT by an
  authenticated owner/admin of that project, on an endpoint whose real work
  is a container rebuild, is not a meaningful amplification; the old
  in-memory short-circuit was the very thing that hid drift. The error text
  wraps the same Docker/store errors the previous version did; no new class
  of detail is exposed.
