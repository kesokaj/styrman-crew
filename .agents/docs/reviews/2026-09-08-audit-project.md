---
owner: main
date: 2026-09-08
---
# Project audit — styrman-crew, whole tree   (2026-09-08)

Scope: the whole repository at `414be72` (v1.24.0 shipped and live), audited
as code that stands — not as a diff. Four agents in parallel; their artifacts
are linked below and are the detail, this is the merge.

- `plans/2026-09-08-audit-recon.md` — structure, uniformity, dead code
- `reviews/2026-09-08-audit-standards.md` — standards (APPROVE WITH NITS)
- `reviews/2026-09-08-audit-security.md` — security (FINDINGS, no high)
- `testing/2026-09-08-audit-verification.md` — suites + coverage map

## Verdict

**No high findings. No live path was found that lets a guest escape its
project, reach another tenant, or reach the host.** The isolation core holds
under direct attack on its own regression history: `internal/files`'
`os.Root` confinement, `projectHomeIsReal`/`realBelow` on every Docker bind
path, the per-user project stores and the auth guard were all probed against
BUG-002/003/004 and none regressed.

What the audit did find is a consistent shape, and it is worth naming because
it is the same shape the ADR-066 release gate hit sixteen times: **a guard
that is correct where it was written and absent where it was not.** Three of
the four mediums below are one file doing the right thing and its siblings
not doing it — the FIFO/`O_NONBLOCK` guard, the symlink confinement, the
ownership assertion. None is a clever new attack; each is an existing,
documented defence that stops at a boundary nobody redrew.

## Summary

| Severity | Security | Standards | Total |
|---|---|---|---|
| 🔴 High | 0 | 0 | **0** |
| 🟡 Medium | 6 | 5 | **11** |
| 🟢 Low | 6 | 5 | **11** |
| Informational | 3 | — | 3 |

Suites: **all green.** Go 17/17 packages, ingress module pass, web 85/85,
firewall `test.sh` 125/125, `integration.sh` 79 pass / 0 fail / 4 skip.
Coverage is where the FAIL lines are (below).

## 🟡 Medium — security

| Severity | File | Line | Issue | Suggested fix |
|---|---|---|---|---|
| 🟡 | `internal/history/history.go`, `internal/files/files.go` | 48,144,215 / 181,416,450 | **Guest-plantable FIFO wedges a server goroutine and pins an OS thread — cross-tenant DoS.** `files.openReg`/`openRegWrite` pass `O_NONBLOCK` and their comments say exactly why. Six sibling opens on the same guest-writable trees do not. `history.searchOne`'s `IsRegular` check sits *after* the blocking open, so it never runs. **Impact is cross-tenant:** each blocked request pins a goroutine AND an OS thread, walking the runtime toward its 10k-thread limit — process death takes every tenant with it | Add `O_NONBLOCK` to the six siblings; move `IsRegular` before the open |
| 🟡 | `deploy/update/styrman-update` | 158-181 | Asserts `${DEPLOY_DIR}/.git` and `deploy/.env` are root-owned, **never the worktree root itself**. Its own comment states the threat — "`-c safe.directory` on a tree someone else can write would let them choose the code this script builds as root" — then checks only `.git`. git's ownership guard covers the repository, not the checked-out files `docker build` reads | Assert `${DEPLOY_DIR}` ownership and mode alongside `.git` |
| 🟡 | `go.mod` / `deploy/caddy/ingress/go.mod` | 7 / 6 | The two modules pin **different** `docker/docker` versions (`v28.5.2` vs `v27.5.1`), and the older is in the internet-facing ingress running `network_mode: host` with the docker socket mounted | Converge both; add a check that fails when the modules disagree on a shared dependency |
| 🟡 | `internal/httpapi/credharvest.go` | 26-142 | Harvest runs `gh auth token`, `sed ~/.npmrc`, `base64 <path>` **inside an untrusted guest** and stores the result as the user's canonical credential, then pushes it to *every* running project of that user. A prompt-injected agent in project A can poison B, C, D. Within one tenant, not across — hence medium | Name the source project in the import UI; diff against the stored value and confirm on change |
| 🟡 | `internal/httpapi/server.go` | 846-895 | No per-user rate limit on project creation or credential import; only the credential push is single-flighted. An authorized user can drive unbounded microVM creation. Same class BUG-015 fixed for the LAN kick | Cap concurrent boots per user; single-flight `importCredentials` |
| 🟡 | `deploy/caddy/ingress/labels.go` | 129-144 | Emails from container labels and `SUPERADMINS` are written **verbatim** into the generated Caddyfile. `sanitizeHostname` strips Caddyfile-significant bytes from the hostname for exactly this reason; the email path has no equivalent. Operator-controlled today | Validate against the email shape `shares.go:23` already uses; drop failures |

