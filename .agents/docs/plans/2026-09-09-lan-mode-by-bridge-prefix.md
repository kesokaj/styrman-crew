---
owner: main
date: 2026-09-09
---
# Plan — LAN mode is the bridge interface prefix   (2026-09-09)

## Why

The operator asked on 2026-09-08 for "a toggle to enable local network traffic
per project", and on 2026-09-09 restated what that was supposed to mean:

> "I just wanted you to give the option to enable local lan access as normal
> docker container have. Not copying a lot of keys and setting up scripts etc.
> If lan on, give NETWORK access from the project to the networks accessible in
> the docker host. If sandbox on. Only allow internet and nothing else."

What shipped instead (ADR-065 in v1.23.0, ADR-066 in v1.24.0) is ~3,500 lines
across 15 commits: a marker directory the server writes, a `/run` spool it
kicks, a systemd path unit, a root reconciler with three rule families and a
status file, a version handshake, a rate limiter, two conf opt-ins, an sshd
hardening precondition, and a permanent SSH-key withholding latch. Thirteen of
the project's twenty bug records (BUG-008 … BUG-020) come from that machinery.
The feature has never been applied on real hardware — it is blocked behind
`install.sh --lan-allow-host 1`, which refuses while this host accepts sshd
password authentication.

**The design error, in one step.** ADR-065's context established that the
server has no `NET_ADMIN` and no iptables binary, so it cannot change a rule —
true — and concluded *therefore the server must ask root to change one*. Every
part listed above follows from that conclusion. The step not taken: the server
does not need to change a rule, it needs the project **not to be matched by
one**. Every styrman rule keys on the bridge INTERFACE name:

```
-A INPUT       -i sdb-+ -j STYRMAN-IN    -> DROP                (host closed)
-A DOCKER-USER -i sdb-+ -j STYRMAN-FWD   -> DROP private ranges (LAN closed)
```

and that interface name is chosen by the server, at `docker network create`,
via `com.docker.network.bridge.name` (`internal/config/config.go:493`) — an
authority it already holds on an operation it already performs.

**The second miss.** R12's cross-project guarantee is enforced by *Docker*, not
by styrman. Docker 29.7.2 writes one rule per bridge, verified live on this
host 2026-09-09:

```
-A DOCKER ! -i sdb-48cabdb8e84 -o sdb-48cabdb8e84 -j DROP
```

A new flow entering a bridge from anywhere but that bridge is dropped, whatever
the bridge is called. The styrman rules were only ever doing the OTHER job:
private-range egress and host access. Which is exactly the job "sandbox" wants
and "LAN" does not.

**The third.** Three review rounds landed on the same mechanism and each fixed
it rather than questioning it. `AGENTS.md` has that rule written down — "when a
SECOND round of findings lands on the same mechanism ... stop fixing and ask
what the mechanism is for". The one time it was applied (ADR-066's latch
amendment: "the highest open finding of the release gate is deleted rather than
fixed") produced the best outcome of the effort.

Operator approval for this plan's direction, 2026-09-09: "aspprove" [sic], on
the scope statement and the exact R12 replacement text, reproduced verbatim in Appendix A of this plan,
after answering two decisions: restart-on-toggle is acceptable, and the SSH-key
latch is to be deleted.

## Scope

**In:**
- A per-project network mode: SANDBOX (default) or LAN, chosen by the server
  through the bridge interface name only.
- Sandbox behaviour byte-identical to today, including the existing bridge
  interface names of every existing project.
- Deletion of `internal/netpolicy/`, the firewall reconcile machinery, the
  spool, the path unit, the status file, the version handshake, the conf
  opt-ins, the sshd precondition, and the `LanEverEnabled` SSH-key latch.
- R12 amendment (approved text in Phase 0) and ADR-067 superseding ADR-065/066.
- A one-shot cleanup of stale `styrman-lan*` rules and LAN conf keys on hosts
  carrying the v4 script.
- Live adversarial verification of cross-project isolation for a LAN project.

**Out:**
- Sandbox drop-list contents, `ip_forward=0`, the host-networking clause, the
  port-preview proxy, the credential broker's transport, IPv6 policy.
- Docker NETWORK names: they stay `sdb-<uuid>` in both modes. Only the kernel
  INTERFACE name varies. This is what keeps project listing, GC, the broker,
  `ports.go` and the ingress watcher untouched.
- Live toggling without a restart (explicitly traded away by the operator).
- Inbound reachability into a project. LAN remains NAT-only outbound.

**Follow-ups noted, not built:**
- `host_converge` in `deploy/update/` stays and keeps shipping firewall changes
  by signed tag; it simply stops being load-bearing for this feature.
- The `styrman-firewall.timer` self-heal (ADR-036) stays as-is.
- TODO.md's open firewall items that survive the deletion are re-checked in
  Phase 6 and struck if their subject no longer exists.

## Surfaces touched

