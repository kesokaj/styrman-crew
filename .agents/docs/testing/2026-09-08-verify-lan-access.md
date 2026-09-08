---
owner: verifier
date: 2026-09-08
---
# Verification — per-project LAN access toggle   (2026-09-08)

Range reviewed: 2ef1e51..HEAD (commits 893bf03 Phase 1, 4ebd5d9 Phase 2,
03fedc8 Phase 3, 3aa7acf Phase 4; working tree == HEAD, `git status --short`
clean at start).

## Status: PASS

Scope: complete for every tier this run is responsible for — Go unit/race
(incl. the netns firewall-script test, which RAN, not skipped), the vendored
Caddy module, frontend check/lint/vitest, shell lint + systemd unit
verification, `deploy/verify-commit.sh` on HEAD in an isolated worktree, the
test-count/deletion diff, and a full changed-file coverage trace. NOT run by
this session: `integration.sh` and `web/e2e/run.sh` (author-run this session,
cited below as author evidence, not re-verified here) and anything requiring
root, `/var/lib/styrman`, `/etc/styrman`, `/run/styrman-firewall`, or a
Docker image build — all excluded per the dispatch.

## Commands

| # | Command | Result |
|---|---|---|
| 1a | `go build ./...` | PASS, exit 0, no output |
| 1b | `go vet ./...` | PASS, exit 0, no output |
| 1c | `gofmt -l internal/ cmd/` | PASS, no files listed |
| 1d | `go test -race -count=1 ./...` | PASS — 16 packages `ok`, `internal/netpolicy` 6.518s (vs ~1s for others — the netns test ran), 2 packages `[no test files]` (`cmd/wsprobe`, `web`) |
| 1e | `go test -race -count=1 -v ./internal/netpolicy/...` (to confirm skip status) | `TestFirewallScriptInNetns` **RAN** (not skipped): `deploy/firewall/test.sh` output shows `passed=83 failed=0`; no `SKIP` line anywhere in the verbose log. `unshare -Urnm` + sysfs remount + `/usr/sbin/iptables` all available on this host; `sudo -n true` exits 0, so the host `STYRMAN-FWD` chain before/after comparison also ran (not the SKIP branch — no SKIP text emitted) |
| 2 | `(cd deploy/caddy/ingress && go test -count=1 ./...)` | PASS — `ok github.com/spgo/devbox/caddy/ingress 2.030s` |
| 3a | `cd web && npm run check` | PASS — `tsc -b`, no output, exit 0 |
| 3b | `npm run lint` | PASS — `oxlint`, no output, exit 0 |
| 3c | `npm test` | PASS — `Test Files 8 passed (8)`, `Tests 80 passed (80)` — matches the plan's "80 tests / 8 files" |
| 4a | `bash -n` over `deploy/firewall/*.sh` + `deploy/update/styrman-update` | PASS — 4/4 files OK |
| 4b | `shellcheck -S warning deploy/firewall/*.sh deploy/update/styrman-update` | PASS — clean, no findings |
| 4c | `systemd-analyze verify deploy/firewall/{styrman-firewall.service,styrman-firewall.timer,styrman-firewall.path}` | PASS — exit 0, no output |
| extra | `SKIP_WEB_FRESHNESS=1 bash deploy/verify-commit.sh HEAD` | PASS — detached worktree of `3aa7acfb284f`: build/vet/race-test/gofmt clean, vendored ingress module ok, harness mirror integrity ok, host scripts (`bash -n`, shellcheck, `systemd-analyze verify`) clean, server + runtime images built (seeds 28 skills / 7 subagents), final line `OK: commit 3aa7acfb284f builds and tests clean in isolation — safe to tag` |

## Not re-run (author-run this session, cited as evidence only)

- `bash .agents/docs/testing/styrman/integration.sh` — log `.../scratchpad/integration-phase2b.log`: `passed=78 failed=0 skipped=4 aborted=0`, `integration exit: 0`. Tail shows the LAN-specific checks passing: "marker present before the delete", "delete removed the LAN marker with the record".
- `bash web/e2e/run.sh` — log `.../scratchpad/e2e-phase3b.log.plain`: `passed=84 failed=0 skipped=2`, `e2e exit: 0`. Tail shows non-LAN checks (appearance, delete flow); the LAN-specific checks (Ports-tab block disabled + "Not available", `PUT` refused with 503 and nothing persisted, Settings "Host firewall" not-installed row) are further up the same log per the plan's Phase 3 checklist entry ("84 passed / 0 failed / 2 skipped on 2026-09-08").

## Test-count comparison (writing-files rule)

