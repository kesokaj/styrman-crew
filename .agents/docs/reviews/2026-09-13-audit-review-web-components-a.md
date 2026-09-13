---
owner: reviewer
date: 2026-09-13
---
# Audit — web/src/components (Browser/Files/Git/Terminal/Sidebar/Editor)   (2026-09-13)

Scope: `web/src/components/BrowserPane.tsx`, `web/src/components/FileExplorer.tsx`,
`web/src/components/SidePanelGit.tsx`, `web/src/components/TerminalPane.tsx`,
`web/src/components/Sidebar.tsx`, `web/src/components/FileEditor.tsx`,
`web/src/components/CodeEditor.tsx` — HEAD `e56e775` (clean tree). This is an
audit against recorded intent (`AGENTS.md`, `REQUIREMENTS.md` R1–R13, the
three newest `.agents/docs/changes/` entries, `.agents/docs/specs/styrman/`,
`.agents/docs/bugs/INDEX.md`), not a diff review — no prior commit range was
handed to this dispatch, and none of the last three `changes/` entries (all
about v1.29.0's Browser view review fixes) modify `SidePanelGit.tsx` or
`FileEditor.tsx`, so those two got no fresh review pass in that cycle. Every
other file here (`BrowserPane.tsx`, `TerminalPane.tsx` indirectly via the wire
protocol) was reviewed 3+ rounds in that same cycle (see BUG-042..052) and no
further defect was found in them beyond what is listed below.

## Verdict: REQUEST CHANGES

One real, concrete, high-severity defect: `SidePanelGit.tsx`'s **Discard**
action rewrites the working tree at a path (git `checkout --`/`clean -fd`)
but never evicts that path from any open editor pane — the exact hazard this
same file's own `treeChanged()` helper exists to prevent for every OTHER
tree-rewriting verb (checkout, pull, stash push/pop), and the exact class of
bug fixed for rename/delete/upload elsewhere in this codebase. A related,
narrower gap: a conflicting `stash pop` (server returns 409 but still writes
conflict markers into the tree, by the server's own comment) skips
`treeChanged()` because it is only called on the success path. Everything
else is either already covered by the very thorough v1.29.0 review cycle or
did not surface a scenario with a concrete failing case.

## Findings

- **[high / confidence: high]** `web/src/components/SidePanelGit.tsx:518-535` —
  the Discard action calls `api.gitDiscard(project, discarding.path,
  discarding.untracked)` and nothing else (`run(() => api.gitDiscard(...))`,
  no `treeChanged()`/`evictFromPanes` call). Server-side, `gitDiscard`
  (`internal/httpapi/git.go:314-330`) runs `git checkout -- <path>` (tracked)
  or `git clean -fd -- <path>` (untracked) — it rewrites or deletes the file
  on disk. If that same path is open in an editor pane (any terminal, via
  `FileExplorer`/`openFileInPane`), the pane's buffer is now stale: a clean
  buffer that matched the pre-discard content will report as clean (matches
  `savedContent`) even though disk no longer matches, and hitting Save writes
  the just-discarded content straight back to disk — silently undoing the
  discard the user just confirmed. This is precisely the hazard this file's
  own `treeChanged()` docstring describes ("Save would otherwise write
  pre-switch content over the new tree") and that `closeFileInPane`/
  `evictFromPanes` exist to close for rename/delete/upload. Fix: call
  `evictFromPanes(project, { scopeKind: "project", path: discarding.path })`
  (imported already for `openDiffInPane`'s sibling helpers; only
  `evictCleanProjectPanes`/`notifyFileSaved`/`openDiffInPane` are currently
  imported from `../lib/editorPane` — add `evictFromPanes`) after a
  successful discard, mirroring the single-path eviction already used for
  uploads in `FileExplorer.tsx:219-225`.

- **[medium / confidence: high]** `web/src/components/SidePanelGit.tsx:429-437` —
  the `stash pop` handler calls `treeChanged()` only after `await
  api.gitStash(project, "pop")` resolves. The button's own tooltip and the
  server (`internal/httpapi/git.go:797-805`, comment: "A conflicting `pop`
  exits non-zero, writes conflict markers into the tree AND KEEPS the stash
  entry") both document that a conflicting pop rewrites the tree but answers
  409. `run()`'s `catch` branch (line 91-95) only toasts the error; it never
  calls `treeChanged()`. So on a conflicting pop, any open clean editor
  buffer for a conflicted path is left showing pre-conflict content, and
  Save would overwrite the conflict markers — the same data-loss shape as
  the Discard finding above, just gated on the rarer conflict outcome. Fix:
  call `treeChanged()` for the stash-pop 409 case too (e.g. call it before
  re-throwing inside a local try/catch around just that verb, or have the
  server distinguish "conflict, tree changed" from other errors and always
  evict on that specific status).

- **[low / confidence: high]** `web/src/components/BrowserPane.tsx:45` —
  `TargetInfo.attached?: boolean` is declared but never read anywhere in the
  file (`grep -n "\.attached"` finds no use). Dead field on an internal type;
  drop it, or wire it in if it was meant to filter `Target.getTargets`
  results.

## Criteria

1. **Dead code** — one low finding (unused `TargetInfo.attached` field in
   `BrowserPane.tsx`); nothing else unreferenced found across the seven files.
2. **Flow tracing** — mostly clean and unusually well-documented (close-code
   handling in `BrowserPane.tsx`/`TerminalPane.tsx` matches
   `terminal-protocol.md` exactly, including the 4002/4003/4004/1009
   semantics and the one-automatic-retry-on-503 rule). The one real break in
   the chain is `SidePanelGit.tsx`'s Discard action, which calls the mutating
   endpoint but drops the caller-side half of the "tree changed → evict
   panes" contract that every sibling verb in the same file honours (see
   findings). `go vet ./...`/`npx tsc -b` both clean.
3. **Standards conformance** — TypeScript/`frontend` idioms followed
   throughout (stable refs for imperative WS/CDP state, compartmentalized
   CodeMirror reconfiguration instead of remounts, `AbortController` used
   correctly in `FileEditor.tsx`'s `runCheck`, no floating promises found —
   every `.catch()`/`await` is accounted for). `npm run check` (tsc) passes
   clean on the current tree.
4. **Directory layout** — all seven files sit correctly under
   `web/src/components/`; no stray placement.
5. **Simplicity** — no hand-rolled auth/crypto in scope; CodeMirror's
   grammar list is deliberately hand-curated (justified in a comment) rather
   than pulling the 120-grammar catalogue — reasonable, stdlib/vetted-lib
   trade-off, not flagged.
6. **Decoupling** — `FileExplorer`/`FileEditor`/`SidePanelGit` communicate
   through the `editorPane.ts` seam (`openFileInPane`, `evictFromPanes`,
   `closeFileInPane`, `notifyFileSaved`) rather than reaching into each
   other; that seam is exactly where the Discard finding falls through a
   gap — the module boundary is sound, one call site just omitted the
   contract other call sites uphold.

## Not examined

- `internal/httpapi/git.go` beyond the two handlers cited above (not in the
  dispatched file list; read only to confirm the server-side behavior behind
  the two findings).
- `web/src/lib/editorPane.ts`, `web/src/lib/api.ts`, `web/src/lib/terminal.ts`,
  `web/src/lib/browserInput.ts`, `web/src/lib/cdp.ts` — read only as much as
  needed to confirm call signatures for the findings above; not reviewed line
  by line in their own right.
- No component-level test file exists for `SidePanelGit.tsx` (none found
  under `web/src/components/`), so the Discard gap above has no test
  currently exercising it either way — not separately flagged as a distinct
  finding since the missing-eviction bug is the more direct, higher-value
  report.
- `web/e2e/`, `internal/orchestrator/ports.go`, `internal/httpapi/browser.go`,
  `image/configs/styrman-browser` — out of scope for this dispatch's file
  list; taken on faith from the already-extensive v1.29.0 review trail
  (BUG-042..052) rather than re-audited here.
- Did not run `npm test`/vitest or the integration/e2e suites in this pass
  (relied on `npm run check` only, plus static reading) — a genuine test run
  covering the Discard/stash-pop-conflict scenario is the natural next step
  to confirm the findings above reproduce as described.