| Path | Change |
|---|---|
| `REQUIREMENTS.md` | R12 amendment (Phase 0) |
| `.agents/docs/architecture/DECISIONS.md` | ADR-067 (Phase 0) |
| `internal/config/config.go` | `BridgeNameFor(netName string, lan bool)` |
| `internal/config/bridgename_test.go` | golden vectors, both modes |
| `internal/orchestrator/orchestrator.go` | `RunOpts.Lan`, `createSpec.lan`, bridge-name mismatch heal in `create()`, `recreateMissingNetwork` |
| `internal/project/store.go` | `SetLanAccess` simplified; delete `LanEverEnabled`, `LanChangedAt`, `RestoreLanAccess` |
| `internal/httpapi/network.go` | 345 lines -> a small GET/PUT; delete `hostFirewall`, `ingressBridgeMismatch`, `lanFor`, `LanExposed`, `kickLAN` |
| `internal/httpapi/server.go` | delete `netpol` field, `NetPolicy()`, `lanFields`; strip `deleteProject`'s marker cleanup (`server.go:1019-1032`), KEEPING `netMu`; pass `Lan` in 3 `EnsureRunning` call sites |
| `internal/httpapi/explorer.go` | drop the admin-only `hostFirewall` field from the settings payload (`explorer.go:455-460`) |
| `web/src/routes/settings.tsx` | delete `HostFirewallSection` (62 lines, `settings.tsx:949`), its call site (`:856-857`) and its import |
| `internal/credbroker/broker.go` | delete `LanChecker`, `Broker.lan`, `lanExposed`, `isHostReachingFile`; `New()` loses its `lan` parameter |
| `internal/credbroker/broker_test.go` | delete `TestIsHostReachingFile` |
| `internal/credstore/credstore.go` | delete `IsHostReaching` (no caller survives) |
| `internal/httpapi/lanprune_test.go` | DELETE (347 lines — it tests only deleted mechanisms) |
| `cmd/styrman/main.go` | delete the marker-sweep goroutine; drop the `api` argument from `credbroker.New` |
| `internal/netpolicy/` | DELETE (6 files, 1,049 lines) |
| `deploy/firewall/styrman-firewall.sh` | 794 -> base rules only; add the one-shot stale-rule cleanup |
| `deploy/firewall/styrman-firewall.path` | DELETE |
| `deploy/firewall/install.sh` | strip LAN conf discovery/validation, the two opt-in flags, the sshd precondition, the spool dir |
| `deploy/firewall/test.sh` | strip the family/reconcile cases; keep base + cleanup cases |
| `deploy/docker-compose.yml` | delete `STYRMAN_FIREWALL_DIR` and the `/run/styrman-firewall` bind |
| `web/src/components/LocalNetwork.tsx` | 206 -> a mode switch with a rebuild notice |
| `web/src/components/LanBadge.tsx` | DELETE (22 lines) or reduce to a two-state badge; rendered on every project row |
| `web/src/components/Sidebar.tsx`, `web/src/routes/index.tsx` | the two `LanBadge` call sites (`Sidebar.tsx:289`, `index.tsx:100`) |
| `web/src/components/SidePanelPorts.tsx` | renders `LocalNetwork`; check its props still line up |
| `web/src/lib/lan.ts`, `web/src/lib/lan.test.ts` | DELETE (101 + 100 lines) — the five-state `off/pending/on/error/unavailable` model |
| `internal/httpapi/server.go` (projectView) | delete `LanState`/`LanMessage` (`server.go:625-626`) and the two `lanFields` fill sites (`:693`, `:916`) |
| `internal/httpapi/network_test.go` | 360 lines — rewrite for the new handler |
| `internal/project/store_lan_test.go` | 163 lines — drop the `LanChangedAt`/`RestoreLanAccess` cases |
| `internal/config/config.go` | also delete `STYRMAN_FIREWALL_DIR` handling |
| `deploy/verify-commit.sh` | drop `./styrman-firewall.path` from `systemd-analyze verify` (`:162`) and refresh the ADR-065 comment (`:146-154`) — **the gate script breaks otherwise** |
| `web/e2e/run.sh` | drop the scratch firewall dir setup (`:69`) |
| `web/src/lib/api.ts` | `lanView` shape; delete the `HostFirewall` interface (`api.ts:33`), `host?` (`:72`) and `hostFirewall?` (`:256`) |
| `.agents/docs/testing/styrman/integration.sh` | replace the LAN block |
| `web/e2e/run.mjs` | replace the two LAN blocks |
| `.agents/docs/deploy/remote.md` | sshd hardening becomes advice, not a precondition |
| `.agents/docs/specs/styrman/guide.md` | the network-mode section |

## Interfaces

### Bridge naming — the whole switch

```go
// BridgeNameFor is the Linux bridge INTERFACE name for a styrman network.
// The 11-hex suffix is a sha1 of the NETWORK name and does not depend on the
// mode, so the same project keeps one identity in `ip link` across a toggle
// and every EXISTING project's bridge name is unchanged by this release.
// The PREFIX is the network policy:
//   sdb-  matched by `-i sdb-+` in INPUT and DOCKER-USER -> host closed,
//         private ranges dropped (sandbox).
//   lan-  matched by no styrman rule -> a normal Docker container: NATs out
//         as the host and reaches whatever the host reaches.
// Cross-project isolation does not depend on either: Docker's own per-bridge
// `! -i <br> -o <br> -j DROP` covers both (R12 layer (a)).
func BridgeNameFor(netName string, lan bool) string {
	sum := sha1.Sum([]byte(netName))
	prefix := "sdb-"
	if lan {
		prefix = "lan-"
	}
	return prefix + hex.EncodeToString(sum[:])[:11]
}
```

