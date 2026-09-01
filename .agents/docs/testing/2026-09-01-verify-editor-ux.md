# Verification — recent-files highlighting / recents strip / terminal-header filename badge (2026-09-01)

## Status: FAIL

Two independent, blocking findings:

1. **A real e2e regression**, caused directly by this diff, not a flake:
   the new filename badge inside the terminal header's "Editor" toggle
   button (`web/src/routes/p.$project.$term.tsx`) changes that button's
   accessible name from `"Editor"` to `"Editor <filename>"` whenever a file
   is open in the pane. `web/e2e/*.mjs` locates that button with
   `page.getByRole("button", { name: "Editor", exact: true })` in two
   places. The first of those (the "toggling back reopens the same file"
   check) now runs with a file already open (from the preceding "clicking a
   file opens it in the editor pane" check), so the exact-name match fails
   and the click times out. Because that check never reaches editor view,
   the very next check ("word wrap toggle is present", which needs the
   `Wrap` button that only renders in editor mode) also fails as a direct
   cascade. The suite's second occurrence of the same locator (inside the
   "deleting an open file clears the editor pane" check) still passes, only
   because by that point the pane is empty ("No file open") so the button's
   name is bare `"Editor"` again — which independently confirms the
   diagnosis rather than contradicting it.
2. **Zero test coverage, at any tier, of the new recents-file logic and
   UI** — see "Coverage of changed code" below.

## Commands run

- `npx tsc -b` (in `web/`) → clean, exit 0, no output.
- `npx oxlint src` (in `web/`) → clean, exit 0, no output.
- Dist freshness: newest `web/src` file (`p.$project.$term.tsx`,
  mtime `1788292613`) is older than the newest `web/dist` output
  (`_shell.html`, mtime `1788292621`) → dist is fresh.
- `grep -rl "styrman.recentFiles\." web/dist/client/assets/` →
  `p._project._term-DdrHDXMa.js` matches; spot-checked the minified
  content directly: ``V=8,H=e=>`styrman.recentFiles.${e}`,U=new Map,W=[]``
  — confirms the rebuilt bundle carries this change.
- `go build ./...` (repo root) → clean, exit 0 (embeds the fresh dist).
- `go test ./... -race` (repo root) → all packages `ok` (server code is
  untouched by this diff; ran anyway per the standing local-tier protocol —
  clean, as expected).
- `bash web/e2e/run.sh styrman-runtime:test` (isolated port
  `STYRMAN_E2E_PORT=18391`, own state dir, full self-hosted build+server+
  Playwright run) → **passed=46 failed=2 skipped=2** (expected, per the
  task and the app's own README for this firewall-hardened host: **48
  passed / 0 failed / 2 skipped**). The 2 skips are the known firewall-only
  auto-skips (`port link serves the app…`, `private link serves the app…`)
  — unaffected. The 2 failures:
  - `FAIL toggling back reopens the same file (no reload)` —
    `locator.click: Timeout 15000ms exceeded` on
    `getByRole("button", { name: "Editor", exact: true })`.
  - `FAIL word wrap toggle is present` — same timeout, on the `Wrap`
    button, as a direct cascade of the failure above (test never reached
    editor view).
  - The final `no uncaught client errors` gate still **passed** (3
    recoverable hydration warnings, called out by the suite itself as
    known-benign per ADR-045) — so this is not a console-error regression,
    it is a locator/behavior regression.
  - `integration.sh` was **not run** — the task correctly identifies this
    as a frontend-only change set with no Go/server changes, and this is
    confirmed by `git diff --stat` (no files under `internal/`,
    `cmd/`, or `deploy/` in this diff — those paths shown dirty in the
    session's `git status` belong to prior, already-shipped work, not this
    change set).

## Root cause (read from the diff, not fixed)

`web/src/routes/p.$project.$term.tsx`, inside the `Editor`/`Terminal`
toggle button's JSX:

```tsx
{
  label;
}
{
  mode === "editor" && pane.file && (
    <span className="ml-1 inline-block max-w-28 truncate align-middle font-mono text-[10px] opacity-75">
      {pane.file.path.includes("/")
        ? pane.file.path.slice(pane.file.path.lastIndexOf("/") + 1)
        : pane.file.path}
    </span>
  );
}
```

The filename `<span>` has no `aria-hidden`, so it is included in the
button's computed accessible name whenever a file is open, turning
`"Editor"` into `"Editor <filename>"`. Any locator doing an exact-name
match on `"Editor"` — which is exactly what the pre-existing e2e suite
does, twice — breaks the moment a file is open. This is a real,
user-visible-to-tooling regression in the reviewed diff, not a test
artifact; it needs `aria-hidden="true"` on the filename span (or an
equivalent fix) plus a re-run of the suite before this can ship. Per my
role I am not fixing it — this is the working session's job.

## Coverage of changed code

- `web/src/routes/p.$project.$term.tsx` (filename badge in the Editor
  toggle) — exercised (accidentally, adversarially) by the pre-existing
  e2e suite, which is how the regression above was caught. Once fixed, the
  same two checks re-passing is the only coverage this gets; there is no
  check that positively asserts the badge shows the right filename or
  truncates correctly.
