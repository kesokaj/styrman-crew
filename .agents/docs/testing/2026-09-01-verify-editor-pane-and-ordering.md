# Verification — editor-pane, terminal ordering, review remediation, syntax-check backend (2026-09-01)

## Status: PASS (with two residual gaps noted, and a scope-creep flag)

This verification caught a real, serious, live bug mid-session — a syntax
checker that could silently report a clean result for genuinely broken code
— which was root-caused and fixed by the working session while I was
verifying it. The fix is confirmed correct by re-running every gate,
including two independent live-API reproductions and two clean e2e runs
after the fix. Final state: all Go/TS gates clean, all 45 e2e checks green,
integration suite 57/0/3. Two coverage gaps remain (not blocking, noted
below) and a large unrelated feature landed in the same working tree and
was **not** verified here (see "scope creep").

## Timeline (why this log is longer than usual)

The working tree changed materially, twice, during this verification —
apparently from another agent/session working the same repo concurrently.
I am recording the sequence because the final PASS depends on a fix that
happened mid-verification, not before it:

1. **Initial state**: the task described `internal/httpapi/check.go` +
   `check_test.go` as new, unwired backend code, alongside the
   editor-pane/ordering feature and its review remediation.
2. **First e2e run (clean, before drift)**: 43 passed / 0 failed / 2
   skipped. All review-remediation checks green.
3. **Mid-session drift #1**: `internal/httpapi/server.go` gained
   `POST /api/projects/{project}/check` → `s.checkFile` (now wired);
   `CodeEditor.tsx`/`api.ts`/`settings.tsx` grew a full CodeMirror
   lint-gutter integration calling it; `package.json` gained
   `@codemirror/lint`; two new e2e checks were added for it. Also, a large,
   **unrelated** feature appeared: `deploy/update/` (new `styrman-update`
   script + systemd unit/timer), `deploy/host-setup.sh`, `image/README.md`,
   `.agents/docs/specs/styrman/{guide,runtime-image}.md`, `AGENTS.md`, and
   `.claude/agent-memory/documentation/*`.
4. **Investigation**: with the check endpoint now live, I created a
   throwaway project via the API and called `POST .../check` directly on a
   shell script with an unmistakable syntax error (`if` with no `fi`).
   **It returned `{"diagnostics":[],"supported":true,"tool":"shellcheck"}`
   — a false "clean" result** — reproduced 3/3 times. I also reproduced the
   identical symptom on the pre-existing, unmodified `formatFile` endpoint
   (`POST .../format` on valid-but-unformatted Go returned `{"content":""}`).
   Root cause, confirmed by manually replicating the guest-side script over
   `docker exec -i`: the two handlers pipe file content over the exec
   stream's stdin then half-close (`CloseWrite()`) to signal EOF, but the
   hijacked exec stream is **one bidirectional connection** carrying both
   stdin and the multiplexed stdout/stderr — half-closing it tore the whole
   stream down, truncating everything the guest command printed after
   reading its input. Confirmed 2 clean isolated e2e runs (`STYRMAN_E2E_PORT`
   18199, 18299) reproduced the same e2e-level failure
   (`syntax check flags a broken shell script` FAILED once, intermittently
   passed once — consistent with a race, not a deterministic bug at that
   layer, though deterministic when hit directly via the API).
5. **Mid-session drift #2 (the fix)**: `internal/orchestrator/exec.go` was
   changed to stop half-closing the stdin write side; `format.go` and
   `check.go` were changed to use `head -c <exact-byte-count>` in the guest
   script instead of `cat` (which waited for EOF that would no longer come).
   New test `TestShellDialectFlag` also landed in `check_test.go`.
6. **Re-verification of the fix**: `go build`/`vet`/`test -race` all clean;
   two more clean, isolated e2e runs (`STYRMAN_E2E_PORT` 18399, 18499) both
   came back **45 passed / 0 failed / 2 skipped** — `syntax check flags a
   broken shell script` and `terminal session survived the editor
   round-trip` (the other check that had been flaky under the earlier,
   contention-heavy conditions) both passed cleanly, twice in a row.
   `integration.sh` re-run once more: 57/0/3.

