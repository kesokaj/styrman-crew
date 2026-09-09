---
owner: main
date: 2026-09-09
---
# Cold-reader check — 2026-09-09 handoff (v1.25.0 shipped)

## A. Status and single next step

- Handoff: "Tag `v1.25.0` on release commit `6e73b43`, pushed and deployed."
  Next Steps: "1. Operator: the browser walk-through; decide on sshd; run
  `install.sh` once to clear the retired units. 2. Next task on this code:
  the record-first restructure (TODO), with the broker test alongside it."
- ROADMAP.md "Now" gives a DIFFERENT next step, but flags itself as stale:
  "The paragraphs below were written while it was pending and are kept as
  the record of how it got there; the handoff … is current." Read literally
  (ignoring that caveat) it still says "Nothing is tagged or deployed yet"
  and "Phase 6 is in progress" — a cold reader who skips the caveat sentence
  gets the wrong status. Two "next steps" exist in the trail; only the
  handoff's is current.
- Plan's one unticked box (`plans/2026-09-09-lan-mode-by-bridge-prefix.md`,
  Phase 5): "The browser E2E criteria walked by hand — verify: **user**" —
  matches handoff Next Step 1's "browser walk-through."

## B. Named files that do not exist

None. Checked and confirmed present: `reviews/2026-09-09-preflight-v1.25.0.md`,
`changes/2026-09-09-v1.25.0-release.md`,
`changes/2026-09-09-adr-067-preflight-fixes.md`,
`reviews/2026-09-09-security-adr-067.md`,
`testing/2026-09-09-lan-mode-live.md`,
`testing/2026-09-09-lan-prefix-probe.md`, `bugs/BUG-028` through `BUG-031`,
and the plan file itself. All under `.agents/docs/`.

## C. Contradictions

- ROADMAP.md "Now" text vs. handoff/plan: ROADMAP's body literally states
  "Nothing is tagged or deployed yet" and "production remains on v1.24.1" —
  both false as of the handoff (tagged, deployed, production is v1.25.0).
  Not a real contradiction — ROADMAP explicitly labels that body stale and
  defers to the handoff — but it is the one place a cold reader could be
  misled if they stop reading after the first sentence.
- No other contradiction found: plan box count (43 checked / 1 unticked)
  matches the handoff's "43 of 44" claim exactly (verified by grep). TODO.md's
  four newest entries (network-mode PUT rollback, mode-toggle race,
  `unmarkIfStillRunning`, vulncheck status) match the handoff's Open items
  5, 6, and 8 one-for-one in substance.

## D. What still requires a guess

- Whether "run `install.sh` once to clear the retired units" (handoff Next
  Step 1c) is meant for the agent or the operator — handoff Open item 3 says
  "Operator's call" contextually but the imperative sentence in Next Steps
  doesn't restate the owner.
- Exact scope/order of "the record-first restructure" as the stated next
  code task — TODO.md's matching item describes the target design (persist
  only after rebuild succeeds, drop rollback on tightening) but not a
  concrete task breakdown or acceptance test list; would need to be turned
  into a plan before starting.
- Whether BUG-028/029/031's fixes and the "broker reachability test" (Open
  item 4, "owed") are the same piece of work as the record-first restructure
  or a separate follow-up — the handoff says "with the broker test alongside
  it" (Next Step 2) suggesting same PR, but doesn't confirm.
