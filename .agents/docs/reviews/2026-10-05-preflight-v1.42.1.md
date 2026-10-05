---
owner: main
date: 2026-10-05
---
# Preflight — v1.42.1   (2026-10-05)

Range reviewed: v1.42.0..4001374

Operator, this session: "go v1.42.1"; "ok go"; at the OOM re-design choice
(A: ship without the OOM change; B: the full fix now), "B go" (2026-10-05).

Version: **v1.42.1**, a PATCH. It fixes released behaviour: v1.42.0's OOM order and
docs, plus review follow-ups. Nothing is added for users; the compose memory caps
and the dockerd drop-in change how hosts run.

## Findings that do not block (FAIL-first: nothing is failing)

| # | Severity / confidence | Where | Finding | Decision |
|---|---|---|---|---|
| 1 | medium / medium | `deploy/docker-compose.yml` (server `mem_limit: 4g`), `internal/history` | The cap lowers the bar for a tenant to crash-loop the server: about 100–250 concurrent history searches over planted 4 MiB transcripts pass 4 GiB, the server is OOM-killed and restarts, and every tenant's sessions drop. Without the cap the same attack needs host-wide memory. (security 2ba3f79..aa2f3b4, finding 1) | left: **v1.42.2** (a search concurrency limit with 429, a streaming scan, `GOMEMLIMIT`) |
| 2 | medium / high | `internal/gitrepo/gitrepo.go:65` | No test reaches the child `setOOMScoreAdj(…, 0)` call: deleting it keeps `oom_test.go` green, which tests the helper only. ADR-085's and the testing README's coverage wording overstates this. (review 2ba3f79..aa2f3b4, finding 1; security, low 3) | left: **v1.42.2** (a fake `git` on PATH printing its own score after the test raises its score) |
| 3 | low / medium | `remote.md:80-88`, `deploy.md:254-263,340-344`, `host-setup.sh:307` | `sudo --preserve-env=GH_TOKEN` may still land in sudo's log as `ENV=GH_TOKEN=…`. That is read from sudo's source, not run. (security finding 2) | left: v1.42.2 (token file / stdin only) |
| 4 | low / high | `orchestrator.go:1683-1686` | The reaper's out-of-time return and log are untested. (review finding 2) | left: v1.42.2 |
| 5 | low / medium | post-release check | `/proc/<dockerd>/oom_score_adj` reads -999 even if systemd never loaded the drop-in, because host-setup writes it directly. Use `systemctl show docker -p OOMScoreAdjust -p StartLimitIntervalUSec`. (review finding 3) | the release notes say so |
| 6 | low | `run.mjs`; ADR-085's Rejected line | The alert printer reads the main page only; ADR-085's reason for not raising the VMs' score rests on a premise the security review disproved. (review lows) | left: v1.42.2 |
| 8 | low / high | `DECISIONS.md:2855` | ADR-084 still reads "Accepted" with no "Superseded by ADR-085" marker. (security addendum, finding 2) | left: v1.42.2 (mark it; ADR-085 already says it supersedes) |
| 9 | low / medium | `gitrepo.go:59-65` | A failed reset of git's OOM score is dropped without a log. (security addendum, finding 3) | left: v1.42.2, with row 2 |
| 7 | — | BUG-091 (open) | The intermittent e2e git-pane 409 failed 3 of 5 runs at 2ba3f79/aa2f3b4 and never reproduced by hand. 4001374's fixture now proves the identity and logs `DIAG BUG-091` when it retries; the release run showed no DIAG and no failure. | open, with instrumentation in place |

The follow-ups go in the handoff's Next Steps (no plan is open).

## Step 2 — reviews and verification

