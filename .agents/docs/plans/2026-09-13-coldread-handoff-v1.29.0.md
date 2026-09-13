---
owner: explorer
date: 2026-09-13
---
# Recon — cold-read of v1.29.0 handoff/ROADMAP/plan/changes/bugs   (2026-09-13)

## Question
Resuming from AGENTS.md → TODO.md → newest handoff → ROADMAP "Now" → the
named active plan → three newest changes/ → bugs/INDEX.md: what is the
status, what's missing on disk, what contradicts, what would still have to
be guessed?

## Summary
The trail is internally consistent and DONE: all boxes in
`.agents/docs/plans/2026-09-13-browser-review-fixes.md` are ticked except
one unchecked "Definition of Done" line item that is itself just a
checklist restating verifier/reviewer/documentation/changelog/bugs/preflight
— all of which the handoff and changes/ show as completed and closed, so
this is a stale checkbox, not open work. Every file path named in the
handoff, ROADMAP "Now", and the release entry exists on disk. Every commit
SHA named in the handoff (c4128f5, 211b2f8, 2a501ae, bac799f, 1318e79,
b6a287d, b6bc2bf, 855bbc3) exists in `git log` with matching descriptions;
tag `v1.29.0` (annotated, object 3fcda73) points at `b6bc2bf` as the handoff
claims. TODO.md's three open Inbox items match the handoff's "deferred, not
built" list word-for-word (Stop control, `liveUnknown` flag, harness
re-seed cap). No contradiction found in versions, SHAs, or open/closed
status between handoff, ROADMAP, TODO.md, and the plan.

## (1) Status / next step, as the trail states them
v1.29.0 is shipped and deployed (auto-updater tick 2026-09-13 10:24,
`styrman-ctl status` verdict OK, tag SSH-signed and verified). **NON-BLOCKING**:
the plan's one unticked box is Definition-of-Done bookkeeping already
satisfied by artifacts named in the handoff — no real remaining task in the
agent's queue. Next step per the handoff: two operator-only checks (fresh
sign-in shows 1.29.0; open a project and confirm `styrman-runtime:1.29.0`
boot) plus the older v1.28.0 manual Browser-view walk — none blocking, all
explicitly "the operator's, not blocking" in the handoff. New work starts
from TODO.md's three Inbox items or a fresh request.

## (2) Named paths not on disk
None. Checked every artifact path named in the handoff (gate report,
release entry, task change entry, ADR-068 amendment reference, BUG-042..053,
verifier logs, tier logs, review + security + 3 cold-read rounds, e2e
scratch log) plus ROADMAP/plan references — all exist. **NON-BLOCKING.**

## (3) Contradictions
None found. Test-count deltas across documents (Phase 1 log: integration
90/0/3; final testing README: 91/0/3) reflect later rounds adding a check,
not a discrepancy — the plan's own phase entries explain each count's
provenance. **NON-BLOCKING** (noted only in case a future reader assumes a
copy error).

## (4) What still has to be guessed
- Whether the plan's single unticked DoD box was left unticked deliberately
  (as a residual checklist artifact) or is an oversight — the handoff and
  changes/ show the work done, but nobody flipped the box. **NON-BLOCKING**,
  cosmetic.
- Whether the operator has performed the two operator-only post-deploy
  checks (fresh sign-in version, project boot on 1.29.0) — not recorded
  anywhere in the trail; the handoff explicitly defers them to the operator
  and does not claim they were done. **NON-BLOCKING** per the handoff's own
  framing, but genuinely unknown from the code/docs alone.
- The `/retro` suggested in the handoff (changelog inventing evidence twice;
  a harness OOM-kill during a green build) has not been run — no artifact
  found. **NON-BLOCKING**, queued but not required for "done".
