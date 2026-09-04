---
owner: bugs
date: 2026-09-04
id: BUG-005
status: fixed
area: cmd/styrman (migration tests)
---
# BUG-005: new migration tests overwrote two live pre-existing tests in the same file

## Symptom
v1.21.0 release reviewer (`.agents/docs/reviews/2026-09-04-review-v1.21.0.md`, medium) found
`cmd/styrman/migrate_test.go` in commit `f3882ac` no longer contained `TestMigrateWorkspaces`
and `TestEnsureReposRefusesUnmigrated`, two tests present since v1.3 pinning the ADR-027
workspace migration and the R13 "never shadow an unmigrated workspace" guard. Both functions
under test still exist and had their input path re-pointed to `base/` by the same change.

## Repro
`git show f3882ac --stat -- cmd/styrman/migrate_test.go` shows `+N/-M` with M > 0 on a commit
that claimed to only add tests.

## Root cause
The agent created new migration tests with `cat > cmd/styrman/migrate_test.go`, without
checking the path already existed, choosing a name that collided with a pre-existing 2026-era
file and overwriting it. Three review rounds (verifier, reviewer, security x2) on the diff all
read it as "added tests" and missed the deletion because the diff header showed a modified file
with the removed hunk above the additions. `go test ./...` stayed green throughout — a deleted
test cannot fail.

## Fix
Both tests restored verbatim from `v1.20.0:cmd/styrman/migrate_test.go` into the same file below
the new tests; all seven tests pass (`go test ./cmd/styrman/ -race`). Restoration rides in the
v1.21.0 pre-tag fix commit.

## Prevention
Never write a test or source file with `cat >`/Write without first checking the path exists
(`ls`, `git ls-files`); add tests via append or a new file name that cannot collide. Read
`git diff --stat` for a deletion count on any commit that claims to only add — a nonzero `-M`
on an "add tests" commit is a stop sign. A verifier that counts tests must compare the count
before and after the change, not just run the suite green.

## Related
`.agents/docs/reviews/2026-09-04-review-v1.21.0.md`.
