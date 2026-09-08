---
owner: bugs
date: 2026-09-08
id: BUG-020
status: fixed
area: web/src/components/LocalNetwork.tsx (ADR-065 LAN switch UI)
---
# BUG-020: LAN switch pinned to stale optimistic state after an out-of-band change

## Symptom
After the per-project LAN access record changed OUT OF BAND — another
browser tab, another admin, or a direct API call — the Local network block
kept rendering the state from its own last PUT reply indefinitely, even
though the 5 s poll returned the true, newer state. The switch claimed the
project was still LAN-enabled after it had been disabled elsewhere.

## Repro
Enable/disable LAN access through the API for a project whose UI is open in
a browser that had itself just made a different LAN change (so it holds an
optimistic override); the switch and the "SSH key is withheld" note never
update to match the new server state. Caught by a new ADR-066 browser e2e
check that disables LAN access via the API and waits for the note to
disappear — it never did.

## Root cause
The component chose between its optimistic override and the poll's data
with an EQUALITY test on the change timestamp:

    const view = override && data?.lanChangedAt !== override.lanChangedAt
      ? override : data;

The intent was "show my own reply until the poll catches up with it".
Equality only holds while the server's newest change IS the one this
component made. Any later out-of-band change moves `data.lanChangedAt`
PAST `override.lanChangedAt`, the inequality stays true forever, and the
component pins itself to the stale reply for as long as it stays mounted.
It appeared to self-heal in ordinary use only because the next change
usually also went through this same component, installing a fresh override.

## Fix
Extracted the comparison to a pure, unit-tested helper `preferOverride` in
`web/src/lib/lan.ts`, changed from "timestamps are equal" to "the poll's
change is no older than the override's" (`Date.parse` ordering, falling
back to the old equality test when either timestamp is missing or
unparseable). Five vitest cases cover it, including the out-of-band case.
The ADR-066 e2e check now passes.

## Prevention
An optimistic-UI override must be discarded when the server's state is NO
OLDER than the override, never only when it is IDENTICAL to it — an
equality test on a monotonic timestamp latches forever the moment anything
else advances that timestamp. More generally: any "show my own write until
the poll catches up" pattern needs an ordering comparison, not an equality
one, and needs a test that changes the state from OUTSIDE the component.

## Related
BUG-014, BUG-018 (same LAN access feature, same family of "server's last
word vs. locally-held state" mistakes). Three related mistakes in this
feature now share a shape — deriving UI/derived state from a value that can
be advanced by more than one writer without an ordering/authority check.
Candidate for a permanent rule in `AGENTS.md` or `.agents/rules/` on
optimistic-UI patterns generally.
