---
owner: changelog
date: 2026-09-08
---

# Per-project LAN access — server half (Phase 2)

Date: 2026-09-08. The server's half of per-project local-network access
(ADR-065): the intent record, the name-only marker files root reads, the
spool kick, the status-file derivation, the API, and the boot/periodic
reconciliation. Continues the Phase 1 host half
(`2026-09-08-lan-access-phase1-host-half.md`).

## Summary

**Verification at completion:** `go build`, `go vet`, gofmt, `go test -race
./...` all packages ok (26 new Go tests: `internal/netpolicy` 9 incl. a
concurrency test, `internal/httpapi/network_test.go` 7, `internal/project`
4, `internal/config` 1, plus the Phase 1 netns wrapper);
`integration.sh styrman-runtime:1.22.0`: 78 passed, 0 failed, 4 skipped
(3 firewall-hardened + the opt-in live block), run twice — before and after
the review fixes. The R12 negative check (an in-guest ping to the LAN
gateway must be dropped) PASSED live on this host; it is the first scripted
proof of that requirement.

Two review gates closed over the combined Phase 2+3 diff (reviewer: APPROVE
with 2 low, both fixed; security: 3 medium + 5 low — seven fixed, the eighth
(a NUL byte in the generated `web/dist/client/_shell.html`, pre-existing
build output outside the reviewed range) noted, not changed). Recorded as
BUG-014..BUG-019.

**Measurement:** none new this phase — the Phase 1 numbers stand (path
trigger 30–62 ms, a toggle lands in about 0.2 s); the status cache is 2 s by
construction and the no-op-PUT throttle is 10 s per project, both pinned by
tests rather than measured.

## What shipped, by file

- `internal/config/config.go` — `NetPolicyDir()` (`<StateDir>/netpolicy`,
  server-only, never a bind source) and `FirewallRunDir` from
  `STYRMAN_FIREWALL_DIR` (default `/run/styrman-firewall`; empty disables
  the feature — truly: every marker is removed at boot).
  `bridgename_test.go` pins the golden vector shared with
  `deploy/firewall/test.sh` (`sdb-3e6a772c-… → sdb-e660d58459e`).
- `internal/project/store.go` — `LanAccess` (`omitempty`) and
  `LanChangedAt` (`omitzero`); `SetLanAccess` stamps the clock only on a
  real change; `RestoreLanAccess` reinstates an exact (value, timestamp) pair
  for the handler's rollback; load dates an undated `lanAccess:true`.
  `store_lan_test.go`: round trip, absent-when-off, rollback on a failed
  save, load-time dating, the exact restore.
- `internal/netpolicy/dir.go` — `Dir` with ONE mutex shared by `SetMarker`
  and the whole `Sync` cycle (the `want()` snapshot is taken inside the
  lock, closing the cold-reader's Sync-vs-PUT race); markers are empty
  `O_EXCL` files named by uuid in a 0750 dir that must be a real directory;
  `Sync` adds missing markers, removes any uuid file with no enabled record
  (fail-closed, one Warn per removal), leaves foreign entries alone;
  `Kick`/`KickThrottled` drop tag-validated `O_EXCL` trigger files into the
  root spool (one lock across the throttle check and the write); `Status`
  reads the root-written file with a 2 s cache and a 64 KiB cap.
  `status.go` — `ParseStatus` for root's key=value grammar (schema 1,
  `version`/`applied_at` required, `lan=`/`pending=`/`error=`/`conf_error=`/
  `warn=`), `Availability` (absent / stale — including future-dated — /
  outdated / conf error / no prefix), `Derive` (a record ON with no readable
  status is an ERROR "cannot confirm", never "unavailable"; a stale status
  that listed the uuid is an error "cannot confirm closed"; per-project
  errors win; agreement is on/off; no-bridge is pending; a mismatch is
  pending for 90 s then an error). `dir_test.go`/`status_test.go` cover all
  of it, including the lock-ordering test the plan required.
