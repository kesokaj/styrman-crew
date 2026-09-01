# Review — editor-ux (open-file highlight, Recent strip, header basename) (2026-09-01)

Scope: the uncommitted working tree at HEAD `ad1a4be` (v1.17.0) — `web/src/lib/editorPane.ts`,
`web/src/components/FileExplorer.tsx`, `web/src/routes/p.$project.$term.tsx`, plus the rebuilt
`web/dist/client`. Reviewer did not author this change. `web/dist` was verified in sync with the
sources (`styrman.recentFiles` present in `assets/p._project._term-DdrHDXMa.js`; every asset
referenced by `_shell.html` exists on disk).

## Verdict: REQUEST CHANGES

The change breaks the existing browser e2e suite in two places, and both are deterministic, not
flaky. Appending the basename _inside_ the Editor toggle button changes that button's accessible
name from `Editor` to `Editor nested.txt`, so `web/e2e/run.mjs:246`
(`getByRole("button", { name: "Editor", exact: true })`) can no longer resolve while a file is
open; and the new Recent chips duplicate file basenames inside the same `<aside>`, so
`panel().getByText("Dockerfile", { exact: true }).click()` at `web/e2e/run.mjs:369` now matches two
elements and dies on a Playwright strict-mode violation. The DoD verifier row cannot go green as
written, so this cannot ship. Everything below that is smaller: a pre-existing dirty-flag bug that
the new basename makes actively misleading, a Recent strip that will eat two to three rows of a
320px panel, and a set of localStorage-validation and comment-accuracy nits. The store design
itself — snapshot identity, the choke point in `openFileInPane`, the try/catch discipline, the
server snapshot — is sound; I found no render-loop or hydration path.

## Findings

