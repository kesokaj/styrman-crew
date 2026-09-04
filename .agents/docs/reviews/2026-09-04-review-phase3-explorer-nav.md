---
owner: reviewer
date: 2026-09-04
---
# Review — phase3-explorer-nav   (2026-09-04)

Range reviewed: `/tmp/claude-1000/-home-kesokaj-workspace-styrman-crew/a1fcfb7b-6662-4bce-8e06-59c0ae200ac4/scratchpad/phase3.diff` (plan/README/e2e/FileExplorer/icons/ui.tsx diff hunks, plus the two new untracked files `web/src/lib/explorerHistory.ts` and `web/src/lib/explorerHistory.test.ts` appended in full)

## Verdict: APPROVE

Nothing broken. The `path` derivation from the new `history` reducer in `FileExplorer.tsx` was traced end to end (every read of `path`/`setPath`/`crumbs`, the `load()` fetch dependency, the project-switch and scope-switch reset effects, the crumb-bar and `../` row) and every call site was updated consistently — no stale-`path` reads, no dropped reset, no double-fetch. Ran the actual suite rather than only reading it: `npm test` → 47/47 passing (13 new in `explorerHistory.test.ts`, matching the plan/README claims), `npm run check` (tsc -b) clean, `npm run lint` (oxlint) clean. The only observation worth recording is a genuinely unused struct field, not a defect.

## Findings

- **[low / confidence: high]** `web/src/lib/explorerHistory.ts:57` — `Crumb.index` is computed and returned by `crumbTrail` but never read: not in `FileExplorer.tsx` (which only uses `c.name`/`c.path`) and not in `explorerHistory.test.ts` (which only asserts `.name`/`.path`). Per review criterion 1 (dead code), delete the field rather than keep an unread one; `grep -n "\.index"` against both consumers confirms zero references.

No correctness, security, or `REQUIREMENTS.md`/`AGENTS.md`/rules violations found. Specific things checked and confirmed sound:
- `path = currentPath(history)` is a pure derivation with no separate `useState` to fall out of sync — this is the *correct* fix for the antipattern the comment describes, not a new risk.
- `pushPath`'s identity-preserving no-op (`currentPath(h) === path` returns the same object) prevents a redundant crumb/entry click from re-rendering or re-triggering `load()`'s `[scope, path]` effect — verified by the "returns the SAME object" unit test and by reading `load`'s dependency array.
- The project-switch effect (`[project]`) still resets both `scope` and `history`/`path` to the root on every project change, same as the old `setPath("")` it replaced.
- The scope-switch (`Project`/`Home` tabs) handler resets history the same way, correctly discarding cross-tree back/forward entries.
- `goUp` (used by both the new "Up one level" button and the pre-existing `../` row) now goes through `pushPath` via `parentOf`, so Up participates in back/forward history exactly like the crumbs/directory clicks it's consistent with.
- `crumbTrail`'s elided crumbs keep their real absolute `path` (not an offset into the shortened list) — verified by both the unit test and by reading `FileExplorer.tsx`'s `onClick={() => setPath(c.path)}`.
- New e2e checks (`web/e2e/run.mjs`) were hand-traced against the reducer's actual transitions (back/forward/up/root-crumb sequence) and are consistent with `pushPath`/`goBack`/`goForward`'s real behavior, not just plausible-looking.
- Doc bookkeeping (`ui-git-improvements-v1.22.0.md` phase checkboxes, `testing/styrman/README.md` counts: 34→47 unit tests, 68→70 e2e checks) matches what was actually added.

## Criteria

1. Dead code — fail (minor): `Crumb.index` field never read, see finding above. Otherwise no orphaned exports, no commented-out code, no unreachable branches.
2. Flow tracing — pass: every changed call site (`setPath`, `goUp`, crumb clicks, directory-row clicks, keyboard handler, scope/project reset effects, `load()`'s dependency array) traced and consistent; no floating promises introduced, no stale closures (functional `setHistory` updates used throughout).
3. Standards conformance — pass: `tsc -b` and `oxlint` both clean; reducer module is plain typed functions with no `any`, discriminated-union-shaped `History`/`Crumb` types, test file sits beside its source per the `typescript-review` checklist.
4. Directory layout — pass: new files land in `web/src/lib/` alongside the project's other pure-logic modules (`api.ts`, `editorPane.ts`, `appearance.ts`), test co-located as `explorerHistory.test.ts`.
5. Simplicity (DIY-except-security) — pass: a plain, dependency-free reducer (stdlib-only); no speculative abstraction beyond the one unused field noted above; extracting it out of the component is what made it unit-testable at all (13 tests with no React renderer needed).
6. Decoupling — pass: `explorerHistory.ts` has one consumer (`FileExplorer.tsx`), a small pure-function interface, and its own test file — a second engineer touching, say, the crumb-bar markup or the upload flow in the same component would not collide with this module.
