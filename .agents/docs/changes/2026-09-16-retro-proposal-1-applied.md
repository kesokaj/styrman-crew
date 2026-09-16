---
owner: changelog
date: 2026-09-16
---
# Retro Proposal 1 applied — the project rule 'a behaviour change includes the tests that describe it' (2026-09-16)

## Summary
Applied retro Proposal 1 on operator's approval. BUG-061's prevention rule is now live: `.agents/rules/tests-describe-behaviour.md` requires that when a requirement's wording or boot-time behaviour changes, the test suites (`.agents/docs/testing/**/*.sh`, `web/e2e/`, `*_test.go`) be grepped for old assertions alongside doc updates. A mirror refresh that changes what the image writes into a home runs Tier 2 before its commit and gets the code lens every time; the Tier 2 count line in `testing/styrman/README.md` changes in the same commit as the checks it counts.

## Type
trivial

## Files changed
- `.agents/rules/tests-describe-behaviour.md` — new rule file
- `.agents/docs/retros/2026-09-16-retro.md` — Proposal 1's status `proposed` → `applied 2026-09-16`, the Summary's last sentence updated; the matching line in `~/.config/sh-agent/retros.jsonl` updated the same way
- `.agents/docs/bugs/INDEX.md` — BUG-061's row moved to a new Graduated table; `.agents/docs/bugs/BUG-061-harness-refresh-settings-check-stale.md` — one line noting the graduation

## Agents dispatched
bugs (BUG-061 row moved to Graduated table), changelog

## Next
Three remaining retro proposals are harness-scope, queued in `~/.config/sh-agent/proposals/2026-09-16-retro.md` for `/retro --harness` in styrman-harness checkout.