- `git diff 2ef1e51..HEAD -- '*.go' | grep '^-func Test'` → **empty** (no pre-existing Go test function deleted).
- Added Go test functions: **23** (`git diff 2ef1e51..HEAD -- '*.go' | grep -c '^+func Test'`) — breaks down as `internal/netpolicy` 11 (across `dir_test.go`, `status_test.go`, `script_test.go`'s 2), `internal/httpapi/network_test.go` 7, `internal/project/store_lan_test.go` 4, `internal/config/bridgename_test.go` 1.
- Added TS tests: **11** (`git diff 2ef1e51..HEAD -- '*.ts' '*.tsx' | grep -c` for `test(`/`it(`) — matches `web/src/lib/lan.test.ts`'s 11 cases exactly.
- `git diff --stat 2ef1e51..HEAD`: 99 files changed, 6552 insertions(+), 214 deletions(-) (includes `web/dist` rebuild artifacts and doc/bug/change records).

## Coverage of changed code

Go — server/host:
- `internal/netpolicy/{dir,status,netpolicy}.go` — `internal/netpolicy/{dir_test,status_test}.go` (11 funcs) + `script_test.go`'s netns run of `deploy/firewall/test.sh` (83 checks, ran live)
- `internal/config/config.go` (`NetPolicyDir`, `FirewallRunDir`) — `internal/config/bridgename_test.go` (golden vector) + exercised transitively by every `internal/netpolicy`/`internal/httpapi` test that sets `cfg.FirewallRunDir`
- `internal/project/store.go` (`LanAccess`, `SetLanAccess`) — `internal/project/store_lan_test.go` (4 funcs: round trip, `LanChangedAt` only on change, rollback, no `.tmp` left)
- `internal/httpapi/network.go` (new GET/PUT) — `internal/httpapi/network_test.go` (7 funcs: validation/isolation, enable gates, tenant-generic messages, no-op throttle, host view admin-only, 20 concurrent PUTs, delete removes marker)
- `internal/httpapi/server.go` (`projectView` LAN fields, `netMu`, routes, `deleteProject` marker removal) — `network_test.go`'s `TestDeleteProjectRemovesMarker` covers delete; list/get fields covered by `TestNetworkHostViewIsAdminOnly` + others. **Kicks after `EnsureRunning`** in `ensureFor`/`createProject`/`startProject` have **NO Tier-1 test** (no orchestrator Docker double) — explicit known gap, matches `TODO.md` and the plan's own Risks section; covered only by the opt-in live integration block and manual testing, neither run this session.
- `internal/httpapi/explorer.go` (`hostFirewall` in settings) — `network_test.go`'s `TestNetworkHostViewIsAdminOnly`
- `internal/httpapi/isolation_test.go` (route-list addition) — the file itself is the test asserting the two new routes are guarded
- `cmd/styrman/main.go` (`wantLAN`, boot `Sync`+`Kick("boot")`, Adopt kick, 2-min re-sync ticker) — `Sync`'s core lock/add/remove logic is unit-tested in `internal/netpolicy/dir_test.go` (incl. the Sync-vs-SetMarker lock case); the wiring itself (this exact boot sequence) has no dedicated Go test but is exercised end-to-end every time `integration.sh`'s self-hosted server boots (author-run evidence above: marker present/removed checks passed)
- `deploy/firewall/styrman-firewall.sh` — `deploy/firewall/test.sh` under `internal/netpolicy/script_test.go` (83 checks, ran)
- `deploy/firewall/test.sh` — is the test
- `deploy/firewall/{styrman-firewall.service,.timer,.path}` — `systemd-analyze verify` (clean) + `deploy/verify-commit.sh`
- `deploy/firewall/install.sh` — **KNOWN GAP, stated by the plan itself**: `bash -n` + shellcheck + `--dry-run` only; no automated exercise of the real install path (root-only, host-mutating). Not re-run interactively this session (would touch `/etc/styrman`/`systemd`, excluded by the dispatch).
- `deploy/host-setup.sh` (§3b → pointer to `install.sh`) — `bash -n` only via `deploy/verify-commit.sh`; no dedicated test
- `deploy/update/styrman-update` (`host_converge()`) — **KNOWN GAP, stated by the plan itself**: `bash -n` + shellcheck clean; no runtime test (root-only, requires the updater's signed-tag machinery)
- `deploy/update/styrman-update.service` (env addition) — no test; reviewed by rereading only
- `deploy/verify-commit.sh` (shell-lint + `systemd-analyze verify` steps) — self-verified by running it this session against HEAD (PASS)
- `deploy/docker-compose.yml` (volume + env) — no automated test; reread only (compose is not exercised by any tier here)

TypeScript/React:
- `web/src/lib/lan.ts` (`lanChip`, `lanLine`, `formatClock`) — `web/src/lib/lan.test.ts` (11 tests, all pure-function derivations)
- `web/src/lib/lan.test.ts` — is the test
- `web/src/lib/api.ts` (`Project.lanAccess/lanChangedAt/lanState/lanMessage`, `Settings.hostFirewall`, `projectNetwork()`, `setProjectLan()`) — types check via `tsc -b`; the two new verbs are called only when the LAN switch is enabled and clicked, which e2e never reaches (the switch stays disabled in the `unavailable` state e2e runs against) — **UNCOVERED by any automated test**; only exercised by the manual on-host check (not run this session, per plan a pending Phase 3/5 item)
- `web/src/components/ui.tsx` (`Switch`) — rendered (disabled) by e2e's LocalNetwork checks; keyboard/enabled-state interaction is untested by e2e here since the switch stays disabled throughout the e2e run
- `web/src/components/LocalNetwork.tsx` — e2e (3 checks: block visible + switch disabled + "Not available" copy; `PUT` → 503 with nothing persisted; no native dialog) — the `pending`/`on`/`error` render branches, the `ConfirmDialog`, and the Retry button are **NOT exercised** (host stays `unavailable` throughout e2e) — matches the plan's own stated gap ("applied-state UI … manual on the converged host")
- `web/src/components/SidePanelPorts.tsx` (renders `LocalNetwork` at top) — indirectly covered by the same e2e checks (the block is visible)
- `web/src/components/LanBadge.tsx` (sidebar/dashboard chip) — its logic (`lanChip`) is vitest-covered; the JSX render itself is **NOT exercised by e2e** in the `unavailable` state, since `lanChip` explicitly renders nothing off/unavailable (`lan.test.ts`: "renders nothing while off or unavailable") — same known gap (applied-state UI, manual only)
- `web/src/components/Sidebar.tsx` (chip usage) — same as `LanBadge.tsx`: no automated render exercise of an on/pending/error chip
- `web/src/routes/index.tsx` (dashboard card chip, 2-line diff) — same as above, no automated render exercise
- `web/src/routes/settings.tsx` (Host firewall admin section) — e2e: "settings shows the host firewall as not installed (admin view)" — the populated (installed, with prefixes/warnings) render is **NOT exercised**, manual-only
- `web/e2e/run.mjs` / `web/e2e/run.sh` — the checks themselves; author-run this session (84/0/2)

Docs/records (`.agents/docs/**`, `AGENTS.md`, `README.md`, `TODO.md`, `deploy/*.md`, `deploy/.env.example`, `deploy/README.md`) — prose, no test tier applies; reviewed by reread per the plan's own checklist, not re-read line-by-line by this verifier pass.

`web/dist/client/**` — build artifacts from `npm run build`/`vitest`/e2e's own rebuild; not source, not separately tested.

### FAIL / UNKNOWN rows (summary)

No FAIL rows — every failing/erroring command above is none; all commands PASS.

UNKNOWN / stated gaps (all previously known, named by the plan itself — none newly discovered this session):
1. `internal/httpapi/server.go` kicks after `EnsureRunning` (`ensureFor`/`createProject`/`startProject`) — no Tier-1 test, no orchestrator Docker double.
2. `deploy/firewall/install.sh` — lint/dry-run only, no live-install test (root-only).
3. `deploy/update/styrman-update`'s `host_converge()` — lint only, no runtime test (root-only, signed-tag machinery).
4. Applied-state UI across the board — `LocalNetwork.tsx`'s pending/on/error branches + `ConfirmDialog` + Retry, `LanBadge.tsx`/`Sidebar.tsx`/`routes/index.tsx` chip rendering, `web/src/lib/api.ts`'s `projectNetwork`/`setProjectLan` verbs, `routes/settings.tsx`'s populated Host-firewall row — none reachable while the host firewall is `unavailable` (the state every e2e run is in); manual-only, pending a converged production host.
5. `cmd/styrman/main.go`'s exact boot-wiring statements (placement of `Sync`/`Kick` calls) — no dedicated Go test; exercised transitively by `integration.sh` booting the real server (author-run, not re-run this session) and by rereading.
6. `deploy/host-setup.sh`, `deploy/update/styrman-update.service`, `deploy/docker-compose.yml` — lint/reread only, no runtime test.

## Environment

- OS: Linux 6.12.107+deb13-amd64
- Go: per `go build`/`go test` above (module toolchain resolved automatically; no explicit `go version` captured, but the harness's runtime image build step reports builder pin `golang:1.25.13-trixie`)
- Node/npm: whatever `web/package.json` pins (`npm run check/lint/test` all ran clean)
- `sudo -n true` → exit 0 (passwordless sudo available on this host, so the live `iptables -S STYRMAN-FWD` before/after comparison inside `TestFirewallScriptInNetns` ran, not skipped)
- Docker: not touched — no image build performed by this verifier
- `/var/lib/styrman`, `/etc/styrman`, `/run/styrman-firewall`: not touched by this verifier

## What was NOT checked

- The operator's real host install (`sudo bash deploy/firewall/install.sh` from `/opt/styrman` at the Phase-1 tag) — plan Phase 1 still has this unticked (`[ ]`).
- The live applied-state check on the converged production host (toggle on → ping succeeds → toggle off → ping fails within a second; `LAN` chip pending→on; dialog copy) — plan Phase 3 and Phase 5 items, both still unticked.
- `integration.sh`'s opt-in live LAN block (`STYRMAN_IT_LIVE_LAN=1`) — not run by the author's cited logs either (those runs used the default scratch-dir path, 4 auto-SKIP as expected on this firewall-hardened host).
- Anything requiring root, a Docker image build, or writes under `/var/lib/styrman`/`/etc/styrman`/`/run/styrman-firewall` — excluded by the dispatch.
- Manual latency measurement beyond what's already recorded in the Phase 1 changelog entry (30–62 ms trigger / 80–120 ms script run, netns-measured) — not re-measured.
