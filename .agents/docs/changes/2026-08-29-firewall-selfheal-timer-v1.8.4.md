# Self-healing network-isolation firewall (v1.8.4)

Date: 2026-08-29. Follow-up to the v1.8.3 isolation fix: the rules only
re-applied on reboot, so a Docker daemon restart, a firewall-manager reload, or
a manual flush could silently drop isolation until the next boot. Operator asked
for a small service that keeps them applied.

## Change (`deploy/host-setup.sh`, applied live)

- `styrman-firewall.service` is no longer `RemainAfterExit=yes` — it's a plain
  oneshot that goes inactive after each run, so it can be re-triggered.
- New `styrman-firewall.timer` (`OnBootSec=15s`, `OnUnitActiveSec=60s`) re-runs
  the service every minute — self-healing: any flush is undone within ~1 min.
  The rules use the `sdb-+` wildcard, so new project bridges are covered with no
  per-network rule.
- `styrman-firewall.sh` rewritten to **delete-then-insert** each rule instead of
  `-C || -I`: iptables-nft's `-C` check is unreliable with the `sdb-+` wildcard
  (it reported a missing rule as present and skipped the insert), which had left
  the 192.168/16 (LAN) rule un-restored.

## Two bugs found and fixed while wiring this

1. `iptables-nft -C` false-positive with the wildcard → switched to
   delete-then-insert.
2. The old `RemainAfterExit=yes` service was stuck `active (exited)`, so both
   `systemctl start` and the timer were no-ops (start on an already-active
   oneshot does nothing). Dropping RemainAfterExit fixed it.

## Verification (live)

Timer active + enabled; service resets to inactive after each run. Deleted the
192.168/16 rule with no manual trigger → the timer **auto-restored it within
5 s**, and LAN stayed blocked. host / LAN / metadata blocked, internet reachable.

## Release

Host-only (no server rebuild): units + `/usr/local/sbin/styrman-firewall.sh`
installed and running live; `deploy/host-setup.sh` updated so a fresh install
reproduces it. Tagged v1.8.4.
