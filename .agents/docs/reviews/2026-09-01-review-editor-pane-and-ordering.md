# Review — editor-pane, terminal ordering, Dockerfile highlighting, e2e web build   (2026-09-01)

Scope: the uncommitted working tree vs `HEAD` (c57abb2) — `internal/conversation/store.go|store_test.go`,
`web/src/components/{CodeEditor,FileExplorer,SidePanel}.tsx`, `web/src/routes/{p.$project.$term,settings}.tsx`,
`web/e2e/{run.sh,run.mjs}`, new `web/src/components/{FileEditor,EditorPane}.tsx`,
new `web/src/lib/editorPane.ts`, plus the untracked changelog entry and the ADR-048 append.
Rebuilt `web/dist/client` assets are treated as build output, not reviewed line-by-line.

## Verdict: REQUEST CHANGES

The blocking problem is that the pane rewrite dropped a safety invariant the modal had: `closeIfOpen`
in `FileExplorer.tsx` still only clears the **modal** viewer, so deleting or renaming a file that is
open in the new editor pane leaves it live with a stale path — and its own comment says exactly what
happens next ("Save on the stale path would recreate what the user just removed"). Because the pane
is now the primary editor, this is a regression on the path everyone will actually use, and it silently
resurrects deleted content. Below that sit two smaller pane bugs (the "Discard and open" confirm
dead-ends on an empty pane; "New file" still opens the old modal on top of the terminal), a blast-radius
miss on the ordering change (`firstTerminalId` no longer lands on the most recent terminal), and the
same stale-asset hazard the e2e fix closed still being wide open in `deploy/Dockerfile.server`.
The three things the operator asked me to scrutinise hardest all came back clean: `naturalLess` is a
valid strict weak ordering (fuzz-verified), the basename `matchFilename` fix is verified correct in node
against the real `@codemirror/language`, and the render-phase reset is correct React with no stale-closure
or double-render hazard.

## Findings

- **[high / confidence: high]** `web/src/components/FileExplorer.tsx:134-142,165,173` — `closeIfOpen()`
  only calls `setOpen(null)`, i.e. it closes the *modal*. Nothing clears the editor **pane**. Delete or
  rename a file that is open in the pane and the pane keeps showing it under the old path; `Save`
  (button or Ctrl-S) then writes it back — recreating a deleted file, or leaving a second copy at the
  pre-rename path. This is precisely the hazard the existing comment documents, now unguarded on the
  primary editing surface. Fix: add a `closeFileInPane(project, term, path)` to `web/src/lib/editorPane.ts`
  (clear `file`, and prefix-match `path + "/"` for directories exactly as `closeIfOpen` does) and call it
  from `closeIfOpen` alongside `setOpen`. Add an e2e check: open a file in the pane, delete it from the
  explorer, assert the pane no longer offers Save on that path.

- **[medium / confidence: high]** `web/src/components/EditorPane.tsx:80-86` (with the effect at `:51-78`)
  — "Discard and open" leaves the pane empty. `discardAndOpen()` does `setLoaded(null); setFile(next)`,
  but `next` is the *same object identity* already in the store, so `set()` hits its own no-op bail-out
  (`editorPane.ts:51`) and never emits; `wanted` therefore does not change, and the loader effect —
  whose dep array is `[wanted]` with `loaded` deliberately excluded — does not re-run. Result: the pane
  renders "No file open." while the store points at the file the user just chose to open; only clicking
  the file again (a fresh object) recovers it. The comment "forces the effect above to load it fresh" is
  wrong. Fix: add `loaded` to the dep array and delete the eslint-disable — the `same` early-return
  already makes the effect idempotent — or extract the fetch into a function `discardAndOpen` can call
  directly. Not covered by any of the 7 new e2e checks.

- **[medium / confidence: high]** `web/src/components/FileExplorer.tsx:144-150` — `createFile()` still
  does `setOpen(await api.readFile(...))`, so **New file** opens the old centered modal on top of the
  terminal even when docked next to one, while clicking an existing file opens the pane. Two different
  editors for two ways of reaching the same file. Fix: route it through `openEntry(p)` after `load()`.

