---
owner: bugs
date: 2026-10-02
id: BUG-088
status: fixed
area: internal/orchestrator/ports.go, internal/httpapi/browser.go
---
# BUG-088: /ports 500'd "port scan exited 9" while a stop was in flight (gap left by BUG-023's fix)

## Symptom
The Phase 2 verifier's browser e2e run (2026-10-02,
`testing/2026-10-02-verify-stale-keys-browser-liveness-updater-status-phase-2.md`)
printed `passed=98 failed=1 skipped=2`. The failing check was "no uncaught
client errors", which saw one `500` from `GET /api/projects/<id>/ports`. Four
earlier full e2e runs the same day had not shown it (an intermittent race).
run.sh deletes the server's state dir on exit, so the message was in no log.

## Repro
Scratch script (not committed): a loopback server (`STYRMAN_AUTH` default
none, scratch state under `/var/tmp`), one project, a tight loop of
`GET /api/projects/<id>/ports` while the network mode is toggled four times
(`PUT /api/projects/<id>/network {"lanAccess":...}`, which rebuilds the
container, ADR-067), then a stop, a start and a delete. Before the fix: three
`500 {"error":{"message":"port scan exited 9"}}` during the toggles. After:
none (only the expected 404 "project not found" after the delete). One run
each way.

## Root cause
`ListeningPorts` re-checks after a failed exec whether the project is still
the running container it scanned (BUG-023's guard). While a stop is under way
(a mode toggle stops, removes and recreates the container) Docker still
reports the SAME container as running until its PID 1 exits, and the exec the
shutdown killed exits non-zero (9 observed on Kata; BUG-023 saw 128). So the
guard read "same container, still running" and returned the failure as a 500.
The scan script ends in `|| true` and never exits non-zero on its own. The
same guard was copied into `browserExec` (`internal/httpapi/browser.go`) with
the same gap.
`Diagnosis missed:` none; no separate diagnosis was dispatched, the repro
gave the exact message and location.

## Fix
New `Orchestrator.GuestGoingAway(ctx, user, project, ran Status) bool` in
`internal/orchestrator/ports.go`: true when the project is no longer that
running container (BUG-023), OR when the project's lock is held right now
(`TryLock` fails), since every lifecycle change (stop, mode toggle via
`WithProject`, boot via `EnsureRunning`) holds that lock for its whole
duration. `ListeningPorts` returns an empty list and `browserExec` returns
"not running". The `guestRuntime` interface (`internal/httpapi/server.go`)
gained the method; the fake in `internal/httpapi/guestexec_test.go` gained it
with a `goingAway` field. Working tree of the 2026-10-02 round (plan
`plans/2026-10-02-stale-keys-browser-liveness-updater-status.md`), before the
release.

Tests: `internal/orchestrator/ports_lifecycle_test.go`
`TestListeningPortsFailureDuringALifecycleChangeIsNoPorts` (lock held, exec
exit 9, container still running -> no ports, no error; failed before the fix
with "port scan exited 9") and
`TestListeningPortsFailureWithNothingInFlightIsAnError` (no lock -> failure
still reported, lock not left held). `internal/httpapi/browser_goingaway_test.go`
`TestBrowserStatusDuringALifecycleChangeIsNotRunning`; with the browser
site's guard reverted it fails with a 500 "browser status is not available
right now".

## Prevention
A guest exec that fails while the project's lock is held belongs to the
lifecycle change, not the command: "is it still the same running container?"
is not enough, because a container being stopped is still reported running;
ask `GuestGoingAway`, never re-derive the check at a call site.

## Related
BUG-023 (same family; its guard covered a vanished container, not one
mid-stop), BUG-021 (the toggle that makes the race routine). Both BUG-023 and
this one were found only by capturing the live failure text.
