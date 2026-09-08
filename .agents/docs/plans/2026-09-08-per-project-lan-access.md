---
owner: main
date: 2026-09-08
---
# Plan — Per-project local-network (LAN) access toggle   (2026-09-08)

**Status: DRAFT — awaiting the operator's decisions D1–D7 (section
"Decisions") and the cold-reader check. No code lands before the R12
amendment is approved (ADR-055 precedent: code shipped against the old
REQUIREMENTS text was a blocking finding).**

Research record: a 15-agent workflow on 2026-09-08 (seven subsystem readers,
three independent designs, three judges, one synthesis, one adversarial
critic). The critic returned `needs-fixes` with seven refutations and thirteen
holes, each verified on this host; every one is folded in below and marked
`(critic)` where it changed the design.

## Why

Operator request (2026-09-08): "a toggle to enable local network traffic per
project. It should be able to change during runtime on or off. […] it should
be per project (microvm)."

Today R12 makes every project internet-only: the root-owned host firewall
(`/usr/local/sbin/styrman-firewall.sh`, written by `deploy/host-setup.sh`
§3b, re-applied every 60 s by `styrman-firewall.timer`) drops all routed
traffic from any `sdb-*` bridge to 10/8, 172.16/12, 192.168/16, 169.254/16
(and, in the repo but NOT yet on the live host, 100.64/10 and 198.18/15). A
project therefore cannot reach a printer, a NAS, a router, or a local model
server on the operator's LAN. The operator wants that to be allowed for
chosen projects, switchable while the project runs.

Two facts shape the whole design:

1. **The server cannot touch the firewall.** `styrman-server` runs as the
   operator's uid, no `NET_ADMIN`, no iptables binary, in its own netns
   (`deploy/docker-compose.yml` `user:`/`sysctls:`; `deploy/Dockerfile.server`).
   Only root on the host can add or remove a rule. So the server may only
   *say* on/off per project; root decides what "local network" means and
   applies it.
2. **Production is already drifted.** The live script (mtime 2026-08-30) has
   four DROPs; the repo's heredoc (2026-09-02 audit fix) has six. Nothing
   delivers host-setup changes: the auto-updater self-installs only
   `deploy/update/*`, and the release procedure (`versioning.md`) has no host
   step. Verified 2026-09-08 with `sudo -n diff` and `sudo -n iptables -S
   STYRMAN-FWD`. This feature's host half would drift the same way unless
   delivery is fixed alongside it.

## Decisions (operator)

The plan is written under the recommended answer to each. A different answer
changes the marked sections; say which.

- **D1 — R12 amendment + ADR-065 before code.** *Recommended: approve the
  text under "R12 amendment" verbatim, landing in ONE commit with ADR-065, the
  amendment entry, and ROADMAP "Now".* Alternatives: narrower (no runtime
  toggle) / wider (host itself or published ports — needs an ADR-029
  reversal) / drop the feature.
- **D2 — Who may turn LAN on.** *Recommended: only an admin, and only on a
  project in their own store (no admin path into another user's project
  exists today and none is added). Turning OFF is always allowed to the
  owner.* Alternative: any owner; or ship an operator knob
  `lanPolicy = admins|owners|off` now (≈30 lines via `operatorOverrides`,
  recorded as a follow-up instead).
- **D3 — Does "local network" include this host and its Docker-published
  ports?** *Recommended: NO in v1.* The host runs sshd and Caddy on
  `0.0.0.0`, the operator has passwordless sudo, and every project holds its
  owner's SSH key and tokens through the broker; opening host INPUT reverses
  ADR-029. Two conf keys are reserved (`LAN_ALLOW_HOST`,
  `LAN_ALLOW_PUBLISHED_PORTS`, parsed as 0) for a later, separate ADR.
- **D4 — How the LAN prefix is established.** *Recommended: discovered ONCE
  by `deploy/firewall/install.sh` (default-route device + its connected
  prefix → root-owned `/etc/styrman/firewall.conf`, printed loudly,
  overridable by `--lan-dev/--lan-cidr`, hand-editable), validated on every
  tick, never re-derived at runtime.* On this host: `LAN_DEV=enp114s0`,
  `LAN_CIDRS=192.168.86.0/24`.
- **D5 — Delivery of the host half.** *Recommended: both `install.sh`
  (manual, once, for this release and for prefix discovery) AND a
  `host_converge()` step in `styrman-update` that installs
  `deploy/firewall/*` from the verified tag on every tick, closing the
  drift class.* Alternative: manual re-runs only (drift stays open).
- **D6 — Sequencing.** *Recommended: host first.* Converge the EXISTING
  six-DROP drift immediately in Phase 0 regardless of the feature; run
  `install.sh` from `/opt/styrman` at the Phase-1 tag; ship the server tag
  last. Both orders are fail-closed; host-first makes the feature usable the
  moment the tag lands.
- **D7 — Opt-in live integration check.** *Recommended: yes, default SKIP.*
  `STYRMAN_IT_LIVE_LAN=1` lets `integration.sh` write ONE marker named by
  its own test project's uuid into production's marker dir plus one spool
  entry, removed by name in the cleanup trap — the only automated
  end-to-end proof of LAN reachability through the real reconciler.

**UI placement (the operator asked for a recommendation):** the control
lives at the top of the **Ports** side-panel tab as a "Local network" block
(the project's network surface; it must work for a STOPPED project, so it
reads the shared project record, not the ports poll); a small **`LAN` chip**
in the sidebar project row and on the dashboard card flags the standing
exception; an admin-only **Settings → Host firewall** section shows the
host's state (script version, last applied, prefixes, projects on). The
crowded three-verb `⋯` menu is not touched.

## Scope

**In**

- `Project.LanAccess` intent in `projects.json`; a server-only marker
  directory root reads by NAME only; a root-parented spool root consumes; a
  root-written status file the server reads; a versioned reconciler section
  in the firewall script; `.path` unit for sub-second application; `install.sh`;
  `host_converge` in the updater; API (`GET`/`PUT
  /api/projects/{id}/network`, `lanState` on the project list, admin
  `hostFirewall` in settings); UI (Ports-tab block, chip, Settings section);
  tests at every tier; docs; the R12 amendment + ADR-065.
- Converging the live host's pre-existing four-vs-six DROP drift (Phase 0).

**Out**

- The host itself and host-published Docker ports (D3); LAN DNS (guest
  resolvers stay 8.8.8.8/1.1.1.1 — access is by IP); an owner/tenant policy
  knob (D2); an admin path into another user's project; IPv6; pinning
  Docker's `default-address-pools`; strict `rp_filter`; running the base
  section before docker.service (the pre-existing ≤15 s boot window).

**Follow-ups noted, not built** (go to `TODO.md` in Phase 4): `lanPolicy`
knob; `LAN_ALLOW_HOST` / `LAN_ALLOW_PUBLISHED_PORTS` with their own ADR and
the `! -o sdb-+ ! -o <ingress br>` guard shape; per-project LAN DNS;
`default-address-pools=172.16.0.0/12` in `daemon.json`; verify Docker
live-restore keeps `DOCKER-USER`/`STYRMAN-*` across a daemon restart.

## How it works

```
 browser ──PUT lanAccess──▶ styrman-server (uid 1000)
                             │ 1. projects.json  LanAccess=true          (intent, owner-scoped)
                             │ 2. <StateDir>/netpolicy/lan/<uuid>        (empty marker; name = grammar)
                             │ 3. /run/styrman-firewall/spool/<uuid>.<ns> (empty trigger)
                             ▼
 styrman-firewall.path (DirectoryNotEmpty on the spool) ──▶ styrman-firewall.service (root oneshot)
   also every 60 s: styrman-firewall.timer                  │ base section (today's rules, verbatim)
                                                            │ reconcile_lan: markers ∩ conf → desired
                                                            │   delete stale RETURNs, insert missing
                                                            │ write /run/styrman-firewall/status
                             ┌──────────────────────────────┘
                             ▼
 styrman-server reads status → lanState on/pending/error/unavailable → UI
```

The one rule per (project, LAN prefix), inserted at position 1 of
`STYRMAN-FWD`, ahead of the physdev RETURN and every DROP:

