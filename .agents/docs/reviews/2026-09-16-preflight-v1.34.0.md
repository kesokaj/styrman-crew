---
owner: main
date: 2026-09-16
---
# Preflight — v1.34.0 (2026-09-16)

Range reviewed: v1.33.0..9dab61d

Release candidate: **MINOR** v1.34.0 — the vendored harness mirror refreshed
twice since v1.33.0 (styrman-harness a81ef4b in 5418f19, then
`4aa19be7eeb5b15e4b876832bd81807cd5d91b37` in 6a0b39e): every project home
would gain that harness (27 skills / 7 subagents; `sh-agent install` now
writes the `attribution` key into `~/.claude/settings.json` when absent —
ADR-072, R5 amended 2026-09-16), and `image/entrypoint.sh` logs the
installer's `settings:` lines at boot. Commits in the range: 5418f19 (a81ef4b
refresh), 4957dfe (handoff), 6a0b39e (4aa19be refresh, R5, ADR-072,
entrypoint), 9dab61d (handoff). No pre-gate commit: the tree was clean when
the gate started. `deploy/verify-commit.sh 9dab61d` is green (below); what
blocks is the integration suite and the review that predicted it.

## VERDICT: NO-SHIP

Blocking, in order:

1. **Reviewer REQUEST CHANGES with a high finding** (Step 2, code lens over
   5418f19..9dab61d, opus): `.agents/docs/testing/styrman/integration.sh:137-138`
   still asserts that a seeded home has no `~/.claude/settings.json`; the
   harness at 4aa19be creates it on every seed. The reviewer reproduced the
   write with `sh-agent install` built from this tree; the main agent read
   the check's text. Recorded as **BUG-061** (open).
2. **Verifier FAIL** on the same cause: Tier 2 `bash
   .agents/docs/testing/styrman/integration.sh` against `styrman-runtime:test`
   rebuilt from this tree printed `passed=93 failed=1 skipped=3 aborted=0`
   in four runs; the single FAIL is `no previous-harness artifacts (shell
   gates, GEMINI.md, settings.json)`. The README's source-of-truth line
   says a green run prints `passed=94 failed=0 skipped=3`.
3. **Security row partial by the gate's literal rule** (BUG-034): the
   release-range security report's `## Not examined` names 15 files of
   the release diff under `.agents/docs/` (trail records — handoffs,
   reviews, testing logs, the two change entries, `DECISIONS.md`, the
   v1.33.0 plan) as scanned for secrets but not read end to end. My
   assessment: low risk, prose records that ship nowhere — but the rule is
   the rule, and the operator asked that a partial security row block. The
   next gate's security dispatch must read every path in its diff, trail
   files included, or say so and block again.

Not shipped: no tag, no push, no deploy. The live host stays at v1.33.0
(updater timer active, `deployed-version` v1.33.0, sha 21d8fd7).

## Findings (FAIL-first)

- **[high / high — OPEN, BUG-061]** `integration.sh:137-138` — the stale
  absence check above. Fix (pending, not applied here): keep the
  `.agents/bin` and `GEMINI.md` clauses, replace the settings.json clause
  with the positive assertion that the file holds the `attribution` key
  with both values empty and no other key, rename the check; the two
  "install's stdout is discarded" comments (`integration.sh:119-121`,
  `image/entrypoint.sh:126-128`) and `.agents/docs/testing/styrman/README.md:448`
  go with it.
- **[medium / high — OPEN, BUG-062]** `image/entrypoint.sh:159-170` — the
  `^settings:` filter drops the installer's `note: `-prefixed lines, which
  are the only report for a settings.json that is not a regular file,
  unreadable, or not a JSON object; in those cases the key is never
  written and the boot log shows nothing. Reviewer finding 3; mechanism
  confirmed by the main agent in the mirror's `cmd/sh-agent/main.go:167`
  and `:654-663`.
