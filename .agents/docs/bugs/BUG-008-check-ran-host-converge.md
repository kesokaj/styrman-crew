---
owner: bugs
date: 2026-09-08
id: BUG-008
status: fixed
area: deploy/update (styrman-update)
---
# BUG-008: `styrman-update --check` could install/restart the firewall units it promises not to touch

## Symptom
Review (`.agents/docs/reviews/2026-09-08-review-lan-phase1.md`, findings 1-2):
`host_converge()` ran unconditionally before the `CHECK_ONLY` exit, so `--check`
— documented at `styrman-update:25` as "report what would happen, change
nothing" — could `atomic_install` a new root-owned `styrman-firewall.sh`/units
and `systemctl daemon-reload`/`enable --now`/`start` them whenever the deployed
tree's firewall files differed from what was on disk. Separately, the
pre-existing FAILED-tag latch (`:349-353`) exited before the function
definitions, so once a release was marked FAILED, no later automatic tick ever
reached `host_converge` again — contradicting the "runs every tick" design
ADR-065 states at `:469-471`.

## Repro
Not reproduced live; found by code review reading the diff (both findings
concrete-scenario walkthroughs in the review file, not executed against a
host).

## Root cause
A new always-run step (`host_converge`) was added to the main flow without
auditing every early exit already above it. `--check`'s contract and the
FAILED-latch's placement both predate the new step and neither was revisited
when it was inserted.

## Fix
`host_converge` moved to run first, ahead of both the FAILED-latch and the
"already up to date"/`--check` exits, with `--check` now reporting only.
Working tree at review time; ships in the Phase 1 checkpoint commit.

## Prevention
Before adding a step to a script's main flow that is meant to run "every
tick"/unconditionally, list every early exit already above the insertion
point and decide its relationship to each one explicitly — don't just append.
A flag's documented contract (here, "`--check` changes nothing") needs a test
that exercises it against every code path added afterward, not just at
introduction.

## Related
BUG-009 (verbatim recurrence of a related root-script hardening finding in
the same phase); review findings 1-2.
