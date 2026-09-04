---
name: writing-files
description: Never create a file at a path without checking it first; a "new tests" commit that deletes lines is a red flag.
---

# Writing files (BUG-005)

The first ADR-064 test file was written with `cat > cmd/styrman/migrate_test.go`
onto a file that had existed since v1.3, silently deleting two live tests; three
review rounds and a green `go test ./...` did not notice, because a deleted
test cannot fail.

- Before `cat >`, `Write`, or a heredoc: `ls` / `git ls-files` the path. If it
  exists, append or edit; never replace.
- New tests go into a new file whose name cannot collide (suffix the feature
  or the ADR), or are appended to the existing one.
- `git diff --stat` before committing anything described as "added": a
  non-zero deletion count on a file you meant to add to needs an explanation.
- A verifier that vouches for tests compares the test count before and
  after the change, not just the suite's exit status.