- **[medium / confidence: high]** `web/src/lib/projects.ts:6-9` — the ordering change has a fourth
  consumer beyond the three Go callers that were checked. `firstTerminalId()` returns `terminals[0].id`,
  and that is the "open a project" entry point for both the Sidebar and the dashboard. With `List` sorted
  by `LastOpened` desc it landed the user on the terminal they last used; it now lands them on the
  alphabetically-first title (titles default to random "adjective noun" pairs, `internal/conversation/names.go:29`,
  so this is effectively arbitrary). If resuming the most recent terminal is still wanted, keep it explicit:
  have the server expose the recency choice, or pick by `max(lastOpened)` client-side. Either way this is
  a user-visible behavior change that neither the changelog nor the ADR mentions.

- **[medium / confidence: high]** `web/src/components/FileEditor.tsx:119` — `previewing` is initialized
  from `isMarkdown` once and never reset with `shownPath`. In the modal this was invisible (the component
  unmounted between files), but the pane stays mounted: open a `.go` file, then a `README.md`, and the
  markdown opens in raw edit mode instead of the documented rendered preview. `previewHtml` is likewise
  stale for a frame. Fix: set both inside the existing render-phase block —
  `setPreviewing(/\.(md|markdown)$/i.test(file.path)); setPreviewHtml(undefined);`.

- **[medium / confidence: medium]** `deploy/Dockerfile.server` — the e2e fix is right but partial. That
  Dockerfile does `COPY . .` + `go build`, so the production image embeds whatever `web/dist/client` is
  committed at that moment; nothing verifies it matches `web/src`. That is the identical failure mode
  `run.sh` just fixed, on the path that actually ships. Cheapest guard: add
  `cd web && npm run build && git diff --exit-code -- web/dist` to `/preflight`. Related and immediate:
  this change deletes 14 tracked `web/dist/client/assets/*` files and adds 14 untracked ones — a
  `git commit -a` would commit the deletions without the additions and ship a UI that 404s its own
  bundle. Stage with `git add -A -- web/dist`. (`integration.sh` correctly needs no change — it is
  API-only.)

- **[low / confidence: high]** `internal/conversation/store.go:265-270` — the ID tie-break only fires when
  the two titles are *byte-identical* (`out[i].Title != out[j].Title`). Titles that `naturalLess` treats
  as *equivalent but not identical* — "Apple"/"apple", "Term 07"/"Term 7" — fall through to a comparator
  that returns false both ways, and `sort.Slice` is not stable, so their relative order is arbitrary and
  can flip whenever the backing slice changes (create/delete/rename). That is a small echo of the
  reshuffling this change set out to kill. Fix:
  `if naturalLess(a,b) { return true }; if naturalLess(b,a) { return false }; return naturalLess(idA, idB)`.

- **[low / confidence: high]** `internal/conversation/store.go:29,124-133` — with `List` no longer sorting
  on it, `LastOpened` is now written and never read: no Go reader, and the frontend only declares the
  field in `web/src/lib/api.ts:62`. `Touch` (called per WS attach, `internal/terminal/bridge.go:114`) still
  does a full atomic JSON save for it. By this file's own precedent — the `AgentSession` field was deleted
  in v1.14.0 with the note "written but never read" (`store.go:16-19`) — this is dead weight: delete
  `LastOpened` + `Touch` + the API field, or record in the ADR why it is deliberately retained.

- **[low / confidence: high]** `.agents/docs/architecture/DECISIONS.md:1670-1717` — ADR-048 was inserted
  *above* ADR-047's trailing `Source:`/`Verification:` footer instead of at the end of the file. ADR-047
  now has no footer and ADR-048 appears to have two, the second pointing at the harness-upgrade changelog.
  In an append-only document this is exactly the kind of drift that misleads a zero-context reader. Move
  the ADR-047 footer back under ADR-047.

- **[low / confidence: high]** `web/src/lib/editorPane.ts:11-13`, `web/src/routes/p.$project.$term.tsx:26-28`,
  `DECISIONS.md` ADR-048 — the recorded rationale is backwards. The route component is **not** remounted
  on a terminal switch: `MatchInner` only supplies a React `key` when `remountDeps`/`defaultRemountDeps`
  is configured (`@tanstack/react-router` `dist/esm/Match.js:81-95`), and neither is set anywhere in
  `web/src`. That is also why `TerminalPane` needs its own explicit `key`. The module store is still the
  right call — but for the *opposite* reason: state survives the switch, so plain `useState` would leak
  terminal A's view onto terminal B. ADR-048 declares this pattern "the precedent, not a one-off", so the
  wrong reason will be copied. Also note `editorUsed` (a ref latched during render) therefore persists
  across terminal switches — harmless, but not what the comment implies.

