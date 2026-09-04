---
owner: verifier
date: 2026-09-04
---
# Verification — release v1.22.0 (2026-09-04)

Range reviewed: v1.21.0..acb5918

## Status: PASS (commands) — coverage review INCOMPLETE, see below

Scope: ran every Tier-1 command AGENTS.md/README name (Go build/vet/test/race,
vendored ingress module, gofmt, frontend check/lint/test) directly against
this tree; did not run the browser e2e suite or integration.sh myself
(explicitly out of scope for this pass, results already obtained today —
see below); traced test coverage for the Go changes and the four new
frontend unit-test files in full, and for a representative slice of the
UI-only files via the e2e transcript, but did NOT individually trace every
changed `.tsx`/`.ts` file to a specific covering test or e2e check (listed
under "What I did not check"). Scope is therefore not complete per the
verifier mandate — reported honestly rather than claimed done.

## Commands

- `go build ./...` → clean, no output, exit 0.
- `go vet ./...` → clean, no output, exit 0.
- `go test ./... -race` → all packages `ok`:
  `cmd/styrman`, `internal/config`, `internal/conversation`,
  `internal/credbroker`, `internal/credstore`, `internal/files`,
  `internal/gitrepo`, `internal/history`, `internal/httpapi`,
  `internal/identity`, `internal/links`, `internal/orchestrator`,
  `internal/project`, `internal/terminal`, `internal/userstore`
  (`cmd/wsprobe`, `web` report "no test files", expected). No failures.
- `(cd deploy/caddy/ingress && go test ./...)` → `ok github.com/spgo/devbox/caddy/ingress` (cached, separate module, confirmed reached).
- `gofmt -l internal/ cmd/` → empty output (exit 0) — no unformatted files.
- `(cd web && npm run check)` → `tsc -b`, clean, no diagnostics.
- `(cd web && npm run lint)` → `oxlint`, clean, no findings.
- `(cd web && npm test)` → `vitest run`:
  **Test Files 7 passed (7)**, **Tests 69 passed (69)**, duration 571ms.

## Frontend test count vs README claim

Observed: **69 tests across 7 files** — exact match to the README's claim
("69 tests across seven files"). Verified by both the vitest summary line
and an independent per-file `it(`/`test(` count:

| file | observed | README-claimed |
|---|---|---|
| api.test.ts | 8 | (pre-existing) |
| api.session.test.ts | 6 | (pre-existing, BUG-001) |
| editorPane.test.ts | 3 | (pre-existing) |
| appearance.test.ts | 17 | 17 (v1.22.0, new) |
| diffReveal.test.ts | 8 | 8 (v1.22.0, new) |
| explorerHistory.test.ts | 13 | 13 (v1.22.0, new) |
| recentsPanel.test.ts | 14 | 14 (v1.22.0, new) |
| **total** | **69** | **69** |

`diffReveal.test.ts` reports as a binary file to `file`/`git diff --stat`
(it deliberately contains bidi-override/control-character fixtures for the
Trojan-Source reveal it tests) — confirmed benign by reading it with
`grep -a`, not a corrupt file.

## Go test-file count before/after (BUG-005 check)

Compared the full `*_test.go` file list at `v1.21.0` vs `HEAD` (excluding
the separate `deploy/caddy/ingress` module): **48 → 49 files**. The only
diff is the addition of `internal/httpapi/git_verbs_test.go`
(new this release, confirmed absent at v1.21.0 via
`git show v1.21.0:internal/httpapi/git_verbs_test.go` → fatal: does not
exist). No existing test file was removed or replaced. `git_verbs_test.go`
contains 4 `Test...` functions (`TestValidCommitHash`,
`TestValidBranchName`, `TestValidBranchNameDefersRefFormatToGit`,
`TestStashOpsIsAClosedSet`) — these cover the pure validation helpers
(`validCommitHash`, `validBranchName`, the stash-op allow-list) added to
`internal/httpapi/git.go`. The six new HTTP handlers themselves
(`gitFetch`, `gitPull`, `gitBranches`, `gitCheckout`, `gitLog`, `gitStash`)
have **no Go unit test** — they are exercised only by the e2e suite's new
"Git verbs: branches, log, stash (v1.22.0)" and "Git diff in the MAIN pane"
sections (confirmed present in `web/e2e/run.mjs`: branch dropdown/create/
switch/invalid-name checks, fetch/pull reachability, stash push -u / pop,
diff-in-main-pane checks). This matches the project's stated pattern
(pure logic gets a Go/vitest unit test, HTTP/UI flows get e2e) rather than
being an unreported gap, but note it explicitly since it is coverage by
e2e, not by `go test`.

