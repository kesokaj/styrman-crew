---
owner: changelog
date: 2026-09-08
---

# Per-project LAN access — UI (Phase 3)

Date: 2026-09-08. The user-facing half of per-project local-network access
(ADR-065): a switch with an honest state line in the Ports tab, a chip on the
sidebar row and the dashboard card, and an admin-only Host firewall section
in Settings. Reviewed together with the Phase 2 server half (one pass over
the combined diff; findings and fixes are listed in the Phase 2 entry).

## Summary

**Verification at completion:** `npm run check` (tsc), `npm run lint`
(oxlint) and `npm test` (vitest 80/80 across 8 files, 11 of them new in
`lan.test.ts`) pass; `bash web/e2e/run.sh`: 84 passed, 0 failed, 2 skipped
(the two host-firewall proxy skips), client-error gate clean — Tier 4 is now
86 checks + the gate. No verifier agent ran for this phase; the author ran
the commands.

The first e2e run failed one of the three new checks and the failure was a
real gap, not a test artefact: with no status file and a record that is off
the server derives `off` (correct — there is nothing to say about a rule
that does not exist), so the block rendered the default line with an
ENABLED switch even though the host half was absent and an enable would
have bounced off a 503. The admin's reply carries `host.available`, so the
block now disables the switch up front and shows the reason. Recorded as
BUG-019.

## What shipped, by file

- `web/src/lib/api.ts` — `Project` gains `lanAccess`, `lanChangedAt`,
  `lanState` (`off|pending|on|error|unavailable`), `lanMessage`, and the
  previously missing `sharedPorts`; new `HostFirewall` and `ProjectNetwork`
  types; `Settings.hostFirewall?`; verbs `projectNetwork(id)` and
  `setProjectLan(id, lanAccess)` (ids URL-encoded).
- `web/src/components/ui.tsx` — a shared `Switch` (`<button role="switch"
  aria-checked aria-label title>`, warn-coloured track when on, disabled
  opacity, Space/Enter through the native button). The first `role=switch`
  in the app; one family per control.
- `web/src/components/LocalNetwork.tsx` (new) — rendered at the top of the
  Ports tab by `SidePanelPorts.tsx`. Polls `GET /api/projects/{id}/network`
  every 5 s (so it works for a STOPPED project), keeps the PUT's reply as an
  override until the poll reports the same `lanChangedAt`, and renders one
  of: loading, off, on (prefix + "Applied HH:MM:SS"), pending (applying /
  closing / "applies when the project next starts"), error (root's message
  in danger colour + a `retry` that re-PUTs the same value), unavailable
  (reason). Admin is inferred from the presence of `host` in the reply.
  Enable opens a danger `ConfirmDialog` naming the blast radius (the
  project's brokered credentials, every LAN device by IP, what stays
  blocked, that turning it off cuts open connections); disable needs no
  dialog. The switch is disabled while pending, for a non-admin when off,
  and when the host cannot accept an enable. Mutations go through `tryUI`
  with the two success toasts.
- `web/src/lib/lan.ts` (new) + `lan.test.ts` (11 tests) — the pure
  derivations: `lanChip` (nothing while off/unavailable; `LAN` in warn
  colours while on/pending, pulsing while pending; danger with root's
  message on error; a generic place name when the prefix is unknown) and
  `lanLine` (the per-state sentence), plus `formatClock`.
- `web/src/components/LanBadge.tsx` (new) — the chip markup over `lanChip`;
  used in `Sidebar.tsx` (after the failure chips, not while deleting) and on
  the dashboard card in `routes/index.tsx`. Not clickable.
- `web/src/routes/settings.tsx` — `HostFirewallSection` after Runtime,
  admins only (the payload is only sent to admins): local-network access
  available or the reason in red, script version (or "not installed" /
  "outdated (have N, need M)" with the install command), last applied,
  prefix + device with its source file, projects with LAN on, warnings.
- `web/e2e/run.sh` — the self-hosted server gets `STYRMAN_FIREWALL_DIR` at
  a scratch dir, so a host-process test server can never read the
  production reconciler's status or kick its spool.
- `web/e2e/run.mjs` — three checks after the ports-tab check: the block
  reports the host half as unavailable with the switch disabled and reading
  off; an enable answers 503 and nothing persists in the list; Settings
  shows the Host firewall section as not installed.
- `.agents/docs/specs/styrman/guide.md` — §2 isolation bullet extended, a
  §4 "Local network" bullet, a §5 troubleshooting row.
- `.agents/docs/testing/styrman/README.md` — Tier 1 and Tier 4 counts and
  descriptions (Tier 2 is in the Phase 2 entry).
- `web/dist/client/**` — rebuilt by the e2e run (the server embeds it).

Not built, deliberately: the applied-state UI (pending → on, the chip, the
dialog) has no automated check because the test harnesses have no root
reconciler; it is verified by hand on the converged production host and
logged in `testing/` (plan Phase 3, still open until the operator installs
the host half).

## Requirements

R12 as amended 2026-09-08 (ADR-065). Non-admins cannot enable (the server
answers 403 and the switch is disabled); the LAN prefix reaches only admins.

## Agents dispatched

- `reviewer` and `security`: one pass over the combined Phase 2+3 diff —
  see the Phase 2 entry for counts, fixes and the one noted-not-changed low.
- `changelog` wrote a first draft of this entry that described behaviour
  and files that do not exist (a retry with exponential backoff, a clickable
  badge, an admin sync endpoint, a route file, wrong colours); the author
  replaced it with this one after checking each line against `git diff`.
