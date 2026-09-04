---
owner: reviewer
date: 2026-09-04
---
# Cold-read review — UI + git improvements plan   (2026-09-04)

Range reviewed: `.agents/docs/plans/2026-09-04-ui-git-improvements-v1.22.0.md` (plan
document, not a diff), cross-checked against the code surfaces it names:
`web/src/routes/p.$project.$term.tsx`, `web/src/lib/editorPane.ts`,
`web/src/lib/terminal.ts`, `web/src/components/TerminalPane.tsx`,
`web/src/components/CodeEditor.tsx`, `web/src/components/SidePanelGit.tsx`,
`web/src/components/SidePanel.tsx`, `web/src/lib/api.ts`,
`internal/httpapi/git.go`, `internal/httpapi/server.go`, `REQUIREMENTS.md`,
`web/e2e/run.mjs`.

## Verdict: design needs revision / implementation not ready

The plan is unusually thorough (interfaces, failure modes, phased tests) and
most of its claims about the existing code check out exactly as cited. But it
has one real API-semantics contradiction with the existing file it extends,
one missing caller wire-up needed for its own Phase 5 to compile as designed,
and one asymmetry where the Terminal gets a fully specified live-reconfigure
mechanism while the Editor — whose current font is baked into a non-reactive,
non-compartmentalized `EditorView.theme(...)` — gets none.

## Gaps (must be decided before implementation)