- `internal/httpapi/network.go` (new) — `GET`/`PUT
  /api/projects/{project}/network`: 400 without a boolean `lanAccess`, 404
  before any authorization signal (another user's project looks absent),
  403 for a non-admin enable, 503 for an enable while root's support is
  absent/stale/outdated/unconfigured (reason in the body), 500 with the
  record rolled back if the marker cannot be written after the save, 200
  with the derived view (`pending` right after a change, never `on`). The
  sequence store → marker → kick runs under `Server.netMu`; a real change
  kicks at once, a no-op PUT kicks throttled (10 s per project); an audit
  line per change; the `host` view (script version, applied time, prefix,
  device, projects on, warnings) for admins only; a non-admin owner gets
  generic messages because root's texts name the LAN prefix. `kickLAN`
  nudges root after a boot only when the record is on and root has not
  applied it. `network_test.go`: validation, cross-user 404 with no marker,
  the 503/403/200 gates, list carries `lanState`, applied → on, error line →
  error, stale + applied after a disable → "cannot confirm closed", the
  admin-only host view, tenant-safe messages, the throttled no-op PUT, 20
  concurrent PUTs ending consistent, delete removes the marker.
- `internal/httpapi/server.go` — `netpol`/`netMu` fields, `New` wires
  `netpolicy.New`, `NetPolicy()` accessor, the two routes, `projectView`
  gains `lanState`/`lanMessage` (one cached status read per list; omitted
  while off with nothing to say), kicks after `EnsureRunning` in
  `ensureFor`/`createProject`/`startProject`, marker removal in
  `deleteProject` immediately before `projects.Remove` (after the wipe
  block's early return) under `netMu`. `explorer.go` — `GET /api/settings`
  gains `hostFirewall` for admins. `isolation_test.go` — the route list
  gains `/network`.
- `cmd/styrman/main.go` — creates `netpolicy/lan` (0750); after
  `httpapi.New` and before the listener: `Sync(wantLAN)` over every user's
  records on disk + `Kick("boot")`; the Adopt goroutine (moved after `New`,
  otherwise unchanged) kicks again once bridges are up; a dedicated
  2-minute ticker re-runs `Sync`; with `STYRMAN_FIREWALL_DIR` empty the
  want-set is empty, so every marker is removed and root closes every
  allowance.
- `deploy/docker-compose.yml` — `STYRMAN_FIREWALL_DIR` env and the
  `/run/styrman-firewall` bind with `rslave` propagation (a spool tmpfs
  mounted after the container started still reaches it).
- `deploy/firewall/install.sh` — marker dirs created 0750 (was 0755).
- `.agents/docs/specs/styrman/api.md` — the project shape, the two routes,
  the settings row, the `STYRMAN_FIREWALL_DIR` env row.
- `.agents/docs/testing/styrman/integration.sh` — the server gets
  `STYRMAN_FIREWALL_DIR=$STATE/fw`; a "local network" block: the R12
  negative check (gated on `sudo -n iptables -S STYRMAN-FWD` and the host
  pinging its gateway), 503 without a status file and no marker, disable
  200, 400 without the key, a crafted fresh status → pending + marker +
  spool + `lanState` in the list, a status listing the uuid → on, disable
  removes the marker; the opt-in `STYRMAN_IT_LIVE_LAN=1` block against the
  production reconciler (default SKIP, marker removed by name in the
  cleanup trap); before the delete the marker is re-created and checked
  gone with the record afterwards. Tier 2 → 82 checks / 4 auto-SKIP.

## Requirements

R12 as amended 2026-09-08 (ADR-065). Enable is admin-only on the admin's
own project; there is no admin path into another user's project; disable is
never refused; the server never touches iptables.

## Agents dispatched

- **reviewer** (`.agents/docs/reviews/2026-09-08-review-lan-phase2-3.md`):
  APPROVE, 2 low, both fixed — the rollback after a failed marker write
  stamped `LanChangedAt` with the request's `now` (now `RestoreLanAccess`);
  `KickThrottled` checked the last-kick time outside the lock it then
  re-took (now one lock). The agent hit its turn limit before the frontend
  checks and wrote the report from what it had verified; the author ran
  those checks (80/80).
- **security** (`.agents/docs/reviews/2026-09-08-security-lan-phase2-3.md`):
  3 medium — fail-open reporting when the status file is unreadable (fixed:
  "cannot confirm" + markers removed when disabled), unthrottled kicks from
  a no-op PUT (fixed: throttled), `rprivate` bind (fixed: `rslave`); 5 low —
  tenant messages leaking the prefix (fixed), marker dir 0755 (fixed: 0750),
  future-dated `applied_at` never stale (fixed), delete-vs-PUT race (fixed:
  under `netMu`), the pre-existing NUL in generated `web/dist` output (noted,
  not changed). Recorded as BUG-014..BUG-019 by the `bugs` agent.
- no verifier agent this phase — the author ran the commands listed under
  Verification; the Definition of Done verifier log comes at Phase 5.
- `changelog` wrote a first draft of this entry that misstated the reviewer's
  findings and cited files that do not exist; the author replaced it after
  checking each line against `git diff`.