**Reviewer — met.**
- `reviews/2026-10-05-review-v1.42.0-2ba3f79.md`: COMPLETE, REQUEST CHANGES, answered by aa2f3b4 (its Outcome).
- `reviews/2026-10-05-review-2ba3f79-aa2f3b4.md`: COMPLETE, **APPROVE WITH NITS** (rows 2, 4, 5, 6).
- 4001374 is a test fixture only (`web/e2e/run.mjs`, +20 lines, one file), below the reviewer threshold. I read it: the check waits for the shell's OUTPUT `idok-e2e`, which the echoed command text `idok-$(` cannot match.

**Security — met.**
- `reviews/2026-10-05-security-v1.42.1-v1.42.0-2ba3f79.md`: COMPLETE. Its high (-1000 inherited by git, a host hang) and medium (dockerd at -500) are fixed in aa2f3b4 (Outcome). Every v1.42.0..2ba3f79 path was read; its Not-examined items are traces and host measurements, not diff paths.
- `reviews/2026-10-05-security-v1.42.1-2ba3f79-aa2f3b4.md`: COMPLETE. The high is fixed in principle (live after deploy); rows 1–3 come from it. Its Not examined names DECISIONS.md and 4001374's files, which are covered by:
- `reviews/2026-10-05-security-v1.42.1-addendum-decisions-4001374.md`: COMPLETE, lows only. ADR-084 and ADR-085, the `run.mjs` hunk and the three trail files were read end to end: no secrets, credentials, tenant data or instruction-shaped text. Lows: ADR-084 carries no "Superseded by ADR-085" marker; the reset write's failure is not logged (rows 8–9).

**Verification — met.**
1. `testing/2026-10-05-verify-v1.42.1-full-r3.md`: PASS at HEAD 4001374.
   - `verify-commit.sh 4001374` exited 0 ("safe to tag", web freshness checked).
   - `kata-config-test` 39/0; `docker-oom-test` 20/0.
   - vulncheck rc 3 with only the accepted GO-2026-4887/4883.
   - Tier 2 `passed=146 failed=0 skipped=3`.
   - e2e `passed=101 failed=0 skipped=2`, with no DIAG and no FAIL.
2. What the release changed was run: compose (`docker compose config`, shown in `commit-gate-r2`: -999 and both caps), the new shell lib test, `integration.sh` (edited), `run.mjs` (edited), and the Go suites inside `verify-commit.sh`.
3. Failures cleared:
   - `verify-v1.42.1-tiers.md` FAIL (e2e git 409s at 2ba3f79) and `verify-v1.42.1-tiers-r2.md` FAIL (the same at aa2f3b4) are cleared by `full-r3` PASS, which reran both tiers. The fix is the fixture's self-check (4001374), and the cause is recorded as BUG-091 (open).
   - `verify-v1.42.1-commit-gate.md` and `-r2` PASS; their ranges stop at 2ba3f79 and aa2f3b4, superseded by `full-r3`.

## Step 3 — trail freshness
- The newest `changes/` entries are this release's work (1433, 1455; the release entry is in this commit). Pass.
- No future dates, and no deleted verification log. Pass.
- `DECISIONS.md` was appended only (ADR-084, ADR-085). Pass.
- No shadowing agents. Pass.
- ROADMAP is untouched since v1.42.0 (no plan open). Report only.

## Step 4 — docs freshness
v1.42.0's six docs-lens items are fixed in 2ba3f79. No docs lens was run over this range; the changed docs (token lines, the OOM paragraph, the testing README) were read by both reviewers.

## What was not checked
- **Fresh-host path unexercised:** `host-setup.sh` gained the dockerd drop-in; there is no new GCP VM, and dave has no gcloud. The lib is covered by `docker-oom-test.sh` (20 checks on a scratch dir).
- `host-setup.sh`'s own root run (post-release, operator) and the live OOM order: it is not provoked, and goes live with the deploy plus the re-run.
- BUG-091's cause.
- The git child reset's call site (row 2).

None of these is a blocking condition: no verification item is unmet, no high security finding is open, no open REQUEST CHANGES, no REQUIREMENTS.md violation, and no shadowing agent.

VERDICT: SHIP
