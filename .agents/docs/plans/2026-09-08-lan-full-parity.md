---
owner: main
date: 2026-09-08
---
# Plan — LAN access: full same-network parity   (2026-09-08)

## Why

ADR-065 shipped a per-project LAN switch whose ON state means "the LAN,
minus the host, minus host-published ports". The operator flipped it for the
first time on 2026-09-08, found the host unreachable, and restated the
intent (their words): *"If lan on it is like they are on the exactly same
network with full access."* The OFF state is confirmed correct and does not
change.

Two decisions were taken in that conversation and are inputs here, not
questions:

1. **Full OUTBOUND parity.** A LAN-enabled project reaches everything a peer
   on `192.168.86.0/24` reaches, host and host-published ports included. It
   still NATs out as the host and there is still no inbound path into the
   VM. True L2 membership (macvlan, own DHCP lease, inbound reachability)
   was explicitly rejected: it breaks the `sdb-*` naming every R12 rule keys
   on, breaks the broker's bridge-only listener, and would make two LAN-on
   projects L2 peers of each other — contradicting R12's *unconditional*
   cross-project clause.
2. **The host's sshd is allowed; the host is hardened separately.** See
   Risks — this is a deployment precondition the operator runs, not code.

## Correction this plan carries

The conversation that produced it opened with a claim that the brokered SSH
key makes a LAN-enabled project root on this host. **That is false on this
host** and the record should not repeat it. Verified 2026-09-08:

- `/home/kesokaj/.ssh/authorized_keys` and `/root/.ssh/authorized_keys` do
  not exist. The brokered key opens other hosts that trust it, not this one.
- The actual exposure `LAN_ALLOW_HOST` opens is `sshd` on `0.0.0.0:22` with
  `passwordauthentication yes`, combined with
  `/etc/sudoers:50 %sudo ALL=(ALL:ALL) NOPASSWD:ALL` — a credential-guessing
  surface whose success is root, not instant root.

