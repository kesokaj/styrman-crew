---
owner: bugs
date: 2026-09-22
id: BUG-075
status: fixed
area: internal/httpapi/explorer.go
---
# BUG-075: managed-home guard checked only the path's first component

## Symptom
`managedHomeTarget` refuses destructive operations on managed subtrees
(`projects`, `.dockerd`, `.styrman`, `.ssh`) by matching those names as
strings, but `os.Root` follows symlinks that stay inside the root: a
symlink (`ln -s .dockerd x`) let a delete on `x/disk.img` reach the managed
subtree through an unmanaged name. The first fix checked only the path's
FIRST component, so `work/sub/link -> ../../.dockerd` walked straight past
it — found independently by both the code and security reviews of the same
diff (`reviews/2026-09-22-review-phase0b-code.md`,
`-security-phase0b.md`).

## Repro
Create `work/sub/link -> ../../.dockerd` below a project home, then delete
through the link at depth; pre-fix code permits it.

## Root cause
A path guard inspected only the first path component instead of every
component the operation would traverse, so a symlink placed deeper than
the top level bypassed it.

## Fix
`traversesALink` lstats every component except the last through the
confined root, failing closed on any error it cannot attribute to a
missing name; the last component is excluded because acting on a link
itself removes only the link. Test:
`internal/httpapi/managedtarget_test.go`,
`TestManagedHomeTargetRefusesASymlinkNestedDeeper`, written before the fix
and failing against the pre-fix code.

## Prevention
When a guard inspects a path, it inspects every component the operation
will traverse, not the first — and the test plants the hostile component
at depth, not only at the top.

## Related
BUG-076 (the same guard's first attempt broke `TestFileDeleteSymlinkEscape`).
Same family as `.agents/rules/guest-writable-paths.md` (BUG-002/003): a
path is only as trusted as its least-trusted parent.
