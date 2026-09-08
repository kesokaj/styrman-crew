---
owner: main
date: 2026-09-08
---

# Live verification — per-project LAN access on production   (2026-09-08)

Range verified: v1.23.0 (`7bf0607`) as deployed, plus the host half installed
by `deploy/firewall/install.sh` run from `/opt/styrman` at that tag. This is
the check the plan's Verification section called for and the verifier log of
the same day could not do, because the host half did not exist yet.

Written by the main agent, not the `verifier` agent: these are first-hand
records of commands run against the production host, several of them
requiring root, which is outside that agent's remit.

## Install

`sudo bash deploy/firewall/install.sh`, from the deploy clone at `v1.23.0`
(verified `7bf0607`, worktree clean, `install.sh` byte-identical to the
reviewed file). Dry-run inspected first. Result:

- Script and three units installed — no-ops, `host_converge` had already
  placed byte-identical copies from the same object.
- `/etc/tmpfiles.d/styrman-firewall.conf` written; run dir and spool created.
- `run-styrman\x2dfirewall-spool.mount` written and enabled: a 1 MiB,
  1024-inode tmpfs at the spool, `mode=700 uid=1000 gid=1000
  nosuid,nodev,noexec`.
- `/etc/styrman/firewall.conf` written (it was absent), discovering
  `LAN_DEV=enp114s0` and `LAN_CIDRS=192.168.86.0/24` from the default route.
- Status went from `warn=conf … missing` to `lan_dev=enp114s0
  lan_cidrs=192.168.86.0/24`, no `conf_error`.

## PASS — the mount propagates into a running container (BUG-016)

The one integration point that could only be proven in production. The
server container was started BEFORE the spool tmpfs existed. Its own mount
table (`/proc/<pid>/mountinfo`) afterwards:

```
310 292 0:25 /styrman-firewall /run/styrman-firewall  ... master:5   -- tmpfs
114 310 0:51 /                 /run/styrman-firewall/spool ... master:100 -- tmpfs rw,size=1024k,nr_inodes=1024,mode=700,uid=1000,gid=1000
```

The tmpfs mounted minutes after container start is visible inside it, with
the right owner and bounds, and no restart was needed. That is exactly what
the `rslave` propagation fix was for; with the default `rprivate` the server
would have kept writing into the unbounded directory underneath, where root
never drains.

## PASS — end to end through the real reconciler

`STYRMAN_IT_LIVE_LAN=1 bash .agents/docs/testing/styrman/integration.sh
styrman-runtime:1.23.0` — **83 passed, 0 failed, 3 skipped** (the three
host→bridge proxy checks a firewall-hardened host always skips; the LAN live
block no longer skips). The five checks that had never run before:

| Check | Result |
|---|---|
| Allow rule at the head of `STYRMAN-FWD` within 5 s of the marker | PASS |
| In-guest ping to the LAN gateway 192.168.86.1 succeeds | PASS |
| The host itself, 192.168.86.58, stays unreachable from the guest | PASS |
| Rule gone within 5 s of the marker being removed | PASS |
| In-guest ping to the gateway dropped again | PASS |

With, in the same run, the R12 negative check on a project with no
allowance: an in-guest ping to the gateway is dropped. So both directions of
the requirement are now demonstrated on real hardware, not inferred:
isolation by default, reachability only for a marked project, and the host
itself unreachable in both cases.

## Clean afterwards

Markers: 0. `styrman-lan:` rules in the chain: 0. Spool drained. Status
healthy. Only `styrman-caddy` and `styrman-server` remain; the throwaway
test project was removed by the suite's own cleanup.

## Known flaw in the live block, not triggered this run

The opt-in block writes its marker into production's marker directory, named
by the uuid of a project that lives only in the test server's own state dir.
The production server's 2-minute reconcile sweeps markers whose uuid matches
no record it knows — by design, fail-closed — so it would delete the test's
marker mid-check if the tick landed inside the roughly 30-second window. It
did not this time (no removal appears in the server log, and the suite's own
cleanup got there first), but the check is racy and a failure would look
like a feature failure rather than a test artefact. Recorded in `TODO.md`.

## What is still not verified

- Docker live-restore preserving `DOCKER-USER` and `STYRMAN-*` across a
  daemon restart. Bounded exposure either way is ≤65 s LAN-closed, never
  LAN-open.
- The applied-state UI on production (pending → on, the chip, the danger
  dialog) was not driven in a browser; the API-level and reconciler-level
  behaviour behind it is covered above and by the e2e suite's unavailable
  branch. A human flipping the switch once is still the last unautomated
  step.
- The published-port DNAT shape, deliberately outside the allowance and
  unverifiable here because this host publishes no Docker port.
