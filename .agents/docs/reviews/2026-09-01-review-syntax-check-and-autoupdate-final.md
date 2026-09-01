# Review — syntax-check + auto-updater, FINAL pre-preflight pass (2026-09-01)

Third and final review of the same uncommitted working tree, after the remediation of
`.agents/docs/reviews/2026-09-01-review-syntax-check-and-autoupdate-rereview.md` and the
`security` review `.agents/docs/reviews/2026-09-01-security-editor-check-and-autoupdate.md`.

Scope: only what changed since the re-review (files touched after 12:24 today) —
`deploy/update/styrman-update{,.service}`, `deploy/Dockerfile.server`, `image/Dockerfile`,
`deploy/host-setup.sh` (step 8c), `internal/httpapi/{check.go,check_test.go}`,
`web/src/components/{TerminalPane.tsx,EditorPane.tsx,FileEditor.tsx}`,
`web/src/lib/editorPane.ts`, `web/src/routes/p.$project.$term.tsx`, `web/e2e/run.{mjs,sh}`,
and the durable docs (ADR-050 correction, ADR-051, changelog, `deploy.md`, `api.md`,
`README.md`, `guide.md`, `AGENTS.md`, `TODO.md`, testing README). Items already passed in
the first two reviews were not re-argued. `web/dist/**` is build output and was inspected
only for commit coherence.

Gates I re-ran myself (the verification log at 13:07 predates the last `check.go` edit at
13:11): `gofmt -l internal cmd` clean, `go vet ./internal/httpapi/` clean,
`go test ./internal/httpapi/ -count=1` ok, `npx tsc -b` clean,
`shellcheck -S error deploy/update/styrman-update deploy/host-setup.sh` clean.

## Verdict: APPROVE

Nothing in the reviewed delta is broken. All ten dispatched items check out on the code,
not just on the claim: the state files are genuinely outside the work tree and `STATE_DIR`
is created before the `flock` (`styrman-update:67`); `PYTHONPYCACHEPREFIX` replaces the
inert `PYTHONDONTWRITEBYTECODE`; `reports_version` now requires `.State.Running=true` and
pre-initialises `got`/`health`/`running` for `set -u`; `docker compose build` covers the
ingress; the `ARG`/`LABEL` pair is last in the final stage; `missingTool` rejects any
multi-line output and the false-positive cases are in `TestMissingTool`; the `checkerFor`
godoc is back on `checkerFor`; the security remediation (signed tags failing closed,
lightweight tags rejected, pinned `known_hosts`, root-ownership assertion before
`-c safe.directory`, tag-regex validation of state values, checksummed hadolint) is all
present and correct; the carried-over lows are recorded in `TODO.md` rather than lost. The
two new UI fixes are sound: the pane store's no-op bail still covers `dirty` so the publish
effect cannot loop, and the `onResizeRef` call cannot fire before the ref is assigned (the
`focusToken` effect no-ops on mount because `lastFocusToken` is seeded from the initial
prop, and the connection effect re-assigns the ref synchronously in the same commit as its
own cleanup, so no timer can observe a disposed terminal).

The one thing to do **before the commit**, and it is not a code change: `web/dist/client/_shell.html`
is tracked-and-modified and now references `/assets/index-CDEz8EmC.js` + `/assets/index-zmUsMBo-.css`,
which are **untracked**, while the old assets are staged-for-deletion. A `git commit -a`
would tag a tree whose shell HTML points at files that do not exist — and the auto-updater
could not catch it, because `/api/health` and the `styrman.version` label are both fine for
a blank UI. Stage with `git add -A -- web/dist`. Behind that, one medium that is *not* a
regression of this round but directly contradicts the feature just added: the new dirty dot
advertises "unsaved edits are parked in the editor", and switching to another terminal
silently throws them away.

## Findings

- **[medium / confidence: high]** `web/dist/client/_shell.html` + `web/dist/client/assets/**`
  — the shipped SPA is split across a tracked-modified HTML file and untracked hashed
  assets (`index-CDEz8EmC.js`, `index-zmUsMBo-.css`, and ~23 others), with the previous
  generation marked deleted. `deploy/Dockerfile.server` embeds `web/dist` (`web/embed.go`),
  and from this batch onward the auto-updater builds **from the tag**, so a commit that
  misses the untracked half produces an image whose HTML 404s its own bundle. Neither the
  container HEALTHCHECK (`/api/health`) nor `reports_version` (label + `.State.Running` +
  health) can see that, so a blank UI would deploy and report VERIFIED with no rollback.
  `web/e2e/run.sh`'s new note says the right thing; make it a preflight step:
  `git add -A -- web/dist` and confirm `git status --porcelain web/dist` is empty before
  tagging.

