---
owner: bugs
date: 2026-10-02
id: BUG-083
status: fixed
area: process — the main agent's own test runs (.agents/docs/testing/styrman/integration.sh)
---
# BUG-083: the main agent edited the integration suite while a run was executing it

## Symptom
A red integration run ended with
`.agents/docs/testing/styrman/integration.sh: line 565: syntax error near unexpected token `}'`
and `passed=107 failed=6 skipped=3 aborted=1`. The suite's last checks (the delete/wipe section) never ran.

## Repro
Start `bash .agents/docs/testing/styrman/integration.sh styrman-runtime:test` in the background. While it runs, rewrite the script in place (a Python `open(path, 'w')` that inserted lines above the point bash had reached).

## Root cause
Bash reads a script from its file as it executes, by offset, so text inserted above the point it had reached shifted what it read next into the middle of another statement. The main agent treated its own background run as finished business and went on to the next task in the same file. The project's frozen-tree rule was written for `verifier` dispatches; the agent did not apply it to a run it had started itself.

Consequence for evidence: the six failures that run was started to show (five new checks and the boot-budget check at 16.40 s) are in the part that ran before the edit. "Everything else passes" was NOT shown by that run; it is taken from the later full runs (`passed=118 failed=0 skipped=3`, twice by the main agent and once by the verifier).

## Fix
No code. The run's evidence was limited to what it printed before the edit, and the full suite was run again after the changes. Later edits of the suite were made only after `pgrep -f "bash .agents/docs/testing/styrman/[i]ntegration.sh"` found no run (the bracket keeps the pattern from matching the shell that runs pgrep; the first, unbracketed attempt matched itself and reported a run that did not exist).

## Prevention
While a suite is running, do not write to the script it is executing or to any file it reads later: the frozen-tree rule covers the main agent's own background runs, not only a verifier's. Check with a self-excluding `pgrep` before editing a suite file, and treat a log that ends in `aborted=1` as evidence only for the lines above the abort.

## Related
`.agents/rules/verify-bare.md` (BUG-033, BUG-041: a signal that measured something else), BUG-034.
