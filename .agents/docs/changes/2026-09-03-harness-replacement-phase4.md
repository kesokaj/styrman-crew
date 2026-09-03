# Harness replacement, phase 4: tests and docs     (date: 2026-09-03)

## Summary

Integration tests gained five checks inside booted projects (sh-agent status no drift; two-line `CLAUDE.md`; 28 skills and 7 agy agents; none of `~/.agents/bin`, `~/.gemini/GEMINI.md`, `~/.claude/settings.json`; boot→ready under R1's 15 s hard limit from container timestamps) — 65 checks total; run green against `styrman-runtime:dev` on real Kata: passed=62 failed=0 skipped=3, boot→ready 1.69 s. Two pre-existing shellcheck warnings (unused `for i` variables) fixed in passing. `.agents/docs/testing/styrman/README.md` Tier 5 rewritten around `sh-agent status` (old ADR-047 latency guard retired); Tier list clarified (automated = Tier 2; manual = mirror refresh, image contents, migration fixture, agent-CLI smoke, re-seed evidence); stale frontend unit-test count (11 → 17 across three files, BUG-001 fix) corrected. `.agents/docs/specs/styrman/runtime-image.md`, `image/README.md`, `versioning.md` (lineup row and release step 1 names harness commit), `ROADMAP.md` (Next item harness-assertion closed), `TODO.md` (roster item ticked; upstream items added), ADR-062 appended. `documentation` cold-read every doc against the code; no corrections needed. `REQUIREMENTS.md` R5 wording "vendored from `devshell_prompt/harness`" proposed for one-line edit to "vendored from `styrman-harness`" but not applied (immutable without operator approval).

## Type

docs + tests, phase 4 of 5

## Files changed

- `.agents/docs/testing/styrman/integration.sh` — five new assertions inside booted project (65 checks total)
- `.agents/docs/testing/styrman/README.md` — Tier 5 rewritten; Tier list reordered (automated/manual); ADR-047 guard retired; BUG-001 (frontend count: 11 → 17) corrected
- `.agents/docs/specs/styrman/runtime-image.md` — step 6 updated
- `image/README.md` — harness provenance updated
- `.agents/docs/specs/styrman/versioning.md` — lineup row; release step 1 names harness commit; ADR-046/047 delivery model noted
- `.agents/docs/architecture/DECISIONS.md` — ADR-062 appended (D1–D8 decisions, consequences, source, verification references)
- `ROADMAP.md` — "Next" harness-assertion item closed
- `TODO.md` — harness-roster item ticked; upstream `styrman-harness` items added (filepath.IsLocal, `(devel)` version, `update` verb, corpus hygiene, permissions.ask rail)

## Agents dispatched

documentation (cold-read of every changed spec/testing/docs file against code; no corrections needed), verifier (integration run in `.agents/scratch/integration-dev.log` summarized here; 62 passed, 0 failed, 3 skipped).

## Verification

- `integration.sh` green with new 65-check count against `styrman-runtime:dev` on real Kata
- `documentation` cold-read confirms docs match code (no changes needed)
- Tier descriptions match actual test automation shape
- Boot→ready 1.69 s (R1 satisfied)

## Requirements

R5 stands (identical defaults from base image). Proposed wording edit to "vendored from `styrman-harness`" awaits operator approval; REQUIREMENTS.md remains immutable without explicit approval.

## Related

- ADR-062 (decisions and verification references)
- Plan: `.agents/docs/plans/2026-09-03-image-harness-replacement.md`
- Testing: `.agents/docs/testing/2026-09-03-verify-harness-phase1.md`, `…-phase2.md` (Phase 4 verification embedded in integration run)
