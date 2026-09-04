---
name: writing-files
description: Never create or delete at a path without checking it first; a "new tests" commit that deletes lines, or an unnamed " D" after an rm, is a red flag.
---

# Writing and deleting files (BUG-005, BUG-006)

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

## Deleting (BUG-006)

`rm -rf web/.claude` was aimed at ONE stray untracked file inside that
directory and took two tracked, committed files with it. Caught only because
the next `git status` showed ` D` on paths nobody had named.

- Before any recursive delete of a DIRECTORY (`rm -rf`, `rm -r`): run
  `git ls-files <dir>`. If it reports anything, delete the single file by
  name instead. Removing a directory to get rid of one file inside it is
  almost never what you meant.
- `git status --porcelain` after any delete. A ` D` line on a path you did
  not name is a stop sign — the same signal as a nonzero deletion count in
  `git diff --stat` on a commit described as "added".
- A directory looking like junk is not evidence: `web/.claude/agent-memory/`
  was committed by an earlier accident of the same kind, which is exactly why
  it looked disposable.

## Working directory

`cd` persists across Bash tool calls in this harness, and a subagent
dispatched afterwards inherits it. A bare `cd web` earlier in a session is
what put an agent's memory file under `web/` in the first place. Use a
subshell — `(cd web && npm test)` — so the next command, and the next agent,
still start where they think they do.