## 🟡 Medium — standards

| Severity | File | Line | Issue | Suggested fix |
|---|---|---|---|---|
| 🟡 | `deploy/firewall/styrman-firewall.sh` | 588 | The scan prefilter is the hard-coded literal `*'--comment "styrman-lan'*`, not derived from `FAMILY_TAGS`. All three tags happen to share that prefix. **A fourth family tagged otherwise would be inserted and never revoked** — `scan_current` is the sole producer of `PURGE` — while `count_applied`, which has no prefilter, still reports it applied. Loosen-without-tighten, latent | Derive the prefilter from `FAMILY_TAGS`, or drop it and rely on the regexes |
| 🟡 | `internal/files/files.go` | 53-80 | The tenancy boundary of package `files` is **process-global mutable state**, installed as a side effect by `httpapi.New`. `openRoot` falls back to bare `os.OpenRoot` — pre-BUG-002 behaviour that follows a guest-plantable symlink — for any unregistered root, silently and with no error. Latent: nothing reaches `files.*` before `httpapi.New` today | Make an unregistered root an error, not a silent fallback |
| 🟡 | `deploy/firewall/install.sh` | 345 | `iptables -S STYRMAN-FWD \| head -12` under `set -euo pipefail`: SIGPIPE on a host with >12 rules fails the installer **after every step succeeded**, and `host-setup.sh:215` reports that as "projects stay isolated only if a previous script is installed". A host with LAN allowances is exactly a host with >12 rules | `\|\| true` on the display pipeline |
| 🟡 | `.agents/docs/specs/styrman/api.md` | — | A release behind: the six ADR-066 fields shipped in v1.24.0 (`allowHost`, `allowPublishedPorts`, `hostAddrs`, `pubSubnets`, `ingressBridge`, `credentialImpact`) appear zero times. `web/src/lib/api.ts` already types them — the spec is the only wrong copy | `documentation` agent |
| 🟡 | `internal/httpapi` | — | 10,288 lines / 33 files; `Server` has 25 fields and 5 mutexes across 7 subsystems; `explorer.go` alone holds files, history, settings, runtime, credential import, user admin and notifications | Split along the existing seams; not urgent, but it is why every review of this package is shallow |

## 🟢 Low (11)

Detail in the two review artifacts. The ones worth knowing:

- `internal/httpapi/git.go:447-450` — `git diff --no-index` takes git out of repository mode, so `?path=../../../etc/shadow` reads an arbitrary guest-readable file. Requester is the project owner, who already has a root shell in that microVM — confused-deputy shape, no boundary crossed, cheap to close.
- `internal/files/files.go:151-170` — `ContainedPath` resolves with `EvalSymlinks` then compares by **string prefix**: check-then-use, and the doc comment says so. Its only caller is the legacy ADR-027 migration, which since ADR-064 runs where no guest can race it.
- Symlink confinement is implemented **four different ways** across `internal/files`, `orchestrator`'s `realBelow` family and two inline `os.OpenRoot` calls in `main.go`. Each correct; none shared.
- `writeJSON` is duplicated with different signatures in `httpapi` and `credbroker` — the same shape as the ssh-predicate duplication the release gate caught.

## Coverage — FAIL lines

Per `AGENTS.md`, a source file with no covering test is a FAIL regardless of
suite results. Ordered by risk:

| # | Surface | Why it ranks here |
|---|---|---|
| 0 | `deploy/update/styrman-update` | **695 lines of root-run, self-replacing deploy/rollback code with zero automated tests**, where its sibling `deploy/firewall/test.sh` has 125 checks. The security pass read it in full and found it unusually well hardened — annotated-tag requirement, SSH-signature-only, last-armor-header defence, root-owned `allowed_signers` assertion — which makes the absence of a regression harness the risk, not the code |
| 1 | `internal/httpapi/stats.go`, `projectenv.go`, `proxy.go` | Live request paths; `proxy.go` is the port-preview surface |
| 2 | `internal/config` path helpers, `cmd/styrman` `transcriptJanitor` | Path construction feeding confinement decisions |
| 3 | `web/src/components/` and `routes/` — **zero `.test.tsx` anywhere** | Covered only by e2e; `lib/terminal.ts`, `lib/hooks.ts` and most of `lib/api.ts` likewise |

## Documentation accuracy bug

`.agents/docs/testing/styrman/README.md` states `integration.sh` is "82
checks". Two independent counts of the same run give **83** (79 pass + 4
skip). The README is the declared source of truth for these counts, so it is
wrong, not the suite. Introduced today: the ADR-066 round-1 fixes added one
check and only the firewall count was updated.

## What was NOT examined

