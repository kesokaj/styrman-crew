---
owner: main
date: 2026-09-13
---
# Preflight — v1.30.1 (2026-09-13)

Range reviewed: v1.30.0..48d930b (plus the release commit)

Release: **PATCH** — two fixes to released behaviour (the firewall script's
missing-`xt_physdev` state reaches its exit path and `styrman-ctl status`
reports it as DEGRADED; the guest entrypoint writes the harness stamp
without interpolating the version) and a developer-side helper
(`image/bump-pins.sh`) that adds nothing for a user of the released
artifact. The rest of the range is the operator's close-out: TODO and
ROADMAP emptied, ADR-070 with the reason for each struck item. Requested by
the operator: "I approve all your recommendations. If we pin stuff make it
easy to upgrade. Do not overcommit, it works. /preflight when done and push
commit to git and make sure the new versions runs and the documentation is
up to date."

## VERDICT: SHIP

Every tier is green on the release tree, the round-2 reviews converged on
three real items (all fixed below), and TODO/ROADMAP are empty by the
operator's rule.

## Findings (FAIL-first)

None blocking.

- **[medium / high, fixed in 48d930b]** The reviewer's and security
  reviewer's real findings on the first commit: turning the firewall's
  physdev echo into `warn()` made the oneshot exit 1 on a degraded host, and
  `deploy/firewall/install.sh` (`set -e`) would then have aborted BEFORE
  enabling the units — BUG-035's shape; the firewall test suite's bare
  `run` had the same abort. Both tolerate the exit now (the updater already
  did). `styrman-ctl status` read `enabled/failed` as OK; it is DEGRADED now
  (test added). The bump helper fed unvalidated publisher strings to `sed`;
  every value is shape-checked first, the rewrite goes through temp copies
  verified line by line and moved together, bare `FROM` lines get a digest,
  `npm`/`awk` are required tools, fetch failures reach the guard, and the
  trust model (TLS + publisher sums, no signature) is stated in the header.
  The integration stamp check could pass vacuously on an empty pair; it
  binds and requires a value now. README counts updated (95 checks / 92).
- **[medium / high, fixed after 48d930b]** Round-2 reviewer and security
  reviewer, converging: `test.sh`'s `run … || true` had made the firewall
  script's exit status unobservable in both directions (a script that
  always exited 1 would still have passed the suite while `styrman-ctl`
  reported DEGRADED everywhere) — `run()` now keeps the status in `RUNRC`
  (errexit-safe) and the suite asserts a clean run exits 0 and a refused
  DROP exits 1 (`PASS=26 FAIL=0`); the bump helper's `mktemp` in /tmp made
  the `mv` a copy that carried mode 600 onto the Dockerfiles — temp copies
  now sit beside the files with the originals' modes; the spec still said
  the build "proves the new sums" — corrected to the stated trust model;
  plus two lows: `FROM scratch`/`--platform` lines are skipped rather than
  bricking the run, a missing digest is "unavailable" in check mode and
  fatal only under `--apply` (Docker Hub's anonymous rate limit hit exactly
  this during the gate), and the digest rewrite is verified per file.
  This last delta (~70 lines: `test.sh`, `bump-pins.sh`, one doc paragraph)
  was read by the main thread and is pinned by the suite run and the check-
  mode run; no third review round.
- **[low / high, recorded]** The trust model of the pins is the publishers'
  own sums over TLS; Node's signed `SHASUMS256.txt.asc` is not verified and
  npm publishes no sum for Claude Code (version pin only). Stated in the
  helper and ADR-069's bump procedure; not changed.

## Review coverage

| Commits | Reviewer | Security | Verifier |
|---|---|---|---|
| d582c9b, 3d50c27 (v1.30.0 trail) | n/a (docs) | n/a | n/a |
| 336a5d1 (the two fixes + helper + tests) | `2026-09-13-review-closeout.md` (opus; REQUEST CHANGES: 3 medium + 5 low → fixed in 48d930b) | `2026-09-13-security-closeout.md` (2 medium + 3 low → fixed / recorded) | `2026-09-13-verify-v1.30.1.md` (PASS) |
| 441507e (ADR-070, TODO, ROADMAP, entry) | n/a (docs) | n/a | n/a |
| 48d930b (review round) | `…-review-closeout-round2.md` (APPROVE WITH NITS: 1 medium + 2 low → fixed) | `…-security-closeout-round2.md` (1 medium + 4 low → fixed / recorded) | extension to 48d930b (PASS) |
| the commit after 48d930b (round-2 fixes, ~70 lines) | read by the main thread (below the review threshold; the suite and the check-mode run are the evidence) | the security round-2 remediations verbatim | firewall suite `PASS=26 FAIL=0`, `bash -n` + `shellcheck` clean, bump helper check mode exit 0 (main thread) |

