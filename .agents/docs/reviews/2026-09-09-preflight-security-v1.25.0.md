---
owner: security
date: 2026-09-09
---
# Security review — preflight gate, v1.25.0 (ADR-067 Phase 6 fixes)   (2026-09-09)

Range reviewed: 5481ea2..eccd1a0

Source in range: `internal/httpapi/network.go`, `internal/httpapi/network_test.go`,
`internal/httpapi/server.go`, `web/src/lib/api.ts`, `web/e2e/run.mjs`. Blast radius
followed outward from the changed handler: `internal/orchestrator/orchestrator.go`
(`Status`, `Stop`/`stopLocked`, `EnsureRunning`, `bridgeModeMismatch`, `create`),
`internal/project/store.go` (`SetLanAccess`, `Remove`), `internal/httpapi/server.go`
(`netMu`, `deleteProject`'s record removal, `getProjectFor`, `requireAdmin`), and the
records that carry the earlier review's findings 2 and 3 (`DECISIONS.md` ADR-067
amendment, `deploy/remote.md`, `BUG-028`). Docs otherwise skipped per dispatch;
`web/dist` skipped.

Predecessor: `.agents/docs/reviews/2026-09-09-security-adr-067.md` (44e81e0..5481ea2).

## Verdict: FINDINGS

**Finding 1 of the earlier review is closed on ONE of its three failure branches, not on
all of them.** The `Stop`-failure branch — the branch BUG-028 names, tests and
mutation-checks — is now correct: the record is rolled back to what is in force and the
caller is told the mode did not change. But the branch immediately above it is not, and
it is worse: when `Status` returns an ERROR the handler treats it as "stopped or absent",
persists the requested mode, and answers **200 OK**. `Orchestrator.Status` maps
container-not-found to `State:"absent"` with a *nil* error (`orchestrator.go:147-160`), so
`err != nil` means the true state is UNKNOWN — a dockerd hiccup, a socket failure, a
context deadline — and the container may well still be running on its `lan-` bridge. For a
revocation that is exactly BUG-028's shape (record and every surface say sandbox, LAN is
in force, nothing reconciles because the mismatch heal fires for stopped containers only),
except the operator is told it succeeded instead of being told it failed. By the dispatch's
own criterion — "any branch where the record claims LESS access than is in force is a
fail-open and a blocking finding" — this blocks. It is not attacker-triggerable; it needs a
Docker fault to coincide with a revocation, which is why it is medium and not high. The new
`fakeMode.Status` never returns an error, so no test reaches this branch.

## Findings

- **[medium / confidence: high]** `internal/httpapi/network.go:113-119` — **the `Status`-error
  branch is still fail-open, and it reports success.** `st, err := s.mode.Status(...)`; the
  guard is `if err != nil || st.State != "running"`, and the comment says "Stopped or
  absent: the record is the truth". That is true for `st.State != "running"` and false for
  `err != nil`: `Status` returns `("absent", nil)` for not-found, so an error is a *failed
  observation*, not evidence of a stopped container. The record has already been written to
  `on` at line 102; the handler then returns `200 OK {lanAccess: on}` without attempting a
  `Stop`. Revocation with a running container behind a transient Docker failure ⇒ record,
  `GET /network`, `projectView.LanAccess`, the switch and the badge all say sandbox while
  the guest keeps `lan-<hash>`: full host reach (incl. sshd) and full LAN reach, until
  someone happens to stop and start the project. Exploitable in principle by anyone who can
  make one `ContainerInspect` fail — in practice a daemon restart or overload during
  maintenance, not a guest capability. Remediation: fail the request on an unknown state —
  roll the record back and 500 exactly as the `Stop` branch does — or fall through to `Stop`
  instead of short-circuiting, and only short-circuit on `err == nil && state != running`.
  Add the `Status`-error case to `fakeMode`.

