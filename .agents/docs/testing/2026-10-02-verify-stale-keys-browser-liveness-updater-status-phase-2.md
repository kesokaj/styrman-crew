---
owner: verifier
date: 2026-10-02
---
# Verification — stale-keys-browser-liveness-updater-status-phase-2   (2026-10-02)

Range reviewed: 837b3b9..worktree

## Status: PASS

Scope: every dispatched command ran once; guide.md exists (not re-read); accepted gaps remain (below), so not `complete`.

Previous run: FAIL at 837b3b9+dirty; Tier 4 `no uncaught client errors` saw one 500 from GET /api/projects/<id>/ports (passed=98 failed=1 skipped=2); diagnosed since as BUG-088 and fixed (Orchestrator.GuestGoingAway). The first run's log is kept verbatim at the end of this file.

## Commands (run 2)
- 1. `go build ./...` -> rc=0
- 1. `go vet ./...` -> rc=0
- 1. `go test ./... -count=1` -> rc=0; 16 packages ok, 2 "[no test files]" (cmd/wsprobe, web), none failed
- 1. `(cd deploy/caddy/ingress && go test ./... -count=1)` -> rc=0
- 2. `go test ./internal/config/ -run 'TestStyrmanUpdate|TestStyrmanCtl' -count=1 -v` -> rc=0; counts: pass 31, fail 0, skip 0 at 837b3b9+dirty; vs previous: first run of this log, pass 31->31; all 10 TestStyrmanUpdateStatus* PASS
- 2. `go test ./internal/orchestrator/ ./internal/httpapi/ -run 'TestListeningPortsFailure|TestBrowserStatusDuringALifecycleChange' -count=1 -v` -> rc=0; 3 PASS (TestListeningPortsFailureDuringALifecycleChangeIsNoPorts, TestListeningPortsFailureWithNothingInFlightIsAnError, TestBrowserStatusDuringALifecycleChangeIsNotRunning)
- 3. `npm run check` rc=0; `npm run lint` rc=0; `npm test` rc=0, 18 files, 210 tests
- 4. `bash -n deploy/update/styrman-update` rc=0; `bash -n deploy/styrman-ctl` rc=0
- 5. `bash .agents/docs/testing/styrman/integration.sh styrman-runtime:test` -> rc=0; `passed=122 failed=0 skipped=3 aborted=0`
- 6. pgrep for both suites printed nothing before Tier 2 and before Tier 4 (one PID printed in the Tier 2 call was the verifier's own command line matching the pattern; rechecked in a separate call: nothing). `bash web/e2e/run.sh` -> rc=0; `passed=99 failed=0 skipped=2` (as expected; `no uncaught client errors` passed). `git status --short web/dist` before and after identical (31 lines): the rebuild changed nothing.
- 7. Suppression scan: no added `nolint`, `.skip(`, `.only(`, `@ts-ignore`, `oxlint-disable`, `eslint-disable` in the diff. Untracked test files: only `t.Skip` at internal/config/styrman_update_status_test.go:34 ("no bash on this host") and :410 ("runs the root refusal; meaningless as root"), both with a reason on the line.
- 8. status before/after (excluding web/dist): only new path seen is `.agents/docs/deploy/gcp.md` (docs, another agent's artifact, listed as seen); no source/test/config path moved.

## Coverage of changed code
- internal/orchestrator/ports.go -> internal/orchestrator/ports_lifecycle_test.go (2 PASS)
- internal/httpapi/browser.go, server.go (guestRuntime) -> internal/httpapi/browser_goingaway_test.go (1 PASS); fake in guestexec_test.go
- deploy/update/styrman-update, .service, deploy/docker-compose.yml -> internal/config/styrman_update_status_test.go (10 PASS)
- deploy/styrman-ctl -> internal/config/styrman_ctl_test.go
- internal/updatestatus/ -> own tests (package ok)
- internal/config/config.go, internal/httpapi/explorer.go -> internal/httpapi/settings_updater_test.go + Tier 2 settings checks
- web/src/lib/updaterNotice.ts -> web/src/lib/updaterNotice.test.ts
- web/src/routes/settings.tsx, web/src/lib/api.ts -> Tier 4 (passed)
- Accepted gaps: real root updater tick on a host; status file on a real host after a release; fresh-host path unexercised.

## Not examined
BUG-088 fix not mutation-tested (no must-fail control run); a single Tier 4 pass cannot prove the /ports race is gone, only that it did not reproduce; guide.md not re-read; Go/Node versions not recorded; hook-test count is 10 (dispatch confirmed 10).

## Environment
Linux 6.12 (Debian 13), Kata runtime mvm, local Docker; image styrman-runtime:test.


# Previous run (verbatim)


Range reviewed: 837b3b9..worktree

## Status: FAIL

Scope: every dispatched command ran once (Tier 2 and Tier 4 included); guide.md exists, not re-read; Tier 4 failed 1 check, so not complete

## Commands
- 1. `go build ./...` → exit 0
- 1. `go vet ./...` → exit 0
- 1. `go test ./... -count=1` → exit 0 (16 packages ok, none failed)
- 1. `(cd deploy/caddy/ingress && go test ./... -count=1)` → exit 0
- 2. `go test ./internal/config/ -run 'TestStyrmanUpdate|TestStyrmanCtl' -count=1 -v` → exit 0; counts: pass 31, fail 0, skip 0 at 837b3b9+dirty; vs previous: none. FINDING: only 10 TestStyrmanUpdateStatus* top-level tests exist and PASS (HooksAreGated, WriteWritesTheSixValidatedFields, WriteRefusesValuesOutsideItsSets, WriteCarriesSinceAcrossAFailingStreak, WriteDirectoryTrust, ExitDecision, Wiring, PathIsPinnedInTheUnitOnly, PathMatchesTheComposeMount, EarlyRefusalWritesNothing); the dispatch expected 11 (plan line 383 says "10 of the 11 new tests" - the 11th may be a test outside this prefix, not identified).
- 3. `(cd web && npm run check)` → exit 0; `npm run lint` → exit 0; `npm test` → exit 0, 18 files, 210 tests passed
- 4. `bash -n deploy/update/styrman-update` → 0; `bash -n deploy/styrman-ctl` → 0
- 5. `docker compose -f deploy/docker-compose.yml --env-file .agents/scratch/verify-p2/env config` → exit 0; STYRMAN_UPDATE_STATUS_FILE: /etc/styrman/status/updater.json present; bind /etc/styrman/status -> /etc/styrman/status read_only: true present
- 6. `bash .agents/docs/testing/styrman/integration.sh styrman-runtime:test` → exit 0; `passed=122 failed=0 skipped=3 aborted=0`; the three "settings: " updater checks PASS (no status file no key; failing status reaches admin code and all; unreadable status is "unknown").
- 7. `bash web/e2e/run.sh` → exit 1; `passed=98 failed=1 skipped=2` (expected 99/0/2). "settings tells an admin when the host's auto-updater is failing" PASS. Failing check: "no uncaught client errors — saw 1: Failed to load resource: 500 [http://127.0.0.1:18090/api/projects/a92d32e0-ce49-48ba-8900-609de7fc3733/ports]". web/dist: `git status --short web/dist` identical before and after (rebuild changed nothing). Both suites had no concurrent run (pgrep empty before each).

## Root cause
Tier 4 check "no uncaught client errors" saw a single 500 from GET /api/projects/<id>/ports during the run. Cause NOT established (no re-run, server log not kept by run.sh). Evidence: no changed file touches ports (internal/orchestrator/ports.go is unchanged; explorer.go change is settings/updater related); BUG-023 (docs/bugs) records the same symptom as a race when a project container vanishes/rebuilds during the poll, so a flake or a regression of that race is plausible but unproven. pre-existing? unknown: the failure is outside the change set's files. Status stays FAIL; a re-dispatch with a second Tier 4 run would show whether it reproduces.

## Other findings
- Only 10 `TestStyrmanUpdateStatus*` tests exist (all PASS), not 11 as the dispatch expected; plan line 383 says "10 of the 11 new tests", so the 11th is unidentified (may be a test outside the prefix). Plan/dispatch count mismatch, not a failure.
- No suppressions/skips diffed (not scanned exhaustively; see Not examined).

## Coverage of changed code
- deploy/update/styrman-update, styrman-update.service, deploy/docker-compose.yml — covered by internal/config/styrman_update_status_test.go (10 tests PASS, incl. Wiring, PathIsPinnedInTheUnitOnly, PathMatchesTheComposeMount) + compose config check
- deploy/styrman-ctl — internal/config/styrman_ctl_test.go (PASS)
- internal/updatestatus/ — own test dir present, package ok in go test
- internal/config/config.go, internal/httpapi/explorer.go — internal/httpapi/settings_updater_test.go + Tier 2 settings checks (PASS)
- web/src/lib/updaterNotice.ts — web/src/lib/updaterNotice.test.ts presence not confirmed by name (ls output); vitest 18 files / 210 pass
- web/src/routes/settings.tsx, web/src/lib/api.ts — Tier 4 check PASS
- Accepted gaps (plan): real root updater tick on a host, status file on a real host after a release, fresh-host path.

## Not examined
Added-suppression scan (`//nolint`, `.skip(` etc.) of the diff; guide.md re-read; counts vs previous log for go test; a second Tier 4 run; the cause of the /ports 500; go test per-test pass counts for the full suite (only package ok lines).

## Environment
Linux 6.12 (Debian 13), Kata runtime mvm, local Docker; image styrman-runtime:test. Go/Node versions not recorded.
