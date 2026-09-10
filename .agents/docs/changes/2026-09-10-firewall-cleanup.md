---
owner: main
date: 2026-09-10
---

# Firewall cleanup — ADR-067 retire spool machinery   (2026-09-10)

Operator ran `sudo bash deploy/firewall/install.sh` from `/opt/styrman` at
v1.25.0 on the production host, closing item 3 (cleanup) of the "Open" list in
the v1.25.0 shipped handoff. The script removes obsolete ADR-065/066
machinery that ADR-067 deleted (the spool watcher, tmpfs mount, tmpfiles
entry, and spool directory), leaving the firewall service and timer unchanged
and `/etc/styrman/firewall.conf` in place (no longer read, per ADR-067).

## Changes made

- Reinstalled `/usr/local/sbin/styrman-firewall.sh` (unchanged from v1.25.0).
- Removed `styrman-firewall.path` systemd unit (watcher for spool changes).
- Removed `run-styrman\x2dfirewall-spool.mount` tmpfs mount.
- Removed `/run/styrman-firewall` spool directory.
- Removed tmpfiles entry for spool setup.
- Left `/etc/styrman/firewall.conf` untouched with a note (no longer read).

## Verification

Post-cleanup, confirmed non-root:

- `systemctl is-enabled styrman-firewall.path` → `not-found`.
- `systemctl list-units --all 'styrman-*'` lists four units:
  `firewall.service`, `firewall.timer`, `update.service`, `update.timer`
  (was five, spool watcher gone).
- `/run/styrman-firewall` directory absent.
- No `run-styrman*` unit files in systemd dirs.
- Printed firewall chains (STYRMAN-FWD, STYRMAN-IN) unchanged.

## Agents dispatched

- `changelog` (this entry).
- `documentation` (ROADMAP "Now" section updated).

## No code, tests, or review

Operator-only procedural cleanup; no source changes, no test updates.
