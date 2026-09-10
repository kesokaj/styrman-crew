---
owner: bugs
date: 2026-09-10
id: BUG-038
status: open
area: web/e2e/run.sh (cleanup trap)
---
# BUG-038: e2e cleanup trap reported bash parse error after a fully green suite

## Symptom
`bash web/e2e/run.sh` (Phase 2 close-out, ~12:00) printed a green suite
(`passed=87 failed=0 skipped=2`), then from the EXIT trap:
```
web/e2e/run.sh: line 133: unexpected EOF while looking for matching `'`
```
and exited 2. Log: `.agents/scratch/e2e-2026-09-10-phase2.log`. Line 133 is
`status=0`, the line before `node … run.mjs || status=$?; exit "${status}"`.

## Repro
Not reproduced. `bash -n web/e2e/run.sh` passes. The trap function run in
isolation with a nonexistent STATE returned 0. A full rerun on the same,
unmodified tree under `bash -x`
(`.agents/scratch/e2e-2026-09-10-final.{log,trace}`) exited 0, printed no
such message, and left no leaked state dirs or containers.

## Root cause
Not established. The only change to `run.sh` that day (one added line at
the top, an `ensure-test-image.sh` call) is not in the trap, so it is not
implicated. Candidate, unverified: the trap's
`docker inspect … --format '{{range .Mounts}}{{.Source}}⏎{{end}}'` — a
single-quoted format string containing a literal newline inside `$( )` —
combined with a container vanishing mid-loop (BUG-024/BUG-027's shape),
causing bash to misreport the enclosing line. Weak: the message names an
unterminated quote, not a substitution failure.

## Fix
Not fixed — status open, not reproduced on the same tree.

## Prevention
If this recurs, capture the trap under `bash -x` at the time it happens
(the clean final run's trace shows the technique works) rather than
after the fact. Consider moving any multi-line/quote-heavy format string
used inside the EXIT trap into a variable defined outside the trap, so a
parse problem in it surfaces at definition time under `bash -n` instead of
only when the trap actually runs.

## Related
BUG-024 (set -e inside EXIT traps swallowing/mangling exit status in this
same trap), BUG-027 (same trap's container filter never matching a real
instance).
