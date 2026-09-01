# Incident — reboot mid-session dropped the site for ~12 minutes (stale `:latest`, boot-race, admin listener hijack)

Date: 2026-09-01. Occurred during the same-day redfix of the two 🔴s from
the whole-project audit (`.agents/docs/changes/2026-09-01-redfix-hardening-and-versioning.md`).
This is an operational incident record, not a code change on its own — the
fixes it motivated are shipped in that sibling changelog entry and in
ADR-053/ADR-054.

## What happened

- The operator SSH'd into the host at **19:27:32 CEST** and ran `sudo
  reboot` at **19:28:03**. This was a deliberate, correct action —
  `unattended-upgrades` had staged a kernel bump (6.12.101 → 6.12.107,
  the kernel the host now runs) and rebooting to pick it up is normal
  maintenance. No fault in the decision to reboot. Two consequences
  followed anyway: it killed the in-flight Claude session mid-remediation
  (the redfix working tree was uncommitted, no `/handoff` had been taken),
  and it exposed a boot-race defect in the Caddy ingress that nothing had
  triggered before.
- **The site was down from ~19:28:42 to 19:40:56** (~12 minutes).
- **Mechanism, reconstructed from `journalctl`, `docker logs`, and `ss`:**
  Docker restarted `styrman-caddy` on boot using the image still tagged
  `:latest` — the **old** image built 2026-08-30. The redfix's Caddy 2.11.4
  build (with the pinned plugins) had already been built at 16:49 that
  same day, but it sat under a *different* tag, unbooted; nothing had
  re-pointed `:latest` at it and nothing forced a restart to pick it up —
  exactly the mutable-tag hazard ADR-053 exists to close. On boot, before
  the host network was fully up, the watcher's first `/load` attempt got a
  400: `caddy-security` could not fetch `accounts.google.com`'s OIDC
  metadata (`network is unreachable`). The **old** watcher binary (no
  retry logic — that only exists in the uncommitted redfix tree) logged
  "caddy reloaded successfully" anyway despite the 400, and latched that
  failed config as applied. Every subsequent scan then read "caddyfile
  unchanged, skipping reload" and did nothing — no retry, ever, for the
  rest of the outage.
- The failed `/load` didn't just fail to serve traffic — it **replaced the
  admin listener**. Caddy swaps in the new config's admin server before
  the new config's apps are provisioned, so a rejected load still moves
  the admin endpoint if the new config named a different one. `localhost:2019`
  died; an admin endpoint came up on `0.0.0.0:26293` — all interfaces,
  "host checking disabled" per Caddy's own log line. Observed live via
  `ss`. At that point **no HTTP servers were running at all**: not the
  site, not a bound admin on loopback, nothing.

## Resolution

At 19:40 the already-built (16:49) new image was deployed manually
(`docker compose up -d caddy`). Verified immediately after: the site
302-redirects through the OAuth portal to `accounts.google.com` as
expected, the admin API is bound to `127.0.0.1:2019` only, and the rogue
`0.0.0.0:26293` listener is gone. Separately, on Caddy 2.11.4, it was
confirmed empirically that a **rejected** `/load` leaves both the admin
server and the running config intact (bad load → 400 from `/load`, admin
still answers 200, site still serves 302) — the 19:28 admin-hijack was
specific to the *old*, unpatched watcher/config shape, not a property of
Caddy itself at the version now shipping.

## Prevention (shipped in the same commit as this incident record)

- Watcher retry/fatal self-heal: a rejected `/load` is never latched as
  applied; it retries on a 15s timer indefinitely, and ≥4 consecutive
  *transport*-level failures (admin unreachable, not just a rejected
  config) make the watcher exit 1 so the container's restart policy
  recovers with a fresh minimal admin config (ADR-054).
- The generated Caddyfile now pins an explicit `admin <loopback-addr>`
  block on every load, so a loaded config — rejected or not — can never
  move the admin listener off loopback (closes the `0.0.0.0:26293` shape
  structurally, not just by fixing this one boot race).
- Immutable versioned image tags end-to-end (ADR-053): no more `:latest`
  for any of the three images, so "what's running" is legible off
  `docker ps` and a reboot can never silently resurrect a stale build.

See `.agents/docs/changes/2026-09-01-redfix-hardening-and-versioning.md`
for the full remediation (code, review/security/verification gates), and
`.agents/docs/architecture/DECISIONS.md` ADR-053/ADR-054 for the decisions
this incident forced.

## Verification

n/a as a standalone item — this entry records an incident and its live
resolution, not a tested code change. The mechanism (stale `:latest`,
latched-despite-400 reload, admin listener hijack, and the rejected-load
intact-admin behavior on 2.11.4) was confirmed directly against
`journalctl`, `docker logs`, and `ss` output during the incident, and
against a real Caddy 2.11.4 instance afterward — not inferred. The
preventive code lands with the redfix changelog entry's own
review/security/verifier gates.
