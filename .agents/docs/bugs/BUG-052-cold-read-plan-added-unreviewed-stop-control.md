---
owner: bugs
date: 2026-09-13
id: BUG-052
status: fixed
area: process — plan scope (2026-09-13-browser-review-fixes)
---
# BUG-052: review-fix plan included a new Stop control beyond the review's findings

## Symptom
Cold-read of the plan took three rounds (12 → 7 → 7 items) and hit the cap.
Round 1's blocking item was not a review finding at all: the author had
added a new Stop control that would kill the browser the agents share.

## Repro
See .agents/docs/reviews/2026-09-13-coldread-browser-review-fixes*.md, round 1.

## Root cause
The plan scope was not limited to the review's findings; a new user-facing
verb was folded in as if it were a fix, without its own scope gate or
design review — so the cold-read had to catch a feature-shaped change
disguised as a bugfix.

## Fix
The Stop control item was struck from the plan; the plan was reduced to the
review's findings only.

## Prevention
A review-fix plan contains the review's findings and nothing else new — a
new user-facing verb (or control) is a feature with its own scope
confirmation gate, never bundled into a fix-the-findings plan.

## Related
Cold-read process itself (three rounds against the cap) — if this recurs,
the "review-fix plan scope" rule should graduate to a checklist item in the
`feature`/`bug` skill or a `.agents/rules/` file.
