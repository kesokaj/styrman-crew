# Phase 2 closeout: web frontend and provisioning state     (date: 2026-09-10)

## Summary

Phase 2 of plan `2026-09-10-todo-closeout.md` closed: web SSH key validation now rejects all control/formatting characters except TAB and U+2028/2029, matching the server's enum; a post-build normalizer (`normalize-shell.mjs`) rewrites prerenderer timestamps to deterministic bytes for reproducible builds; file checker now combines caller's abort signal with 20 s timeout via `AbortSignal.any`, with debounced previous checks aborted; linter mounts only for checkable extensions with e2e drift guard; draft discard now clears the parked buffer and reloads from disk; project state machine gained `provisioning` with precedence (deleting > provisioning > live); sidebar badge reflects state with error cascading. Server-side: `Server.provisioning` sync.Map set before boot with deferred delete; `deriveState` logic. Tests: +4 sshKey, +5 projectBadge, state machine tests; vitest 147 tests, e2e 87 passed / 0 failed / 2 skipped.

## Changes

**H. SSH key control-character rejection**

`web/src/lib/sshKey.ts` — Rejects all of `\p{C}` (Unicode category Other) except TAB (U+0009) and line/paragraph separators (U+2028/U+2029); `abbreviateKey` collapses `\p{Zs}` (space separator) to single space. Key-type regex now equals the server's enum. `web/src/lib/sshKey.test.ts` — 4 new tests covering control chars, formatting, and separators.

**B. Deterministic build artifact**

`web/scripts/normalize-shell.mjs` (new) — Runs after `vite build && tsc -b`; rewrites the prerenderer's `u:<13-digit timestamp>` to `u:0` in `dist/client/_shell.html` as raw bytes (file's documented NUL survives). Requires exactly one match; fatals otherwise. Two consecutive builds now produce byte-identical artifacts; nothing in `web/src` reads this value.

**K7. Checkable file abort handling**

`web/src/lib/api.ts` — `api.checkFile(..., signal?)` combines caller's signal with 20 s timeout via `AbortSignal.any`; `request()` helper gates both. `web/src/components/CodeEditor.tsx` — `FileEditor.runCheck` aborts previous in-flight check per debounce; catch ignores own abort (review finding: first draft replaced the timeout entirely).

**K8. Linter mount and drift guard**

`web/src/lib/api.ts` — `CHECKABLE_EXTENSIONS` exported from `api.ts`, mirrors `checkerFor`. `web/src/components/CodeEditor.tsx` — `lintGutter`/`linter` mount only when file has a checker. `web/e2e/run.mjs` — New check "checkable extensions match the server's checkers": reads `CHECKABLE_EXTENSIONS` from source and asks check endpoint for each plus `.xyzzy` (all should fail correctly).

**K9. Draft discard and reload**

`web/src/lib/editorPane.ts` — `discardDraft(project, term)` clears parked buffer and dirty flag; close-confirm "Discard" now calls it and drops loaded doc so next open re-reads from disk (prior: switched views, kept edits). Test seam `__stateForTest` for vitest.

**29. Project state badge**

`web/src/lib/projectBadge.ts` (new) — Pure function with precedence: deleting > deleteError > provisionError > provisioning. `web/src/components/Sidebar.tsx` — Renders badge result. `web/src/lib/projectBadge.test.ts` — 5 tests. Demoted from e2e by cold reader: `deleteError` is in-memory only, not end-to-end-inducible.

**N. Server provisioning state and cascade**

`cmd/styrman/api.go` — `Server.provisioning` sync.Map set before boot goroutine (whose first statement is deferred delete). `deriveState(deleting, provisioning, live)` logic (deleting > provisioning > live) used by GET and list endpoints. `internal/httpapi/api.ts` — `Project.state` union gains `"provisioning"`. `cmd/styrman/state_test.go` (new) — `TestDeriveState` and `TestProjectViewsReportProvisioning`.

## Files changed

- `web/src/lib/sshKey.ts` — control-char rejection, space collapsing
- `web/src/lib/sshKey.test.ts` — +4 tests
- `web/scripts/normalize-shell.mjs` — new, timestamp determinism
- `web/src/lib/api.ts` — abort signal combining, CHECKABLE_EXTENSIONS export
- `web/src/components/CodeEditor.tsx` — debounce abort handling, linter mount guard
- `web/src/lib/editorPane.ts` — discardDraft implementation
- `web/src/lib/projectBadge.ts` — new, state badge function
- `web/src/lib/projectBadge.test.ts` — new, 5 tests
- `web/src/components/Sidebar.tsx` — renders badge
- `web/e2e/run.mjs` — checkable extensions drift guard
- `cmd/styrman/api.go` — provisioning sync.Map, deriveState logic
- `internal/httpapi/api.ts` — state union gain
- `cmd/styrman/state_test.go` — new, state machine tests

## Agents dispatched

verifier, reviewer, security, changelog

## Verification

- verifier log: `.agents/docs/testing/2026-09-10-verify-closeout-phase2.md` (tsc, oxlint clean, vitest 11 files/147 tests, go test -race httpapi, byte-identity across two builds)
- e2e on real Kata: 87 passed, 0 failed, 2 skipped (runner exit code 2 despite green suite under investigation for Phase 6)
- Code reviews: `.agents/docs/reviews/2026-09-10-review-closeout-phase2.md` (approve with nits, both fixed); `.agents/docs/reviews/2026-09-10-security-closeout-phase2.md` (five lows: octal literal fixed, one-match guard fixed, three noted)

## Requirements

n/a

## Related

- Plan: `.agents/docs/plans/2026-09-10-todo-closeout.md`
- Phase 1: `.agents/docs/changes/2026-09-10-closeout-phase1-server.md`