1. **New git endpoints' "no repo" status contradicts the existing convention
   in the same file.** The plan's endpoint table (plan lines 203–218) gives
   `GET …/git/branches` a `404 no repo` and `GET …/git/log` a `404 no repo;
   409 no commits yet`. But `gitStatus` (`internal/httpapi/git.go:94-119`,
   specifically lines 108-114) treats "not a git repo yet" as a **200** with
   `Repo: false` — a soft state the panel already renders calmly ("Not a git
   repository yet — it initializes on first boot", `SidePanelGit.tsx:61-67`).
   The plan does not say whether the new "no repo" case is a deliberate,
   different HTTP contract (fine, but then it needs its own frontend
   handling, since the panel's existing `!data.repo` branch never fires for
   these calls) or a drafting slip that should match `gitStatus`'s 200+flag
   shape. This changes both the Go handler and the `git_verbs_test.go` table
   the plan specifies as the first Phase-6 task.

2. **`GitTab` has no `term` prop, but Phase 5 requires one.** The plan says
   Git-tab file rows must call `openDiffInPane(project, term, target)`
   (Interfaces, "Diff pane"; Phase 5 task "Git-tab file rows route to the
   main pane"). `openDiffInPane`'s signature takes `project, term, target`
   (plan lines 176). But `SidePanel.tsx:100` calls `<GitTab project={project}
   />` — no `term`, even though `SidePanel` already receives and threads
   `term` to sibling tabs (`PortsTab project={project} term={term}` at line
   97, `SidePanel.tsx:28-34`). The plan's "Surfaces touched" table (plan
   lines 79-95) never lists `SidePanel.tsx`, so an implementer following the
   table literally will not find the one-line caller change needed to make
   `GitTab` compile against the new interface. Small fix, but it is a caller
   the plan's own flow requires and omits.

3. **Editor font live-apply has no mechanism, unlike the terminal's fully
   specified one.** The Terminal Interfaces section (plan lines 142-158)
   spells out `term.options.fontSize/fontFamily` assignment + `fit()` +
   resize-frame, and even cites the exact re-measure gotcha already present
   at `TerminalPane.tsx:287-309` ("reassigning a font option forces xterm's
   char-size service to re-measure"). The Editor gets no equivalent
   subsection — only a phase task, "Editor font wiring in
   FileEditor/CodeEditor" (plan line 372). But `CodeEditor.tsx:244-253`
   bakes `fontSize: "12px"` and the `fontFamily` into a bare
   `EditorView.theme(...)` extension inside the mount-once effect — not
   wrapped in a `Compartment`, unlike `themeComp`/`wrapComp`
   (`CodeEditor.tsx:163-166, 213-214`) which exist specifically so dark-mode
   and wrap can reconfigure in place without remounting. Nothing in the plan
   says whether editor font must live-update an already-open buffer (needing
   a new `fontComp` Compartment + reconfigure effect, mirroring the existing
   pattern) or may apply only on next file open (since `CodeEditor` already
   remounts on filename change, per the comment at `FileEditor.tsx:124`).
   The E2E golden path (plan lines 337-346) only asserts live-update for the
   *terminal* font change ("the open terminal re-renders … without dropping
   its tmux session") — the editor is never exercised for this. That silence
   is itself ambiguous: is it an intentional scope cut (editor need not
   live-update) or an oversight? As written, a first-pass implementer could
   plausibly ship either behavior and both would pass every test the plan
   specifies.

4. **`DiffTarget` cannot carry what the DiffPane header is specified to
   show.** The Diff pane spec (plan lines 193-201) says the header shows
   "the path (or 'Working tree' / **the commit subject**)". `DiffTarget`
   (plan lines 163-168) carries `commit?: string` — the hash only, no
   subject. `GET …/git/log` (plan line 216) is the only endpoint that
   returns a commit's `subject`, and it returns a *list*, not a
   single-commit lookup. The plan does not say how `DiffPane` (which only
   receives `{project, term, onShowTerminal}` and reads the target from the
   pane store, per plan lines 193-195) is meant to obtain the subject text
   for the header when a log entry is clicked in Phase 6 — whether
   `DiffTarget` gains a `subject` field populated by the log view at open
   time, or `DiffPane` does a second fetch (`git show -s --format=%s <hash>`
   or similar, not in the endpoint table) to look it up. This is a genuine
   missing field/endpoint, not an implementation-freedom question, because
   there is currently no way to satisfy the stated header requirement.

## Readiness

A first-pass implementer could ship Phases 1–4 without a question (GOPATH,
appearance store, explorer navigation, recent files are all fully specified
down to exact line ranges and clamping rules, and I did not find a
discrepancy against the cited code). Phases 5–6 (git diff pane + git verbs)
cannot be shipped as specified without resolving the four gaps above.
Questions I would have to ask the author before starting Phase 5/6:

1. Do the new git endpoints' "no repo" cases return 404 as tabulated, or 200
   with a soft flag to match `gitStatus`'s existing convention — and if 404
   is deliberate, what does the panel show for it (a toast? the same calm
   "not a git repository yet" text, wired some other way)?
2. Who threads `term` into `GitTab` — is `SidePanel.tsx:100` updated to pass
   `term={term}`, or does `GitTab` get `term` some other way (a route hook)?
3. Must an already-open editor's font live-update when Settings changes it,
   the way the terminal does — requiring a new CodeMirror `Compartment` — or
   is "applies on next file open" acceptable? What should
   `CodeEditor.tsx`'s font-related props look like (a `fontSize`/`fontFamily`
   pair mirroring `dark`/`wrap`)?
4. Where does the commit subject shown in the diff-pane header come from
   when opened from the Log view — a new field on `DiffTarget` populated by
   the caller, or a second API call? If a new call, what is its shape (not
   in the endpoint table)?
5. (Minor, worth a one-line confirmation, not blocking) The editor's current
   hardcoded size is `12px` (`CodeEditor.tsx:247`); the new
   `Appearance.editorFontSize` default is `13` (plan line 110). Is the
   ambient default size change (12→13 for every existing user on first
   release) intentional, or should the shipped default match today's 12px?

## Criteria

1. Dead code — n/a for a plan document; the plan itself correctly identifies
   one piece of code to leave alone (the `../` row, `draft`'s omission from
   `set()`'s guard) rather than pruning speculatively.
2. Flow tracing — fails at one hop: `openDiffInPane(project, term, …)` is
   called from a component (`GitTab`) that is not given `term` by its only
   caller; see Gap 2.
3. Standards conformance — the plan explicitly follows `writing-files.md`
   (BUG-005: new/suffixed test files, called out by name) and
   `guest-writable-paths.md` is not implicated (no new guest-writable path).
   Git input-validation section (plan lines 228-238) matches the
   `gitExec`-never-a-shell pattern already in the file.
4. Directory layout — new files (`DiffPane.tsx`, `*.test.ts` suffixed,
   `git_verbs_test.go`) all land beside their siblings per existing
   convention; no issue found.
5. Simplicity — no new dependency beyond one justified, conditionally-cut
   font package; git argv validation is hand-written but is input
   sanitization, not auth/crypto, so the stdlib-first bar applies and is met
   (regex + `check-ref-format`, no shell).
6. Decoupling — the `DiffView` extraction (shared by side panel and main
   pane) is the right call and is stated as deliberate; the diff-pane header
   needing commit-subject data it cannot reach (Gap 4) is itself a
   decoupling smell — `DiffPane` is being asked to render data its own
   inputs cannot supply.

design needs revision
implementation not ready