## Already-obtained results (NOT re-run this pass)

Per instruction, I did **not** run these myself today; citing results
already obtained against this exact tree earlier today:

- `bash web/e2e/run.sh` — passed=81 failed=0 skipped=2 (82 checks total).
- `bash .agents/docs/testing/styrman/integration.sh` — passed=65 failed=0
  skipped=3.

I have no first-hand output from either run to paste; the counts above are
as reported to me, not something I observed directly.

## What I did not check

Traced to a specific test (unit or, via reading `web/e2e/run.mjs`, e2e):

- `internal/httpapi/git.go` validation helpers + `git_verbs_test.go` (Go unit).
- `internal/httpapi/git.go` HTTP handlers (fetch/pull/branches/checkout/log/stash) — e2e "Git verbs" section.
- `web/src/components/DiffView.tsx` (`reveal`) — `diffReveal.test.ts`.
- `web/src/components/DiffPane.tsx` — e2e "git diff opens in the MAIN pane" / "clicking a SECOND file swaps the diff".
- `web/src/lib/hooks.ts` (appearance store) and `web/src/lib/fonts.ts` (re-exported `FONT_STACKS`) — `appearance.test.ts`; also e2e "Appearance: fonts" section.
- `web/src/lib/explorerHistory.ts` — `explorerHistory.test.ts`; also e2e "explorer back/forward/up" and breadcrumb checks.
- `web/src/lib/recentsPanel.ts` — `recentsPanel.test.ts`; also e2e recents-grid checks.

**Not individually traced to a covering test or e2e check this pass** (ran
out of turns before completing the mapping — this is a real gap in this
verification, not a claim that coverage is missing):

- `web/src/components/CodeEditor.tsx` (73 lines changed)
- `web/src/components/FileEditor.tsx` (5 lines changed)
- `web/src/components/FileExplorer.tsx` (221 lines changed — beyond the nav/breadcrumb/recents e2e checks already confirmed, the rest of this diff is untraced)
- `web/src/components/SidePanel.tsx` (2 lines changed)
- `web/src/components/SidePanelGit.tsx` (351 lines changed — the largest single UI diff in the release; only the branch/fetch/pull/stash/diff sub-flows were confirmed via e2e grep, not the full diff)
- `web/src/components/TerminalPane.tsx` (33 lines changed)
- `web/src/components/icons.tsx` (13 lines added)
- `web/src/components/ui.tsx` (4 lines changed)
- `web/src/index.css` (9 lines changed — styling only, likely no logic to cover)
- `web/src/lib/api.ts` (104 lines changed — partially exercised by `api.test.ts`/`api.session.test.ts` but not diffed line-by-line against those tests)
- `web/src/lib/terminal.ts` (21 lines changed — `DEFAULT_TERM_FONT_FAMILY`/`DEFAULT_TERM_FONT_SIZE` confirmed read by `appearance.test.ts`, rest untraced)
- `web/src/routes/p.$project.$term.tsx` (28 lines changed)
- `web/src/routes/settings.tsx` (96 lines changed — font controls confirmed present in e2e, rest untraced)
- `internal/httpapi/server.go` (8 lines changed — new routes wiring, presumably reached transitively by `git_verbs_test.go`/e2e but not confirmed directly)
- `deploy/verify-commit.sh` (16 lines added), `image/skel/bash_exports`, `image/skel/bashrc` (2 lines each), `.agents/docs/testing/styrman/integration.sh` (17 lines added) — none traced to a test this pass.

Because these are untraced rather than confirmed-uncovered, I am not
marking them as FAIL rows; per AGENTS.md a changed file with **no**
covering test is a FAIL, but I do not yet have evidence either way for
this list — reporting the gap rather than guessing.

## Environment

- OS: Linux 6.12.107+deb13-amd64.
- Go: go1.25.0 linux/amd64.
- Node: v20.19.2, npm 9.2.0.
- Repo clean at HEAD `acb591823a495e2de3f32b8803a56aef22dc583f` (`git status --short` empty at start).
