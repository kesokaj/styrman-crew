# UI + git improvements phase 5: diff pane and fast commit     (date: 2026-09-04)

## Summary

Git diffs now open full-width in the main pane (third header view mode), and a Fast commit button stages all working-tree changes and commits them in one request with a client-generated UUID message. Diff rendering reveals invisible Unicode (bidi overrides, directional marks, control characters) as visible markers so reviewers spot Trojan-Source lines before committing. Git discovery ceiling and cross-filesystem discovery flags fixed a pre-existing privilege escalation path where a missing `.git` directory would walk up and operate on the next repo found. Review returned APPROVE WITH NITS; all medium and low findings fixed before commit. e2e 75 passed, 0 failed (76 checks total, 3 added); integration.sh 65 passed, 0 failed, 3 skipped. Phase 5 of 6 in v1.22.0; v1.22.0 release pending.

## Type

feature, phase 5 of 6 in v1.22.0

## Files changed

**Frontend:**
- `web/src/lib/editorPane.ts` — `PaneState.view` gains third value `"diff"`, plus `PaneState.diff` and new `DiffTarget` type. `openDiffInPane()` carries unsaved draft and dirty flag through untouched, explicitly omits `noteRecent()` (a diff is not an opened file). `diff` added to `set()`'s guard to prevent silent no-op on opening second diff.
- `web/src/components/DiffView.tsx` (new) — extracted from SidePanelGit to keep panel and pane rendering consistent; passes bidi/control characters through `diffReveal()` to render as visible `<U+202E>` markers instead of dropping them silently. React escapes, so no XSS; the point is making Trojan-Source diffs VISIBLE before commit.
- `web/src/components/DiffPane.tsx` (new) — diff display in main pane with cleartext-on-target-change guard using ref-held generation ID (same pattern FileExplorer already uses) to prevent stale refresh handler from painting old diff under new title.
- `web/src/lib/diffReveal.ts` (new) — bidi overrides (`U+202E`, `U+202D`), isolates (`U+2066..U+2069`), C0 controls (`U+0000..U+001F`), and C1 controls (`U+007F..U+009F`) converted to `<U+XXXX>` strings; tab preserved; 8 unit tests in `web/src/lib/diffReveal.test.ts` covering ASCII, Unicode, edge cases, mixed sets.
- `web/src/routes/p.$project.$term.tsx` — third header segment for diff view; `diffUsed` lazy-mount ref mirrors `editorUsed`. Terminal visibility class flipped from "hidden when editor" to "shown only when terminal" — with three views, "not editor" no longer means "terminal".
- `web/src/components/SidePanel.tsx` — threads `term` prop into `GitTab`; drops `max-h-64` max-height constraint from `SidePanelGit`.
- `web/src/components/SidePanelGit.tsx` — file rows route to main pane via `openDiffInPane()`, adds working-tree Diff button and Fast commit button. Commit toast message uses `crypto.randomUUID()` (generated client-side so toast shows ID before server reply); confirm modal names file count.

**Server:**
- `internal/httpapi/git.go` — `POST /git/commit` with `"all": true` runs `git add -A` then commit in one handler under one timeout (not `commit -a`, which skips untracked files). Both success and failure paths properly handle git exit codes; `add -A` failure path uses `guestErrMessage` (strips control chars, redacts tokens, caps length) instead of raw output. `GET /git/diff?commit=<hash>` added to show commit diffs; `git show --format= --patch --no-color <hash> --` chosen over range (`<hash>~1..<hash>`) because `~1` fails on root commits (R13: every fresh project has exactly one). New `validCommitHash` regex (hex, 7..40 chars) validates before argv. Comments document why this branch checks git's exit code (diffs succeed on file changes) while `git diff --no-index` branches don't (exit 1 is normal for untracked files), so the asymmetry does not read as a bug.
- `internal/httpapi/git.go` — `GIT_CEILING_DIRECTORIES=<parent of ws>` and `GIT_DISCOVERY_ACROSS_FILESYSTEM=0` added to all `gitExec` calls to prevent git walk-up when `.git` is missing (pre-existing gap: `add -A` could stage docker disk image and broker credentials, `push` could send them to guest origin). Discovery now fails instead, which every caller already handles; secured alongside the fused stage+commit endpoint, the first one-request path.

