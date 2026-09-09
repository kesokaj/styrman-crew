---
owner: main
date: 2026-09-09
---
# Probe — does the bridge interface prefix alone decide LAN reach?

Run before writing any code for the ADR-067 plan
(`plans/2026-09-09-lan-mode-by-bridge-prefix.md`), to test its single
load-bearing assumption on the real host rather than reason about it. Two
throwaway Docker networks, no styrman code involved, no source file changed.
Both networks and the probe container were removed by exact name afterwards
(BUG-006); `docker network ls` and `/sys/class/net` confirmed clean.

Host: this machine, Docker 29.7.2, uplink `enp114s0`, gateway `192.168.86.1`,
host address `192.168.86.58`. The root firewall service (v4) was started before
each measurement so the base rules were live.

## Setup

```
docker network create --driver bridge -o com.docker.network.bridge.name=lan-probe0 styrman-probe-lan
docker network create --driver bridge -o com.docker.network.bridge.name=sdb-probe0 styrman-probe-sdb
```

The ONLY difference between them is the four-character interface prefix.

## Result 1 — the prefix is the switch

| from bridge | gateway 192.168.86.1 | host 192.168.86.58 | internet 1.1.1.1 |
|---|---|---|---|
| `lan-probe0` | **REACHABLE** | **REACHABLE** | REACHABLE |
| `sdb-probe0` | blocked | blocked | REACHABLE |

`lan-probe0` is "a normal docker container" — it reaches what the host reaches.
`sdb-probe0` is "sandbox" — internet and nothing else. Neither required a rule
to be added, removed, or asked for: `-i sdb-+` in `INPUT` and `DOCKER-USER`
simply does not match `lan-probe0`.

## Result 2 — R12's cross-project guarantee survives it

A victim container was run on `styrman-probe-sdb` at `172.21.0.2` listening on
TCP 9999, and attacked from `styrman-probe-lan`:

| attack | result |
|---|---|
| ICMP to the other project's container | blocked |
| TCP 9999 to the other project's container | blocked |
| the same, with `--cap-add NET_ADMIN` and a manual route for `172.21.0.0/16` via the bridge gateway (the attack R12 names by name) | blocked |

Docker 29.7.2 writes one rule per bridge and it is what enforces this:

```
-A DOCKER ! -i sdb-probe0 -o sdb-probe0 -j DROP
```

A new flow entering a bridge from anywhere but that bridge is dropped, whatever
the source bridge is called. Cross-project isolation is therefore independent
of the styrman rules and of the mode — which is the fact the plan's Risk 2
rests on, now measured rather than assumed.

## What this does NOT establish

- It was measured with plain Docker containers, not Kata microVMs on the `mvm`
  runtime. Phase 5 repeats every check against two real projects.
- It says nothing about a Docker version other than 29.7.2. The per-bridge DROP
  is the load-bearing rule; a major Docker upgrade must re-run this probe.
- The manual-route step reported the route as already present (the container's
  default route already pointed at the bridge gateway, so the traffic did reach
  the host and was dropped there). The meaningful result is that it was
  dropped; a Phase 5 repeat should use a subnet not covered by the default
  route to make the added route observably the path under test.
