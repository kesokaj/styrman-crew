---
owner: bugs
date: 2026-10-05
id: BUG-091
status: open
area: internal/httpapi/git.go, web/e2e/run.mjs (git pane)
---
# BUG-091: intermittent e2e failure of the git-pane checks (HTTP 409 on git commit / stash)

## Symptom
In some browser e2e runs the git-pane checks of `web/e2e/run.mjs` fail together:
- "fast commit stages everything and commits a UUID" (waitForSelector "Committed as ..." timed out, 30 s)
- "branch dropdown lists branches and creates one"
- "switching to an existing branch works"
- "an invalid branch name is refused before any round trip"
- "history lists commits and opens one in the main pane"
- "stash push -u captures untracked work, pop restores it"
- "no uncaught client errors": HTTP 409 Conflict on `POST /api/projects/<id>/git/commit` and `/git/stash`.

Occurrence (2026-10-05):
- At 2ba3f79 (v1.42.1 candidate): 2 of 4 e2e runs failed this way (`.agents/scratch/e2e-forced-fail-2.log`, `.agents/scratch/verify-v1.42.1-tiers/e2e.txt`); 2 passed (e2e-v1421-check, e2e-diag, both 101/0/2).
- At v1.42.0 (b1eabeb and earlier): 5 of 5 runs passed the git checks. One other run at b1eabeb failed only "terminal session survived the editor round-trip" (15 s timeout, under host load) and passed on rerun.

## Repro
Intermittent; no deterministic repro. `bash web/e2e/run.sh` at 2ba3f79 failed 2 of 4 times.

## Root cause
Unknown (status open). The 409 comes from `internal/httpapi/git.go`: `git add -A` or `git commit` failed in the guest (writeErr 409 carrying guest git output, around lines 399 and 423). The guest's message was never captured: `check()` in run.mjs prints only the first line of the Playwright error. One instrumented run (printing role=alert/status text on failure) passed, so nothing was captured.
The source diff b1eabeb..2ba3f79 touches no git path (internal/orchestrator setup-VM reaper and pre-ADR-027 remove, deploy/docker-compose.yml).
Hypotheses, unverified:
- timing against the background harness install after boot (ADR-078);
- host load from parallel jobs (the verifier's failing run overlapped a security review that ran git against a 12.9 MB config in .agents/scratch).

## Fix
Not fixed.

## Prevention
A check that fails on a server error records the response body or the UI's error text, never only the selector timeout.
Next step for whoever picks it up: keep the alert/status instrumentation in `check()` on failure (or log the 409 body server-side) so the next occurrence names git's error.

## Related
BUG-090 (found the same day). `.agents/rules/e2e-checks-leave-the-suite-as-they-found-it.md` (a failure that reads as many defects).
