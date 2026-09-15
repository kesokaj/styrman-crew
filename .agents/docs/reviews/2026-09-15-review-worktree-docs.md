---
owner: reviewer
date: 2026-09-15
---
# Review — worktree-docs (2026-09-15)

Range reviewed: working tree, `image/harness/**` (verbatim mirror refresh of the external `styrman-harness`
repo, COMMIT `cceee548e` → `a81ef4be3`) — the 32 tracked files listed in the dispatch. No untracked files.
Compared against `README.md`, `AGENTS.md`, `.agents/docs/specs/styrman/{runtime-image,versioning}.md`,
`.agents/docs/testing/styrman/README.md` and `integration.sh`, `.agents/docs/deploy/*.md`, and the newest
`changes/` entries.

## Verdict: APPROVE

Nothing in styrman-crew's own living docs goes stale from this refresh. The mirror is internally consistent
with itself (message-text changes in `apply.go`/`main.go` match their own doc/test updates), and every
number or behavioral claim styrman-crew's own docs make about the harness (27 skills, 7 subagents,
`--dry-run`, the vendor-default rule, the `~/.claude/CLAUDE.md` handling, the plain-copy/BUG-015 message)
still holds against the new code — confirmed by running `go run ./cmd/sh-agent list` inside the refreshed
`image/harness` (`### Skills (27)`, `### Subagents (7)`) and by reading `main.go`/`apply.go` for the
`--dry-run`, `--force`, and `--vendor` default behavior the styrman README lines describe (all pre-existing
at HEAD, not introduced by this diff — the styrman README additions are catch-up, not new claims outrunning
code).

Removed/changed wording specifically checked by `git grep` against styrman-crew's own `README.md`, `AGENTS.md`,
`.agents/rules/`, `.agents/docs/specs/`, `.agents/docs/testing/styrman/README.md`, `.agents/docs/deploy/`:
- Old: "add the line `@AGENTS.md` to it yourself" (former `image/harness/README.md`/`main.go` wording for a
  foreign `~/.claude/CLAUDE.md`) — no hit anywhere in styrman-crew's own docs; new wording ("move its content
  into a rule file under `rules/`...") has no stale echo to fix.
- Old: "target all [default]" for `--vendor` — no hit; default-changed-to-recorded-vendor behavior is not
  restated (and thus not contradicted) in styrman's own docs.
- Dropped: `context/AGENTS.md`'s "Workflow skills, also typed as `/name`: ..." line — not referenced verbatim
  anywhere in styrman-crew's own docs (that file is the harness's own rendered context, not something
  styrman-crew's docs quote).
- "27 skills and 7 agy agents" (styrman `.agents/docs/testing/2026-09-14-v1.33.0-phase6-verify.md`,
  `.agents/docs/testing/styrman/README.md:405`, `integration.sh:118`) — numbers unchanged by this refresh,
  verified live; not a finding.
- `BUG-033`/`BUG-041` cited by name in `image/harness/skills/feature/SKILL.md`'s Step 4 — checked against
  styrman-crew's actual `.agents/docs/bugs/BUG-033-*.md`/`BUG-041-*.md` and `INDEX.md` rows; description
  matches.

`image/harness/ROADMAP.md`'s new "Now" line points at a harness-repo-local plan
(`.agents/docs/plans/2026-09-15-harness-practices-g01-g26.md`) that does not exist in styrman-crew's tree —
expected and not a finding: `VENDORED.md` states the mirror's own paths are internal to the upstream
`styrman-harness` repo, not styrman-crew's `.agents/docs/plans/`.

## Findings
None.

## Criteria
1. Dead code — n/a — docs lens
2. Flow tracing — n/a — docs lens
3. Standards conformance — n/a — docs lens
4. Directory layout — n/a — docs lens
5. Simplicity (DIY-except-security) — n/a — docs lens
6. Decoupling — n/a — docs lens

## Not examined
- `image/harness/SHA256SUMS` and `COMMIT` — hash/pin churn only, not read line-by-line (verified only that the
  `COMMIT` value matches the diff header and that a live `sh-agent list` run against the refreshed tree
  produces the counts styrman's docs cite).
- Full byte-for-byte diff of `image/harness/skills/init-project/SKILL.md`'s update-mode logic against every
  styrman-crew doc that might describe `/init-project` behavior — styrman-crew has no doc making claims about
  `/init-project`'s update mode, so none was checked for staleness beyond the `git grep` above.
- Whether the operator's own harness checkout (`~/workspace/styrman-harness`, separate from
  `image/harness/`) is at the same commit — out of scope for this working-tree diff.
- `deploy/verify-commit.sh`'s harness-mirror gate step-by-step re-execution — not run; relied on the existing
  2026-09-14 verification logs plus the direct `sh-agent list`/code checks done here.
