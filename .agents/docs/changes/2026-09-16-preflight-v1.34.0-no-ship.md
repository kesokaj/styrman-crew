---
owner: changelog
date: 2026-09-16
---
# Preflight v1.34.0 — NO-SHIP     (2026-09-16)

## Summary
/preflight was run to cut v1.34.0 (a MINOR: project homes gain the harness at styrman-harness 4aa19be, attribution key in ~/.claude/settings.json per ADR-072 and R5 amendment) over range v1.33.0..9dab61d. Gate report (.agents/docs/reviews/2026-09-16-preflight-v1.34.0.md) gates release; no tag, no push, no deploy; live host stays at v1.33.0.

## Type
release gate

## Range and verdict
Commits: 5418f19 (mirror to a81ef4b), 4957dfe (handoff), 6a0b39e (mirror to 4aa19be + R5 amendment + ADR-072 + entrypoint logging), 9dab61d (handoff).
**Verdict: NO-SHIP** — blocking conditions prevent release.

## Blocking
1. Code-lens REQUEST CHANGES (HIGH): .agents/docs/testing/styrman/integration.sh:137–138 asserts seeded home has no ~/.claude/settings.json; 4aa19be now creates it.
2. Verifier FAIL: Tier 2 integration printed `passed=93 failed=1 skipped=3 aborted=0` in each of four runs — the one FAIL being the check `no previous-harness artifacts (shell gates, GEMINI.md, settings.json)`.
3. Security Not examined: 15 trail files under .agents/docs/ scanned for secrets only, not read end to end (BUG-034 / operator instruction).

## Agents dispatched
- reviewer (code) → .agents/docs/reviews/2026-09-16-review-5418f19-9dab61d-code.md, REQUEST CHANGES (1 high, 2 medium, 2 low)
- security → .agents/docs/reviews/2026-09-16-security-v1.33.0-9dab61d.md, FINDINGS (no high; 1 medium: accepted docker/docker advisories GO-2026-4887/4883; 3 low in vendored mirror)
- verifier → .agents/docs/testing/2026-09-16-verify-v1.34.0-release.md, FAIL (root 291/0/1, ingress 53/0/0, mirror 79/0/1; deploy/verify-commit.sh 9dab61d OK for tagging; Tier 5 manual 1–2)
- reviewer (docs) → .agents/docs/reviews/2026-09-16-review-v1.33.0-9dab61d-docs.md, REQUEST CHANGES (2 medium, 1 low)
- documentation → fixed the 3 docs findings in the two files below; diff read by the main agent (5 added / 2 deleted lines in each), no corrections
- bugs → BUG-061, BUG-062 filed (open, fixes pending)

## Files changed
- .agents/docs/testing/styrman/README.md — 5 added / 2 deleted (lines 402–408)
- .agents/docs/specs/styrman/runtime-image.md — 5 added / 2 deleted (lines 244–246)
- .agents/docs/bugs/ — BUG-061-harness-refresh-settings-check-stale.md, BUG-062-entrypoint-settings-filter-drops-note-lines.md (new); INDEX.md (updated)
- .agents/docs/reviews/ — 2026-09-16-preflight-v1.34.0.md, 2026-09-16-review-5418f19-9dab61d-code.md, 2026-09-16-review-v1.33.0-9dab61d-docs.md, 2026-09-16-security-v1.33.0-9dab61d.md (all new)
- .agents/docs/testing/ — 2026-09-16-verify-v1.34.0-release.md (new)
- .agents/docs/changes/ — this entry (new)

## Not verified
- `settings:` line in real project boot log (verifier hit recycled containers; wording confirmed by direct sh-agent install in throwaway container)
- Operator post-release verification on re-seeded project (no release cut)
- local-lan-test project restart (attached tmux client)

## Next
Fix BUG-061 and BUG-062 under bug skill (Tier 2 must print `passed=94 failed=0 skipped=3` with renamed check; reviewer over fix); re-run /preflight v1.34.0 with security agent reading every diff path end to end; post-release verification on one re-seeded project.

## Requirements
n/a
