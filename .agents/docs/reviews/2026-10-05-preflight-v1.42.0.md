---
owner: main
date: 2026-10-05
---
# Preflight — v1.42.0   (2026-10-05)

Range reviewed: v1.41.1..b1eabeb

Operator, this session: "crack a new version and push it to local and gcp";
"option 1 go"; "ok go ahead. asap"; at the security re-confirmation (option
1: ship now, OOM protection in v1.42.1; option 2: hold): "go all approved"
(2026-10-05), taken as option 1, the recommendation.

Version: **v1.42.0**, a MINOR. It adds behaviour (per-project resources) and
breaks no existing deployment step. Projects rebuild once on their next start
by design, and `host-setup.sh` needs a root re-run per host (below).

## Findings that do not block (FAIL-first: nothing is failing)

| # | Severity / confidence | Where | Finding | Decision |
|---|---|---|---|---|
| 1 | medium / high | `internal/orchestrator/resources.go`, host OOM settings | The OOM killer would pick `styrman-server` and Caddy (`oom_score_adj` 0) before any project VM (cloud-hypervisor at −999), so a whole-host-sized VM exhausting memory takes the platform down for every tenant. (security v1.41.1..f507278, finding 1) | left: operator chose ship now, fix in **v1.42.1** (`oom_score_adj` for server/Caddy in compose; ADR-083 corrected) |
| 2 | medium / high | `gcp.md:196`, `remote.md:30`, `specs/styrman/deploy.md:159` | The post-release step writes `sudo GH_TOKEN=… bash host-setup.sh`, which puts the broad token on the command line and in sudo's log. (security, finding 2) | left: docs fix in **v1.42.1**; the release notes give the token-free form (`/etc/styrman/gh-token` is read automatically) |
| 3 | low–medium | `internal/httpapi/resources.go` | Any owner may set the whole host and a 500 GB disk; there is no per-user budget or admin-only cap. (review finding 2; security lows) | left: operator, follow-up |
| 4 | medium (deferred) | `orchestrator.go` `ReapSetupVMs` | The reaper's zombie retry has no test of its own (the shared `forceRemove` is tested). (review f507278..b1eabeb, finding 1; security addendum, low 3) | deferred by the main agent, not the operator: the reviewer's own alternative; follow-up to extend the fake's `ContainerList` |
| 5 | low | `integration.sh:534-537` | The test-only host `truncate` follows a guest-plantable symlink in the project home. (security addendum, low 1) | left: follow-up (`truncate -c` plus a regular-file check) |
| 6 | low | `orchestrator.go:1684-1688` | The 1-minute reap bound is per VM, serial. (security addendum, low 2) | left: follow-up |
| 7 | low | e2e | The first Resources check was never forced red; the `finally` restore has not run in a failing state. (review round 2, finding 2) | left: follow-up |
| 8 | low | various | `zombieRetryEvery`'s comment; the one-shot remove after a failed stop on the pre-ADR-027 path (`:332`); the docs lens's six stale spots. | left: follow-up |

The follow-ups are listed in the plan's Scope § Follow-ups and the handoff.

## Step 2 — reviews and verification

**Reviewer — met.**
- `reviews/2026-10-05-review-project-resources.md` (58ddcd3~1..3b67939, COMPLETE, APPROVE WITH NITS): finding 1 fixed; 2 left: operator; 3 rebutted (D28).
- `reviews/2026-10-05-review-f507278.md` (3b67939..f507278, COMPLETE, REQUEST CHANGES): answered by 8a92b6a/b1eabeb.
- `reviews/2026-10-05-review-f507278-b1eabeb.md` (f507278..b1eabeb, COMPLETE): round 2 REQUEST CHANGES on the untested reaper; round 3 **APPROVE WITH NITS** after the deferral (row 4).

**Security — met.**
- `reviews/2026-10-05-security-v1.42.0-v1.41.1-f507278.md` (COMPLETE, 2 medium, no high): every release-diff path read. Its `## Not examined` names the hash-named `web/dist` bundles "not read line by line", proven equal to a rebuild of the commit's own source by `verify-commit.sh`'s per-file sha256 gate (run on f507278 and b1eabeb). That is the reading the dispatch defined for those files, so I judge the path read, not unexamined. Its other `Not examined` items are not release-diff paths: the 8a92b6a/b1eabeb edits (covered by the addendum), dave's token file, the GCP VM, the untracked `deploy/.env`, the journal, and OOM victim order under real pressure.
- `reviews/2026-10-05-security-v1.42.0-f507278-b1eabeb.md` (COMPLETE, lows only): all six diff paths read; nothing in `Not examined` is a diff path.
- The earlier `reviews/2026-10-05-security-project-resources.md` (58ddcd3~1..3b67939) carries its Outcome.