- **[medium / confidence: medium]** `internal/httpapi/network.go:148-153` +
  `internal/orchestrator/orchestrator.go:232-241, 253-270, 350-352, 821-838` — **the
  `EnsureRunning`-failure branch's premise ("the container is down, so nothing is in
  force") is not guaranteed.** `EnsureRunning` can return an error with the container
  RUNNING: `waitReady` fails at `orchestrator.go:350` after a successful `ContainerStart`.
  On its own that is harmless, because the rebuild path started the container on the NEW
  bridge. It stops being harmless when `bridgeModeMismatch` false-negatives — it returns
  `false` on any `NetworkInspect` error by design (`orchestrator.go:822-825`, documented as
  "answers false on anything it cannot establish" because a `true` force-removes a
  container). Then the stopped container is started on its OLD network
  (`case err == nil:` → `ContainerStart`), `waitReady` times out, the handler 500s — and
  the record says sandbox while the container runs on `lan-`. Same class as finding 1, one
  layer down, and `TestNetworkFailedRestartKeepsTheRecord` asserts the opposite premise for
  the *enable* direction only. Narrower (needs a `NetworkInspect` failure) hence medium
  confidence, not high. Remediation: for the tightening direction (`on == false`), verify
  after a failed `EnsureRunning` — one `Status`/`NetworkInspect` — and roll the record back
  if the container is running; or persist the tightened record only after a verified
  rebuild, which was the earlier review's "cheapest correct variant" and remains so.

- **[low / confidence: high]** `internal/httpapi/network.go:132-134` — **the rollback is a
  blind write, not a compare-and-swap.** `netMu` is released across `Status`/`Stop` (up to
  `lanRebuildTimeout`, 3 min, correctly — a server-wide mutex must not be held that long),
  then reacquired to write `!on` regardless of what the record says by then. Analysis of who
  else can write: only this handler and `deleteProject`. A concurrent delete is safe —
  `Remove` makes `SetLanAccess` return `ErrNotFound` (`store.go:226-229`), so a deleted
  project's record is never resurrected (good, and worth keeping in a test). A single
  concurrent PUT is also benign: it can only change the value by requesting `!on`, which is
  what the rollback writes. The lost update needs three overlapping PUTs — A `on=true`, B
  `on=false`, C `on=true` where C's rebuild succeeds — after which A's late rollback writes
  `false` over C's legitimate `true` while LAN is in force. **Answer to question 2: yes,
  the rollback can overwrite a newer value, but only an ADMIN can reach the fail-open
  direction**, because `on=true` is admin-gated (`network.go:79`). A non-admin may only
  submit `on=false`, whose rollback writes `true` — the record then over-reports access,
  which is the safe direction for reporting; note it is not entirely inert, since
  `ensureFor`/`EnsureRunning` apply `p.LanAccess` at the next start, so a non-admin's failed
  disable re-arms the LAN grant that was already in force (restores the prior admin
  decision, does not create one). Remediation: conditional write (`set !on only if the
  record still reads on`), or hold a per-project apply lock for the whole PUT.

- **[low / confidence: high]** `.agents/docs/architecture/DECISIONS.md` (ADR-067 amendment,
  "Consequences") and `.agents/docs/bugs/BUG-028-*.md` (`status: fixed`) — **the record
  overclaims the fix.** "A failed revocation now FAILS CLOSED … so no surface reports
  'sandboxed' while a container still runs on a `lan-` bridge" is true of the `Stop` branch
  and false of the two branches above. `Verification:` cites the one test that exists. The
  next reader will treat the whole handler as settled. Remediation: qualify to "a failed
  **Stop**", and either keep BUG-028 open or open a follow-up naming the `Status`-error and
  post-start-`EnsureRunning` branches.

- **[low / confidence: medium]** `internal/httpapi/network.go:136-142` — **the loudest signal
  in the handler is also raised for a benign race, and it asserts an unverified fact.** The
  `ERROR` line "rollback failed, record disagrees with the running container … needs a
  human" fires on `ErrNotFound` too, i.e. whenever the project was deleted mid-PUT — routine,
  not actionable. Meanwhile `"actual", !on` states what the handler never checked: a `Stop`
  error does not prove the container still runs (`stopLocked`'s graceful `kill -TERM 1` +
  `waitStopped` may already have powered the guest off and only `ContainerStop`'s timeout
  errored). The residual fail-open in findings 1 and 2 has no signal at all, and this one
  will be tuned out. Remediation: branch on `errors.Is(rollback, project.ErrNotFound)` and
  log that at debug/warn; phrase the fact as "requested mode not applied", not as an
  assertion about the container.

