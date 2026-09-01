# Verification — editor-ux round 2 (recent-files strip / open-file highlight / header basename badge) (2026-09-01)

## Status: FAIL

Round 1's two deterministic e2e breaks are confirmed fixed — the suite is a clean 48/0/2 with
the client-error gate green. But round 1's other blocking finding, **zero test coverage of the
new recents-file logic and UI**, was never addressed in this round's fix set and is still true
today. Per the constitution's covering-tests rule (changed code with no covering test = FAIL,
no exceptions), that alone keeps this FAIL regardless of the green suite below.

## Commands run

1. `cd web && npx tsc -b` → clean, exit 0, no output.
   `npx oxlint src` → clean, exit 0, no output.
2. `node --check web/e2e/run.mjs` → exit 0 (valid syntax).
   `grep -n "fileList()" web/e2e/run.mjs` → 9 matches, all 9 file-click locators from round 1's
   diagnosis now go through `fileList()` (`subdir`, `nested.txt`, `Dockerfile` ×2, `one.txt`,
   `two.txt`, `broken.sh`, `fine.sh`, `lint.py`). Remaining bare `panel().getByText(...)` calls
   are tab/section clicks (`Files`, `Home`, `Project`, `Ports`, `History`, `Git`, `Clipboard`,
   `conv-1`), not file-click locators — correctly left unscoped.
   `grep -rl "recent-files" web/dist/client/assets/` and `grep -rl "file-list" ...` and
   `grep -rl "styrman.recentFiles" ...` → all three resolve to the same built asset,
   `web/dist/client/assets/p._project._term-D0pTeHwC.js` — the `data-testid`s and the recents
   store both made it into the shipped bundle.
3. Dist freshness: newest `web/src` file (`p.$project.$term.tsx` / `editorPane.ts` /
   `FileExplorer.tsx`, mtime `1788293413`) is older than the newest `web/dist` output
   (`_shell.html`, mtime `1788293455`) → dist is fresh, not stale.
   `go build ./...` (repo root) → clean, exit 0 (embeds the fresh dist).
