---
owner: security
date: 2026-09-09
---
# Security review — preflight v1.25.0 round 3: the revocation path, once more, branch by branch   (2026-09-09)

Range reviewed: eccd1a0..worktree
(`git diff HEAD -- internal/httpapi/network.go internal/orchestrator/orchestrator.go internal/httpapi/network_test.go`)

Predecessors: `2026-09-09-preflight-security-v1.25.0.md` (round 1, `5481ea2..eccd1a0`) and
`2026-09-09-preflight-security-v1.25.0-round2.md` (same worktree range, earlier blobs).
Round 2 already existed, so this is round 3 per the dispatch.

## The tree is NOT frozen — pin these blobs

I was told the source tree was frozen. It was not: `internal/orchestrator/orchestrator.go`
changed under me mid-review (`9e8d3b4` → `d122a75`, numstat 60/11 → 72/14), and `TODO.md`
became dirty between two of my own `git diff` invocations. Round 2 reports the same thing
happening to it, one blob earlier. **This review is written against**:

| file | blob | numstat |
|---|---|---|
| `internal/httpapi/network.go` | `01dac9ae` | 45/13 |
| `internal/httpapi/network_test.go` | `d406e0df` | 52/1 |
| `internal/orchestrator/orchestrator.go` | `d122a750` | **72/14** — one revision NEWER than round 2 |

Whatever gets tagged must be re-diffed against those three hashes (`deploy/verify-commit.sh`
exists for this). A gate that reviews a moving target certifies nothing, and this is the
second review in a row to say so. Integrity: all three are text (no `-` in either numstat
column, BUG-007 clean); `network_test.go` goes 8 → 10 `func Test` with none removed
(BUG-005 clean); `git status --porcelain` shows no unexplained ` D` (BUG-006 clean).

Two of round 2's findings were fixed inside that newest revision and are confirmed closed
here: `unmarkIfStillRunning` now inspects on a fresh `context.Background()` + 5 s budget
(`orchestrator.go:1032-1043`) instead of the caller's possibly-dead ctx, and
`bridgeModeMismatch`'s doc comment no longer opens with the contradicted "answers false on
anything it cannot establish". The first of those fixes has a security consequence round 2
could not have seen — see finding 3.

## Verdict: FINDINGS