```
iptables -w 5 -I STYRMAN-FWD 1 -i sdb-<11hex> -o <LAN_DEV> -s <bridge-subnet> -d <LAN_CIDR> \
        -m comment --comment "styrman-lan:<uuid>" -j RETURN
# as rendered by `iptables -S` (iptables v1.8.11 nf_tables, verified in a netns on this host):
-A STYRMAN-FWD -s 172.20.0.0/16 -d 192.168.86.0/24 -i sdb-e660d58459e -o enp114s0 -m comment --comment "styrman-lan:3e6a772c-b177-4cfc-b209-b14db514bac2" -j RETURN
```

Why each token: `-i sdb-<hash>` = exactly this project's bridge
(`config.BridgeNameFor(config.NetworkName(uuid))`, sha1-derived, byte-equal
to `printf 'sdb-%s' "$uuid" | sha1sum | cut -c1-11`, verified live:
`sdb-3e6a772c-…` → `sdb-e660d58459e`). `-s <bridge subnet>` because
`rp_filter` is loose (2) on every interface and Docker's MASQUERADE matches
only the bridge subnet — a VM with `NET_ADMIN` could otherwise emit
spoofed-source packets that leave un-NATed. `-o <LAN_DEV>` pins egress to the
uplink so a post-DNAT packet (destination rewritten to a container on
docker0/br-*) or a bridge-bound packet can never match. `RETURN`, not
`ACCEPT`: RETURN unwinds to the end of `DOCKER-USER` and lets Docker's own
`DOCKER-FORWARD` chain run (conntrack accepts, then the `! -i X -o X DROP`
cross-bridge isolation, then `-i sdb-<hash> -j ACCEPT`); ACCEPT is terminal
for the FORWARD hook and would skip Docker's second line of isolation.
Position 1: the base DROPs are only ever `-C || -A` appended, so a head insert
stays ahead across ticks and after a flush-and-restore, even if `xt_physdev`
failed to load.

Toggle OFF deletes the rule; because `DOCKER-USER` runs before any conntrack
ESTABLISHED accept (verified: `FORWARD` = `-j DOCKER-USER`, `-j
DOCKER-FORWARD`), the next outbound packet of every open project→LAN flow is
dropped at once. Clients see timeouts, not resets — stated in the UI.

## What "local network" means

