---
owner: verifier
date: 2026-09-10
---
# Verification — todo-closeout Phase 4 (terminal re-authorization)   (2026-09-10)

Range reviewed: working tree vs HEAD (0378825) — uncommitted changes for Phase 4
(item I), coexisting with uncommitted Phases 2/3 (verified separately, see
`2026-09-10-verify-closeout-phase2.md`).

Files in scope: `internal/terminal/bridge.go`, `internal/terminal/reauth_test.go`
(new), `internal/config/config.go`, `internal/httpapi/server.go`,
`web/src/lib/api.ts`, `web/src/components/TerminalPane.tsx`.

## Status: PASS

Scope: complete — all commands the dispatch named ran; Go unit/race suite,
full `go test ./...`, test-count check, frontend check/lint/vitest, and
`git diff --numstat` on the changed files.

## Commands
- `gofmt -l internal/` → no output (clean).
- `go vet ./...` → no output (clean).
- `go test ./internal/terminal/ ./internal/httpapi/ -count=1 -race` → both `ok`
  (terminal 1.089s, httpapi 1.672s).
- `go test ./... -count=1` → all 16 packages `ok` (2 report "no test files"),
  none failing.
- `grep -c '^func Test' internal/terminal/reauth_test.go` → `3`, matching
  `TestRevocationWatchFiresOnceWhenAuthorizationIsLost`,
  `TestRevocationWatchNeverFiresWhileAuthorized`,
  `TestRevocationWatchIgnoresANilCheck`.
- `(cd web && npm run check)` → `tsc -b`, no output/errors.
- `(cd web && npm run lint)` → `oxlint`, no output/errors.
- `(cd web && npx vitest run)` → `Test Files 11 passed (11)`, `Tests 147 passed
  (147)`.
- `git diff --numstat` on the six changed files → `internal/config/config.go`
  5/0, `internal/httpapi/server.go` 45/14, `internal/terminal/bridge.go`
  65/4, `web/src/components/TerminalPane.tsx` 10/0, `web/src/lib/api.ts`
  26/4; no `-` (binary-marker) in either column.
  `internal/terminal/reauth_test.go` is untracked (new file, per BUG-005
  guard — never overwrote an existing path); `file` reports it as
  "Unicode text, UTF-8 text" (not binary — BUG-007 guard).

## Root cause  (FAIL only)
N/A — all steps passed.

## Coverage of changed code
- `internal/terminal/bridge.go` `revocationWatch` — covered by the three
  new tests in `internal/terminal/reauth_test.go` (fires once on loss,
  never fires while authorized, tolerates a nil check).
- `internal/config/config.go` `CloseAccessRevoked = 4003` — indirectly
  exercised (constant referenced by the covered close path and by the
  frontend `CLOSE_REVOKED` constant); no dedicated test, acceptable for a
  named constant.
- `internal/httpapi/server.go` wiring (`stillAuthorized = users.IsAuthorized`
  only when `cfg.AuthMode == "header"`) — confirmed present at line 222-223;
  not exercised by a dedicated unit test in this pass — GAP (not called out
  by name in the dispatch's gap list, but no test asserts the AuthMode=none
  vs AuthMode=header branch of this wiring).
- **GAP (named by dispatch):** the watch goroutine inside `serve()`
  (`internal/terminal/bridge.go` ~line 371-383) and the 4003 close it
  triggers have no unit test — `serve()` requires a live container exec,
  which `reauth_test.go` does not set up. `revocationWatch` itself (the
  pure logic) is unit-tested; the goroutine's wiring into `serve` is not.
- **GAP (named by dispatch):** `web/src/components/TerminalPane.tsx`'s
  `revoked` state and its `CLOSE_REVOKED` handling (line 236) have no
  component-level unit test. `web/src/lib/api.ts`'s `CLOSE_REVOKED` export
  itself is a plain constant, not independently tested, but is consumed
  by the (untested) component logic.

## Environment
Linux 6.12.107+deb13-amd64. `go version` not captured explicitly but build/
test ran without toolchain errors. Node/npm per project `web/package.json`
(vitest v4.1.11 reported). No container/Colima steps required for this scope.
