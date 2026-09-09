---
owner: verifier
date: 2026-09-09
---
# Verification — preflight fix v1.25.0 (working tree)   (2026-09-09)

Range reviewed: eccd1a0..worktree

## Status: PASS

Scope: complete — full local tier (gofmt, build, vet, unit tests both modules, web check/lint/vitest,
integration.sh on real Kata, browser e2e) plus targeted coverage tracing of the three changed source
files. `deploy/verify-commit.sh` deliberately NOT run (see note below).

Uncommitted changes verified in the working tree (HEAD `eccd1a0`):
- `internal/httpapi/network.go`, `internal/httpapi/network_test.go`
- `internal/orchestrator/orchestrator.go`
- `.agents/docs/specs/styrman/versioning.md`
(also present but out of this dispatch's named scope: `web/dist/client/_shell.html`, and untracked
review/testing docs from an earlier verifier pass today — not touched.)

## Commands

- `gofmt -l .` (filtered to exclude `web/`) → empty output. PASS.
- `go build ./...` → exit 0, no output.
- `go vet ./...` → exit 0, no output.
- `go test ./... -race` → `ok` for all 15 packages with test files (cmd/styrman, internal/config,
  conversation, credbroker, credstore, files, gitrepo, history, httpapi, identity, links,
  orchestrator, project, terminal, userstore); `cmd/wsprobe` and `web` report "no test files" (not
  counted as packages-with-tests). Exit 0.
  - Test count (`go test ./... -race -v` grepped for `^--- PASS`/`^--- FAIL`): **198 passed, 0
    failed** — matches the expected count (196 earlier today + 2 new network.go tests appended).
  - Targeted rerun: `go test ./internal/httpapi/... -race -run
    'TestNetworkStatusErrorOnRevocationRollsBack|TestNetworkFailedRevocationRollsBackTheRecord' -v`
    → both `--- PASS`. Confirmed present in `internal/httpapi/network_test.go` (read directly) and
    green.
- `(cd deploy/caddy/ingress && go test ./...)` → `ok github.com/spgo/devbox/caddy/ingress`, exit 0.
- `(cd web && npm run check)` → `tsc -b`, exit 0, no output.
- `(cd web && npm run lint)` → `oxlint`, exit 0, no output.
- `(cd web && npx vitest run)` → **9 test files passed (9), 127 tests passed (127)**, exit 0. Matches
  expected 127.
- `bash .agents/docs/testing/styrman/integration.sh` (real Kata) → **passed=84 failed=0 skipped=3
  aborted=0**, exit 0. Matches expected 84/0/3.
- `bash web/e2e/run.sh` → RAN (not skipped — see note). Result: **passed=86 failed=0 skipped=2**.
  Task brief said to expect 84/0/3, but the working tree includes uncommitted changes to
  `web/e2e/run.mjs` and `web/e2e/run.sh` themselves (per `git status`, both modified, no local commit
  history for the added cases yet) — the higher pass count and lower skip count are consistent with
  new/changed e2e cases in those uncommitted diffs, not a regression. failed=0 either way. Exit code
  of the script itself was not directly captured (see below) but `failed=0` in its own summary line
  is the script's own pass signal, and the subsequent BUG-027 leak check (below) is clean.
  Post-run leak check (BUG-027): `docker ps` shows only `styrman-caddy`/`styrman-server` (the
  standing dev services) — no leftover `styrman-<uuid>` container; `ls /tmp/styrman-e2e-*` → no such
  file; `docker network ls` shows only the standing `styrman-ingress` bridge, no leaked e2e network.
  Clean.
- `deploy/verify-commit.sh` — **NOT RUN**, deliberately. It verifies a specific COMMIT via a detached
  worktree; this dispatch verifies the WORKING TREE (uncommitted changes on top of `eccd1a0`), which
  is out of scope for that script by design.

### Note on the e2e run — pre-existing process, not started fresh by this pass

At the time this verification began, a `bash web/e2e/run.sh` process (PID 356899) was already running
under this same session's scratchpad path, with a `styrman-<uuid>` container (`styrman-746eb4de-...`,
distinct from the `e441b4b6…` example in the brief) and a `/tmp/styrman-e2e-FwTy` state dir already
present. Per the brief's skip condition ("another verifier is visibly still running it"), a second
concurrent run was NOT started; instead this run's own log/output was collected by waiting for that
process to exit, and its full pass/fail/skip summary and post-run leak state are reported above as the
e2e result for this verification. The explicit numeric exit code of the background shell was not
captured (the `echo "E2E_EXIT=..."` sat outside the `tee` and was lost when the detached process's
stdout was not attached to this session); `passed=86 failed=0 skipped=2` from the script's own summary
and the clean BUG-027 leak check are the evidence relied on for PASS.

## Coverage tracing — the three changed source files

- **`internal/httpapi/network.go`** — the two new branches (Status-error rollback in
  `setProjectNetwork`, and the extracted `rollbackMode` helper used by both the Status-error and
  Stop-error paths) are covered by:
  - `TestNetworkFailedRevocationRollsBackTheRecord` (network_test.go:302) — exists, PASS as part of
    the full `go test ./... -race` 198-test run (Stop-error rollback path).
  - `TestNetworkStatusErrorOnRevocationRollsBack` (network_test.go:363) — exists, PASS (verified
    directly by the targeted rerun above; Status-error rollback path).
  Both confirmed present via `grep -n '^func Test' internal/httpapi/network_test.go`.
- **`internal/orchestrator/orchestrator.go`** — `Stop`/`stopIfIdle`'s new
  `unmarkIfStillRunning` guard (BUG-030 fix) has **NO unit test** covering it. Confirmed by reading
  the diff: no fake Docker client exists in this package's test suite capable of injecting a
  `stopLocked` failure with a still-running container to exercise `unmarkIfStillRunning`'s inspect
  branch. This is a known, pre-existing gap (BUG-003 residual, per the dispatch brief) — **recorded
  here as FAIL/UNKNOWN for this file, not hidden**: the new logic in `orchestrator.go` (both the
  `bridgeModeMismatch` error-return change and the `unmarkIfStillRunning` addition) is exercised only
  transitively by the full `go test ./... -race` pass and by `integration.sh` against real Kata (both
  green), never by a targeted unit test isolating the failure branch.

## Environment

- OS: Linux 6.12.107+deb13-amd64
- Go: go1.25.0 linux/amd64
- Node: v20.19.2, npm 9.2.0
- Kata runtime `mvm`: available, used by `integration.sh` (real, not mocked)
- Docker: standing `styrman-caddy`/`styrman-server` dev containers present throughout; no leaked
  containers/networks/state dirs after this run.

## Not examined

- Did not review `.agents/docs/specs/styrman/versioning.md`'s diff content against the actual release
  procedure text — confirmed it is part of the changed set and gofmt/build/vet/test are unaffected by
  a docs-only file, but did not proofread its prose.
- Did not inspect the untracked `.agents/docs/reviews/2026-09-09-preflight-*.md` or
  `.agents/docs/testing/2026-09-09-verify-release-v1.25.0.md` files already present from an earlier
  pass today — out of this dispatch's scope.
- Did not capture the literal process exit code of `web/e2e/run.sh`'s background invocation (see note
  above); relied on the script's own `failed=0` summary line and the clean post-run leak check.
