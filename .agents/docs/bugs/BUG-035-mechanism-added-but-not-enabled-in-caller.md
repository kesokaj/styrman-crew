---
owner: bugs
date: 2026-09-10
id: BUG-035
status: fixed
area: deploy/firewall, integration test harness (TODO close-out)
---
# BUG-035: a new unit/flag was added but no caller was updated to enable it

## Symptom
`styrman-firewall.service` gained `[Install] WantedBy=multi-user.target`, but
neither `deploy/firewall/install.sh` nor the updater's `host_converge` ran
`systemctl enable` for it — reviewer and security both flagged it in Phase 3;
live `is-enabled` reported `static`. Same shape in the test harness:
`ensure-test-image.sh` forwards `$1` so `integration.sh styrman-runtime:dev`
would re-stamp the production image tag — a guard existed in the suites'
own defaults, but not in the new script that called into them.

## Repro
`deploy/firewall/install.sh` then `systemctl is-enabled styrman-firewall`
returns `static` instead of `enabled`. For the second case:
`ensure-test-image.sh` invoked with the production tag re-stamps it, bypassing
the guard that exists only in the suites' default-argument path.

## Root cause
A mechanism (unit section, flag, default) was added in the place that defines
it but not propagated to every installer/caller that must invoke it for the
mechanism to take effect. A guard living only inside one caller's default
value is invisible to a new caller that supplies its own argument.

## Fix
Reviewer/security findings from Phase 3 fixed same day (uncommitted at time
of this record; see plan `.agents/docs/plans/2026-09-10-todo-closeout.md`
Phase 3 tasks) — `install.sh` and `host_converge` now run `systemctl enable`.

## Prevention
When adding a unit section, a flag, or a default, grep every
installer/caller that must honour it and list them explicitly in the change
entry. A guard that lives in a caller's default argument must be re-stated
inside any new callee that can supply its own argument — do not rely on the
default being inherited.

## Related
BUG-034, BUG-036 — same TODO close-out, found by the same review pass.