- Browser e2e was deliberately not re-run (last green 89/0/2).
- The explorer did not sweep `internal/{files,conversation,links,history,userstore}` or the 11,402-line `web/` tree for dead code.
- The security pass names its own largest gaps: `deploy/caddy/ingress/watcher.go` and `cloudflare.go` (never test-run, and uncovered by the root `go test ./...` because the ingress is a separate module), most of `web/`, and `internal/orchestrator`'s destroy/cleanup paths.
- No functional test of the ingress against a live Caddy; no fuzzing; no dependency review beyond `govulncheck` and `npm audit`.
- `govulncheck` reports both modules importing `docker/docker` with GO-2026-4887 and GO-2026-4883 at **`Fixed in: N/A`** — no upgrade exists. Daemon-side Moby defects reached through `init` traces of the client SDK; standing exposure, not actionable today beyond the version convergence above.

## Recommendation

Nothing here blocks anything or needs a hotfix. The highest-value work, in
order: the six missing `O_NONBLOCK` opens (a documented guard that stops at a
boundary), a test harness for `styrman-update`, the firewall prefilter trap,
and `files.Trust`'s silent fallback. The first and third are small; the
second is the one that would have caught the most, for the longest.

---

## Dispositions (2026-09-08)

Fixed, each with a test that fails without the fix (commits `271a7e8`,
`00dd134`, `80765ca`, plus `05db066` for the spec):

| Finding | Sev | Disposition |
|---|---|---|
| FIFO wedges a goroutine + OS thread (6 opens) | 🟡 | **Fixed** — `O_NONBLOCK` on all six, `searchOne` Lstats before opening. `internal/history/fifo_test.go` plants real FIFOs |
| Firewall prefilter hard-coded, not from `FAMILY_TAGS` | 🟡 | **Fixed** — `is_ours()` derived from the table; `test.sh` §23c |
| `install.sh` SIGPIPE fails the installer after success | 🟡 | **Fixed** — `\|\| true` on the display pipeline |
| `styrman-update` never asserts the worktree root | 🟡 | **Fixed** — `trusted_dir "${DEPLOY_DIR}"` before `.git` |
| Ingress writes emails verbatim into the Caddyfile | 🟡 | **Fixed** — shape validation excluding Caddyfile-significant bytes |
| `docker/docker` diverges between modules | 🟡 | **Fixed** — both on `v28.5.2`; `verify-commit.sh` now blocks on it |
| `api.md` a release behind | 🟡 | **Fixed** — `05db066` |
| `files.Trust` silent fallback | 🟡 | **Partly** — warns once per root; deliberately NOT an error, reasoned in the code |
| `git diff --no-index` path traversal | 🟢 | **Fixed** — routed through `scopeRel` |
| `LanChecker` contract describes the superseded rule | 🟢 | **Fixed** |
| No cross-language bridge-name assertion | 🟢 | **False finding** — both sides assert the same golden vector for the same uuid; the agent had not run the shell suite |
| README integration count | — | **Fixed** — 82 → 83 |

Not fixed, with reasons — these are judgements, not omissions:

| Finding | Sev | Why not |
|---|---|---|
| `internal/httpapi` is 7 subsystems in one package | 🟡 | A refactor of 10,288 lines across 33 files, not a fix. Needs its own plan and ADR; doing it inside an audit pass is how the release gate's defects happened |
| Credential harvest poisons sibling projects | 🟡 | The remediation is a UX decision about the import flow (name the source, diff and confirm on change). Worth doing; it is a design change, and the operator should choose the shape |
| No per-user rate limit on project creation | 🟡 | `REQUIREMENTS.md` puts abuse hardening out of scope, and the audit itself records it as a documented accepted risk. Single-flighting `importCredentials` is the cheap half and is worth picking up |
| `api.ts` returns `body as T` with no runtime validation | 🟢 | ~40 endpoints; adopting a validator is a design decision |
| `ContainedPath` TOCTOU | 🟢 | Its only caller is the legacy ADR-027 migration, which since ADR-064 runs where no guest can race it. Documented in place |
| Symlink guard implemented four ways | 🟢 | The audit itself says "nothing urgent". Unifying it touches every confinement site — the highest-risk refactor in the repo to do casually |
| SPA catch-all / `allUsers` / `credentialContent` | 🟢 | The security pass marked the first "none required"; the others are informational-adjacent and unchanged by this pass |
| `styrman-update` has no test harness | — | **Partly** — it now has its first automated test (the ordering check covers it), but not a real harness. That remains the largest untested surface and is its own task |
| `web/src/components` + `routes` have zero `.test.tsx` | — | Large, and e2e covers the paths that matter today |

Suites after the fixes: `deploy/firewall/test.sh` **128/0** (was 125), go
`vet`/`test` clean both modules, web 85/85, `verify-commit.sh` green on
`80765ca` including the new dependency guard.
