---
owner: bugs
date: 2026-09-09
id: BUG-023
status: fixed
area: internal/orchestrator/ports.go
---
# BUG-023: port scan 500'd with "exited 128" when a network-mode rebuild raced the poll

## Symptom
`GET /api/projects/<id>/ports` returned 500
`{"error":{"message":"port scan exited 128"}}` while a project was being
rebuilt, tripping the e2e client-error gate ("no uncaught client errors —
saw 1").

## Repro
Poll `/ports` in a loop at ~0.5s while PUTting a network-mode change on a
running project; roughly one poll per toggle returns 500. Timing-sensitive
— a slower poll cadence often misses it.

## Root cause
`ListeningPorts` (`internal/orchestrator/ports.go:64`) calls `o.Status`,
returns `[]Port{}` if not running, then `o.Exec`s a port-scan script
against `st.ContainerID`. Status and Exec are two calls; the container can
disappear between them. Previously a rare race (a stop landing on a poll
— an un-root-caused transient 404 already sat in TODO.md from the same
family); ADR-067 makes it routine, because a mode toggle REPLACES the
container while the UI polls this endpoint every few seconds.

Fix took TWO attempts, which is the reportable part. The first guarded
only `if err != nil`, re-asking `o.Status` and returning `[]Port{}` when
the project was no longer the running container that had been scanned. It
did not work: a vanished container does NOT surface as a transport error —
the exec SUCCEEDS and returns exit code 128, falling through to the
separate `if code != 0` branch and `fmt.Errorf("port scan exited %d")`
(line 93). Only capturing the live 500 body made the exit-code path
visible.

## Fix
Folded both paths into one `if err != nil || code != 0` guard that
re-asks Status once and returns `[]Port{}` when the project is no longer
running or the container id has changed, keeping the real error otherwise
— the same shape the git endpoints use for a guest that is not there. No
unit test: `internal/orchestrator` has no fake Docker client (`o.docker`
is a concrete `*client.Client` — documented BUG-003 residual, sized in
TODO.md as its own task); browser e2e is its only cover today.

## Prevention
A guest exec reports a vanished container BOTH ways — as a transport
error and as a non-zero exit code — so a guard for one is not a guard for
the failure. When adding a "did the thing go away?" recheck, enumerate
every branch the underlying call can fail through, and confirm the fix
against the captured error text rather than the branch you assumed.

## Related
BUG-003 (same missing fake-Docker-client residual), BUG-021 (the toggle
that made this race routine).