- **[medium / high — FIXED in the working tree]** reviewer finding 2 =
  docs lens findings 1-2: `.agents/docs/testing/styrman/README.md:402-408`
  said install's stdout is discarded and listed settings.json among files
  a seeded home must not have; and docs lens finding 3 (low):
  `.agents/docs/specs/styrman/runtime-image.md:244-246` said "NO
  settings.json". All three rewritten by the `documentation` agent from a
  closed fact list; diff read by the main agent (`git diff --numstat`: 5/2
  and 5/2 lines, text, only those two files); zero corrections.
  `README.md:448` (reviewer finding 2's second site) is NOT fixed — it
  belongs with BUG-061's fix.
- **[medium / high — carried, accepted]** security finding 1:
  `bash deploy/vulncheck.sh` exits 3 tree-wide — GO-2026-4887 and
  GO-2026-4883 in `github.com/docker/docker@v28.5.2+incompatible`, called,
  no fix available; the pair the script's header accepts; `image/harness`
  clean. Not introduced by the range; the release entry restates the
  acceptance.
- **[low / high — left: styrman-harness]** security finding 2:
  `image/harness/cmd/sh-agent/main.go:603-607` — a read-only
  `~/.claude/settings.json` is still replaced by the rename. Carried from
  the 4aa19be report's finding 3; filed upstream (harness plan
  `2026-09-16-attribution-write.md`).
- **[low / high — left: styrman-harness]** security finding 3:
  `main.go:487-611` — the textual JSON splice is guarded by `json.Valid`
  only; the suggested fix (assert the key set round-trips through
  `map[string]json.RawMessage`) is upstream's, the mirror being verbatim.
- **[low / medium — left: styrman-harness]** security finding 4:
  `main.go:503-509` — `os.MkdirAll` follows a symlinked `~/.claude`; the
  agent verified this adds no surface `apply.go` did not already have.
- **[low / medium — left: styrman-harness]** reviewer finding 4: a dead
  `error` return in `attributionHint` (vendored).
- **[low / medium — rebutted]** reviewer finding 5: `README.md:174-175`
  (the v1.20.0 release bullet) says "no settings files" — dated history,
  dispositioned the same way by the 2026-09-16 docs lens of the 4aa19be
  refresh; not a living statement.

## Review rows

- Code (5418f19..9dab61d, opus):
  `.agents/docs/reviews/2026-09-16-review-5418f19-9dab61d-code.md` —
  REQUEST CHANGES (1 high, 2 medium, 2 low). 5418f19 itself is covered by
  `.agents/docs/reviews/2026-09-15-review-worktree-code.md` (APPROVE) and
  was not re-reviewed.
- Security (v1.33.0..9dab61d, opus):
  `.agents/docs/reviews/2026-09-16-security-v1.33.0-9dab61d.md` —
  FINDINGS, no high (1 medium carried, 3 low). Verified there: SHA256SUMS
  155/155 both directions, the mirror byte-identical to upstream 4aa19be
  (`git archive` + `diff -rq`), `image/harness/go.mod` has no requires, no
  secrets in the added lines. `## Not examined` names the 15 trail files —
  blocking condition 3.
- Verification: `.agents/docs/testing/2026-09-16-verify-v1.34.0-release.md`
  — **FAIL** (Tier 2). Green in the same log: root module build/vet/test
  291/0/1, ingress 53/0/0, mirror module 79/0/1 (unchanged vs the 4aa19be
  refresh log), vulncheck the accepted pair only, `deploy/verify-commit.sh
  9dab61d` → `OK: commit 9dab61d9140e builds and tests clean in isolation —
  safe to tag` (full run: web/dist freshness, the frontend trio, both
  images, `runtime image seeds 27 skills, 7 subagents (styrman-harness
  4aa19be)`, plain copy), Tier 5 steps 1-2 (mirror diff the three expected
  lines, `sh-agent list` 27/7, `install --dry-run` shows the settings.json
  creation). The verifier stopped at its turn limit twice and was resumed
  twice; one count corrected by the main agent (it wrote three integration
  runs where its scratch holds four).
- Docs lens (v1.33.0..9dab61d):
  `.agents/docs/reviews/2026-09-16-review-v1.33.0-9dab61d-docs.md` —
  REQUEST CHANGES, advisory (2 medium, 1 low), all three fixed above.
- Earlier rows over the two refreshes, cited not repeated:
  `2026-09-15-review-worktree-{code,docs,merged}.md`,
  `2026-09-15-security-worktree.md`,
  `2026-09-16-security-harness-refresh-4aa19be.md` (with `## Outcome`),
  `2026-09-16-review-worktree-docs-harness-refresh.md`, verifier logs
  `2026-09-15-verify-harness-refresh-a81ef4b.md` and
  `2026-09-16-verify-harness-refresh-4aa19be.md`. The 4aa19be commit never
  had a code lens — the omission that let BUG-061 reach this gate.
