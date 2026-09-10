---
owner: main
date: 2026-09-10
---

# Updater enables firewall unit on every tick    (date: 2026-09-10)

## Summary

v1.27.0's boot-time firewall unit was installed via Phase 3 with a `systemctl enable styrman-firewall.service` call guarded by an `if [ "${changed}" = "1" ]` block in `host_converge`. After the 13:30 deploy, the unit file landed but remained disabled on the live host: the tick that installed the unit file ran the PREVIOUS updater binary (which had no enable line), and the next tick, the new updater saw no unit file change since v1.27.0 and skipped the entire conditional block. A race between code deployment and mechanism execution — the same pattern as BUG-035, which occurred the same day.

**Fix:** moved `systemctl enable styrman-firewall.service` outside the `if [ "${changed}" = "1" ]` conditional so it runs idempotent on every `host_converge` tick. The rule "a mechanism that must reach every caller should not be guarded by state-change conditionals" graduated to `.agents/rules/mechanism-reaches-every-caller.md` as a prevention rule. Until this release ships, the operator's one-time `sudo bash deploy/firewall/install.sh` (already required by v1.27.0) enables the unit and narrows the boot window to the 5 second first timer tick.

## Type

bugfix

## Files changed

- `deploy/update/styrman-update` — `host_converge()` function: moved `systemctl enable styrman-firewall.service` outside the changed-files conditional (12-line diff)

## Agents dispatched

bugs (graduated rule to `.agents/rules/mechanism-reaches-every-caller.md`), changelog (this entry).

## Verification

- `bash -n` syntax check on `deploy/update/styrman-update` (passed)
- 12-line diff, below reviewer threshold; read by author (no separate review dispatch)

## Requirements

n/a — operational fix (unit enablement), no user-facing behavior change.

## Related

- BUG-035 (same-day incident, same pattern: `systemctl enable` guarded in a conditional that deferred on second tick)
- `.agents/rules/mechanism-reaches-every-caller.md` (prevention rule)
- v1.27.0 (requires manual `sudo bash deploy/firewall/install.sh` to enable unit until this fix ships)
