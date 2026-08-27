# 2026-08-27 — Reliability core (Batch B)

Second remediation batch from the review. All Go/orchestrator reliability
fixes that bite even a single operator.

- **I1 — network sweep no longer bricks idle projects.** `CleanupLegacy`
  removed *every* `sdb-` network; a stopped project's network was deleted on
  restart → `EnsureRunning` failed "network not found" forever (violated R7).
  Now it keeps networks referenced by any current managed container (keep-set
  from labels) and removes only truly-orphaned ones. Defense in depth:
  `EnsureRunning` recreates + reconnects a missing project network on a start
  failure (`recreateMissingNetwork`).
- **I5 — terminal reopen no longer hangs / spirals.** The activeConns swap is
  now atomic under `swapMu` (two simultaneous attaches can't both attach and
  trip the tmux `-D` reconnect loop), and the old connection's `Close(4001)`
  runs in a goroutine (a dead peer's ~5-10 s close handshake no longer stalls
  the new attach — restored the "reopen < 5 s" behavior). Frame read limit
  capped at 16 MiB (N12).
- **I6 — server listener starts immediately after a reboot.** Boot `Adopt` +
  the self-attach loop moved to a bounded background goroutine (60 s); a slow
  dockerd restarting 10 Kata VMs no longer delays `/api/health` into an
  ingress 502.
- **I2 — idle reaper won't kill a just-opened terminal.** New `stopIfIdle`
  takes the per-project lock and re-checks activity before stopping, so an
  attach that raced the reap decision keeps the VM (and its tmux state).
- **I3 — setup VMs no longer leak.** They are created with restart policy
  `no` (ephemeral, don't resurrect after reboot) and reaped by `ReapSetupVMs`
  when a setup is abandoned (running longer than the idle timeout).
- **I16 — a failed stale-setup-VM removal returns an error** instead of
  falling through to a misleading "name already in use" 500.
- **I4/N24 — `profileStore.GuestName` race fixed** with a `sync.RWMutex`
  (read on every terminal attach, written from Settings); `setGuestUser` now
  fsyncs like the other stores.
- **I7 — exec no longer leaks a goroutine/hijacked conn** on a wedged guest
  command: `StdCopy` runs in a goroutine and is abandoned on ctx cancel
  (att.Close unblocks it), with the buffer read only after it stops (no race).
- **I8 — port scan is one `ss -ltnH` pass** instead of an
  O(listening × open-fds) `/proc/*/fd` walk (~190 ms/port before); falls back
  to a `/proc/net/tcp` scan if `ss` is absent.

Tests: `containerGuest` + `restartPolicy` unit tests (N19); `go test -race`
clean; integration 56/56 (live port scan), e2e 32/32; runtime image rebuilt;
prod redeployed.