Golden vectors pinned in `internal/config/bridgename_test.go`:
- `BridgeNameFor(NetworkName("3e6a772c-b177-4cfc-b209-b14db514bac2"), false) == "sdb-e660d58459e"` — the vector the live host already carries; proves no existing bridge is renamed.
- `BridgeNameFor(..., true) == "lan-e660d58459e"` — same suffix, different prefix.
- `BridgeNameFor(SetupNetwork("alice"), false)` keeps the `sdb-` prefix and 15-char length. The setup VM is never given `lan`.

The shell no longer recomputes this name (there are no per-project rules), so
`internal/netpolicy/script_test.go`'s Go/shell parity test is deleted with the
package. The Go golden vector stays, for the no-rename guarantee.

### Applying the mode

```go
// orchestrator
type RunOpts struct { Name, Dir, GuestUser string; Lan bool }   // + Lan
type createSpec struct { /* ... */ lan bool }                    // + lan
```

The mode is fixed at `docker network create`, so applying a change means
rebuilding the network, which means the project's container must not be
attached to it. `EnsureRunning` already has exactly this pattern **twice** —
the pre-ADR-027 workspace-bind recreate (`orchestrator.go:187-216`) and the
image upgrade (`orchestrator.go:206-216`) both remove the container and set
`err = errdefs.NotFound(...)` so the `case errdefs.IsNotFound(err)` branch
rebuilds it. A third condition of the same shape carries this feature, and
nothing else needs a new code path.

**Cold-reader round 1 rejected an earlier design that put the whole heal in
`create()`.** That was wrong and the correction is load-bearing: `create()` is
reached ONLY when the container does not exist (`orchestrator.go:247`). A
toggle on an existing project takes the `case err == nil:` reuse branch
(`orchestrator.go:229-246`), which calls `ContainerStart` and nothing else — so
the earlier design would have restarted the same container on the same bridge
and silently done nothing. `Stop()` (`orchestrator.go:918-951`) does not remove
the container either.

**(a) In `EnsureRunning`, a third force-recreate condition**, placed beside the
image-upgrade one and guarded identically:

```go
// LAN/sandbox mode (ADR-067): the mode IS the bridge interface prefix and it
// is fixed at network create, so a container attached to a network whose
// bridge name disagrees with the project's current mode has to be rebuilt.
// STOPPED containers only, exactly like the image upgrade above: recreating a
// running one would kill live terminal sessions. The toggle handler stops the
// project first for that reason, and a project stopped in the wrong mode is
// healed on its next start however it got there.
if err == nil && (insp.State == nil || !insp.State.Running) {
    if o.bridgeModeMismatch(ctx, config.NetworkName(project), opts.Lan) {
        if rerr := o.docker.ContainerRemove(ctx, insp.ID, container.RemoveOptions{Force: true}); rerr != nil {
            return "", fmt.Errorf("recreate for network mode: %w", rerr)
        }
        err = errdefs.NotFound(fmt.Errorf("recreated"))
    }
}
```

`bridgeModeMismatch` inspects the network and compares
`Options["com.docker.network.bridge.name"]` with
`config.BridgeNameFor(netName, lan)`. A missing network is NOT a mismatch (the
existing `recreateMissingNetwork` path owns that case); an inspect error is not
a mismatch either, so a Docker hiccup can never force-remove a container.

**(b) In `create()`, rebuild a network whose bridge name is wrong.** By the
time `create()` runs, the project's own container is gone (branch (a) removed
it, or it never existed), so the only endpoint left is the server's own
port-preview/broker attach:

```go
want := config.BridgeNameFor(netName, spec.lan)
insp, err := o.docker.NetworkInspect(ctx, netName, network.InspectOptions{})
switch {
case err == nil && insp.Options["com.docker.network.bridge.name"] != want:
    // Detach ourselves first: NetworkRemove refuses while ANY endpoint
    // remains, running or stopped. Same two calls Destroy already uses
    // (orchestrator.go:1108-1110), INCLUDING its guard — a server running
    // outside a container has no SelfContainer to disconnect.
    if o.cfg.SelfContainer != "" {
        _ = o.docker.NetworkDisconnect(ctx, netName, o.cfg.SelfContainer, true)
    }
    if rerr := o.docker.NetworkRemove(ctx, netName); rerr != nil {
        return "", fmt.Errorf("rebuild network %s for mode change: %w", netName, rerr)
    }
    o.invalidateSelfAttach(netName)
    fallthrough    // create it below under `want`
case errdefs.IsNotFound(err):
    // create with `want`
case err != nil:
    return "", fmt.Errorf("inspect network %s: %w", netName, err)
default:
    // reuse
}
```

`recreateMissingNetwork(ctx, containerID, netName string, lan bool)` takes the
flag the same way. Every network create in the package goes through one helper
so a future call site cannot forget the mode.

**The cost, stated plainly:** a toggle REBUILDS the project's container, it
does not merely restart it. The home and the workspace are bind mounts and are
untouched; packages the user installed replay from the ADR-028 apt manifest;
the container's own rootfs layer is discarded. This is the identical, already
accepted cost of the image-upgrade recreate above it, and the UI copy says so
at the point of toggling.

### API

```
GET  /api/projects/{id}            -> { ..., "lanAccess": bool }
PUT  /api/projects/{id}/network    <- { "lanAccess": bool }
                                   -> 200 { "lanAccess": bool, "rebuilt": bool }
```

