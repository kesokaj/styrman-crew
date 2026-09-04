---
name: writing-files
description: Never create or delete at a path without checking it first; a "new tests" commit that deletes lines, or an unnamed " D" after an rm, is a red flag.
---

# Writing and deleting files (BUG-005, BUG-006, BUG-007)

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

## Verify it is TEXT (BUG-007)

Two v1.22.0 source files were written through a Python heredoc that emitted
RAW control bytes where the two-character escape was meant. The code worked —
NUL is a legal JavaScript string character — so every test passed. But git
classified both files as binary, and a code review, a security review and two
later phase reviews all read "Binary files differ" and signed off on files
they could not see.

- After any heredoc or generator write: `git diff --numstat <path>`. A `-` in
  both columns, or `file <path>` saying "data", means the content is invisible
  to every reviewer. Stop, exactly as for BUG-005's deletion count.
- Never emit a raw control byte into source. Write the ESCAPE and let the
  TARGET language decode it. In a Python heredoc that means a raw string
  prefix, or doubling the backslashes.
- A target language that accepts the byte silently (TypeScript, Markdown) is
  more dangerous than one that rejects it (Go's `illegal character NUL`): the
  same mistake in a Go test failed loudly the same day and was fixed at once.
- This applies to REVIEW ARTIFACTS too. The security findings file that raised
  this had embedded the bytes it was describing, making itself unreadable.
- Reviewing a diff that says "Binary files differ" for something that should
  be source is not a review. Refuse rather than skip.

The three sections above are one family: BUG-005 wrote over a file without
looking, BUG-006 deleted files without looking, BUG-007 wrote files nobody
could look at. Each was invisible in the normal review path, and each is
caught by asking git what actually changed before committing.
