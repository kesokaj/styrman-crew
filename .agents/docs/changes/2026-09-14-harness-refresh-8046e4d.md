---
owner: changelog
date: 2026-09-14
---
# Harness refresh to commit 8046e4d (2026-09-14)

## Summary

Vendored harness mirror (`image/harness/`, a plain .git-less copy) refreshed from styrman-harness commit `6893e2218414617da2bb88259b905ca3882b2659` to **`8046e4d5f24b009bd2461ebef28931cade7a4acd`** (the harness `main` on 2026-09-14; the floor was `2c8990e`, the harness's BUG-015 fix: `install` from a plain copy never rebuilds and never runs git). Recipe: `image/harness/VENDORED.md` § Refresh run verbatim (`rsync --delete`, COMMIT, SHA256SUMS regenerated over 154 files, `diff -rq` printed exactly the three expected `Only in image/harness` lines: COMMIT, SHA256SUMS, VENDORED.md). The harness gained upstream: `/review` skill (replaces `/audit`, folded into `/review --project --all`), gcloud-auth-verification folded into gcloud-setup, `sh-agent install --vendor all|claude|antigravity`, the plain-copy install path. Skills count 28 → 27; subagents remain 7. Crew-side edits: VENDORED.md release note restatement, entrypoint.sh install guard drop, integration.sh/testing README test count updates, README.md and specs updates. Go.mod unchanged; no image pins changed.

## Type

task

## Files changed

- `image/harness/` — 54 modified, 2 deleted (skills/audit/SKILL.md, skills/gcloud-auth-verification/SKILL.md), 5 untracked (cmd/sh-agent/main_test.go, skills/golang/references/{golang-samber.md, golang-structure.md}, skills/init-project/assets/agents-tree/docs/retros/, skills/review/)
- `image/harness/VENDORED.md` — release note now cites the COMMIT; "Known cosmetic gap" replaced with plain-copy note
- `image/entrypoint.sh` — comments updated; install line dropped GIT_CONFIG_GLOBAL guard (plain copy, never rebuilt)
- `.agents/docs/testing/styrman/integration.sh` — "28 skills" check → 27; two new checks for plain-copy repo labeling and boot log commit stamp
- `.agents/docs/testing/styrman/README.md` — check count 95 → 97; test run result `passed=94 failed=0 skipped=3`; harness section updated
- `README.md` — the v1.20.0 release-history line no longer states a skill count (the current roster is in `image/README.md`)
- `image/README.md` — skills count 27; plain-copy note added
- `.agents/docs/specs/styrman/runtime-image.md` — 27 skills; plain-copy note
- `deploy/verify-commit.sh` — "150-odd files" comment updated

## Agents dispatched

verifier, reviewer, security, reviewer (docs lens), bugs (BUG-057 grep-q/pipefail false FAIL in integration.sh; BUG-058 upstream TestDryRunNeverBuilds assumes checkout, not in project scope), changelog

## Verification

Observed by the main agent before the dispatch, each command bare with its exit status read (the tests row's own artifact is the verifier's log, `.agents/docs/testing/2026-09-14-verify-harness-refresh-8046e4d.md`; agent verdicts are recorded in the release gate report, not here): `go test ./...` rc 0 (15 packages ok); `(cd deploy/caddy/ingress && go test ./...)` rc 0; `go vet ./...` rc 0; `bash deploy/vulncheck.sh` rc 3 (GO-2026-4887/4883, the two docker/docker daemon-side findings the script header records as accepted per release, in the root and ingress modules; `image/harness` "No vulnerabilities found"); web `npm run check`, `npm run lint`, `npm test` each rc 0 (178 tests, 15 files); integration.sh first run failed (grep -q under pipefail exited 141 before sh-agent status finished); fixed and second run `passed=94 failed=0 skipped=3` rc 0; image checks: `sh-agent list` prints Skills (27)/Subagents (7); `install --dry-run` prints plain-copy message; `status` in fresh home prints plain-copy repo line and `drift none — 259 files match the repo`; the image's `/styrman/entrypoint.sh` has no GIT_CONFIG_GLOBAL on the install line.

Observed but not this project's: `(cd image/harness && go test ./...)` FAILs in `TestDryRunNeverBuilds` (cmd/sh-agent) because that upstream test assumes its own git checkout; the same test passes in `~/workspace/styrman-harness`; `deploy/verify-commit.sh` vets the mirror and never runs its tests (BUG-058, carried to the operator for the harness repo).

Release: ships as v1.31.0 (MINOR: adds user-visible behaviour — `/review`, `--vendor`, the rewritten context — and breaks nothing); the release has its own entry. The 8 git-tracked `.claude/agent-memory/**` files that mention `/audit` and the old harness are listed for the operator in the handoff, not deleted.

## Review rounds (recorded after the dispatch above; verdicts in the gate report)

- Round 1 — reviewer: APPROVE WITH NITS, two lows, both fixed: the new boot-log check compared the container's commit with itself and failed open on a failed inner exec (now reads the commit from the host file `${REPO}/image/harness/COMMIT`, length-checks both values); `VENDORED.md` § Refresh says the mirror's own `go test ./...` is red by construction (BUG-058); `SHA256SUMS` regenerated. Security: one medium (the harness's root-steps sentence in `context/dev-setup.md` is rendered into every project VM's AGENTS.md) rebutted and carried to the operator for the harness — inside a project microVM the user already holds `NOPASSWD: ALL` and the docker group by design, so the sentence changes when an agent asks, not what it can reach, and the mirror is verbatim (ADR-062); two lows on upstream path handling (`apply.go` manifest paths, frontmatter `name:`) rebutted for this project (same user, own home) and carried upstream; one low fixed at first by a `test ! -e /styrman/harness/.git` in `image/Dockerfile`.
- Round 2 — reviewer: both round-1 findings closed; noted that the round-1 verifier log and this entry predate the fixes (the suite was re-run: `passed=94 failed=0 skipped=3` on the rebuilt image, and the verifier's round-2 log records it). Security: every disposition holds; NEW low: `image/.dockerignore` (`.git`, `**/.git`) keeps a `.git` out of the build context, so the Dockerfile assertion could never fire — the verifier's round 2 proved it (a planted `harness/.git` still built, rc 0). The mechanism was DELETED rather than repaired: the Dockerfile comment names `.dockerignore` as the guarantee, `entrypoint.sh`'s comment says the same; image rebuilt, suite `passed=94 failed=0 skipped=3` again (12:33).
- Round 3 — security and verifier dispatched over the deletion; results in the gate report.

## Docs

Docs lens dispatched over the same tree. VENDORED.md, integration.sh, the testing README, runtime-image.md, image/README.md and README.md's v1.20.0 line updated with the refresh; entrypoint.sh's comments say why the 20 s cap holds (plain copy, never rebuilt, never a git call).

## Requirements

n/a