- **[low / confidence: medium]** `web/src/routes/p.$project.$term.tsx:120-140` — switching Editor → Terminal
  does not return keyboard focus to the terminal. The old modal did (`useModalA11y` restores focus to the
  opener, `ui.tsx:90-95`); the toggle has no equivalent, so focus lands on `<body>` and the first keystrokes
  go nowhere until the user clicks the terminal. The new e2e check hides this by calling
  `.xterm textarea .focus()` explicitly before typing (`run.mjs`, "terminal session survived…"). Fix: on
  transition to `view === "terminal"`, call the terminal's `focus()` (expose it from `TerminalPane`, which
  already holds `styrRef.current.term`), and drop the explicit focus from the e2e check so it guards the
  real behavior.

- **[low / confidence: high]** `internal/conversation/store.go:211-243` — `naturalLess` is ASCII-only:
  `lower()` folds A–Z only and everything else is compared bytewise. For UTF-8 that yields code-point
  order, which is coherent, but for user-settable titles on a Swedish operator's box it means "Ärende"
  sorts after "Zebra" and "Åke"/"åke" no longer fold together — while the doc comment promises
  "case-insensitive" flatly. Either fold with `unicode.ToLower` over runes, or say "ASCII case-insensitive;
  non-ASCII sorts by code point" in the comment. Do **not** pull in `x/text/collate` for this.

- **[low / confidence: high]** `web/src/components/FileEditor.tsx:20,49` — `FORMATTABLE` and `extOf` are
  exported but referenced only inside their own file (`fmtSize` and `useFileEditorState` are the genuinely
  shared ones). Drop the `export` keyword.