- **[high / confidence: high]** `web/src/routes/p.$project.$term.tsx:120-128` — the basename span is
  rendered as a _child of the Editor toggle button_, so the button's accessible name is computed
  from contents and becomes `"Editor nested.txt"`. `web/e2e/run.mjs:246` ("toggling back reopens the
  same file") does `getByRole("button", { name: "Editor", exact: true }).click()` at a point where
  `nested.txt` is open — it will time out. (`run.mjs:384` still passes only by luck: the file was
  just deleted, so `pane.file` is null there.) Fix: either move the basename out of the button (a
  sibling `<span>` in the header, which is also better a11y — the toggle's name should not churn),
  or give the button a stable `aria-label="Editor"` and update the e2e locator. Do not "fix" this by
  loosening `exact: true` alone; the name-churn is the real defect.
- **[high / confidence: high]** `web/src/components/FileExplorer.tsx:231-266` — the Recent chips
  render file basenames inside the same `<aside>` as the listing, so any e2e locator that clicks a
  file by exact text now has two candidates. Concretely: `web/e2e/run.mjs:369`
  (`panel().getByText("Dockerfile", { exact: true }).click()`) runs _after_ `Dockerfile` was opened
  at `run.mjs:262`, so a `Dockerfile` chip exists → strict-mode violation, check fails. This is a
  standing trap for every future "click the file by name" check, not a one-off. Fix: scope the
  listing locators (e.g. `panel().locator("div.group").filter({ hasText: ... })` as the delete check
  already does), and give the strip a stable hook — `data-testid="recent-files"` or
  `role="list"`/`aria-label="Recently opened"` — so tests can exclude or target it deliberately.
- **[medium / confidence: high]** `web/src/lib/editorPane.ts:82` + `web/src/components/EditorPane.tsx:52-54,67-70`
  — `openFileInPane` writes `dirty: false` unconditionally, but when the pane is dirty the load
  effect parks the request in `pending` and never loads anything. `EditorPane`'s republish effect is
  keyed on `[st.dirty, setDirty]`, and `st.dirty` did not change, so the store is left claiming
  clean while the buffer is still dirty. Root cause is pre-existing, but this diff makes it
  actively misleading: the header now reads `Editor two.txt` with **no unsaved dot** while the pane
  still holds a dirty `one.txt` behind the confirm dialog — the exact "edits are parked here" signal
  the dot exists to give. Fix: stop resetting `dirty` in `openFileInPane` (let `EditorPane` own it
  on successful load), or restore it in the pending branch.
- **[medium / confidence: medium]** `web/src/components/FileExplorer.tsx:78-79` and
  `web/src/routes/p.$project.$term.tsx:120` — both new affordances read `paneState.file`, which is
  the _requested_ file, not the _loaded_ one. On the dirty→different-file path the explorer
  highlights, and the header names, a file the editor is not showing (and may never show, if the
  user cancels). Related: `noteRecent` fires on intent at `editorPane.ts:81`, so a cancelled open
  still lands at the head of the Recent strip — a chip for a file that was never displayed. The
  dialog body does name both files, so this is confusing rather than dangerous. Fix: derive the
  highlight from the loaded file (publish it, or suppress the highlight while `pending` is set), and
  move `noteRecent` behind a successful read if the choke-point design can absorb it.
- **[medium / confidence: medium]** `web/src/components/FileExplorer.tsx:232` — `flex-wrap` with up
  to five `max-w-40` chips inside `SidePanel`'s `w-80` aside (~304px of content width) wraps to two
  or three ~24px rows and permanently steals 50–75px from the listing, on every project, forever
  (there is no dismiss and no "clear recents"). Prefer a single non-wrapping row —
  `flex-nowrap overflow-x-auto` with `shrink-0` chips — or cut the display to three. Confidence is
  medium only because I did not render it; the arithmetic is straightforward.
- **[medium / confidence: medium]** `web/src/routes/p.$project.$term.tsx:62,95,121` — the header is
  `flex h-10 ... gap-2` with no `flex-wrap`, and neither the terminal-title button nor the toggle
  group carries `min-w-0`/`shrink-0`. Adding up to `max-w-28` (112px) to the Editor button on a
  375px viewport pushes the row over and the toggle group's own text has no truncation to fall back
  on. The project-id span next to it already solved this with `hidden sm:inline`; mirror that
  (`hidden sm:inline-block`) or add `shrink-0` to the toggle group and `min-w-0 truncate` to the
  title.
- **[low / confidence: high]** `web/src/lib/editorPane.ts:110-118` — the storage-read validator
  checks shape but not content or bounds. It accepts `path: ""`, absolute paths, and `../`
  traversal (a blank chip, or a request the server then has to reject), does not dedup, and does not
  bound the array — a legacy or corrupted blob with duplicate `{scopeKind,path}` pairs produces
  duplicate React keys at `FileExplorer.tsx:243`, and a 10k-entry blob is held in memory until the
  next write trims it. The `typescript-review` boundary rule wants storage reads fully validated.
  Fix: reject empty/absolute/`..`-containing paths, dedup, and `.slice(0, RECENTS_CAP)` on read.
- **[low / confidence: high]** `web/src/lib/editorPane.ts:161,176-180` — the recents prune keys on
  `path` alone with no scope, so deleting `notes.md` in the project scope also drops a home-scope
  `notes.md` chip. The pane sweep above it has the same blind spot (pre-existing), which is the
  argument for fixing it once: give `closeFileInPane` a `Scope` (or a `scopeKind`) parameter — the
  single caller at `FileExplorer.tsx:128` already has it in hand.
- **[low / confidence: high]** `web/src/lib/editorPane.ts:173-175` — the comment claims a stale chip
  "would recreate the file via the pane's Save". It cannot: clicking a chip for a missing file makes
  `api.readFile` fail, `loaded` stays null, and `FileEditorToolbar` (the only Save button) never
  renders — the pane shows "No file open". The prune is still worth having for hygiene, but this
  repo holds comments to explaining the real _why_; as written it asserts a guarantee the code does
  not provide.
- **[low / confidence: high]** `web/src/lib/editorPane.ts:176-180` — recents are pruned only on
  explorer-initiated delete/rename. Per R4/AGENTS the terminal is the primary interface, so files
  disappear via `rm` far more often than via the explorer; stale chips are the normal case, not the
  edge case. Clicking one drops the user on an empty editor plus an error toast with no self-heal.
  Cheapest fix: prune the recent entry when `EditorPane`'s read fails with a not-found.
- **[low / confidence: high]** `web/src/lib/editorPane.ts:94` vs `FileExplorer.tsx:236` —
  `RECENTS_CAP = 8` but the UI hard-codes `.slice(0, 5)`; three entries are stored and never
  shown, and the display count is a bare literal in the component while the cap lives in the lib.
  Either export one constant or comment why the store keeps headroom the UI does not use.
- **[low / confidence: medium]** `web/src/components/FileExplorer.tsx:255-259,322-326,348-354` — the
  "this one is open" state is conveyed by background colour (and, on the name, `font-medium`) with
  no programmatic equivalent: no `aria-current`, no `aria-pressed`. Add `aria-current="true"` to the
  open chip and the open row's name. Separately, `title` is not reliably announced by screen readers
  and never appears on touch — give the chips `aria-label` with the full path in addition to
  `title`, since the visible label is only a basename and two files can share one.
- **[low / confidence: medium]** `web/src/components/FileExplorer.tsx:322-326` — the open row swaps
  `hover:bg-bg-hover` out for `bg-accent-soft`, so the currently-open row is the one row in the list
  with no hover affordance. `hover:bg-bg-active` (as the row action buttons already use) keeps the
  highlight and the feedback.
- **[low / confidence: medium]** `web/src/lib/editorPane.ts:98-135` — no cross-tab coherence: the
  in-memory `recents` map is never invalidated, and `storeRecents` writes the whole array, so two
  tabs on the same project silently clobber each other's history. No `storage` listener, and no
  cleanup of `styrman.recentFiles.<uuid>` keys when a project is deleted (100+ project lifetimes per
  REQUIREMENTS → an unbounded, if tiny, key leak). Acceptable to accept; not acceptable to leave
  undocumented next to a comment that says the list is "worth keeping across reloads".
- **[low / confidence: high]** `FileExplorer.tsx:237-239,247-252` and `p.$project.$term.tsx:122-126`
  — basename extraction is now open-coded three times (a fourth variant, dirname, sits at
  `FileExplorer.tsx:146`), and `Scope` reconstruction from a kind is open-coded twice
  (`FileExplorer.tsx:176-179` and `247-252`). Two three-line helpers in `lib/` (`basename`,
  `scopeOf`) remove all of it. Storing the full `Scope` in `RecentFile` instead of `scopeKind` would
  delete the reconstruction entirely — `Scope` is exactly `{kind, project}`, so nothing is saved by
  splitting it.

## Criteria

1. **Dead code — pass.** No orphaned exports, no commented-out blocks, no unreferenced files. Every
   new export (`RecentFile`, `useRecentFiles`) has a live consumer. The only smell is the 8-vs-5
   cap gap, filed as a low nit rather than dead code.
2. **Flow tracing — fail.** The store→pane→header/explorer path is broken in two places: the pane
   publishes `dirty: false` for a file it never loaded (finding 3), and both new affordances read
   the _requested_ file rather than the _loaded_ one (finding 4). Prune coverage stops at the
   explorer, so terminal-side deletions leave stale chips (finding 10). `openFileInPane` is a
   genuine single choke point (only `FileExplorer` calls it — verified by grep), and no changed
   function calls into a stub. No floating promises introduced; `catch {}` blocks are deliberate and
   commented.
3. **Standards conformance — partial.** TypeScript typing is clean (no `any`, the `as RecentFile`
   casts sit inside a type predicate that is itself the runtime check). But the
   `typescript-review` storage-boundary rule is only half-honoured (finding 7), no covering test was
   added for genuinely testable pure logic, and — the blocking part — two existing e2e checks are
   broken (findings 1 and 2). `useSyncExternalStore` discipline is correct: snapshots are cached by
   identity in `loadRecents`, arrays are replaced rather than mutated, `getServerSnapshot` returns
   the stable shared `NO_RECENTS`, and `localStorage` access is inside try/catch so a Node prerender
   pass degrades instead of throwing. I traced for a render loop and found none; the only cost is a
   redundant `emit()` when the same file is re-opened.
4. **Directory layout — pass.** Store logic in `web/src/lib/`, component in `web/src/components/`,
   route change in `web/src/routes/`. Everything stays inside `web/`. `web/dist/client` is a tracked
   build artifact by design (`go:embed`) and was correctly rebuilt — note the new hashed assets are
   untracked, so the commit must `git add -A web/dist` or the binary will embed a stale bundle.
5. **Simplicity (DIY-except-security) — pass.** No new dependencies; `localStorage` + `JSON` from
   the platform, no abstraction layer over either. No auth, crypto, hashing, session, or JWT/OIDC
   code is touched, so the security exemption does not apply. The only complexity that does not buy
   anything is the `scopeKind`-instead-of-`Scope` split and the duplicated basename logic
   (finding 14).
6. **Decoupling — pass with a nit.** The store stays the seam between the explorer, the pane and
   the route header; the explorer still does not reach into the pane. `closeFileInPane` now carries
   two responsibilities (pane sweep + recents prune) behind one name, which is defensible since both
   answer the same event, but its `(project, path)` signature is too narrow for either job
   (finding 8). Nobody has to serialise edits: `editorPane.ts` is small and its two concerns sit in
   clearly delimited sections.

## Design-decision consistency (dispatch item 4)

The module header at `editorPane.ts:18-19` says the open file is "session context, not a durable
preference", and the new block at `:86-90` explicitly contrasts itself with it. The distinction is
defensible and documented: _which file is on screen right now_ is per-terminal session state that
must not survive a reload, while _which files this project has touched_ is a per-project navigation
history whose whole value is surviving one. Two things would make it airtight — say out loud that
the recents list is deliberately per-_project_ and therefore shared across that project's terminals
(the current text only implies it), and record that it is not cross-tab coherent and is never
garbage-collected when a project is deleted (finding 13).

---

# Round 2 — delta re-review (2026-09-01)

Scope: the same uncommitted working tree at HEAD `ad1a4be`, re-read from source after the
round-1 fixes. Files re-read in full: `web/src/lib/editorPane.ts`,
`web/src/components/FileExplorer.tsx`, `web/src/routes/p.$project.$term.tsx`,
`web/e2e/run.mjs`, plus `web/src/components/EditorPane.tsx`, `web/src/components/FileEditor.tsx`
and `web/src/components/ui.tsx` (unchanged, read to trace the flows). Claims were not taken on
trust; every "fixed" below was verified in the code. Reviewer did not author the fixes.

## Verdict: APPROVE WITH NITS

Both round-1 blockers are genuinely closed and the fix code is correct — I re-traced all three
EditorPane dirty flows and the dirty-preservation change is right, not merely plausible. Nothing
in the changed source blocks the commit. The one thing I would not sign off on for the **v1.17.1
release** is coverage: the change set ships three new user-visible behaviours (Recent chips, the
open-file highlight, the header basename badge) and adds **zero** e2e checks for any of them —
`grep -in recent web/e2e/*.mjs` returns nothing. Every e2e edit in this diff is defensive
(scoping existing locators so the new UI stops breaking them). `AGENTS.md` states "changed code
without covering tests = FAIL", and `web/` has no unit tier (no vitest, no `test` script, no
`*.test.ts` anywhere) so e2e is the only tier available. That is the verifier's row to
adjudicate, but from the review side it is the one open item worth holding the tag for; adding a
chip-click check is roughly five lines.

## Round-1 findings — verified status

| # | Round-1 finding | Status |
|---|---|---|
| 1 | Editor toggle accessible-name churn (high) | **Fixed** — `p.$project.$term.tsx:110` `aria-label={label}`; both toggles now have stable names `Terminal`/`Editor`, so `run.mjs:250` and `:388` resolve. Verified no other element in the tree takes the name `Editor`. |
| 2 | Recent chips collide with listing locators (high) | **Fixed** — `data-testid="recent-files"` (`FileExplorer.tsx:231`), `data-testid="file-list"` (`:293`), `fileList()` helper at `run.mjs:68`, applied to all 9 file-click locators (`:195,207,269,294,301,319,332,355,374`). Playwright's default `testIdAttribute` is `data-testid` and no override is configured, so `getByTestId` resolves. |
| 3 | `openFileInPane` clobbered `dirty` (medium) | **Fixed** — `editorPane.ts:86-90` now carries `dirty: get(project, term).dirty`. Flows re-traced below. |
| 4 | Highlight/badge track the *requested* file; `noteRecent` fires on intent (medium) | **Accepted residual**, unchanged in code. Recorded here. |
| 5 | Chip strip wraps and eats listing rows (medium) | **Fixed** — `FileExplorer.tsx:232` is `flex … overflow-x-auto` with `shrink-0` chips. CSS `flex-wrap` defaults to `nowrap`, so this is a genuine single row (~32px), not a lucky wrap. |
| 6 | Header overflow at 375px (medium) | **Fixed** — the badge is `hidden … sm:inline-block` (`:126`), mirroring the project-id span, so sub-`sm` viewports gain no width. |
| 7 | Storage read not validated/bounded (low) | **Mostly fixed** — see finding R2-1: array length still unbounded. |
| 8 | Recents prune is scope-blind (low) | **Not fixed**, not in the accepted list — see R2-3. |
| 9 | Wrong "would recreate the file" comment (low) | **Fixed** — `editorPane.ts:196-200` now states the real hazard (a stale chip's open just fails to load) and documents the guest-side `rm` gap as aged-out-by-cap. |
| 10 | No prune on a failed read (low) | **Accepted**, and now documented in the code comment itself (`:199-200`). Durable. |
| 11 | `RECENTS_CAP = 8` vs `.slice(0, 5)` (low) | **Fixed** — the `.slice(0, 5)` is gone; all 8 render. |
| 12 | No `aria-current`; `title` alone on chips (low) | **Fixed** — `aria-current` on the open chip (`:255`) and the open row's name (`:352`); chips carry `aria-label` with the scope-qualified full path (`:254`). |
| 13 | Open row loses its hover affordance (low) | **Not fixed**, not in the accepted list — see R2-4. |
| 14 | No cross-tab coherence, no key GC (low) | **Accepted.** Recorded in round 1 and again here; still not in the code comment — see R2-5. |
| 15 | `basename`/`scopeOf` open-coded (low) | **Half fixed** — `basename` is a shared export (`editorPane.ts:94`) used in all three sites. `Scope` reconstruction is still open-coded twice (`FileExplorer.tsx:178-179`, `:250-251`). |

## Fresh-eyes on the fix code

**The `aria-label` pin reads correctly.** `<button aria-pressed={…} aria-label="Editor">` gives
name `Editor`, role `button`, state pressed/not-pressed — exactly what round 1 asked for, and
`aria-label` legitimately wins over contents. No regression for AT: before this change set the
badge did not exist and the dirty dot is an empty `<span title>`, which contributed nothing to
the computed name, so the accessible name was already bare `Editor`. See R2-2 for the cost.

**Dirty preservation is correct across all three flows.** Traced against
`EditorPane.tsx:52-98` and `useFileEditorState`'s during-render reset (`FileEditor.tsx:122-123`,
keyed on `file.path`):
- *Clean open* — `loaded` is null, so the load effect fetches; `dirty` was already false, so
  preserving it changes nothing. Correct.
- *Dirty → discard* — `openFileInPane` writes `file: two.txt, dirty: true`; the load effect sees
  `loaded && dirtyRef.current` and parks in `pending`; the header keeps its dot while the dialog
  is up (this is the fix). `discardAndOpen` clears `loaded` → the hook re-renders against
  `EMPTY` (path `""` ≠ previous path) → `dirty` recomputes false → the publish effect at
  `EditorPane.tsx:52-54` writes `dirty: false`. No stale dot survives.
- *Dirty → keep editing* — `ConfirmDialog` fires `onCancel` on the Cancel button, the backdrop
  *and* Escape (`ui.tsx:245-257`), so `keepEditing` always runs and points the store back at
  `loaded.req`; `dirty` stays true, which is now the truth. I specifically probed the
  Escape/backdrop path for a leak (store pointing at the un-opened file with `pending` already
  cleared) — `onCancel` covers it. Not a bug.
- Bonus, unclaimed: re-opening the *same* dirty file (clicking its row or chip again) used to
  clear the dot via the unconditional reset. It no longer does — the load effect's `same` check
  returns early and `dirty` survives.

**e2e locator scoping is consistent.** All 9 file-by-name clicks go through `fileList()`. The
remaining `panel()`-scoped locators were each checked for a chip collision: `Files`/`Project`/
`Home` are tab labels no chip can produce for the fixtures in play; `panel().locator("div.group")`
(`:380`) cannot match a chip (chips are `<button>`, no `group` class); `panel().getByLabel(
"Delete Dockerfile")` cannot match `aria-label="Open recent file Dockerfile"`. Scoping to
`fileList()` also incidentally closed a *pre-existing* trap — the breadcrumb sits outside the
`file-list` div, so `getByText("subdir")` no longer risks matching the crumb. One theoretical
hole remains, filed as R2-6.

**The storage validator** rejects non-objects, bad `scopeKind`, non-strings, empty strings,
`length > 512`, leading `/`, any `..` segment, and dedups on `${scopeKind}/${path}` — the same key
used for the React `key`, so the duplicate-key hazard is closed at the source. One gap: R2-1.

**The chip strip** is a real single non-wrapping row; `Recent` label and chips are all `shrink-0`,
chips `max-w-40 truncate`. Listing height loss is one row, once. `tsc -b` and `oxlint src` are
both clean (re-run: exit 0, no output). `web/dist/client` is rebuilt and in sync — every asset
referenced by `_shell.html` exists on disk, and the new bundle contains both `recent-files` and
the `Open recent file` label. The new hashed assets are **untracked**; the commit must
`git add -A web/dist` or the Go binary will embed a stale bundle.

## New / remaining findings

- **[medium / confidence: high]** `web/e2e/run.mjs` — no check exercises any of the three new
  behaviours. There is no chip click, no assertion on the open-row highlight, and no assertion on
  the header basename badge; `grep -in recent web/e2e/*.mjs` is empty. `web/` has no unit tier
  (no `vitest`/`jest`, no `test` script in `package.json`, no `*.test.ts` under `web/src`), so
  e2e is the only tier this code can be covered at, and `AGENTS.md` makes uncovered changed code
  a verifier FAIL. Concrete fix, inside the existing "clicking a file opens it" check: after
  opening `nested.txt`, assert `panel().getByTestId("recent-files")` is visible, click
  `panel().getByLabel(/^Open recent file .*nested\.txt$/)`, and assert `.cm-content` shows its
  content — three assertions, one chip round-trip, no new fixture.
- **[low / confidence: high]** `web/src/lib/editorPane.ts:114-148` (R2-1) — the comment at
  `:123-126` promises to "bound the length", but only `path.length` is bounded; the *array* is
  not. Round 1 asked for `.slice(0, RECENTS_CAP)` on read. This matters slightly more now than it
  did in round 1, because removing the UI's `.slice(0, 5)` (correctly, per finding 11) means
  `FileExplorer.tsx:236` renders every entry the validator returns — a corrupted or
  foreign-written blob renders N chips instead of 8. Not reachable by an attacker (own-origin
  storage), so low. Fix: `.slice(0, RECENTS_CAP)` on the filtered list, or amend the comment to
  say only path length is bounded.
- **[low / confidence: medium]** `web/src/routes/p.$project.$term.tsx:110-137` (R2-2) — the
  `aria-label` pin is the right call, but it makes the button's two visual payloads —
  the filename badge and the unsaved dot — completely unavailable to a screen reader (an
  `aria-label` suppresses contents, and `title` on a decorative `<span>` is not reliably
  announced). No regression versus `main`, but the badge exists precisely to answer "which file
  is parked in the hidden editor". Fix without reintroducing name churn: a visually-hidden
  `<span class="sr-only">` sibling *outside* the button, or `aria-describedby` pointing at the
  badge. Worth one line; not worth holding the tag.
- **[low / confidence: high]** `web/src/lib/editorPane.ts:201-205` (R2-3) — the recents prune
  still keys on `path` alone, so deleting `notes.md` in the project scope also drops a
  home-scope `notes.md` chip. Round-1 finding 8, unfixed and *not* in the accepted-residual list.
  The comment at `:198-200` documents the path-prefix coarseness but is silent on the scope
  blindness. `RecentFile` already carries `scopeKind`; the single caller
  (`FileExplorer.tsx:128`) has the `Scope` in hand. Either fix it or record it.
- **[low / confidence: medium]** `web/src/components/FileExplorer.tsx:323-327,256-259` (R2-4) —
  round-1 finding 13, unfixed and unrecorded: the open row swaps `hover:bg-bg-hover` for
  `bg-accent-soft`, and the open chip likewise drops its `hover:` classes, so the one row and
  the one chip a user is most likely to click are the only ones with no hover feedback.
  `hover:bg-bg-active` on both keeps the highlight and restores the affordance.
- **[low / confidence: high]** `web/src/lib/editorPane.ts:98-103` (R2-5) — round-1 finding 14 is
  accepted, and this report is the durable record, but the module comment a maintainer will
  actually read still says only "a convenience worth keeping across reloads". Two clauses —
  "last writer wins across tabs; per-project keys are never garbage-collected when a project is
  deleted" — put the accepted trade next to the code that makes it.
- **[low / confidence: medium]** `web/e2e/run.mjs:173,177,183` and friends (R2-6) — the scope
  tabs and the panel-tab strip are still located with unscoped `panel().getByText("Project"|
  "Home"|"Files", { exact: true })`. A recent chip renders a bare basename, so a fixture file
  literally named `Project`, `Home` or `Files` would strict-mode-collide. No such fixture exists
  today and I would not add one; noting it so the next person who adds a fixture knows the rule
  is "listing clicks go through `fileList()`, tab clicks are basename-sensitive".
- **[low / confidence: medium]** `web/src/components/FileEditor.tsx:122-123` (R2-7,
  pre-existing, exposure increased) — the during-render reset is keyed on `file.path` only, not
  on scope. Opening `~/notes.md` while `project:notes.md` is loaded leaves the previous buffer on
  screen under the new file's toolbar. This diff does not cause it, but the Recent strip
  interleaves home- and project-scoped chips in one row, which makes a same-path cross-scope open
  a one-click action for the first time. Cheap fix: key the reset on
  `` `${scope.kind}:${file.path}` ``. Flagging for the record, not for this tag.

## Criteria (round 2)

1. **Dead code — pass.** Nothing orphaned by the fixes. The `.slice(0, 5)` was deleted rather
   than commented out, the `basename` helper has three live callers, `fileList()` has nine. No
   `console.log`/`debugger`/`TODO` in the added lines (grepped).
2. **Flow tracing — pass** (was fail). The store→pane→header/explorer path is now consistent:
   `dirty` survives a parked open and is re-published by the pane on every load boundary,
   including the `EMPTY` boundary. `openFileInPane` is still the single choke point. No floating
   promises added; the two `catch {}` blocks are deliberate and commented. The requested-vs-loaded
   divergence (round-1 finding 4) remains, is bounded by a modal that names both files, and is
   recorded as accepted.
3. **Standards conformance — pass with a gap.** `tsc -b` clean, `oxlint src` clean, no `any`, the
   `as RecentFile` cast lives inside its own type predicate, `useSyncExternalStore` discipline
   (cached snapshot identity, replaced arrays, stable `getServerSnapshot`, try/catch around
   `localStorage`) is correct. The storage-boundary rule is now substantively honoured. The gap
   is the absent covering test, above.
4. **Directory layout — pass.** No new files. Everything stayed in `web/src/lib`,
   `web/src/components`, `web/src/routes`, `web/e2e`. `web/dist/client` correctly rebuilt.
5. **Simplicity (DIY-except-security) — pass.** No new dependencies. No auth, crypto, hashing,
   session or JWT/OIDC code touched, so the security exemption does not engage. The remaining
   avoidable complexity is the `scopeKind`-instead-of-`Scope` split (finding 15) — cosmetic.
6. **Decoupling — pass.** The store is still the only seam; the explorer reads pane state through
   `useEditorPane` rather than reaching into the pane, and the e2e `fileList()` helper is the
   right shape of seam for the test suite. `closeFileInPane` still carries two responsibilities
   behind one name with a signature too narrow for the second (R2-3).

## Blocking summary — plain answer

- **(a) Commit — nothing blocking from this review.** The source is correct and clean; the two
  round-1 blockers are genuinely closed. Two mechanical preconditions that are not review
  findings: `git add -A web/dist` (the new hashed assets are untracked — commit without them and
  the embedded bundle is stale), and the on-disk verification artifact
  `.agents/docs/testing/2026-09-01-verify-editor-ux.md` still reads **FAIL**; per the DoD a
  round-2 verifier log must supersede it *before* the commit closes.
- **(b) v1.17.1 release — one item I would hold for:** the missing e2e coverage of the three new
  behaviours (medium finding above). It is ~5 lines inside an existing check. If the operator
  decides the defensive locator scoping plus a green 48/0/2 suite is sufficient evidence, that is
  a legitimate call to make explicitly and record in the changelog entry — but it should be made,
  not assumed. Every other open item here is a low and none of them should hold the tag.

## Accepted residuals — on the record

Recorded durably by this report, per the round-2 dispatch: (1) the explorer highlight and the
header badge reflect the *requested* file while an open is parked behind the discard-confirm —
the dialog covers the screen and `keepEditing` restores truth on every dismissal path;
(2) `noteRecent` fires on intent, so a cancelled open still lands at the head of the strip;
(3) no cross-tab coherence — last writer wins; (4) no garbage collection of
`styrman.recentFiles.<uuid>` keys when a project is deleted; (5) guest-side `rm` leaves stale
chips that age out via the cap (this one is also documented in the code at
`editorPane.ts:199-200`).