| Destination from the project | Result | Deciding rule |
|---|---|---|
| LAN hosts in `LAN_CIDRS` (router, NAS, printer, a LAN machine running a model server) | **allowed** (project-initiated only; by IP) | the per-project RETURN, then Docker's `-i sdb-X -j ACCEPT`, MASQUERADE as the host's LAN IP |
| This host at 192.168.86.58, 172.20.0.1, 172.18.0.1, 127/8 (sshd :22, Caddy :80/:443) | dropped | `INPUT -i sdb-+ -j STYRMAN-IN` → DROP (untouched) |
| A port this host publishes via Docker (`-p 192.168.86.58:P`) | dropped | PREROUTING DNAT rewrites the destination to 172.17/16 or 172.18/16 BEFORE filter FORWARD; `-d <LAN_CIDR>` and `-o <LAN_DEV>` cannot match; `-d 172.16.0.0/12 DROP` decides |
| Other projects (`sdb-*`, 172.16/12 by IPAM) | dropped | `-d 172.16.0.0/12 DROP` **and** Docker's `! -i X -o X DROP` (kept because we RETURN); the conf validator refuses any CIDR overlapping 172.16/12 or ANY Docker bridge's connected route (covers Docker's 192.168.0.0/16 fallback pool) |
| styrman server / ingress bridge / broker (172.18/16, 172.18.0.2, 172.20.255.254) | dropped | 172.16/12 DROP + the existing raw PREROUTING drops |
| Cloud metadata 169.254.169.254, tailnet 100.64/10, 198.18/15, 10/8, the rest of 192.168/16 | dropped | base DROPs; overlap refused in conf |
| LAN → project inbound NEW | refused | `DOCKER-USER` does not match `-i enp114s0`; Docker's cross-bridge DROP; project IPs are not routable from the LAN |
| Spoofed source from the VM | dropped | `-s <bridge subnet>` on every allow |
| IPv6 | nothing changes | bridges have `disable_ipv6=1`; no v6 exemption is ever written |
| The setup VM (`sdb-sys-setup-<user>`) | never allowed | its network name is not a uuid, so no marker can name it |

Everything the project runs gets the allowance — agent shells, in-guest DinD
containers (NATed to the VM IP), anything using the guest's sudo — because
enforcement is host-side precisely because the guest is adversarial.

## Surfaces touched

Host (new directory `deploy/firewall/`, replacing the heredocs in
`deploy/host-setup.sh:149-266`):
`deploy/firewall/styrman-firewall.sh`, `.service`, `.timer`, `.path`,
`install.sh`, `test.sh`; `deploy/host-setup.sh` §3b (delegates to
`install.sh`; `SCRIPT_DIR` moves above §3b); `deploy/update/styrman-update`
(`host_converge()`), `deploy/update/styrman-update.service` (env);
`deploy/verify-commit.sh` (shell lint + `systemd-analyze verify`);
`deploy/docker-compose.yml` (one volume, one env).

Server: `internal/config/config.go` (`NetPolicyDir`, `FirewallRunDir`,
`STYRMAN_FIREWALL_DIR`); new package `internal/netpolicy`;
`internal/project/store.go` (`LanAccess`, `LanChangedAt`, `SetLanAccess`);
`cmd/styrman/main.go` (dir creation, boot `Sync` + kick, a dedicated 2-min re-sync ticker);
`internal/httpapi/network.go` (new: GET/PUT), `server.go` (`projectView`
fields, routes, `netMu`, kicks after `EnsureRunning` in `ensureFor`,
`createProject`, `startProject`; marker removal in `deleteProject`),
`explorer.go` (`hostFirewall` in settings for admins); `internal/config`
golden test for `BridgeNameFor`.

UI (`web/src`): `lib/api.ts` (types + two verbs), `lib/lan.ts` (new, pure
chip/label derivation), `components/ui.tsx` (`Switch`),
`components/SidePanelPorts.tsx` (block), `components/Sidebar.tsx` (chip),
`routes/index.tsx` (card chip), `routes/settings.tsx` (Host firewall
section); `web/e2e/run.mjs`, `web/e2e/run.sh`.

Tests: `internal/netpolicy/*_test.go`, `internal/project/store_lan_test.go`,
`internal/httpapi/network_test.go`, `internal/config/bridgename_test.go`,
`web/src/lib/lan.test.ts`, `.agents/docs/testing/styrman/integration.sh`,
`deploy/firewall/test.sh` (run under `go test` via `internal/netpolicy/script_test.go`).

Docs: `REQUIREMENTS.md` (R12, after approval), `DECISIONS.md` (ADR-065 +
amendment), `ROADMAP.md`, `TODO.md`, `specs/styrman/guide.md`, `deploy.md`,
`versioning.md`, `api.md`, `.agents/docs/deploy/remote.md`, `deploy/README.md`,
`deploy/.env.example`, `README.md`, `AGENTS.md`, testing `README.md`,
`.agents/rules/guest-writable-paths.md` (one line: the marker dir is
server-only and never a bind source).

## Interfaces

### Host side

**Files and ownership**

| Path | Owner / mode | Written by | Read by |
|---|---|---|---|
| `<StateDir>/netpolicy/lan/<uuid>` | operator uid, 0644 in 0755 dirs (`netpolicy` AND `netpolicy/lan` both chowned — critic hole) | server (`O_CREATE\|O_EXCL`, `os.Remove`) | root: names only, `-f`, `! -L`, uuid regex; contents never read |
| `/run/styrman-firewall/` | root 0755 (tmpfiles.d at boot; repaired in place by every run; never recreated while the container's bind is live) | root | — |
| `/run/styrman-firewall/spool/` | operator uid 0700 under the root parent | server drops `<tag>.<unixnano>` (`O_EXCL`) | root consumes with `find -mindepth 1 -delete` (never follows symlinks; parent is root-owned so uid 1000 cannot swap `spool` for a link) |
| `/run/styrman-firewall/status` | root 0644, atomic `mktemp` + `mv -fT` | root | server (2 s TTL cache) |
| `/run/styrman-firewall/lock` | root | `flock` | — |
| `/etc/styrman/firewall.conf` | root 0644, written by `install.sh` ONLY IF ABSENT | operator | root (regex-parsed, never sourced; ignored unless root-owned and not g/o-writable) |

Compose gains `- /run/styrman-firewall:/run/styrman-firewall` (rw; `status`
is protected by DAC) and `STYRMAN_FIREWALL_DIR=/run/styrman-firewall`.
Before the host half exists Docker creates the dir root-owned and empty; the
server reports every project `unavailable` and refuses enables — harmless.

**`/etc/styrman/firewall.conf` grammar** (each line `^(LAN_DEV|LAN_CIDRS|LAN_ALLOW_HOST|LAN_ALLOW_PUBLISHED_PORTS)=([A-Za-z0-9./ ,_-]*)$`):

```
# written by deploy/firewall/install.sh 2026-09-08 — edit, then: sudo systemctl start styrman-firewall.service
LAN_DEV=enp114s0
LAN_CIDRS=192.168.86.0/24
```

Validation on every run: `LAN_DEV` matches `^[A-Za-z0-9_.-]{1,15}$`, exists in
`/sys/class/net`, and is NOT a Docker bridge **by name** (`docker0`,
`br-[0-9a-f]{12}`, `sdb-*`) — a kernel-bridge uplink (`br0` on a KVM host)
only logs a warning (critic: R9/R11 portability). Each CIDR: dotted quad
`/16..30`, host bits zero (bash arithmetic — iptables silently normalises
`.7/24` to `.0/24` and a set-compare would churn), no overlap with
172.16.0.0/12, 169.254.0.0/16, 100.64.0.0/10, 198.18.0.0/15, 127.0.0.0/8,
0.0.0.0/8, 224.0.0.0/3, nor any connected route (`ip -o -4 route show scope
link`) whose device is a Docker bridge by the same name test — `LAN_DEV`'s
own route is excluded from the overlap set (critic). Any invalid token →
`LAN_CIDRS=∅`, `conf_error=<reason>`, every per-project allow deleted (fail
closed, visible).

**`/run/styrman-firewall/status` grammar** (key=value, unknown keys ignored; any parse failure or size > 64 KiB = unavailable):

```
schema=1
version=3                       # STYRMAN_FW_VERSION of the writer; 3 is the FIRST version that writes status (critic)
applied_at=<unix seconds>
lan_dev=enp114s0
lan_cidrs=192.168.86.0/24 [<cidr> ...]
lan=<uuid> [<uuid> ...]         # uuid listed only when EVERY (uuid, cidr) rule passed -C (critic hole)
pending=<uuid>:no-bridge        # repeatable
error=<uuid>:<cidr>:<text>      # repeatable; text sanitised to [A-Za-z0-9 ._:/-]
conf_error=<text>               # present ⇒ lan_cidrs empty and all allows removed
warn=<text>                     # repeatable (ignored marker names, refused cidr, non-Docker bridge uplink …)
spool=<n> run_ms=<int>
```

**`deploy/firewall/styrman-firewall.sh`** — `STYRMAN_FW_VERSION=3` at the
top; `set -eu`; requires `STYRMAN_FW_STATE_DIR`, `STYRMAN_FW_RUN_DIR`,
`STYRMAN_FW_CONF` from the environment with NO in-script defaults (ADR-052;
a hand-run without them applies the base section and logs "per-project LAN
reconcile skipped (the unit sets STYRMAN_FW_*)"). Sections, in order:

0. **Base section FIRST** (security review: nothing a lower-privileged uid
   controls may delay R12 enforcement), then the LAN prelude below.
1. `flock -w 25 "$RUN/lock"` (blocking — a unit run waits for a hand-run
   rather than exiting with the spool full; `TimeoutStartSec=30s`) (critic).
2. **Drain the spool FIRST in the LAN section** (`find "$RUN/spool" -mindepth 1 -delete`, count
   kept for the status line) and install an `EXIT` trap that drains again,
   so no exit path — guard failure, base-section death, iptables error — can
   leave `DirectoryNotEmpty` true and the unit re-triggering in a tight loop
   (critic: measured re-trigger immediately on exit). Entries created after
   this point survive and re-trigger on exit: no toggle is lost.
3. **Base section**: today's heredoc body verbatim (`ensure_chain`,
   `ensure_rule` `-C || -A`, `ensure_jump` textual, INPUT→STYRMAN-IN DROP,
   DOCKER-USER→STYRMAN-FWD physdev RETURN + six DROPs, IPv6 mirror). Runs
   before anything new so base isolation can never be missing because the
   new section failed.
4. **Guards**: `RUN` absolute, not inside `STATE`, root-owned and not
   g/o-writable; `install -d -m 0755 -o root -g root "$RUN"`; `install -d -m
   0700 -o "$(stat -c %u "$STATE")" "$RUN/spool"`; `MARKERS=$STATE/netpolicy/lan`
   must be a directory (else `warn=` and desired = ∅).
5. **Conf** as above.
6. **Desired**: for each regular, non-symlink `$MARKERS/<name>` whose name
   matches the lowercase UUIDv4 regex (others counted into `warn=`):
   `I=sdb-$(printf 'sdb-%s' "$U" | sha1sum | cut -c1-11)`; if
   `/sys/class/net/$I` is absent → `pending=$U:no-bridge`; else
   `S=$(ip -o -4 route show dev "$I" scope link | awk '{print $1}')` (must
   match the CIDR regex). Desired = {(I, LAN_DEV, S, C, U) : C ∈ LAN_CIDRS}.
7. **Current** (as implemented after the security review — deletion is BY
   SPEC, never by a number another root writer could shift): every
   `iptables -S STYRMAN-FWD` line is matched against the ONE strict regex of
   our rendering
   (`^-A STYRMAN-FWD -s <cidr> -d <cidr> -i sdb-[0-9a-f]{11} -o [A-Za-z0-9_.-]{1,15} -m comment --comment "styrman-lan:<uuid>" -j RETURN$`),
   with its 1-based index. A well-formed key is kept only when it occurs
   exactly once, before the first `-j DROP`, and is desired; otherwise EVERY
   instance is deleted by full spec rebuilt from the five regex captures
   (`-D STYRMAN-FWD -i … -o … -s … -d … -m comment --comment … -j RETURN`,
   repeated while `-C` succeeds) and, if desired, re-inserted at the head. A
   line carrying `styrman-lan:` that does NOT match the regex is malformed:
   the chain is re-read and the exact line is deleted by its then-current
   number only if it is still there — nothing from `-S` is ever re-tokenised
   or replayed as argv (critic hole + security review medium).
8. **Delete current∖desired FIRST (tighten before loosen), then insert
   desired∖current** with `iptables -C … || iptables -w 5 -I STYRMAN-FWD 1 …`
   (a bare `-I` duplicates). A no-op tick performs reads only (`-S`, `-C`):
   zero writes, so the 60 s self-heal never drops a packet of a live flow.
   Per-rule failures are caught (`error=`) and the run continues.
9. **Status** written atomically as root 0640 with the state dir's group
   (security review: it names every LAN-enabled project); then, if the path
   unit is inactive, `systemctl start` it — at most once per ten minutes
   (`<run>/.revived`), so a flooded spool buys one bounded burst per revive,
   not one per timer tick (security review).
10. One journal line only when something changed or warned:
    `styrman-firewall: lan desired=2 added=1 removed=0 dev=enp114s0 cidrs=192.168.86.0/24 spool=1 run_ms=180`;
    `lan ON project=<uuid> iface=sdb-… src=172.20.0.0/16`; WARN lines for
    refused CIDRs, ignored names, missing bridges. Quiet runs leave
    `applied_at`/`run_ms` in the status file.

**Units** (`deploy/firewall/`, installed verbatim; a non-default `STATE_DIR`
gets a drop-in `styrman-firewall.service.d/override.conf` written by
`install.sh`, which survives reinstall — critic hole):

```
# styrman-firewall.service
[Unit]
Description=styrman project network isolation + per-project LAN policy (R12, ADR-029/036/065)
After=docker.service            # Wants= dropped: a 60 s tick must not start a dockerd the operator stopped (critic hole)
StartLimitIntervalSec=60        # bounded, not unlimited (security review): 20 starts/min is a hard
StartLimitBurst=20              # ceiling on the root work a flooded spool can drive
[Service]
Type=oneshot
Environment=STYRMAN_FW_STATE_DIR=/var/lib/styrman
Environment=STYRMAN_FW_RUN_DIR=/run/styrman-firewall
Environment=STYRMAN_FW_CONF=/etc/styrman/firewall.conf
ExecStart=/usr/local/sbin/styrman-firewall.sh
TimeoutStartSec=30s

# styrman-firewall.timer — unchanged from today (OnBootSec=15s, OnUnitActiveSec=60s, AccuracySec=5s)

# styrman-firewall.path
[Unit]
Description=Apply styrman per-project LAN policy on change
# NO After=docker.service: path units are Before=paths.target, basic.target is After=paths.target,
# docker.service is After=basic.target — an ordering CYCLE systemd breaks by deleting a job
# unpredictably (critic, verified with systemd-analyze verify). The .service already orders itself.
[Path]
DirectoryNotEmpty=/run/styrman-firewall/spool
Unit=styrman-firewall.service
TriggerLimitIntervalSec=60s
TriggerLimitBurst=20
[Install]
WantedBy=paths.target
```

No `MakeDirectory=` (it would create root-owned dirs the server cannot write);
`/etc/tmpfiles.d/styrman-firewall.conf` is GENERATED by `install.sh` from
the state-dir owner: `d /run/styrman-firewall 0755 root root -` and `d
/run/styrman-firewall/spool 0700 <uid> <gid> -`. The spool is additionally a
dedicated tmpfs (`run-styrman\x2dfirewall-spool.mount`, also generated:
`size=1M,nr_inodes=1024,mode=0700,uid=,gid=,nosuid,nodev,noexec`, mounted at
boot before docker) so the server's uid can never fill `/run` through it
(security review medium). A server container that bound the run dir before
the mount existed must be restarted once to see it.

**`deploy/firewall/install.sh`** (root, idempotent, narrow): flags
`--state-dir` (default `${STYRMAN_STATE_DIR:-/var/lib/styrman}`),
`--lan-dev`, `--lan-cidr` (repeatable), `--prefix` overrides for
`/usr/local/sbin`, the unit dir, `tmpfiles.d` and `/etc/styrman` so a root
test could target scratch paths (ADR-052), and `--dry-run` (prints every
action, writes nothing). Steps: `install -m 0755` the script; `install -m
0644` the three units; write the tmpfiles conf and `systemd-tmpfiles
--create` it; `mkdir -p netpolicy/lan` + non-recursive `chown` of BOTH
levels to the state-dir owner; write `firewall.conf` ONLY IF ABSENT from
flags or discovery (`ip -4 route show default` → dev; `ip -4 -o route show
dev $DEV scope link` → prefixes), refusing with a clear error when there is
no default route and no flag; `systemctl daemon-reload`; `systemctl start
styrman-firewall.service`; `systemctl enable --now styrman-firewall.timer
styrman-firewall.path`; print `iptables -S STYRMAN-FWD | head -12` and the
status file. In v1 `install.sh` itself is verified by `bash -n`, shellcheck
and a `--dry-run` run, plus the real Phase-1 converge on this host recorded
in the changelog — stated as such, not claimed as a test.

**`deploy/update/styrman-update` — `host_converge()`**: runs on every tick
after `verify_tag` and the CURRENT computation but before the "already up to
date" exit, and again after `self_install` in the deploy and rollback
branches. Gate: `REQUIRE_SIGNED=1`, `STYRMAN_FW_INSTALL_BIN` and
`STYRMAN_UPDATER_UNIT_DIR` set by the unit (ADR-052), and `git rev-parse
HEAD` equal to the verified SHA recorded in `/etc/styrman/update` — else
`log WARN host_converge skipped: <reason>` on EVERY tick (critic hole: loud,
not once). Then for `styrman-firewall.sh` (0755 → `$STYRMAN_FW_INSTALL_BIN`)
and the three units (0644 → unit dir): `differs && atomic_install` from
`git show <SHA>:deploy/firewall/<f>`; if anything changed: `daemon-reload`,
`enable --now` timer + path, `start` service. Never touches `firewall.conf`
or the tmpfiles conf. Non-fatal. The updater already builds and runs images
as root from the same verified tree, so this adds no new trust edge.

### Server side

```go
// internal/config/config.go
func (c *Config) NetPolicyDir() string { return filepath.Join(c.StateDir, "netpolicy") }
// Config.FirewallRunDir string — STYRMAN_FIREWALL_DIR, default "/run/styrman-firewall"; "" disables the
// feature (every project "unavailable", enable → 503); tests set it to t.TempDir().

// internal/netpolicy (stdlib only)
const MinScriptVersion = 3          // TestMinScriptVersionMatchesScript greps STYRMAN_FW_VERSION= from deploy/firewall/styrman-firewall.sh
const StaleAfter   = 3 * time.Minute
const PendingGrace = 90 * time.Second
const KickDedupe   = 10 * time.Second

type Status struct {
    Schema, Version int; AppliedAt time.Time; LanDev string; LanCidrs []string
    Applied map[string]struct{}; Pending, Errors map[string]string; ConfError string; Warnings []string; RunMS int
}
type State string // "off" | "pending" | "on" | "error" | "unavailable"

type Dir struct { /* markers, spool, statusPath, logger; mu sync.Mutex (see below); status cache (2 s); lastKick map[string]time.Time */ }
func New(cfg *config.Config, logger *slog.Logger) *Dir
func (d *Dir) SetMarker(uuid string, on bool) error          // takes d.mu; uuid validated with project's uuidRe; O_EXCL create / os.Remove; both idempotent
func (d *Dir) Sync(want func() (map[string]bool, error)) (added, removed int, err error)
    // takes d.mu for the WHOLE cycle: calls want() (the disk snapshot) INSIDE the lock, then adds missing markers and
    // removes uuid files not in want (Warn per removal); refuses a symlinked or non-dir markers dir; ignores
    // non-uuid names, dirs, symlinks (Warn). Because SetMarker also takes d.mu, a PUT can never create a marker
    // between Sync's snapshot and its delete pass (cold-reader gap 1): the only interleaving left is a store save
    // landing after the snapshot, which makes Sync skip a marker that does not exist yet — the PUT then creates it
    // after Sync releases the lock, and the next Sync sees the saved record. Store saves are atomic (tmp+rename),
    // so want() always reads a consistent projects.json.
func (d *Dir) Kick(tag string) bool                          // no lock; O_EXCL create in the spool; per-tag 10 s dedupe; false on error (Warn at most hourly)
func (d *Dir) Status(now time.Time) *Status                  // nil when absent / unparseable / > 64 KiB; 2 s TTL cache
func ParseStatus(r io.Reader) (*Status, error)
func Availability(st *Status, now time.Time) (ok bool, reason string)
    // nil → "host firewall support is not installed — run deploy/firewall/install.sh on the server"
    // AppliedAt older than StaleAfter → "host firewall timer has not run for <N>m"
    // Version < MinScriptVersion → "host firewall script is outdated (have N, need M) — run deploy/firewall/install.sh"
    // ConfError → it; len(LanCidrs)==0 → "no local-network prefix configured (/etc/styrman/firewall.conf)"
func Derive(lan bool, changedAt time.Time, uuid string, st *Status, now time.Time) (State, string)
```

`Derive` table (critic: never render "off" while root's last word says the
rule exists):

| record | status | result |
|---|---|---|
| any | file absent or unparseable | lan ? `error` "cannot confirm the host firewall's state — no readable status file, and the rule may still be present" : `off`. (Security review of Phase 2: reporting a record-on project as "unavailable" here was fail-OPEN — the bind or dir can vanish while the rule stays live. Also: an empty `STYRMAN_FIREWALL_DIR` now removes every marker at boot so "disabled" is true for existing grants.) A future-dated `applied_at` (clock skew) counts as stale. |
| any | STALE (present, old) and uuid ∈ `Applied` | `error`: "cannot confirm closed — host firewall not running (rule may still be present); journalctl -u styrman-firewall" |
| true | uuid ∈ `Applied` (all CIDRs) | `on` |
| false | uuid ∉ `Applied` | `off` |
| any | `Errors[uuid]` | `error` + text |
| true | `Pending[uuid] == "no-bridge"` | `pending`: "applies when the project next starts" |
| mismatch, `now-changedAt < PendingGrace` | | `pending`: "applying on the host firewall" / "closing on the host firewall" |
| mismatch otherwise | | `error`: "the host firewall has not applied this change for Ns — journalctl -u styrman-firewall on the server" |

```go
// internal/project/store.go — beside SharedPorts
LanAccess    bool      `json:"lanAccess,omitempty"`
LanChangedAt time.Time `json:"lanChangedAt,omitzero"`          // go 1.25: omitzero
func (s *Store) SetLanAccess(id string, on bool, now time.Time) error
// copy → set → LanChangedAt = now only when changed → save() → roll back on error (the SetPortShares shape).
// Load: lanAccess:true with a zero LanChangedAt gets LanChangedAt = load time, so the pending→error clock is defined.
```

**Routes** (new file `internal/httpapi/network.go`; registered in the
project block of `RoutesWithAuth`; `isolation_test.go`'s route list gains
both):

```
GET /api/projects/{project}/network
  200 {"lanAccess":false,"lanState":"off","lanMessage":"","lanChangedAt":"…"(omitted when zero),
       "host":{"available":true,"reason":"","version":3,"requiredVersion":3,"appliedAt":"…",
               "lanDev":"enp114s0","lanCidrs":["192.168.86.0/24"],"lanProjects":1,"warnings":[]}}
  `host` only for admins (non-admins do not learn the operator's LAN prefix); arrays never null.
  404 — getProjectFor fails (unknown id, or another user's project — before any authz signal leaks).

PUT /api/projects/{project}/network   body {"lanAccess": true|false}
  decoded into struct{ LanAccess *bool }; nil / bad JSON → 400 {"error":{"message":"lanAccess (boolean) required"}}
  404 — not the requester's project
  403 {"error":{"message":"admin only"}} — requireAdmin, ENABLE only; DISABLE by the owner always proceeds
  503 — ENABLE only, netpolicy.Availability false, reason verbatim
  500 — SetLanAccess persist failure; or marker write failure after the store saved (record rolled back)
  200 — the GET body; lanState is "pending" right after a change, never "on" from the request itself; idempotent — a no-op PUT re-kicks THROTTLED (10 s per project; security review: a tenant looping disable must not drive root's reconciler); a non-admin owner gets generic messages (root's texts name the LAN prefix)
  Order, under s.netMu (server-level mutex — critic: two overlapping PUTs could otherwise leave record OFF with the marker PRESENT):
    validate → 404 → authz → availability → SetLanAccess → SetMarker (rollback on error) → Kick → audit → respond
  audit: s.logger.Info("lan access changed", "by", email, "project", id, "projectName", name, "lanAccess", on, "kicked", kicked)
  No orchestrator/Docker call: works running, stopped or absent. csrfGuard + 1 MiB body cap as for every non-GET.
```

`projectView` (list + get) gains `lanAccess` (omitempty), `lanChangedAt`
(omitzero), `lanState`, `lanMessage` (omitempty; absent only when off with
nothing applied and no error), all from ONE cached `netpol.Status` per
request — this is what carries state to the sidebar through the existing
5 s shared poll. `GET /api/settings` gains `hostFirewall` (same object as
`host`) for admins only. `PATCH /api/projects/{id}` stays rename-only.

**Boot / runtime reconciliation** (`cmd/styrman/main.go`): define once
`wantLAN := func() (map[string]bool, error) { want := map[string]bool{};
err := forEachProject(cfg, logger, func(u string, p project.Project) {
want[p.ID] = p.LanAccess }); return want, err }` — it opens fresh `Store`
instances from disk (the existing `forEachProject`), which is a consistent
snapshot because every store save is tmp+rename. At boot, after
`ensureRepos`, BEFORE the listener (so it precedes any `userSpace` open):
`netpol.Sync(wantLAN)`; `netpol.Kick("boot")`. A user whose `projects.json`
fails to load contributes nothing to `want`; their markers are REMOVED and
each removal is logged at Warn with the reason (fail-closed and loud —
critic hole). The Adopt goroutine kicks once more after
`EnsureSelfAttached`. **Runtime re-sync (critic):** a DEDICATED
`time.NewTicker(2 * time.Minute)` goroutine in `main.go` (NOT the
orchestrator's `RunIdleReaper`, which knows nothing of stores or
netpolicy) calls `netpol.Sync(wantLAN)` so record and marker converge
without a restart. Consistency with concurrent PUTs is guaranteed by
`Dir.mu` as described under `Sync` above — the snapshot is taken inside the
same lock `SetMarker` needs, so no marker can be created between the
snapshot and the delete pass.

**Kicks after boot** (`ensureFor`, `createProject`, `startProject`): only
when the record has `LanAccess` AND the cached status does not already list
the uuid as applied (i.e. `pending:no-bridge`) — plus `Kick`'s 10 s per-tag
dedupe — so a terminal reconnect never costs a root reconcile (critic hole).

**Destroy** (`deleteProject`, `internal/httpapi/server.go:882-983`): the
marker removal goes at ONE point — the statement immediately BEFORE
`sp.projects.Remove(p.ID)`, i.e. AFTER the optional wipe block and its
`wipeFailed` early return (cold-reader gap 2). A failed wipe therefore
keeps record, container state AND marker together, so a project kept alive
for a retry keeps its LAN access; only a delete that is about to remove the
record removes the marker. There: `netpol.SetMarker(p.ID, false)` +
`netpol.Kick("delete")`; Warn on failure, never block the record removal
(the boot/2-min Sync GCs a leftover; the rule keys on an interface that no
longer exists and matches nothing).

### UI

`api.ts`: `Project` gains `lanAccess?: boolean; lanChangedAt?: string;
lanState?: "off"|"pending"|"on"|"error"|"unavailable"; lanMessage?: string`
(and the missing `sharedPorts?` while the type is open); `Settings` gains
`hostFirewall?`; verbs `projectNetwork(id)` and `setProjectLan(id, lanAccess)`
beside `renameProject`.

`ui.tsx`: a shared `Switch` after `TextInput` — `<button role="switch"
aria-checked aria-label title>` with a `bg-warn` track when on, `bg-border`
off, `disabled:opacity-40`, Space/Enter toggle (no `role="switch"` exists
today; one family per control).

`SidePanelPorts.tsx`: a "Local network" block at the top of `PortsTab`,
reading `useProjects().data?.projects.find(p => p.id === project)` (works
while STOPPED). After a PUT it keeps the response as a local override until
the shared poll reports a newer `lanChangedAt` (the `PortShare` pattern),
then `refresh()`. Mutation through `tryUI`. Enable goes through
`ConfirmDialog` `danger`; disable needs no dialog. States and copy (dot +
one `text-[11px] text-text-dim` line; rendered from the SERVER's `lanState`,
never from the request lifecycle):

- loading: "Loading…"
- off: "Internet only (default). This project cannot reach your local network, this server, or other projects."
- on-pending: switch checked + disabled, dot `bg-warn animate-pulse`; "Applying on the host firewall — usually under a second, up to a minute." / no-bridge: "Saved — applies when the project next starts."
- on: dot `bg-warn`; "Can reach 192.168.86.0/24 by IP (LAN names and .local do not resolve). Still blocked: this server, other projects, cloud metadata. Applied 12:01:05."
- off-pending: switch unchecked + disabled; "Closing on the host firewall — open LAN connections drop immediately."
- error: switch reflects `lanAccess`; `text-danger` "Host firewall reported: <lanMessage>" + a text Button "Retry" that re-PUTs the same value.
- unavailable: switch disabled; "Not available on this server: <reason>".
- non-admin owner: switch disabled, title "Only an operator (admin) can allow local network access."

Confirm dialog (enable): title `Allow local network access for "<name>"?`;
body: "Everything this project runs — the agent, its shells, its Docker
containers — will be able to open connections to devices on
192.168.86.0/24, the operator's LAN, by IP address. This project holds your
git, SSH and cloud credentials through the broker; any LAN machine that
trusts them becomes reachable from an AI agent. This server, other projects
and cloud metadata stay blocked, and LAN hosts still cannot connect in.
Turning it off later cuts open connections immediately." Confirm label
"Allow LAN access". Toasts: "Local network access requested — applying on
the host" / "Local network access removed — open LAN connections were cut".

`Sidebar.tsx` row (beside `deleteError`/`provisionError`, `text-[10px]`):
on → `LAN` in `bg-warn-soft text-warn`, title "Local network access allowed:
192.168.86.0/24"; pending → same, `animate-pulse`, title "applying…"; error
→ `text-danger`, title = `lanMessage`; off/unavailable → nothing. The
dashboard card (`routes/index.tsx`) shows the same word. Derivation lives
in `web/src/lib/lan.ts` (`lanLabel(p)`) so vitest covers it.

`settings.tsx`: admin-only "Host firewall" section next to Runtime — Script
version ("3" / red "outdated (have N, need M) — run deploy/firewall/install.sh
on the server" / "not installed"), Last applied (relative; red "timer not
running" when stale), Local network prefixes + device ("192.168.86.0/24 via
enp114s0 — from /etc/styrman/firewall.conf"), Projects with LAN on, Warnings.

## R12 amendment (needs the operator's approval — D1)

Append to R12 after "See ADR-015.", keeping the old wording as history per
the R5 convention; in the same edit correct line 78's "RFC1918 + metadata"
to "RFC1918, CGNAT/benchmark (100.64/10, 198.18/15) + metadata" (the script
drops six ranges; the text undercounts):

> Local-network exception (ADR-065, operator approval <DATE>; was: the
> private-range egress drop applied unconditionally to every project): an
> admin may allow individual projects, each toggled separately, running or
> stopped, to reach the host's local network — exactly the IPv4 prefixes the
> operator configured in the root-owned `/etc/styrman/firewall.conf` (this
> deployment: 192.168.86.0/24 via enp114s0), never wider than a /16 and
> never overlapping 172.16/12, 169.254/16, 100.64/10, 198.18/15 or any
> Docker bridge. The allowance is a per-bridge RETURN inserted ahead of the
> drops by the same root-owned, self-healing firewall service, keyed on that
> project's exact bridge interface, its bridge subnet and the uplink device,
> driven by an intent file the server writes and root validates by name
> only; it is off by default, visible in the UI while on, revocable at
> runtime, removed with the project, and disappears fail-closed whenever
> that service is absent. Everything else stays dropped for that project
> too: the host itself (INPUT drop — it runs sshd and Caddy and the project
> holds its owner's credentials), host-published Docker ports (the drop sees
> the post-DNAT container address), every other project and system bridge,
> cloud metadata and the tailnet; LAN hosts still cannot initiate
> connections into any project. The cross-project guarantee in the first
> sentence, `ip_forward=0` and the host-networking clause are unconditional.

ADR-065 (Context / Decision / Consequences / Source / Verification, the
ADR-064 shape) plus an "ADR-065 — amendment" entry quoting the approval
verbatim (the ADR-062 shape); ADR-029/036 are superseded in place, never
edited.

## Failure modes

| Failure | What the user sees | What the API returns |
|---|---|---|
| Host half not installed (no status file; local dev) | every project `unavailable`; Settings "not installed" | ENABLE 503 naming the missing step; DISABLE 200 |
| Live script older than `MinScriptVersion` | `unavailable`; Settings row red "outdated (have N, need M)" | ENABLE 503 |
| Timer/service dead (status stale > 3 min) | projects that were `on` show `error` "cannot confirm closed"; others `unavailable` | ENABLE 503; DISABLE 200 but state stays `error` until a fresh status omits the uuid |
| Spool missing/unwritable | `pending` → applied by the timer ≤ 65 s; one hourly server Warn | 200 "queued for the 60 s reconciler" |
| `.path` unit failed or job-deleted at boot | latency degrades to the timer; the script revives it on its next run | — |
| Toggle while the script is mid-run | never lost: marker before spool; post-drain entries re-trigger on exit | 200 pending |
| External flush (dockerd restart, firewalld reload, manual `-F`) | base DROPs + allows restored together ≤ 65 s; project BLOCKED in the window; UI on→pending→on | — |
| Host reboot | tmpfiles recreates `/run/styrman-firewall`; OnBootSec=15s reconciles from markers; boot Sync re-derives markers; Adopt kicks again once bridges exist | — |
| `firewall.conf` invalid / overlapping / uplink is a Docker bridge | ALL allows deleted; every LAN-on project `error` with `conf_error` text | ENABLE 503 |
| Project absent (no bridge, `-s` unknown) | `pending` "applies when the project next starts"; kick after boot installs it ~1 s later | 200 pending |
| Network recreated with a new subnet | reconciler sees a different `-s`, replaces the rule on the next kick/tick | — |
| One iptables call fails mid-run | `error=<uuid>:<cidr>:<text>` → `error` for that project; next tick retries | — |
| `projects.json` save fails | nothing changes | 500 "cannot persist setting" |
| Marker write fails after the store saved | record rolled back | 500 "saved, then rolled back: the host firewall could not be signalled" |
| Server crash between store save and marker write | `pending` → `error` after 90 s; the 2-min re-sync or the next boot re-projects it; Retry re-PUTs | — |
| Toggle-off or destroy with open LAN flows | timeouts, no RST (stated in the copy) | — |
| Two overlapping PUTs | serialised by `netMu`; last writer wins consistently | 200 each |
| PUT racing the 2-min `Sync` | `Dir.mu` makes the snapshot + delete pass atomic against `SetMarker`; the toggle is never reverted | 200 pending |
| Delete with `wipe=true` whose wipe fails | project stays (with `deleteError`) and KEEPS its LAN access and marker; a later successful delete removes both | DELETE's existing 202/error path |
| Compromised server (uid 1000) | can mark any real uuid on and fill the spool; cannot widen CIDRs, touch STYRMAN-IN, inject rule text, forge status, or redirect root's delete | — |

## Verification

Tiers this feature needs: unit (Go + vitest + the netns script test under
`go test`), integration (`integration.sh`, deterministic scratch-dir checks +
the opt-in live block), e2e (Playwright, the `unavailable` branch), and a
MANUAL end-to-end on the converged production host, logged.

- **Tier 1 (Go, `t.TempDir`, no Docker, no root; new files only):**
  `internal/config/bridgename_test.go` — golden vector
  `sdb-3e6a772c-b177-4cfc-b209-b14db514bac2 → sdb-e660d58459e` (the SAME
  vector `deploy/firewall/test.sh` asserts, locking the Go and shell
  derivations together; the `echo`-newline trap recorded in a comment) +
  the setup-network case. `internal/project/store_lan_test.go` — round trip
  through a second `NewStore`, `LanChangedAt` only on change, field absent
  from the bytes when off, `ErrNotFound`, rollback on save failure, no
  `.tmp` left. `internal/netpolicy/*_test.go` — SetMarker create/remove
  idempotent + rejects non-uuid; Sync adds/removes/ignores symlink, dir,
  `sys-setup-x`, refuses a symlinked markers dir; Kick `O_EXCL` + dedupe +
  false on a missing spool; ParseStatus fixtures (missing, > 64 KiB,
  garbage, stale, `version=3` vs `Min=4` for the outdated branch — critic:
  the live host has NO status file, so it is "not installed", never "have
  1"); Availability + Derive over every (lan, applied, pending, error,
  stale, age) combination; `TestMinScriptVersionMatchesScript`.
  `internal/netpolicy/script_test.go` — skips unless `unshare -Urnm sh -c
  'mount -t sysfs sysfs /sys'` succeeds and `/usr/sbin/iptables` exists
  (critic: a plain `-Urn` netns still shows the HOST's `/sys/class/net`);
  runs `deploy/firewall/test.sh` under `unshare -Urnm` with sysfs remounted
  and asserts `/sys/class/net` holds only `lo` + the dummies first. test.sh
  proves: env unset → base rules only, no status; base idempotent (rule
  count identical across two runs); one marker → line 1 is the exact RETURN
  (TEST-NET-1 `192.0.2.0/24` as scratch LAN, dummy `enp0` and
  `sdb-e660d58459e` with routes), physdev next, six DROPs after; second run
  zero writes; marker removed → rule gone, a duplicated rule fully purged;
  malformed comment line deleted by number; symlinked marker / non-uuid /
  dir / 35-char name ignored with `warn=`; spool consumed incl. a
  symlink-to-dir entry (target untouched) and a nested dir; spool drained
  even when the conf guard fails and when the base section is forced to die
  (critic); `-F STYRMAN-FWD` then rerun restores base + allows in order; a
  RETURN appended after the DROPs is moved to the head; conf cases (missing,
  non-root-owned ignored, `.7/24` host bits, bridge overlap, 169.254/16,
  100.64/10, prefix < 16, kernel-bridge uplink → warning only, Docker-bridge
  uplink → conf_error); missing bridge → `pending=…:no-bridge`, bridge
  appears → installed with its subnet, subnet change → replaced; status
  grammar complete incl. `error=<uuid>:<cidr>:…` on a partial failure;
  `bash -n` + shellcheck clean; and, ONLY when `sudo -n true` succeeds, the
  host's `iptables -S STYRMAN-FWD` byte-identical before/after (else a
  printed SKIP — critic hole). `internal/httpapi/network_test.go` —
  `testRoutes`/`guestTest` pattern with `cfg.FirewallRunDir = t.TempDir()`:
  400s, unknown 404, another user's project 404 + no marker, non-admin 403
  on enable / 200 on disable, no status → 503 + store unchanged, outdated →
  503, fresh status → 200 pending + marker + spool + store + audit line,
  status listing the uuid → `on`, error line → `error`, stale status +
  applied → `error` "cannot confirm closed", DELETE removes the marker,
  settings `hostFirewall` for admins only, two concurrent PUTs end
  consistent. vitest: `web/src/lib/lan.test.ts`. `deploy/verify-commit.sh`
  gains `bash -n` + conditional shellcheck over `deploy/firewall/*.sh` and
  `systemd-analyze verify` over the three units (critic).
- **Tier 2 (`integration.sh`, new block after the ports section; the
  self-hosted server gets `STYRMAN_FIREWALL_DIR=$STATE/fw`):** the FIRST
  scripted R12 negative check — `docker exec $CN ping -c1 -W2 <default gw>`
  FAILS, gated on `sudo -n iptables -S STYRMAN-FWD` and the host itself
  pinging the gateway, else SKIP with reason; PUT true → 503 (no status,
  fail-closed), no marker; PUT false → 200; craft `$STATE/fw/status` (fresh,
  `version=3`, `lan_cidrs`) → PUT true → 200 pending, marker + spool entry
  present, list carries `lanAccess`/`lanState`; DELETE → marker gone.
  Opt-in `STYRMAN_IT_LIVE_LAN=1` (D7): marker + spool via `sudo -n` → within
  5 s the head rule is the RETURN for this project's iface with `-s <bridge
  subnet>`, in-guest ping to the gateway SUCCEEDS, ping to the host's LAN IP
  still FAILS; remove → within 5 s the rule is gone and the ping fails.
  README Tier 2 count and auto-SKIP count re-measured.
- **Tier 4 (Playwright, `run.sh` sets `STYRMAN_FIREWALL_DIR` to scratch):**
  "Local network" block visible with `[role=switch]` disabled and the "Not
  available on this server" copy; `page.request.put(...)` → 503; Settings
  renders Host firewall as not installed; no native dialog. Count
  re-measured.
- **Manual on the converged production host (Phase 3, logged in
  `.agents/docs/testing/2026-09-XX-verify-lan-toggle.md` with measured
  latency):** toggle on → chip pending → on; in-guest `ping 192.168.86.1`
  succeeds, `ping 192.168.86.58` fails, `curl` to the other project's bridge
  IP fails; `systemctl start styrman-firewall.service` keeps the rule; stop
  → start keeps it; toggle off → the ping fails within a second; delete →
  rule gone.

**E2E criteria.** Golden path: an admin opens a running project's Ports
tab, flips "Local network" on, confirms the danger dialog, sees pending
then on within a few seconds, and a `ping 192.168.86.1` in the project's
terminal answers; flipping it off makes the next ping fail within a second.
Edge case: the same switch on a host without the reconciler is disabled with
"Not available on this server: host firewall support is not installed", and
`PUT` answers 503 — nothing opens.

## Phases

Model note for the operator: Phases 0, 1 and 2 are the security-critical
ones (root-run script, R12 enforcement, authz); Phases 3–5 are routine.

### Phase 0 — Decision record + drift converge (no feature code)
- [x] Operator answers D1–D7 ("Go do work", 2026-09-08 — all seven recommendations) — verify: reread, user
- [ ] **OPERATOR (root; the agent's attempt was blocked by the auto-mode classifier):** converge the live host's existing drift NOW: copy `deploy/host-setup.sh:167-232` over `/usr/local/sbin/styrman-firewall.sh`, `systemctl start styrman-firewall.service`, confirm six DROPs in `sudo iptables -S STYRMAN-FWD` — a security gap independent of the feature — verify: `iptables -S` shows 100.64/10 and 198.18/15, AI; record in a `changes/` entry
- [x] ADR-065 + amendment entry in `DECISIONS.md`, R12 edit in `REQUIREMENTS.md` (approval quoted verbatim), ROADMAP "Now" → this plan, in ONE commit (`9aff5d6`) — verify: reread, both
- [x] Fix pre-existing doc drift the readers found: `deploy.md:582` four→six ranges; "60 checks" in `deploy/local.md:40` and `guide.md:520` now point at the README (`dbe66a8`) — verify: grep, AI
- [x] Cold-reader check of this plan (`reviewer`, plan file only) → both "ready" tokens on round 2 (`reviews/2026-09-08-cold-reader-lan-access-plan.md`) — verify: review file

### Phase 1 — Host half (root script, units, installer, updater) — security-critical
- [x] `deploy/firewall/styrman-firewall.sh`: version constant, env guards, blocking flock, drain-first + drain-on-failed-exit, base section verbatim, conf parse/validate, desired/current, delete-by-number, insert-if-missing, status, path self-heal, change-only log — verify: tier 1 netns test via `go test ./internal/netpolicy/` (83 checks), AI
- [x] `deploy/firewall/{service,timer,path}` per Interfaces (no `After=` on the path, no `Wants=` on the service) — verify: `systemd-analyze verify` clean, AI
- [x] `deploy/firewall/test.sh` (every case under Verification) + `internal/netpolicy/script_test.go` wrapper (`unshare -Urnm` + sysfs remount, skip-not-fail; host chain compared before/after via `sudo -n`) — verify: `go test ./...` on this host runs it (4.8 s), not skips; the verifier states the skip status
- [x] `deploy/firewall/install.sh` (flags, `--dry-run`, tmpfiles, both-level chown, conf discovery ONLY IF ABSENT, drop-in for a non-default state dir) — verify: `bash -n`, shellcheck, `--dry-run` output reviewed (discovers enp114s0 / 192.168.86.0/24 here), AI
- [x] `deploy/host-setup.sh` §3b → a pointer; the `install.sh` call became §5c AFTER the state dir exists and is chowned (the installer needs both), so `SCRIPT_DIR` did not have to move — verify: `bash -n`, diff shows the heredoc removed and nothing else, AI
- [x] `deploy/update/styrman-update`: `host_converge()` + unit env + WARN-every-tick skip; the two early-exit blocks moved below the function definitions unchanged — verify: tier 1 `bash -n`/shellcheck; reasoned reread, AI
- [x] `deploy/verify-commit.sh`: shell lint + `systemd-analyze verify` steps — verify: run it (Phase 1 checkpoint), AI
- [x] Measure spool→rule latency with a transient `systemd-run --user` path unit on a scratch dir (not the live units): trigger 30–62 ms over five runs, script run 80–120 ms in the netns → a toggle lands in about 0.2 s — verify: number in the changelog, AI
- [x] `security` agent over the script + installer + updater diff (1 high, 3 medium, 7 low) and `reviewer` (1 high, 1 medium, 2 low): all fixed except three rebutted in writing — see the Phase 1 `changes/` entry — verify: findings fixed or rebutted in writing
- [ ] Converge THIS host with `sudo bash deploy/firewall/install.sh` run from `/opt/styrman` at the Phase-1 commit (never the working tree); record status file present, `version=3`, `applied_at` advancing, zero projects, `systemctl status styrman-firewall.path` active — verify: outputs pasted in the `changes/` entry, both
- [ ] Changelog "host-only, applied live", commit, push — checkpoint

### Phase 2 — Server half — security-critical (authz, root-read input)
- [x] `config`: `NetPolicyDir`, `FirewallRunDir`/`STYRMAN_FIREWALL_DIR`; `main.go` creates `netpolicy/lan` — verify: tier 1, AI
- [x] `internal/config/bridgename_test.go` golden vector — verify: tier 1, AI
- [x] `internal/netpolicy`: Dir, SetMarker, Sync, Kick + KickThrottled, Status/ParseStatus, Availability, Derive + tests (incl. the Sync-vs-SetMarker lock test) + `TestMinScriptVersionMatchesScript` — verify: tier 1 `-race`, AI
- [x] `project.Store.SetLanAccess` + load-time `LanChangedAt` + `store_lan_test.go` — verify: tier 1, AI
- [x] `main.go`: `wantLAN` snapshot func; boot `Sync(wantLAN)` + `Kick("boot")` before the listener; Adopt kick; a dedicated 2-min ticker goroutine calling `Sync(wantLAN)` — verify: tier 1 for Sync incl. a test that a `SetMarker` issued while `Sync` holds the lock is never deleted by that cycle; reread for placement, AI
- [x] `httpapi/network.go` GET/PUT under `netMu`, 503 gate, audit; routes; `isolation_test.go` list; `network_test.go` (5 tests incl. 20 concurrent PUTs and the stale-status "cannot confirm closed" case) — verify: tier 1, AI
- [x] `projectView` `lanState`/`lanMessage` from one cached status; settings `hostFirewall` (admin) — verify: tier 1, AI
- [x] `deleteProject` marker removal + kick immediately before `sp.projects.Remove` (after the wipe block's early return); conditional kicks after `EnsureRunning` in `ensureFor`/`createProject`/`startProject` — verify: tier 1 for delete (`TestDeleteProjectRemovesMarker`; the failed-wipe case cannot be driven without a Docker double — stated); tier 2 live block / manual for the kicks, AI
- [x] Compose volume + env; `api.md` rows; server env table — verify: reread, AI
- [x] `integration.sh` block (R12 negative check, scratch-dir checks, opt-in live block) + README Tier 2 counts (82; 4 auto-SKIP) — verify: `bash integration.sh` 78 passed / 0 failed / 4 skipped on 2026-09-08 (the R12 negative check PASSED live); the live block runs once the operator has installed the host half
- [ ] `reviewer` (diff only) + `security` — verify: findings fixed or rebutted
- [ ] Changelog, commit, push — checkpoint

### Phase 3 — UI
- [x] `api.ts` types (+ `sharedPorts`) and verbs; `lib/lan.ts` + `lan.test.ts` (11 tests) — verify: tier 1 `npm test` 80/80, AI
- [x] `ui.tsx` `Switch` — verify: tier 1 check/lint; tier 3 keyboard, AI
- [x] `components/LocalNetwork.tsx` (rendered at the top of `SidePanelPorts`): six states, ConfirmDialog danger, Retry, override-until-poll; reads `GET /network` every 5 s so it works on a stopped project — verify: tier 3 + e2e, both
- [x] Sidebar + dashboard `LAN` chip (`components/LanBadge.tsx` over `lib/lan.ts`) — verify: tier 3, both
- [x] Settings "Host firewall" section (admin) — verify: tier 3, AI
- [x] e2e checks (unavailable branch with the switch disabled, 503 with nothing persisted, Settings not-installed row) + README Tier 1/4 counts (80 tests / 8 files; 86 checks + gate); `web/dist` rebuilt by the run — verify: `bash web/e2e/run.sh` 84 passed / 0 failed / 2 skipped on 2026-09-08 (the first run caught a real gap: with no status file the record derives "off", so the switch was enabled until `host.available` was wired into the block), AI
- [ ] Manual end-to-end on production (the Verification list) with measured latency, logged in `testing/` — verify: log file, both
- [ ] `reviewer` if ≥ 50 lines (it will be) — verify: findings fixed or rebutted
- [ ] Changelog, commit, push — checkpoint

### Phase 4 — Delivery + docs
- [x] `remote.md` Prerequisites/Procedure/Verify (install.sh step; `sudo iptables -S STYRMAN-FWD | head`, `systemctl status styrman-firewall.path`, `cat /run/styrman-firewall/status`); `versioning.md` boxed host note; `deploy/README.md`; `deploy/.env.example` — verify: reread, AI
- [x] `deploy.md` isolation section (the exception, six ranges, "styrman-lan-tagged RETURNs are the ONE class of rule the script deletes on demand", install.sh + host_converge, reserved keys); `guide.md` §2/§4 + troubleshooting row; `README.md` "Hard isolation" bullet; `AGENTS.md` architecture bullet + counts; testing README ADR-052 section (netns test); `guest-writable-paths.md` one line — verify: `documentation` agent (after the Phase 2/3 reviews), reread, AI
- [x] `TODO.md` follow-ups (list under Scope) + the correction to the 2026-09-02 "DONE" item that production never received — verify: reread, AI
- [ ] Changelog, commit, push — checkpoint

### Phase 5 — Verification + release
- [ ] `verifier` log naming which tiers ran with the host side installed, the test-count before/after, and the measured latency — verify: log in `testing/`
- [ ] `/preflight` → signed tag → updater deploys; next tick `host_converge` is a no-op (install.sh already ran) — verify: `journalctl -u styrman-update`, both
- [ ] Post-release: fresh sign-in; one project toggled on → ping → off end-to-end; release changelog with "What was NOT checked" (Docker live-restore preservation; the published-port DNAT shape — both deliberately outside the allowance); handoff — verify: both

## Risks

- **Blast radius of "local network".** A LAN-enabled project's agent holds
  its owner's SSH key and tokens; every LAN machine that trusts them is
  reachable. Mitigated by admin-only enable, the danger dialog naming it, a
  loud chip, and the host itself staying closed. Widening is a decision,
  never a default.
- **New root-parsed input surfaces**, bounded but real: root reads NAMES in
  an operator-owned dir and deletes inside a root-parented spool; the
  `security` agent reviews the script; the netns test carries the hostile
  cases. Deletion by rule number keeps `-S` text out of any command line.
- **Root code ships by signed tag** (`host_converge`): no new trust edge
  (the updater already builds and runs images as root from the same
  object), but a hostile signed tag can now rewrite the firewall script; the
  allowed-signers file is the control.
- **LAN definition is host-derived once**: a VLAN-only, PPPoE /30 or
  VPN-default-route host gets a wrong or empty prefix; `install.sh` prints
  the choice, Settings shows the effective prefixes, the conf is editable —
  a wrong guess is visible, not prevented.
- **Docker's fallback pool** (192.168.0.0/16 in /20s after fifteen 172.x
  /16s — inferred from Docker defaults; pools are unset here) could hand a
  16th project a subnet inside a broad LAN CIDR; overlap refusal turns that
  into a visible `conf_error` that takes LAN access down for ALL projects.
  Pinning `default-address-pools` is a follow-up.
- **Rule-order coupling**: a future base edit that `-I`nserts a DROP at
  position 1 silently disables the feature; the netns test asserts order.
- **Rendering coupling**: the strict `-S` regex is pinned for iptables
  1.8.11; a future rendering change would classify our own rules as
  "not ours" → delete + re-insert every tick (visible churn in the log, a
  packet-drop blip per tick for live flows). The version constant and the
  test are the guard; a `warn=` line makes it visible.
- **Kicks after `EnsureRunning` have no Tier 1 test** (no orchestrator
  Docker double — `TODO.md`); covered by the opt-in live block and the
  manual check only, stated in the verifier log.
- **Pre-existing boot window** (no STYRMAN rules between dockerd
  auto-restarting VMs and OnBootSec=15s) is unchanged; running the base
  section `Before=docker.service` is a follow-up, unverified.
- **Docker live-restore across a daemon restart** preserving `DOCKER-USER`
  and `STYRMAN-*` is unverified; exposure is bounded to ≤ 65 s of
  LAN-closed, never LAN-open.
- **`unshare -Urnm`** may be unavailable on another host; the Go wrapper
  skips and the verifier must say so.
- **Hand-maintained test counts** live in four places and are already
  inconsistent; every tier this feature adds changes them.
