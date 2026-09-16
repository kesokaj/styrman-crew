---
owner: main
date: 2026-09-16
---
# Preflight — v1.34.0, second run (2026-09-16)

Range reviewed: v1.33.0..3743c29 (plus the release commit)

Release: **MINOR** v1.34.0 — every project home gains the agent harness at
styrman-harness `4aa19be7eeb5b15e4b876832bd81807cd5d91b37` (27 skills / 7
subagents; `sh-agent install` writes the `attribution` key into
`~/.claude/settings.json` when absent, so no commit made by Claude Code in a
project carries a Co-Authored-By trailer — ADR-072, R5 amended 2026-09-16),
the entrypoint logs the installer's `settings:` lines and its attribution
`note:` lines at boot, and the Tier 2 suite asserts the file (BUG-061,
BUG-062 fixed; 104 checks). Nothing breaks: no endpoint, flag, or deploy
step changes; `web/` untouched.

Commits in the range: 5418f19 (mirror refresh to a81ef4b), 4957dfe
(handoff), 6a0b39e (mirror refresh to 4aa19be, R5, ADR-072, entrypoint
logging), 9dab61d (handoff), 6bdbe02 (first gate: NO-SHIP trail, three docs
fixes, BUG-061/062 records), 8ada624 (handoff), 3743c29 (BUG-061/062 fix
under the bug skill, with its own verifier, reviewer and security rows).
No pre-gate commit: the tree was clean when this run started.

The first run of this gate, `.agents/docs/reviews/2026-09-16-preflight-v1.34.0.md`
(NO-SHIP), carries the `## Outcome` that resolved its three blocking
conditions; this run gates the tree that resolves them.

## VERDICT: SHIP

