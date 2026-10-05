---
owner: main
date: 2026-10-05
---
# Preflight — v1.42.2   (2026-10-05)

Range reviewed: v1.42.1..12c573e

Operator, this session: "go v1.42.2" (2026-10-05).

Version: **v1.42.2**, a PATCH. It fixes released behaviour: v1.42.1's memory cap
had made a tenant crash-loop possible through history search and transcript
reads. It also adds tests and fixes the docs.

## Findings that do not block (FAIL-first: nothing is failing)

| # | Severity / confidence | Where | Finding | Decision |
|---|---|---|---|---|
| 1 | medium / medium | `internal/httpapi/explorer.go` `readFile`; PUT `/file`, `/format`, `/check` | The editor's file read (up to about 12 MiB of JSON) and 8 MiB request bodies are still unbounded and have no write deadline: about 200–250 never-read requests could push the server past its 4g cap. Live in v1.42.1 as well; v1.42.2 does not make it worse. (security addendum ef22f5c..f3a5f53, finding 1) | left: **v1.42.3**, one limiter for every heavy endpoint plus server timeouts in `cmd/styrman/main.go` |
| 2 | medium / medium | `explorer.go` `acquireSlot` | The 30 s write deadline has no test (httptest's recorder cannot take one), and `api.md` states it. A scratch probe showed it does not leak across keep-alive requests. (review ef04b30..f3a5f53, finding 1; security, low 3) | left: v1.42.3 (a real-server test) |
| 3 | low / high | `explorer.go` | The read and search slots are server-wide with no per-user share, so one tenant can make others get a retryable 429. A per-user cap was tried (c350683) and reverted (ef04b30): it broke a quick second search, because the panel does not abort its previous one. (reviews; security) | left: v1.42.3 (client abort plus server-side cancel) |
| 4 | low | deploy docs | Token snippet: a failed `shred` reports "no token installed"; the plaintext file's mode is not forced; only the doc refuses an empty file; host-setup step 8c's "ONE repository" wording; `.env` briefly world-readable at host-setup.sh:240-248 (older). (review; security addendum lows) | left: v1.42.3 |
| 6 | low / medium | `web/e2e/run.sh:87-95`, `run.mjs:16,56,1457-1486,17` | E2e harness safety, all from before this release (git blame: v1.0.0, v1.25.0, v1.41.0): the exit cleanup can delete networks created during the run; the default `--base` is production's own listener; the updater-status check writes and removes whatever path the inherited `STYRMAN_UPDATE_STATUS_FILE` names; screenshots go to a fixed `/tmp` path. (security run.mjs review) | left: v1.42.3 |
| 5 | — | BUG-091 (open) | Intermittent e2e git 409, consistent with the BUG-092 focus race; its fixture self-checks. | open |

## Step 2 — reviews and verification

**Reviewer — met.**
- `reviews/2026-10-05-review-v1.42.1-ef22f5c.md`: APPROVE WITH NITS (Outcome: fixed in c350683, partly reverted in ef04b30).
- `reviews/2026-10-05-review-ef22f5c-c350683.md`: APPROVE WITH NITS (Outcome: the per-user cap reverted; token line guarded).
- `reviews/2026-10-05-review-ef04b30-f3a5f53.md`: APPROVE WITH NITS (rows 2–4).
- 12c573e is a 10-line e2e edit, below the reviewer threshold; I read it. It waits for `document.activeElement` to be the xterm textarea, which still fails if focus never returns.

**Security — met.**
- `reviews/2026-10-05-security-v1.42.2-v1.42.1-ef22f5c.md`: COMPLETE, 2 medium, no high (Outcome). Its Not-examined six files were read in full by the next addendum.
- `reviews/2026-10-05-security-v1.42.2-addendum-ef22f5c-f3a5f53.md`: COMPLETE. All 16 changed files and the six named files were read; nothing blocks; row 1 and the lows come from it.
- `reviews/2026-10-05-security-v1.42.2-addendum-f3a5f53-12c573e.md`: COMPLETE, no new weakness. Its only Not-examined item, the rest of `run.mjs`, is covered by:
- `reviews/2026-10-05-security-v1.42.2-run-mjs-12c573e.md`: COMPLETE. All 2071 lines of `run.mjs` were read; nothing under Not examined names any part of it. Four lows, all from before this release (row 6).

**Verification — met.**
1. `testing/2026-10-05-verify-v1.42.2-full-r6.md`: PASS at HEAD 12c573e.
   - `verify-commit.sh 12c573e` exited 0 ("safe to tag").
   - `kata-config-test` 39/0; `docker-oom-test` 20/0.
   - vulncheck rc 3 with only GO-2026-4887/4883.
   - `docker compose config`: styrman has `GOMEMLIMIT` 3GiB, a 4g cap and -999.
   - Tier 2 `passed=146 failed=0 skipped=3`.
   - e2e run **twice**, both `passed=101 failed=0 skipped=2`, with the editor round-trip check passing each time.
2. What the range changed was run: compose, the Go suites (inside `verify-commit.sh`), `integration.sh` and `run.mjs` (edited).
3. Failures cleared:
   - `verify-v1.42.2-full-r4.md` FAIL and `verify-v1.42.2-e2e-r5.md` FAIL (both "terminal session survived the editor round-trip") are cleared by `full-r6`, which reran e2e twice. The fix is 12c573e: the check waits for the terminal's focus before typing (BUG-092, fixed). An A/B run at ef04b30 failed the same way, so f3a5f53 was not the cause.
   - `full.md`, `full-r2.md` and `full-r3.md` are IN PROGRESS: runs stopped by the main agent before the tree changed. They cover nothing and show no failing command.

## Step 3 — trail freshness
- The newest `changes/` entries are this release's work (1534, 1541, 1543, 1548; the release entry is in this commit). Pass.
- No future dates, and no deleted verification log. Pass.
- `DECISIONS.md`: an append (ADR-085 amendment 1) and ADR-084's "Superseded by ADR-085" marker, which the ADR rule allows. Pass.
- No shadowing agents. Pass.

## What was not checked
- The editor file endpoints' memory bound (row 1) and the write deadline's runtime behaviour under a real slow client (row 2).
- BUG-091's cause.

VERDICT: SHIP