4. `STYRMAN_E2E_PORT=18391 bash web/e2e/run.sh styrman-runtime:test` (isolated port, own state
   dir under `/tmp/styrman-e2e-9xb3`, self-hosted build+server+Playwright run against
   `styrman-runtime:test` — never `:dev`, per ADR-052/the app's testing README) →
   **passed=48 failed=0 skipped=2**, exactly the expected result for this firewall-hardened
   host. Both of round 1's breaks are gone:
   - `PASS toggling back reopens the same file (no reload)`
   - `PASS word wrap toggle is present`
   The 2 skips are the known firewall-only auto-skips (port link / private link serving from
   inside the microVM) — unaffected, unrelated to this diff.
   `no uncaught client errors` gate → **PASS** (3 recoverable hydration warnings, called out by
   the suite itself as known-benign per ADR-045).
5. `go test -race -count=1 ./...` (repo root) → all 14 testable packages `ok` (`cmd/wsprobe`,
   `internal/identity`, `web` report "no test files", as before — none of them are in this
   diff's scope). `go vet ./...` → clean, exit 0 (run in addition to the dispatched list, per
   the standing local-tier protocol).

## Coverage of changed code

- `web/src/routes/p.$project.$term.tsx` (Editor/Terminal toggle `aria-label`, filename badge
  moved out of the accessible name, `hidden ... sm:inline-block`) — covered: the e2e suite's
  "toggling back reopens the same file" and "word wrap toggle is present" checks now pass with
  a file open, which is a direct regression test for the accessible-name bug this round fixed.
- `web/e2e/run.mjs` (`fileList()` helper, 9 call sites) — self-covering; this file *is* the
  test.
- `web/src/lib/editorPane.ts` — `openFileInPane`'s dirty-flag preservation is exercised
  indirectly by "discard-and-open loads the second file" (PASS), but that check does not assert
  the dirty dot survives the pending/parked state the review finding was actually about — it
  only asserts the eventual load succeeds. `noteRecent`, `loadRecents`, `storeRecents`,
  `useRecentFiles`, and the recents-pruning branch added to `closeFileInPane` remain
  **NOT COVERED by any test at any tier** — unchanged from round 1.
  `grep -in recent web/e2e/*.mjs` → only a comment (line 65, explaining why `fileList()` exists);
  no check clicks a Recent chip, asserts the strip's presence, or exercises the pruning path.
  No frontend unit-test framework exists in this repo (confirmed again:
  `web/package.json` scripts are `dev`/`build`/`lint`/`preview`/`check` only, no `test` script,
  no `vitest`/`jest` dependency; `find web -iname "*.test.*" -o -iname "*.spec.*"` outside
  `node_modules`/`dist` returns nothing).
- `web/src/components/FileExplorer.tsx` — the `data-testid="recent-files"` / `data-testid=
  "file-list"` hooks exist and are now used by `web/e2e/run.mjs` (item 2 above), but only to
  *scope away from* the Recent strip, not to test it. The Recent chip strip itself (rendering,
  click-to-reopen, `~/` prefix for home scope, `aria-current` on the open chip) and the
  open-file row/name highlighting (`isOpen`, `aria-current`, `font-medium`/`bg-accent-soft`)
  remain **NOT COVERED by any test at any tier** — unchanged from round 1.

## Coverage judgement

Round 1 recorded two independent blocking findings. This round's diff fixes finding 1 in full
(verified above — clean 48/0/2, both specific checks now pass) but does not touch finding 2:
the recents store (`editorPane.ts`) and the Recent chip strip / highlighting
(`FileExplorer.tsx`) still have no covering test anywhere — not a unit test (none exist for
`web/` in this repo, a known accepted gap) and not even incidental e2e coverage (unlike
`closeFileInPane`'s pre-existing pane-clearing half, which the "deleting an open file clears the
editor pane" check does exercise). Per the constitution ("Changed code without covering tests =
FAIL... never write or edit tests or source: when coverage is missing, the answer is FAIL"),
this is still a FAIL on the coverage axis alone, independent of the now-green e2e suite.

## Environment

- Tier: local only, Linux (Debian trixie host), plain Docker (server present; Kata `mvm`
  runtime available per prior verifications on this host — not re-checked this round since no
  container-runtime-affecting code changed).
- Used `styrman-runtime:test` (pre-existing local tag, confirmed via `docker images` alongside
  `:1.16.0`/`:1.16.1`/`:1.17.0`/`:dev`/`:latest`) — did **not** build or touch
  `styrman-runtime:dev`, per ADR-052 and the app's testing README warning.
- `web/dist/client` is dirty after the e2e run (fresh content-hashed asset names,
  `web/dist/client/assets/p._project._term-D0pTeHwC.js` replacing round 1's
  `...-DdrHDXMa.js`) — expected build output per the README, not a stray edit.
- No production containers, `/var/lib/styrman`, or `/etc/styrman` were touched. The e2e run's
  isolated state dir (`/tmp/styrman-e2e-9xb3`) and its own project/container(s) were cleaned up
  by the harness's own `cleanup()` trap.

## Blocking findings (must be fixed and re-verified before this ships)

1. `web/src/lib/editorPane.ts` (recents store: `loadRecents`, `storeRecents`, `noteRecent`,
   `useRecentFiles`, the recents-pruning addition to `closeFileInPane`) and
   `web/src/components/FileExplorer.tsx` (Recent chip strip render/click-to-reopen, open-file
   row/name highlighting) — still no covering test at any tier. This is the exact same finding
   round 1 filed as blocking finding #2; it was not addressed by this round's fix set. Needs at
   minimum an e2e check that opens two files in sequence, asserts both appear as Recent chips
   in the right order (newest first, using the now-available `data-testid="recent-files"`
   hook), clicking a chip reopens that file, and that deleting the currently-open file's Recent
   chip disappears too (exercises the new pruning branch). A second check should assert the
   open-file row/chip carries `aria-current="true"` and the highlight class, to catch a
   regression in `isOpen`.