No blocking condition: verifier PASS over the range (Tier 2
`passed=101 failed=0 skipped=3`, `deploy/verify-commit.sh 3743c29` "safe
to tag"); security FINDINGS with no high and a `## Not examined` that
names no release-diff path (all 74 read end to end, trail files included);
reviewer APPROVE WITH NITS on the fix commit, the earlier REQUEST CHANGES
resolved; docs lens APPROVE WITH NITS; no REQUIREMENTS.md violation; no
project agent shadows a harness agent.

## Findings (FAIL-first)

None blocking.

- **[medium / high — FIXED in the release commit]** reviewer (fix commit)
  #1 = security #3: ADR-072's Consequences paragraph said the entrypoint
  logs `^settings:` lines "(created/refused/failed)" and that every
  advisory is logged — the claim BUG-062 disproved. An amendment paragraph
  is appended to ADR-072 (`DECISIONS.md`, 2 lines added, 0 changed, by the
  changelog agent).
- **[low / high — FIXED in the release commit]** reviewer #2: BUG-062's Fix
  section named the intermediate pattern; corrected to the committed one.
- **[low / low — REBUTTED]** reviewer #3: the re-seed check greps the
  container's accumulated `docker logs`, so a `created with` line from an
  earlier boot could satisfy it if the undo failed. Not a defect: the
  container holding that log was created at the persistence recreate, after
  the plant, and its first boot logged the `note:` line and never `created
  with` (the first boot's container was `docker rm -f`'d with its log); a
  match can only come from the boot after the undo, and the undo is itself
  a check.
- **[low / medium — REBUTTED]** security #1: the boot-log greps are
  unanchored `grep -F`, so a guest that controls PID 1's stdout could forge
  a matching tail. Those checks test the entrypoint's logging, which the
  entrypoint comment calls a diagnostic, not an attestation; the
  security-bearing assertion is the `jq` read of `~/.claude/settings.json`
  as the project user (`integration.sh` "settings.json holds exactly the
  attribution key" and the re-seed check); the only guest that could forge
  is the suite's own throwaway project, and `projectLogs` is owner-scoped.
  Anchoring the greps is a tightening candidate (handoff Next Steps).
- **[low / medium — left: styrman-harness]** security #2: the vendored
  installer's `readSettings` opens the file without `O_NOFOLLOW` between
  its `Lstat` and the temp+rename; same uid on both sides, no escalation;
  upstream's fix (`O_NOFOLLOW` + `f.Stat()`, no dependency).
- **[medium / high — carried, accepted]** `bash deploy/vulncheck.sh` exits
  3: GO-2026-4887 and GO-2026-4883 in `github.com/docker/docker@v28.5.2`,
  called, no fix available — the pair the script's header accepts;
  `image/harness` clean. Restated in the release entry.
- **[low — left: styrman-harness]** four carried lows in the vendored
  `cmd/sh-agent/main.go` (a read-only settings.json still replaced; the
  textual JSON splice guarded by `json.Valid` only; `MkdirAll` through a
  symlinked `~/.claude`; a dead `error` return) — recorded in the security
  report; for the harness session.
- **[low / high — FIXED]** docs lens #1: the fix's changes entry said "was
  94" for a previous total of 97; a Correction section is appended to it
  (append-only file).
- Observation, out of range (security): `.claude/settings.json` is tracked
  with `{"permissions": {"allow": ["Bash"]}}` — the operator's recorded
  standing permission (AGENTS.md § Environment); no action.

## Review rows

- Code, the fix commit (8ada624..3743c29, opus):
  `.agents/docs/reviews/2026-09-16-review-8ada624-3743c29-code.md` —
  APPROVE WITH NITS; the three security-review fixes verified correct and
  complete, the anchored pattern re-validated against 14 lines. Earlier
  commits: 5418f19 by `2026-09-15-review-worktree-code.md` (APPROVE);
  6a0b39e and 9dab61d by `2026-09-16-review-5418f19-9dab61d-code.md`
  (REQUEST CHANGES — resolved, see the first gate report's Outcome);
  6bdbe02 and 8ada624 are trail only; the fix as a working tree by
  `2026-09-16-review-worktree-bug-061-062-code.md` (APPROVE).
- Security, the whole range (v1.33.0..3743c29, opus):
  `.agents/docs/reviews/2026-09-16-security-v1.33.0-3743c29.md` — FINDINGS,
  three low, nothing blocking; `## Not examined` names no diff path.
  Verified there: SHA256SUMS 155/155, `COMMIT` = 4aa19be, no symlinks, no
  `.git`, no secrets or instruction-shaped text in the 34 trail files; the
  two mediums the 2026-09-15 review left upstream are closed at 4aa19be.
  The fix's own row: `2026-09-16-security-worktree-bug-061-062.md` (three
  low, all fixed, Outcome written).
- Verification: `.agents/docs/testing/2026-09-16-verify-v1.34.0-release-2.md`
  — **PASS**: root module build/vet/test 348/0/1, ingress 57/0/0, mirror
  79/0/1 with `sh-agent list` 27/7, vulncheck the accepted pair only,
  shellcheck clean on both changed scripts, `deploy/verify-commit.sh
  3743c29` full run → `OK: commit 3743c29f749b builds and tests clean in
  isolation — safe to tag` (web/dist freshness, the frontend trio, both
  images, `runtime image seeds 27 skills, 7 subagents (styrman-harness
  4aa19be)`, plain copy), Tier 2 once `passed=101 failed=0 skipped=3
  aborted=0` with the four named ADR-072/BUG-062 lines PASS, Tier 5 steps
  1-2 (mirror diff the three expected lines, dry-run shows the settings
  write). It notes the previous log's prose counts (291/53) were a
  miscount of the same raw output (348/57); no regression.
- Docs lens: `.agents/docs/reviews/2026-09-16-review-v1.33.0-3743c29-docs.md`
  — APPROVE WITH NITS (one low, corrected).
- The fix's rows: verifier `testing/2026-09-16-verify-bug-061-062.md` PASS
  (Tier 2 101/0/3); reviewer APPROVE; security three low fixed.
- Shadow agents: none.

## Verification observed by the main agent (bare, status read)

- Two bare Tier 2 runs on the fix (`.agents/scratch/bug-061-062/
  integration-run1.txt`, `integration-run2.txt`): each
  `passed=101 failed=0 skipped=3 aborted=0`, rc 0; run 2 on the final tree
  with the test image rebuilt.
- The jq assertion validated must-pass (the key alone → rc 0) and
  must-fail (`{}`, an extra key, `commit` set, a missing file → 1/1/1/2);
  the filter validated on the installer's real directory-case output (old
  pattern 0 lines, new pattern the `note:` line) and on the PATH hint with
  `/home/attribution/` in its path (not matched).
- `bash -n` and `shellcheck -S warning` on both scripts: rc 0.
- The verifier's scratch (`.agents/scratch/verify-v1.34.0-release-2/`):
  `verify-commit.txt` ends "safe to tag"; `integration.txt` ends
  `passed=101 failed=0 skipped=3 aborted=0`; `--- FAIL:` count 0 in all
  three Go logs.
- Live host before the tag: `styrman-update.timer` active and enabled,
  deployed v1.33.0 (sha 21d8fd7); `/etc/styrman/allowed_signers` is the
  operator's ed25519 key; one project microVM (`local-lan-test`) running on
  `styrman-runtime:1.33.0` with an attached tmux client — left alone;
  Docker Hub anonymous budget 100/100 at the start of this run.

## Trail freshness

- Newest `changes/` entry `2026-09-16-bug-061-062-fix.md` describes the
  fix commit; the release entry is written in the release commit. Pass.
- No `date:` in the future under `.agents/docs`. Pass.
- Append-only: across the range `DECISIONS.md` only gained ADR-072 (10
  lines, 6a0b39e) and its amendment (2 lines, the release commit); the
  fix's changes entry gains an appended Correction section (4 lines, 0
  deleted) in the release commit; no past entry modified. Pass.
- `ROADMAP.md` and `TODO.md` untouched since v1.33.0, both headers-only.
  Report-only.

## What was not checked

- Browser e2e (`web/e2e/run.sh`): `web/` untouched since v1.33.0; the
  frontend trio ran inside `verify-commit.sh`.
- `npm audit` (no `web/` path in the range); Tier 5 steps 3-5.
- The `settings:` and `note:` lines in a PRODUCTION project's boot log —
  observed on the suite's itest project only; the post-release check on a
  re-seeded project covers production.
- The styrman-harness checkout's newer commits (HEAD `8b95f22`, ahead of
  the mirrored 4aa19be): not in this release; the mirror diff still shows
  only the three expected lines.
- The vendored markdown as a standing prompt-injection corpus (upstream's).
- The release commit itself: verified by `deploy/verify-commit.sh` on it
  after it exists (recorded below).