## Answers to the dispatched questions

**1. Does the fix close finding 1? Branch by branch** (record vs what is in force; `on` is
the requested mode, `!on` the previous one):

| branch | record after | in force | response | verdict |
|---|---|---|---|---|
| `Status` err != nil (state UNKNOWN) | `on` | possibly `!on` — container may be running on the old bridge | **200 OK** | **FAIL-OPEN for `on=false`** (finding 1) |
| `Status` ok, state != running | `on` | nothing (container down); applied at next start by the mismatch heal | 200 OK | correct |
| `Stop` err, rollback ok | `!on` | `!on` (container up, old bridge) | 500 "mode was NOT changed" | **correct — this is the fixed branch** |
| `Stop` err, rollback err | `on` | `!on` | 500 | fail-open for `on=false`; only an ERROR log, nothing reconciles (residual, unavoidable at this layer; see finding 5 about the signal) |
| `EnsureRunning` err, container really down | `on` | nothing | 500 "start it to apply" | correct |
| `EnsureRunning` err after `ContainerStart` + `bridgeModeMismatch` false-negative | `on` | `!on` (running, old bridge) | 500 | **fail-open for `on=false`** (finding 2) |

So: the fix is real and the direction of the change is right, but "closed" is true of one
branch out of the three that can fail. Not clear for release without either the `Status`
branch fixed or an explicit, recorded acceptance that a revocation can still silently
fail open on a Docker fault.

**2. Rollback race** — see finding 3. Structurally possible; the record-under-reports
outcome requires an admin `on=true` in flight; a non-admin acting alone can only make the
record over-report. No project-record resurrection: `SetLanAccess` on a removed id returns
`ErrNotFound`.

