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
| `deploy/firewall/install.sh` | writes `LAN_ALLOW_HOST` (default **0**) and later `LAN_ALLOW_PUBLISHED_PORTS`, `INGRESS_BRIDGE=`; `--lan-allow-host` and `--ingress-bridge` flags |
| `deploy/firewall/test.sh` | new cases per Verification |
| `internal/netpolicy/netpolicy.go` | `MinScriptVersion` 4 |
| `internal/netpolicy/status.go` | parse `lan_allow_host`, `lan_allow_published_ports`; surface in `Status` |
| `internal/netpolicy/dir.go` | no change expected — confirm |
| `internal/credbroker/broker.go` | `LanChecker`; `files` filters ssh-shaped credentials |
| `internal/httpapi/network.go` | prune materialized key on enable; expose capability + impact in `lanView` |
| `internal/httpapi/server.go` | `LanExposed` method (uses the existing `s.space(user).projects` + `s.netpol`) |
| `cmd/styrman/main.go:180` | pass the `Server` into `credbroker.New` as the checker — this is where the broker is built, NOT `httpapi/setup.go` |
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

### Rule-family machinery — GENERALIZE, do not duplicate

`scan_current`, `rule_args`, `reconcile`, `count_applied`, `delete_malformed`
and `RULE_RE` are today hardwired to chain `STYRMAN-FWD` and the single
`styrman-lan:` tag (script lines 199, 336-353, 364-370, 380-391, 409). Three
families cannot be three copies of that: a delete-by-spec or ordering bug
fixed in one copy and missed in the others is precisely BUG-013 again.

**Decision: one generic implementation parameterized by family.** A family is
`(tag, chain, regex, args-builder)`:

| tag | chain | fields in the key | wanted per project |
|---|---|---|---|
| `lan` | `STYRMAN-FWD` | iface, uplink dev, bridge subnet, lan cidr | one per `LAN_CIDRS` entry |
| `lanhost` | `STYRMAN-IN` | iface, bridge subnet, host addr | one per host address on `LAN_DEV` |
| `lanpub` | `STYRMAN-FWD` | iface, bridge subnet, allowed bridge subnet | one per allowed bridge subnet |

- Keys carry the tag as their **first** field and the uuid as their **last**;
  the fields between are the family's own and their COUNT VARIES BY FAMILY
  (`lan` needs three — uplink dev, bridge subnet, lan cidr — where `lanhost`
  and `lanpub` need two). The generic code addresses only those two fixed
  positions, via `${key%% *}` and `${key##* }`, and never parses the middle;
  only `rule_args_<tag>` and `fam_key_<tag>` know a family's arity. So the
  `lan` key is `lan <iface> <dev> <src> <dst> <uuid>` (six fields) while
  `lanhost` is `lanhost <iface> <src> <dst> <uuid>` (five). `DESIRED`, `SEEN`,
  `PURGE`, `MALFORMED` stay single associative arrays.
- `rule_args` dispatches on the key's tag to `rule_args_<tag>`, which builds
  `ARGS` and also yields the family's chain — so `reconcile` needs no chain
  argument, it reads one off the key.
- `scan_current` runs **once per distinct chain** (two: `STYRMAN-FWD`,
  `STYRMAN-IN`), not once per family — `lan` and `lanpub` share
  `STYRMAN-FWD`, and two reads would give them disagreeing first-DROP
  positions. Within one line: skip unless it is `-A <chain> `; count it;
  record the first `-j DROP` position; skip unless it carries the
  `--comment "styrman-lan` prefix; then try each family bound to THIS chain
  in `FAMILY_TAGS` order and take the first regex that matches, which yields
  the key via `fam_key_<tag>`. The families' regexes are disjoint by
  construction — each pins its own literal tag inside the comment — so
  "first match wins" is not an ordering dependency, and a tagged line matching
  none of them is malformed. The first-DROP test then applies to the key
  regardless of which family produced it.
- `delete_malformed` takes the chain as a parameter; `MALFORMED` entries
  carry `(chain, line)` so the re-read-and-confirm-by-number step targets the
  right chain. This answers the reviewer's question 2: one function, a
  parameter, not a twin.
