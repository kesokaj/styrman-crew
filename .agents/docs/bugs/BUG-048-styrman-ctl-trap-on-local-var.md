---
owner: bugs
date: 2026-09-13
id: BUG-048
status: fixed
area: deploy/styrman-ctl (setup_auto_update)
---
# BUG-048: root-only token temp dir leaked because an EXIT trap named a `local` variable

## Symptom
An interrupted `setup_auto_update` run left the root-only token temp dir on
disk instead of being cleaned up.

## Repro
Interrupt `styrman-ctl` mid-`setup_auto_update` (or let the function return
normally then check for a lingering trap effect on a later signal).

## Root cause
`trap 'rm -rf "${tmpd}"'` named `tmpd`, a `local` variable to the function.
Once the function returned, `tmpd` no longer existed in scope for the trap,
so the trap executed `rm -rf ""` — effectively a no-op — instead of removing
the real directory, and the directory leaked.

## Fix
Replaced with a global `CTL_TMPD` and a guarded trap cleared on all four
signals so the trap always names a variable that exists when it fires.

## Prevention
A trap that names a variable must name one that still exists at the moment
the trap can fire — a `local` variable scoped to the function that installs
the trap does not survive the function's return.

## Related
BUG-047 (same file/batch, different mechanism).
