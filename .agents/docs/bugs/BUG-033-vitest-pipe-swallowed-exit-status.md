---
owner: bugs
date: 2026-09-10
id: BUG-033
status: fixed
area: process — pre-commit verification chain (web/)
---
# BUG-033: red vitest case committed and pushed to origin/main

## Symptom
`fdef866` was committed AND pushed with 1 failing test. Terminal output showed
`1 failed | 137 passed` from vitest, but the shell chain continued past it into
`git commit` and `git push`. Failing case:
`still accepts a comment with inner spaces and a tab separator`
(`web/src/lib/sshKey.test.ts`).

## Repro
Run: `npm run check && npm run lint && npx vitest run 2>&1 | grep -E 'Tests ' && npm run build && ... && git commit && git push`
with a vitest suite containing at least one failing test. The chain reaches
`git push` regardless of the failure.

## Root cause
`npx vitest run 2>&1 | grep -E 'Tests '` pipes the runner into a filter; the
pipeline's exit status is `grep`'s (0, since the summary line matched "Tests "),
not vitest's. The `&&` chain therefore evaluated the FILTER's success, not the
TEST RUN's. The regression itself was real: the same commit added a reject of
every `\p{Cc}` control character in `.pub` comment parsing, which also swallowed
TAB — a legal field separator in a `.pub` line.

## Fix
`5df2959`: TAB exempted via `line.replaceAll("\t", " ")` before the control-char
class test; 138/138 green; lint clean. (A second draft used a literal control
byte in the regex and tripped oxlint's `no-control-regex`, caught because that
chain was read before committing.)

## Prevention
Never pipe a verification command into a filter inside a `&&` chain that leads
to a commit — the chain guards the filter's exit status, not the command's. Run
the command bare (or `set -o pipefail`) and filter captured output afterward,
as a separate step. A commit chain ends at the last verification command; the
commit itself is issued only after reading that command's result on screen —
"the summary said 1 failed" was visible and unread; a chain built so it cannot
stop on it is the defect, not the eyes.

## Related
Same family as BUG-005/006/007 (`.agents/rules/writing-files.md`): each is
"the tool told you what changed/passed and the process didn't ask." Not yet a
third repeat within this specific pipe-swallows-status shape, but if it
recurs, the `set -o pipefail` rule belongs in `.agents/rules/writing-files.md`
or a project `AGENTS.md` verification-chain rule.