- Shadow agents: none (`.claude/agents/` and `.agents/agents/` empty).

## Verification observed by the main agent (bare, status read)

- `.agents/scratch/verify-v1.34.0-release/verify-commit.txt` ends
  `OK: commit 9dab61d9140e builds and tests clean in isolation — safe to
  tag`; `integration.txt`, `integration2.txt`, `integration3.txt`,
  `integration4.txt` each end `passed=93 failed=1 skipped=3 aborted=0`
  with the same FAIL line; the check before it, `boot log records the
  harness deployed at the mirror's commit`, PASS in each.
- The docs fixes: `git diff --numstat` 5/2 `runtime-image.md`, 5/2
  `testing/styrman/README.md`; no other path touched.
- Live host before the gate: `styrman-update.timer` active and enabled;
  last tick 12:42 "already up to date" at v1.33.0; `/etc/styrman/allowed_signers`
  is the operator's ed25519 key (matches `~/.ssh/id_ed25519.pub`); one
  project microVM (`local-lan-test`, LAN mode) running on
  `styrman-runtime:1.33.0` with an attached, focused tmux client — not
  touched; its home has no `~/.claude/settings.json` (the 1.33.0 harness
  wrote none). Docker Hub anonymous budget 99/100 at 12:48, 100/100 at
  13:15 (the gate's builds fit inside one hour's window).
- NOT observed: a `settings:` line in a real project boot log. The
  verifier's three capture attempts hit the operator's live container, a
  removed container, and a recycled itest container (the suite's LAN
  toggle recreates the project, and the seed runs once per stamp); the
  exact wording was confirmed by a direct `sh-agent install` inside a
  throwaway container of the freshly built `styrman-runtime:test`. The
  post-release check in the operator's brief still stands, unrun.

## Trail freshness

- Newest `changes/` entry `2026-09-16-harness-refresh-4aa19be.md` describes
  the release diff. Pass.
- No `date:` in the future under `.agents/docs`. Pass.
- Append-only: `git log --diff-filter=M v1.33.0..HEAD -- .agents/docs/changes
  .agents/docs/architecture/DECISIONS.md` lists 6a0b39e only because
  `DECISIONS.md` gained ADR-072 (10 lines added, 0 deleted across the
  range); no past entry modified. Pass.
- `ROADMAP.md` and `TODO.md` untouched since v1.33.0, both headers-only.
  Report-only.

## What was not checked

- Browser e2e (`web/e2e/run.sh`): `web/` untouched in the range; the
  frontend trio ran inside `verify-commit.sh`.
- `npm audit` (no `web/` path in the range); Tier 5 steps 3-5.
- A real boot showing the `settings:` line (above).
- The 15 trail files end to end by the security agent (blocking 3).
- The release commit itself: none was made.

## Pre-gate work committed with this report (no release commit)

The three docs fixes, BUG-061 and BUG-062 with their INDEX rows, the four
agent artifacts, the changes entry and this report ride in one trail
commit on `main`, unpushed: the operator's instruction bundled the push
with the release and the release did not happen. Nothing here is a
version bump, a tag, or a deploy.

## Next, for the task that resolves this

1. Under the `bug` skill: fix BUG-061 (the check, the two comments,
   `README.md:448`) and BUG-062 (match `^settings:` and `^note: `, or log
   every installer line bounded as now; fix the comment). Tier 2 must
   print `passed=94 failed=0 skipped=3` with the renamed check; the
   reviewer over the fix (a bug fix always gets one).
2. Re-run `/preflight v1.34.0` over `v1.33.0..<new HEAD>`, briefing the
   security agent to read every path in that diff end to end, trail
   files included.
3. Then the operator's post-release verification on one re-seeded
   project (boot log lines, `grep -c '"attribution"'` = 1, `sh-agent
   status` 27/7 with no note line, a Claude Code commit without a
   trailer), recorded as `.agents/docs/testing/<date>-verify-release-v1.34.0.md`.

`## Outcome` is appended by that task, one line per blocking finding, in
the same commit as the fix.
