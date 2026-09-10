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
default being inherited. See also
`.agents/rules/mechanism-reaches-every-caller.md`.

## Recurrence (2026-09-10, same day)

The fix itself reproduced the class it fixed. `systemctl enable
styrman-firewall.service` was added to the updater's `host_converge`, but
INSIDE the `if [ "${changed}" = "1" ]` block. v1.27.0 shipped at 13:30; the
tick that installed the new unit file ran the PREVIOUS updater binary (no
enable line yet); the next tick (13:36, the new updater) saw no file change
and skipped the block. `systemctl is-enabled styrman-firewall.service` still
read `disabled` after deploy. Caught by main checking the live host, per the
ADR-067 amendment's rule that verification after `install.sh`/deploy is the
operator's, which held again here.

Root cause, as a class: the "caller" was traced by name but not by the
PATH the first execution actually takes — a self-updating tool's install
tick runs the OLD binary, so a step gated on "something changed this tick"
can never see the change that shipped it. Fix in the working tree: the
`systemctl enable` call moved OUT of the changed-block to run
unconditionally every tick (idempotent), so the second tick catches up;
ships with the next tag.

Prevention addition: when a step is added to a self-updating tool's
converge/install path, ask which VERSION of the tool executes the first
run after the change ships (usually the OLD one) — gate nothing needed on
that first run behind a "did anything change" condition; make it
unconditional or idempotent instead. Verify with the exact live-host
command the bug record names, not by reading the unit file's text. See
`.agents/rules/mechanism-reaches-every-caller.md`.

## Related
BUG-034, BUG-036 — same TODO close-out, found by the same review pass.
BUG-034 (claims vs evidence — verify on the live host, not from source
text). This is the second occurrence of "mechanism added, caller path not
fully traced" on the same day; the rule now lives in
`.agents/rules/mechanism-reaches-every-caller.md`.
