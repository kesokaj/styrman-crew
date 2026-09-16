---
owner: changelog
date: 2026-09-16
---
# Harness refresh to 4aa19be     (date: 2026-09-16)

## Summary

Mirror refreshed from `styrman-harness` a81ef4b to 4aa19be (28 files changed). Upstream subjects: BUG-039..044 fixes, eight retro proposals applied, the `sh-agent install` attribution setting write (upstream ADR-025, R4). Local edits: REQUIREMENTS.md R5 (operator approval), `image/entrypoint.sh` comment and logging, ADR-072.

## Type

mirror refresh

## Files changed

- `image/harness/` — refreshed to 4aa19be by `image/harness/VENDORED.md` § Refresh; upstream: `git -C /home/kesokaj/workspace/styrman-harness log --oneline a81ef4b..4aa19be` (8 commits: BUG-039..044 fixes, attribution setting feature, retro proposals)
- `image/harness/COMMIT` — SHA 4aa19be7eeb5b15e4b876832bd81807cd5d91b37
- `image/harness/SHA256SUMS` — mirror attestation refreshed
- `REQUIREMENTS.md:49-52` — R5 amended: "no hooks or scripts, and no settings file except the one key the harness's own R4 allows — `attribution` in `~/.claude/settings.json`, commit and PR trailers off, written only when absent — ADR-062, operator approval 2026-09-04, the exception approved 2026-09-16 (ADR-072)" (operator directive 2026-09-16)
- `image/entrypoint.sh:110-133` — comment updated to document the attribution key write and its safety (created/absent-only, present key left alone); updated logging: `settings:` lines from install output are now captured and logged, so writes are auditable in the boot log
- `.agents/docs/architecture/DECISIONS.md` — ADR-072 appended, "The harness may write one vendor setting into a project home: Claude Code's attribution key (amends ADR-062's settings clause)", accepted 2026-09-16

## Agents dispatched

verifier, security, reviewer (docs lens), changelog

## Verification

**Verifier:** `.agents/docs/testing/2026-09-16-verify-harness-refresh-4aa19be.md`, **PASS** — counts: **pass 79, fail 0, skip 1** (`go test ./... -count=1 -v` in `image/harness`); mirror fidelity confirmed (SHA256SUMS check, file-list check, no symlinks); `go vet`, `gofmt`, `sh-agent list` all clean. The new `TestAttributionNote` and `TestAttributionSet` test functions (upstream 42ba2f5) confirmed present and passing. Deploy verification (`deploy/verify-commit.sh`) not run because this refresh is uncommitted; integration tests (Tier 2) last run 2026-09-15 at a81ef4b: `passed=94 failed=0 skipped=3`.

**Security:** `.agents/docs/reviews/2026-09-16-security-harness-refresh-4aa19be.md`, **FINDINGS** — one medium resolved (ADR-072 now on disk and cited; R5 amended with operator approval; entrypoint logs adornments from install so the write is visible in boot log); two low findings: one fixed in entrypoint (capture and log install's stdout `settings:` lines with control-char strip, matching the MCP logging block shape); one filed upstream as a Follow-up (read-only settings.json is still replaced by rename; harness plan to refuse a file the owner cannot write). Verified: twelve probe shapes, symlink refused, no concurrent writer in current wiring, deps clean (`GO-2026-4887`, `GO-2026-4883` unchanged in server/ingress modules, image/harness "No vulnerabilities found").

**Reviewer (docs):** `.agents/docs/reviews/2026-09-16-review-worktree-docs-harness-refresh.md`, **REQUEST CHANGES** round 1 → **fixed with approval** — Finding 1 (high): R5's "no settings files" now contradicts the code; operator approval (2026-09-16, "Amend crew's requirement: allow that one key") received; R5 amended and ADR-072 added. Finding 2 (low, historical): v1.20.0 release-history bullet (README.md:174-175) is a dated snapshot, left unchanged (predates the attribution feature).

## Docs

- `REQUIREMENTS.md` R5 — changed (operator approval 2026-09-16; quotes above under Files changed)
- `image/entrypoint.sh` — comment and logging changed (documented in Files changed; `bash -n` clean)
- `.agents/docs/architecture/DECISIONS.md` — ADR-072 added (appended, not edited)

## Requirements

**R5** — amended with operator approval (ADR-072): the one exception the harness's R4 allows is now styrman's own R5.

---

**Not yet done:** push and release (`/preflight`, `versioning.md`; previous handoff recommended v1.34.0; release entry names COMMIT `4aa19be7eeb5b15e4b876832bd81807cd5d91b37`). Deploy-path full e2e (`deploy/verify-commit.sh` + integration tests Tier 2) runs after the commit; recorded in the handoff.
