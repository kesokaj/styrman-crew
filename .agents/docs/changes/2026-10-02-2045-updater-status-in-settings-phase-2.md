---
owner: main
date: 2026-10-02
---
# Settings tells admins when the host's auto-updater is failing   (2026-10-02)

Phase 2 of `plans/2026-10-02-stale-keys-browser-liveness-updater-status.md`:
item 5 of the v1.40.0 handoff's decisions, in the shape the operator chose
("Yes, updater status in Settings (Recommended)"; "Why do we need a special gcp
fix?"). This host's updater failed on every tick for ten days (BUG-084), and
nothing outside `journalctl` said so. Design: ADR-081.

## What changed
- `deploy/update/styrman-update`: at the end of every real tick, one EXIT
  handler (`on_exit`, installed once for every mode, armed below the lock and
  never under `--check`) writes one JSON line with six keys to
  `STYRMAN_UPDATE_STATUS_FILE`. Each failure site sets its code inside its
  failure branch: `fetch-auth` (git fetch output matched an auth error), `fetch`,
  `no-tags`, `signature`, `latched`, `rolled-back`, `deploy-failed`. Any other
  failure is `error`, and a clean exit is `ok`. The write refuses a directory
  that is a symlink, not owned by the trusted uid, or group/world-writable; on
  a refusal it logs a WARN and the tick's own result is unchanged. Two gated
  test hooks: `__write-status` and `__exit-status`. The hand-run arm of the
  `DOCKER_CONFIG` case no longer sets its own trap; `on_exit` removes its
  directory.
- `deploy/update/styrman-update.service`:
  `Environment=STYRMAN_UPDATE_STATUS_FILE=/etc/styrman/status/updater.json`.
- `deploy/docker-compose.yml`: the server gets the same variable and
  `/etc/styrman/status:/etc/styrman/status:ro`, the only part of `/etc/styrman`
  it sees.
- `internal/updatestatus` (new): reads at most 4 KiB, decodes strictly and
  re-validates every field. The states are `ok`, `failing`, `stale` (older than
  60 minutes) and `unknown`, which is never shown as ok. With no file, or no
  configured path, it returns nothing.
- `internal/config/config.go`: `UpdateStatusFile`, no default.
  `internal/httpapi/explorer.go`: `GET /api/settings` carries `updater` for
  admins only.
- `web/src/lib/updaterNotice.ts` (new), `web/src/lib/api.ts`,
  `web/src/routes/settings.tsx`: one notice under the Settings heading, built
  from the code. Nothing the updater or a remote wrote is rendered. A
  `deploy-failed` notice does not claim which version the host runs.
  `web/dist/client` rebuilt.

## The Ports panel's 500 during a rebuild (BUG-088)
The first Phase 2 verifier run failed Tier 4 on one uncaught `500` from
`GET /api/projects/<id>/ports`. A scratch repro polled that endpoint while it
toggled a project's network mode four times, then stopped, started and deleted
the project. It got three `500 "port scan exited 9"` during the toggles. While
a stop is under way, Docker still reports the same container as running, and
the exec the shutdown kills exits 9. BUG-023's guard asked only "gone or
replaced?". `Orchestrator.GuestGoingAway` (`internal/orchestrator/ports.go`)
also answers yes when the project's lock is held: a stop, a mode toggle or a
boot is in flight. `ListeningPorts` and `browserExec`
(`internal/httpapi/browser.go`, which had copied the guard) both call it. Tests:
`internal/orchestrator/ports_lifecycle_test.go` (2) and
`internal/httpapi/browser_goingaway_test.go` (1). Both fail without the fix:
"port scan exited 9", and a 500 from the browser status route. The repro
against the fixed server: no 500s (one run each way). Record:
`bugs/BUG-088-ports-500-while-a-stop-is-in-flight.md`.

## Every caller of the status variable (`.agents/rules/mechanism-reaches-every-caller.md`)
- `styrman-update.service` (the timer's tick). `host-setup.sh` and the
  updater's own `self_install` copy this unit whole, so neither changed.
- `deploy/styrman-ctl` `UPDATER_ENV` (`update --setup`'s `--check`, which
  writes nothing, and `update --redeploy`'s `--force`), as
  `${ETC_DIR}/status/updater.json`.
- `deploy/docker-compose.yml`, the server's side.
- The two suites, each with a file under its own state dir.
On a host taking this release, the deploying tick runs the PREVIOUS updater,
and `self_install` then installs the new script and unit. The first status is
written one tick (5 minutes) later.

## Tests
- `internal/config/styrman_update_status_test.go` (new, 10 tests,
  `TestStyrmanUpdateStatus*`). Against the old script, 9 of them failed, and
  so did the extended `TestStyrmanCtlUpdateSetupPlan`. The early-refusal test
  passes there trivially (nothing to write); with one global's initialisation
  removed it fails ("unbound variable"). The text pins are NOT evidence of a
  real root tick.
- `internal/updatestatus/updatestatus_test.go`,
  `internal/httpapi/settings_updater_test.go`. Removing the strict decode, or
  the admin check, fails its test.
- `web/src/lib/updaterNotice.test.ts` (7 tests). Web unit total: 210 in 18
  files.
- Tier 2: three new checks. Before the change: `passed=120 failed=2
  skipped=3`. After: `passed=122 failed=0 skipped=3` (125 checks).
- Tier 4: one new check. Before the change: `passed=98 failed=1 skipped=2`,
  that check alone, which is also its deliberate-failure run. After:
  `passed=99 failed=0 skipped=2` (101 checks).
- Verifier log: `testing/2026-10-02-verify-stale-keys-browser-liveness-updater-status-phase-2.md`
  (its first run FAILED on the BUG-088 race; updated in place after the fix).
- Not covered by any tier: a real root tick on a host; the file appearing on a
  host after a release (this host cannot take one until BUG-084's token is
  replaced); the fresh-host path.

## Docs
`specs/styrman/versioning.md` (the status file and its codes),
`specs/styrman/deploy.md` (the `/etc/styrman` layout, the mount, the
`styrman-ctl` rows), `specs/styrman/api.md` (the settings payload),
`specs/styrman/guide.md` (a troubleshooting row), `deploy/README.md`,
`testing/styrman/README.md` (counts and the test inventory). The
`documentation` agent wrote the spec and README edits from a closed fact list,
and they were read against the code before staging. ADR-081.

## Learnings for the local host
None from a second target: GCP was not touched. Two facts about the shared
path, checked on this host: Docker creates a missing bind source for
`/etc/styrman/status` as an empty root-owned 0755 directory, read-only inside
the container (a throwaway compose service run here). On this host,
`t.TempDir()` is 0775 under umask 002, so a test that needs a "trusted"
directory must `chmod` it.
