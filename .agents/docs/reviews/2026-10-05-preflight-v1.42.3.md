---
owner: main
date: 2026-10-05
---
# Preflight — v1.42.3   (2026-10-05)

Range reviewed: v1.42.2..2829e51

Operator, this session: "go v1.42.3" (2026-10-05).

Version: **v1.42.3**, a PATCH. It bounds the editor's file endpoints, tests the
slot deadlines on a real server, closes token and `.env` lows, and makes the e2e
harness production-safe.

## Findings that do not block (FAIL-first: nothing is failing)

| # | Severity / confidence | Where | Finding | Decision |
|---|---|---|---|---|
| 1 | medium / medium | `internal/httpapi/git.go` (diff, log, show), file and transcript listings, multipart upload | **Not every memory-heavy endpoint is bounded.** The v1.42.3 work entry's claim was false. Git output is up to 2 MiB of guest-controlled text (about 12–16 MiB JSON), with no slot and no deadline: the same tenant crash-loop class under the 4g cap, open since v1.42.1. (security v1.42.2..a265b48, finding 2) | left: **v1.42.4**, one central byte budget for building JSON responses and decoding bodies instead of more one-by-one slots |
| 2 | medium / high | `explorer.go` `acquireSlot`, editor endpoints | Slots are server-wide: one tenant can keep the editor's 8 busy (slow `/check`s, or bodies re-sent every 30 s) and other tenants get a retryable 429. 2829e51 bounds a trickled body to 30 s; it is a bounded denial, not a crash. (security finding 1; review a265b48..2829e51, finding 1) | left: v1.42.4 (per-tenant accounting, with the history fairness items) |
| 3 | low | `explorer.go` | An 8 MB body over an uplink below about 2.2 Mbit/s hits the 30 s read deadline with a 400 (should be 408). `api.md`'s GET `/file` row mentions a body deadline. `/format` and `/check` have no deadline test of their own. (review a265b48..2829e51) | left: v1.42.4 |
| 4 | low | token tooling | `umask 077 cat >` does not fix an existing file's mode, and the paste echoes; `styrman-update` and `styrman-ctl` do not refuse an empty token file. A failed host-setup run leaves `.env.tmp.<pid>`. (security finding 3; review v1.42.2..a265b48) | left: v1.42.4 |
| 6 | low | `explorer.go:487-489`, `write_deadline_test.go` | The read deadline also applies to GETs without a body: harmless while the write deadline equals it, but a later longer write deadline would cancel a reused connection's next request and leak an i/o-timeout text. Set the read deadline only on requests with a body. The test does not assert the slot stayed held until about the deadline. (security addendum a265b48..2829e51) | left: v1.42.4 |
| 5 | low | e2e | Screenshot dirs pile up on tmpfs `/tmp`; clearing or closing the history panel does not abort a running search. (review v1.42.2..a265b48) | left: v1.42.4 |

## Step 2 — reviews and verification

**Reviewer — met.**
- `reviews/2026-10-05-review-v1.42.2-a265b48.md`: APPROVE WITH NITS. Finding 1 (no read deadline) fixed in 2829e51; finding 2 (deploy.md token scope) fixed; Outcome recorded.
- `reviews/2026-10-05-review-a265b48-2829e51.md`: APPROVE WITH NITS (rows 2–3; Outcome).

**Security — met.**
- `reviews/2026-10-05-security-v1.42.3-v1.42.2-a265b48.md`: COMPLETE, 2 medium, no high. Every path was read; the dist bundles are covered by `verify-commit.sh`'s per-file sha256 rebuild check (Outcome recorded).
- `reviews/2026-10-05-security-v1.42.3-addendum-a265b48-2829e51.md`: COMPLETE, 1 medium (row 2, the per-user share), 2 low (row 6). Every path was read, nothing is under Not examined, and `run.mjs` differs by one comment line.

**Verification — met.**
1. `testing/2026-10-05-verify-v1.42.3-full-r2.md`: PASS at HEAD 2829e51.
   - `verify-commit.sh 2829e51` exited 0 ("safe to tag"; the web bundle rebuilt and fresh).
   - `kata-config-test` 39/0; `docker-oom-test` 20/0.
   - vulncheck rc 3 with only GO-2026-4887/4883.
   - Tier 2 `passed=146 failed=0 skipped=3`.
   - e2e run **twice**, both `passed=101 failed=0 skipped=2`.
   - The harness guards exit 2 as designed. After both runs only production's network `sdb-0e5de8e8…` remains.
2. What the range changed was run: the Go suites (inside `verify-commit.sh`, including both real-server deadline tests), the web suites and bundle, `run.sh`/`run.mjs` (edited; run twice), and `host-setup.sh` (bash -n and shellcheck inside `verify-commit.sh`; its root run is not exercised here).
3. `verify-v1.42.3-full.md` is IN PROGRESS: a run stopped by the main agent before the tree changed. It covers nothing and shows no failing command.

## Step 3 — trail freshness
- The newest `changes/` entries are this release's work (1637, 1643; the release entry is in this commit). Pass.
- No future dates, no deleted logs, `DECISIONS.md` untouched, no shadowing agents. Pass.

## What was not checked
- The remaining unbounded endpoints (row 1). This is a known, open crash-loop path for a tenant under the 4g cap, unchanged by this release.
- `host-setup.sh`'s `.env` and empty-token paths on a real root run (fresh-host path unexercised).

VERDICT: SHIP
