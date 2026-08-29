# SECURITY: project network isolation was inert; + Ctrl+C fix (v1.8.3)

Date: 2026-08-29. Three operator-reported issues.

## 1. Ctrl+C didn't interrupt (e.g. `ping`)

The terminal's Ctrl+C handler copied whenever ANY selection existed, so a
lingering selection (copy-on-select leaves the highlight) swallowed the
interrupt. Fixed: plain **Ctrl+C always passes through as ^C/SIGINT**;
copy-on-select already puts selections on the clipboard, and Cmd+C (mac) still
copies explicitly. `web/src/lib/terminal.ts`.

## 2. Same IP shown for different projects

Not a backend bug — `projectStats` returns the correct, distinct per-project IP
(each project is its own /16 bridge). The status bar showing a stale IP after a
project switch is the same class fixed in v1.6.1 (`usePolled` reset key); a hard
refresh resolves it. (The network recreation in #3 also reassigned the IPs.)

## 3. SECURITY: project VMs could reach the host and the LAN

**Root cause:** the ADR-029 isolation rule matched the bridge INTERFACE
`-i sdb-+`, but Docker names bridge interfaces `br-<hash>` (the network is named
`sdb-<uuid>`, the interface is not). So the rule matched nothing and **isolation
was never active** — a project VM could reach the host (`192.168.86.58:22`), the
LAN, cloud metadata, and everything, confirmed live.

**Fix (two parts):**
- Name the bridge interface `sdb-<hash>` so the firewall matches:
  `config.BridgeNameFor` (sha1-derived, ≤15 chars — interface names are capped);
  both `NetworkCreate` calls now set `com.docker.network.bridge.name`
  (`internal/config/config.go`, `internal/orchestrator/orchestrator.go`).
- Block egress properly (`deploy/host-setup.sh` → `styrman-firewall.sh`): the
  existing `INPUT -i sdb-+ DROP` (host), PLUS `DOCKER-USER -i sdb-+ -d <private>
  DROP` for 10/8, 172.16/12, 192.168/16, 169.254/16 (LAN, other projects, cloud
  metadata) — routed traffic Docker's FORWARD never guarded. Public internet,
  return traffic, the port-preview proxy, and the same-bridge broker are all
  unaffected.

**Applied live:** deployed the server, recreated both project networks (they now
have `sdb-*` bridges), installed + ran the updated `styrman-firewall.sh`.
**Verified from a project VM:** host / LAN / metadata → blocked; internet →
reachable; broker + ssh key still present.

Existing projects created before this need their network recreated (done for the
two live ones); new projects get the `sdb-*` bridge automatically.

## Release

Built + deployed live as styrman-server 1.8.3; firewall applied on the host.