- `WANT_COUNT[uuid]` becomes the **total across all families** and
  `APPLIED_COUNT[uuid]` likewise, so `lan=` keeps its existing all-or-nothing
  meaning: a project is reported ON only when every rule it wants, in every
  family, is present. No status-schema change; `netpolicy` needs no change to
  read it.

A third source of churn joins the two the plan already named (`LAN_CIDRS`
edits, DHCP address changes): the allowed-bridge set is re-derived each run,
so a new Docker network appearing or disappearing adds or removes `lanpub`
rules on the next tick. That is correct behaviour — parity with a LAN peer —
but it means a quiet run is no longer the only steady state, and the journal
line must stay one-per-run (it already gates on added/removed/errors).

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

**Why a wrong value here is dangerous, and what makes it detectable.** This
key is a safety EXCLUSION: if it names the wrong bridge, the real ingress
bridge stays inside the allowed set and a LAN-enabled project can reach the
ingress through a published-port DNAT — the opposite of the key's purpose.
Three things close that, and none of them is "trust the operator":

1. **Derivation, not typing.** Docker names a bridge `br-` + the first 12 hex
   of its network id, so the value is deterministic:
   `br-$(docker network inspect styrman-ingress --format '{{.Id}}' | cut -c1-12)`
   — verified live 2026-09-08 (`styrman-ingress` → `02a045873a1d` →
   `br-02a045873a1d`, matching the host's actual interface).
   `install.sh` derives it that way and **`die`s if it cannot**, exactly as it
   already does for `LAN_DEV` (`install.sh:130-141`); it is never defaulted
   and never guessed. A `--ingress-bridge` flag overrides for a host whose
   docker CLI is unavailable.
2. **A stale value fails closed by itself.** Because the name is derived from
   the network id, recreating `styrman-ingress` changes the bridge name — so
   a conf left behind by a recreate names an interface that no longer exists,
   the existence check fails, and the published-port family is skipped
   entirely with a `conf_error`. Stale is loud, not silent.
3. **Wrong-but-valid is caught by the server, not root.** Root cannot tell
   which docker bridge is the ingress without the docker daemon, which a root
   firewall script must not depend on. So the script REPORTS what it used —
   `ingress_bridge=` in the status file — and the server, which does know its
   own ingress network, compares and surfaces a mismatch in the host-firewall
   view. Root fails closed on absence; the server flags divergence.

### Status file additions

```
lan_allow_host=1
lan_allow_published_ports=1
lan_host_addrs=192.168.86.58
ingress_bridge=br-02a045873a1d
```

Additive; `schema` stays 1. **Confirmed 2026-09-08**, so this is settled and
not a task: `ParseStatus` (`internal/netpolicy/status.go:60`) switches on the
key with no `default` case, so unknown keys are ignored; only `schema` is
gated (must be exactly 1). A v4 script's status file therefore parses cleanly
in a v3 server mid-upgrade.

### Go

```go
// internal/credbroker
type LanChecker interface {
    // LanExposed answers "could this project reach the LAN right now?",
    // NOT "what does its record say". See the semantics below.
    LanExposed(user, project string) (bool, error)
}
func New(store *credstore.Store, resolve Resolver, lan LanChecker, logger *slog.Logger) *Broker

// The key is withheld when LanExposed is true OR returns an error (fail
// closed: a transient store error must never be the way a LAN-enabled
// project gets the key). Costs ssh push in that project; GH_TOKEN over
// HTTPS still covers github.com.
func isHostReachingFile(path string) bool   // "~/.ssh/" prefix, tested
```

**Implemented by `*httpapi.Server`**, which is the only type holding both
halves — per-user project stores (`s.space(user).projects`) and root's last
word (`s.netpol`). It is built at `cmd/styrman/main.go:104`, well before the
broker at `main.go:180`, so it is simply passed in; no new package edge and
no dependency cycle. `internal/orchestrator` — today's `Resolver` — cannot
do this: it has no `internal/project` dependency and no view of
`Project.LanAccess`.

**Record-vs-converged, decided:** `LanExposed` returns true when **any** of
these holds, which is deliberately the union, not the record alone:

1. the project record says `LanAccess` (covers enable, from the instant it
   is requested — withhold early);
2. root's status still lists the project in `Applied` (covers disable, until
   the rule is genuinely gone);
