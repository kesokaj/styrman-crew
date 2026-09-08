---
owner: changelog
date: 2026-09-08
---

# Per-project LAN access — host-half implementation (Phase 1)

Date: 2026-09-08. The host-facing half of per-project local-network access (ADR-065, R12 amendment) shipped with tests, security/code review gates, and measurement. The server half and UI remain Phase 2. The host's pre-existing drift (four vs. six DROP ranges on the live machine) is neither converged nor installed yet; delivery infrastructure to fix it in-place is included.

## Summary

**Verification at completion:** deploy/firewall/test.sh: 83 checks passed. go test -race ./...: all packages ok (netpolicy 5.5 s). Measured: path-unit trigger latency 30–62 ms (five runs), script execution 80–120 ms in the netns, toggle application ~0.2 s end-to-end.

Two review gates closed (reviewer: 1 high, 1 medium, 2 low — all fixed; security: 1 high, 3 medium, 7 low — the high, the three mediums and four lows fixed, three lows rebutted in writing as documented risk or recorded follow-up). No verifier agent ran for this phase: the author ran the commands listed above; the commit itself is gated by `deploy/verify-commit.sh` and the Definition of Done row is the Phase 5 verifier log. One defect surfaced in fixing (write_status race in a command substitution) and remedied.

This is the host-reconciliation half: root-owned script, units, install harness, integration with the auto-updater, and Go-side intent→marker mirroring that the server will consume. The feature is fail-closed by construction (block by default, per-project exceptions only), and the script is stateless (reconcile from markers + config on every run/tick).

## Host infrastructure — root-level reconciliation