I verified the tree in the state it settled into (per instructions: treat
on-disk changes as deliberate, don't revert) and re-ran every gate against
that final state rather than reporting on a stale snapshot.

## Commands run (final state)

- `go build ./...` → clean.
- `go vet ./...` → clean.
- `go test -race -count=1 ./...` → all packages `ok`, run three times across
  the session (before drift, after drift #1, after the fix) — clean every
  time.
- `gofmt -l internal/ cmd/` → no output.
- `(cd deploy/caddy/ingress && go test -count=1 ./...)` → `ok`.
- `go test ./internal/httpapi/ -run "Check|MissingTool" -v` → all subtests
  PASS (`TestParseCheckOutput` ×9, `TestParseCheckOutputClean`,
  `TestCheckerFor`, `TestShellDialectFlag`, `TestMissingTool`).
- `(cd web && npx tsc -b)` → clean.
- `(cd web && npx oxlint)` → exit 0, no output.
- `docker run --rm --entrypoint bash styrman-runtime:dev -c 'shellcheck --version; hadolint --version'`
  → `ShellCheck 0.10.0`, `Haskell Dockerfile Linter 2.15.1`. Image built
  2026-09-01T10:28, after the Dockerfile diff — confirms the build picked up
  the new `apt-get install shellcheck` + hadolint-binary lines.
- `bash .agents/docs/testing/styrman/integration.sh styrman-runtime:dev`,
  isolated, run twice (before and after the exec.go fix): **57 passed / 0
  failed / 3 skipped** both times.
- `bash web/e2e/run.sh` (via `run.mjs`), run **six times** total this
  session, with distinct `STYRMAN_E2E_PORT`s to avoid interference once I
  identified a collision:
  1. Default port, before drift: **43/0/2.**
  2. Default port, collided with a concurrent second `run.sh` invocation on
     the same port + shared `e2e-ui` project name: **25 passed / 18 failed /
     2 skipped**, all traced to `ERR_CONNECTION_REFUSED` once one run's
     cleanup deleted the other's project mid-flight (screenshot
     `/tmp/styrman-shots/99-failure.png` shows the project vanishing under a
     live terminal). **Discarded, not a real result.**
  3. `STYRMAN_E2E_PORT=18199`, isolated, before the exec.go fix: **43/2/2** —
     `terminal session survived the editor round-trip` and
     `syntax check flags a broken shell script` both failed.
  4. `STYRMAN_E2E_PORT=18299`, isolated, before the fix: **44/1/2** —
     `syntax check flags a broken shell script` passed this time (debug
     output showed the correct 4 shellcheck diagnostics — intermittent);
     `terminal session survived the editor round-trip` failed again.
  5. `STYRMAN_E2E_PORT=18399`, isolated, **after** the exec.go/format.go/
     check.go fix: **45/0/2.** Both previously-failing checks passed.
  6. `STYRMAN_E2E_PORT=18499`, isolated, repeat: **45/0/2** again.
- Direct API reproduction (own throwaway project `verify-check-dbg` /
  `c1de50b0-e8b9-4696-86df-eb5a6d663f82`, created and fully deleted via the
  API): confirmed the pre-fix bug 3/3 times on `/check` and once on
  `/format`; did not re-test post-fix via raw curl since the e2e suite's own
  direct-fetch debug assertion inside `syntax check flags a broken shell
  script` now exercises the identical code path and passed twice.

## Review remediation — verified against
`.agents/docs/reviews/2026-09-01-review-editor-pane-and-ordering.md`

All eight findings are genuinely fixed in the code (confirmed by reading
the diffs, not the changelog):

- **[high] delete/rename-while-open** — `web/src/lib/editorPane.ts`
  `closeFileInPane()` clears every terminal's pane in the project
  (prefix-matched for directories, same rule as the old `closeIfOpen`);
  `FileExplorer.tsx`'s `closeIfOpen` calls it; `EditorPane.tsx` has a second
  effect (`if (!wanted && loaded) setLoaded(null)`) that clears the
  actually-rendered `loaded` buffer, not just the store — matches the
  review's own diagnosis that clearing the store alone wasn't enough. E2e
  check "deleting an open file clears the editor pane" passed in every run.
- **[medium] `discardAndOpen` dead-end** — `loaded` is now in the load
  effect's dependency array (`EditorPane.tsx`); the `same`-file bail-out
  keeps it idempotent. **Still not covered by any test** (unit or e2e) —
  see coverage gaps below.
- **[medium] `createFile` opens the old modal** — now calls `openEntry(p)`
  (the pane), confirmed in `FileExplorer.tsx`.
- **[medium] `firstTerminalId` blast radius** — `web/src/lib/projects.ts`
  now explicitly picks `max(lastOpened)` across terminals rather than
  `terminals[0]`; `LastOpened` is a read field again.
- **[medium] `previewing`/`previewHtml` staleness** — both reset inside the
  existing render-phase block in `FileEditor.tsx`.
- **[low] `naturalLess` non-ASCII folding + tie-break stability** —
  `store.go` folds case via `unicode.ToLower` over runes; `List`'s
  comparator checks both directions before falling back to ID. New tests
  `TestNaturalLessUnicodeFolding`/`TestListStableForEquivalentTitles` cover
  exactly this.
- **[low] focus lost on Editor→Terminal** — `focusToken` prop on
  `TerminalPane`; `showTerminal()` bumps it; the e2e check's explicit
  `.focus()` call was removed. Passed cleanly in both post-fix isolated
  runs; the two failures seen pre-fix (runs 3–4 above) are attributed to
  host contention from a concurrent second session rather than a code
  defect in this mechanism — the relevant source (`TerminalPane.tsx`,
  `p.$project.$term.tsx`) was byte-identical across the failing and passing
  runs. See the caveat about what this check actually proves, below.
- **[low] ADR-047/048 footer drift** — `DECISIONS.md` has ADR-047's footer
  immediately after its own body, followed by a full ADR-048 with its own
  footer and a same-day correction fixing the route-remount rationale.
  Confirmed by locating all `## ADR-04[5-8]` / `Source:` / `Verification:`
  line positions.

Also confirmed: `store.go` `List()` sorts by natural title order (not
`LastOpened`); `CodeEditor.tsx` matches on `filename.split("/").pop()`; the
old modal `FileViewer` is fully deleted (`FileExplorer.tsx` down to ~400
lines); `FORMATTABLE`/`extOf` no longer exported; `AGENTS.md` no longer
hardcodes an e2e check count and defers to the README, which now correctly
says 45 checks.

## The check.go bug, and what it says about the tests

The task asked me to judge whether `check.go`'s unit-test coverage is
honest, and specifically whether a clean file really produces zero
diagnostics and a broken one is never silently passed.

- `parseCheckOutput`, `checkerFor`, `missingTool` are well tested against
  real captured tool output for all 8 tool paths, including
  clean-file-means-zero-diagnostics for every tool. Judged on their own,
  honest and adequate.
- **But none of them exercise the actual delivery mechanism** — the
  `checkFile` HTTP handler and the `orchestrator.Exec` stdin plumbing that
  gets the file's content into the guest. That is exactly where the bug
  was: a real, live, reproducible false-negative ("this broken file has no
  problems") in the one thing the feature promises never to do. The bug was
  real (confirmed independently via direct API calls against a throwaway
  project, not just an e2e flake) and it was **not caused by new code alone**
  — it lived in `internal/orchestrator/exec.go`, shared with the
  pre-existing, equally untested `formatFile`/"Format button" endpoint,
  which exhibited the identical symptom (empty output) and has apparently
  had zero test coverage, at any tier, since it shipped.
- The fix (`head -c <exact length>` instead of `cat` + no half-close) is
  correct: it matches the actual constraint of a hijacked Docker exec
  stream (one bidirectional connection, not independently closable halves),
  and I confirmed it working end-to-end twice via clean e2e runs plus the
  two new e2e checks' own direct-fetch debug assertions.
- **Residual gap**: even after the fix, there is still no *handler-level*
  Go test for `checkFile` (or `formatFile`) that would catch a regression
  of this exact kind without needing a live Docker/Kata stack. The only
  thing guarding this now is the e2e suite. Recommend a fake
  `Orchestrator`/exec double so `go test` alone can catch this class of bug
  next time.

## Test-coverage judgement

- **`discardAndOpen` fix**: not covered by any test (no unit-test framework
  exists for `web/`, consistent with this project's e2e-only frontend
  convention; no e2e check exercises the discard-and-open-a-different-file
  flow). Plausible by reading the code, unverified by any gate.
- **Rename-while-open**: not directly covered, but shares the same
  `closeFileInPane` call the tested delete path exercises — low risk.
- **`terminal session survived the editor round-trip`**: worth flagging
  even though it now passes reliably. Per `internal/terminal/bridge.go:
  354-355`, a browser-side disconnect does **not** kill the backing tmux
  session (`tmux new-session -A -D` reattaches and repaints), and
  `TerminalPane.tsx`'s `ws.onopen` unconditionally calls `term.focus()` on
  every connect. So a regressed implementation that fully unmounted and
  remounted `TerminalPane` on every view toggle would **also** pass this
  exact assertion — it does not by itself distinguish "kept mounted, hidden
  via CSS" from "torn down and reattached." What it correctly guards,
  specifically because the e2e's own explicit `.focus()` call was removed,
  is the no-op-reconnect case: if the pane stays mounted (so `ws.onopen`
  never refires) and `focusToken` weren't wired, focus would land on
  `<body>` and the typed keystrokes would go nowhere. That is real
  protection, just narrower than the check's name and the ADR/changelog
  claim.
- **`syntax check reports a CLEAN file as clean`**: honest — asserts the
  absence of `.cm-lint-marker-error` after a real debounced check cycle.
- **`syntax check flags a broken shell script`**: well designed (real
  content, real debounce wait, DOM assertion, plus a same-check direct-API
  debug fetch separating UI-wiring bugs from backend bugs) and it is the
  test that actually caught the live bug this session. Good test.
- These two checks are the first-ever coverage, at any tier, of the
  `Stdin`-over-`orchestrator.Exec` mechanism, which also backs the
  pre-existing `formatFile`. That gap is now closed for `check.go` by e2e
  only; `formatFile` still has none.

## Coverage of changed code

- `internal/conversation/store.go` — covered (`TestListNaturalOrder`,
  `TestNaturalLess`, `TestNaturalLessUnicodeFolding`,
  `TestListStableForEquivalentTitles`).
- `internal/httpapi/check.go` — pure helpers covered by unit tests; the
  `checkFile` handler + exec/stdin integration covered only by e2e (no Go
  handler-level test) — acceptable given the e2e coverage now exists and
  passes, but noted as a gap to close.
- `internal/orchestrator/exec.go` (the actual fix) — covered indirectly by
  `go test -race ./...` (no regression) and directly by the two e2e syntax
  -check assertions plus the pre-existing `integration.sh` suite (57/0/3,
  which exercises other `Exec` call sites without Stdin).
- `web/src/lib/editorPane.ts` (`closeFileInPane`, `notifyFileSaved`) —
  covered by the "deleting an open file clears the editor pane" e2e check.
- `web/src/components/EditorPane.tsx` (`discardAndOpen`) — **NOT COVERED**
  by any test.
- `web/src/components/TerminalPane.tsx` (`focusToken`) — covered by e2e,
  passing.
- `web/src/components/CodeEditor.tsx` (basename matching, wrap compartment,
  linter wiring) — covered by "Dockerfile highlights," "word wrap toggle,"
  and the two syntax-check e2e checks.
- `deploy/update/`, `deploy/host-setup.sh`, doc-only files — **out of
  scope, not evaluated** (see below).

## Scope creep — flagged, not verified

A large, unrelated feature landed in the same working tree during this
session: `deploy/update/` (new `styrman-update` script + systemd
`.service`/`.timer`), `deploy/host-setup.sh` changes, `image/README.md`,
`.agents/docs/specs/styrman/{guide,runtime-image}.md`, and
`.claude/agent-memory/documentation/*`. **None of this has anything to do
with editor-pane-and-ordering.** I did not review, test, or verify any of
it — it is unscoped for this verification request and should get its own
review/verification pass before it ships, ideally as a separate commit.

## Environment

- Tier: local only, Linux (Debian trixie host), plain Docker (server
  29.7.2, no Colima needed).
- `styrman-runtime:dev` built 2026-09-01T10:28 CEST, confirmed to contain
  `shellcheck 0.10.0` and `hadolint 2.15.1`.
- `web/dist/client` is dirty after every e2e run (fresh content-hashed
  assets) — expected build output per `run.sh`'s own header comment.
- The operator's live projects `test3321`
  (`styrman-928e709a-8e9c-4040-a6e6-b5dc2d66816b`) and `asdasd`
  (`styrman-36782cc5-…`) were never started, stopped, or exec'd into. My own
  throwaway project (`verify-check-dbg`,
  `c1de50b0-e8b9-4696-86df-eb5a6d663f82`) was created via the API and fully
  deleted (`?wipe=true`, container confirmed gone) before this log was
  written. A stray `e2e-ui` project and a `chk` project belonging to the
  concurrent session were left untouched (not mine to clean up; the harness
  self-cleans `e2e-ui` on its next run anyway).

## Recommended follow-ups (non-blocking)

1. Add a handler-level Go test for `checkFile` and `formatFile` (fake
   `Orchestrator`/exec double) so this exact class of bug is caught by
   `go test` next time, not just e2e.
2. Add e2e or unit coverage for `discardAndOpen`.
3. Get `deploy/update/`, `deploy/host-setup.sh`, and the related doc changes
   into their own reviewed, verified change rather than bundled with this
   diff's eventual commit.
