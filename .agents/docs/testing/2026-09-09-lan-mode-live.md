---
owner: main
date: 2026-09-09
---
# Live adversarial run — ADR-067 network mode on real hardware

The plan (`plans/2026-09-09-lan-mode-by-bridge-prefix.md`) calls this "the
gate for the whole plan": the one thing ADR-065/066 never had. Those shipped
twice and **were never once applied on real hardware**. This is the run that
was missing.

Real Kata, this host, 2026-09-09. Two projects on a throwaway server
(`STYRMAN_STATE_DIR` under a `mktemp` dir, port 18094, image
`styrman-runtime:test` rebuilt today) so production was never touched.
Production's own project (`e441b4b6`, "test123") stayed stopped throughout and
was still the only survivor at the end.

Host facts, read rather than assumed:

    default via 192.168.5.1 dev enp114s0 src 192.168.5.10
    LAN peer used: 192.168.5.131 (host reaches it — verified, so there is a
                   real signal to invert)

The plan's text names `192.168.86.1` as the gateway; that was illustrative.
The real gateway on this host is `192.168.5.1`.

## Step 1 — two projects, one per mode

| project | id | bridge (sandbox) | bridge (LAN) |
|---|---|---|---|
| A | `c050d62e-…` | `sdb-5c5ad5b539a` | `lan-5c5ad5b539a` |
| B | `8f4de5ef-…` | `sdb-e242c838b30` | — (stayed sandbox) |

**The suffix is unchanged across the flip.** `sdb-5c5ad5b539a` ->
`lan-5c5ad5b539a` -> `sdb-5c5ad5b539a`: only the four-character prefix moves,
which is the whole design claim and the thing BUG-025's fixture had been
checking wrongly.

## Step 2 — reachability, before and after

`docker exec styrman-<id> ping -c1 -W2 <ip>`:

| target | A sandbox | A **LAN** | B sandbox (throughout) |
|---|---|---|---|
| `192.168.5.1` gateway | blocked | **REACHABLE** | blocked |
| `192.168.5.10` host uplink | blocked | **REACHABLE** | blocked |
| `192.168.5.131` LAN peer | blocked | **REACHABLE** | blocked |
| `1.1.1.1` internet | REACHABLE | REACHABLE | REACHABLE |

Sandbox is internet-only. LAN is an ordinary machine on the network. That is
the operator's request — "give the option to enable local lan access as normal
docker container have" — met, and met without a line of root-mediated
machinery.

## Step 3 — cross-project isolation, including the attack R12 names

A was in LAN mode for all of this. B's container `172.21.0.2`, B's subnet
`172.21.0.0/16`, B's bridge gateway `172.21.0.1`. A's container `172.20.0.2`.

    A -> 172.21.0.2  (B's container)       blocked
    A -> 172.21.0.1  (host on B's bridge)  REACHABLE   <- see Finding below

The attack: the guest **does** hold `cap_net_admin` (confirmed with `capsh
--print`), so the manual route R12 names is actually available:

    # docker exec styrman-<A> ip route add 172.21.0.0/16 via 172.20.0.1 dev eth0
    # docker exec styrman-<A> ip route
    172.21.0.0/16 via 172.20.0.1 dev eth0        <- route installed
    # docker exec styrman-<A> ping -c1 -W3 172.21.0.2
    RESULT: still blocked

**R12 holds.** The route installs, the host has `ip_forward=1`, and the packet
still dies — because the rule that stops it is Docker's, not styrman's, and it
keys on the interface rather than on any address the guest can influence:

    -A DOCKER ! -i sdb-e242c838b30 -o sdb-e242c838b30 -j DROP
    -A DOCKER ! -i lan-5c5ad5b539a -o lan-5c5ad5b539a -j DROP

One per bridge, written for the `lan-` bridge exactly as for the `sdb-` one.
This is the plan's second load-bearing claim, now measured under attack rather
than read off a rules dump.

And styrman's own jumps, confirming the other half:

    -A INPUT       -i sdb-+ -j STYRMAN-IN
    -A DOCKER-USER -i sdb-+ -j STYRMAN-FWD

Rules in a `STYRMAN-*` chain matching `-i lan-`: **0**.

## Step 4 — B unchanged

Every B row in the Step 2 table is identical before A's flip, while A was in
LAN mode, and after A returned to sandbox. B was never restarted and its
bridge name never changed.

## Step 5 — the host's chains are as we found them

`iptables -S` captured before the run and after teardown: **identical**, byte
for byte (38 rules). The per-bridge Docker rules appear and disappear with the
networks; nothing styrman writes persists, because under ADR-067 styrman
writes nothing. No `styrman-lan:*`, `styrman-lanhost:*` or `styrman-lanpub:*`
rule existed at any point.

## Finding — a LAN project reaches the host on OTHER projects' bridges

Not a step of the plan; found by running it. `A -> 172.21.0.1` succeeds, and
`172.21.0.1` is the host's address on **B's** bridge.

This is a direct consequence of what LAN mode means: the `sdb-+` wildcard in
`INPUT -i sdb-+ -j STYRMAN-IN` no longer matches, so the host is reachable
from a LAN project — and "the host" is *every* address the host holds, not
only its LAN uplink. ADR-067 states that a LAN project reaches "this server
including its SSH service"; it does not state that this includes the host's
addresses on other projects' bridge interfaces. B's *container* stays
unreachable, so R12's cross-project clause is intact. But the surface is wider
than the ADR's wording implies, and it should be written down rather than
discovered later.

Why it matters enough to flag: the credential broker listens on `:8091` and
identifies its caller **by source IP** (`internal/credbroker/broker.go:62`,
`Orchestrator.ResolveProjectByIP`). The credentials it serves are per-USER
(`store.GetGit(user, …)`, `store.AllEnv(user)`), so a project reaching a
different bridge's broker address still resolves to its own caller identity
and gets its own user's secrets — no crossing on the same host today. The
question worth a security reviewer's eye is the cross-user case combined with
`cap_net_admin` in the guest, and the fact that ADR-067 **deleted** the SSH-key
withholding latch that ADR-066 had added for adjacent reasons (operator
decision, "Delete it").

**Not tested here:** the broker was absent from this run. It only starts when
`cfg.SelfContainer != ""` (`cmd/styrman/main.go:121`), i.e. only when the
server itself runs containerized, and this run used a host-process server. So
no claim is made about broker reachability from a LAN project — it was not
exercised, and must not be read as having passed. Phase 6's `security` agent
should take this as an explicit question against the compose deployment.

## Verdict

Steps 1-5 pass. Sandbox behaviour is unchanged, LAN behaviour is what the
operator asked for, cross-project isolation survives a `NET_ADMIN` attack, and
the host's firewall is untouched by the whole exercise.

Outstanding against this gate: the browser criteria walked by hand (the plan
assigns those to the operator), and the broker question above.
