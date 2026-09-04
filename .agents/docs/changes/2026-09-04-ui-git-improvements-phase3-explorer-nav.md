# UI + git improvements phase 3: file explorer path navigation (date: 2026-09-04)

## Summary

Real path navigation in the file explorer side panel with back, forward, and "up one level" controls. A pure history reducer (`explorerHistory.ts`, 50-entry cap with oldest-first eviction) decouples navigation state from React rendering, enabling unit tests of all edge cases without a renderer. The `path` state in FileExplorer is now DERIVED from the history (`currentPath(history)`) rather than a second `useState`, eliminating sync bugs. Back/Forward controls disabled when at ends of range. Crumb bar redesigned: middle elision (root › … › last three) instead of wrapping, explicit hover targets and aria-labels, consistent `text-xs` sizing. Keyboard: `Alt+←` (back), `Alt+→` (forward) bound to the panel only, NOT window — xterm sends Alt+Arrow to the guest where readline uses it for word motion. History resets on project change and Project/Home scope toggle to prevent orphaned paths. No new dependencies; no server-bound paths constructed from history — crumb targets are server-listed segments and `parentOf` only truncates.

## Type

feature, phase 3 of 6 in v1.22.0

## Files changed

- `web/src/lib/explorerHistory.ts` (new) — pure reducer module: `History` interface (path + index), `pushPath` (idempotent: same object if path already current), `goBack`, `goForward`, `canGoBack`, `canGoForward`, `currentPath`, `parentOf`, `crumbTrail`; 50-entry cap dropping oldest-first; no React dependency, unit-testable in isolation
- `web/src/components/FileExplorer.tsx` — `path` now derived via `currentPath(history)` instead of second `useState`; Back/Forward/Up controls wired to history; `Alt+←`/`Alt+→` keyboard binds on panel capture phase; history reset on project change and scope toggle
- `web/src/components/FileExplorer.tsx` (crumb bar) — middle elision (root › … › last three) replacing line-wrapping; per-crumb hover backgrounds and real hit targets; explicit aria-labels ("Project root" / "Home root") for stable accessible naming independent of user-renameable project name; `text-xs` consistent sizing
- `web/src/components/icons.tsx` (new) — `ChevronLeftIcon`, `ArrowUpIcon` for navigation controls
- `web/src/components/ui.tsx` — `IconButton` gained `disabled:pointer-events-none disabled:opacity-30` for disabled navigation states
- `web/src/lib/explorerHistory.test.ts` (new) — 13 unit tests covering: initial state, pushPath idempotence and eviction, goBack/goForward bounds, canGoBack/canGoForward flags, currentPath derivation, parentOf edge cases, crumbTrail structure

## Agents dispatched

reviewer (phase 3 diff, returned APPROVE; found and deleted unused `Crumb.index` field).

## Verification

**Frontend:** tsc -b clean, oxlint clean, vitest 47/47 (13 new in explorerHistory.test.ts covering idempotence, bounds, eviction, crumb structure); no type errors. **Backend:** go vet and go test ./... all clean. **Browser e2e:** 70 checks (two path-navigation checks added: crumb click navigation, back/forward controls), 69 passed, 0 failed, 2 skipped; suite re-run after small refactor (unused field deletion), all green. **Testing README:** updated counts: 34→47 unit tests across five frontend files; 68→70 e2e checks.

## Edge cases & decisions

1. **History is idempotent.** Clicking the current crumb or re-selecting the same path from a listing calls `pushPath(history, samePath)`, which returns the same History object. No re-render or re-fetch is triggered; the click is a no-op at the React level. Verified by unit tests in `explorerHistory.test.ts`.

2. **History resets on tree changes.** Project change and Project/Home scope toggle both reset history to the new tree's root. Different trees mean orphaned paths (a path that existed in project A may not exist in project B). Decision made explicit in code comments.

3. **Keyboard binds on panel, not window.** `Alt+←` and `Alt+→` are bound to the FileExplorer panel's capture phase, NOT window. The rationale: xterm (browser terminal) forwards Alt+Arrow to the guest where readline binds them for word motion. Binding to window would steal those keystrokes. Binding to the panel (file explorer only) avoids the conflict while leaving xterm's Alt+Arrow intact for shell navigation.

4. **No new paths are constructed.** Crumb navigation targets are indices into the breadcrumb trail computed from segments the server itself listed. `parentOf` only truncates. No new server-bound paths are synthesized from history; paths are read-only slices of server data.

5. **Crumb elision over wrapping.** Long paths (> 3 segments after root) are elided in the middle (`root › … › seg_n-2 › seg_n-1 › seg_n`) rather than wrapping onto a second line. Elided crumbs keep the real absolute path on hover (aria-label for a11y, tooltip for discovery). Shrinks the listing viewport loss from variable height to fixed.

## Related

- Plan: `.agents/docs/plans/2026-09-04-ui-git-improvements-v1.22.0.md` (Phase 3 complete, phases 4–6 pending)
- Review: `.agents/docs/reviews/2026-09-04-review-phase3-explorer-nav.md` (APPROVE; unused field deleted)

## Requirements

n/a — path navigation is a UI feature with no new dependencies, auth, or deployment changes. Phases 4–6 (git integration) will map to R13 (guest git isolation). R11/R12 (guest isolation, networks) unaffected.