- **[medium / confidence: medium]** `web/src/components/EditorPane.tsx:41-44,92-95` +
  `web/src/components/FileEditor.tsx:122-131` — unsaved edits are silently discarded when
  the user switches to another terminal. `TerminalPage` is not remounted on a terminal
  switch and `EditorPane` carries no `key`, so its props change in place: `wanted` becomes
  the new terminal's `file` (usually `null`), the effect `if (!wanted && loaded) setLoaded(null)`
  fires, `useFileEditorState` falls back to `EMPTY` (`path: ""`), the render-phase
  `shownPath !== file.path` reset runs, and `text`/`savedContent` are replaced with `""`.
  No prompt — unlike opening a different file (`pending` + confirm) or closing the file
  (`confirmClose`). Not introduced by this round, but this round is the one that promises
  the user their edits are safely parked: the dot on the Editor tab says "unsaved changes"
  and the store comment says the dirty flag exists precisely so the state cannot be lost
  while the pane is hidden. Also a small consistency artefact: on the switch,
  `setDirty(false)` lands on the *new* terminal's key, so the old terminal keeps
  `dirty: true` in the store until its pane reloads. Fix in the same place the dirty flag
  now lives — park the buffer per `${project}/${term}` in the pane store (it already keys
  `view`, `file` and `dirty` that way) — or, at minimum, prompt on the switch. If it is not
  fixed in this cycle it belongs in `TODO.md` with the other carried-over items, not only
  in this report.

- **[low / confidence: high]** `web/src/components/TerminalPane.tsx:71-90` — the four-step
  `focusToken` schedule now calls `onResizeRef.current?.()` at 0/50/150/400 ms, and that
  handler sends a `{"type":"resize"}` frame unconditionally whenever `cols >= 10`. So one
  Editor→Terminal toggle sends up to four identical resize messages, each of which is a
  full tmux pane resize + TUI redraw inside the microVM — the exact cost the 120 ms
  debounce three lines above exists to avoid. It is not harmful (the guard blocks the
  collapsed size, and the closure reads the live `ws`, so a pre-connection call is a no-op
  and `onopen` re-sends anyway), just wasteful. Track the last-sent `cols`/`rows` in the
  effect scope and skip the send when unchanged; the refit itself is cheap and can stay
  on all four ticks.

- **[low / confidence: high]** `deploy/host-setup.sh` step 8c (auto-clone branch) — the
  updater was hardened to `StrictHostKeyChecking=yes` against a pinned
  `/etc/styrman/known_hosts`, but host-setup's own automatic clone still uses
  `StrictHostKeyChecking=accept-new` and never creates that file. The printed manual
  instructions do the `ssh-keyscan`; the automatic path does not. Result: if
  `/etc/styrman/deploy-key` exists at setup time, the clone succeeds via TOFU into root's
  `~/.ssh/known_hosts`, `deploy/.env` is later copied in, the timer auto-enables — and
  every tick then dies in `git fetch` with a message that blames the *deploy key*
  (`styrman-update:103`), never mentioning the pinned host-key file that is actually
  missing. Fix: `ssh-keyscan` into `/etc/styrman/known_hosts` in the auto-clone branch and
  use it there too, and add "or `/etc/styrman/known_hosts` is missing/stale" to the fetch
  failure message.