- `web/src/lib/editorPane.ts` — `noteRecent`, `loadRecents`,
  `storeRecents`, `useRecentFiles`, and the new recents-pruning branch
  added to `closeFileInPane` — **NOT COVERED by any test at any tier.**
  `web/e2e/*.mjs` has zero references to "recent" (`grep -in recent
web/e2e/*.mjs` → no matches). No unit-test framework exists for `web/`
  in this repo (pre-existing, known gap — see
  `.agents/docs/testing/2026-09-01-verify-editor-pane-and-ordering.md`).
  This is exactly the kind of logic (localStorage persistence, dedup, cap,
  prefix-based pruning on delete/rename, try/catch degrade-on-blocked-
  storage) that silently regresses without a test — and this same diff
  independently demonstrates, via the Editor-button regression above, that
  "looks right by reading the code" is not a reliable substitute here.
- `web/src/components/FileExplorer.tsx` — the "Recent" chip strip
  (rendering, click-to-reopen, ~/ prefix for home scope, currently-open
  chip highlight) and the open-file row/name highlighting in the entry
  list — **NOT COVERED by any test at any tier.** No e2e check clicks a
  Recent chip, asserts the chip strip's presence/absence, or asserts the
  highlighted-row styling.
- `web/src/components/CodeEditor.tsx` / basename matching / word-wrap /
  Dockerfile highlighting — unaffected by this diff, still covered by
  their existing e2e checks (word-wrap check currently failing only as a
  cascade, per above, not because that check's own logic broke).

## Coverage judgement (frontend unit-test calculus)

This repo has zero frontend unit tests as a known, accepted 🟢 TODO, and
prior verifications (see the 2026-09-01
`verify-editor-pane-and-ordering.md` log) accepted e2e-only coverage for
comparable pane-state logic. That precedent does **not** extend to this
diff, for two reasons specific to it:

1. The recents feature (`editorPane.ts` additions + the "Recent" chip
   strip + highlighting) has **no e2e coverage at all** — not even
   indirect/incidental coverage the way `closeFileInPane`'s delete-pruning
   got covered by "deleting an open file clears the editor pane" for the
   _pane_-clearing half. The _recents_-pruning half added in the same
   function (the `startsWith(path + "/")` filter against the recents list)
   is invoked by nothing in the suite.
2. This diff already produced one real, non-obvious regression
   (the accessible-name break) in a part of the change that "reads fine."
   That is direct evidence, from this exact diff, that a code read is not
   sufficient assurance here, which weighs against waiving coverage for
   the other new, equally untested surface (the recents store) on a
   "plausible by reading it" basis.

Verdict on this axis alone (independent of the e2e failure): **FAIL** —
changed code with no covering test, per the constitution's covering-tests
rule. Recommend, non-blocking once the badge bug is fixed: at minimum an
e2e check that opens two files in sequence, asserts both appear as Recent
chips in the right order (newest first), clicking a chip reopens that
file, and that deleting the currently-open file's Recent chip disappears
too (exercises the new pruning branch, not just the pre-existing pane-
clearing one).

## Environment

- Tier: local only, Linux (Debian trixie host), plain Docker (server
  29.7.2). `docker run --runtime mvm --rm debian:trixie uname -r` → `6.18.28`
  (Kata `mvm` confirmed working).
- Used `styrman-runtime:test` (pre-existing local tag) — did **not**
  build or touch `styrman-runtime:dev`, per the repo's own testing README
  warning and ADR-052.
- `web/dist/client` is dirty after the e2e run (fresh content-hashed
  asset names) — expected build output, not a stray edit; matches the
  pattern already visible in `git status` before this run.
- No production containers, `/var/lib/styrman`, or `/etc/styrman` were
  touched. The e2e run's isolated state dir
  (`/tmp/styrman-e2e-ka5c`) and its own project/container(s) were cleaned
  up by the harness's own `cleanup()` trap (confirmed by the script's
  `[exited with code 0]`).

## Blocking findings (must be fixed and re-verified before this ships)

1. `web/src/routes/p.$project.$term.tsx` — the filename badge inside the
   Editor toggle button breaks its accessible name, causing 2 real e2e
   failures (`toggling back reopens the same file (no reload)`,
   `word wrap toggle is present`, the latter a cascade of the former). Fix
   (e.g. `aria-hidden="true"` on the filename span) and re-run
   `bash web/e2e/run.sh` to confirm a clean 48/0/2 before shipping.
2. `web/src/lib/editorPane.ts` (recents store: `loadRecents`,
   `storeRecents`, `noteRecent`, `useRecentFiles`, and the recents-pruning
   addition to `closeFileInPane`) and `web/src/components/FileExplorer.tsx`
   (Recent chip strip, open-file row/name highlighting) — no covering test
   at any tier. Needs at least e2e coverage before this can be called
   verified, per the constitution's covering-tests rule.
