---
owner: bugs
date: 2026-09-22
id: BUG-076
status: fixed
area: internal/httpapi/explorer.go
---
# BUG-076: managed-home guard's first fix refused deleting the link itself

## Symptom
The first version of the `managedHomeTarget`/`traversesALink` guard (see
BUG-075) refused any path whose first component was a symlink, including
the link itself. That broke `TestFileDeleteSymlinkEscape` in
`internal/httpapi/explorer_files_test.go:238`, which requires that
deleting a bad symlink returns 204 — removing only the link is ordinary
housekeeping. Caught by the existing test, not by review.

## Repro
Run `internal/httpapi/explorer_files_test.go`'s `TestFileDeleteSymlinkEscape`
against the first-draft guard; it fails (expects 204, guard refuses).

## Root cause
The guard was written broader than the property it needed to enforce: it
treated "path's first component is a symlink" as always hostile, without
distinguishing "operating on the link itself" (legitimate) from
"traversing through the link to something beyond it" (hostile).

## Fix
Final guard (`traversesALink`, same commit as BUG-075) excludes the last
path component from the symlink check, so deleting a link itself is
permitted while traversal through a link is not.

## Prevention
A guard added in front of a destructive operation is tested against the
legitimate uses of that operation as well as the hostile one, and the
distinction gets its own assertion so the next edit cannot re-broaden it.
This is the guard-fail-open family: see the global hard rule "a guard in
front of a destructive action keeps the data when its check errors" —
here the failure mode was over-refusal on the legitimate path instead,
caught only because a pre-existing test already asserted that path.

## Related
BUG-075 (the guard's final shape and its own gap). Same commit,
`internal/httpapi/managedtarget_test.go` and
`internal/httpapi/explorer_files_test.go:238`.