**Verification — met.**
1. Logs reaching HEAD (b1eabeb; only dated records change after it):
   - `testing/2026-10-05-verify-v1.42.0-commit-gate-r2.md` — PASS: `deploy/verify-commit.sh b1eabeb` exit 0, "safe to tag" (build, vet, `go test -race`, ingress module, gofmt, web/dist fresh, shellcheck, npm check/lint/test, server and runtime image builds); `kata-config-test.sh` 39/0; `vulncheck.sh` rc 3 with only the accepted GO-2026-4887/4883.
   - `testing/2026-10-05-verify-v1.42.0-tiers-r2.md` — Tier 2 `passed=146 failed=0 skipped=3` (README: 149 checks; the 3 SKIPs are the host-firewall preview checks), all resources checks including the grow-retry branch. Its e2e was `passed=100 failed=1` ("terminal session survived the editor round-trip", a 15 s timeout while an image build and a security measurement ran on the host).
   - `testing/2026-10-05-verify-v1.42.0-e2e-r3.md` — PASS: e2e rerun on a quiet host (load 1.36), `passed=101 failed=0 skipped=2`, with that check and both Resources checks passing.
2. What the release changed was run:
   - Commands: `verify-commit.sh`, `vulncheck.sh`, `kata-config-test.sh` (new), `integration.sh` (edited) and `web/e2e/run.mjs` (edited) are all shown run in the logs above.
   - Tiers named by the plan: unit, web, the host-setup shell test, Tier 2 and e2e are covered above. "host-setup on dave (root, operator)" is the post-release step and is not run (below).
3. Failures cleared:
   - `commit-gate.md` FAIL (vulncheck rc 3 only) is cleared by `vulncheck.md` PASS, which reran it and says "the earlier commit-gate FAIL was the dispatch's missing expectation for exit 3, not the code".
   - `tiers.md` BLOCKED (the tree moved during the run; both tiers had passed at f507278) is superseded by `tiers-r2.md` and `e2e-r3.md` at the later HEAD, which reran both tiers. It was not updated in place.
   - `tiers-r2.md`'s e2e FAIL is cleared by `e2e-r3.md` PASS, which reran it. No code changed between them; the cause is not proven, and host-load flakiness is consistent with the evidence.

## Step 3 — trail freshness
- The newest `changes/` entries are this release's work (phases 1–4); the release entry is in this commit. Pass.
- No future `date:` anywhere. Pass.
- No verification log deleted (`git log --diff-filter=D` on `testing/` is empty). Pass.
- `DECISIONS.md` was touched once, in 58ddcd3, as an append only (`+23 −0`); `changes/` has no modified entries. Pass.
- ROADMAP was touched (58ddcd3, "Now" names the plan). TODO.md is untouched and empty. Report only.
- No project agent shadows reviewer, security, verifier or explorer. Pass.

## Step 4 — docs freshness (advisory)
`reviews/2026-10-05-review-v1.41.1-f507278-docs.md` (APPROVE WITH NITS) found six stale spots, none blocking:
- `runtime-image.md:224,381`, medium: the disk env row and boot step 8 do not mention the grow;
- `run.mjs:1597`, low: fixed in 8a92b6a;
- `api.md:270`, low: `STYRMAN_DOCKER_DISK_GB` is now only the default for new projects;
- `testing/styrman/README.md:8`, low: the vitest count and the new Go test files;
- `deploy.md:143` vs `integration.sh:479`, low: `nproc` reads "setting + 1" while unlimited shows the host count, which is consistent (vCPUs clamp at the host).

Owner: `documentation`, as v1.42.1 follow-ups.

## What was not checked
- **Fresh-host path unexercised** (versioning.md step 0, `.agents/rules/shared-path-learnings.md`): `host-setup.sh` changed (the Kata wrapper and reclaim), and no new GCP VM was brought up; dave has no gcloud. The lib's behaviour is covered by `kata-config-test.sh` (39 checks on a scratch prefix).
- `host-setup.sh`'s own root run on any real host: that is the post-release step. Until it runs, dave keeps reclaim off, and the GCP VM likely runs QEMU, where full-size starts are unmeasured and may be refused.
- Kata sizing on QEMU: measured on Cloud Hypervisor only.
- The OOM victim order under real memory pressure: settings read, never provoked.
- The per-call-site tests for the reaper and the guest-changed/home-moved setup removals.

None of these is a blocking condition: no verification item is unmet, there is no high security finding, no open REQUEST CHANGES, no REQUIREMENTS.md violation, and no shadowing agent.

VERDICT: SHIP
