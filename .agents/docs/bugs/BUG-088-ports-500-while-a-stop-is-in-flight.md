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
The first fix then over-corrected (see Fix): it read "the lock is held" as
"the guest is going away". The lock is also held when nothing is going away,
by `EnsureRunning` on an already-running project (every terminal attach) and
by the idle reaper's busy check, so a real scan or launcher failure in that
window would have been hidden as "no ports" or "not running" (code review of
v1.40.0..5571b57, `reviews/2026-10-02-review-v1.40.0-5571b57-code.md`,
finding 1, medium).
`Diagnosis missed:` the first fix's reading of the lock ("held means the guest
is going away") was not checked against every holder of that lock; grepping
`lockProject(` and `WithProject(` callers would have shown `EnsureRunning`'s
no-op path.

## Fix
First fix (commit 5571b57): new `Orchestrator.GuestGoingAway(ctx, user,
project, ran Status) bool` in `internal/orchestrator/ports.go` answered true
when the project was no longer that running container (BUG-023) OR when
`TryLock` failed. Wrong, per the review above.

Revised fix (working tree after 5571b57, before the release): after a failed
exec, `GuestGoingAway` first asks whether the project is still the same
running container (BUG-023). If it is, it WAITS for the project's lock to come
free, re-checking every 100 ms and bounded by the request's context, then asks
again. A stop or mode toggle then reads stopped or replaced (no ports / not
running); a no-op holder leaves the same container running, so the failure is
reported; a request that gives up first gets the failure. `ListeningPorts`
and `browserExec` (`internal/httpapi/browser.go`) call it; the `guestRuntime`
interface and the fake in `internal/httpapi/guestexec_test.go` carry the
method.

Tests, `internal/orchestrator/ports_lifecycle_test.go` (five, replacing
`TestListeningPortsFailureDuringALifecycleChangeIsNoPorts`):
`TestListeningPortsFailureDuringAStopIsNoPorts`,
`...DuringAModeToggleIsNoPorts`, `...UnderANoOpLockHolderIsAnError` (the
review's case), `...WithNothingInFlightIsAnError`,
`TestListeningPortsWaitIsBoundedByTheRequest`. With the first fix's "lock held
means gone" logic put back, four of the five fail; all pass with `-race`.
`internal/httpapi/browser_goingaway_test.go`
`TestBrowserStatusDuringALifecycleChangeIsNotRunning` covers the browser site.

Third step (release gate): the code review
(`reviews/2026-10-02-review-5571b57-57179a4-code.md`, finding 1, low) found
that the revised wait checked only the lock. A mode toggle holds the lock
through stop, recreate and `waitReady` (up to 90 s), longer than the page waits
for a poll, so the poll could still end in the 500. Now `GuestGoingAway` asks
Docker on every 100 ms tick while it waits. It answers "going away" as soon as
the container is down or replaced, and only asks once more after the lock comes
free if it is still the same running container.
New test `TestListeningPortsAnswersAsSoonAsTheGuestIsGone`: the lock is held
for the whole test, the container stops, and the scan must answer. It failed
against the previous loop with "the scan never reached GuestGoingAway's wait"
(the old loop never asked again). The tests now wait on the fake Docker
client's recorded calls, not a fixed sleep. `ports_lifecycle_test.go` has six
tests.

Fourth step (release gate, round 3): the security review
(`reviews/2026-10-02-security-57179a4-624808d.md`, finding 1, low) found that a
parked request whose client never gives up asked Docker about 10 times a second
for the whole of a lock hold (up to 90 s in a toggle's `waitReady`), with
nothing bounding it server-side. `GuestGoingAway` now has its own ceiling,
`lifecycleWaitMax` = 20 s (the page gives up on a poll after 20 s anyway),
whatever the request's deadline. Past it the failure is reported as it stands.
New test `TestListeningPortsWaitHasACeiling`: the lock is held for good and the
client never gives up, and the scan must answer with the failure. Before the fix
it never answered. `ports_lifecycle_test.go` now has seven tests. The operator
chose to fix this before the release ("Fix all five, then ship").

Fifth step (fixed 2026-10-03): shipped as known in v1.41.0
(`reviews/2026-10-02-preflight-v1.41.0.md`, item 2). The 20 s ceiling bounded
each parked request, but ten parked requests on one project asked Docker ten
times as often. Operator: "Do it fix them". `GuestGoingAway` still makes its
two quick checks itself. If it must wait, every request parked on the same
project and container shares ONE wait, `awaitLifecycle`, keyed in
`Orchestrator.lifecycleWaits`. That wait runs on its own context, capped at
`lifecycleWaitMax` (20 s), so one client going away does not end the wait for
the others. Each request still returns when its own ctx ends.
New test `TestListeningPortsParkedRequestsShareOneWait`: 10 parked requests
asked Docker 50 times in 5 ticks before the fix; the test allows at most 8.
`ports_lifecycle_test.go` now has eight tests, all passing three times under
`-race`.

## Prevention
A held lock says that something is in flight, not WHAT it is. Wait for it and
ask again (is it still the same running container?), and list every holder of
a lock before giving its being held a meaning. Ask `GuestGoingAway`, never
re-derive the check at a call site.

## Related
BUG-023 (same family; its guard covered a vanished container, not one
mid-stop), BUG-021 (the toggle that makes the race routine). Both BUG-023 and
this one were found only by capturing the live failure text.
