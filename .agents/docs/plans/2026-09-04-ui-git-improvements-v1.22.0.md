---
owner: main
date: 2026-09-04
---
# Plan — UI + git improvements (v1.22.0)   (2026-09-04)

## Why

Six operator-reported friction points, raised in one batch on 2026-09-04 and
approved the same day (with two cuts, recorded under Scope → Out):

1. `GOPATH` is `$HOME/go`; the operator wants `$HOME/.go` so the home's
   top level stays clean.
2. Terminal and editor font size/family are hardcoded — no way to adjust
   them for a different screen or eyesight.
3. The file explorer's path row is `text-[11px]` with no back/forward at
   all; navigating deep trees is fiddly.
4. The Recent-files strip is a single horizontally scrolling row in a 320 px
   panel, so ~2 of 8 chips are visible and the rest need a scroll gesture.
   The operator wants a wrapping grid, newest first, collapsible and
   resizable.
5. The git diff renders in a `max-h-64` box at the bottom of the side panel —
   too small to read. Committing routine work needs a hand-typed message.
6. The git panel has status/stage/unstage/discard/commit/push/diff and
   nothing else; pull, branches, log and stash are missing.

R13 makes git the substrate and names the side panel's client explicitly, so
6 extends a requirement rather than inventing a surface. Nothing here touches
R1–R12.

## Scope

**In**

- `image/skel` GOPATH change + `README.md` prose, new project homes only.
- Settings → Appearance: terminal font size + family, editor font size +
  family, persisted per browser in `localStorage`.
- File explorer: back/forward history, an explicit Up control, a readable
  crumb bar.
- Recent files: wrapping chip grid, newest first, collapse toggle, drag
  resize, cap raised 8 → 16.
- Git: diffs open in the main pane as a third per-terminal view; a
  fast-commit button (`git add -A` + commit with a generated UUID);
  `POST /git/commit` gains `"all"`.
- Git: `fetch`, `pull --ff-only`, branch list/checkout/create, `log`,
  `stash push|pop|list` — new endpoints, new panel controls.

**Out** (explicitly cut by the operator, 2026-09-04)

- **Global UI density rework.** The 80 arbitrary `text-[9|10|11px]` classes
  across 18 files stay as they are; there is no root font-size scale and no
  `uiDensity` preference. Phases 3 and 4 fix the crumb bar's and the recents
  strip's own sizes locally instead.
- **`git commit --amend`.** Dropped as the one added verb that can lose
  committed work. Stash push/pop/list stays.

**Out** (not asked for)

- Accent-colour picker or additional palettes beyond light/dark/system.
- Server-side persistence of appearance preferences (theme is already
  per-browser `localStorage`; the new keys follow it).
- Merge, rebase, cherry-pick, remote management, tag operations.

**Follow-ups noted, not built**

- `REQUIREMENTS.md:31` uses `~/go` as an example of per-project tool state.
  It is now stale prose, but `REQUIREMENTS.md` is immutable without explicit
  operator approval (AGENTS.md, Hard rules) — **this plan does not touch
  it**. Raise the one-word edit with the operator separately.
- `web/src/index.css:3` cites "R9" for self-hosted fonts; R9 has since been
  rewritten to mean the deployment target (REQUIREMENTS.md:9). The practice
  (bundle fonts, never a CDN) is right; the citation is stale. Note for
  `documentation`, do not renumber requirements here.
- The open TODO item "`web/src/lib/api.ts` interpolates `project`, terminal
  `id`, and link `token` without `encodeURIComponent`" is **partly** closed
  here: the whole git block (`api.ts:588-628`) gets encoded, new and existing
  methods alike, because phase 6 rewrites it anyway. The terminal `id` and
  link `token` templates elsewhere in the file are untouched and stay on the
  TODO — narrow the item, do not tick it.

## Surfaces touched

| Area | Files |
|---|---|
| Guest shell | `image/skel/bash_exports`, `image/skel/bashrc` |
| Docs | `README.md`, `.agents/docs/specs/styrman/api.md`, `.agents/docs/specs/styrman/guide.md`, `.agents/docs/specs/styrman/runtime-image.md` |
| Preferences | `web/src/lib/hooks.ts` (new appearance store next to the theme store at `:98-175`) |
| Terminal | `web/src/lib/terminal.ts:99-112` (`createTerminal` opts), `web/src/components/TerminalPane.tsx` |
| Editor | `web/src/components/FileEditor.tsx`, `web/src/components/CodeEditor.tsx` (+ `web/src/routes/settings.tsx:704-710`, the second call site — left untouched by optional props) |
| Settings | `web/src/routes/settings.tsx:779-809` (`Appearance` section) |
| Explorer | `web/src/components/FileExplorer.tsx` (crumbs `:280-298`, `../` row `:320-327`, recents `:429-472`) |
| Pane | `web/src/lib/editorPane.ts` (`PaneState`, `set`), `web/src/routes/p.$project.$term.tsx` (header toggle), new `web/src/components/DiffPane.tsx` |
| Git panel | `web/src/components/SidePanelGit.tsx`, `web/src/components/SidePanel.tsx:100` (thread `term` into `GitTab`) |
| Git API client | `web/src/lib/api.ts:588-630` |
| Git server | `internal/httpapi/git.go`, `internal/httpapi/server.go:266-272` (route table) |
| Fonts | `web/package.json` (`@fontsource-variable/*`), `web/src/index.css` |
| Tests | `web/src/lib/*.test.ts` (new files, suffixed — BUG-005), `internal/httpapi/git_*_test.go` (new file), `web/e2e/run.mjs` |

## Interfaces

### Appearance preference store (`web/src/lib/hooks.ts`)

