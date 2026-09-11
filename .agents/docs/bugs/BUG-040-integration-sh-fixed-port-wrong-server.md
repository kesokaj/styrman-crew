---
owner: bugs
date: 2026-09-11
id: BUG-040
status: open
area: .agents/docs/testing/styrman/integration.sh (operator workflow)
---
# BUG-040: integration.sh hard-codes port 18080; a run against a local server on that port silently tested the wrong server

## Symptom
On 2026-09-11 a Browser-view hand-walk ran a local server on
127.0.0.1:18080 (a scratch state dir) while `integration.sh` was started
in the background. The suite's own server could not bind, but its
readiness probe (`curl /api/health`) was answered by the OTHER server, so
all 90 checks ran against the hand-walk's server — creating an `itest`
project in the wrong state dir, interleaving with the walk's stop/start of
its own project, and producing a "green" run (passed=90 failed=0
skipped=3) that verified nothing about the suite's own server.

## Repro
Start a local server bound to 127.0.0.1:18080, then run
`.agents/docs/testing/styrman/integration.sh` in the background — its
readiness probe passes against the other server and the suite proceeds
against it.

## Root cause
`integration.sh` line 21 hard-codes `PORT=18080` with no bind check before
starting its own server, and its readiness probe only checks "does
something answer on the port", not "is this MY server". Contrast:
`web/e2e/run.sh` takes `STYRMAN_E2E_PORT` (default 18090).

## Fix
Same day (2026-09-11, the browser-view plan's Phase 4 commit):
`integration.sh` reads `STYRMAN_ITEST_PORT` (default 18080) and REFUSES to
start when that port is already bound (`ss -ltn` before its own server
starts), naming the override in the message; `testing/styrman/README.md`
says so. The PID-confirming readiness probe from the prevention rule is
NOT done — with the bind check in front of it, the window is the race
between the check and the bind, accepted. The invalid run was stopped and
rerun cleanly once the hand-walk's server was noticed in the log
(passed=90 failed=0 skipped=3 against the correct server).

## Prevention
A self-hosting test suite must (1) fail if its port is already bound
BEFORE starting its own server (`ss -ltn` check), and (2) make the port
overridable, matching `web/e2e/run.sh`'s `STYRMAN_E2E_PORT` pattern. A
readiness probe must identify the server it started (PID alive AND health
answers), never just "something answers on the port".

## Related
BUG-034 ("silence from your own instrumentation" — same class: a green
run against the wrong target is worse than a red one).