3. the status is unreadable, stale, or otherwise cannot confirm (2) —
   "cannot confirm" is never read as "off", which is BUG-014's rule applied
   to the credential side.

Without (2) a disable would re-serve the key while the firewall rule was
still live for up to 60 s — a project holding both LAN reach and the key,
which is the exact combination this whole phase exists to prevent. The
login shell only re-fetches `/files` on a new session, so the window needs a
new shell to be hit; that makes it unlikely, not closed, and the check is
nearly free because `network.go:182` `kickLAN` already does precisely this
record-plus-`st.Applied` lookup and is the shape to copy.

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
| `LanExposed` errors | Key withheld, `slog.Error`. Visible as a project that cannot ssh-push. |
| LAN disabled, rule not yet removed | Key stays withheld until root's status drops the project from `Applied` (≤60 s, usually ~0.2 s via the spool kick). A new login shell in that window gets no key; an existing shell is unaffected either way. |
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

### Phase 1 — Root: generalize the rule-family machinery (NO behaviour change)
The safety property of this phase is that it changes nothing observable:
`deploy/firewall/test.sh` must pass **unmodified** at the end of it, and a
run against the live chains must produce a byte-identical status file.
- [x] Introduce the family table and tag-prefixed keys; port the existing `lan` family onto it as the only family — verify: `test.sh` unmodified, AI
- [x] `rule_args` dispatches on tag and yields the chain; `scan_current` loops over distinct chains; `delete_malformed` takes a chain parameter — verify: `test.sh` unmodified, AI
- [x] `WANT_COUNT`/`APPLIED_COUNT` summed across families (one family today, so identical results) — verify: `test.sh` unmodified, AI
- [x] Diff read end to end for delete-by-spec (BUG-013) and `inherit_errexit` (BUG-010) survival — verify: read + `security` agent, AI

**Phase 1 result (2026-09-08):** 105 insertions, 41 deletions in
`styrman-firewall.sh`; `test.sh` untouched and passing **83/0, identical to
the pre-change baseline**. `bash -n` and `shellcheck -S warning` clean,
`go vet ./...` clean, `go test ./...` green including
`TestFirewallScriptInNetns` and `TestMinScriptVersionMatchesScript`. Also
exercised against the LIVE host chains with a scratch state dir: conf parsed,
`lan_dev`/`lan_cidrs` populated, empty marker dir yielding no allowances, and
`STYRMAN-FWD`/`STYRMAN-IN` byte-identical before and after. Version stays 3 —
the bump belongs to Phase 2, which is why `test.sh`'s `version=3` assertion
still holds unmodified. One deliberate text change: the insert-failure
`ERRORS` entry is now `<uuid>:<tag> insert failed` (was `<uuid>:<dst uuid>:
insert failed`, whose field-stripping assumed the old fixed arity); it is an
error path no test asserts, and the tag is more useful now there are three
families.

### Phase 2 — Root: host reachability
- [x] Parse `LAN_ALLOW_HOST`; discover host addresses on `LAN_DEV`; validate each is inside `LAN_CIDRS` — verify: `test.sh`, AI
- [x] `lanhost` family registered in the table (chain `STYRMAN-IN`) — verify: `test.sh`, AI
- [x] `STYRMAN_FW_VERSION` 4 + `netpolicy.MinScriptVersion` 4 + the existing pin test — verify: `go test ./internal/netpolicy/`, AI
- [x] Status keys `lan_allow_host`, `lan_host_addrs` — verify: unit, AI
- [x] `install.sh` writes the new keys; `--lan-allow-host` flag — verify: `test.sh` + dry run, AI
- [x] Security findings from the Phase 1 review fixed in the same file — verify: `test.sh`, AI

**Phase 2 result (2026-09-08):** `test.sh` **102 / 0** (was 83 at the Phase 1
baseline; +19 checks). `go vet`, `go test ./...` and the vendored ingress
module all clean.