**3. The `modeRuntime` seam** — **no authorization decision passes through it.** Ordering in
`setProjectNetwork`: body decode → `getProjectFor` (404 before any authz signal, so another
user's project is indistinguishable from a missing one) → `requireAdmin` when `on == true`
→ `deletingState` 409 → record write under `netMu` → *only then* `s.mode`. Swapping `mode`
changes what happens to the container, never who may ask. The field is unexported, assigned
once in `New` (`server.go:180`) to the same `*orchestrator.Orchestrator` as `orch`/`guest`,
and reassigned only in `_test.go`; no second production constructor of `Server` exists (the
other `&Server{}` literals are all in test files), so there is no path to a nil `mode`. Same
shape as the v1.22.0 `guestRuntime` seam. Not a finding.

**4. `api.ts` / `run.mjs` beyond timeouts** — nothing. `web/src/lib/api.ts` is +15/−7, all of
it comment prose plus `150000 → 210000`; `web/e2e/run.mjs` is +5/−4, two identical constant
changes plus a comment. No new endpoint, header, credential, token handling, `dangerously*`
sink, or auth-relevant client logic. The pairing is now correct against the server
(`lanRebuildTimeout = 3 min` < 210 s). One non-security note, low confidence because I did
not inspect the production edge config: the ingress plugin sets no reverse-proxy
read/write timeout, but the public path is fronted by Cloudflare, whose origin timeout is
typically 100 s — if so, the browser sees the truncation regardless of the 210 s abort
signal. Availability/UX only; it cannot desynchronize the record, since the server's context
is `context.Background()`-derived and survives client disconnect.

## Earlier findings 2 and 3: are they RECORDED, and is the record softened?

- **Finding 2 (sshd password auth)** — recorded, and **not softened**. ADR-067's amendment
  states the exposure in the sharpest available terms: "The host's sshd is reachable from a
  LAN project with whatever authentication the host allows … On the production host at the
  time of writing: `passwordauthentication yes`, listening on `0.0.0.0:22`. That is a real
  exposure the moment any project is put in LAN mode." `deploy/remote.md` retitles the
  section "**Advice, not a precondition**", says plainly that nothing in the code gates LAN
  mode on it, spells out `%sudo NOPASSWD:ALL` ⇒ a guessed password is root, and repeats that
  this host is unhardened. The **decision** to demote a gate to advice is the operator's and
  is recorded as such; the *facts* the earlier review found are all present. No softening
  found. What is not written down anywhere I could find: that the demotion means an operator
  can enable LAN mode on an unhardened host with no interlock at all — worth one sentence in
  `/preflight`'s deploy step, since prose in `remote.md` is now the only control.
- **Finding 3 (broker rests on one layer, unmeasured)** — recorded, **not softened, and
  correctly marked owed**. The amendment says the bridge-only binding "now rests on one
  layer, not two", reproduces the `containerHoldsIP` argument, and states "**Not measured** …
  The test that settles it is specified in the security review (compose deploy, two users,
  four probes) and is owed before this is called closed", with `Verification: … the broker
  test above is OWED, not done.` The handoff carries it as item 3 and the plan gates it on
  the next change to `internal/credbroker`. Accurate.
- The earlier review's accepted-risk note (the deleted SSH-key latch, the transitive path)
  was already recorded in ADR-067 part (4) — "the transitive path ADR-066 named … is
  reopened and accepted, not overlooked" — with the operator's verbatim decision. Faithful.

## Checked

- **Secrets** — none. No `.env`/`.pem`/`.key`/`tfvars`/`tfstate`/credential path in the
  range (`git diff --name-only` filtered); grep of the source diff for
  key/secret/token/password/private-key patterns: zero hits. New log lines
  (`network.go:108,138,141`) carry project id, actor email and booleans — no credential
  material, and the mode-change audit line is preserved.
- **Injection** — no new parsing boundary. The handler's only input remains a single
  `*bool` with an explicit nil check; no path, template, SQL, shell or argv construction is
  added anywhere in the range. `web/e2e/run.mjs` and `api.ts` add no new request
  construction.
- **Authz/authn** — unchanged and correct: 404-before-authz via `getProjectFor`,
  `requireAdmin` only for enabling, tightening always allowed to the owner, delete-in-flight
  409, `CanEnable` still a server-side field (BUG-019). The new seam sits entirely after the
  authorization decision (question 3). No route lost middleware; the ingress-token guard is
  untouched. The unrelated `server.go` hunk deletes the empty `if err == nil {}` block the
  earlier review flagged — good; the dangling `NetPolicy` doc comment it also flagged is
  gone from the file.
- **Crypto** — nothing hand-rolled, nothing touched. No auth, session, JWT/OIDC, password or
  hashing code in the range.
- **Dependencies** — **none changed**: `go.mod`, `go.sum`, `deploy/caddy/ingress/go.{mod,sum}`,
  `web/package.json`, `web/package-lock.json` are all untouched in 5481ea2..eccd1a0.
  `go vet ./internal/httpapi/` clean. `govulncheck ./...` (root module) run here: 31 callable
  vulnerabilities, one module + stdlib — **identical to the predecessor review's count and
  with the same caveat**: this was the local `go1.25.0` toolchain, not the pinned
  `golang:1.25.13-trixie` builder `deploy/vulncheck.sh` uses and that builds the release, so
  the great majority are stdlib advisories already fixed in the builder's Go. What survives
  the toolchain difference is pre-existing and out of range
  (`github.com/docker/docker@v28.5.2+incompatible`, GO-2026-4887 / GO-2026-4883, both
  "Fixed in: N/A"). `bash deploy/vulncheck.sh` is the number that matters for the gate; I
  did not run it (it needs the builder image). `npm audit` not re-run — `web/` dependencies
  are byte-identical in this range; the predecessor's 1 high (js-yaml GHSA-2883-xcg3-v3hh)
  stands unchanged.
- **GCP surface** — none. Self-hosted; `docs/deploy/gcp.md` is "not applicable". No IAM
  binding, service account, `--allow-unauthenticated`, Secret Manager path or key file in
  the range.

## Not examined

- **Nothing was executed.** No Go test run, no `web` suite, no integration or e2e script, no
  compose deploy. The verifier's counts in `changes/2026-09-09-adr-067-phase6-gate.md` are
  taken on trust; the two new tests were read, and `fakeMode` was read closely enough to
  state which branch it does *not* cover (`Status` error).
- **The broker reachability test is still owed** — unchanged from the predecessor review.
  Nothing in this range measures it, and this review adds no measurement either.
- **Uncommitted working tree** — `.agents/docs/specs/styrman/versioning.md` (modified) and
  `.agents/docs/testing/2026-09-09-verify-release-v1.25.0.md` (untracked) are outside
  5481ea2..eccd1a0 and were not reviewed. Docs paths only.
- **`web/dist/client/**`** — skipped per dispatch. The rebuilt bundles were not diffed;
  a mismatch between `web/src` and the embedded `dist` would not have been caught here.
- **Everything the predecessor listed as not examined** (`deploy/firewall/test.sh` execution,
  Kata guest hardening, IPv6 on a `lan-` bridge) remains not examined; this range does not
  touch those files.
- **Finding 1's blast radius in the UI** — `LocalNetwork.tsx` / `lan.ts` were not read; they
  render the record, so they inherit whatever the record says rather than adding a control.

## Addendum — working tree moved under this review (NOT in 5481ea2..eccd1a0)

While this review was being written the main thread modified `internal/httpapi/network.go`,
`internal/httpapi/network_test.go` and `internal/orchestrator/orchestrator.go` in the
working tree. Those changes are **not** in the reviewed range and the findings above stand
as written against `eccd1a0`. Read at working-tree state, for the release gate's benefit:

- **Finding 1 is addressed, and correctly.** The `Status` guard is split: `err != nil` now
  calls the new `rollbackMode` helper and answers 500 "the project could not be inspected,
  so the mode was NOT changed"; `st.State != "running"` keeps the old 200 path. That is the
  remediation this report asked for, in the safe direction for both `on=true` and
  `on=false` (an inspect failure discards the operator's change and they retry; nothing is
  ever reported as tighter than it is). Two tests were APPENDED, not overwritten
  (`git diff --numstat` on `network_test.go`: +53/−2, the 2 being an import reorder):
  `TestNetworkStatusErrorOnRevocationRollsBack` and
  `TestNetworkAbsentContainerJustWritesTheRecord`, the second pinning the distinction
  between an inspect failure and `("absent", nil)`. `fakeMode` gained `statusErr`. I did not
  run the suite.
- **Finding 2 is NOT addressed.** The `EnsureRunning` branch is byte-identical, and
  `bridgeModeMismatch` still returns `false` on a `NetworkInspect` error, so the
  post-`ContainerStart` `waitReady` failure path can still leave a running container on the
  old bridge under a record claiming the new mode.
- **Findings 3 and 5 are NOT addressed.** `rollbackMode` is a clean extraction of the same
  blind write (`SetLanAccess(id, !requested)` under a re-taken `netMu`), and it still logs
  `ERROR … "actual", !requested` for the `ErrNotFound` (project deleted mid-PUT) case.
- **New, low / confidence medium:** `orchestrator.go:989-1002` now deletes the
  deliberately-stopped marker when `stopLocked` returns an error. Correct for the reviewer's
  UX finding, but note the interaction with a failed revocation: `stopLocked` can power the
  guest off via `kill -TERM 1` and *then* error on `ContainerStop`'s timeout, in which case
  the marker no longer suppresses terminal auto-reconnect, `ensureFor` runs
  `EnsureRunning(Lan: p.LanAccess)` — and `p.LanAccess` has just been rolled back to LAN —
  so the project comes back up with LAN access moments after an admin's revocation returned
  500. Consistent with the record, therefore not a fail-open by this report's criterion, but
  it converts "stays down until a human acts" into "returns in the mode being revoked". The
  operator-visible remedy is to retry the revocation; worth one sentence wherever the
  failed-revocation behaviour is documented.
- The reviewed-range verdict is unchanged. If v1.25.0 is cut from a commit that includes
  these working-tree changes, that commit is a different range from the one reviewed here
  and finding 1's status should be re-read from it rather than from this line.