Mirrors the existing theme store exactly: module-level value + listener set +
`useSyncExternalStore`, written through to `localStorage` under **one key
holding one JSON object**. Per-FIELD fallback is what keeps a corrupt value
from taking the others down — not per-key storage. (An earlier draft of this
plan said "one key per preference" here and named a single JSON key two
paragraphs later; the single object is intended.)

```ts
export type FontFamilyPref = "google-sans-code" | "jetbrains-mono" | "system";
export type Appearance = {
  termFontSize: number;             // clamped 8..32, default 13
  termFontFamily: FontFamilyPref;   // default "google-sans-code"
  editorFontSize: number;           // clamped 8..32, default 12
  editorFontFamily: FontFamilyPref; // default "system"
};
export function useAppearance(): Appearance;
export function setAppearance(patch: Partial<Appearance>): void;
```

**The four defaults are exactly today's hardcoded values, and deliberately
differ between the two surfaces**: the terminal is 13 px on Google Sans Code
(`terminal.ts:104-106`), the editor is 12 px on the bare system stack
(`CodeEditor.tsx:246-252` — `ui-monospace, SFMono-Regular, Menlo, monospace`,
*not* a brand face). Defaulting both to 13/`google-sans-code` would silently
restyle and resize every existing user's editor on upgrade. Nobody's rendering
changes until they touch a control.

Keys: `styrman.appearance.v1` (one JSON object). A parse failure, a missing
key, an out-of-range number, or an unknown family falls back to the default
for that field alone — never throws, never clears the others. `localStorage`
access is wrapped in try/catch (private windows throw on access, not just on
write).

`FontFamilyPref` resolves through one exported map so the terminal (which
needs a raw CSS font stack string for xterm) and the editor (a CSS class)
cannot drift:

```ts
export const FONT_STACKS: Record<FontFamilyPref, string> = {
  // VERBATIM from terminal.ts:104-106 — all six parts. Do not "tidy" it.
  "google-sans-code":
    "'Google Sans Code Variable', 'JetBrains Mono', ui-monospace, SFMono-Regular, Menlo, monospace",
  "jetbrains-mono":
    "'JetBrains Mono Variable', ui-monospace, SFMono-Regular, Menlo, monospace",
  // VERBATIM from CodeEditor.tsx:251 — the editor's current stack.
  system: "ui-monospace, SFMono-Regular, Menlo, monospace",
};
```

The two literals above are copied character-for-character from the code they
replace, because "nothing changes until you touch a control" is only true if
they are. Note `'JetBrains Mono'` (non-variable) already sits in the terminal
stack today and was never bundled (`index.css:4-5`) — it resolves only for a
user who has it installed locally. It stays anyway: removing it would change
rendering for exactly those users, which is the thing this guarantee forbids.
The newly bundled face is a *different* family name, `'JetBrains Mono
Variable'`, so the two never collide.

Exactly one new npm dependency: `@fontsource-variable/jetbrains-mono`
(OFL, self-hosted, latin subset — same treatment as the two existing faces
in `index.css:4-5`). "system" costs nothing. If the `web/dist` size delta
exceeds ~150 KB the family picker ships with `google-sans-code` + `system`
only and the dependency is dropped; record the measured delta in the
verification log either way.

### Terminal (`web/src/lib/terminal.ts`)