- **[low / confidence: high]** `web/src/components/EditorPane.tsx:41` — `onSaved` is a no-op, so saving in
  the pane never refreshes the explorer listing; the modal passes `load` and does. Thread a refresh callback
  (or have the pane bump the same generation the explorer's `load()` watches) so size/mtime don't go stale.

- **[low / confidence: high]** docs drift on the counts the project treats as authoritative: `AGENTS.md`
  ("`run.mjs`'s 34 Playwright checks") and `.agents/docs/testing/styrman/README.md:49,61` ("36 checks…
  34 + 2 auto-SKIP") both predate the 7 new checks and the reported 42/0/2. `AGENTS.md` points at that
  README as the source of truth for counts, so both need the update in this same commit (documentation
  DoD row). Nit while you are there: the changelog says `FileExplorer.tsx` "shrank from 727 lines to 521";
  `git show HEAD:` puts the original at 710.

- **[low / confidence: high]** `web/e2e/run.sh:35-39` — the fix is correct, but note it makes an e2e run
  mutate *tracked* files: every run rewrites `web/dist/client` with fresh content hashes, so `git status`
  is dirty afterwards. Worth one line in the script's header comment so nobody mistakes it for stray edits.

- **[info / confidence: high]** Keeping both panes mounted is sound, and I checked the two failure modes
  worth worrying about. (1) Resize: hiding via Tailwind `hidden` (`display:none`) fires the `ResizeObserver`
  in `TerminalPane.tsx:244`, but `FitAddon.fit()` in `@xterm/addon-fit@0.11.0` early-returns on the `NaN`
  dimensions a display-none parent produces, and the `cols < 10 || rows < 3` guard at `:232` blocks any
  bogus resize frame reaching tmux — so no session-size corruption. (2) A11y: `display:none` removes both
  subtrees from the accessibility tree and the focus order, so the hidden terminal and the hidden CodeMirror
  cannot take focus or keystrokes — `hidden` is the right choice over `visibility`/`opacity`. Residual cost
  is that the hidden terminal keeps consuming WebSocket output and running its rAF render loop; that is the
  deliberate price of keeping the session alive, and it is bounded to one terminal.

### Verified sound (recording these so they are not re-litigated)

- `naturalLess` is a valid strict weak ordering. I extracted it and fuzzed ~200k triples over an alphabet
  of digits/letters/case/space/dot/dash for irreflexivity, asymmetry, transitivity of `<`, **and**
  transitivity of equivalence — no violations. Leading zeros, all-digit strings, empty strings, prefixes,
  and 400-digit runs all behave. Byte indexing is safe for UTF-8: no byte ≥ 0x80 can be an ASCII digit, so
  multi-byte sequences are never split (the ordering caveat is the separate low finding above).
- The `matchFilename` fix is correct. Verified against the installed `@codemirror/language`:
  `projects/x/Dockerfile → NONE` but `Dockerfile → Dockerfile`, and `a/.bashrc → NONE` but
  `.bashrc → Shell`; the basename form also handles `Dockerfile.dev`, `Containerfile`, lowercase spellings,
  and `x.dockerfile`. (`Dockerfile.json` resolves to the Dockerfile grammar because filename regexes are
  tested before the extension fallback — pre-existing, negligible.)
- The render-phase reset in `useFileEditorState` is the correct fix and is correct React. `CodeEditor`'s
  mount effect is keyed `[filename, readOnly]` and builds `new EditorView({doc: value})` from that render's
  closure, so an effect-based reset genuinely was one render too late. Setting state during render of the
  *same* component re-renders before children render or effects fire; no double-render or stale-closure
  hazard. Its only gap is `previewing`/`previewHtml`, filed above.
- The `Compartment`-based `wrap` reconfiguration is the right mechanism (no remount ⇒ cursor, scroll and
  undo survive), and reusing one `Compartment` ref across successive `EditorView` instances is fine.
- No new dependencies (`web/package.json` untouched, `go.mod` untouched) — constraint honored.

## Criteria

1. **Dead code — fail (minor).** `LastOpened` is now write-only; `FORMATTABLE`/`extOf` are exported with no
   external consumer. Note also that the changelog's and ADR's stated justification for keeping the modal
   ("still used by Settings") is false — Settings renders `CredViewer`/`CodeEditor` directly, never
   `FileExplorer`; the only surviving entry point into the modal is `createFile`, which itself should move
   to the pane (finding 3). Once it does, `FileViewer` and its modal chrome become deletable — decide
   deliberately rather than leaving a second editor reachable only by accident.
2. **Flow tracing — fail.** Two broken hops: `closeIfOpen` never reaches the pane (high finding), and
   `discardAndOpen` relies on an effect that cannot re-run (medium). One missed consumer of the reordering:
   `firstTerminalId`. Everything else traced clean — `List`'s three Go callers genuinely don't depend on
   recency, `SidePanel` is the sole `FileExplorer` host and always passes `term`, and error paths are
   consistently routed through `tryUI`.
3. **Standards conformance — pass with nits.** Go: gofmt/vet-clean, stdlib-only, comments explain *why*.
   TS/React: hooks unconditional and above the early returns, no floating promises of consequence, `unknown`
   narrowing preserved. Nits: two refs written during render (`editorUsed`, `dirtyRef`) — works, and
   `dirtyRef` is justified in place, but both are outside React's documented purity contract; and
   `openEntry` returns `Promise | undefined` depending on the branch, which is why one call site needs
   `void` and the other doesn't. `oxlint` re-run here: clean.
4. **Directory layout — pass.** `FileEditor.tsx`/`EditorPane.tsx` under `web/src/components/`, the store
   under `web/src/lib/` next to the existing `projects.ts` module store, Go tests beside their source. An
   unfamiliar engineer would find all four where they expect them.
5. **Simplicity (DIY-except-security) — pass.** No new dependencies; `naturalLess` is ~30 lines of stdlib
   rather than a natural-sort package — the right call. No auth/crypto/session/JWT surface is touched, so
   the security inversion doesn't apply. The one avoidable complexity is `EditorPane`'s three-way
   `wanted`/`loaded`/`pending` state machine, which is where both pane bugs live; folding the fetch into a
   named function called from both the effect and `discardAndOpen` would shrink it and fix finding 2 at once.
6. **Decoupling — pass.** Extracting `useFileEditorState` + toolbar + body genuinely gives one implementation
   with two mount points, and `FileExplorer` lost 189 lines to it. The module store keyed `${project}/${term}`
   is a deep, narrow seam that two engineers could work either side of. The one new coupling —
   `FileExplorer → lib/editorPane` — is explicit, prop-gated, and appropriate.

## Verification assessment

The gates that were run (`go build`/`vet`/`test -race`, the separate ingress module, `tsc -b`, `oxlint`,
`integration.sh` 57/0/3, e2e 42/0/2) are the right set and the 7 new e2e checks are well chosen — in
particular "terminal session survived the editor round-trip" pins the load-bearing never-unmount constraint.
Three gaps: (a) no check covers delete/rename-while-open or discard-and-open, the two paths that are broken;
(b) the terminal-survival check calls `.focus()` explicitly, which masks the lost-focus regression it would
otherwise have caught; (c) no gate exists for the committed-`dist` staleness on the *deploy* path — the very
class of bug this change set fixed for e2e.