- **[low / confidence: high]** `deploy/update/styrman-update:122-133`, `deploy/host-setup.sh`
  step 8c, `.agents/docs/specs/styrman/deploy.md:304-316` — the signing docs cover the
  *signer* and never the *verifier*. `git verify-tag` runs as root on the deploy host and
  fails unless the operator's public key is in root's GPG keyring (or, for SSH signing,
  `gpg.ssh.allowedSignersFile` is set in **root's** git config on that host, which the
  parenthetical in host-setup does not make clear). A properly `-s`-signed tag will
  therefore still be refused on a fresh host, and the failure message says "Sign your
  release tags (git tag -s)" — advice the operator has already followed. Mitigated by
  `--check` catching it before the timer is enabled and by `git verify-tag`'s own stderr
  being logged. Add a fourth setup prerequisite ("install the signer's public key for
  root") and soften the message to "no valid signature *on this host*".

- **[low / confidence: high]** `.agents/docs/changes/2026-09-01-syntax-check-and-autoupdate.md:239,259`
  — still says e2e **47 passed / 0 failed / 2 skipped** in both places, after the suite grew
  by the two new checks. `AGENTS.md:98` says 48+2, the testing README says 50 (48+2), and
  `run.mjs` has 49 `await check(` calls plus the client-error gate. Three docs, and the
  changelog — the append-only record of what shipped — is the one left stale. Separately,
  `.agents/docs/testing/styrman/README.md:49,79-80` says "50 checks **+** a final
  client-error gate", but the 50 (48 pass + 2 skip) already *includes* that gate: it is 49
  checks + 1 gate. Off by one in the doc that declares itself the source of truth.

- **[low / confidence: high]** `image/Dockerfile:38-48` — two adjacent comments contradict
  each other. The first correctly says hadolint "is PINNED with a checksum, unlike the
  `latest` fetches around it"; six lines later the second says "Both are pinned to the same
  'latest release binary' pattern yq already uses". The second sentence describes the code
  that was replaced. Delete it (and note that `shellcheck`, which it also describes, comes
  from apt, so "both" was never right after the pin landed).

- **[low / confidence: high]** `internal/httpapi/check_test.go:182` — `func contains(s, sub
  string) bool { return strings.Contains(s, sub) }` is a one-line alias with a single
  caller (line 94). The hand-rolled `indexOf` is correctly gone; finish the job and call
  `strings.Contains` directly (criterion 5, stdlib-first — a wrapper whose interface equals
  its implementation is a shallow module).

- **[low / confidence: high]** `.agents/docs/architecture/DECISIONS.md` (ADR-050) — the new
  **Correction** block ends with its own `Source:`/`Verification:` footer, and the ADR's
  original footer follows immediately after, so ADR-050 now renders with two footers
  pointing at different documents. This is the same drift flagged for ADR-047/048 in the
  editor-pane review. Keep one footer at the end of the ADR and fold the correction's
  sources into it (or label it `Correction source:`).

- **[low / confidence: high]** `deploy/host-setup.sh` step 8c heredoc — "Two prerequisites,
  both root-only:" is followed by an enumerated list of **three**. The signing item was
  appended without updating the lead-in.

- Unchanged and still open, recorded only so the count is honest — all are now captured in
  `TODO.md`, so no action is required in this batch: `deploy()` still `fail`s (exits) on a
  checkout error instead of `return 1`; `Persistent=true` is still inert on the monotonic
  timer; step 8c is still physically before step 8b; `mktemp -p .` still writes into the
  workspace root; the `A || B && C` in `missingTool` is still unparenthesised; the
  Terraform checker still reports `STYRMAN_TOOL gcc`; `checkFile` still takes no
  `AbortSignal`; the close-file dialog's "Discard" still discards nothing.

## Criteria

1. **Dead code** — pass. `FORMATTABLE`/`extOf` are unexported now; the modal file viewer
   and its 382 lines are gone from `FileExplorer.tsx` with no dangling references; the
   hand-rolled `indexOf` is deleted. Two residues, both low: the `contains` alias in
   `check_test.go` and the stale hadolint comment. `LastOpened` remains written-and-unread
   but is now explicitly justified in `store.go:257-260`, which is what the earlier finding
   asked for.
2. **Flow tracing** — pass. Traced `focusToken` → `onResizeRef` → the connection effect
   (ref assignment vs. effect ordering vs. cleanup: no stale-closure or disposed-terminal
   window), `setDirty` → store `set()` → `useSyncExternalStore` (no-op bail intact, no
   render loop), and the updater's `deploy` → `reports_version` → rollback → latch path
   with the state files relocated (the latch now survives `git clean -d`; `STATE_DIR` is
   created before the lock; malformed state is regex-rejected before reaching git argv).
   The one flow that breaks is the terminal-switch path through `EditorPane`, above.
3. **Standards conformance** — pass. `gofmt`/`vet`/`go test`/`tsc -b`/`shellcheck -S error`
   all clean on my own re-run, which matters because the last `check.go` edit postdates the
   verification log. Go error paths in the changed code wrap or handle; the shell is
   `set -euo pipefail`-safe (the guarded `. /etc/styrman/update.env` does not trip errexit —
   tested).
4. **Directory layout** — pass. Updater under `deploy/update/`, state at
   `/var/lib/styrman/update/` (out of the clone, matching the "disposable checkout" model),
   Go tests beside their code, frontend entirely under `web/`.
5. **Simplicity / DIY-except-security** — pass. No new dependencies; the version check is a
   `docker inspect` label read rather than an HTTP+auth path; hadolint pinned with a
   sha256. Nothing security-sensitive is hand-rolled — tag trust is delegated to
   `git verify-tag`, host-key trust to OpenSSH with a pinned `known_hosts`, and the script
   fails closed on both. Only nit: the `contains` wrapper.
6. **Decoupling** — pass, with one contract to keep visible. The pane store is a clean seam
   (the explorer and the editor pane never touch each other; `notifyFileSaved` /
   `closeFileInPane` are the only couplings), and `onResizeRef` keeps the resize path inside
   `TerminalPane` rather than exposing the fit addon. The `deploy/Dockerfile.server` LABEL ↔
   `styrman-update` `reports_version()` contract is cross-file and load-bearing, but it is
   now recorded in ADR-050's correction, `deploy.md`, and comments in both files — that is
   as visible as a shell-level contract gets.
