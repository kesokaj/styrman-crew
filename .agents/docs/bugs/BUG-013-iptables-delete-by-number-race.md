---
owner: bugs
date: 2026-09-08
id: BUG-013
status: fixed
area: deploy/firewall (styrman-firewall.sh)
---
# BUG-013: iptables rules deleted by number after a separate `-S` read could hit the wrong rule under a concurrent root writer

## Symptom
Security review (`.agents/docs/reviews/2026-09-08-security-lan-phase1.md`,
finding 4, medium): `deploy/firewall/styrman-firewall.sh:299-334` selected
rules by number from one `iptables -S` read (`:320`) and deleted them by
number in separate `iptables -D STYRMAN-FWD "${n}"` calls (`:331`). The
script's flock only excludes other runs of itself; any other root writer of
`STYRMAN-FWD` between the read and a delete (an operator hand-run,
`iptables-restore`, a firewall-manager reload) shifts the numbering, so the
delete removes the neighbour instead — which at the head of the chain is a
base `-j DROP`, opening cross-project/LAN/metadata reachability until the
next tick, silently (the delete still returns 0).

## Repro
Not exploited live; race window reasoned from the read/delete split by the
security reviewer.

## Root cause
Deletion used positional numbering captured at read time instead of a
content match, so the operation was only correct under an assumption (no
other root writer touches the chain between read and delete) that the
script itself cannot enforce.

## Fix
Delete by full rule spec (`-D STYRMAN-FWD -s .. -d .. -i .. -o .. -m comment
--comment .. -j RETURN`), reconstructed from `RULE_RE`'s captures, which
cannot hit the wrong rule; the numbered-delete path kept only for genuinely
malformed lines, re-read to confirm the line still matches before firing.

## Prevention
When deleting from a mutable, externally-writable list (iptables chain,
any ordered resource another process can also modify) by a position
captured in an earlier read, treat the position as stale by the time of the
delete — delete by full content match instead, and reserve
position-based deletion for cases already known malformed/unmatched.

## Related
Security review 2026-09-08; BUG-012 (same review, same script).
