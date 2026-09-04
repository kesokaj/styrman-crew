# UI + git improvements phase 2: appearance settings (terminal and editor fonts) (date: 2026-09-04)

## Summary

Settings → Appearance gains terminal and editor font size + family controls. Appearance store modeled on the existing theme store — module-level + `useSyncExternalStore` + localStorage with ONE key `styrman.appearance.v1` holding a JSON object with per-field fallback and deliberate per-surface defaults (terminal 13px/Google Sans Code, editor 12px/bare system stack kept different to avoid silent restyling on upgrade). Terminal font live-applies via `onResizeRef` path; editor font changes persist and apply on next mount (route unmounts the page, so no in-buffer live-apply is possible — decision anchored to that constraint). Bundle delta 89.8 KB (2357778 → 2449700 B; five JetBrains Mono woff2 subsets at 84216 B), under the 150 KB plan cutoff, so the `@fontsource-variable/jetbrains-mono` dependency ships. Scope correction: the plan promised editor font live-update on an open buffer but `/settings` is a separate route (unmounts terminal + editor), so the preference persists and applies on NEXT mount — the `fontComp` compartment in CodeEditor is kept as a correctness measure. Reviewer returned APPROVE WITH NITS (one dead export deleted), security found and fixed three low findings (font string injection, CDN verification, const mutability). Unit/integration/e2e all green (tsc -b, oxlint, vitest 34/34, go vet, go test, e2e 67/67 passed + 2 skipped = 68 total).

## Type

feature, phase 2 of 6 in v1.22.0

## Files changed

- `web/src/lib/fonts.ts` (new) — `FontFamilyPref` union (google-sans-code | jetbrains-mono | system), `FONT_STACKS` lookup table, `FONT_FAMILY_LABELS`, size bounds; no React, reusable by terminal and editor consumers
- `web/src/lib/hooks.ts` — new appearance store at `:176-223`, module-level + `useSyncExternalStore` + localStorage; re-exports font vocabulary from `fonts.ts`
- `web/src/lib/terminal.ts` — `createTerminal` takes optional `fontSize` and `fontFamily` parameters
- `web/src/components/TerminalPane.tsx` — live-apply effect reusing `onResizeRef` refit-and-ship path
- `web/src/components/CodeEditor.tsx` — new `fontComp` Compartment alongside `themeComp`/`wrapComp`; only the two font rules moved from the static theme; optional `fontSize`/`fontFamily` props
- `web/src/components/FileEditor.tsx` — wiring + `FontRows` component (size slider + family radiogroup with per-face preview)
- `web/src/routes/settings.tsx` — wiring + `FontRows` component rendered in Appearance settings
- `web/src/index.css` — bundles `@fontsource-variable/jetbrains-mono` import, corrects stale "R9" citation (R9 rewritten 2026-08-30 to mean deployment target, so now states no-CDN property directly)
- `web/package.json` — adds `@fontsource-variable/jetbrains-mono` (one new npm dependency)

## Decisions recorded

1. **Per-surface defaults deliberately differ.** Terminal: 13px/Google Sans Code (unchanged). Editor: 12px/bare system stack (unchanged). Flattening them to one default would have silently restyled every existing user's editor on upgrade. A test (`appearance.test.ts:…`) asserts they stay different.

2. **Editor font live-update is not reachable.** The plan promised in-buffer cursor/scroll/undo survival when the editor font changes while Settings is open. That IS implemented via the `fontComp` compartment in CodeEditor. But `/settings` is a separate route, so opening it unmounts the terminal page and its editor (`p.$project.$term.tsx:30-33`). What ships: the preference persists and applies on next mount. The compartment is kept as a correctness/robustness measure for the store's contract, not user-visible behaviour. Moving controls next to the sidebar theme radio would make live-apply real but was deliberately not done — operator requested Settings, not a sidebar redesign.

3. **FontFamily must be typed, not a string.** Security finding #1: `fontFamily?: string` was unconstrained and CodeEditor/terminal concatenate values into CSS with no escaping. Now `FontFamilyPref` union with `FONT_STACKS` lookup done inside the consumers, so no arbitrary string reaches CSS.

4. **CDN imports now gated at release.** Security finding #2: "never a CDN" rested only on a code comment. Deploy gate in `deploy/verify-commit.sh` now greps built CSS for absolute `url(http…)` and fails the release gate if found. Verified both directions (clean build, catches a planted CDN import).

5. **Appearance defaults are frozen.** Security finding #3: `APPEARANCE_DEFAULT` is now `Object.freeze`d, matching its docstring.

## Agents dispatched

reviewer (phase 2 diff, APPROVE WITH NITS); security (phase 2 diff, three findings all fixed).

## Verification

**Frontend:** tsc -b, oxlint, vitest 34/34 (17 new in `appearance.test.ts` covering defaults, fallback, per-surface divergence, localStorage round-trip, corrupted JSON, missing key); backend: go vet, go test ./... all clean. **Build:** runtime image AMD64 confirmed carries no external font URLs (visual + deploy gate pass). **Bundle:** delta measured at 89.8 KB (five JetBrains Mono woff2 subsets, 84216 B), verified under 150 KB cutoff; npm audit 0 vulns; committed dist woff2 sha256-identical to upstream; dependency ships no JavaScript and no install hooks. **Browser e2e:** 67 passed, 0 failed, 2 skipped = 68 total checks (6 appearance checks added: theme controls, terminal font live-apply, editor font persistence, family preview buttons); suite re-run after security refactor, all green. **Edge cases:** Two e2e checks were wrong on first write: one used a file a later check deletes (caught by running), one forgot `page.goto` resets module-level pane store (caught by running). Both fixed before commit.

## Scope correction note (honest log)

The plan promised: "The editor font live-updates an already-open buffer, matching the terminal. Cursor, scroll, and undo survive." Implementation is ready (the `fontComp` Compartment), but the contract is not reachable: Settings is a route, and opening it unmounts the terminal page (`p.$project.$term.tsx` unmounts on route change at lines 30–33), which unmounts its editor. What ships: the preference persists across mounts and applies on next open — the `fontComp` compartment exists and works correctly, so no follow-up is needed unless the operator moves these controls into the sidebar (which would keep the terminal page mounted). Marked as expected and intentional in the plan's Phase 2 notes.

## Related

- Plan: `.agents/docs/plans/2026-09-04-ui-git-improvements-v1.22.0.md` (Phase 2 complete, phases 3–6 pending)
- Review: `.agents/docs/reviews/2026-09-04-review-phase2-appearance.md` (APPROVE WITH NITS: one dead `__reloadAppearanceForTests` export deleted)
- Security review: `.agents/docs/reviews/2026-09-04-security-phase2-appearance.md` (three low findings: font injection, CDN gate, const mutability — all fixed)

## Requirements

n/a — appearance preferences are user-facing but do not require `REQUIREMENTS.md` changes. R9 (no CDN fonts) is reinforced by the new deploy gate; R11/R12/R13 (guest isolation, networks, git) remain unaffected. Later phases (explorer, git integration) will map to R13.
