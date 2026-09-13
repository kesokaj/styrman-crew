---
owner: bugs
date: 2026-09-13
id: BUG-049
status: fixed
area: .agents/docs/testing/styrman/integration.sh
---
# BUG-049: port guard silently passed when `ss` was missing

## Symptom
The suite's port-in-use guard `if ss … | awk | grep -q` evaluated false (i.e.
"guard passed", port considered free) whenever `ss` itself was missing from
the host, rather than failing loudly.

## Repro
Run the guard on a host without `ss` installed; observe it reports the port
free regardless of actual state.

## Root cause
The pipeline's exit status is the last command's (`grep -q`), which is false
("no match") both when the port is genuinely free AND when `ss` fails to run
at all — the guard could not distinguish "checked and clear" from "could not
check".

## Fix
Guard now requires `ss` to be present and capture-then-grep instead of
piping directly, so a missing/failing `ss` is a hard failure, not a silent
pass.

## Prevention
Same family as BUG-040 (fail-open self-check): a guard built from a pipeline
must fail loudly when any earlier stage in the pipe can't run, not just
report "no match" from whatever stage happened to run.

## Related
BUG-040 (same suite, same fail-open family).
