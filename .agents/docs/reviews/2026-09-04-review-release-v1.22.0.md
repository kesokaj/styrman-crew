---
owner: reviewer
date: 2026-09-04
---
# Review — release v1.22.0 (ui+git improvements, six phases)   (2026-09-04)

Range reviewed: v1.21.0..acb5918

Scope as dispatched: CROSS-PHASE interactions and whole-release coherence only.
Per-phase findings already recorded in `reviews/2026-09-04-review-phase{2..6}-*.md`
and `reviews/2026-09-04-security-phase{2,5,6}-*.md` are NOT re-litigated — except
where I could show a fix landed only partially across the release, which is
precisely what a per-phase review could not see.

## Verdict: APPROVE WITH NITS

Nothing in this range is broken as shipped: `go build ./...`, `go vet ./...`,
`go test ./internal/httpapi/` and `web` `vitest` (69 tests / 7 files, matching the
testing README's new count) all pass, and the committed `web/dist/client` really
does contain the phase 3–6 UI ("Fast commit", "Stash all", "No diff open",
"Resize recent files", "Up one level", `styrman.recents.height`), so the embedded
SPA matches the reviewed source. The worst thing I found is a security remediation
that was applied to two of the four call sites it named: `internal/httpapi/git.go`
still returns raw, uncapped, unstripped guest/remote git output from `gitPush` and
from `gitCommit`'s commit failure, including the one site (`push`) the phase-5
security review singled out as the realistic leak. Everything else is a low-severity
truth-in-labelling gap and one genuine cross-phase behaviour hole (git verbs that
rewrite the working tree do not tell the editor pane its buffer is now stale).

## Findings

- **[medium / confidence: high]** `internal/httpapi/git.go:339` and `:371` — the
  `firstLine(out)` → `guestErrMessage(...)` remediation is half-applied across the
  release. The phase-5 security report says "switch git.go's four `firstLine(out)`
  call sites"; the phase-6 report repeats it for `gitRun` and adds "Pre-existing in
  `gitRun`/`gitPush`; widened here". Phase 5/6 fixed two sites — the `add -A` branch
  (`:318`) and `gitRun` (`:728`) — and left `gitCommit`'s 409 (`:339`) and
  `gitPush`'s 502 (`:371`) on the raw helper. `firstLine` applies no
  `stripControls`, no `redactSecrets` and no length cap; `gitPush`'s first error
  line is exactly the text a remote server influences (`remote:` sideband) and the
  one that can carry a URL. The result is that the file now has two conventions for
  the same class of text, with a comment at `:314` and `:722` asserting the safe one
  is the rule. Fix: `writeErr(w, http.StatusConflict, errors.New(guestErrMessage("git commit failed", out)))`
  at `:339` and `writeErr(w, http.StatusBadGateway, errors.New(guestErrMessage("git push failed", out)))`
  at `:371`, or make `firstLine` itself apply `redactSecrets(stripControls(...))`
  plus the cap and delete the distinction.

- **[medium / confidence: medium]** `web/src/components/SidePanelGit.tsx` (whole
  file) vs `web/src/lib/editorPane.ts:evictFromPanes` — phase 6 added four verbs
  that rewrite the working tree (`checkout`, `pull`, `stash push`, `stash pop`) on
  top of the pre-existing `discard`, and none of them notifies the editor pane.
  `EditorPane` loads a file exactly once per `wanted` change and never re-reads it;
  `FileExplorer` polls its listing, but the open buffer does not. So: open
  `web/src/foo.ts` in the pane, switch branches or pull in the Git tab, press Save —
  the pre-switch content is written over the new one, silently, with no conflict and
  no prompt. With phase 5's parked `draft` the stale buffer also survives a terminal
  switch. The project already has the exact hammer for this: `evictFromPanes` was
  added for "content replaced underneath the editor (an upload)". `SidePanelGit`
  imports only `openDiffInPane`. Fix: call `evictFromPanes(project, {scopeKind:
  "project", path})` for a discarded path, and after `checkout`/`pull`/`stash`
  either evict the whole project's panes or force a reload of the open file.
  Confidence is medium only on how often it bites, not on the mechanism, which I
  traced end to end.

- **[low / confidence: high]** `web/src/components/SidePanelGit.tsx:185` — the Fetch
  button's tooltip reads `git fetch --all --prune (updates remote refs, touches no
  files)`. The server deliberately does NOT prune: `git.go:460-472` and
  `specs/styrman/api.md` both record dropping `--prune` as a security decision
  (a guest-written `remote.<name>.fetch` refspec makes a pruning fetch delete local
  branches). The UI is now the only place in the repo that still claims the
  behaviour the release removed on purpose. Delete `--prune` from the title.

- **[low / confidence: high]** `web/src/components/DiffPane.tsx:101` and
  `SidePanelGit.tsx:381` — the release added `reveal()` for the diff BODY
  (`DiffView.tsx`, with `diffReveal.test.ts` behind it) because a bidi/control byte
  can make a line render differently from how it applies. The commit SUBJECT, which
  phase 6 introduced and phase 5's `DiffTarget.subject` carries straight into the
  pane header, is rendered raw in both the log list and the diff header. Same
  untrusted source (a commit in a cloned repo), same pane, same purpose, opposite
  treatment. No XSS — React escapes text children — so this is deception-only, and
  low. Fix: run the subject through the same `reveal` on render.

## Criteria

1. **Dead code** — pass. No orphaned exports or commented-out blocks introduced;
   `editorPane.ts`'s new `openDiffInPane`/`DiffTarget` have real callers, and every
   new `lib/` module (`explorerHistory`, `recentsPanel`, `fonts`) is imported. The
   two half-dead conventions I did flag (`firstLine`) are a live inconsistency, not
   dead code.
2. **Flow tracing** — one hole, finding 2 (tree-mutating git verbs → stale editor
   buffer). Otherwise the seams hold: `term` is plumbed `route → SidePanel → GitTab`
   and is a required `string` on the only mount site, so `openDiffInPane` can never
   key an orphan pane; the third pane view's lazy-mount latch (`diffUsed`) mirrors
   `editorUsed` exactly; `openFileInPane`/`openDiffInPane` each carry the other's
   state through, so an editor with unsaved edits plus an open diff keeps `dirty`,
   `draft`, `file` and `diff` simultaneously; `set()`'s no-op guard correctly gained
   `prev.diff`; `evictFromPanes` leaves `view` alone so a delete while on the diff
   view does not flip the pane. Go side: every new handler goes through
   `gitContext` → `gitCtxErr` (mutations) or the 200-with-`state` shape (reads),
   every one takes an explicit `context.WithTimeout`, and the read/mutation status
   split is stated in one comment at `git.go:443` and honoured by all six.
   `DiffPane`'s generation ref correctly covers the manual-refresh path the effect
   cleanup could not.
3. **Standards conformance** — pass. `go build`, `go vet`, `go test ./internal/httpapi/`
   green; `vitest` 69/69. Guest-supplied strings that become argv are gated before
   any git call (`validCommitHash`, `validBranchName`, the closed `stashOps` set),
   with `git_verbs_test.go` pinning the argv-abuse cases and the deliberate
   deferral of ref-format validity to git. No new dependency in `go.mod`; the one
   `web/package.json` addition is a self-hosted font package, and
   `deploy/verify-commit.sh` gained a gate that fails the build if any CSS
   references an absolute http(s) origin.
4. **Directory layout** — pass. New Go code stays in `internal/httpapi/` beside its
   siblings with the test next to it; new frontend logic lands in `web/src/lib/`
   with each test file adjacent; `DiffView`/`DiffPane` sit in `web/src/components/`.
   Nothing at the repo root.
5. **Simplicity (DIY-except-security)** — pass. No hand-rolled auth, crypto,
   hashing, sessions or JWT/OIDC anywhere in the range. `crypto.randomUUID()` is
   used as a commit MESSAGE, not as a token. `explorerHistory` and `recentsPanel`
   are pure reducers/parsers extracted specifically so the edge cases are testable
   without a renderer — justified, not speculative.
6. **Decoupling** — pass, with the one coupling GAP that is finding 2: the pane
   store is the seam every surface already uses (`notifyFileSaved`,
   `evictFromPanes`), and the git panel is the one new writer that does not use it.
   Otherwise `DiffView` was extracted rather than copied when diffs gained a second
   home, `DiffPane` renders from its own inputs (the `subject` travels with the
   target) instead of refetching, and the font vocabulary lives in `lib/fonts.ts` so
   `terminal.ts` (no React) and `hooks.ts` share one union type.

## Cross-phase checks that came back clean

- **dist/source coherence** — the committed `web/dist/client` bundle contains every
  phase 3–6 string I probed; the embedded SPA is not stale. (Note for future
  reviewers: these chunk files are detected as binary by `grep`; use `grep -a`.)
- **Documentation coherence** — `specs/styrman/api.md` documents all six new
  endpoints plus `?commit=` and `"all"`, including the deliberate asymmetries
  (only the `commit=` branch checks the exit code; reads 200, mutations 4xx).
  `testing/styrman/README.md` moved its counts 65→68 integration and 62→82 e2e and
  17→69 unit, and the three new integration checks exist in `integration.sh`.
- **BUG-006** — both tracked files under `web/.claude/agent-memory/documentation/`
  are present at HEAD and absent from the range's diffstat, so the accidental
  `rm -rf` really was recovered; `.agents/rules/writing-files.md` gained the
  deleting/working-directory clauses the bug record asked for.
- **Phase 1 GOPATH move** — the only remaining `$HOME/go` reference in the repo is
  the one in `specs/styrman/runtime-image.md` that deliberately explains that
  existing homes keep the old path (skel is seeded write-once). Nothing in
  `image/`, `deploy/`, `internal/` or `cmd/` still points at `$HOME/go/bin`.
- **Terminal hidden behind the third view** — the font effect's immediate refit
  (`TerminalPane.tsx:123`) cannot fire while the terminal is hidden, because the
  only writer lives on `/settings`, a different route that unmounts it; and every
  path back to the terminal (`toggle`, `EditorPane`'s X, `DiffPane`'s X) goes
  through `showTerminal()`, which bumps `focusToken` and drives the existing
  refit-after-unhide mitigation. No new zero-size-fit path.

## What I did not check

- **Runtime behaviour.** I ran no integration script, no Playwright e2e, and no
  real Kata VM. Every git handler's behaviour against actual git is reasoned from
  the code and the recorded verifications, not observed. Findings 1 and 2 are
  static traces; I did not reproduce the stale-buffer overwrite in a browser.
- **`web/e2e/run.mjs` (+335 lines).** I did not read the new e2e checks, so I
  cannot say whether the twenty added checks actually cover the combinations the
  dispatch asked about — in particular I did not verify that anything exercises
  "open editor with unsaved edits + an open diff + a terminal switch" end to end.
  The testing README claims six appearance / two nav / three recents / three
  diff+fast-commit / six git-verb checks; I took that on trust.
- **Phase 2 appearance surface in detail** (`settings.tsx`, `CodeEditor.tsx`,
  `fonts.ts`, `index.css`, the five new woff2 subsets). I read the store in
  `hooks.ts` and the terminal wiring, and confirmed the tests pass and the CDN gate
  exists, but I did not review the Settings UI or the CodeMirror font application.
- **`npm run check` / `npm run lint`** were not run in this session (only `npm test`).
- **The vendored Caddy ingress module** — untouched by this range, not tested here.
- The per-phase reviews and security reports were read only far enough to avoid
  re-litigating them; I did not independently re-verify that each of their other
  findings was fixed, except the `firstLine` one that finding 1 reports.
