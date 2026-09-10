---
owner: bugs
date: 2026-09-10
id: BUG-034
status: fixed
area: process — plan/changelog claims (TODO close-out)
---
# BUG-034: plan and changelog asserted facts about code nobody had read or measured

## Symptom
Round 1 of the cold-reader check on `.agents/docs/plans/2026-09-10-todo-closeout.md`
found the plan asserting: a `deleteError` is persisted in the store (it is
in-memory only); an `AppHost` config field that does not exist; and "the same
line scan `toolDetails` uses" for a check that in fact ignores sections. Separately,
the `changelog` agent's ADR-067 amendment stated "measurement confirms Docker
preserves a pre-existing DOCKER-USER chain" — no such measurement was made or
logged anywhere. Caught by main before commit and rewritten to the two facts
actually established.

## Repro
Not reproduced as a test; caught by cold-reader round 1 (plan) and by main's
read of the changelog draft before commit (ADR-067 amendment).

## Root cause
Both are the same mechanism: a claim about code or system behavior was
written from inference or a plausible-sounding restatement rather than from
reading the source or running a measurement, and nothing downstream forced
the writer to cite where the claim came from.

## Fix
Plan corrected during cold-reader round 1 (same day, uncommitted draft).
Changelog ADR-067 entry rewritten to the two verified facts: restart policy
`unless-stopped`, and the script inserts into a chain only Docker creates.

## Prevention
A record of a decision names its evidence by file:line or a command run;
"measurement confirms" (or any claim about code/runtime behavior) without a
citable source is a claim to strike, not to soften. Applies to plans,
changelog entries, and ADR amendments alike.

## Related
Same family as BUG-005/006/007 (`.agents/rules/writing-files.md`): trust what
the tool/source shows, not what was inferred. BUG-035 and BUG-036 are the
other two classes found in this same TODO close-out; see also BUG-033.