The broker narrowing below is therefore justified by the TRANSITIVE path
(ADR-065 follow-up (g): LAN-enabled project → a LAN host that trusts the
operator's key → back here), which is real, and not by host root.

## Scope

**In:**
- `LAN_ALLOW_HOST` — per-project RETURN in `STYRMAN-IN` for the host's own
  address on `LAN_DEV`.
- `LAN_ALLOW_PUBLISHED_PORTS` — per-project RETURN in `STYRMAN-FWD` for
  DNATed traffic to a non-project, non-ingress Docker bridge.
- Both conf keys move from reserved-parsed-as-0 to real, **absent still
  meaning 0** so an existing host does not silently gain host access on
  upgrade.
- `STYRMAN_FW_VERSION` 3 → 4 and `netpolicy.MinScriptVersion` 3 → 4.
- Status file reports the two new capabilities so the server can tell a user
  "the host conf has not opted in" instead of showing a switch that no-ops.
- Broker withholds `~/.ssh/id_ed25519` from a LAN-enabled project and the
  server prunes the materialized copy on enable.
- UI: the enable path becomes an explicit confirmation naming both
  consequences.
- ADR-066; R12 amendment (exact text below, operator approval required
  before it is committed).

**Out:**
- L2 membership / inbound LAN → project. Rejected above.
- Reaching other private ranges (10/8, tailnet, metadata). "Same network"
  means `LAN_CIDRS`, nothing wider.
- Cross-project reachability. Unconditional in R12 and stays so.
- Hardening the host's sshd. Operator action, documented, not automated.

**Follow-ups noted, not built:** the `lanPolicy = admins|owners|off` knob
(ADR-065 (a)); per-project LAN DNS (c); `default-address-pools` pinning (d).

## Surfaces touched

| Path | Change |
|---|---|
| `deploy/firewall/styrman-firewall.sh` | two new rule families, two new conf keys, host-address discovery, allowed-bridge discovery, version 4 |
| `deploy/firewall/install.sh` | writes `LAN_ALLOW_HOST=1`, `LAN_ALLOW_PUBLISHED_PORTS=1`, `INGRESS_BRIDGE=`; `--ingress-bridge` flag |
| `deploy/firewall/test.sh` | new cases per Verification |
| `internal/netpolicy/netpolicy.go` | `MinScriptVersion` 4 |
| `internal/netpolicy/status.go` | parse `lan_allow_host`, `lan_allow_published_ports`; surface in `Status` |
| `internal/netpolicy/dir.go` | no change expected — confirm |
| `internal/credbroker/broker.go` | `LanChecker`; `files` filters ssh-shaped credentials |
| `internal/httpapi/network.go` | prune materialized key on enable; expose capability + impact in `lanView` |
| `internal/httpapi/setup.go` (broker wiring) | pass the checker into `credbroker.New` |
| `web/src/lib/api.ts`, `components/LocalNetwork.tsx`, `lib/lan.ts` | new fields, confirmation copy |
| `.agents/docs/testing/styrman/integration.sh` | live checks |
| `REQUIREMENTS.md`, `DECISIONS.md`, `specs/styrman/guide.md`, `docs/deploy/remote.md` | R12, ADR-066, guide, sshd precondition |

## Interfaces

### Rule shapes — VERIFIED against iptables 1.8.11 (nf_tables) on this host

The shape TODO item (b) recorded is **impossible** and this plan does not
use it. `! -o sdb-+ ! -o <ingress br>` was probed live and rejected:

```
iptables v1.8.11 (nf_tables): multiple --out-interface options not allowed
```

A rule carries at most one `--out-interface`. The exclusion is therefore
expressed as a **positive destination allowlist**, which also matches the
project's fail-closed style better than a negation:

```
# host (STYRMAN-IN, head), one per host address on LAN_DEV
-s <bridge subnet> -d <host addr>/32 -i sdb-<hash> \
  -m comment --comment "styrman-lanhost:<uuid>" -j RETURN

# published ports (STYRMAN-FWD, head), one per ALLOWED bridge subnet
-s <bridge subnet> -d <allowed bridge subnet> -i sdb-<hash> \
  -m conntrack --ctstate DNAT \
  -m comment --comment "styrman-lanpub:<uuid>" -j RETURN
```

Both were inserted into a throwaway chain and accepted; `--ctstate DNAT`
matches only connections whose reply is NATed, i.e. genuinely published
ports. `-S` renders both as `-s … -d … -i …` (source before destination
before interface) — the two new regexes must match that order, which is
NOT the order the arguments are written in.

**Allowed bridge subnet** = the connected route of every Docker bridge that
is neither `sdb-*` nor `INGRESS_BRIDGE`. Root derives it; the server never
names it. If `INGRESS_BRIDGE` is unset or does not resolve, the
published-port family is **not applied at all** (fail closed) and the status
file says why.

### Conf (`/etc/styrman/firewall.conf`)

```
LAN_DEV=enp114s0
LAN_CIDRS=192.168.86.0/24
LAN_ALLOW_HOST=1                 # absent or 0 = today's behaviour
LAN_ALLOW_PUBLISHED_PORTS=1      # absent or 0 = today's behaviour
INGRESS_BRIDGE=br-02a045873a1d   # required when PUBLISHED_PORTS=1
```

Parser: same grammar as today (the four keys are already accepted and
discarded), values `0`/`1` only, anything else is a `conf_error` that closes
every allowance. `INGRESS_BRIDGE` validated as `br-[0-9a-f]{12}`, must exist,
must not be `sdb-*`.

### Status file additions

```
lan_allow_host=1
lan_allow_published_ports=1
lan_host_addrs=192.168.86.58
```

Additive; `schema` stays 1. **Task: confirm `ParseStatus` ignores unknown
keys** before relying on that (a v4 script's status must not break a v3
server mid-upgrade).

### Go

```go
// internal/credbroker
type LanChecker interface {
    LanEnabled(user, project string) (bool, error)
}
func New(store *credstore.Store, resolve Resolver, lan LanChecker, logger *slog.Logger) *Broker

// withheld when LanEnabled is true OR returns an error (fail closed:
// a transient store error must never be the way a LAN-enabled project
// gets the key). Costs ssh push in that project; GH_TOKEN over HTTPS
// still covers github.com.
func isHostReachingFile(path string) bool   // "~/.ssh/" prefix, tested
```

`lanView` gains `credentialImpact: "ssh-key-withheld"` and
`hostAccess: bool` / `publishedPortAccess: bool` derived from the status
file, so the UI can distinguish "on" from "on, but the host conf has not
opted in".

## Failure modes

| Condition | Behaviour |
|---|---|
| Script v3, server v4 | `Availability` already returns "outdated" → enable 503s with the existing message. `host_converge` converges within a tick. |
| Conf lacks the new keys (upgrade) | Both parse 0. LAN works exactly as v1.23.0. Status says `lan_allow_host=0`; UI shows the switch as on with a "host not included — run install.sh" note. |
| `INGRESS_BRIDGE` unset with `PUBLISHED_PORTS=1` | Published-port family skipped entirely, `conf_error` recorded, host family unaffected. |
| Host address changes (DHCP) | Re-derived every run; the stale rule is not in `DESIRED` and is deleted by spec on the next tick (≤60 s). |
| `LanEnabled` errors | Key withheld, `slog.Error`. Visible as a project that cannot ssh-push. |
| Key already materialized before enable | Pruned from `~/.ssh/id_ed25519` on enable; a copy the guest made elsewhere survives. **Documented, not solved** — enabling LAN on an existing project means that key was exposed to it. |

## Verification

- **Unit (Go):** `netpolicy` status parsing of the three new keys and of an
  unknown key; `credbroker` withholding (on, off, error) and
  `isHostReachingFile`; `httpapi` prune-on-enable through a confined root.
- **Shell (`deploy/firewall/test.sh`):** conf parsing (valid, 0, absent,
  garbage, bad `INGRESS_BRIDGE`); host rule added/removed with the marker;
  published-port rule present only when both keys are set; allowlist
  excludes `sdb-*` and the ingress bridge; delete-by-spec under a concurrent
  writer; `MinScriptVersion` pinned to the script.
- **Integration (`integration.sh`, live Kata):** marked project pings
  `192.168.86.58`; unmarked cannot; **a published port is created for the
  test** (`docker run -p`) and reached from a marked project — this closes
  the "reasoned, not observed" gap the v1.23.0 handoff recorded; a marked
  project's `/files` contains no `~/.ssh` entry.
- **E2E criteria:** in a browser, enable LAN on a project, accept the new
  confirmation, and from that project's terminal `ping -c1 192.168.86.58`
  succeeds and `ls ~/.ssh/id_ed25519` is absent; disable, and the ping fails
  within 5 s. Edge case: with `LAN_ALLOW_HOST=0` in the conf the switch
  still enables but the UI says the host is not included.

## Phases

### Phase 1 — Root: host reachability
- [ ] `styrman-firewall.sh`: parse `LAN_ALLOW_HOST`, discover host addresses on `LAN_DEV`, validate each is inside `LAN_CIDRS` — verify: `test.sh`, AI
- [ ] `styrman-lanhost:` family — collect/scan/reconcile/count, delete by spec (BUG-013) — verify: `test.sh`, AI
- [ ] `STYRMAN_FW_VERSION` 4 + `netpolicy.MinScriptVersion` 4 + pin test — verify: `go test ./internal/netpolicy/`, AI
- [ ] Status keys + `ParseStatus` unknown-key confirmation — verify: unit, AI
- [ ] `install.sh` writes the new keys; `--lan-allow-host` flag — verify: `test.sh` + dry run, AI

### Phase 2 — Root: published ports
- [ ] `INGRESS_BRIDGE` conf key, validation, fail-closed skip — verify: `test.sh`, AI
- [ ] Allowed-bridge-subnet derivation excluding `sdb-*` and the ingress bridge — verify: `test.sh`, AI
- [ ] `styrman-lanpub:` family with `--ctstate DNAT` — verify: `test.sh`, AI
- [ ] Live proof with a real published container — verify: `integration.sh`, both

### Phase 3 — Server: broker narrowing
- [ ] `LanChecker` + `files` filter + fail-closed-on-error — verify: `go test ./internal/credbroker/`, AI
- [ ] Wire the checker at construction — verify: build + existing tests, AI
- [ ] Prune materialized `~/.ssh/id_ed25519` on enable, through a confined root (BUG-002/003) — verify: unit with a planted symlink, AI

### Phase 4 — API + UI
- [ ] `lanView` capability + impact fields — verify: `go test ./internal/httpapi/`, AI
- [ ] `LocalNetwork.tsx` confirmation naming both consequences; conf-not-opted-in note — verify: `npm run check && npm test`, AI
- [ ] Browser walk of the golden path and the edge case — verify: e2e, both

### Phase 5 — Records
- [ ] ADR-066 amending ADR-065 (5) and D3, carrying the negated-`-o` finding — verify: read, AI
- [ ] R12 amendment — **operator approval on the exact text first** — verify: user
- [ ] Guide + `docs/deploy/remote.md` sshd precondition with exact commands — verify: `documentation`, AI
- [ ] `TODO.md` item (b) corrected: its recorded rule shape is invalid — verify: read, AI
- [ ] Release: `verifier`, `reviewer`, `security`, `/preflight` — verify: gate report, both

## Proposed R12 amendment (NOT applied — needs approval)

Replace the sentence beginning *"Everything else stays dropped for that
project too: the host itself (INPUT drop …)"* with:

> A LAN-enabled project reaches the configured prefixes with the same reach
> a peer on that network has: the host's own address on the uplink included,
> and host-published Docker ports included, both only when the root-owned
> conf opts in (`LAN_ALLOW_HOST`, `LAN_ALLOW_PUBLISHED_PORTS`; absent means
> no). The host's sshd is inside that allowance, so a deployment enabling it
> MUST NOT accept password authentication on the uplink. Still dropped for
> that project: every other project and system bridge, the ingress bridge,
> cloud metadata, the tailnet and every private range outside the configured
> prefixes; LAN hosts still cannot initiate connections into any project,
> which remains NAT-only outbound. A LAN-enabled project is not served the
> operator's SSH key by the credential broker. (Was, before ADR-066: the
> host, its published ports and the SSH key were all inside the allowance's
> exclusions.)

## Risks

1. **sshd hardening is a precondition, and getting it wrong locks the
   operator out of their own machine.** The host has `id_ed25519` but no
   `authorized_keys`, so today's access is by password. Sequence, run BY THE
   OPERATOR with a second session open: install the pubkey
   (`ssh-copy-id` or append `~/.ssh/id_ed25519.pub` to
   `~/.ssh/authorized_keys`, `chmod 600`), prove a NEW session logs in with
   the key while the current one stays open, only then set
   `PasswordAuthentication no` and `sudo systemctl reload ssh`. Never in one
   step, never without the second session.
2. **`--ctstate DNAT` semantics.** Verified to parse; that it matches only
   published-port traffic is asserted by the Phase 2 live test, not assumed.
3. **A v4 script with a v3 conf is the normal upgrade state**, not an error.
   The UI must say so rather than look broken.
4. **Two new rule families double the reconcile surface** the v1.23.0
   security review examined. Both need `security` on the diff; the
   delete-by-spec discipline (BUG-013) and `inherit_errexit` (BUG-010) apply
   unchanged.
5. **The prune is best-effort.** See Failure modes.
