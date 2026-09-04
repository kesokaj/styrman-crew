---
owner: reviewer
date: 2026-09-04
---
# Review — phase2-appearance   (2026-09-04)

Range reviewed: `/tmp/claude-1000/-home-kesokaj-workspace-styrman-crew/a1fcfb7b-6662-4bce-8e06-59c0ae200ac4/scratchpad/phase2.diff`
(plan file, testing README, `web/e2e/run.mjs`, `web/package.json`, `CodeEditor.tsx`,
`FileEditor.tsx`, `TerminalPane.tsx`, `index.css`, `lib/hooks.ts`, `lib/terminal.ts`,
`routes/settings.tsx`, plus the untracked `web/src/lib/appearance.test.ts` appended
at the end of the diff file). `web/dist` and `package-lock.json` are modified in the
working tree but are not part of this diff and were not reviewed.

## Verdict: APPROVE WITH NITS

The diff is sound: the appearance store, its sanitisation, the live-apply paths in
`TerminalPane`/`CodeEditor`, and the new unit + e2e tests all trace correctly end to
end, and `npm test`, `npm run lint`, and `npm run check` all pass clean (33/33 tests,
0 lint errors, 0 type errors). The plan-file claims (test counts, the `p.$project.$term.tsx:30`
citation, the R9 rationale for the CSS comment rewrite) were checked against the
actual code and are accurate. The one real issue is a small piece of dead code left
behind by the test seam.

## Findings
- **[low / confidence: high]** `web/src/lib/hooks.ts:331` (`__reloadAppearanceForTests`) — exported test seam with zero callers anywhere in the repo (`grep -rn __reloadAppearanceForTests .` finds only the definition). The new `appearance.test.ts` uses the `vi.resetModules()` + fresh-`import` pattern instead, so this function is unreachable dead code. Delete it, or if it is meant for a future test, wire up at least one caller now.

## Criteria
1. **Dead code** — fail (one item, see above); everything else added is reachable and used (`DEFAULT_TERM_FONT_*`, `DEFAULT_EDITOR_FONT_*`, `FONT_STACKS`, `FONT_FAMILY_LABELS` all have live callers/tests).
2. **Flow tracing** — pass. `lib/hooks.ts` (`useAppearance`/`setAppearance`/`FONT_STACKS`) → `TerminalPane.tsx` (mount opts + live-apply effect reusing `onResizeRef`) and → `FileEditor.tsx` → `CodeEditor.tsx` (`fontComp` compartment, defaulted for the credential-viewer call site in `settings.tsx:716` which correctly passes no font props) all agree in shape and defaults. `localStorage` access is wrapped for both read and write (private-window `SecurityError` on access, not just write) and errors never propagate. No floating promises introduced.
3. **Standards conformance** — pass. Compartment-based reconfigure mirrors the existing `wrapComp`/`themeComp` pattern; `Object.hasOwn` used correctly instead of `in` to block prototype-key injection (`"toString"`); size sanitisation is a total function with a fallback per field.
4. **Directory layout** — pass. New test file sits beside `hooks.ts` inside `web/src/lib/`, matching the project's "tests beside the code" convention; no stray files.
5. **Simplicity** — pass. No new runtime dependency beyond the justified font package (measured +89.8 KB, recorded in the plan against a stated cutoff); one `localStorage` key with per-field sanitisation rather than per-key storage or a new abstraction layer.
6. **Decoupling** — pass. `FONT_STACKS` is the single place a family pref becomes a CSS string, preventing the terminal and editor surfaces from drifting; `TerminalPane`/`CodeEditor`/`settings.tsx` each only import what they consume from `hooks.ts`.