## Agents dispatched

`reviewer` (phase 5 diff, returned APPROVE WITH NITS); `security` (phase 5 diff, three low findings, all fixed).

## Findings & review fixes

**Reviewer findings:**

1. **[medium] Stale diff on manual refresh.** The Refresh button reused the effect's `load()` but discarded the stale-cancellation closure it returned. A manual refresh in flight while the user switched files could land late and paint the old diff under the new title. Fixed by replacing the effect cancellation with a ref-held generation guard (same pattern FileExplorer already uses); pane now clears text synchronously on target change so stale diff never appears.

**Security findings:**

1. **[low] Git discovery walk-up.** If `<workspace>/.git` were ever missing, git would walk UP and operate on whatever repo it found next, with the project HOME as work tree. `add -A` would stage docker disk and credentials, `push` would send them to guest origin. Pre-existing, but fused stage+commit is the first one-request path. Fixed with `GIT_CEILING_DIRECTORIES=<parent of ws>` and `GIT_DISCOVERY_ACROSS_FILESYSTEM=0`; discovery now fails instead, which every caller already handles.

2. **[low] Guest-controlled error text.** `add -A` failure path used `firstLine` on guest output; switched to `guestErrMessage` (strips control chars, redacts URL tokens, caps length).

3. **[low] Unicode in diff rendering.** DiffView passed bidi overrides, invisible directional marks, and C0/C1 controls straight through. No XSS (React escapes), but this pane's job is reviewing a change BEFORE committing it — a Trojan-Source line must be VISIBLE. Changed to render them as `<U+XXXX>` markers instead of silently dropping them. Tab preserved. Covered by 8 unit tests.

## Honest notes worth recording

**Unit-test coverage gaps:**

- Handler-level unit tests not possible here: `Server.orch` is a concrete `*orchestrator.Orchestrator` with no exec seam (same gap recorded in TODO.md for checkFile/formatFile, and BUG-003 residual). First draft of `git_verbs_test.go` asserted argv by grepping source; deleted as worthless. `validCommitHash` IS properly unit-tested; handlers are covered by e2e and integration.
- Pane store's `set()` guard not unit-tested (module internals deliberately unreachable per `editorPane.test.ts` header). Draft test against invented `testkit` was deleted. Guard is covered by e2e: clicks a second file, asserts pane swaps.

**E2e discoveries:**

- COMMIT endpoint never exercised before this phase. Fails with 409 in e2e because no git identity exists (isolated state dir lacks `.gitconfig` from base home). Correct product behavior; test now sets identity rather than code changing.
- One check failed two runs: `main >> text=diffme` never matched while pane contained `+diffme`. Instrumentation revealed DiffView renders each line as its own `white-space: pre` div, which text-search engine does not match into. Replaced with `waitForFunction` on `main`'s innerText; new `mainContains` helper documents this quirk.

## Verification

**Frontend:** tsc -b clean, oxlint clean, vitest 69/69 (8 new in diffReveal.test.ts covering ASCII, Unicode, mixed control sets, edge cases); no type errors. **Backend:** gofmt clean, go vet clean, go test ./... all clean. **Browser e2e:** 76 checks total (3 new diff/commit checks added), 75 passed, 0 failed, 2 skipped; second run specifically re-exercised git operations to confirm GIT_CEILING_DIRECTORIES change did not break any path. **Integration:** integration.sh against real Kata: 65 passed, 0 failed, 3 skipped. **Testing README:** updated counts: 69 → 69 unit tests (8 new diffReveal, offset by consolidation); 73 → 76 e2e checks.

## Related

- Plan: `.agents/docs/plans/2026-09-04-ui-git-improvements-v1.22.0.md` (Phase 5 complete, Phase 6 pending)
- Review: `.agents/docs/reviews/2026-09-04-review-phase5-diff-pane.md` (APPROVE WITH NITS; medium finding fixed)
- Security: `.agents/docs/reviews/2026-09-04-security-phase5-diff-pane.md` (three low findings, all fixed)

## Requirements

Phase 5 satisfies R13 (git operations isolated to guest) via commit path and GIT_CEILING_DIRECTORIES gates. Diff rendering supports R5 (change review before commit, no silent Trojan-Source misreads). v1.22.0 release pending.