`createTerminal` gains `fontSize` and `fontFamily` in its opts (both
optional, defaulting to today's hardcoded values so no call site breaks).
`TerminalPane` subscribes to the store and applies changes to a LIVE terminal
without reconnecting:

```ts
term.options.fontSize = size;
term.options.fontFamily = stack;
fitAddon.fit();          // metrics changed → re-measure
// then send the resize JSON frame, as the existing resize path does
```

The resize frame matters: the WS protocol treats JSON text as a resize
(AGENTS.md), and a font change alters rows/cols, so skipping it leaves tmux
believing the old geometry.

### Editor font (`web/src/components/CodeEditor.tsx`)

**Decision: the editor font live-updates an already-open buffer**, matching
the terminal. A setting that visibly does nothing until you reopen a file
reads as broken, and the file already has the pattern to do it properly.

`CodeEditor` gains `fontSize?: number` and `fontFamily?: string` (a resolved
stack from `FONT_STACKS`). **Optional, unlike the required `dark`/`wrap`
props** (`CodeEditor.tsx:142-155`), defaulting to today's hardcoded 12 px and
system stack.

That is deliberate, because `FileEditor` is not the only call site:
`settings.tsx:704-710` mounts a second, read-only `<CodeEditor>` as the
credential/secret viewer. Required props would stop that file compiling, and
passing it the store values would be wrong anyway — it displays secrets, not
code, and has no reason to track the user's *editor* font preference. Optional
props leave that call site untouched and byte-identical. Today the font is baked into a bare `EditorView.theme({...})` inside
the mount-once extension list (`CodeEditor.tsx:244-253`) — *not* a
`Compartment`, unlike `themeComp` and `wrapComp` (`:162-166`, `:213-214`),
which exist precisely so dark mode and wrap can reconfigure in place without
losing cursor, scroll position and undo history (`:163-165`).

So: add a third `fontComp = useRef(new Compartment())`, move the `&` /
`.cm-scroller` font rules out of the static theme into
`fontComp.current.of(EditorView.theme({...}))`, and reconfigure it from an
effect keyed on the two props — the same shape as the existing wrap/theme
reconfigure effects. Height and `backgroundColor` stay in the static theme;
only the two font rules move, so the compartment carries the smallest
possible slice.

`FileEditor` reads the store and passes the props down. `CodeEditor`
remounts on filename change anyway (`FileEditor.tsx:124`), but that must not
be the mechanism — it does not fire when Settings changes while a file is
open, which is exactly the case being fixed.

### Pane view (`web/src/lib/editorPane.ts`)

```ts
export type DiffTarget = {
  path?: string;        // absent = whole working tree
  staged: boolean;
  untracked: boolean;
  commit?: string;      // set by the log view (phase 6)
  subject?: string;     // commit subject, supplied WITH `commit` by its opener
};
export type PaneState = {
  view: "terminal" | "editor" | "diff";   // was "terminal" | "editor"
  file: PaneFile | null;
  diff: DiffTarget | null;                // NEW
  dirty: boolean;
  draft: { path: string; text: string } | null;
};
export function openDiffInPane(project: string, term: string, target: DiffTarget): void;
```

**Gotcha, must not be missed:** `set()` (`editorPane.ts:69-82`) bails out
early by comparing `view`, `file` and `dirty` by identity. `diff` must join
that comparison or opening a second diff while already on the diff view is a
silent no-op. The existing omission of `draft` from that check is deliberate
and stays.

`openDiffInPane` does **not** call `noteRecent` — a diff is not an opened
file and must not displace the recents list.

Switching to the diff view while the editor holds unsaved edits does not
prompt: the editor stays mounted and hidden exactly as it does for the
terminal view, and `dirty` keeps showing on the header badge. Only *opening a
different file* prompts, which is unchanged.

### Diff pane (`web/src/components/DiffPane.tsx`)

Props `{ project, term, onShowTerminal }`. Reads the target from the pane
store, fetches through `api.gitDiff`, renders the same colourised hunk view
`SidePanelGit.tsx`'s `DiffView` uses — **extracted** into a shared component
rather than copied, so the side panel and the main pane cannot diverge.
Header shows the path, or "Working tree" when `path` is absent, or
`<shortHash> <subject>` when `commit` is set — **the subject travels on the
target, supplied by whoever opened it**. The log view already holds it from
`GET /git/log`, so there is no second lookup and no single-commit endpoint;
`DiffPane` renders only what its own inputs carry. A `commit` arriving
without a `subject` (nothing constructs one today) falls back to the short
hash alone rather than fetching.

Also in the header: a staged/working badge, a refresh control, and a close
that returns to the terminal. An empty diff renders "No changes", not a
blank pane.

**Mount lifecycle — copy `EditorPane`'s, do not invent one.**
`p.$project.$term.tsx:39-44` lazy-mounts the editor behind
`editorUsed.current` (latched when `pane.view === "editor" || pane.file`) and
then keeps it mounted, hidden with CSS rather than unmounted. That shape is
not incidental: the comments at `:30-44` and `:100-103` record two prior
review findings behind it — unmounting tears down live state, and the ref
latches across terminal switches because the route does not remount. The diff
view gets the exact analogue: a `diffUsed` ref latched on
`pane.view === "diff" || pane.diff`, a hidden-`<div>` toggle, no unmount. An
implementer must not re-derive this; it is the third consumer of a pattern
that took two rounds to get right.

`DiffPane` is cheaper to unmount than the editor (no unsaved buffer, no live
socket), so this is consistency and one refetch, not correctness. It is still
the rule here, because a fourth pane view arriving later should find one
pattern in this file, not two.

**The "Diff" segment button always renders**, like the other two — it is not
conditional on a target existing. With no diff ever opened it shows the same
kind of empty state the editor already does (`EditorPane.tsx:158-163`, "No
file open. Pick one from the Files panel — it opens here"): *"No diff open.
Pick a file in the Git panel — it opens here."* Rejected the alternative (show
the button only once `pane.diff` is set) because it makes the header controls shift
under the pointer when a diff opens, and because a feature you cannot see is a
feature nobody finds. Existing e2e locators for the two-button control keep
working; the new one needs its own `aria-label="Diff"`, matching the pinned
labels at `:105-160` (that pinning exists because the editor button's filename
badge is a child and would otherwise join the accessible name).

### Git API — new and changed endpoints (`internal/httpapi/git.go`)

All follow the file's existing shape: `s.gitContext(r)` for
user/cid/guest/workspace, `s.gitExec` with direct argv (never a shell), a
`context.WithTimeout`, non-zero exit → `writeErr` with `firstLine(out)`.

| Method + path | Body / query | Success | Error |
|---|---|---|---|
| `POST …/git/commit` *(changed)* | `{"message": string, "all": bool}` | 200 `{output}` | 400 empty message; 409 git's own message |
| `POST …/git/fetch` | — | 200 `{output}` | 502 on transport failure |
| `POST …/git/pull` | — | 200 `{output}` | 409 no upstream / not fast-forward; 502 transport |
| `GET …/git/branches` | — | 200 `{repo, current, local[], remote[]}` | 502 git failure |
| `POST …/git/checkout` | `{"branch": string, "create": bool}` | 200 `{output}` | 400 invalid ref; 409 dirty tree blocks the switch |
| `GET …/git/log` | `?limit=` (1..200, default 50) | 200 `{repo, commits:[{hash, shortHash, subject, author, date}]}` | 502 git failure |
| `POST …/git/stash` | `{"op": "push"\|"pop"}` | 200 `{output}` | 400 unknown op; 409 nothing to stash / empty stack |
| `POST …/git/stash` | `{"op": "list"}` | 200 `{entries:[{ref, subject}]}` | 400 unknown op |
| `GET …/git/diff` *(extended)* | existing + `?commit=<hash>` | 200 `{state, diff}` | 400 malformed hash; 404 unknown revision |

**Read endpoints report "no repo" and "no commits" as 200, not 4xx.** This
follows the convention `gitStatus` already sets in this file
(`git.go:107-114`): a missing repository is a soft state reported as `200`
with `Repo: false`, and the panel renders it calmly ("Not a git repository
yet — it initializes on first boot", `SidePanelGit.tsx:61-67`) rather than as
an error. `branches` and `log` therefore carry the same `repo` flag, and a
repo with zero commits returns `{repo: true, commits: []}` — an empty list is
the honest representation of "no commits", and the frontend renders an empty
state it has to have anyway. (`gitPush`'s existing 409 for the same condition
is right *there* precisely because it is a mutation: pushing nothing is a
failed action, listing nothing is not.)

**A stopped VM is also a 200 for these two**, matching `gitStatus` and
`gitDiff`, which special-case `errVMNotRunning` into a 200 carrying
`state: "stopped"` (`git.go:96-101`, `:334-338`) rather than routing it
through `gitCtxErr`'s 409 like the mutations do (`git.go:379-384`). So
`branches` and `log` return `{state:"stopped", repo:false, ...}`. The panel
already gates every tab on the status call's own `state`, so this keeps one
story: reads describe the world, mutations refuse.

**Mutating endpoints keep 4xx**, as `gitPush`/`gitCommit` do: `checkout`,
`stash`, `pull` and `fetch` return 409 with git's own first line. They are
only reachable from a panel that has already rendered from a `repo: true`
status, so the no-repo case is not a normal path for them.

`commit` with `"all": true` runs `git add -A` and then `commit -m` inside one
handler under one timeout. `-A`, not `commit -a`, because `-a` skips
untracked files and "commit everything" must include them. The existing
author-injection (`-c user.name=… -c user.email=…`, `git.go:279-284`) applies
unchanged. When `add -A` succeeds and `commit` then fails, the response is
the commit's 409 and the tree is left staged — stated in the API doc, because
it is observable.

**Commit-scoped diff (`?commit=`).** The invocation is
`git show --format= --patch --no-color <hash> --`. Chosen over
`git diff <hash>~1..<hash>` because `~1` fails outright on a **root commit**,
while `show` renders it correctly as a diff against the empty tree; the empty
`--format=` suppresses the commit header, which `DiffPane` already renders
from the target's `subject`. A merge commit shows an empty patch (git's
default without `-m`/`--cc`) — acceptable, and the pane's "No changes" state
covers it rather than a blank box.

**This new branch DOES check `gitExec`'s exit code; the existing branches
deliberately do not, and must not start.** `gitDiff` today discards the code
in both branches (`git.go:353-358`, `_, out, _ =`). That is correct for the
untracked branch: `git diff --no-index` exits **1 when the files differ**,
which is the normal success case there — adding a code check would turn every
untracked-file diff into an error. `git show` has no such convention (0 on
success, non-zero on a bad revision), so the commit path checks it and maps a
non-zero exit to **404 "unknown revision"**. Without that check, a
well-formed-but-nonexistent hash would return `200` with an empty diff,
indistinguishable from "this commit changed nothing". The reasoning goes in a
code comment, because the asymmetry looks like an oversight otherwise.

**Stash list is structured, not raw text.** `{op:"list"}` returns typed
entries like `log` and `branches` do, parsed server-side from
`git stash list --format=%gd%x00%gs` (NUL-separated, same technique as `log`).
The frontend must not parse git porcelain — that is the server's job here, and
`push`/`pop` keep the plain `{output}` shape because they have no list to
render.

**`stash push` runs `git stash push -u`** — with untracked files. Same
reasoning as `add -A` over `commit -a` above: a "stash my work" button that
silently leaves new files in the tree loses exactly the work its user thought
they had put away. `-u` does not include ignored files (that would need `-a`),
which is right — build output is not WIP.

**`stash pop` conflicts are reported, never auto-resolved.** A conflicting pop
exits non-zero, writes conflict markers into the tree, AND keeps the stash
entry (git's own behaviour, and the safe half of it). The handler returns 409
with git's first line; it does NOT attempt `git checkout --theirs`, a reset,
or a re-stash. Unlike `checkout`, whose 409 fires *before* any mutation, this
409 arrives with the working tree already modified — the API doc and the
toast must both say the stash entry was kept, or the user will assume their
stash is gone.

**Input validation** — `branch` (checkout) and `commit`/`hash` (diff, log)
are the first *user-supplied* strings this file turns into git argv:

- reject empty, longer than 255 bytes, or a leading `-` (an option, not a ref);
- validate with `git check-ref-format --branch <name>` for branches and
  `^[0-9a-fA-F]{7,40}$` for commit hashes, before any other git call;
- always pass `--` before user values where git accepts it;
- `stash` `op` is a closed enum matched by `switch`, never interpolated.

`gitExec` already builds `[]string` argv and never a shell string, so
quoting is not the exposure; option injection and ref traversal are.

### Git API client (`web/src/lib/api.ts`)

New methods `gitFetch`, `gitPull`, `gitBranches`, `gitCheckout`, `gitLog`,
`gitStash`; `gitCommit(project, message, opts?: {all?: boolean})`; `gitDiff`
opts gain `commit?: string`.

**Every method in the git block gets `encodeURIComponent(project)` — the
seven existing ones too, not just the new ones.** Ending the release with
`gitLog` encoding and `gitStatus` not, on the same object, is worse than
either state: there would be no rule a reader could infer beyond "was this
line retyped in 2026-09". It is seven one-line edits inside the block this
phase already rewrites (`api.ts:588-628`). The practical exposure is
unchanged (`project` is a server-issued UUID), so this is consistency work
riding along, not a security fix — and it does not touch the terminal `id`
and link `token` templates elsewhere in the file, which stay on the TODO.

### File explorer navigation (`web/src/components/FileExplorer.tsx`)

History is component state, not a route or a store: a `{stack: string[],
index: number}` reset whenever `scope` changes (project ⇄ home are separate
trees; carrying one's history into the other would offer "back" to a path
that does not exist there). `setPath` pushes and truncates the forward tail;
back/forward move the index without pushing. Buttons disable at the ends and
carry `aria-label`s "Back"/"Forward"/"Up". `Alt+←`/`Alt+→` bound on the
panel, not on `window`, so they cannot fight the terminal's key handling.

The crumb bar goes from `text-[11px]` to `text-xs` with per-crumb hover
backgrounds and real hit targets; a long path elides in the middle
(`~ › … › internal › httpapi`) instead of wrapping to a second line. The
`../` row inside the listing stays — muscle memory, and `web/e2e/run.mjs`
drives it.

### Recent files (`web/src/components/FileExplorer.tsx`, `editorPane.ts`)

`RECENTS_CAP` 8 → 16. Container becomes `flex flex-wrap` with a header row
`▾ RECENT (n)` and a top-border drag handle. Two new `localStorage` keys,
**global, not per project** (a UI preference, unlike the per-project recents
list itself):

- `styrman.recents.collapsed` — `"1"` / absent;
- `styrman.recents.height` — px, clamped 48..320, snapped back into range on
  read.

The strip stays **below** the listing. `FileExplorer.tsx:429-433` records why
(mounted above, its first appearance shifted every row under a stationary
pointer and stole a double-click's second press); the grid is taller and more
variable than the row it replaces, so that hazard gets worse, not better.
Its height is fixed by the stored value, not by content, so gaining a chip
never reflows the listing.

## Failure modes

| Failure | What the user sees |
|---|---|
| Corrupt/absent `styrman.appearance.v1` | Defaults render; no toast, no throw. A private window that throws on `localStorage` access behaves the same. |
| Font family fails to load | The stack's `ui-monospace` fallback renders; layout unaffected (all candidates are monospace). |
| Font size applied, resize frame lost | Terminal re-measures locally; tmux geometry corrects on the next real resize. Not silent corruption. |
| Diff pane fetch fails | Pane shows `error: <git's message>`, refresh control retries. The pane store keeps its target so the retry is one click. |
| Diff target is a deleted file | `gitDiff` returns an empty diff; pane shows "No changes" rather than an empty box. |
| Fast commit with nothing to commit | 409 "nothing to commit"; toast shows git's own line; nothing staged is left behind because `add -A` found nothing. |
| Fast commit: `add -A` ok, `commit` fails | 409 with git's message; **the tree stays staged** — documented in `api.md`, surfaced in the toast. |
| `pull` not fast-forward | 409 "not possible to fast-forward"; no merge started, no conflicted tree in a headless VM. |
| `checkout` with a dirty tree | 409 carrying git's "local changes would be overwritten" line and the file list. |
| `checkout` with an invalid ref | 400 "invalid branch name" from `check-ref-format`, before any git mutation runs. |
| `stash pop` with an empty stack | 409 "No stash entries found." |
| `log` before the first commit | 200 `{repo:true, commits:[]}`; the Log section shows "No commits yet". Not an error — see the read/write status split in Interfaces. |
| `branches`/`log` before the repo exists | 200 with `repo:false`; the panel's existing "Not a git repository yet" branch (`SidePanelGit.tsx:61-67`) already covers the tab. |
| Settings changes the editor font while a file is open | The open buffer re-renders in place via the new `fontComp` compartment — cursor, scroll and undo history survive (that is what the compartment is for). |
| A `commit` diff target arrives without a subject | Header falls back to the short hash; no second fetch, no blank header. |
| VM stopped | Every new endpoint inherits `gitContext`'s `errVMNotRunning` → the panel's existing "Start the project to use git." |
| Recents height beyond the panel | Clamped 48..320 on read and on drag; the listing keeps `min-h-0 flex-1` so it can never collapse to zero. |

## Verification

**Tier 1 — unit.** `go test ./...` (+ `-race` on `internal/httpapi`),
`go vet ./...`, and in `web/`: `npm run check && npm run lint && npm test`.
New tests:

- `web/src/lib/appearance.test.ts` *(new file, no collision)* — defaults,
  clamping, unknown family, corrupt JSON, a throwing `localStorage`.
- `web/src/lib/editorPane.diff.test.ts` *(new file — `editorPane.test.ts`
  already exists; BUG-005)* — `openDiffInPane` sets the view, the `set()`
  no-op guard includes `diff`, opening a diff does not touch recents,
  `RECENTS_CAP` 16 sanitising.
- `internal/httpapi/git_verbs_test.go` *(new file)* — ref validation table
  (leading `-`, empty, 256 bytes, traversal, valid), the `stash` op enum,
  `log` limit clamping, and `commit {all:true}` argv shape including the
  untracked case.

**Tier 2 — integration.** `bash .agents/docs/testing/styrman/integration.sh`
against real Kata. Adds checks: a fresh project reports
`GOPATH=/home/<user>/.go`; `POST /git/commit {all:true}` commits an untracked
file; `GET /git/branches` lists the default branch; `GET /git/log` returns
that commit. Counts live in
`.agents/docs/testing/styrman/README.md` and get updated there, not here.

**Tier 3 — e2e.** `bash web/e2e/run.sh` (62 checks today). Adds: changing
terminal font size re-renders; back/forward/up walk a tree and disable at the
ends; the recents grid wraps, collapses, and its collapsed state survives a
reload; a git file row opens the diff in the MAIN pane (asserting the pane's
width, not just its presence); fast-commit round-trips. **Updates** the
existing `recents strip rendered N chips, cap is 8` assertion
(`web/e2e/run.mjs:448`) to 16, and re-checks the header segment control's
locators, which gain a third button.

**Tier 4 — build.** `docker build image/` (AMD64) for the skel change, and a
fresh project created from it.

**E2E criteria (golden path).** In a browser against a real project: open
Settings, set terminal font to JetBrains Mono at 16 — the open terminal
re-renders without dropping its tmux session. (Note: reaching Settings
unmounts the terminal page, so "without dropping the session" means the tmux
session server-side survives the route change and reattaches — not that the
xterm instance was never torn down. See the Phase 2 note on why there is no
in-tab live-update path.) Open the Files panel, walk
three directories deep, use Back twice and Up once, confirm the crumb bar is
legible and the controls disable at the root. Open four files; the Recent
grid wraps with the newest top-left; collapse it, reload, it is still
collapsed; drag it taller and the listing shrinks. Open the Git panel, click
a changed file — the diff fills the main pane. Press Fast commit, confirm,
and see a UUID-titled commit in the new Log view. Create a branch, switch to
it, stash and pop.

**Edge case.** With unsaved editor edits parked, switch to the diff view and
back: no prompt, the dirty dot stays, the buffer survives. Then attempt
`checkout` with the tree dirty and confirm the 409 surfaces git's own message
rather than a generic failure.

## Phases

Dependency order: guest shell → preference store → UI consumers → pane
plumbing → API → panel UI → e2e. Phase 2 no longer blocks 3 and 4 (the
density rework that coupled them is cut), but the order is kept so the
smallest, most isolated changes land first.

### Phase 1 — GOPATH (`$HOME/.go`)
- [x] `image/skel/bash_exports:7` → `export GOPATH="$HOME/.go"` — verify: `grep`, unit N/A
- [x] `image/skel/bashrc:24` → `$HOME/.go/bin` on PATH — verify: `grep`
- [x] `README.md:34` prose `~/go` → `~/.go`; leave `REQUIREMENTS.md` untouched — verify: reread, both files
- [x] Rebuild the runtime image AMD64 — verify: tier 4, `docker build`, AI
- [x] Fresh project reports the new GOPATH — verify: tier 2 integration check, AI
- [x] Note in `runtime-image.md` that existing homes keep `$HOME/go` — verify: reread, user

### Phase 2 — Appearance settings (terminal + editor fonts)
- [x] Appearance store in `lib/hooks.ts` (+ `FONT_STACKS`) — verify: tier 1, `npm test`, AI — 16 tests in `appearance.test.ts`
- [x] `@fontsource-variable/jetbrains-mono` + `index.css` import; **measured delta 89.8 KB** (2 357 778 → 2 449 700 B; 5 woff2 subsets, 84 216 B), under the 150 KB cut-off, so the dependency stays — verify: tier 1 build, AI
- [x] `createTerminal` opts + live apply in `TerminalPane` (reuses the existing `onResizeRef` refit-and-ship path rather than duplicating it) — verify: tier 3, AI
- [x] `fontComp` Compartment in `CodeEditor` + OPTIONAL `fontSize`/`fontFamily` props; only the two font rules moved out of the static theme — verify: tier 1 + tier 3, AI
- [x] `FileEditor` reads the store and passes the props down — verify: tier 3, AI
- [x] ~~Editor font change applies to an ALREADY-OPEN buffer without losing cursor/scroll/undo~~ — **NOT REACHABLE, and the plan was wrong to promise it.** `/settings` is a separate route, so opening it unmounts the terminal page and its editor (stated outright at `p.$project.$term.tsx:30-33`). There is no in-tab path where a font change reaches an already-mounted editor or terminal, so there is no such behaviour to test. What ships instead: the preference persists and applies on the next mount (e2e covers that). `CodeEditor`'s `fontComp` compartment and `TerminalPane`'s live-apply effect are kept — they are correct, they are the established pattern for this file, and they are what makes the store's contract hold if a control ever moves somewhere a surface is mounted — but they are defensive, not user-visible behaviour. Recorded rather than quietly dropped, because the round-2 review reasoned about this mechanism at length on the assumption it was load-bearing.
- [x] Settings → Appearance rows below the existing Theme row (size slider + family radiogroup per surface; each family button previews its own face) — verify: tier 3, both
- [x] `web/dist` rebuilt and committed (it is `go:embed`-ed) — verify: `deploy/verify-commit.sh` freshness step, AI

### Phase 3 — Explorer path navigation
- [x] History stack + Back/Forward/Up controls, reset on scope change — extracted as a testable reducer, `lib/explorerHistory.ts` (13 unit tests) — verify: tier 1 + tier 3, AI
- [x] `Alt+←`/`Alt+→` bound on the panel (NOT window — xterm sends Alt+Arrow to the guest for readline word motion) — verify: tier 3, AI
- [x] Crumb bar at `text-xs` with hit targets and middle elision; the root crumb gained an explicit `aria-label` because its visible text is the user-renameable project name — verify: tier 3 + eyes, both
- [x] `web/dist` rebuilt — verify: freshness step, AI

### Phase 4 — Recent files grid
- [x] `RECENTS_CAP` 8 → 16. The sanitiser itself stays e2e-covered, not unit-covered: `sanitizeRecents` is not exported and `editorPane.test.ts`'s header records why that module's internals are deliberately not reached from unit tests. The e2e seeds 20 junk entries and asserts exactly 16 chips render — verify: tier 3, AI
- [x] Wrapping grid, newest first, header with count — verify: tier 3, AI
- [x] Collapse toggle + drag resize, persisted globally and clamped in `lib/recentsPanel.ts` (14 unit tests); pointer events with capture, and the height persisted once at drag END rather than at pointermove rate — verify: tier 1 + tier 3, AI
- [x] Update the e2e cap assertion to 16 — verify: tier 3, AI
- [x] `web/dist` rebuilt — verify: freshness step, AI

### Phase 5 — Git diff in the main pane + fast commit
- [ ] `PaneState.diff` + `openDiffInPane`; add `diff` to `set()`'s no-op guard — verify: tier 1, AI
- [ ] Extract `DiffView` out of `SidePanelGit.tsx` into a shared component — verify: tier 1, AI
- [ ] `DiffPane.tsx` + third header segment (always rendered, `aria-label="Diff"`, empty state) — verify: tier 3, AI
- [ ] `diffUsed` lazy-mount ref + hidden-div toggle mirroring `editorUsed` (`p.$project.$term.tsx:39-44`) — verify: tier 3, AI
- [ ] `SidePanel.tsx:100` passes `term={term}` to `GitTab` (it already threads it to `PortsTab` at `:97`) — verify: tier 1 `npm run check`, AI
- [ ] Git-tab file rows route to the main pane; drop the `max-h-64` box; add a working-tree diff button — verify: tier 3, AI
- [ ] Server: `commit {all:true}` = `add -A` + commit, one handler — verify: tier 1 + tier 2, AI
- [ ] Fast-commit button, `crypto.randomUUID()` message, confirm showing the file count — verify: tier 3, both
- [ ] `api.md` records the staged-tree-on-partial-failure behaviour — verify: reread, AI
- [ ] `web/dist` rebuilt — verify: freshness step, AI

### Phase 6 — Git verbs (fetch, pull, branches, log, stash)
- [ ] Ref/hash/op validation helpers + their table test FIRST — verify: tier 1 `-race`, AI
- [ ] `fetch` + `pull --ff-only` handlers and routes — verify: tier 1 + tier 2, AI
- [ ] `branches` + `checkout` handlers and routes — verify: tier 1 + tier 2, AI
- [ ] `log` handler (limit clamped 1..200) — verify: tier 1 + tier 2, AI
- [ ] `diff?commit=` via `git show --format= --patch`, exit code checked → 404; comment why the sibling branches do NOT check it (`--no-index` exits 1 on difference) — verify: tier 1 + tier 2, AI
- [ ] Root-commit diff returns a real patch, not an error — verify: tier 2 integration, AI
- [ ] `stash` handler: closed op enum; `list` returns structured entries parsed from `--format=%gd%x00%gs` — verify: tier 1, AI
- [ ] `api.ts` client methods added, and `encodeURIComponent(project)` applied across the WHOLE git block incl. the seven existing methods — verify: tier 1, AI
- [ ] Narrow (do not tick) the `api.ts` encoding item in `TODO.md` to the terminal `id` / link `token` templates — verify: reread, AI
- [ ] Panel UI: Pull button, branch dropdown, Log section, stash controls — verify: tier 3, both
- [ ] Log entry click → diff in the main pane, passing `commit` AND `subject` on the target — verify: tier 3, AI
- [ ] `security` agent on the whole phase-6 diff (user input → argv) — verify: review, AI
- [ ] `api.md` + `guide.md` updated — verify: `documentation` agent, AI
- [ ] `web/dist` rebuilt — verify: freshness step, AI

### Release
- [ ] Full `go test ./...`, `(cd deploy/caddy/ingress && go test ./...)`, `go vet`, web checks, integration, e2e — verify: `verifier`, AI
- [ ] `bash deploy/vulncheck.sh` (a new npm dependency landed) — verify: AI
- [ ] `deploy/verify-commit.sh` on the release commit — verify: AI
- [ ] Tag + release v1.22.0 per `specs/styrman/versioning.md`, runtime image rebuilt at the same version (phase 1 forces it) — verify: user

## Cold-reader resolutions (round 1, 2026-09-04)

`reviewer` returned `design needs revision / implementation not ready`
(`.agents/docs/reviews/2026-09-04-coldread-ui-git-improvements.md`). All five
findings were accepted; none were rebutted.

1. **New endpoints' "no repo" contradicted `gitStatus`'s convention** — real
   contradiction with `git.go:107-114`. Resolved toward the existing
   convention: read endpoints (`branches`, `log`) return 200 with a `repo`
   flag, and "no commits yet" is `commits: []`, not 409. Mutating endpoints
   keep 4xx. The split and its reasoning are now stated in Interfaces.
2. **`GitTab` had no `term`, which Phase 5 needs** — correct, and
   `SidePanel.tsx` was missing from Surfaces touched entirely. Added to the
   table and as its own Phase 5 task box; `SidePanel` already threads `term`
   to `PortsTab` (`:97`), so it is a one-line change once it is visible.
3. **Editor font live-apply had no mechanism** — correct, and the asymmetry
   with the fully-specified terminal path was a genuine hole. Decided: the
   editor DOES live-update, via a new `fontComp` `Compartment` mirroring the
   existing `themeComp`/`wrapComp`. Full subsection added to Interfaces,
   three task boxes in Phase 2, and an e2e assertion that cursor and scroll
   survive.
4. **`DiffTarget` could not carry the commit subject its header showed** —
   correct. Resolved by adding `subject?: string` to the target, supplied by
   the log view that already has it, rather than a second fetch or a new
   single-commit endpoint. `DiffPane` now renders only what its inputs carry.
5. **Editor default 12px → 13px would have been an unannounced restyle** —
   the best catch of the five. The two surfaces' current values differ in
   BOTH size and family (terminal 13 px Google Sans Code; editor 12 px bare
   system stack), and the plan had flattened them into one default. Fixed:
   every default is now exactly today's value, so nothing changes for anyone
   until they touch a control.

## Cold-reader resolutions (round 2, 2026-09-04)

`reviewer`, fresh reader, verified all five round-1 fixes against the code and
found six new gaps
(`.agents/docs/reviews/2026-09-04-coldread-ui-git-improvements-round2.md`).
All six accepted; none rebutted.

1. **`?commit=` had no specified git invocation** — resolved to
   `git show --format= --patch --no-color <hash> --`, chosen because
   `<hash>~1..` fails on a root commit. Merge-commit behaviour stated.
2. **`gitDiff` swallows exit codes** — the reviewer read this as an open
   question; checking the code confirmed it is **deliberate** in the
   untracked branch, because `git diff --no-index` exits 1 when files differ
   (`git.go:353-356`). So: the new `show` branch checks its code (→ 404 on an
   unknown revision, which would otherwise be a 200 with an empty diff,
   indistinguishable from "changed nothing"); the existing branches are left
   exactly as they are, with a comment so the asymmetry does not read as a
   bug. A blanket "fix" here would have broken every untracked-file diff.
3. **`stash list` had no shape** — now structured entries like `log` and
   `branches`, parsed server-side. The frontend does not parse porcelain.
4. **`DiffPane`'s mount lifecycle was unspecified** — resolved to copy
   `EditorPane`'s `diffUsed` ref + hidden-div pattern verbatim, with the
   reasoning (two prior review findings) cited so it is not re-derived.
5. **Third segment button's empty state was undecided** — always rendered,
   with an empty state mirroring the editor's wording; the conditional
   alternative was considered and rejected (header controls shifting under
   the pointer, and undiscoverability).
6. **`FONT_STACKS` literal did not match today's string** — the plan's
   3-part stack dropped three fallbacks from the real 6-part chain at
   `terminal.ts:104-106`, quietly contradicting round-1 finding 5's own fix.
   Both literals are now copied verbatim, with a note on why the unbundled
   `'JetBrains Mono'` entry stays.
7. **`encodeURIComponent` would have ended up inconsistent** — resolved by
   encoding the whole git block including the seven existing methods, and by
   narrowing rather than ticking the TODO item.

## Cold-reader resolutions (round 3, 2026-09-04)

`reviewer`, fresh reader, verified round 2's fixes and found five more
(`.agents/docs/reviews/2026-09-04-coldread-ui-git-improvements-round3.md`).
This is the workflow's third round, so the operator was asked at the cap
(2026-09-04) and chose "apply the five, start phase 2" — the findings had
narrowed to local implementation decisions rather than requirement gaps, and
none of the five changed anything the operator asked for. All accepted.

1. **The appearance store section contradicted itself** — "one key per
   preference" in one paragraph, `styrman.appearance.v1` "one JSON object"
   two paragraphs later. Drafting error. Resolved to the single object with
   per-field fallback, and the stale sentence is gone.
2. **`settings.tsx:704-710` is a second `<CodeEditor>` call site** that the
   plan never listed. Mirroring the required `dark`/`wrap` props would have
   broken `npm run check` on a file this feature has no business touching.
   Resolved: the two new props are OPTIONAL with today's values as defaults,
   so the credential viewer stays byte-identical. Added to Surfaces touched.
3. **VM-stopped behaviour for `branches`/`log` was unspecified** — the file
   holds two conventions and the plan's generic wording did not say which.
   Resolved to the read convention (200 + `state:"stopped"`), consistent with
   the read/write split already recorded above.
4. **`stash push` did not specify `-u`** — resolved to `-u`, on the same
   reasoning that picked `add -A` over `commit -a`. Guessing the other way
   silently strands untracked WIP.
5. **`stash pop` conflict handling was unaddressed** — resolved to a 409
   carrying git's line, with the tree already modified and the stash entry
   kept, and an explicit ban on auto-resolving. Both the doc and the toast
   have to say the entry was kept.

## Risks

1. **The runtime image must be rebuilt and re-released** for the skel change,
   and `versioning.md` requires the three images to line up at one version.
   A phase-1-only "quick fix" is not deployable on its own.
2. **Existing project homes keep `$HOME/go`** and anything `go install`-ed
   there leaves `PATH`. Accepted by the operator ("new project homes only");
   it must be stated in the release notes or it will read as a regression.
3. **`web/dist` is committed and `go:embed`-ed.** Every frontend phase must
   rebuild it, or `deploy/verify-commit.sh`'s freshness step fails at the
   gate — after five phases of work. Rebuild per phase, not at the end.
4. **e2e locator churn.** The header segment control gains a third button and
   the recents cap changes; both are asserted today. Expect `run.mjs` edits
   in phases 4 and 5, and re-run the full 62 checks, not the new ones alone.
5. **Phase 6 is the security-relevant one.** It is the first path taking
   user-supplied strings into git argv. Validation lands *before* the
   handlers (first task in the phase), and `security` reviews the phase diff
   regardless of size.
6. **`set()`'s no-op guard** in `editorPane.ts` silently swallows state
   changes for any field not in its comparison. Adding `diff` to the state
   without adding it to the guard produces a bug that looks like "the second
   diff click does nothing" — called out in Interfaces, tested in phase 5.
7. **Bundle size** from a third font family. Mitigation and the fallback
   (ship `system` only) are in Interfaces; the measured delta goes in the
   verification log.
8. **Two HTTP conventions live in one file** after this change: read
   endpoints report absence as 200 + a flag, mutations report it as 4xx.
   That is `git.go`'s existing split and the plan now follows it, but it is
   the kind of thing a later contributor "fixes" into inconsistency. The
   reasoning is recorded in Interfaces and belongs in `api.md`.
9. **Scope size.** Six phases in one release. Each phase is an independent
   checkpoint (commit + push), so the release can be cut short at any phase
   boundary if something bites.
