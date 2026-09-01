# Terminal ordering fix, Dockerfile/dotfile highlighting fix, and the editor moves from a modal to a main-pane view

Date: 2026-09-01. Three operator-requested fixes/features, unrelated to the
same-day harness re-sync (`2026-09-01-harness-upgrade-v1.15.0.md`, separate
entry, do not conflate).

## 1. Terminal ordering fixed (`internal/conversation/store.go`)

`List` sorted by `LastOpened` descending, so the sidebar reshuffled itself
every time a terminal was opened — the just-opened terminal jumped to the
top. It now sorts by `Title` in natural/alphanumeric order via a new
`naturalLess` helper: digit runs compare numerically so "Terminal 2" sorts
before "Terminal 10" (a plain byte-wise sort puts "10" first), comparison is
case-insensitive, and ties break on `ID` for stable ordering across records
with identical titles. `LastOpened` is still recorded by `Touch` — it's just
no longer an ordering key. New tests `TestListNaturalOrder` and
`TestNaturalLess` in `internal/conversation/store_test.go`.

## 2. Dockerfile + shell-dotfile syntax highlighting fixed (`web/src/components/CodeEditor.tsx`)

The grammar registrations were already correct; the *lookup* was broken.
`FileExplorer` passed `filename={file.path}` — a full path — into
`CodeEditor`, and CodeMirror's `LanguageDescription.matchFilename` tests each
grammar's `filename` regex against the *whole string it's given*, so
`projects/x/Dockerfile` never matched `/^(dockerfile|containerfile)…/i`.
Extension-matched grammars (`.go`, `.py`, etc.) kept working because
`matchFilename` falls back to splitting on the last dot when no `filename`
regex matches — which is exactly why only the two filename-matched entries
looked broken, and why this same bug silently broke `.bashrc`/`.profile`/
`.zshrc` highlighting too, not just Dockerfiles. Fixed by matching on the
basename inside `CodeEditor` (`filename.split("/").pop()`), one place
covering both call sites (the pane and the modal share this component). The
Dockerfile regex was also broadened to `/^(dockerfile|containerfile)(\..*)?$/i`
so `Dockerfile.dev` and the lowercase spellings match, alongside the existing
`dockerfile`/`containerfile` extensions.

## 3. Editor moved from a modal to a main-pane view, with word wrap

Previously the editor was a centered modal launched from the 320px side
panel, covering the terminal underneath. It's now a second view of the SAME
main pane the terminal occupies, toggled via a `Terminal | Editor` control in
the terminal header, with state kept **per terminal** — one terminal in a
project can sit on an editor while another shows a live shell. Clicking a
file in the Files panel opens it in that terminal's editor pane and switches
the pane to it automatically.

New files:
- `web/src/lib/editorPane.ts` — module-level store keyed
  `${project}/${term}` via `useSyncExternalStore`. Deliberately module-level
  rather than route state: `TerminalPane` is keyed by project/term and
  remounts on every terminal switch, so route-local state would forget which
  view each terminal was on. Deliberately NOT persisted to localStorage —
  which file is open is session context, not a durable preference.
- `web/src/components/FileEditor.tsx` — `useFileEditorState` /
  `FileEditorToolbar` / `FileEditorBody`, extracted so the modal (still used
  by Settings, which has no terminal in context) and the new pane share one
  implementation instead of two copies.
- `web/src/components/EditorPane.tsx` — the pane wrapper.

`FileExplorer.tsx` shrank from 727 lines to 521 as the duplicated editor
logic moved into the shared module above.

**Word wrap** was added: `EditorView.lineWrapping` is applied through a
CodeMirror `Compartment` so toggling reconfigures the view in place instead
of remounting it and losing cursor position, scroll offset, and undo
history. A `Wrap` toggle sits in the editor toolbar, persisted to
`localStorage["styrman-editor-wrap"]`, default ON.

**Critical implementation constraint, worth recording so it isn't
regressed:** the terminal is hidden with CSS when the pane shows the editor,
and is NEVER unmounted. `TerminalPane`'s cleanup effect
(`TerminalPane.tsx:272-281`) closes the terminal's WebSocket on unmount —
unmounting on every toggle would kill the live session each time the
operator switched to the editor and back.

## Bug caught by the new e2e coverage (honest log)

The first version of `useFileEditorState` reset the editor's text in a
`useEffect` that fired when the open file changed. That runs too late:
`CodeEditor` remounts on a filename change (it's keyed by `filename` in its
own effect dependency array) and takes its *initial* document from the
`value` prop in the *same render* — so opening a second file in the pane
briefly showed the PREVIOUS file's content under the new file's name.
Fixed with a render-phase state reset (React's documented "adjusting state
when props change" pattern) instead of an effect. The new e2e Dockerfile
check caught this before it shipped.

## e2e runner was silently testing stale frontend assets (`web/e2e/run.sh`)

The runner built only the Go binary before running Playwright, but the
server embeds `web/dist/client` via `go:embed` — so any frontend change
under test was silently absent from the running server unless `npm run
build` had been run by hand first. This produced a real false failure during
this work: a fix appeared not to work because it simply hadn't been built.
`run.sh` now runs `npm run build` before the Go build, with
`STYRMAN_E2E_SKIP_WEB=1` to opt out when iterating on server-only changes.

## Verification

- `go build ./...`, `go vet ./...`, `go test -race ./...` — all ok, including
  the `deploy/caddy/ingress` module.
- `tsc -b` and `oxlint` — clean.
- `integration.sh` — 57 passed / 0 failed / 3 skipped.
- e2e (`web/e2e/run.mjs`) — 42 passed / 0 failed / 2 skipped, up from 35: 7
  new checks added covering file-click opens in the pane (not a modal),
  correct content is shown, toggling back to the terminal, **the terminal
  session survives an editor round-trip** (types a command, switches to the
  editor and back, still sees the output), reopening the same file, the wrap
  toggle, and Dockerfile syntax highlighting.
