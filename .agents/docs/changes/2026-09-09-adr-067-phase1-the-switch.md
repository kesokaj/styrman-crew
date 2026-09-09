---
owner: main
date: 2026-09-09
---
# ADR-067 Phase 1 — the switch itself

The whole feature, in ~190 lines and no new privilege. `BridgeNameFor` gained a
mode argument, and the four-character bridge prefix now decides whether the
root firewall's `-i sdb-+` jumps match a project at all.

## What changed

- **`internal/config/config.go`** — `BridgeNameFor(netName string, lan bool)`.
  The sha1 suffix still hashes the NETWORK name only, never the mode, so a
  project keeps one identity in `ip link` across a toggle and **no existing
  project's bridge is renamed by this release**. Only the prefix varies:
  `sdb-` (matched by the drops) or `lan-` (matched by nothing).
- **`internal/orchestrator/orchestrator.go`** — `RunOpts.Lan`,
  `createSpec.lan`, `createBridgeNetwork` (now the only `NetworkCreate` call
  site in the package), `bridgeModeMismatch`, a third force-recreate condition
  in `EnsureRunning`, the network rebuild in `create()`, and
  `recreateMissingNetwork(.., lan bool)`.
- **`internal/httpapi/server.go`** — all three `EnsureRunning` call sites pass
  `Lan: p.LanAccess`.

## Two design points worth keeping

**The mechanism had to move.** The plan's first draft put the bridge-name heal
in `create()`, and the cold-reader check caught that `create()` runs only when
the *container* does not exist (`orchestrator.go:247`). A toggle on an existing
project takes the reuse branch, which calls `ContainerStart` and nothing else —
so that design would have restarted the same container on the same bridge and
**silently done nothing**. The fix reuses a pattern already in the function
twice (the pre-ADR-027 workspace bind and the image upgrade): remove the
stopped container and set `err = errdefs.NotFound`, so the create branch
rebuilds it. Consequence, stated in the UI copy in Phase 4: a toggle *rebuilds*
the project container. Home and workspace are bind mounts and survive; the
ADR-028 apt manifest replays installed packages; the rootfs layer is discarded.

**`bridgeModeMismatch` fails toward not-rebuilding, with one exception.** A
true return force-removes a container, so a missing network is not a mismatch
(`recreateMissingNetwork` owns that case) and an inspect error is not one
either — a Docker hiccup can never destroy a project. The exception is a
network with **no** `com.docker.network.bridge.name`: no styrman code path
creates one, so it means manual intervention, and an unnamed bridge matches no
firewall rule — the project is silently unconfined until it is rebuilt. That
case rebuilds, and logs a warning saying why.

## Verification

- `go build ./...`, `go vet ./...`, `gofmt -l` clean.
- `go test ./... -race`: 16 packages ok, no failures.
- **Test count 213 → 216** (BUG-005 guard: compared before and after, not just
  the exit status). The three are new; none was lost.
- `git diff --numstat` shows no binary file (BUG-007 guard).
- The two new tests are the security property in test form:
  `TestLanBridgeEscapesTheFirewallWildcard` asserts both directions — a LAN
  bridge must NOT carry `sdb-`, and a sandbox bridge must ALWAYS carry it. If
  the sandbox direction ever fails, projects that should be confined are
  silently on the LAN. `TestBridgeNameForModeIsThePrefix` pins that the 11-hex
  suffix is identical across modes, which is what guarantees no existing
  project's bridge is renamed.

Not yet done: the deletions (Phase 2), the firewall shrink (Phase 3), the UI
(Phase 4). `Project.LanAccess` is still driven by the old handler, so the
switch is wired but the old machinery still runs alongside it.
