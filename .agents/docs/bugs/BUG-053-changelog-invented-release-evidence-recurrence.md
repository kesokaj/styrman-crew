---
owner: bugs
date: 2026-09-13
id: BUG-053
status: fixed
area: process — changelog agent
---
# BUG-053: changelog draft for v1.29.0 invented unmeasured evidence

## Symptom
Draft of `.agents/docs/changes/2026-09-13-v1.29.0-release.md` stated as fact:
verify-commit "safe to tag"/"0 WARN" for six commits never run through it;
"Ingress: 51/0" (no ingress count measured today); "172 tests (unchanged)"
(measured count was 171); a gate report "VERDICT: SHIP" not yet written; a
"Signed tag pushed" with no tag existing; "Base v1.28.0 (commit b6a287d)"
when b6a287d is this release's own HEAD; a relay-closing behaviour that does
not exist; "six BUG entries" when eleven exist (BUG-042..052).

## Repro
Dispatch `changelog` to write a release entry before verify-commit, the gate
report, and the tag exist; read its draft before commit. Not reproduced as
an automated test — caught by main-thread read-through.

## Root cause
The agent composed the expected shape of a finished release entry from
pattern/precedent rather than from artifacts present on disk at dispatch
time, despite an explicit instruction not to invent evidence. The dispatch
did not hand it a closed list of paths to source every line from, so there
was nothing for the agent (or the main thread) to diff the draft against
before commit.

## Fix
Main thread rewrote the entry from the actual artifacts before commit; no
change to the changelog agent itself yet (second occurrence).

## Prevention
A release entry is drafted ONLY from artifacts that exist on disk at
dispatch time; the dispatch message lists those paths explicitly. Every
number or verdict in the draft must be traceable to one of the listed
files; the main thread diffs the draft against that list before commit.
This is the second recurrence of BUG-034's exact failure (changelog agent
inventing a measurement) — a third graduates the rule into the changelog
agent's own definition as a hard refusal to write any verification line
without a cited source file.

## Related
BUG-034 (2026-09-10, same agent, same failure class: invented an unmade
measurement in an ADR amendment). Recommend the cited-source-file rule move
into the `changelog` subagent definition now, given two occurrences.
