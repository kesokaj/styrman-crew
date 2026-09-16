---
owner: bugs
date: 2026-09-16
id: BUG-063
status: open
area: internal/orchestrator/orchestrator.go (stopLocked, restartPolicy)
---
# BUG-063: Stop's in-guest SIGTERM lets Docker's restart policy revive the project before ContainerStop runs, then the revived VM gets hard-killed

## Symptom
Observed 2026-09-16 on the production host, right after the v1.34.0 deploy,
project "google" (id f8e77d67-04bf-4099-a933-16f8aa4bfc3d, container
43ba9e48…, image styrman-runtime:1.33.0, created 12:27:15Z; server came up
12:30:31Z). Stopping the project made the VM exit cleanly, Docker restarted
the container by itself one second later, and the stop that followed
hard-killed the restarted VM 35 s later, exit code 255.

Container log: `12:30:42.068 [styrman] shutting down project f8e77d67-…`
then `12:30:45.5 [styrman] dockerd ready` / `project … ready` (second boot).
Server log: `12:30:43.208 "terminal closed" … "reason":"project stopped"`,
then four WARNs 12:31:01–12:31:20 `notifications: status failed` /
`terminals: live scan failed, reporting none live` with `docker inspect …
context deadline exceeded`. dockerd journal: `14:30:43.794 "restarting
container" container=43ba9e48… exitCode=0 exitedAt="2026-09-16
12:30:43.230809596" manualRestart=false`, `14:30:54.332 "stopping
restart-manager"`. containerd journal 14:31:29: `"failed to ping agent:
… DeadlineExceeded"`, `"Wait for process failed" … error="Dead agent"`,
`"sandbox stopped unexpectedly"`, `"Agent did not stop sandbox" error="Dead
agent"`. `docker inspect`: `started=2026-09-16T12:30:43.843Z
finished=2026-09-16T12:31:29.332Z status=exited exit=255`,
`HostConfig.RestartPolicy.Name=unless-stopped`, `RestartCount=1`. No OOM or
hypervisor line in the kernel journal for the window; 60 GB host memory
available. API now reports the project `"state":"stopped"`.

## Repro
Not reproduced on demand; observed once, live, immediately after the
v1.34.0 deploy while stopping project "google".

## Root cause
`stopLocked` (orchestrator.go:1195-1210) first execs `kill -TERM 1` in the
guest so the entrypoint's graceful-shutdown trap runs (the I9 dirty-journal
fix — `docker stop` alone powers the VM off without delivering SIGTERM to
init), then `waitStopped` polls up to 12 s, then calls `ContainerStop` with
a 30 s timeout. The in-guest TERM makes PID 1 exit 0 and the container's
task ends before Docker has been told the container is being stopped.
Project containers run with `unless-stopped` (`restartPolicy`, line 1509),
so Docker's restart manager restarts the container itself
(`manualRestart=false`) — the second boot seen in the log. The later
`ContainerStop` then stops the freshly-rebooted VM instead of the one that
was actually meant to be stopped; here its Kata agent had stopped
answering ("Dead agent") and the stop ended in a hard kill after 35 s —
the exact dirty shutdown the graceful TERM was added to avoid — while the
server's `docker inspect` calls for the container blocked.

Not introduced by v1.34.0: the v1.33.0..v1.34.0 commit range does not
touch orchestrator.go.

## Fix
Not fixed — open, a separate task. Direction to evaluate (not verified):
make Docker aware of the stop before the guest exits — e.g. issue
`ContainerStop` (which marks the container manually stopped so the restart
policy stays quiet) concurrently with or before the in-guest TERM, or make
the restart policy not restart on a clean guest-initiated exit.

## Prevention
A container meant to power off gracefully from the inside must tell the
daemon it is being stopped BEFORE the guest exits — an in-guest exit under
`unless-stopped` is a restart, not a stop. Verify a Stop path by the
container's final state (`RestartCount` stays 0, exit code 0, one boot),
not by "not running" at one instant; a Tier 2 check for this does not yet
exist.

## Related
BUG-030 (the sticky-stop marker also touches `stopLocked`), the I9
dirty-journal fix in image/entrypoint.sh, verification notes
.agents/docs/testing/2026-09-16-verify-release-v1.34.0.md.
