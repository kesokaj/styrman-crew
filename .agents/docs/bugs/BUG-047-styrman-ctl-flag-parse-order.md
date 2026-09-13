---
owner: bugs
date: 2026-09-13
id: BUG-047
status: fixed
area: deploy/styrman-ctl
---
# BUG-047: `styrman-ctl --dry-run status` read as "unknown verb --dry-run"

## Symptom
`styrman-ctl --dry-run status` failed with "unknown verb --dry-run" instead
of running `status` in dry-run mode.

## Repro
`styrman-ctl --dry-run status`

## Root cause
`main()` took `$1` as the verb before parsing any flags, so a global flag
placed before the verb was mistaken for the verb itself.

## Fix
Flags are now collected from the whole argv before the verb is chosen.

## Prevention
A shell CLI must parse global flags across the whole argument list before
selecting the verb/subcommand — never assume `$1` is the verb without first
stripping recognized global flags from anywhere in argv.

## Related
Same file/batch as BUG-048 (trap-on-local-var, same commit range).