**New files:**
- `deploy/firewall/styrman-firewall.sh` (STYRMAN_FW_VERSION=3): base section (today's six DROP rules verbatim), then LAN reconciliation. Reads markers by name only (`<StateDir>/netpolicy/lan/<uuid>`), reads prefix config from `/etc/styrman/firewall.conf` (validated, regex-parsed, never sourced), calculates desired rules, reconciles current vs. desired via iptables (delete-by-spec, no renumbering), writes atomic status. Drains the spool on entry and again on a failed exit (a healthy exit leaves late arrivals for the path unit) to prevent re-triggering loops. All paths from environment (ADR-052).
- `deploy/firewall/install.sh`: idempotent installer. Discovers default-route device and LAN prefix once (or accepts `--lan-dev` / `--lan-cidr` flags), writes `/etc/styrman/firewall.conf` ONLY IF ABSENT, installs the script and three systemd units, `systemctl daemon-reload`, enables and starts the timer and path unit. Validates LAN prefixes (dotted-quad `/16..30`, host bits zero, no overlap with reserved ranges or Docker bridge routes). `--dry-run` for testing; `--prefix` for root-test scratch paths (ADR-052).
- `deploy/firewall/styrman-firewall.service`: oneshot, After=docker (not Wants=, because a manual stop must stay stopped), `StartLimitIntervalSec=60 StartLimitBurst=20`, `TimeoutStartSec=30s`. Environment pins all paths (no in-script defaults).
- `deploy/firewall/styrman-firewall.path`: triggers on `DirectoryNotEmpty` of the spool, no After=docker (prevents systemd job-deletion cycles), `TriggerLimitIntervalSec=60s TriggerLimitBurst=20`.
- `deploy/firewall/styrman-firewall.timer`: unchanged, `OnBootSec=15s OnUnitActiveSec=60s`.
- `deploy/firewall/test.sh`: 83 checks run inside an unprivileged netns with remounted sysfs. Proves base rules idempotent, one marker produces the exact head-of-chain rule, physdev RETURN next, six DROPs after; second run zero writes; deletion removes the rule; malformed comments are purged by number; non-uuid/symlink/dir names ignored with warning. Runs under `go test` via `internal/netpolicy/script_test.go`.

**Modified files:**
- `deploy/host-setup.sh` — §3b heredoc removed (left a pointer comment), §5c calls the installer after state dir exists, §5 gains symlink refusal on state roots and uses `chown -h`.
- `deploy/update/styrman-update` — new `host_converge()` function runs on every tick (after tag verification, before "already up to date" exit, and again after deploy/rollback). Gate: REQUIRE_SIGNED=1, environment vars set by the unit, HEAD == recorded deployed SHA. Installs `deploy/firewall/styrman-firewall.sh` (0755 → `$STYRMAN_FW_INSTALL_BIN`) and the three units (0644 → unit dir) from the verified git object; never touches `firewall.conf` or tmpfiles. Non-fatal if a file doesn't differ or can't install. Under `--check` it only reports drift, changing nothing (regression: an earlier draft applied changes under `--check`).
- `deploy/update/styrman-update.service` — adds `STYRMAN_FW_INSTALL_BIN` environment to pin the install target.
- `deploy/verify-commit.sh` — bash -n over `deploy/firewall/install.sh`, shellcheck -S warning, `systemd-analyze verify` over the three units.

**Host-side file ownership and security:**
- Marker directory (`<StateDir>/netpolicy/lan/`) is operator uid, both parent and child chowned by install.sh with refusal of symlinks. Server creates/deletes marker files by name.
- `/run/styrman-firewall/` root-owned 0755 with a dedicated tmpfs mount for the spool (1 MiB, 1024 inodes, created by generated tmpfiles.d entry). Server drops spool trigger files; root consumes with find-delete.
- `/run/styrman-firewall/status` root 0640 (readable by the operator uid via DAC, which the container can do since /run/styrman-firewall is bind-mounted rw).
- `/etc/styrman/firewall.conf` root 0644, written by install.sh ONLY IF ABSENT (discovered or flagged, never re-derived).

## Server-side intent capture (netpolicy Go package)

**New package internal/netpolicy:**
- `MinScriptVersion=3` asserted by `TestMinScriptVersionMatchesScript` (greps the deploy/firewall/styrman-firewall.sh header).
- `SetMarker(uuid, on/off)` — idempotent create/delete with uuid regex validation, O_EXCL.
- `Sync(want func() (map[string]bool, error))` — takes a write lock for the entire cycle (snapshot inside the lock, then adds missing, deletes unwanted markers); garbage-collects non-uuid names, dirs, symlinks with warnings.
- `Status() *Status` — parses `/run/styrman-firewall/status`, 2 s TTL cache.
- `Availability(st *Status, now time.Time)` — evaluates whether the host half is installed, current, and ready (version, timing, config error).
- `Derive(lanAccess bool, changedAt, uuid, st, now)` — maps the record's intent and the host's status word into on / pending / error / unavailable states for the UI.

**Tests: 100+ new cases across netpolicy_test.go and script_test.go:**
- Marker create/delete idempotence + uuid validation.
- Sync symlink/dir/non-uuid/sys-setup-x refusal, markers-dir symlink refusal.
- Status parsing: missing, > 64 KiB, malformed, version outdated, stale (absent file).
- Availability table (file absent, stale, version mismatch, conf error).
- Derive state machine (on, pending, error, unavailable, grace period).
- `TestMinScriptVersionMatchesScript` confirms host and Go versions agree.
- `TestFirewallScriptInNetns`: runs deploy/firewall/test.sh under `unshare -Urnm`, skips on systems without netns support (loudly); verifies chain state before/after via `sudo -n iptables -S` when sudo available.

## Integration with the auto-updater and boot

**Deploy:**
- Compose gains `- /run/styrman-firewall:/run/styrman-firewall` (rw) volume and `STYRMAN_FIREWALL_DIR=/run/styrman-firewall` environment.
- `host_converge()` is gated on a successfully verified and deployed SHA; it runs on every tick, installing from git if the deployed files differ. The updater already builds and runs root code (images), so no new trust boundary.
- `--check` mode now correctly refuses to install under the check gate, only reporting drift.

**Boot:**
- Entry point will call `netpol.Sync(wantLAN)` after `ensureRepos` and before the listener, capturing initial intent from `projects.json` into markers.
- A 2-minute re-sync ticker reconciles the two every 120 s (captured in a separate goroutine, not the orchestrator's RunIdleReaper).
- No in-guest state: everything is host-driven.

## Verification

**Static analysis:** `bash -n` and `shellcheck -S` on all shell scripts (3 units verified by `systemd-analyze verify`).

**Unit tests (Go):** All existing packages pass `go test -race ./...` with new netpolicy tests at 5.5 s. Config golden test for bridge-name derivation (sdb-<11hex> matches the expected hash).

**Integration (shell):** deploy/firewall/test.sh runs 83 checks in an isolated netns, proving: base rules apply and stay idempotent, one marker produces the exact rule shape and position, removal cleans up, malformed markers are handled, and the spool drains correctly. Orchestrated by `internal/netpolicy/script_test.go`.

**Measurements:** Path-unit trigger latency 30–62 ms (five cold runs on this host); script execution 80–120 ms inside the netns test; toggle application end-to-end ~200 ms. Status file atomicity tested with failing mktemp mid-run (the exit trap drains the spool even on failure).

## Agents dispatched

- **reviewer**: 1 high finding (fixed), 1 medium finding (fixed), 2 low findings (fixed). Reviews: host_converge ran under --check despite the promise to change nothing. Fixed: --check now reports without applying. Marker-write race if two PUTs arrived: fixed with server-level netMu. Missing test coverage for drain-on-exit. All rebutted or fixed.
- **security**: 1 high finding (fixed), 3 medium findings (fixed), 7 low findings (fixed), 3 findings rebutted in writing. High: install.sh could follow symlinks in the state dir when creating marker paths; fixed with plain mkdir, symlink refusal, owner assertion, and chown -h in host-setup.sh §5. Medium: unbounded spool, infinite StartLimit revive, deletion-by-number race against another root writer; fixed with dedicated 1 MiB tmpfs, StartLimitBurst=20/60s, and deletion-by-spec. Low: marker ownership, test.sh netns guard, -w flag, status mode, and three rebutted: transitive SSH-key→LAN→host risk (accepted and documented), Docker default-address-pools overlap (recorded follow-up for Phase 4), working-tree Phase 2 files in commit scope (staged separately). Found while fixing: write_status inside a command substitution hid failures; fixed with direct call and inherit_errexit, test 9 now proves the drain via failing mktemp.
- no verifier agent this phase — `go test -race ./...` run by the author: all packages pass (netpolicy 5.5 s); `deploy/verify-commit.sh` runs on the checkpoint commit.

## Files changed

Host-facing:
- `deploy/firewall/styrman-firewall.sh` (new, 300 lines)
- `deploy/firewall/install.sh` (new, 200 lines)
- `deploy/firewall/styrman-firewall.{service,timer,path}` (new, 50 lines each)
- `deploy/firewall/test.sh` (new, 400 lines)
- `deploy/host-setup.sh` (§3b heredoc removed, §5 changes, §5c installer call)
- `deploy/update/styrman-update` (host_converge function, ~150 lines)
- `deploy/update/styrman-update.service` (env line)
- `deploy/verify-commit.sh` (shell lint, systemd-analyze)

Server-facing (intent capture only, no orchestration):
- `internal/netpolicy/netpolicy.go` (new, ~400 lines, 5 exported types)
- `internal/netpolicy/script_test.go` (new, ~100 lines)
- `internal/netpolicy/netpolicy_test.go` (new, ~1500 lines, 40+ test cases)
- `internal/config/config.go` (NetPolicyDir, FirewallRunDir accessors; no breaking changes)

Docker/delivery:
- `deploy/docker-compose.yml` (volume and env for /run/styrman-firewall)
- `deploy/Dockerfile.server` (no changes; server stays unprivileged)

## Requirements

R12 amendment approved 2026-09-08; ADR-065 dated 2026-09-08. No R1–R11 changes; Phase 1 does not satisfy any new requirement items (satisfaction arrives across phases 2–5). The feature is bounded by R12's unconditional cross-project isolation and ip_forward=0 statements.

## Notes

The live host carries a drift (four DROPs as of 2026-08-30; repo has six as of 2026-09-02). Phase 0 (ADR-065 + R12 amendment + docs) is complete and committed. Phase 1 code is ready. Operator action required before the feature is live on the production host: `sudo bash deploy/firewall/install.sh` from `/opt/styrman` at this commit's tag, which both discovers the LAN prefix and installs the script + units for the auto-updater to maintain. Until then the live host is unchanged; the feature's host half is not installed; every project reports `unavailable` and the server refuses enables (fail-closed). TODO.md line ~120 records the historical delivery gap (v1.19.0 claimed 100.64/198.18 drops were done; repo has them, production doesn't — host_converge closes this class of gap).
