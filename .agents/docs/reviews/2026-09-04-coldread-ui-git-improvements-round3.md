---
owner: reviewer
date: 2026-09-04
---
# Cold-read review (round 3) — UI + git improvements v1.22.0

Range reviewed: `.agents/docs/plans/2026-09-04-ui-git-improvements-v1.22.0.md`
(plan document), cross-checked against the code surfaces it names.

## Verdict: design needs revision / implementation not ready

The plan is far more rigorous than the two rounds before it — most of what I
checked against the actual code matched exactly (the theme-store pattern,
the `set()` no-op guard, `themeComp`/`wrapComp`, the `editorUsed` mount
pattern, `GitTab`'s missing `term`, `gitDiff`'s exit-code swallow, the author
injection lines). But five concrete points remain undecided or internally
contradictory, and one is a genuine missed call site that will fail
`npm run check` if implemented exactly as mirrored. None of these are
stylistic — each is a decision a first-pass implementer cannot make from the
document alone.

## Gaps

- **[medium/high confidence]** Appearance store: contradictory key model.
  Interfaces §"Appearance preference store" says "written through to
  `localStorage`, **one key per preference** so a corrupt value cannot take
  the others down" (plan text right after the type block), then two
  paragraphs later: "Keys: `styrman.appearance.v1` (**one JSON object**)."
  These are mutually exclusive designs (4 `localStorage` keys vs. 1). The
  per-field fallback description ("a parse failure, a missing key... falls
  back to the default for that field alone") only makes sense for the
  single-JSON-object reading, so that's probably the intended one — but the
  plan says the opposite in the same section, and an implementer has no way
  to know which sentence is stale.

- **[high confidence]** `web/src/routes/settings.tsx:704-710` — an untouched
  second `<CodeEditor>` call site. The Interfaces section for the editor font
  ("`CodeEditor.tsx:183`") says `CodeEditor` gains `fontSize: number` and
  `fontFamily: string` as props "mirroring the existing `dark` and `wrap`
  props" — which are non-optional in the current signature
  (`CodeEditor.tsx:142-155`: `dark: boolean; readOnly: boolean; wrap: boolean`,
  no `?`). Mirrored that way, `fontSize`/`fontFamily` are required too, and
  `settings.tsx:704`'s read-only credential/secret viewer — the file's ONLY
  other `<CodeEditor>` usage, not in Surfaces touched, not in Phase 2's task
  list — will fail to compile without them. The plan never says whether this
  viewer should read the appearance store too, or get its own hardcoded
  fallback values, or whether the two new props should in fact be optional
  (which would also work but isn't what "mirroring dark/wrap" implies).

- **[medium confidence]** VM-stopped behavior for the two new *read*
  endpoints is unspecified/contradictory. The endpoint table's Error column
  for `GET …/git/branches` and `GET …/git/log` lists only "502 git failure"
  — no VM-stopped row. The Failure-modes table says "Every new endpoint
  inherits `gitContext`'s `errVMNotRunning` → the panel's existing 'Start
  the project to use git.'" — worded as if uniform, but the two EXISTING
  conventions it could mean produce different HTTP responses: `gitStatus`/
  `gitDiff` special-case `errVMNotRunning` into a 200 with a `state`/`stopped`
  field (`git.go:96-101`, `:334-338`); every mutating handler instead routes
  it through `gitCtxErr` → 409 (`git.go:379-384`). The Interfaces section's
  "read endpoints report... as 200" principle is stated for "no repo"/"no
  commits", not explicitly for "VM stopped" — so it's a real open question
  which of the two existing patterns `branches`/`log` should follow, and the
  answer changes what `git_verbs_test.go` has to assert.

- **[medium confidence]** `stash push`'s exact invocation is unspecified.
  Unlike `commit {all:true}` (`git add -A`, explicitly justified over
  `-a`) and `diff?commit=` (`git show --format= --patch --no-color`, with
  reasoning), the plan never states whether `stash push` runs plain
  `git stash push` (leaves untracked files in the tree) or `git stash push -u`
  (captures them too). Given the fast-commit rationale a page earlier
  ("`-A`, not `commit -a`, because `-a` skips untracked files and 'commit
  everything' must include them"), an implementer has real reason to expect
  the same logic applies to stash — but the plan doesn't say so, and
  guessing wrong silently drops untracked WIP.

- **[medium confidence]** `stash pop` conflict handling is unaddressed. The
  Failure-modes table only covers "stash pop with an empty stack" → 409
  "No stash entries found." A `git stash pop` that conflicts with the
  working tree behaves differently from every other guarded mutation in this
  file (`checkout`'s dirty-tree 409 fires *before* any mutation; `pull
  --ff-only`'s failure also mutates nothing): it can partially apply the
  stash, leave conflict markers in tracked files, and still exit non-zero —
  and this project explicitly has no merge/conflict UI (Scope → Out: "Merge,
  rebase, cherry-pick..."). The plan's generic "409 with git's first line"
  treatment, copied from the clean-guard endpoints, doesn't say what the user
  sees or what state the tree is in after that specific 409.

## Readiness

Could a first-pass implementer ship Phase 1 through 6 without asking a
question? No — at minimum the five points above need a decision. In
addition:

- The working tree at review time already carries **uncommitted** edits that
  are byte-for-byte Phase 1's task list (`image/skel/bash_exports:7`,
  `image/skel/bashrc:24`, `README.md:34` all already read `.go`/`$HOME/.go`;
  `git diff` confirms these are unstaged modifications, not prior commits).
  `ROADMAP.md` is also already edited to say v1.22.0 is "in flight." A
  first-pass implementer needs to ask whether this is the start of this very
  plan (continue and commit it as Phase 1) or stray work to discard before
  starting — proceeding either way without asking risks losing or
  double-describing that work.
- Where does the extracted shared `DiffView` component live? The plan says
  "extracted... into a shared component" (used by both `SidePanelGit.tsx` and
  the new `DiffPane.tsx`) but names no destination file, and it isn't in
  Surfaces touched. Low stakes, but it's a real omission — ask, or pick
  `web/src/components/DiffView.tsx` and note the choice.
- Does `GET …/git/diff?commit=<hash>` take precedence over `path`/`staged`/
  `untracked` if a caller (or a test) sends both? Not specified. Low
  real-world risk since the frontend only ever sends one shape, but a table
  test (per Verification, Tier 1) would need to pick a behavior.

## Criteria

1. Dead code — n/a, planning document; no removals specified as retained.
2. Flow tracing — fails at one point: the `CodeEditor` prop change doesn't
   trace to its second caller (`settings.tsx:704`).
3. Standards conformance — plan correctly cites and mirrors existing
   patterns (theme store, compartments, gitCtxErr/writeErr, `-A` vs `-a`
   reasoning) everywhere I checked except the two contradictions above.
4. Directory layout — Surfaces touched table is accurate for every file I
   opened except the missing `DiffView` destination and the missing
   `settings.tsx` mention.
5. Simplicity — no over-engineering found; `check-ref-format` reuse of git
   itself for ref validation is the right call, not a hand-rolled regex.
6. Decoupling — the shared `DiffView` extraction and `openDiffInPane` are a
   clean seam; the one soft spot is the unnamed destination file for the
   extracted component.

design needs revision
implementation not ready
