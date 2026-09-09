---
owner: main
date: 2026-09-09
---
# Preflight — v1.25.0 (ADR-067: the network mode is the bridge interface prefix)

Range reviewed: v1.24.1..b142106

Base: `v1.24.1` (`d93d7d5`, the last tag). Head: `b142106`, the pre-gate fix
commit this gate produced (see "Pre-gate commit"). 12 commits, 46 source
files, roughly +1,800 / −4,500 outside docs and `web/dist`.

**VERDICT: SHIP** — v1.25.0, MINOR.

## What is still wrong (lead with it)

Nothing blocking. Every high and medium found by seven review passes is
either fixed in `b142106` or is a documented residual that needs more than
one fault to reach, has a recovery, and has its shorter fix named. In
severity order:

| sev / conf | where | finding | disposition |
|---|---|---|---|
| medium / high | `internal/httpapi/network.go` PUT | **Record-first design, last branch.** A revocation persists sandbox, then applies. Three single-fault fail-opens on that path were found and fixed today (BUG-028 `Stop` fails; BUG-029 `Status` errors; BUG-031 mismatch-check errors). What remains needs TWO faults: the store WRITE failing during the rollback, after the first write succeeded. ERROR-logged. Recovery: Stop, then Start (the stopped-container heal applies the record). | Accepted. The fix that deletes the class — persist AFTER apply on tightening — is in `TODO.md` with the reasoning: three rounds on one mechanism is AGENTS.md's signal to delete it, not defend it, and that restructure is not a release-eve change. |
| medium / medium | same PUT + `POST /start` | **Toggle vs concurrent start.** `RunOpts.Lan` is a caller snapshot read outside the orchestrator lock; a start that read the OLD record can bring the container up on the old bridge after the toggle's `Status` said stopped. Running containers are never re-checked. Needs a race inside one request's window. | Accepted; fix sketch in `TODO.md` (re-read the record where the bridge name is decided). |
| medium / high | same PUT | **No-op short-circuit trusts the record**, so a wrong record (only reachable via the two rows above) cannot be re-applied by re-issuing the PUT. | Accepted: Stop-then-Start is the self-service recovery today; folded into the TODO restructure (compare against the LIVE bridge). |
| medium / — | production host, `sshd` | `passwordauthentication yes` on `0.0.0.0:22`, reachable from any LAN-mode project. ADR-066 gated on this; ADR-067 deleted the gate at the operator's decision. | **Operator's.** Stated in `deploy/remote.md` as advice and in ADR-067's amendment as a recorded cost. Not a code finding. |
| medium / — | credential broker | The broker's bridge-only binding now rests on Docker's per-bridge DROP alone, and broker reachability from a LAN project is **untested** (it only starts containerized; the live run used a host process). By reading: `containerHoldsIP` plus per-user secrets mean a project reaching any broker address is served its own user's. | **Owed**, not blocking: the test is specified in `reviews/2026-09-09-security-adr-067.md`; ADR-067's amendment marks it owed. |
| low / high | `internal/orchestrator` | `unmarkIfStillRunning` and `bridgeModeMismatch`'s error path — one of them a security fix — have **no unit test**. The package has no fake Docker client (BUG-003's residual). Verifier records it as FAIL/UNKNOWN. | Reported, not hidden. Covered transitively by integration/e2e. |
| low / medium | `unmarkIfStillRunning` | One `Running` sample can catch a stop in flight and clear the marker; the tenant's own reconnect could then boot its own VM. No access gain. Plus: silent when the marker is kept. | `TODO.md` (two-sample check; one `Warn`). |
| low | vulnerability scan | `deploy/vulncheck.sh` exits 3 on GO-2026-4887 / GO-2026-4883 (`docker/docker`, `Fixed in: N/A`), unchanged since v1.24.1. A review's ad-hoc "31 stdlib vulns" figure was **not reproduced** by the authoritative scan. | Unchanged from the last release; `TODO.md` carries the unconfirmed figure for the next builder bump. |

Four remaining lows from the review reports (log-level nits, a stale
comment, a cosmetic race note) are in the reports and not restated.

## The rows

**Reviewer** (`reviewer`, Opus) — four rounds, because each fix commit is
re-reviewed rather than trusted, and each round found something the last
had opened:
- `reviews/2026-09-09-review-adr-067.md` — `44e81e0..5481ea2`, the bulk of
  ADR-067. Approve with nits; 3 fixed in `5697e14`.
- `reviews/2026-09-09-preflight-review-v1.25.0.md` — `5481ea2..eccd1a0`.
  **REQUEST CHANGES**: BUG-029 (medium), BUG-030, gofmt.
- `...-round2.md` — the fix. Approve with nits; two lows taken.
- `...-round3.md`, `...-round4.md` — the subsequent hunks. Approve with
  nits; all prior lows closed except the untestable-orchestrator one.

**Security** (`security`) — three rounds:
- `reviews/2026-09-09-security-adr-067.md` — `44e81e0..5481ea2`. 0/3/4.
  Finding 1 (fail-open) became BUG-028.