- Enable requires admin acting on a project in their own store; the owner may
  always disable. (Kept from ADR-065 part 4 — it costs nothing and no admin
  path into another user's project exists or is added.)
- A no-op PUT (`lanAccess` already at that value) returns 200 and
  `rebuilt: false` without touching Docker.
- **Timeout budget.** The handler is synchronous and does `Stop()` (30 s
  grace) then `EnsureRunning` (container remove, network rebuild, container
  create, start). `startProject` (`server.go:1104`) already gives a plain
  start 2 minutes; a rebuild is strictly more work, so this handler takes
  `context.WithTimeout(r.Context(), 3*time.Minute)`. Stated because the first
  draft left it unspecified and the surrounding file ranges from 6 s to
  90 s — an implementer would otherwise have guessed.
- A real change writes the record, then: if the project is running, `Stop()` it
  and call `EnsureRunning` with the new mode (`rebuilt: true`); if it is
  stopped, return `rebuilt: false` — the mode applies at its next start,
  because the mismatch check in `EnsureRunning` runs on every start.
  The response field is `rebuilt`, not `restarted`, because that is what
  happens (see "The cost, stated plainly" above).
- 503 / version handshake / `pending` / `unavailable` / `error` states are all
  DELETED. There is no root half to be unavailable.

### What `netMu` is for after this change (a decision, not a deletion)

`s.netMu` (`server.go:117`) today serializes the store write + marker write +
kick in the toggle handler (`network.go:196`) against `deleteProject`'s marker
removal + record removal (`server.go:1025-1032`), so a concurrent PUT cannot
re-create a marker between its removal and the record's.

The markers go, but **the race does not**: a LAN-mode PUT still writes the
project record and then rebuilds a container, and a delete still removes the
record, and those must not interleave. `netMu` is therefore **kept**, with both
its comment and its job narrowed to: *serialize a mode change against a project
deletion and against another mode change.* Only the `SetMarker`/`Kick` calls
are removed from the two sites. Deleting the mutex along with the markers would
reopen a race a security review closed, which is the kind of collateral this
plan exists to avoid.

### Removed identifiers

`netpolicy.*` (whole package), `Server.netpol`, `Server.NetPolicy`,
`Server.lanFields`, `Server.hostFirewall`, `Server.ingressBridgeMismatch`,
`Server.lanFor`, `Server.LanExposed`, `Server.kickLAN`,
`Server.pruneHostReachingCreds`, `Server.ingressBr`, `Server.ingressBrMu`,
`Server.ingressBrAt`, `Orchestrator.BridgeNameForNetwork`,
`credbroker.LanChecker`, `credbroker.isHostReachingFile`, `Broker.lan`,
`Broker.lanExposed`, `credstore.IsHostReaching`, `Project.LanEverEnabled`,
`Project.LanChangedAt`, `Store.RestoreLanAccess`, `hostFirewallView`,
`STYRMAN_FIREWALL_DIR`. `credbroker.New` loses its `lan` parameter.

Three of these were missed by the plan's first draft and caught by the
cold reader, all for the same reason — they are reached through a mechanism
rather than named by it:

- **`pruneHostReachingCreds` (`network.go:270`) DELETES the SSH key from a
  project's home the moment LAN is enabled.** It is a SECOND withholding
  mechanism, independent of the broker latch, and the plan's Risk 4 (the
  operator's decision that a LAN project keeps its key) is contradicted
  outright if only the latch goes. It, its call site (`network.go:245`), and
  `lanprune_test.go` are deleted.
- **`Server.ingressBr` / `ingressBrMu` / `ingressBrAt` and
  `Orchestrator.BridgeNameForNetwork`** exist solely to feed
  `ingressBridgeMismatch`; they are dead the moment it goes.
- **`credstore.IsHostReaching`** loses its last non-test caller once the prune
  and `isHostReachingFile` go.

`Project.LanAccess` stays, same JSON tag `lanAccess`, so existing
`projects.json` records keep their value across the upgrade. `lanEverEnabled`
and `lanChangedAt` become unknown fields and are dropped on the next write —
verified by a store round-trip test against a fixture carrying both.

## Failure modes

| Situation | What happens | What the user sees |
|---|---|---|
| Toggle while running | `Stop()`, then `EnsureRunning` force-removes the container, rebuilds the network under the other bridge name, recreates the container | project rebuilds (slower than a restart); home and workspace untouched; apt manifest replays; switch shows the new mode |
| Toggle while stopped | record only; applied at next start | switch shows the new mode, no restart |
| Rebuild fails mid-toggle | record already changed; bridge may be old or the container gone | project shows its start error; the next successful start heals it, because the mismatch check runs on every start |
| Container removed but network rebuild then fails | container gone, record changed | the project is stopped and reports the error; the next start recreates both. No data loss: home and workspace are host binds |
| Network exists with the wrong bridge name | `create()` removes and recreates it | one extra second on start; no user-visible state |
| Root firewall script absent or never run | base rules missing -> SANDBOX projects are not confined | **fail-open, pre-existing** (ADR-036's timer + `host_converge` are the mitigation). Covered by an integration check asserting both base jumps exist; called out in Risks |
| Docker's per-bridge DROP absent (older/other Docker) | cross-project isolation would rest on the sandbox drops alone, which a LAN project does not have | Phase 5 asserts the rule exists AND that the attack fails; the plan fails if either does |
| Two LAN projects | each on its own bridge; Docker drops bridge-to-bridge both ways | no cross-project reach, same as sandbox |
| Setup VM | network name is not a uuid, never passed `Lan` | always sandboxed, structurally |

## Verification

**Unit** (`go test ./... -race`):
- `BridgeNameFor` golden vectors, both modes, plus the setup-VM case.
- Store round-trip: a fixture record carrying `lanEverEnabled` and
  `lanChangedAt` loads, keeps `lanAccess`, and drops the two dead fields.
- `SetLanAccess` no-op returns without a write.
- The network handler's authz matrix: admin-enable ok, owner-disable ok,
  non-admin-enable 403, cross-user 404, malformed body 400, no-op
  `rebuilt:false`.

**Integration** (`.agents/docs/testing/styrman/integration.sh`, real Kata):
- A sandbox project's bridge interface exists and starts with `sdb-`.
- Base jumps present: `INPUT -i sdb-+ -j STYRMAN-IN` and
  `DOCKER-USER -i sdb-+ -j STYRMAN-FWD`.
- Sandbox project: gateway and host unreachable; a public IP reachable.
- LAN project: bridge starts with `lan-`; no styrman rule names it; gateway,
  host and a LAN peer address reachable; still NATs out as the host.
- Toggle sandbox->LAN->sandbox on one project; the bridge name follows and the
  reachability follows with it.
- No `styrman-lan:*`, `styrman-lanhost:*` or `styrman-lanpub:*` rule exists
  anywhere after the new script has run once.

**E2E** (`web/e2e/run.sh`): the switch renders both modes, toggling calls the
endpoint, the disabled/unavailable states are gone, a non-admin cannot enable.

**Live adversarial (R12 requires it, Phase 5, this host, real Kata)** — the
gate for the whole plan:
1. Two projects, A in LAN mode, B in sandbox mode, both running.
2. A reaches `192.168.86.1` (gateway), the host's uplink address, and one other
   LAN host. B reaches none of them.
3. **A cannot reach B's bridge subnet or B's container**, and still cannot with
   `NET_ADMIN` inside A adding a manual route via the server — the same attack
   R12 already names. Recorded with the exact commands and output.
4. B is unchanged from before the release on every check.
5. `iptables -S` before and after, diffed, showing only the base chains.

**E2E criteria:** in a browser, an admin flips one project to LAN, watches it
restart, opens a terminal in it and pings the LAN gateway successfully; flips
it back, watches it restart, and the same ping now times out.

## Phases

### Phase 0 — Requirements and decision record (no code)
- [x] Replace `REQUIREMENTS.md` lines **70-113 inclusive** (the whole numbered item 12, from `12. **R12 - Project network isolation**` through `clause are unconditional.`) with **Appendix A** verbatim. The boundary is the numbered list item, not a sentence inside it — the first draft said "everything from 'Enforced by naming the bridge interface'", which the cold reader correctly called ambiguous. Item 13 (`13. **R13 - Git is the substrate**`) begins at line 115 and must be untouched — verify: `git diff REQUIREMENTS.md` shows one hunk, item 13's first line unchanged, and the new text matches Appendix A word for word; operator reading
- [x] ADR-067 in `DECISIONS.md`: the prefix is the switch; supersedes ADR-065 and ADR-066 and all three amendments in place, keeping them as history; records the two verified facts (Docker's per-bridge DROP; the interface name is server-chosen) and the operator's two decisions — verify: append-only, `git diff --numstat` shows insertions only, AI
- [x] Point `ROADMAP.md` "Now" at this plan — verify: read back, AI
- [x] Commit Phase 0 alone, before any source edit — verify: `git show --stat`, AI

### Phase 1 — The switch (Go, additive; tree stays green)
- [ ] `config.BridgeNameFor(netName string, lan bool)` + the comment above — verify: `go build ./...`, AI
- [ ] Golden vectors in `internal/config/bridgename_test.go` (append; the file exists — writing-files rule) — verify: `go test ./internal/config/ -race -run BridgeName`, AI
- [ ] `RunOpts.Lan`, `createSpec.lan`, one network-create helper, `recreateMissingNetwork(.., lan bool)` — verify: `go build ./... && go vet ./...`, AI
- [ ] `Orchestrator.bridgeModeMismatch` + the third force-recreate condition in `EnsureRunning`, beside the image-upgrade one (`orchestrator.go:206-216`); stopped containers only — verify: `go build ./...`, AI
- [ ] The network rebuild in `create()` (self-disconnect, remove, recreate under the wanted name) — verify: `go build ./...`, AI
- [ ] Three `EnsureRunning` call sites in `internal/httpapi/server.go` pass `Lan: p.LanAccess` — verify: `grep -rn 'EnsureRunning('` shows no call site without `Lan`, AI
- [ ] `go test ./... -race` green, test count compared before/after (BUG-005) — verify: counts recorded in the phase entry, AI

### Phase 2 — Delete the server-side machinery
- [ ] Rewrite `internal/httpapi/network.go` to the small GET/PUT above — verify: `go build ./...`, AI
- [ ] Delete `internal/netpolicy/` (`git rm -r`, then `git status --porcelain` for unnamed ` D` lines — BUG-006) — verify: `git diff --stat` deletions match the 6 files, AI
- [ ] Strip `Server.netpol`/`NetPolicy`/`lanFields`/`kickLAN`/`hostFirewall`/`ingressBridgeMismatch`/`lanFor` and the now-orphaned `ingressBr`/`ingressBrMu`/`ingressBrAt`; delete `Orchestrator.BridgeNameForNetwork`; delete the marker sweep in `cmd/styrman/main.go` — verify: `go build ./... && go vet ./...`, AI
- [ ] **Delete `pruneHostReachingCreds` and its call site (`network.go:245,270`) and `git rm internal/httpapi/lanprune_test.go`.** This is the SECOND withholding mechanism, independent of the broker latch: it deletes the SSH key out of the project home on every LAN-enable. Leaving it would contradict the operator's decision while every named identifier looked deleted — verify: `grep -rn 'pruneHostReachingCreds\|lanprune'` returns nothing; `git status --porcelain` shows no unnamed ` D` (BUG-006), AI
- [ ] Delete `credbroker.LanChecker`, `Broker.lan`, `lanExposed`, `isHostReachingFile`, the withholding branch, and the `lan` parameter of `credbroker.New` (one caller, `cmd/styrman/main.go:182`); delete `credstore.IsHostReaching` and `TestIsHostReachingFile` — verify: `go build ./... && go vet ./...`; `grep -rn IsHostReaching` returns nothing, AI
- [ ] `Project`: delete `LanEverEnabled`, `LanChangedAt`, `RestoreLanAccess`; simplify `SetLanAccess`; update `internal/project/store_lan_test.go` (163 lines) — verify: store round-trip test on a fixture carrying both dead fields, AI
- [ ] Delete `projectView.LanState`/`LanMessage` (`server.go:625-626`) and the two `lanFields` fill sites (`:693`, `:916`); rewrite `internal/httpapi/network_test.go` (360 lines) for the new handler — verify: `go build ./... && go test ./internal/httpapi/ -race`, AI
- [ ] Strip `deleteProject`'s `SetMarker`/`Kick` block (`server.go:1019-1032`) but KEEP `s.netMu` around the record removal, and narrow its comment per "What `netMu` is for" above — verify: `go build ./...`; `grep -rn 'netpol'` returns nothing; `grep -rn netMu` still shows both sites, AI
- [ ] Drop the admin `hostFirewall` field from the settings payload (`explorer.go:455-460`) — verify: `go build ./...`; `grep -rn hostFirewall --include=*.go` returns nothing, AI
- [ ] Remove `STYRMAN_FIREWALL_DIR` and the `/run/styrman-firewall` bind from `deploy/docker-compose.yml` — verify: `docker compose -f deploy/docker-compose.yml config` parses, AI
- [ ] `go test ./... -race` green; test count compared — verify: counts recorded, AI

### Phase 3 — Shrink the root firewall to base rules
- [ ] `styrman-firewall.sh`: delete the family/reconcile machinery, conf `LAN_*` parsing and validation, the spool drain, the status write; keep the base section, the one-time pre-chain migration, the IPv6 loop, `-w` — verify: `bash -n` and `shellcheck`, AI
- [ ] Add a one-shot cleanup: delete every rule tagged `styrman-lan:`, `styrman-lanhost:` or `styrman-lanpub:` by full content match (BUG-013), never by number; refuse to delete a DROP — verify: a `test.sh` case that plants all three tags and asserts they are gone and the base rules intact, AI
- [ ] Delete `styrman-firewall.path`; drop its `Wants`/`Also` from the unit and installer; **drop `./styrman-firewall.path` from `deploy/verify-commit.sh:162`'s `systemd-analyze verify` list and refresh its `:146-154` comment** — the release gate fails on a missing unit file otherwise — verify: `bash deploy/verify-commit.sh`, AI
- [ ] `install.sh`: strip LAN CIDR discovery/validation, `--lan-allow-host`, `--lan-allow-published-ports`, the sshd precondition, the spool dir; keep the BUG-009 non-symlink+owner assertions on every path it still touches — verify: `bash -n`, `shellcheck`, and `test.sh`, AI
- [ ] `test.sh`: strip the family cases, keep base + the new cleanup case — verify: `sudo bash deploy/firewall/test.sh` passes, AI
- [ ] Converge this host and confirm the live chains hold exactly the base rules — verify: `iptables -S` diffed against the pre-change capture, **user + AI**

### Phase 4 — UI and API surface
- [ ] `LocalNetwork.tsx` -> a two-state mode switch, a one-line explanation of each mode, and "changing this restarts the project"; delete the unavailable/pending/error/version copy and the permanent-SSH-key warning — verify: `npm run check && npm run lint`, AI
- [ ] Delete `HostFirewallSection` (`settings.tsx:949`, 62 lines), its call site (`:856-857`) and its `HostFirewall` import — verify: `npm run check`; `grep -rn HostFirewall web/src` returns nothing, AI
- [ ] Delete `web/src/lib/lan.ts` + `lan.test.ts` and reduce `LanBadge.tsx` to a two-state badge (or delete it and let the row show nothing when sandboxed); update both call sites (`Sidebar.tsx:289`, `index.tsx:100`) — verify: `npm run check && npm test`; `grep -rn 'lanState\|lanMessage' web/src` returns nothing, AI
- [ ] **This is the one deletion `tsc` cannot catch**: the frontend LAN types are optional, so a stale badge renders on `undefined` rather than failing the build. Assert it in a test, not by reading — verify: a vitest case that a sandbox project row renders no LAN badge and a LAN project row renders exactly one, AI
- [ ] `web/src/lib/api.ts`: lanView shape; delete `HostFirewall` (`:33`), `host?` (`:72`), `hostFirewall?` (`:256`) — verify: `tsc -b` clean, AI
- [ ] `npm test` green — verify: counts recorded, AI

### Phase 5 — Verification on real hardware
- [ ] Replace the `integration.sh` LAN block with the checks listed under Verification; delete `STYRMAN_IT_LIVE_LAN` and its documented race (TODO.md item, now moot) — verify: `bash .agents/docs/testing/styrman/integration.sh`, AI
- [ ] Replace the two e2e LAN blocks — verify: `bash web/e2e/run.sh`, AI
- [ ] **The live adversarial run**, all five steps, written to `.agents/docs/testing/2026-09-09-lan-mode-live.md` — verify: real Kata, this host, **user + AI**
- [ ] The browser E2E criteria walked by hand — verify: **user**

### Phase 6 — Definition of Done and release
- [ ] `verifier` over the whole change, with test counts before/after — verify: log in `testing/`, AI
- [ ] `reviewer` (diff only) and `security` (the diff touches the firewall, the broker and an authz path) — verify: reports in `reviews/`; findings fixed or rebutted in writing, AI
- [ ] `documentation`: `deploy/remote.md` (sshd becomes advice), `specs/styrman/guide.md`, `AGENTS.md`'s network paragraph, the testing README counts — verify: files listed, AI
- [ ] `changelog` entry per phase + the release entry; `bugs` for anything that went wrong on the way; strike the TODO.md items whose subject no longer exists — verify: paths returned, AI
- [ ] Version: MINOR. It changes behaviour for a user of the released artifact (a new network mode with different semantics) and breaks nothing that worked — the deleted machinery never successfully applied an allowance on any deployment — verify: `versioning.md`'s rule read back against this change, AI
- [ ] `/preflight` — verify: SHIP report, commit, tag, push, deploy, **user + AI**

## Risks

1. **Fail-open direction reversed for the sandbox.** Previously an absent root
   half could only remove LAN access. Now the base rules ARE the sandbox: if
   `styrman-firewall.sh` never runs, every sandboxed project silently has LAN
   reach. This is pre-existing for the base rules (ADR-036's timer exists for
   exactly it) and is not made worse by this change — but the sandbox now has
   no second belt, so Phase 5 asserts both base jumps on the live host and
   `integration.sh` keeps asserting them. Deliberately NOT mitigated with new
   server-side state: deriving policy from root's word is the mechanism this
   plan deletes.
2. **Cross-project isolation now rests on Docker alone for a LAN project.**
   Verified present on Docker 29.7.2 here; asserted and attacked in Phase 5.
   If the attack succeeds the plan fails and LAN mode does not ship.
   A Docker major upgrade should re-run that check — recorded as a TODO.
3. **A LAN project can reach the host's sshd.** By construction and by the
   operator's stated intent. It is advice in `deploy/remote.md`, not a gate;
   the gate is what stopped the previous design from ever being used.
4. **The SSH key is served to a LAN project.** The operator chose this over the
   latch. The transitive path ADR-066 named (LAN project -> a host trusting the
   key -> back here) is reopened and is recorded in ADR-067 as accepted, not
   overlooked.
5. **Existing bridges must not be renamed.** The sandbox golden vector is the
   guard; if it ever changes, every project's network is recreated on upgrade.
6. **A toggle rebuilds the container, it does not restart it.** The rootfs
   layer is discarded; the ADR-028 apt manifest replays installed packages, and
   home + workspace are bind mounts. This is the image-upgrade path's already
   accepted cost, but it is a real difference from "flip a switch" and the UI
   must say so before the user confirms. Caught by the cold reader: the plan's
   first draft claimed a plain stop/start would work, which `orchestrator.go:229`
   shows it would not — it would have silently done nothing at all.
7. **`web/dist/client` is committed and embedded.** Phase 4 must rebuild it or
   the server serves the old switch. `verify-commit.sh`'s freshness gate covers
   this; noted because it has bitten before.


## Appendix A — the approved R12 replacement text

Replaces `REQUIREMENTS.md` lines 70-113 inclusive (the whole of numbered item
12). Approved by the operator on 2026-09-09 as presented in chat, alongside the
two decisions recorded in "Why". Reproduced here so Phase 0 needs no second
round and so a later reader can check what was actually approved.

```markdown
12. **R12 — Project network isolation** (operator directive 2026-08-26):
    project microVMs MUST NOT be able to reach one another over the network.
    Each project gets its own bridge (`sdb-<uuid>`) shared with nothing but
    the styrman server; the server originates connections for the port-preview
    proxy but has `ip_forward=0` so it can never route between projects.
    Enforced in two independent layers. (a) Cross-project isolation is
    Docker's own, unconditional for every project whatever its mode: Docker 29
    writes `-A DOCKER ! -i <br> -o <br> -j DROP` per bridge, dropping any new
    flow entering a bridge from anywhere but that bridge; `ip_forward=0` on
    the server keeps it from routing between them. (b) The sandbox is
    styrman's: a sandboxed project's bridge INTERFACE is named `sdb-<hash>`
    (the Docker NETWORK stays `sdb-<uuid>`; the interface is `sdb-` plus an
    11-hex hash of the network name — the 15-char iface limit), which the
    root-owned self-healing firewall (ADR-036) matches on the shared `sdb-`
    prefix to drop everything from that bridge to the host (`INPUT`) and to
    RFC1918, the CGNAT/benchmark ranges 100.64/10 and 198.18/15, and metadata
    169.254/16 (`DOCKER-USER`). Verified adversarially (a project with
    NET_ADMIN adding a manual route through the server stays blocked). See
    ADR-015, ADR-067.
    Per-project mode (ADR-067, operator approval 2026-09-09; supersedes
    ADR-065/066's design, kept as history in those entries): each project is
    SANDBOX (default) or LAN, toggled by an admin at any time; the toggle
    rebuilds the project's Docker network and container. A LAN project's
    bridge interface is named `lan-<hash>`, which no styrman rule matches, so
    it is exactly a normal Docker container on this host: it NATs out as the
    host and reaches whatever the host reaches — the local network, the
    host's own addresses (sshd included), and host-published Docker ports. It
    gains no reach into other projects, because layer (a) is not styrman's to
    weaken. There is no allowlist, no per-prefix configuration and no
    root-side per-project state: the switch is the interface name, chosen by
    the server at network create. A deployment using LAN mode SHOULD NOT
    accept password authentication on the uplink — advice in
    `docs/deploy/remote.md`, not an enforced precondition. LAN hosts still
    cannot initiate connections into any project (NAT-only outbound). Host
    networking is for the Caddy ingress ONLY, never for projects. The setup VM
    is always sandboxed.
    (Was, ADR-065/066: an admin-toggled allowance of exactly the IPv4 prefixes
    in root-owned `/etc/styrman/firewall.conf`, applied as per-bridge RETURNs
    by the root firewall service from an intent file the server wrote and root
    validated by name, with `LAN_ALLOW_HOST` and `LAN_ALLOW_PUBLISHED_PORTS`
    conf opt-ins, an sshd password-auth prohibition enforced by `install.sh`,
    and the credential broker permanently withholding the operator's SSH key
    from any project that had ever been LAN-enabled. Before ADR-065 the
    private-range egress drop applied unconditionally to every project.)
    The cross-project guarantee in the first sentence, `ip_forward=0` and the
    host-networking clause remain unconditional.
```


## Appendix B — the consumer inventory, and why it is generated not written

Three cold-reader rounds found eleven items. **Nine of them were the same
mistake**: a consumer of a deleted identifier that was not in this plan's
"Surfaces touched" table. Round 1 found `pruneHostReachingCreds` and four
orphans; round 2 found `deleteProject`'s marker cleanup and the whole
`HostFirewallSection`; round 3 found `LanBadge` and the five-state `lib/lan`
model. Each round I added the named items by hand and each following round
found more. Hand-curating this list is the defect, not any individual miss.

So the list below is **generated** — one grep per deleted identifier across the
whole tree — and it found eight surfaces that three review rounds had not:
`network_test.go`, `store_lan_test.go`, `config.go`'s `STYRMAN_FIREWALL_DIR`,
`SidePanelPorts.tsx`, `Sidebar.tsx`, `routes/index.tsx`, `web/e2e/run.sh`, and
`deploy/verify-commit.sh` (which would have failed the release gate on a
`systemd-analyze verify` of the deleted `.path` unit).

**The completion check for Phases 2, 3 and 4 is this sweep re-run, expecting
zero hits**, not a reading of the table above:

```bash
for id in netpolicy 's\.netpol' 'NetPolicy()' lanFields hostFirewall \
          hostFirewallView ingressBridgeMismatch lanFor LanExposed kickLAN \
          pruneHostReachingCreds ingressBr BridgeNameForNetwork LanChecker \
          isHostReachingFile IsHostReaching LanEverEnabled LanChangedAt \
          RestoreLanAccess STYRMAN_FIREWALL_DIR LanState LanMessage \
          MinScriptVersion; do
    grep -rn "$id" --include=*.go . && echo "STILL PRESENT: $id"
done
for id in HostFirewall hostFirewall lanState lanMessage LanBadge 'lib/lan'; do
    grep -rn "$id" web/src web/e2e | grep -v node_modules | grep -v dist \
        && echo "STILL PRESENT: $id"
done
grep -rn 'styrman-lan\|STYRMAN_FIREWALL_DIR\|LAN_CIDRS\|LAN_ALLOW\|lan_allow' \
    --include=*.sh --include=*.yml --include=*.service --include=*.path \
    --include=*.timer .
```

`lanAccess` is deliberately NOT in the sweep: it is the one identifier that
survives, in `Project.LanAccess`, the API body, and `web/e2e/run.mjs`.

### Generated inventory, 2026-09-09

**Go** — `cmd/styrman/main.go`, `internal/config/config.go`,
`internal/credbroker/{broker.go,broker_test.go}`, `internal/credstore/credstore.go`,
`internal/httpapi/{network.go,network_test.go,server.go,explorer.go,lanprune_test.go}`,
`internal/netpolicy/*` (whole package), `internal/orchestrator/orchestrator.go`,
`internal/project/{store.go,store_lan_test.go}`.

**Web** — `web/src/lib/{api.ts,lan.ts,lan.test.ts}`,
`web/src/components/{LocalNetwork.tsx,LanBadge.tsx,Sidebar.tsx,SidePanelPorts.tsx}`,
`web/src/routes/{settings.tsx,index.tsx}`, `web/e2e/{run.sh,run.mjs}`.

**Shell / units / compose** — `deploy/firewall/{styrman-firewall.sh,install.sh,test.sh}`,
`deploy/firewall/styrman-firewall.path` (deleted), `deploy/docker-compose.yml`,
`deploy/verify-commit.sh`, `.agents/docs/testing/styrman/integration.sh`.