**No branch of the revocation path leaves the record claiming less access than is in force
by a single fault.** Round 1's finding 2 (`bridgeModeMismatch` false-negative restarting a
revoked container on `lan-`) is genuinely closed, in the correct direction: the container
stays DOWN, and "down" is the one state with nothing in force. The two residual fail-opens
both need a second fault or a race — a project-store write failure (round 2's finding 1) or
a concurrent start whose record snapshot straddles the PUT (round 2's finding 2). I confirm
both independently and do not re-count them as new.

What is new and worse than round 2 assessed: **the record is authoritative for the no-op
short-circuit, so a PUT can never heal a record/bridge divergence** (`network.go:125-130`).
Once either residual fires, the operator's obvious remedy — re-issue the revocation — returns
`200 OK` and touches nothing. The one surviving fail-open therefore has no self-service
recovery, only "re-enable LAN, then revoke again" (two rebuilds, and a window in which LAN
is deliberately restored) or "Stop, then Start". That is a medium, and it is the reason I
would want one sentence in the operator record before the tag, not a code change.

## Per-branch adjudication — PUT /api/projects/{id}/network, revocation (`on == false`)

"In force" = the bridge prefix the container is actually attached to; a stopped or absent
container has nothing in force. All branches follow the initial `SetLanAccess(p.ID, false)`
write at `network.go:131`.

| # | Branch | Record after | In force | Container | Verdict |
|---|---|---|---|---|---|
| 0 | **No-op** (`p.LanAccess == on`, `network.go:126`) | unchanged | whatever it was — **never checked** | untouched | **PASS in steady state, and the hole in every recovery story.** See finding 1. |
| 1 | **Status error** (`network.go:144-156`) | rolled back to LAN | LAN, if the container is up at all | up (presumed; unknown by construction) | **PASS.** Record ≥ force. `stopCalls==0 && ensureCalls==0` asserted by the new test; 500 says "NOT changed", which is true of the container. |
| 2 | **Absent** (`st.State=="absent"`, `network.go:157`) | sandbox | nothing | none | **PASS.** `create()` rebuilds the network when the bridge name disagrees (`orchestrator.go:861-870`) and fails closed on an inspect error there too (`inspect network %s`). |
| 3 | **Stopped** (same branch) | sandbox | nothing | down | **PASS.** Next start heals via `bridgeModeMismatch`, which now refuses rather than guesses. Premise conditional on branch 0/finding 2 (concurrent start). |
| 4 | **Stop error**, rollback OK (`network.go:163-179`) | rolled back to LAN | LAN | up | **PASS — BUG-028's fixed branch.** The marker cleared by `unmarkIfStillRunning` lets terminals reconnect into a project whose record correctly says LAN. |
| 5 | **Stop error**, rollback `ErrNotFound` | none (project deleted mid-PUT) | container removed by the delete path | gone | **PASS.** INFO, not ERROR. Correct: `SetLanAccess` on a removed id cannot resurrect a record. |
| 6 | **Stop error**, rollback error (`network.go:58-63`) | **sandbox** | **LAN** | up | **FAIL-OPEN.** Round 2's finding 1; needs a project-store write failure between two writes. ERROR-logged, and — new — not repairable by retrying the PUT (finding 1). |
| 7 | **EnsureRunning error, container really down** (`network.go:180-186`) | sandbox | nothing | down | **PASS.** Covers the home-symlink refusal, the recreate/remove failure, `create`'s failures (which force-remove and tear down their own network), and the new inspect refusal. The marker survives (only a *successful* `EnsureRunning` deletes it), so auto-reconnects stay refused and the next explicit start applies sandbox. |
| 7a | **EnsureRunning error from the NEW inspect refusal** (`orchestrator.go:233-236`) | sandbox | nothing | down | **PASS, and this is the fix working.** Round 1's finding 2 needed `bridgeModeMismatch` to answer "no mismatch" on an unverifiable inspect; it now returns the error, `EnsureRunning` returns before `ContainerStart`, and nothing is in force. |
| 7b | **EnsureRunning error AFTER a successful start** (`waitReady` timeout, `orchestrator.go:239`) | sandbox | **sandbox** | up, not ready | **PASS.** For a revocation the bridge is `lan-`, so the mismatch is `true`, the container is removed and `create` rebuilds the network to `sdb-`. The only way to reach `ContainerStart` without a rebuild is a `NotFound` network, and then the start fails and `recreateMissingNetwork` builds the bridge under the NEW mode. There is no path left that starts a revoked container on `lan-`. |
| 8 | **Concurrent start racing the record read** (not a failure branch) | sandbox | **LAN** | up | **FAIL-OPEN.** Round 2's finding 2, confirmed. `RunOpts.Lan` is the caller's snapshot; a start parked on the per-project lock while `Stop` holds it resumes with `Lan:true`, and `EnsureRunning` never re-checks a RUNNING container. |

Branches 6 and 8 are the two where the record can claim less access than is in force. Both
were already on the record in round 2; I add only that neither is repairable by the natural
operator action (finding 1).

## Findings

- **[medium / confidence: high]** `internal/httpapi/network.go:125-130` — **the no-op
  short-circuit trusts the record it is supposed to be reconciling, so no PUT can ever heal a
  divergence.** `if p.LanAccess == on { 200 }` returns before `Status`, before `Stop`, before
  `EnsureRunning`, and it is the *only* place the handler decides whether to touch the
  container. After branch 6 (rollback write failed) or branch 8 (racing start) the record
  reads `sandbox` while the container sits on `lan-`; the operator sees "sandbox" in the UI,
  and re-issuing `PUT {lanAccess:false}` — the one action anyone would take — is a silent
  no-op that answers `200 OK` and confirms the false state. Recovery requires an ADMIN to
  re-enable LAN (deliberately restoring the access being revoked, plus a rebuild) and revoke
  again, or a Stop/Start cycle to reach the mismatch heal. So the surviving fail-open is not
  "logged at ERROR and a human fixes it" but "logged at ERROR and the human's first two
  attempts confirm the lie". Exploitable in principle only after another fault, but that is
  precisely the state the ERROR log exists to announce. Remediation, cheapest first: on a
  no-op PUT for a RUNNING project, still run the bridge check (one `NetworkInspect`) and
  rebuild on disagreement; or make `EnsureRunning` re-read `LanAccess` from the store under
  the project lock so every start reconciles; or document the "re-enable then revoke" recovery
  in ADR-067's amendment and in `deploy/remote.md` beside the failed-revocation text.

- **[low / confidence: high]** `internal/httpapi/network.go:58-63` (branch 6) — **the rollback
  write failure remains the one single-fault fail-open**, confirming round 2's finding 1
  independently. `Store.save()` rewrites the whole file, so the first write's success a second
  earlier proves nothing about the second (ENOSPC, read-only remount, EIO). Record says
  sandbox, `lan-` is in force, and `bridgeModeMismatch` is consulted for STOPPED containers
  only, so nothing reconciles. Kept at low rather than round 2's medium *only* because it
  needs a store fault that is not attacker-reachable — with finding 1 layered on top I would
  accept round 2's medium without argument; the two findings should be read as one item.
  Remediation unchanged and still the shortest: for the tightening direction persist the
  record only after the rebuild succeeds, which deletes `rollbackMode` from the dangerous
  direction instead of adding another handler to it.

- **[low / confidence: medium]** `internal/orchestrator/orchestrator.go:1032-1043` —
  **`unmarkIfStillRunning`'s fresh 5 s context removed an accidental fail-safe, and "Running"
  is transient by construction on this path.** Round 2 observed that inspecting with the
  caller's expired ctx made the guard inert; the fix (a `context.Background()` + 5 s budget)
  is right for BUG-030 and correct in isolation, but note what it changes for security: the
  guard is invoked on the very path that just told the daemon to stop the container, and
  Docker reports `State.Running == true` throughout a stop in progress (Kata teardown of a
  wedged sandbox can outlive the caller's 2–3 min ctx). So the sequence "stop request errors
  → fresh inspect says Running → marker deleted → the daemon finishes the kill" is now
  reachable on paths where the dead ctx previously kept the marker, and an open tab's
  automatic reconnect boots the microVM back after a stop the user or the reaper asked for.
  Impact is resource, not access: the tenant's own project, their own quota, and they can boot
  it themselves anyway (`POST .../start`, `?boot=1`). Medium confidence because it depends on
  daemon timing I did not measure. Remediation if it is ever worth one: require a container
  that is running *and* not being stopped — re-inspect once after a short delay, or key the
  unmark on the stop having provably not been accepted rather than on `State.Running`.

- **[low / confidence: high]** `internal/orchestrator/` — **neither security-load-bearing
  orchestrator change has a test.** No `_test.go` in the repo mentions `unmarkIfStillRunning`
  or `bridgeModeMismatch`; `internal/orchestrator`'s suite is unchanged (1/3/1/3/6 `func Test`
  per file, HEAD and worktree identical) and runs in 0.005 s because the package has no fake
  Docker client. Both new tests live in `internal/httpapi` and exercise the handler seam. The
  refusal at `orchestrator.go:233-236` is the fix for a medium finding and is verified by
  reading only. BUG-031's record says this in as many words ("**No unit test**"), which is
  honest; the project's own Definition of Done still calls changed code without a covering
  test a FAIL, and this is the third fail-open in this path in one day found by review rather
  than by a test. That, not any individual finding, is the argument for the orchestrator's
  missing seam.

- **[low / confidence: high]** `internal/httpapi/network.go:154` — the 500 body wraps the raw
  Docker error, which can carry daemon socket paths and internal container names back to an
  authenticated tenant (same shape as the pre-existing "could not be stopped" message and as
  `startProject`'s `writeErr(w, 500, err)`). Log the cause, return a generic sentence.
  Unchanged from round 2; recorded so it is not lost.

- **[informational]** `internal/httpapi/network.go:139-140` — the audit line "network mode
  changed" is emitted *before* the apply and is never retracted; branches 1, 4 and 6 all log
  it and then log a `Warn`/`ERROR` that the change did not take. An operator grepping the
  audit trail for mode changes gets false positives. Either move the line after a successful
  apply or make the rollback line reference it explicitly.

## Answers to the two narrower questions

**1. `unmarkIfStillRunning` — can a non-admin use a failing stop to keep a project running an
admin meant stopped, or resurrect one? No, on both counts, and the marker is not an
authorization control.** Independently reconstructed, agreeing with round 2:

- Its only two readers are `internal/terminal/bridge.go:123` (a non-`boot=1` WS reconnect
  gets close 4002) and `bridge.go:407` (close-code selection). No handler, no proxy, no
  `EnsureRunning` path consults it; nothing about *who may do what* depends on it.
- **There is no cross-tenant stop to subvert.** `stopProject` (`server.go:1070`) resolves the
  project through `getProjectFor` → `s.who(r)` → the *caller's own* `userSpace`, so an admin
  cannot stop, or even see, another user's project — 404, indistinguishable from missing. The
  same is true of the LAN toggle, which means a LAN-enabled project is always the admin's OWN
  project. "A project an admin meant stopped" exists only for the admin's own projects, for
  the idle reaper (host policy, not a person), and for the boot-time workspace migration
  (`cmd/styrman/main.go:276`), which runs before traffic and retries next boot.
- **The owner already has everything the marker withholds**: `POST /api/projects/{id}/start`
  and `?boot=1` both bypass it. Clearing it removes only the guard against *automatic*
  reconnect boots — no state becomes reachable that was not.
- **If the re-inspect errors, the marker is KEPT** (`err == nil && State != nil && Running` is
  the only delete condition). Fail-closed: the cost is BUG-030's symptom returning (reconnects
  refused on a project that never stopped) until the owner presses Start. That is the right
  direction and is the case I would want a test on.
- The residual is timing, not authorization, and it is finding 3: `Running` is what Docker
  reports *during* a stop, so the marker can be cleared moments before the container actually
  dies, after which an open tab's auto-reconnect boots it — idle-reaper evasion / an
  unexpected resurrection of the tenant's own VM. Round 2 called this path "closed in the
  current tree"; against the newer blob it is narrowed, not closed, and I would not leave that
  sentence standing in the record.
- Interaction with a failed revocation: the marker may be cleared, the tenant's terminals
  reconnect into a VM still on `lan-` — but the record was rolled back to LAN in the same
  branch, so every surface reports LAN. Consistent, therefore not a fail-open by the gate's
  own rule. It does convert "stays down until a human acts" into "comes back in the mode being
  revoked", which is worth one sentence wherever the failed-revocation behaviour is written
  down.

**2. The new refusal to start on a network-inspect error — is there a DoS angle, can a tenant
induce it for another tenant's project? No cross-tenant lever; the availability cost is real,
bounded, and the right trade.**

- **Scope is narrower than "any start".** The check runs only in the `err == nil && !Running`
  branch (`orchestrator.go:232`) — an EXISTING, STOPPED container. A running container never
  reaches it; an absent one goes through `create()`, which already failed closed on a
  non-NotFound `NetworkInspect` before this diff (`inspect network %s: %w`). So the added
  refusal covers exactly the stopped-container start.
- **The inspected object is not tenant-reachable.** The name is `config.NetworkName(project)`
  = `sdb-<uuid>`, created, named and removed only by the server on the HOST daemon; a tenant's
  dockerd lives inside their own microVM and cannot create, rename, remove or shadow a host
  network, so there is no name-collision or ID-prefix-ambiguity lever (Docker's ambiguous-name
  500 needs two host networks matching one name, which only the server could create). No API
  returns another tenant's network name, and the UUID is unguessable regardless.
- **The only way to make another tenant's `NetworkInspect` fail is to break the shared host
  daemon or its socket** — which already denies `ContainerInspect`, `ContainerStart`,
  `Exec` and every terminal, with or without this change. The change does not create a new
  amplification: one failed inspect blocks one start, no retry storm, no lock held (the
  per-project lock is held, but only for that call's duration).
- **Honest residual:** the failure is now hard where it used to be silently degraded. A daemon
  that answers `ContainerInspect` but times out on `NetworkInspect` (an overloaded host, which
  a tenant CAN contribute to through their own project churn — creates, image pulls, DinD
  load) will refuse starts for stopped projects whose mode never changed, including other
  tenants'. That is availability-only, self-healing when the daemon recovers, and strictly
  preferable to starting a container on a bridge nobody verified. I would not gate the release
  on it; I would not call it "no cost" either.

## Checked

- **Secrets** — none. `git diff HEAD --name-only` shows no `.env*`, `*.pem`, `*.key`,
  `*.tfvars`, `*.tfstate` or credential JSON; grep of the source diff for
  key/secret/token/password patterns: zero hits. New log lines carry project id, booleans and
  error strings; the one email (`"by", u.Email`) is the pre-existing audit line.
- **Injection** — no new parsing boundary. The only external input is `{"lanAccess": bool}`
  into a `*bool` with an explicit nil check (400 otherwise); the project id comes from
  `r.PathValue` and is resolved through the caller's own store before use. Nothing new reaches
  a shell, argv, SQL, template or filesystem path; `config.NetworkName`/`BridgeNameFor` are
  server-derived (sha1 of the network name, 11 hex chars) and take no user text.
- **Authz/authn** — unchanged and correct: 404-before-authz (`getProjectFor` first),
  `requireAdmin` (`auth.go:100`) on enabling only, tightening never refused, `deletingState`
  409 resurrection guard, `CanEnable` as its own server-side field (BUG-019). The new
  Status-error branch returns before any orchestrator call, so it cannot be used to probe
  another tenant. Nothing hand-rolled; no route lost middleware.
- **Crypto** — nothing hand-rolled, nothing touched. No auth, session, JWT/OIDC, password or
  hashing code in range.
- **Dependencies** — none changed: `go.mod`, `go.sum`, `deploy/caddy/ingress/go.{mod,sum}`,
  `web/package.json`, `web/package-lock.json` all clean in `git diff HEAD --name-only`. One
  new *internal* import (`internal/project`, for `ErrNotFound`). I did not re-run
  `govulncheck` in this round: the module graph is byte-identical to the tree round 2 scanned,
  and its result stands — 31 called vulnerabilities, all Go standard library at the local
  go1.25 toolchain (e.g. GO-2025-4007), none introduced or made reachable by this diff. The
  authoritative number for the gate is `bash deploy/vulncheck.sh` (pinned builder, both
  modules), which I did not run; if the builder's Go is < 1.25.3, bumping it is the fix. `npm
  audit` not re-run — no `web/` dependency changed; round 1's 1 high (js-yaml
  GHSA-2883-xcg3-v3hh) stands.
- **GCP surface** — not applicable, self-hosted; `docs/deploy/gcp.md` records it as out of
  scope. No IAM, service account, `--allow-unauthenticated`, Secret Manager path or key file
  in range.
- **Build/test on the reviewed blobs** — `go build ./...`, `go vet ./internal/httpapi
  ./internal/orchestrator`, and `go test -count=1 ./internal/httpapi ./internal/orchestrator
  ./internal/project` all pass; the two new handler tests
  (`TestNetworkStatusErrorOnRevocationRollsBack`,
  `TestNetworkAbsentContainerJustWritesTheRecord`) run and pass individually.

## Not examined

- **Live behaviour.** No integration run (`.agents/docs/testing/styrman/integration.sh`), no
  Kata run, no browser e2e, no compose deploy. Every branch above is reasoned from source and
  the unit tests. Findings 1, 2 and 3 are failure modes I did not reproduce, and no exploit
  was written or should be inferred from this file.
- **The daemon-side timing in finding 3** — whether `State.Running` stays true long enough
  after a failed `ContainerStop` for the 5 s inspect to catch it is asserted from Docker/Kata
  semantics, not measured. That is the whole of its medium confidence.
- **`deploy/vulncheck.sh`** (pinned two-module scan) and the vendored Caddy ingress module —
  untouched by this diff, not in its blast radius, not run.
- **`deploy/firewall/`** — ADR-067 Phases 2/3 deleted the root-mediated LAN machinery; none of
  it is reachable from these three files.
- **The other dirty paths** — `.agents/docs/specs/styrman/versioning.md`, `TODO.md`,
  `web/dist/client/_shell.html`, `DECISIONS.md`, `bugs/INDEX.md` and the BUG-029/030/031
  records — are outside the dispatched range. I read the DECISIONS.md correction and the
  BUG-031 record only to check whether they overclaim; they do not (the ADR correction names
  the rollback-write branch as the one that still needs a human, and BUG-031 states plainly
  that it has no unit test). Neither mentions the no-op short-circuit of finding 1, and the
  round-2 sentence declaring the resurrection path "closed" should be qualified against the
  newer blob.
- **`web/dist/client/**`** — not diffed; a mismatch between `web/src` and the embedded bundle
  would not have been caught here.
- **The credential-broker reachability test** — still OWED from round 1, still unmeasured,
  and nothing in this range measures it.
