---
owner: main
date: 2026-09-08
---

# LAN access: the operator flipped the switch — last unautomated check closed   (2026-09-08)

ADR-065 shipped in v1.23.0 with exactly one step no test could cover: a
human turning the Ports-tab switch on in a browser and looking at the
result. The operator ran it. It behaved, in both directions.

Closes Next Step 1 of the handoff
`.agents/docs/handoffs/2026-09-08-v1.23.0-shipped-and-proven.md`.

## Reported by the operator (browser + guest shell, not observed here)

| Switch | Target | Result |
|---|---|---|
| ON | `192.168.86.1` (LAN gateway) | reachable |
| ON | `192.168.86.58` (the host itself) | **not** reachable |
| OFF | `192.168.86.1` | not reachable |

The first and third rows are the feature working: the head-of-chain RETURN
appears and matches, and without it `-A STYRMAN-FWD -d 192.168.0.0/16 -j
DROP` catches the same packet. This is the first confirmation of the
allowance through the real UI path — every prior proof drove the marker
file directly (`testing/2026-09-08-verify-lan-live.md`).

The second row is also correct, and was the operator's question. The
allowance is a FORWARD-path rule; a packet addressed to the host's own LAN
address is delivered locally and never reaches `STYRMAN-FWD` at all. It hits
`INPUT`, which the LAN feature does not touch. ADR-065 part (5) names the
host first in what stays closed for a LAN-enabled project, and R12 carries
the same sentence. `integration.sh` asserts it in both marker states.

## Host state read first-hand afterwards (LAN already switched back off)

```
$ sudo iptables -S INPUT | grep sdb
-A INPUT -i sdb-+ -j STYRMAN-IN
$ sudo iptables -S STYRMAN-IN
-N STYRMAN-IN
-A STYRMAN-IN -j DROP
$ sudo iptables -S STYRMAN-FWD          # six DROPs, no styrman-lan: rules
$ sudo cat /run/styrman-firewall/status
schema=1  version=3  lan_dev=enp114s0  lan_cidrs=192.168.86.0/24
lan=      spool=0    run_ms=84
$ sudo ls /var/lib/styrman/netpolicy/lan/     # empty
```

Consistent with the switch having been turned back off: no markers, empty
`lan=`, no tagged rules, spool drained. The teardown path left nothing
behind, which is the fourth assertion of the live block reproduced by hand.

## Scope note

Written directly rather than by the `changelog` agent: the facts are in the
session transcript, which a subagent does not receive, and that agent
fabricated content in four entries on 2026-09-08 (handoff, "Agent notes").
Every claim above is either labelled as the operator's report or was read
from this host in this session.

## Follow-on

The operator's stated intent for the switch has changed as a result — ON
should mean full parity with a host on the same LAN, including the host
itself. That is a trajectory change against R12 and needs the
propose/approve/ADR path, not an edit here. Nothing in this entry
anticipates it.
