---
owner: verifier
date: 2026-09-03
---
# Verification — image-harness-replacement Phase 3 (server)   (2026-09-03)

Range reviewed: working tree at HEAD c1a6eb9, scoped to `internal/httpapi/setup.go`,
`internal/httpapi/setup_roster_test.go`, `web/src/routes/settings.tsx`,
`web/dist/client`. Working-tree changes under `image/` (Phases 1-2) explicitly
out of scope, not run or reviewed.

## Status: PASS

Scope: complete for Phase 3's own list (1-5 below), plus coverage mapping
of every changed hunk in setup.go to the two new tests. `integration.sh`
and `web/e2e/run.sh` intentionally NOT run per instruction.

## Commands

1. `gofmt -l internal/ cmd/` → no output (clean).
   `go vet ./...` → no output (clean).
   `go test ./... -race -count=1` → all packages `ok` (cmd/styrman, internal/config,
   conversation, credbroker, credstore, files, gitrepo, history, httpapi, identity,
   links, orchestrator, project, terminal, userstore).
   `go test ./internal/httpapi/ -run Roster -v -count=1`:
   - `TestRosterReadsTheNewestSeededProjectHome` — PASS
   - `TestRosterIsEmptyWithoutASeededProject` — PASS

2. Bite test: `git stash push -- internal/httpapi/setup.go` (stashed cleanly),
   then `go test ./internal/httpapi/ -run Roster -count=1 -v` on the OLD code:
   - `TestRosterReadsTheNewestSeededProjectHome` — FAIL (`claude agents = [base-only], want [reviewer verifier]`; `claude skills = [], want [2911]`; `agy agents = []`; `agy skills = []`)
   - `TestRosterIsEmptyWithoutASeededProject` — FAIL (`roster should be empty ... got {..Agents:[base-only]..}`)
   Both tests bite. `git stash pop` restored the file; `git diff --stat -- internal/httpapi/setup.go`
   after pop shows `1 file changed, 37 insertions(+), 13 deletions(-)` — the change is back.

3. `cd web && npm run check` → `tsc -b`, no output, exit 0.
   `npm run lint` → `oxlint`, no output, exit 0.
   `npm test` → vitest: `Test Files 3 passed (3)`, `Tests 17 passed (17)`.

4. `git status --short -- web/dist` confined entirely to paths under
   `web/dist/client/` (checked with an awk filter — no output = confined, confirmed).
   `_shell.html` is single-line/no-newline (`file` reports "data"); `grep -oa` extracted
   its 4 referenced JS files, all present on disk:
   `assets/index-CUxGQN3J.js`, `assets/jsx-runtime-DE3RlOCf.js`,
   `assets/rolldown-runtime-hePW80VL.js`, `assets/routes-CBKNi6Ra.js`
   (plus `assets/index-CMzUvFDp.css`, present). The string `subagents` appears in
   exactly one asset file: `web/dist/client/assets/settings-BQc7bH0A.js`
   (`grep -l subagents assets/*.js` → single match).

5. `go build ./...` → exit 0, no output.

## Root cause (FAIL only)

N/A — PASS.

## Coverage of changed code

Diff hunks in `internal/httpapi/setup.go`, each mapped to the two Roster tests:

- `listNames` empty-home early return (`if home == "" { return nil }`) —
  exercised by `TestRosterIsEmptyWithoutASeededProject` (no seeded project →
  `rosterHome` returns `""` → `listNames("", ...)` → nil, asserted).
- `listNames` `.bak.` filter (was hidden-only before) — exercised: seeded
  `.claude/agents/testing.md.bak.20260901-065226` and
  `.gemini/config/agents/old.md.bak.20260901-065226`; both excluded from the
  asserted `[reviewer verifier]` want-list.
- `listNames` hidden-file filter (`.hidden.md`) — seeded and excluded from the
  same want-list; covered by the same assertion (not isolated as its own case,
  but exercised).
- `rosterHome` newest-seeded-project selection — exercised: `old` and `mid`
  both carry `.config/sh-agent/state.json`, `mid` is newer and its skills
  (`mid[:4]`) are what's asserted, explicitly rejecting `old`'s.
- `toolDetails` reading Skills/Agents/Commands from `rh` (roster home) instead
  of `h` (base home) for both claude and agy — exercised: base home seeded
  with `.claude/agents/base-only.md`, asserted absent from the result in both
  tests.
- agy `Agents` from `.gemini/config/agents` (directory names, not
  `plugins/harness/agents`) — exercised: `.gemini/config/agents/{reviewer,verifier}/agent.md`
  seeded, asserted as `agy.Agents`.

All five named hunks (`.bak.` filter, hidden-file filter, empty-home early
return, newest-seeded selection, agy agents path) are exercised.

`web/src/routes/settings.tsx` (`rosterOf` helper, "N skills · M subagents"
string) has no unit test — acceptable: it is a display-only string
composition over data already covered server-side by the Roster tests and by
`npm run check`/`lint` for type/syntax correctness; behavioral verification
(the string actually rendering correctly in the browser) is deferred to the
Playwright e2e in Phase 5 per the plan, not a gap in this phase.

## Environment

Linux 6.12.107+deb13-amd64, repo at c1a6eb9a5a9ecd248cd13e10ebb9e649e05ed1b1
(main, clean before this run). Go and Node versions not queried this run;
tooling invoked exactly as the project's `AGENTS.md` documents
(`gofmt`, `go vet`, `go test`, `npm run check`/`lint`/`test`, `go build`).
No Docker/Colima steps were part of this scope.
