---
owner: main
date: 2026-09-08
---

# LAN parity Phase 3: host-published Docker ports, with the ingress excluded by allowlist   (2026-09-08)

Plan: `.agents/docs/plans/2026-09-08-lan-full-parity.md` (Phase 3, 5 of 6
boxes; the sixth is written but needs an opted-in host to run). ADR-066 part
(3), second half.

## The rule, and why it is shaped this way

```
-A STYRMAN-FWD -s <bridge subnet> -d <allowed bridge subnet> -i sdb-<hash> \
   -m conntrack --ctstate DNAT \
   -m comment --comment "styrman-lanpub:<uuid>" -j RETURN
```

`--ctstate DNAT` matches only connections the host redirected from a
published port, so the rule cannot be used to reach an arbitrary address in
the destination bridge's subnet — it is "the things this host publishes",
not "that whole bridge".

The destination is a **positive allowlist** of bridge subnets: every Docker
bridge that is neither a project bridge nor the configured ingress. This is
not a style preference. `TODO.md` had recorded the exclusion as
`! -o sdb-+ ! -o <ingress br>` since the v1.23.0 security review, and that
rule cannot be written — iptables accepts one `--out-interface` per rule:

```
iptables v1.8.11 (nf_tables): multiple --out-interface options not allowed
```

An allowlist also fails closed by construction, where a negation fails open
when it is wrong.

## INGRESS_BRIDGE is derived, never typed

That value is a safety exclusion: naming the wrong bridge would leave the
real ingress inside the reachable set, the opposite of the key's purpose.
Three things stand between it and a mistake, none of them "trust the
operator":

- **install.sh derives it** — `br-` + the first 12 hex of the
  `styrman-ingress` network id — and `die`s if it cannot, exactly as it
  already does for `LAN_DEV`. `--ingress-bridge` overrides for a host without
  a docker CLI.
- **A stale value fails closed by itself.** Docker derives the bridge name
  from the network id, so a recreated network leaves a name that no longer
  exists; the existence check then refuses the whole family.
- **Root reports what it used** (`ingress_bridge=` in the status file) so the
  server — which, unlike a root script that must not depend on the docker
  daemon, knows its own ingress network — can compare and flag a
  wrong-but-valid value. That comparison is Phase 5.

A missing, malformed or non-existent `INGRESS_BRIDGE` skips **only** the
published-port family, with a warn naming the reason; it does not take the
basic LAN allowance down with it. A malformed `LAN_ALLOW_PUBLISHED_PORTS`
value is a grammar error and does close everything, consistent with the other
keys.

## Verified on the real host, without granting anything

Run against the live chains with a scratch state dir and no markers, so no
rule was inserted:

```
lan_host_addrs=192.168.86.58/32
ingress_bridge=br-02a045873a1d
lan_pub_subnets=172.17.0.0/16
```

docker0 only. The ingress (`172.18.0.0/16`) and the live project bridge
(`172.19.0.0/16`) are both excluded, from root's own routing view.
`STYRMAN-FWD` and `STYRMAN-IN` byte-identical before and after.

## Verification

- `deploy/firewall/test.sh` **120 / 0**, up from 102: +18 checks. The new
  sections build a stand-in Docker topology inside the namespace — `docker0`
  as the published-container bridge and `br-0123456789ab` as the ingress —
  and assert the allowance reaches the first and never the second, never a
  project bridge, sits ahead of the DROPs, is idempotent, and disappears when
  the key is turned off, the bridge is stale, or the marker goes.
- `go vet ./...`, `go test ./...`, ingress module: clean.

## integration.sh: the live half

The live block's assertions now take their expectations from root's own
status file rather than assuming what a deployment chose. On a host that has
not opted in it asserts the host stays unreachable AND that no host allowance
exists; on an opted-in host it asserts the opposite. The published-port check
starts a real container with `docker run -p` and fetches through it from
inside the guest — the check the v1.23.0 handoff recorded as missing ("the
published-port DNAT shape is reasoned, not observed"). It skips with a reason
where the conf has not opted in, which is the case on this host today.

## Section 13 of test.sh changed meaning

It asserted that a reserved key is parsed as 0 and ignored. ADR-066 turned
both formerly-reserved keys real, so nothing is reserved any more. It now
asserts the property that still matters: a key outside the grammar is a
`conf_error` that closes every allowance, rather than something silently
skipped.
