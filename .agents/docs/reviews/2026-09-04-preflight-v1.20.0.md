---
owner: main
date: 2026-09-04
---
# Preflight — v1.20.0 (2026-09-04)

Range reviewed: v1.19.1..f7b2647 (the release commit that carries this report and the release changelog entry is docs-only on top of f7b2647; `deploy/verify-commit.sh` is run again on the tagged commit before the tag, result in the tag message and the session report).

## VERDICT: SHIP

Nothing blocks. What was broken during the gate, in the order it was found, and its state now:

1. **[high / confidence: high] FIXED — the browser e2e failed 21 of 63 checks** (`testing/2026-09-04-verify-v1.20.0.md`): every project listing came back empty. BUG-004: below a `files.Trust` base a root is reached through `Root.OpenRoot`, and under Go 1.25 a sub-root's `DirEntry.Info()` fails; `files.List` and `history.List` skipped every entry. Fixed with `rt.Lstat` through the root, two unit tests below a trusted base, and `integration.sh` listing checks that assert a real entry (the old `grep -q entries` passed on an empty list). Re-verified: integration passed=62 failed=0 skipped=3 aborted=0; e2e passed=61 failed=0 skipped=2 (`testing/2026-09-04-verify-v1.20.0-round2.md`). Commit f7b2647.
2. **[medium / high] FIXED — two raw `os.MkdirAll`s on guest-plantable paths** (release reviewer, `reviews/2026-09-04-review-v1.20.0.md`): the project-home creation in the orchestrator and `httpapi.createProject`, and the workspace mount target inside the home, now go through an `os.Root` (`mkdirAllConfined`, `files.MkdirAllPerm`). Commit f7b2647.
3. **[medium / high] FIXED — `internal/history` opened project-home roots with raw `os.OpenRoot`**, bypassing `files.Trust`; now `files.OpenRoot`. Commit f7b2647.
4. **[medium / medium] FIXED — `integration.sh` could abort under `set -e` and exit 0**; a missing ready line is tolerated and an abort now exits 1 (`aborted=` in the summary). Commit f7b2647.
5. **[medium / high] ACCEPTED, residual stated — the check-to-Docker window** (`reviews/2026-09-04-security-v1.20.0.md`, "Blocking: no"): `projectHomeIsReal` gates every create and start, but the `Lstat` walk and Docker's own resolution are not atomic; a tenant looping a swap from a running setup VM while stop/starting could still land a host path as `$HOME`. Strictly narrower than v1.19.1, where no check existed at all. Durable fix: the un-nesting ADR proposal in `TODO.md`. The Phase 3 security review did not converge in three rounds (`reviews/2026-09-03-security-harness-phase3.md`); the operator chose this gate's fresh pass ("go preflight"), which found no new blocking issue.
6. **[medium / high] FIXED before commit — ADR-063 claimed a redeploy rotates sessions** (`reviews/2026-09-04-security-session-lifetime.md`): with `STYRMAN_JWT_KEY` pinned they survive; the text now says so and that rotating the key is the only force-expire lever. Lows recorded in `TODO.md` (WebSocket handshake-only auth, no `caddy validate` gate, substring assertion, accepted lows from the phase reviews, the upstream `sh-agent` items).
7. **[low / high] FIXED — README "Status" named v1.18.1 as deployed**; the `documentation` agent brought it to v1.20.0 with the release bullet.

## Rows

- **Reviewer:** `reviews/2026-09-04-review-v1.20.0.md` (opus) over the source commits no earlier report covered — approve with nits, all fixed above; earlier per-phase reviews `reviews/2026-09-03-review-harness-phase{2,3}.md`. BUG-001's 47-line source diff was read by the author (below threshold), recorded in its changelog.
- **Security:** `reviews/2026-09-04-security-v1.20.0.md` (consolidated, non-blocking) plus the per-surface reports `2026-09-03-security-fail-to-fetch.md` (2 rounds), `…-harness-phase1.md` (2), `…-harness-phase2.md` (2), `…-harness-phase3.md` (3, not converged — see 5), `2026-09-04-security-session-lifetime.md`. `deploy/vulncheck.sh`: only the accepted docker/docker GO-2026-4887/4883 pair, `image/harness` clean; `npm audit`: 0.
- **Verification:** `testing/2026-09-04-verify-v1.20.0.md` (round 1: unit/vet/gofmt, frontend 17/17, vulncheck, `verify-commit.sh d24fe40` OK, integration 62/0/3 — then the e2e FAIL) and `…-round2.md` (PASS after the fixes). `deploy/verify-commit.sh f7b2647` run after the fixes (`.agents/scratch/verify-commit-f7b2647.log`; the result line is copied into the tag message). Production containers verified untouched before and after every suite.

## Trail freshness

- Newest `changes/` entry describes the release work: pass (`2026-09-04-v1.20.0-gate-fixes.md`, then the release entry).
- No future `date:` under `.agents/docs`: pass.
- `changes/` and `DECISIONS.md` append-only since v1.19.1: pass (0 deletions; the "modified" hits are appends — ADR-062, its amendment, ADR-063).
- `ROADMAP.md` and `TODO.md` touched since base: yes (6 commits).

## Docs freshness

`README.md` (Status → v1.20.0), `.agents/docs/specs/styrman/{runtime-image,versioning,deploy}.md`, `image/README.md`, `image/harness/VENDORED.md`, `.agents/docs/testing/styrman/README.md`, `.agents/docs/deploy/*.md` cold-read by the `documentation` agent against the code (2026-09-03 and 2026-09-04): pass. `REQUIREMENTS.md` R5 updated with the operator's approval (24490c9, ADR-062 amendment).

## Pre-gate commits on the branch

24490c9 (R5), d24fe40 (session lifetime), f7b2647 (gate fixes) — all pushed; the tag lands on the release commit above them.

## What was not checked

- Manual Tier 5 step 4 (`claude -p` / `agy` inside a fresh project with the operator's sign-in) — needs the operator, on the deployed version.
- The TOCTOU residual in 5 was not exercised adversarially against a running setup VM.
- The WebSocket handshake-only authorization low (`TODO.md`) — unchanged behaviour, wider window with eight-hour tokens.
- The deploy itself runs through the auto-updater from the signed tag; verification of the live revision (health, Settings version, a project boot on `styrman-runtime:1.20.0`) is done after the push and reported in the session, not here.
