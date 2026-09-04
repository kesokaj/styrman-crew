---
owner: bugs
date: 2026-09-04
id: BUG-006
status: fixed
area: agent memory / working-directory hygiene (no source area)
---
# BUG-006: `rm -rf web/.claude` to remove one stray file deleted two tracked, committed files

## Symptom
During v1.22.0 phase 3, a `changelog` subagent wrote a memory file to
`web/.claude/agent-memory/changelog/project_conventions.md` (stray — canonical
location is repo-root `.claude/agent-memory/`). Judged it stray and ran
`rm -rf web/.claude`, which also deleted two TRACKED, committed files:
`web/.claude/agent-memory/documentation/MEMORY.md` and
`web/.claude/agent-memory/documentation/project_styrman_docs_structure.md`.

## Repro
Not reproduced as a test; caught live via `git status --porcelain` immediately
after the delete, showing ` D` entries for two paths not intended to be
touched. Restored with `git checkout -- web/.claude/`. Nothing committed or
pushed; no lasting damage.

## Root cause
Two parts. (1) Checked that the target file was safe to remove (`git
check-ignore`, inspected content) but never checked what else lived under the
directory being deleted — `rm -rf <dir>` was aimed at one file inside it, not
the whole tree. (2) The stray file existed because several earlier Bash calls
used a bare `cd web`, which persists across tool calls in this harness; a
subagent dispatched afterward inherited `web/` as cwd and resolved its memory
path relative to it. The pre-existing tracked files under
`web/.claude/agent-memory/documentation/` are almost certainly the same
accident from an earlier session, committed rather than noticed.

## Fix
Not a code fix — recovered in-session with `git checkout -- web/.claude/`
before anything was committed.

## Prevention
Before any recursive delete (`rm -rf`/`rm -r`) of a directory, run
`git ls-files <dir>` (and `ls -R`) and account for everything reported;
deleting a directory to remove one file inside it is never right if the
directory holds tracked content — delete the single file by name instead.
Run `git status --porcelain` after any delete: a ` D` line on a path not
named is a stop sign, the same class of signal as a nonzero deletion count
in `git diff --stat` on an "add" commit (see `.agents/rules/writing-files.md`,
BUG-005 — that rule covers overwrite via `cat >`/Write/heredoc but not
deletion; this is the same class and should be added there).

Secondary: prefer `(cd web && ...)` subshells over a bare `cd web` in this
harness — the working directory persists across tool calls and silently
reparents later commands and any subagent dispatched afterward, which is how
the stray file (and its earlier-session sibling) ended up under `web/.claude/`
in the first place.

## Related
BUG-005 (same root class: destructive filesystem op without checking existing
tracked content first). `.agents/rules/writing-files.md` should gain a
"before deleting a directory" clause covering both BUG-005 and BUG-006 —
recommend folding this rule into that file directly since it already exists
for the sibling case.
