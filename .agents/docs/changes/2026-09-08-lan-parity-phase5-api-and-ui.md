---
owner: main
date: 2026-09-08
---

# LAN parity Phase 5: the switch tells the truth about what it grants   (2026-09-08)

Plan: `.agents/docs/plans/2026-09-08-lan-full-parity.md` (Phase 5, 5 of 5
boxes). ADR-066, API and UI.

## A prerequisite the plan implied but never listed

`ParseStatus` ignored unknown keys — the forward-compatibility this plan
relied on — which also meant the server could not see the ADR-066 keys at
all. `Status` now carries `AllowHost`, `AllowPublishedPorts`,
`IngressBridge`, `HostAddrs` and `PubSubnets`, and a status file without them
parses as "opted into nothing", which is the normal state of a host
mid-upgrade.

## The switch no longer over-promises

The old confirmation said "This server, other projects and cloud metadata
stay blocked". Under ADR-066 that can be false. The dialog is now written
from what root reports:

- **Host conf opted in**: names the server's own address as inside the
  allowance, in danger colour, and says an agent can attempt to log in to the
  machine it runs on. Adds published containers when that key is on too.
- **Not opted in**: says the server and its published ports stay blocked
  *because the operator has not opted in* — not because the feature cannot.
- **Always**: the project will no longer be given the SSH key, pushing over
  SSH stops working, HTTPS with the token still does, a materialized copy is
  removed, and a project that has run before must treat that key as exposed.

Two notes under the switch: one when it is on but the host conf does not
include the host (naming the exact conf key), and one stating the SSH key is
withheld.

`credentialImpact` is sent to owners as well as admins — it describes their
own project. The new host fields stay inside the admin-only `host` view,
which is where the operator's prefixes and bridge names already live
(BUG-017).

## The other half of the ingress cross-check

Root reports the bridge it excluded from the published-port allowlist but
cannot tell whether it is the right one — a root firewall script must not
depend on the Docker daemon. The server can: `BridgeNameForNetwork` resolves
the configured ingress network (`STYRMAN_INGRESS_NETWORK`, default
`styrman-ingress`) to `br-` + the first 12 hex of its id, honouring a
`com.docker.network.bridge.name` override, and a mismatch becomes a warning
on the host-firewall view. An empty answer is "no opinion", never a
mismatch — and it is cached for a minute, because `lanView` renders per
project in a listing.

## BUG-020, found by the new e2e

A browser check disabled LAN access through the API and waited for the
withheld-key note to clear. It never did. `LocalNetwork.tsx` chose between
its optimistic PUT reply and the poll with an **equality** test on
`lanChangedAt`, so any out-of-band change moved the poll's timestamp past the
override's and pinned the component to a stale reply for as long as it stayed
mounted. Pre-existing since v1.23.0; it self-heals in ordinary use only
because the next change usually goes through the same component.

Extracted to a pure `preferOverride` in `lan.ts` and changed to an ordering
comparison, with the old equality as the fallback when a timestamp is missing
or unparseable. Five vitest cases, including the out-of-band one.
`.agents/docs/bugs/BUG-020-lan-switch-stale-override.md`.

## One e2e check was wrong, not the code

A first draft tried to click the switch off while the record was pending. The
component correctly disables it then — "nothing flips while pending" is the
point of deriving state from root's word rather than the request's. The check
now asserts exactly that, and disables through the API, which is legitimate
because tightening is never refused.

## Verification

- Browser e2e **89 / 0 / 2 skipped** (84 passing before this phase), with
  five new ADR-066 checks. They plant a fake root status file in the e2e's
  own scratch firewall dir — nothing root-owned, nothing production.
- `npm run check` and `lint` clean; vitest **85** (was 80).
- `go vet ./...`, `go test ./...`, `deploy/firewall/test.sh` 120/0.
- `web/dist/client` rebuilt and committed (it is embedded via `go:embed`, so
  a stale dist would ship the old UI). The 14 deletions are hashed asset
  names rotating against 14 additions — checked, per BUG-006.
