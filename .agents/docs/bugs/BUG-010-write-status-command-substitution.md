---
owner: bugs
date: 2026-09-08
id: BUG-010
status: fixed
area: deploy/firewall (styrman-firewall.sh)
---
# BUG-010: `write_status` inside `$(...)` silently swallowed its own failures under `set -e`

## Symptom
Found while reworking test 9: `write_status` (which can fail, e.g. a failing
`mktemp`) was invoked inside a command substitution. `set -e` does not
propagate a failure inside `$(...)` back to the calling script unless
`inherit_errexit` is enabled, so a failing status write was silently
swallowed instead of aborting the run.

## Repro
Not independently reproduced as a standalone case; surfaced while reworking
`deploy/firewall/test.sh` test 9 (see BUG-011).

## Root cause
`write_status`, a function with side effects (writes a file) and a failure
mode, was called in a value-producing position (`$(...)`) in a `set -e`
script that had not opted into `inherit_errexit`, so bash's normal
errexit-in-subshell suppression applied.

## Fix
Direct call instead of command substitution, plus `shopt -s inherit_errexit`
added at the top of the script.

## Prevention
In any `set -e` script, never place a function with side effects inside
`$(...)` — errexit does not propagate into a command substitution unless
`inherit_errexit` is set. New root scripts in `deploy/firewall/` and
`deploy/update/` should set `shopt -s inherit_errexit` alongside `set -eu` at
the top, as a standing convention, not something re-derived per script.

## Related
BUG-011 (found in the same test-9 rework); `deploy/firewall/styrman-firewall.sh`.
