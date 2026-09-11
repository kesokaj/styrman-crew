---
owner: bugs
date: 2026-09-11
id: BUG-041
status: fixed
area: process — pre-commit verification chain (styrman-ctl Phase 4, security-round-4)
---
# BUG-041: a failing `go test` was committed because it was piped into `tail -1` inside the `&&` chain leading to the commit

## Symptom
Commit `b01914a` landed with two failing tests
(`TestStyrmanCtlUninstallPlans`, `TestStyrmanCtlUninstallConfirmations`).
The chain run was
`go test … 2>&1 | tail -1 && go vet ./... && … && git commit`; it printed a
line containing `FAIL` but proceeded to `go vet`, later steps, and the
commit anyway, because the chain's exit status was `tail`'s (0), not
`go test`'s. Caught only because the word FAIL was still visible in the
captured output after the fact.

## Repro
Not reproduced as a live run; reconstructed from the commit's shell history
and the failing test names, which matched a test fixture missing a compose
file a new marker check required.

## Root cause
The exact anti-pattern AGENTS.md's hard rules already name: "A verification
command is never piped into a filter inside a `&&` chain that leads to a
commit: the chain then guards the filter's exit status, not the command's."
Repeated under time pressure after many prior green runs in the same
session, on a one-liner built to keep output short.

## Fix
Next commit reran the tests bare, read the exit status before committing,
and fixed the fixture (added the missing compose file the new marker check
required).

## Prevention
Restates the existing AGENTS.md hard rule — it was not followed here, so
treat this as a rule that recurs rather than a new one to add: run
verification bare, capture the exit status (variable or log file), read it,
then issue the commit as its own call. `set -o pipefail` in a one-off shell
is not a substitute — the harness's Bash calls do not carry a shell state
that persists it.

## Related
BUG-033 (identical mechanism: `grep` inside a `&&` chain guarding a commit).
BUG-034 (silence from instrumentation read as evidence). BUG-040 (a green
run that verified the wrong target). All four are "a green signal that
measured something else" — this rule has now recurred three times
(BUG-033, BUG-041, and BUG-034/040 in the same family) and belongs
permanently enforced, not just stated in AGENTS.md prose — e.g. a repo-level
pre-commit check or a verifier-agent habit that greps the exact command
string for a bare `|` before a trailing `&&`.
