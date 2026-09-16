---
owner: main
date: 2026-09-16
---
# Handoff — 2026-09-16 — harness mirror refreshed to 4aa19be, committed locally, verify-commit OK, not shipped

Session ID: c50e0fd4-6273-4e35-b1ef-7639dff8bb07        (a styrman-harness session; same machine: `claude --resume c50e0fd4-6273-4e35-b1ef-7639dff8bb07`)
Project: /home/kesokaj/workspace/styrman-crew
Branch: main

## Context

The operator asked, from the styrman-harness session, to "refresh the two mirrors from HEAD". `image/harness/` is now styrman-harness 4aa19be (from a81ef4b), committed as 6a0b39e under this repository's Definition of Done. Not pushed and not released: the mirror ships only through a release, which is the operator's call.

## Completed

- `image/harness/` refreshed by `VENDORED.md` § Refresh; `COMMIT` = `4aa19be7eeb5b15e4b876832bd81807cd5d91b37`; `diff -rq` against the checkout shows only the three local files; sums 155/155.
- What the range brings (upstream a81ef4b..4aa19be): BUG-039..044 fixes, eight retro proposals applied, and one behaviour that matters here — `sh-agent install` writes `"attribution": {"commit": "", "pr": ""}` into `~/.claude/settings.json` when the key is absent, in every project home on its next seed, so no commit from a project carries a Co-Authored-By trailer. That contradicted R5 ("no hooks, scripts, or settings files") and the entrypoint's "NO settings.json"; the operator amended R5 (2026-09-16), ADR-072 records the exception, the entrypoint comment says the same and the entrypoint now logs the installer's `settings:` lines at boot.
- Verifier PASS (`testing/2026-09-16-verify-harness-refresh-4aa19be.md`: mirror tests 79/0/1, `list` 27/7, vulncheck only the accepted docker findings). Security FINDINGS, every item resolved or filed upstream (`reviews/2026-09-16-security-harness-refresh-4aa19be.md` § Outcome; twelve probe shapes left every other key of a settings.json byte for byte). Docs lens REQUEST CHANGES on R5, fixed with the approval (`reviews/2026-09-16-review-worktree-docs-harness-refresh.md` § Outcome).
- `deploy/verify-commit.sh 6a0b39e` after the commit: `OK: commit 6a0b39eec3e7 builds and tests clean in isolation — safe to tag`; `runtime image seeds 27 skills, 7 subagents (styrman-harness 4aa19be)`, plain copy. Log: `.agents/scratch/verify-commit-6a0b39e/log.txt` (scratch, not committed).
- `changes/2026-09-16-harness-refresh-4aa19be.md`; ADR-072.

## Open / In Progress

- The push and the release (Next Steps 1). Nothing else in flight from this refresh.
- Requirements edits awaiting approval: none — R5's amendment was approved and is in 6a0b39e.

## Key Decisions

- R5 amended: no settings file except the attribution key, written only when absent — by the operator, 2026-09-16, "Amend crew's requirement: allow that one key (Recommended)"; ADR-072.
- The installer's `settings:` lines are logged at boot (security finding 2) so a created, refused or failed settings.json is visible; stderr passes through as before.
- A read-only settings.json still replaced by the rename is upstream's to fix (filed in the harness plan `2026-09-16-attribution-write.md`).

## Corrections

- None.

## Agent notes (unverified)

- The write path was probed with a host-built binary against scratch homes, never through a container boot; the release's Tier 2 integration run is where a real seed shows the `settings:` line in the boot log.

## Environment State

- Branch: main, clean, three commits ahead of origin (5418f19, 4957dfe from the a81ef4b refresh; 6a0b39e and this handoff), not pushed.
- Running processes: none. The `verify-commit.sh` worktree and images are its own clean-up.

## Artifacts to Read

- `changes/2026-09-16-harness-refresh-4aa19be.md`, ADR-072 in `architecture/DECISIONS.md`, the three review and testing files named above, `image/harness/VENDORED.md`.

## Next Steps

1. **Operator: ship the refresh.** Push `main` and cut the release per `.agents/docs/specs/styrman/versioning.md` through `/preflight`; the release entry names COMMIT `4aa19be7eeb5b15e4b876832bd81807cd5d91b37`. Version, recommended: a MINOR (`v1.34.0`): project homes gain behaviour (the attribution key, the rewritten skills, agents and context) and nothing breaks. In the gate, treat a missing or partial security row as blocking (the 2026-09-16 merged record's finding 2 still applies).
2. **After the release, on one re-seeded project:** the boot log shows `agent harness deployed (version …, styrman-harness 4aa19be)` and one `settings: /home/<user>/.claude/settings.json created with "attribution": …` (or `… added; other keys untouched`) line; `grep -c '"attribution"' ~/.claude/settings.json` in that home prints 1; a commit made by Claude Code there carries no trailer.