**Correction to this plan's own text.** It said `install.sh` writes
`LAN_ALLOW_HOST=1`. It writes **0**. Writing 1 on a fresh install would mean
every new deployment grants host reach by default, which contradicts
ADR-066 part (2) ("absent still meaning 0, so an existing deployment does not
silently gain host access... opting in is a human running install.sh") — the
plan text was inconsistent with the decision it implements. `install.sh` now
takes `--lan-allow-host 0|1`, defaults to 0 on a fresh conf, and — uniquely
among its steps — will update that ONE key in an existing conf, because the
conf is otherwise never rewritten once present and the operator would
otherwise have to hand-edit it. Without the flag on an existing conf it prints
a note saying the host is not reachable and how to opt in.

### Phase 3 — Root: published ports
- [x] `INGRESS_BRIDGE` conf key, validation, fail-closed skip when unset OR when the named interface does not exist — verify: `test.sh`, AI
- [x] `install.sh` DERIVES it from `docker network inspect styrman-ingress` and `die`s on failure, as it already does for `LAN_DEV`; `--ingress-bridge` override — verify: `test.sh` + dry run, AI
- [x] Report `ingress_bridge=` in the status file — verify: unit, AI
- [x] Allowed-bridge-subnet derivation excluding `sdb-*` and the ingress bridge — verify: `test.sh`, AI
- [x] `lanpub` family with `-m conntrack --ctstate DNAT` — verify: `test.sh`, AI
- [-] Live proof with a real published container (`docker run -p`) — closes the "reasoned, not observed" gap from the v1.23.0 handoff — verify: `integration.sh`, both — **written, not yet run against an opted-in host**: the assertions exist in `integration.sh` and take their expectations from root's own status file, so they are correct on a host that has NOT opted in (and assert the host stays closed there). Actually exercising the reachability half needs `LAN_ALLOW_PUBLISHED_PORTS=1` in the production conf, which is the operator's deliberate act. — moved to handoffs/2026-09-16-v1.35.0-harness-channel-live.md Next Steps (2026-09-16)

**Phase 3 result (2026-09-08):** `test.sh` **120 / 0** (was 102 after Phase 2;
+18 checks). `go vet`, `go test ./...` and the ingress module clean.
Verified against the REAL host topology with a scratch state dir and no
markers, so no rule was inserted and nothing was granted: root derived
`lan_host_addrs=192.168.86.58/32`, `ingress_bridge=br-02a045873a1d` and
`lan_pub_subnets=172.17.0.0/16` — docker0 only, with the ingress
(`172.18.0.0/16`) and the live project bridge (`172.19.0.0/16`) both
correctly excluded. `STYRMAN-FWD` and `STYRMAN-IN` byte-identical before and
after.

### Phase 4 — Server: broker narrowing
- [x] `LanExposed` on `*httpapi.Server`: record OR `st.Applied` OR cannot-confirm, modelled on `network.go:182` `kickLAN` — verify: `go test ./internal/httpapi/`, AI
- [x] `credbroker.LanChecker` + `files` filter + `isHostReachingFile`, fail closed on error — verify: `go test ./internal/credbroker/`, AI
- [x] Wire the `Server` into `credbroker.New` at `cmd/styrman/main.go:180` — verify: build + existing tests, AI
- [x] Prune materialized `~/.ssh/id_ed25519` on enable through a confined root (BUG-002/003), with a planted-symlink test — verify: unit, AI

**Phase 4 result (2026-09-08):** the union rule lives in
`netpolicy.Exposed`, beside `Derive`, with 8 table cases; the broker gains
`LanChecker`, `isHostReachingFile` and a fail-closed wrapper (error OR an
unwired checker both withhold); `*httpapi.Server` implements it, guarded by
`netpol.Enabled()` so a local dev run with no firewall half does not read as
"cannot confirm" and silently strip every project's key. Prune paths come
from the credential store rather than a hardcoded list, so they cannot drift
from what the broker withholds. `go vet`, `go test ./...`, the ingress module
and `test.sh` (120/0, untouched) all clean.

**The symlink test was mutation-checked**, not merely written: swapping
`files.Remove` for `os.Remove` makes it fail with the victim file outside the
tenant tree deleted, and restoring the confined call makes it pass. A test
for a confinement property that has never been shown to fail without the
confinement is not evidence.

### Phase 5 — API + UI
- [x] `netpolicy.Status` parses the ADR-066 keys — a prerequisite this plan implied but did not list: `ParseStatus` ignored them, so the server could not see the opt-ins at all — verify: `go test ./internal/netpolicy/`, AI
- [x] `lanView` capability + impact fields — verify: `go test ./internal/httpapi/`, AI
- [x] Server compares root's `ingress_bridge=` against its OWN ingress network's bridge and surfaces a mismatch — root cannot know which bridge is the ingress without the docker daemon, so this is the half that catches a wrong-but-valid conf value — verify: `go test ./internal/httpapi/`, AI
- [x] `LocalNetwork.tsx` confirmation naming both consequences; conf-not-opted-in note — verify: `npm run check && npm test`, AI
- [x] Browser walk of the golden path and the edge case — verify: e2e, both

**Phase 5 result (2026-09-08):** browser e2e **89 passed / 0 failed / 2
skipped** (was 84/2 skipped before this phase), including five new ADR-066
checks that plant a fake root status file in the e2e's scratch firewall dir
and drive the real UI: the dialog when the host conf does NOT include the
host, the dialog when it does, confirming and seeing the withheld-key note,
a pending record refusing to flip, and disable being accepted while pending.
`npm run check`/`lint` clean, vitest **85** (was 80). `go vet`,
`go test ./...` and `test.sh` (120/0) clean.

**A pre-existing bug fell out of the e2e.** `LocalNetwork.tsx` chose between
its optimistic PUT reply and the poll with an EQUALITY test on
`lanChangedAt`, so any out-of-band change (another tab, another admin, the
API) moved the poll's timestamp past the override's and pinned the component
to a stale reply for as long as it stayed mounted. Extracted to a pure,
unit-tested `preferOverride` in `lan.ts` and changed to an ordering
comparison. Recorded as a numbered bug.

**One e2e check was wrong, not the code.** A first draft tried to click the
switch off while the record was pending; the component correctly disables it
then ("nothing flips while pending"). The check now asserts that, and
disables through the API — which is legitimate because tightening is never
refused.

### Phase 6 — Records
- [x] ADR-066 amending ADR-065 (5) and D3, carrying the negated-`-o` finding — verify: read, AI
- [x] R12 amendment — operator approved the exact text 2026-09-08 ("approve the R12 text and start phase 1"); ADR-066's amendment entry records it verbatim with its scope — verify: user
- [x] Guide + `docs/deploy/remote.md` sshd precondition with exact commands — verify: `documentation`, AI
- [x] `TODO.md` item (b) corrected — its recorded rule shape is invalid, and Phase 3 marked it RESOLVED — verify: read, AI
**Phase 6 docs (2026-09-08):** `guide.md` corrected in three places — the
isolation bullet no longer claims the host and its published ports are
unconditionally dropped, the Local network section describes parity and both
consequences, and two troubleshooting rows added (ssh push failing after
enable; the host still unreachable because the conf has not opted in).
`deploy/remote.md` gained the opt-in commands and the **sshd hardening
precondition** as a five-step sequence with a second session held open
throughout, plus the plain statement that this host is not hardened and has
not opted in. `AGENTS.md`'s architecture bullet and the testing README's
counts updated.

- [x] Release: `verifier`, `reviewer`, `security`, `/preflight` — **DONE 2026-09-08**: gate
  `reviews/2026-09-08-preflight-v1.24.0.md` (16 findings across three rounds, all closed),
  shipped as v1.24.0 and deployed unattended; the audit's fixes followed as v1.24.1.

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
4. **Phase 1 refactors security-critical code that has already passed a
   security review, before adding anything.** That is the deliberate trade:
   three near-copies of the reconcile machinery would put the next
   BUG-013-class bug in whichever copy got missed. The mitigation is that
   Phase 1 is behaviour-preserving by construction — `test.sh` unmodified is
   its pass condition — and that `security` reviews the refactor diff on its
   own, before the new families exist to confuse the reading.
5. **The prune is best-effort.** See Failure modes.
