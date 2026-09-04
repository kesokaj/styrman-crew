---
owner: main
date: 2026-09-04
status: Phases 1–2 done (2026-09-04); release v1.21.0 via /preflight pending
---
# Plan — un-nest the project homes from the setup VM's mount   (2026-09-04)

## Why

Project homes live at `HomesDir/<user>/p/<uuid>` and the setup VM mounts their parent, `HomesDir/<user>`, as its `/home/<guest>` (`internal/orchestrator/orchestrator.go:594-602`). A root guest in the setup VM can therefore replace `p/<uuid>` (or `p` itself) with a symlink; Docker resolves a bind source on the host, so the next project start would mount any host path as that project's home. v1.20.0 added guards (`files.Trust`, `projectHomeIsReal`/`realBelow`, the confined janitor and `MkdirAll`s — BUG-002/003/004) but the check-to-mount window remains (`reviews/2026-09-04-security-v1.20.0.md`, medium). The operator chose to remove the layout that needs the guards ("Go with your recommendation: un-nest", 2026-09-04; the decision page). REQUIREMENTS: R3 (per-project homes) and R12 (isolation) unchanged; R1's boot budget must hold. Prior records in this area: BUG-002, BUG-003, BUG-004; rule `.agents/rules/guest-writable-paths.md`.

## Scope

In: the setup VM's home becomes `HomesDir/<user>/base`; a one-time host-side migration moves an existing base home's contents into `base/`; a stale setup container (old bind) is recreated; the guards stay. Release as `v1.21.0` through `/preflight`.
Out: project-home paths, the credential broker, the runtime image, and the harness — all untouched. The `files.Trust`/`realBelow` guards are not removed (belt and braces).
Follow-ups noted, not built: nothing.

## Surfaces touched

- `internal/config/config.go` — `UserHome` (now `…/base`), new `TenantDir`.
- `cmd/styrman/main.go` — `migrateLegacy` step 1 (its rename target is `UserHome`, now `…/base`: it must `os.MkdirAll(TenantDir(user))` first — today nothing creates the parent; a legacy home therefore migrates straight into the new layout, and `migrateBaseHomes`, which runs right after it, finds nothing to move for that user); new `migrateBaseHomes`; the janitor is unchanged (it opens `HomesDir/<u>` then `p/<uuid>`).
- `internal/orchestrator/orchestrator.go` — `EnsureSetup` (stale-bind recreate; real-directory check), `EnsureRunning` call sites that mean the tenant dir (lines 226, 272, 275, 282).
- `internal/httpapi/server.go:726-727` (project create) — CHANGES: both calls mean the parent of `p/` and become `TenantDir` (see Interfaces); the pre-create exists so the Files "Home" scope works before the first boot, and it must land at `HomesDir/<user>/p/<id>`, where the credential sweep's glob (`credentials.go:262`) and `ProjectHome` look.
- `internal/httpapi/setup.go:34,127,214`, `credentials.go:263`, `git.go:73`, `explorer.go:443` — read the BASE home via `UserHome`: no code change, the path moves with the function.
- `internal/config/config.go:364` (`LegacyWorkspaceExists`: `UserHome/projects/<dir>`) and `cmd/styrman/main.go:228` (`migrateWorkspaces`: `ContainedPath(UserHome, "projects/"+p.Dir)`) — the pre-ADR-027 workspace location inside the base home. No code change, but they depend on ORDER: `migrateBaseHomes` (main.go, right after `migrateLegacy` at :44) moves a stray `projects/` into `base/` before `migrateWorkspaces` runs (after `orchestrator.New`, :66), so both keep finding a legacy workspace at `UserHome/projects/<dir>` = `base/projects/<dir>`. Unit test: `migrateBaseHomes` moves `projects/` too. No such workspace exists on this host (migrated at v1.19.x), so the fixture only asserts the directory moved.
- Comments at the repointed lines describe the guest-writable boundary and must be rewritten to the new one — `orchestrator.go:263-297` (the create path: the tenant dir is server-only; `base/` and `p/<uuid>` are the guest mounts), `:592-593` (`EnsureSetup`), `server.go:719-722` (`createProject`), `main.go:369-378` (the janitor), and the rule file `.agents/rules/guest-writable-paths.md` (its first paragraph states the old layout).
- Docs: ADR-064 in `DECISIONS.md`; `AGENTS.md` architecture bullet; `.agents/docs/specs/styrman/deploy.md` (the per-project-homes paragraph), `guide.md`/`runtime-image.md` where they name the setup VM's home; `README.md` Status line for v1.21.0 (release step).

