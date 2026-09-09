---
owner: bugs
date: 2026-09-09
id: BUG-024
status: fixed
area: web/e2e/run.sh (cleanup trap)
---
# BUG-024: unguarded docker inspect in the EXIT trap aborted cleanup and flipped the suite's exit status

This closes a standing TODO.md item recorded 2026-09-08 as "not
reproducible on demand".

## Symptom
Two symptoms, long believed separate. (1) `web/e2e/run.sh` exited 1 while
the suite itself printed `passed=86 failed=0 skipped=2` — backwards for
anything automated, and would let a genuinely red run look green just as
easily. (2) `/tmp/styrman-e2e-*` state dirs accumulated (8 present on
2026-09-09; 7 on 2026-09-08), so cleanup was not completing.

## Repro
Reproduced on every run once the ADR-067 toggle checks were in place. The
earlier TODO note said it was seen twice and would not reproduce on
demand.

## Root cause
One line. `set -e` is in force inside an EXIT trap. `cleanup()`
(`web/e2e/run.sh:21`) loops over `docker ps -aq --filter
label=styrman.managed=true` and, for each, does
`env=$(docker inspect "$c" ... 2>/dev/null)` (line 35). A failing command
substitution in an ASSIGNMENT aborts under `set -e`. A container listed by
`docker ps -aq` a moment earlier can be gone by the time the loop body
inspects it — this run's own project, which the server deletes
asynchronously. The trap therefore aborted MID-LOOP, before its final
`sudo rm -rf "${STATE}"`, which is why the state dirs survived; and the
aborted trap handed the shell exit status 1 after a run that had reported
zero failures. Both symptoms, one cause. Earlier investigation had ruled
out the trap because `cleanup`'s last command ends in `|| true` and
returns 0 — true, but the trap never reached it. Proven in isolation with
a 12-line script reproducing the abort, and by instrumenting run.sh to
print node's own status: node exited 0 while run.sh exited 1.

## Fix
`env=$(docker inspect ...) || continue` guards the assignment and skips a
container that is already gone (line 35). Separately, run.sh now captures
the suite's status (`node ... || status=$?`) and ends with
`exit "${status}"` (lines 74-79) so the trap can never decide the exit
code again. Verified: exit 0, `passed=86 failed=0`, no leftover dirs.

## Prevention
`set -e` applies inside EXIT traps, so an unguarded command in cleanup can
abandon the rest of cleanup AND overwrite the script's exit status — guard
every command in a trap, especially an assignment from a command
substitution, and never let a script's exit code fall out of its trap when
it has a real status to report. Anything that enumerates
containers/PIDs/files and then acts on each must tolerate an entry
vanishing between the listing and the action.

## Related
BUG-021 (the toggle work that made the race routine enough to hit every
run). TODO.md's prior "not reproducible on demand" note for this symptom
should be removed now that it is fixed and root-caused.
