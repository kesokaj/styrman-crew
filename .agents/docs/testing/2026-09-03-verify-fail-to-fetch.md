---
owner: verifier
date: 2026-09-03
---
# Verification — fail-to-fetch session-expiry fix (2026-09-03)

Range reviewed: working tree (uncommitted) — `web/src/lib/api.ts` (modified), `web/src/lib/api.session.test.ts` (new), `web/dist/client/**` (rebuilt)

## Status: PASS

Scope: complete for the items listed in the dispatch — web check/lint/test, Go build/vet/test, ingress module test, dist freshness + string check. `web/e2e/run.sh` and `.agents/docs/testing/styrman/integration.sh` intentionally not run (out of scope, microVM-booting).

## Commands

- `cd web && npm run check` → PASS. `tsc -b` completed, no output (no errors).
- `npm run lint` → PASS. `oxlint` completed, no output (no findings).
- `npm test` (vitest run, then rerun with `--reporter=verbose` to confirm per-file/test names) → PASS.
  ```
  Test Files  3 passed (3)
       Tests  12 passed (12)
  ```
  Verbose listing confirms the new file ran:
  ```
  ✓ src/lib/api.session.test.ts > request() when the ingress session has expired
    > re-authenticates once instead of surfacing 'Failed to fetch' until F5   23ms
  ```
  Test count: 12 tests across 3 files (1 new test in the new file `api.session.test.ts`; `api.test.ts` and `editorPane.test.ts` unchanged, pre-existing).
- `go build ./...` (repo root) → PASS, no output. Confirms the Go build succeeds embedding the rebuilt `web/dist/client` via `go:embed`.
- `go vet ./...` → PASS, no output.
- `go test ./...` → PASS. All 12 testable packages `ok` (cmd/styrman, internal/config, conversation, credbroker, credstore, files, gitrepo, history, httpapi, identity, links, orchestrator, project, terminal, userstore); `cmd/wsprobe` and `web` report "no test files" (expected — web/ is embedded assets, its own tests run under vitest above).
- `(cd deploy/caddy/ingress && go test ./...)` → PASS. `ok  github.com/spgo/devbox/caddy/ingress (cached)`. Module unchanged by this diff; run as the gate requires.

## Dist freshness check

- `git status --short -- web/dist` shows changes confined to `web/dist/client` and `web/dist/client/assets` only (modified `_shell.html`, deleted old content-hashed JS files, new content-hashed JS files under `assets/`) — consistent with a rebuild that reassigns asset hashes. No changes outside `web/dist/client`.
- `grep -l "opaqueredirect" web/dist/client/assets/*.js` → exactly one match: `web/dist/client/assets/index-TFG2hSpN.js`. The fix landed in the built bundle exactly once, no duplication.

## Coverage of changed code

Changed lines in `request()` (web/src/lib/api.ts):
- `signal: init?.signal ?? AbortSignal.timeout(20000),` — pre-existing, unchanged.
- `redirect: "manual",` (new) — exercised: the test's fetch stub (`browserFacingAnExpiredSession`) branches on `init?.redirect === "manual"`; since the assertion path requires the mock to return the `opaqueredirect` response rather than throwing `TypeError("Failed to fetch")`, the test would fail if `redirect: "manual"` were not passed through to `fetch`. Covered.
- `if (res.status === 401 || res.type === "opaqueredirect")` (condition extended with `res.type === "opaqueredirect"`) — exercised: the stub returns `type: "opaqueredirect", status: 0`, so only the new disjunct trips the branch (status 0 would not satisfy `=== 401`). The test asserts `reAuthOnce()` fired (toast text + `reload` called once after advancing timers) and that `api.listProjects()` rejects with `/session expired/i`. Covered.
- `throw new ApiError("session expired — reloading to sign in", res.status || 401)` — exercised via the `rejects.toThrow(/session expired/i)` assertion. Covered.

No changed line in `request()` is uncovered.

## Environment

- OS: Linux 6.12.107+deb13-amd64
- Node/npm: vitest v4.1.11 (from test output); `go version` not separately queried but build/vet/test all succeeded against the repo's pinned toolchain.
- Docker/Colima: not applicable — no container or e2e tests run in this pass (out of scope per dispatch).