## Interfaces

```go
// internal/config/config.go
// TenantDir is HomesDir/<user>: the server-only parent of base/ and p/.
// No guest ever mounts it (ADR-064).
func (c *Config) TenantDir(user string) string { return filepath.Join(c.HomesDir, user) }
// UserHome is the setup VM's home: HomesDir/<user>/base (was HomesDir/<user>).
func (c *Config) UserHome(user string) string { return filepath.Join(c.TenantDir(user), "base") }
// ProjectHome unchanged: HomesDir/<user>/p/<uuid>.
```

```go
// cmd/styrman/main.go — called right after migrateLegacy, before the orchestrator adopts state.
// For every directory HomesDir/<user>: if it holds any entry other than "base" and "p",
// create base/ (0700; the server's own uid — the guest runs with the same uid, config.go:165)
// and os.Rename each such entry into base/. Refuse a user whose "base" exists but is not a
// real directory (Lstat: symlink or file) — log an error, touch nothing. Idempotent; logs
// "migrated base home" with the entry count per user. Errors never abort startup.
func migrateBaseHomes(cfg *config.Config, logger *slog.Logger)

// migrateLegacy step 1 (main.go:182) gains one line before its os.Rename:
//   os.MkdirAll(cfg.TenantDir(user), 0o700)   // the parent of base/, new since ADR-064
// so StateDir/home renames to HomesDir/<user>/base directly; a test is added (the function has
// none): a StateDir with home/ → homes/<user>/base/ holds it; a second run is a no-op. Order stays
// migrateLegacy → migrateBaseHomes.
```

```go
// internal/orchestrator/orchestrator.go — EnsureSetup
// Before reusing an existing setup container: read insp.Mounts; the mount whose Destination
// is "/home/<guestUser>" must have Source == o.cfg.UserHome(user); otherwise log
// "setup VM home moved — recreating" and force-remove it (same shape as the guest-changed
// branch at :572-581). Before creating: os.MkdirAll(TenantDir) then mkdirAllConfined(TenantDir,
// "base"); then realBelow(TenantDir, UserHome) — a symlinked base is refused with an error.
// EnsureRunning: the four UserHome(user) uses that mean the parent of p/ become TenantDir(user).
```

```go
// internal/httpapi/server.go:726-727 — createProject: os.MkdirAll(TenantDir) then
// files.MkdirAllPerm(TenantDir, "p/<id>", 0o700).
```

No API, JSON, or env change. `files.Trust(cfg.HomesDir, …)` is unchanged: the tenant root it confines to is still `HomesDir/<user>`, now containing `base/` and `p/`.

## Failure modes

- Migration cannot create or rename for a user (permissions, disk): logged per user, startup continues; that user's setup VM would mount an empty `base/` (fresh onboarding) while their old files stay in the tenant dir — visible in the log, recoverable by hand (`mv`). Not silent: the log line names the path.
- `base` is a planted symlink or a file: migration refuses and logs; `EnsureSetup` refuses to bind it (`realBelow` error → the API returns the existing 500 shape "setup container…"); the user's onboarding is blocked until the operator removes it — the safe failure.
- A setup VM is running during the deploy: its mount is the old tenant dir; the renames happen underneath it (same inode; the guest sees its files under `~/base/`). The next `EnsureSetup` sees the stale bind and recreates it. An in-progress `styrman-setup` sign-in is interrupted once; nothing is lost (the broker already holds harvested credentials, and the files are in `base/`).
- A project VM: unaffected — its bind is `p/<uuid>`, never touched.
- Downgrade to v1.20.x after the migration: the old server would mount the tenant dir again and see `base/` and `p/` inside an "empty-looking" home — readiness flags read the old paths and show not-set. Recorded as a known one-way migration in ADR-064; the rollback story is "roll forward or move `base/*` back up by hand".

## Verification

