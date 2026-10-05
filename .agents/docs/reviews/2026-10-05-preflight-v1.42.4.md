---
owner: main
date: 2026-10-05
---
# Preflight — v1.42.4   (2026-10-05)

Range reviewed: v1.42.3..d4356c9

Operator, this session: "yes, admin cap, yes, yes — go v1.42.4"; "go all"
(scope); slot share "Half of each pool (Recommended)"; at the plan's
cold-read cap: "What the fuck are you doing. Why cant you cut a new
version/release?" (taken as go on D20-D22) (2026-10-05).

Version: **v1.42.4**, a PATCH as the operator named it. It bounds released
behaviour (memory, slots, body deadlines) and fixes BUG-074/091. The admin cap
for non-admin projects is a new setting; the operator named the version.

## Findings that do not block (FAIL-first: nothing is failing)

| # | Severity / confidence | Where | Finding | Decision |
|---|---|---|---|---|
| 1 | low / high | `internal/httpapi/auth.go` (`/api/health`) | The health exemption still reads the decoded path: `GET /api%2Fhealth` passes auth to the SPA catch-all, which answers 404 for any decoded `/api/` path. Nothing reachable. Predates the range. (security addendum3, finding 1) | left: accepted, named in the release entry |
| 2 | low / low | `internal/membudget` | The waiter queue is scanned under the mutex on every acquire/release; bounded by in-flight connections and the 2 s wait, ≤ 5 users. (security, finding 2) | left: accepted, named in the release entry |
| 3 | low / high | `format.go`, `check.go`; `deploy/update/styrman-update` | Not covered: the 408 call sites in format/check (they need a running VM before the body is read; the shared helper is tested through writeFile) and the updater's main-flow `token_file_ok` call (needs root; the function is tested through its hook). (review outcomes 5, 7) | left: accepted, named in the release entry |
| 5 | low / high | `deploy/verify-commit.sh` | The web freshness and web check steps are skipped silently when `SKIP_WEB_FRESHNESS` is set or npm is missing, and it still prints "safe to tag". Predates the range; this release's runs had npm and no skip (`.agents/scratch/verify-r4/vc2.txt:28-30`: "committed assets match a fresh build"). (security addendum 4, finding 1) | left: accepted, named in the release entry |
| 4 | low | `testing/…-verify-v1.42.4-full{,-r2,-r3}.md` | Three verifier logs are IN PROGRESS: runs stopped by the main agent before the tree changed (fixes followed). They cover nothing and show no failing command. | left |

## Step 2 — reviews and verification

**Reviewer — met.**
- `reviews/2026-10-05-review-v1.42.3-5a8afa1.md`: COMPLETE, REQUEST CHANGES (1 medium, 6 low). Answered in its Outcome (5 fixed with tests, each mutation-checked; 1 rebutted; 1 left) by be4c4fe and 07f5517.
- `reviews/2026-10-05-review-5a8afa1-07f5517.md`: COMPLETE, APPROVE WITH NITS (Outcome: both fixed, 676488c).
- `reviews/2026-10-05-review-07f5517-561c23b.md`: COMPLETE, APPROVE WITH NITS (Outcome: fixed in 6f72a82; one claim corrected; one left).
- `reviews/2026-10-05-review-561c23b-57de009.md`: COMPLETE, APPROVE WITH NITS (Outcome: fixed in 96c5f95 and the release entry; one left).
- 57de009..d4356c9 outside the trail is one test (+22) and comment-only edits in `auth.go`/`server.go`, below the reviewer threshold; I read it.

**Security — met.**
- `reviews/2026-10-05-security-v1.42.4-v1.42.3-5a8afa1.md`: COMPLETE, 1 medium (encoded `%2F` buys the upload cap) fixed in 07f5517 with ADR-086 amendment 1; 2 lows (Outcome). Its Not examined (trail files read partly, `web/dist/**`) is covered by addendum 4.
- `-addendum-5a8afa1-07f5517.md`: COMPLETE, 1 medium (tunnel exemptions by decoded path) fixed in 561c23b (amendment 2); 1 low left.
- `-addendum2-07f5517-561c23b.md`: COMPLETE, lows only; authGuard by route fixed in 57de009 (amendment 3).
- `-addendum3-561c23b-57de009.md`: COMPLETE, lows only (Outcome: 2 fixed, 1 left = row 1).
- `-addendum4-coverage.md`: COMPLETE, lows only. It read end to end the six trail files the first report read partly, the range 57de009..d4356c9, and covered `web/dist/**` by the rebuilt-from-source equivalence (`verify-commit.sh d4356c9`) plus a secret-shape grep (positive-controlled): nothing found. No release-diff path remains under any report's Not examined.
- No high finding in any report.

**Verification — met.**
1. `testing/2026-10-05-verify-v1.42.4-full-r4.md`: PASS, Range v1.42.3..d4356c9.
   - `verify-commit.sh 57de009` and `verify-commit.sh d4356c9` both exit 0 ("safe to tag"): both Go modules, gofmt, web dist freshness, web check/lint/test, server image build.
   - `kata-config-test` 39/0; `docker-oom-test` 20/0; vulncheck rc 3 with only GO-2026-4883/4887 (accepted).
   - Tier 2 `passed=146 failed=0 skipped=3` (the README's count line).
   - e2e twice, each `passed=101 failed=0 skipped=2`, no `DIAG BUG-091`, "screenshots removed (green run)".
   - Steps 3-8 ran on 57de009; after it only a test file and comment-only lines changed (the verifier's grep of non-comment lines printed nothing).
2. What the release changed was run: the Go suites, the ingress module and the web checks (inside `verify-commit.sh`), `integration.sh`, `run.sh`/`run.mjs` (edited), and the caddy image (`caddy adapt` of a generated config, a live strip request — main agent, recorded in ADR-088 and the BUG-074 record). The plan names unit, Tier 2 and e2e; all ran.
3. Failures cleared: none failed. The must-fail runs of BUG-091 (a: `failed=0` with DIAG; b: `failed=9`, all in the identity chain) were deliberate and recorded in BUG-091.

## Step 3 — trail freshness
- The newest `changes/` entry (1929, the work entry) describes this release's work; the release entry rides in this commit. Pass.
- No future dates; no deleted verification log; `changes/` untouched; `DECISIONS.md` appended only (ADR-086..088, ADR-086 amendments 1-3). Pass.
- No shadowing agents (`.claude/agents`, `.agents/agents` empty). Pass.
- ROADMAP "Now" names the plan, which this release closes.

## Step 4 — docs freshness
`reviews/2026-10-05-review-v1.42.3-d4356c9-docs.md`: COMPLETE, APPROVE WITH NITS (advisory). One medium (a stale `csrfGuard` comment in `web/src/lib/api.ts:455` and its test header) and lows (the web test count line, three doc mentions of the non-admin cap/empty token/2 MiB bound, the one-time sign-out in deploy docs). Owner: main agent, fixed in a docs/comment-only commit right after this tag (no behaviour change).

## What was not checked
- **Fresh-host path unexercised:** no `host-setup.sh` change; the updater's token check reaches every host at its next tick.
- A browser holding sessions for two deployments (BUG-074's own repro) — operator step on the GCP host after the release.
- A non-admin identity end to end in a browser (e2e and Tier 2 run as the AuthMode none admin); the cap is unit-tested with a non-admin user.
- Caddy's handling of `%2F` on the way in (the guards no longer depend on it).

VERDICT: SHIP
