---
owner: main
date: 2026-09-16
---
# Preflight — v1.35.0 (2026-09-16)

Range reviewed: v1.34.1..19d882c

## VERDICT: SHIP

No blocking finding. Every verifier row is PASS; every reviewer verdict on the release diff is APPROVE (one interim REQUEST CHANGES on a fix round, resolved in the next); no security finding above medium and every one fixed or rebutted in writing; no `REQUIREMENTS.md` violation (R5 was amended with the operator's approval in 567b81f and the code implements the amended text); no project agent shadows a harness review agent (`.claude/agents/` and `.agents/agents/` hold none of the four names). The last security row, over the fix rounds `5ba2b1b..edda5cd`, is recorded in "Review rows" below.

## Findings (FAIL-first)

None blocking. Findings raised during the gate and their disposition, worst first:

- [medium / high] `deploy/styrman-ctl` — the harness origin and clone path were operator-overridable outside the test seam and the origin reached a root `git clone` unvalidated (security, round 1). Fixed: both variables gated behind `STYRMAN_CTL_TEST`, the clone path a literal, the origin matched as a whole string against the plain-https pattern `https_origin` uses; must-fail tests for the transport, a two-line value and the seam guard (`TestStyrmanCtlHarnessOriginIsGated`).
- [medium / medium] `internal/orchestrator/orchestrator.go` — a refused channel left a stopped container's stale binds mounted, then a disabled one, then a moved one (security round 1, code review round 1, security round 2). Fixed: the recreate rule is one comparison — the harness binds a stopped container carries must equal what `harnessBinds` answers now (plan D25–D26); tests for absent, disabled, refused, moved and equal.
- [medium / low → resolved] the `:ro` of the mounted export enforced only as a guest flag would let one project poison every other's `sh-agent` (security round 1). Resolved by measurement on this host's Kata 3.31.0 / Docker 29.7.2: guest root with `CAP_SYS_ADMIN` and AppArmor off can flip the guest mount to rw and a write still fails with "Read-only file system"; nothing lands on the host; the rw control succeeds. Plan D27; Tier 2 repeats the probe every run and requires the remount to have succeeded first.
- [medium / high] `deploy/styrman-ctl` — a latched failed harness tag never degraded `status` (code review round 1). Fixed, with a must-pass/must-fail test.
- [medium / high] BUG-064 — the harness unit pair reached the host only through `self_install`, which runs the previous updater on the deploy tick (found by the main agent reading before the gate). Fixed: `host_converge` installs the pair every tick; record and index row.
- Low findings (all fixed, listed in the reports' Outcome sections): the smoke home on shared `/tmp` moved under the content dir; builder and smoke containers run with no network, no capabilities, no-new-privileges; `.tmp-*`/`.smoke-*` cleanup and `.new.*` orphan sweeps; `repo`/`bin` must be real directories; the token path travels through the git call's environment, never interpolated into the helper snippet; stale comments, a stale help text, a stale check name, a dead helper. Rebutted (reasons in the reports): the Tier 2 boot-log greps as evidence (the direct mount checks are the evidence), the clone's origin not re-asserted by the updater (root ownership, `trusted_dir`, the signature gate), the smoke home owned by uid 1000 (the operator's own uid by design).

## Review rows

- Reviewer (opus, the strongest model): `reviews/2026-09-16-review-567b81f-43dc991-code.md` APPROVE WITH NITS (5 findings, fixed in cafd632); `reviews/2026-09-16-review-43dc991-5ba2b1b-code.md` REQUEST CHANGES (5, fixed in 2f86816); `reviews/2026-09-16-review-5ba2b1b-2f86816-code.md` APPROVE WITH NITS (1, fixed in e3aef79); `reviews/2026-09-16-review-2f86816-e3aef79-code.md` APPROVE WITH NITS (2, fixed in edda5cd — a dead helper deleted and a test header, 11+/13− over two files, read by the main agent). Commits before the feature (`v1.34.1..567b81f`) change only `.agents/docs`, `.agents/rules/tests-describe-behaviour.md`, `REQUIREMENTS.md` and `ROADMAP.md` — trail, no source. Coverage: every source commit in the range is under a report's `Range reviewed`, except edda5cd's nit diff as stated.
- Security: `reviews/2026-09-16-security-567b81f-43dc991.md` (nine findings, worst medium; `## Not examined` names Kata's enforcement — resolved by the probe — and `deploy/vulncheck.sh` — run by the verifier — and no release-diff path); `reviews/2026-09-16-security-43dc991-5ba2b1b.md` (nine, worst medium, four already fixed by 2f86816 when it wrote, the rest fixed or rebutted in e3aef79); `reviews/2026-09-16-security-5ba2b1b-edda5cd.md` over the fix rounds: three low findings, nothing at medium or above, every security-relevant fix confirmed correct and complete (the helper snippet git receives measured constant with a quote, `$(`, space or newline in the path; the whole-string origin match refusing the three values the per-line grep accepted); the three fixed in 19d882c (the origin host pinned to github.com, a test assertion that would catch a re-interpolated path, the remount probe reporting the remount's own outcome in its name) — Outcome in the report; `## Not examined` names `deploy/vulncheck.sh` (run by the verifier) and the Tier 2/Tier 4 runs (the verifier's rows), no release-diff path.
- Verification: `testing/2026-09-16-verify-v1.35.0-release.md` PASS (range ..5ba2b1b: Tier 1 with `-race`, the ingress module, the frontend's tsc/oxlint/vitest 178 tests, `bash -n` and `shellcheck -S warning` on six scripts, `systemd-analyze verify` on four units, the runtime image built from HEAD's `image/`, Tier 2 once `passed=114 failed=0 skipped=3 aborted=0`, `deploy/vulncheck.sh` exit 3 for GO-2026-4887 and GO-2026-4883 in github.com/docker/docker — the same two accepted at the v1.34.0 and v1.34.1 gates, `go.mod`/`go.sum` unchanged, `image/harness` clean); `-round-2.md` PASS (..2f86816, Tier 2 again); `-round-3.md` PASS (..e3aef79, Tier 2 again, 14/14 orchestrator and 19/19 ctl cases). `-round-4.md` PASS (..19d882c, the final tree: Tier 1 with `-race`, 18/18 ctl cases, `bash -n` and `shellcheck -S warning`, Tier 2 once more `passed=114 failed=0 skipped=3 aborted=0` with the remount check's name reading `remounted-rw`). `image/` is unchanged since the round-1 image build, so every Tier 2 run used the image built from the release tree's `image/`.
- Docs lens: `reviews/2026-09-16-review-567b81f-3ede401-docs.md` REQUEST CHANGES (three stale statements, fixed in 6eda8ca); `reviews/2026-09-16-review-v1.34.1-5ba2b1b-docs.md` APPROVE (two deploy.md gaps, fixed in 2f86816). Advisory; nothing left for a follow-up.

## Trail freshness

- Newest `changes/` entry: `2026-09-16-v1.35.0-release.md` (this release), preceded by the feature's phase entries and BUG-064 — pass.
- No artifact under `.agents/docs` carries a future `date:` — pass.
- `changes/` and `DECISIONS.md` append-only in the range (`git log --diff-filter=M v1.34.1..HEAD -- .agents/docs/changes` empty; the DECISIONS.md diff removes no line) — pass.
- `ROADMAP.md` and `TODO.md` touched in the range (567b81f) — report-only; both are empty past ROADMAP's "Now", which the closing handoff clears.

## Verification observed by the main agent (bare, status read)

- Tier 2 before the phase-3 checkpoint: `passed=113 failed=0 skipped=3` (116 checks then), then the verifier's three runs at 117 checks.
- The Kata read-only probe by hand (the must-pass state) and the rw control (the must-fail state), 2026-09-16.
- `make-scratch-harness.sh` by hand: v1.0.0 delivered in 3.8 s, v1.0.1 swapped on the second call.
- The live host before the tag: `styrman-server:1.34.1` healthy, deployed v1.34.1 (sha f7d1b15), `styrman-update.timer` enabled and active, no harness units installed yet (expected), verdict OK.

## What was not checked

- The browser e2e tier (`web/e2e/run.sh`): no frontend source changed in the range (the `web/` tree is untouched).
- `deploy/host-setup.sh`'s new lines and the updater's `host_converge` and `self_install` are root-only with no test tier: they are read on the host after the deploy (the Deploy section) — the harness unit pair should appear after the deploy's SECOND updater tick (BUG-064).
- The channel's live delivery: needs the operator's fine-grained token (`styrman-ctl update --setup --harness-token-file`) and a signed `vX.Y.Z` tag on styrman-harness, neither of which exists yet; until then the server logs the channel absent and every project seeds from the image (`source image`), which the Deploy section verifies.
- `deploy/verify-commit.sh` runs on the release commit before the tag (versioning.md step 2) and is recorded in the Deploy section.

## Deploy (observed after the tag)

Recorded in the follow-up commit after the tag, as for v1.34.1: `deploy/verify-commit.sh <release commit>` (versioning.md step 2), the signed tag verified against a scratch copy of `/etc/styrman/allowed_signers`, the push, the updater tick that deploys it, `styrman-server:1.35.0` healthy, the server log's `harness channel absent` line, the harness unit pair installed by the SECOND tick's `host_converge` (BUG-064), `styrman-ctl status` printing the `harness` line as "not set up", and a throwaway project's boot log ending `source image`.