- Unit (`go test ./... -race`): `config`: `TenantDir`/`UserHome`/`ProjectHome` shapes. `cmd/styrman`: `migrateBaseHomes` — old-layout user (dotfiles + `p/` + a stray file) → everything but `p/` lands in `base/`, `p/` byte-identical, second run no-op; already-migrated user untouched; user with `base -> /etc` symlink refused and untouched; a user dir that is itself a symlink skipped. `orchestrator`: a pure helper `setupHomeStale(mounts []types.MountPoint, guestUser, want string) bool` tested with a fake mounts slice; `realBelow` already tested.
- Integration (`integration.sh styrman-runtime:test`, 65 checks): unchanged suite must stay green — project creation now creates `TenantDir/p/<uuid>` through the confined mkdir.
- E2E (`web/e2e/run.sh`): unchanged suite green (the setup banner reads `/api/setup/status` from the base home).
- Upgrade fixture (host, isolated server, no production): a state dir laid out the OLD way — `homes/<me>/projects/legacy-ws/keep.txt` (a pre-ADR-027 leftover, expected at `base/projects/legacy-ws/` afterwards), `homes/<me>/.claude/marker`, `homes/<me>/.styrman/setup-done` (the marker `internal/httpapi/setup.go:29` reads), `homes/<me>/p/<uuid>/keep.txt` — plus a setup container created by the v1.20.0 binary (bind = tenant dir); start the NEW server: log shows `migrated base home … entries=N`; `homes/<me>/base/.claude/marker` exists; `p/<uuid>/keep.txt` untouched; `GET /api/setup/status` reports done (reads the marker from base); trigger the setup VM (`POST /api/setup/…` per `api.md`) → `docker inspect` shows the `/home/<guest>` mount Source ending in `/base` and the old container gone. Boot→ready of a project still within R1 (integration prints it).
- E2E criteria: golden path — fresh install: sign-in state lands in `base/`, projects boot, `sh-agent status` no drift, settings roster shows. Edge — upgrade from the old layout as above, and a planted `base` symlink refused with a log line.

## Phases

### Phase 1 — config, migration, orchestrator, server
- [x] `config.TenantDir` + `UserHome` → `base`; config test — verify: unit, `go test ./internal/config/`, AI
- [x] `migrateLegacy` step 1 creates `TenantDir` before its rename (test added); `migrateBaseHomes` in `cmd/styrman/main.go`, wired after `migrateLegacy`; tests — verify: unit, `go test ./cmd/styrman/ -race`, AI
- [x] `EnsureSetup`: stale-bind recreate (`setupHomeStale`), confined `base` creation, `realBelow(TenantDir, UserHome)`; `EnsureRunning` (orchestrator.go:226,272,275,282) and `createProject` (server.go:726-727) call sites → `TenantDir`; the boundary comments at those lines, the janitor's, and `.agents/rules/guest-writable-paths.md` rewritten for the new layout; tests — verify: unit, `go test ./internal/orchestrator/ ./internal/httpapi/ -race`, AI
- [x] Full suite green; changelog entry; security rounds 1–2 (blocking high closed by the root-confined migration and the startup sweep); committed `f3882ac`, pushed — verify: AI

### Phase 2 — docs, suites, upgrade fixture, release
- [x] ADR-064; `AGENTS.md`, `deploy.md` wording; `documentation` cold-read fixed `api.md`, `guide.md`, `README.md` — verify: AI
- [x] `integration.sh` (62/0/3, boot→ready 1.72 s) and `web/e2e/run.sh` (61/0/2) green on `styrman-runtime:test` (`testing/2026-09-04-verify-unnest-phase2.md`); the upgrade fixture — PASS in three runs (`testing/2026-09-04-e2e-unnest-upgrade-fixture.md`) — verify: integration + e2e + fixture, `verifier`, AI
- [ ] `/preflight` for `v1.21.0` (reviewer + security over the release diff; `verify-commit.sh`; signed tag; auto-updater deploy; live check) — verify: AI, live check both

## Risks

- The migration runs as the server's uid over directories the guests wrote with the same uid: renames need only write permission on the tenant dir, which the server created. A tenant dir created by an older root-run server (pre-compose `user:`) would fail the rename — logged, not fatal; none exist on this host (the compose file has run as the operator's uid since the multi-tenant cut).
- Existing setup containers keep the old bind until `EnsureSetup` next runs; the stale check is what makes the layout change take effect without an operator step.
- Everything that composes `HomesDir/<user>` by hand, per `grep -rn 'HomesDir' internal cmd` (run 2026-09-04): `internal/httpapi/server.go:126` (`files.Trust` — tenant root, stays), `cmd/styrman/main.go:362-379` (the janitor — tenant root then `p/`, stays), `internal/httpapi/credentials.go:262` (globs `HomesDir/<user>/p/*` for the project homes — stays, `p/` does not move), `internal/orchestrator/orchestrator.go:976` (a root-list check — stays). Phase 1 reruns the grep and lists any new hit in the changelog.