- `reviews/2026-09-09-preflight-security-v1.25.0.md` — `5481ea2..eccd1a0`.
  0/2/3. Independently found BUG-029 and, below it, BUG-031.
- `...-round2.md`, `...-round3.md` — the fixes. Per-branch walk: **no
  single-fault fail-open remains** on the revocation path. Residuals above.

**Verifier** (`verifier`) — `testing/2026-09-09-verify-release-v1.25.0-final.md`
on the exact tree of `b142106` (frozen; the earlier
`...-verify-release-v1.25.0.md` is marked PARTIAL/SUPERSEDED — it hit its
turn limit on a stale range and carries no verdict):

| tier | result |
|---|---|
| `gofmt -l`, `go build`, `go vet` | clean |
| `go test ./... -race` | 15 packages, **198** tests, 0 fail (194 baseline + 4 appended; no drop) |
| ingress module | 51/0 |
| `tsc -b`, `oxlint`, vitest | clean; **127** across 9 files |
| `integration.sh` (real Kata) | **84 / 0 / 3** |
| `web/e2e/run.sh` | **86 / 0 / 2, exit 0**; no leaked container, network or state dir; production untouched |
| `deploy/firewall/test.sh` (`unshare -Urnm`) | **24 / 0** |
| `deploy/vulncheck.sh` | exit 3, the two known unfixable `docker/docker` advisories only |
| `deploy/verify-commit.sh b142106` | **OK — "builds and tests clean in isolation — safe to tag"** |

**Live adversarial run** — `testing/2026-09-09-lan-mode-live.md`: all five
steps on this host; R12 held under the `cap_net_admin` route attack;
`iptables -S` identical before and after.

## Trail (Step 3)

- Newest `changes/` entry describes this release's work — pass
  (`2026-09-09-adr-067-preflight-fixes.md`; one entry per phase 0–6 plus the
  preflight fixes).
- No future `date:` under the docs root — pass.
- `changes/` and `DECISIONS.md` append-only since base — pass. Two commits
  show as modifying `DECISIONS.md`; both are pure appends (numstat
  `55 0`, `32 0`, plus `18 0` in `b142106`). No `changes/` entry was
  modified after creation.
- `ROADMAP.md` and `TODO.md` touched since base — yes, in five commits.
  Report-only.

## Docs (Step 4)

`README.md`, `specs/`, `deploy/` checked against the release diff. One
finding: `specs/styrman/versioning.md` still carried an "ADR-065 host-side
files" paragraph (`.path` unit, spool, marker dir, a UI reporting "LAN
unavailable") — **fixed in `b142106`**, rewritten to ADR-067 including the
post-upgrade cleanup caveat. `deploy/remote.md`, `specs/styrman/guide.md`,
`AGENTS.md`'s network paragraph, the testing README's counts and
`ROADMAP.md` were updated in `5697e14`. `README.md` carries no stale claim
and was not touched (report-only).

## Version

**v1.25.0**, MINOR, by AGENTS.md's rule: new behaviour for a user of the
artifact (a working two-mode network switch), nothing that worked broken
(the deleted machinery never applied an allowance on any deployment). The
major test was checked live, not assumed: an upgrading host needs NO action
for the feature to work — the mode is server-side. The retired
`styrman-firewall.path` and spool mount stay enabled and inert on an
auto-updated host by the updater's design; `install.sh` converges them
(`deploy/remote.md` "Post-upgrade cleanup"). No tracked file pins a version;
the tag is the version.

## Pre-gate commit

`b142106` — "ADR-067 preflight: three fail-opens in one revocation path,
closed one round at a time". Source (+169/−28 across `network.go`,
`network_test.go`, `orchestrator.go`) plus every record of the gate so far.
Went through its own DoD rows (reviewer ×3, security ×2, verifier ×2,
changelog) before commit. Staged by name; manual secret scan clean (no
`gitleaks` on this host).

## What was not checked

- **Broker reachability from a LAN-mode project** — untested, see above.
- **The browser walk-through** (plan Phase 5, box 4) — the operator's; the
  shell-level equivalent passed in the live run.
- **A real Google sign-in after deploy** — the one deploy-verification step
  an agent cannot perform (`versioning.md` step 5). Health and version
  label will be checked; the OAuth portal surviving the bump will not.
- **The auto-updater's build from the pushed tag** — `verify-commit.sh` ran
  the same server-image build on the same commit, which is the closest
  proxy; the real thing happens on the next timer tick and is watched.
- **`README.md` describing the new mode** — it carries no false claim; whether
  it should say more is a docs-completeness call, not a gate finding.
- The "31 stdlib vulns" figure — unconfirmed either way.

## Ship

Signed tag `v1.25.0` on the release commit that carries this report and the
release changelog entry; plain push; the enabled `styrman-update.timer`
(deployed `v1.24.1`, `deployed-sha e0c9596`, no `failed-version` latch,
last tick "already up to date") deploys within ~5 min with automatic
rollback on a failed health check. Signing uses `git -c gpg.format=ssh -c
user.signingkey=…` — the key matches `/etc/styrman/allowed_signers` — so no
stored git config changes (AGENTS.md).