## Trail freshness

- Newest `changes/` entry `2026-09-13-todo-closeout.md` describes this
  release's work; the release entry is written after this verdict. PASS.
- No future `date:` under `.agents/docs`. PASS.
- Append-only: no modified past `changes/` entries in the range;
  `DECISIONS.md` +15/−0 (ADR-070 appended). PASS.
- `ROADMAP.md` "Now" is empty by the operator's rule; `TODO.md` Inbox is
  empty (441507e). PASS — the intended end state.

## Docs freshness

`specs/styrman/runtime-image.md` names the bump helper and its trust model;
`testing/styrman/README.md` carries the new integration count; `deploy/`
docs describe the updater gate from v1.30.0; nothing else changed for a
user. PASS.

## Verification (tiers)

- Unit/build/vet/fmt/shell/frontend: `testing/2026-09-13-verify-v1.30.1.md` (PASS
  at 336a5d1; extension to 48d930b dispatched).
- Image rebuilt from the close-out tree (`image/entrypoint.sh` changed);
  integration `passed=92 failed=0 skipped=3` rc=0 with the new
  "harness stamp equals the image version after boot" check PASS
  (`testing/2026-09-13-integration-v1.30.1.log.md`); e2e `passed=92 failed=0
  skipped=2` rc=0 (`testing/2026-09-13-e2e-v1.30.1.log.md`); the firewall script's
  own suite under `unshare -Urnm`: `PASS=26 FAIL=0`
  (`testing/2026-09-13-firewall-test-v1.30.1.log.md`, at 48d930b's `test.sh`).
  Integration on the final tree (48d930b, the bound stamp check):
  `passed=92 failed=0 skipped=3` rc=0, stamp check PASS
  (`testing/2026-09-13-integration-v1.30.1-final.log.md`).

## What was not checked

- A host without `xt_physdev` (the condition the firewall fix reports): no
  such host is available; the `warn()` path is pinned by the text test and
  the `styrman-ctl` verdict by its unit test.
- `bump-pins.sh --apply` on a real delta (everything is current today; the
  check mode ran and the rewrite path is unit-less — a shell script with
  shape checks, read by two reviewers).
- `deploy/verify-commit.sh` on the release commit and the updater's deploy:
  appended below once observed.

## Release commit gate (appended after the release commit)

`deploy/verify-commit.sh 5a4dcde` — first run 13:33 FAILED on Docker Hub's
anonymous pull rate limit (`429 Too Many Requests` resolving the pinned
`debian:trixie` base; BUG-056), not on the code; no tag was pushed. The
window returned at 14:03 (`ratelimit-remaining=100`); second run — exit 0:
"OK: commit 5a4dcdef0e76 builds and tests clean in isolation — safe to tag".
Log: `.agents/scratch/verify-commit-v1.30.1.log`. Tag `v1.30.1` signed on
5a4dcde.

## Deploy (appended once observed)

Tag pushed 14:04:40. Updater tick 14:07: "v1.30.1 signature OK (allowed
signer)"; containers recreated 14:08:07; "[styrman-update] OK — v1.30.1 is
live" 14:08:18. `styrman-ctl status` verdict OK at 14:08 (server healthy on
1.30.1, deployed sha 5a4dcde, firewall enabled/inactive + timer active);
in-container `/api/health` 200; the 1.30.1 runtime image answers
`go1.27.1`, Node `v22.23.2`, Claude Code `2.1.270`, `agy 1.2.2` and carries
the argument-form stamp write; the converged host firewall script carries
the `warn` form and its post-deploy run left the unit `inactive` (a good
oneshot run, not `failed`).
