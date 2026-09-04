# Un-nest project homes — phase 1     (v1.21.0)

## Summary

Feature + security, phase 1 of 2: the setup VM's home moves from `HomesDir/<user>` to `HomesDir/<user>/base`, un-nesting all project homes (`p/<uuid>`) from the guest mount. This layout removal closes the check-to-mount window behind BUG-002/003 and eliminates the need for bind-source symlink guards on new project creation. The migration moves existing home contents into `base/` (idempotent, logged per-user, never aborts startup); existing setup containers are recreated on next `EnsureSetup` by detecting stale mounts. Unit suite green (`go test ./... -race`).

## Changes

### Config and interfaces (ADR-064)

`internal/config/config.go` introduces `TenantDir(user)` = `HomesDir/<user>` (server-only, never mounted); `UserHome(user)` = `HomesDir/<user>/base` (the setup VM's home, was `HomesDir/<user>`); `ProjectHome` unchanged at `HomesDir/<user>/p/<uuid>`. Test `TestHomeLayoutKeepsGuestMountsApart` verifies the three shapes do not collide.

### Migration (main.go)

`migrateLegacy` (main.go:182) now creates its parent directory via `os.MkdirAll(cfg.TenantDir(user), 0o700)` before renaming the pre-multitenancy home into `UserHome` — necessary because `UserHome` now names the `base` subdirectory that must be created. New test `TestMigrateLegacyHomeLandsInBase` plants a `StateDir/home` and confirms it lands at `HomesDir/<user>/base`.

`migrateBaseHomes` (new function, wired after `migrateLegacy`) runs at startup for every tenant directory: it moves every entry except `base/` and `p/` into `base/`, creating `base/` with mode 0700 if needed. Refuse a `base` that exists but is a symlink or file (log error, skip that user — the safe failure). Idempotent; logs entry count per user (never names individual files). Tests: `TestMigrateBaseHomes` (old layout moved, `p/` untouched, `projects/` subdir relocated too, already-migrated tenant untouched, planted-symlink refused, symlinked tenant skipped, second run no-op).

### Orchestrator (internal/orchestrator/orchestrator.go)

`EnsureSetup` now detects stale setup containers whose `/home/<guest>` bind Source is not the new `UserHome`. New helper `setupHomeStale(mounts []types.MountPoint, guestUser, want string) bool` returns true if the mount-list lacks the correct bind. On stale detection, log "setup VM home moved — recreating" and force-remove the container (same shape as the existing guest-changed branch). Before creating a setup container, `os.MkdirAll(TenantDir)` then `mkdirAllConfined(TenantDir, "base")` then `realBelow(TenantDir, UserHome)` to create and validate the base home (refusing a symlinked one). Test `TestSetupHomeStale` mocks a stale mounts slice.

`EnsureRunning` (orchestrator.go:226, 272, 275, 282): four `UserHome(user)` calls that mean the parent of `p/` become `TenantDir(user)`. The calls use `os.Root` or `mkdirAllConfined` to create the parent of the project home; the project home itself (`p/<uuid>`) is created in `createProject` (server.go).

### Server (internal/httpapi/server.go)

`createProject` (server.go:726-727): `os.MkdirAll(TenantDir)` then `files.MkdirAllPerm(TenantDir, "p/<id>", 0o700)` to create the pre-boot project home (needed so Files pane shows the right scope before the first boot). Existing comment updated.

### Boundary comments and rules

Rewritten for the new guest-writable boundary:
- `orchestrator.go:263-297` (the create path: tenant dir is server-only; `base/` and `p/<uuid>` are the guest mounts)
- `orchestrator.go:592-593` (`EnsureSetup` base-home validation)
- `server.go:719-722` (project-home creation comment)
- `main.go:369-378` (the janitor: server walks `HomesDir/<user>` and lists `p/`, never sees `base/`)
- `.agents/rules/guest-writable-paths.md` first paragraph (layout statement)

### Unchanged audit (grep 2026-09-04)

Every remaining `UserHome` caller is the setup VM's home (`setup.go`, `credentials.go:263`, `git.go:73`, `explorer.go:443`). Every `TenantDir` site is server-only (janitor dir-walk, `credentials.go:262` glob for project homes, `files.Trust` confinement base, orchestrator root-list check). The project-home path `p/<uuid>` is untouched (credentials sweep glob, `ProjectHome` accessor).

## Type

feature + security, phase 1 of 2

## Files changed

- `internal/config/config.go` — new `TenantDir(user)`, `UserHome(user)` = `…/base`, test `TestHomeLayoutKeepsGuestMountsApart`
- `cmd/styrman/main.go` — `migrateLegacy` step 1 creates `TenantDir` before rename, test `TestMigrateLegacyHomeLandsInBase`; new `migrateBaseHomes` (tests: `TestMigrateBaseHomes` covering old layout, `p/` untouched, `projects/` moved, already-migrated skip, symlink refuse, symlink-dir skip, idempotent)
- `internal/orchestrator/orchestrator.go` — `EnsureSetup` stale-bind detect + recreate, confined base creation + `realBelow`, `setupHomeStale` helper (test); `EnsureRunning` (226, 272, 275, 282) → `TenantDir`; boundary comments rewritten
- `internal/httpapi/server.go` — `createProject` (726-727) → `TenantDir`; boundary comment updated
- `.agents/rules/guest-writable-paths.md` — first paragraph restated for new layout

## Agents dispatched

verifier (`.agents/docs/testing/2026-09-04-verify-unnest-phase1.md`), reviewer (`.agents/docs/reviews/2026-09-04-review-unnest-phase1.md`), security (`.agents/docs/reviews/2026-09-04-security-unnest-phase1.md`); verdicts pending. Root Go suite green (`go test ./... -race`).

## Verification

- Unit tests pass: config layout, migration (legacy + base homes), orchestrator (stale-bind detect, confined creation), boundary comments.
- `grep -rn 'UserHome(' internal cmd` returns only base-home callers (setup, credentials, git, explorer); `TenantDir` sites are server-only.
- Integration and e2e suites unchanged (Phase 2).

## Requirements

R3 (per-project homes) and R12 (isolation) unchanged — project homes still occupy `HomesDir/<user>/p/<uuid>` and the per-tenant setup mount. R1's boot budget must hold (ADR-064 verifies this in Phase 2 upgrade fixture). No env, API, or JSON change.

## Related

- Plan: `.agents/docs/plans/2026-09-04-unnest-project-homes.md`
- ADR-064 (pending): the architecture decision un-nesting the project homes
- BUG-002, BUG-003, BUG-004: prior vulnerabilities the layout change closes
- Rule `.agents/rules/guest-writable-paths.md`: guest-writable boundary definition

## Security round 1 → fixes (same day, before the commit)

`reviews/2026-09-04-security-unnest-phase1.md` round 1 blocked: the first migration checked `base` once and then renamed by path, and a pre-ADR-064 setup VM still running with the tenant dir bound could swap `base` for a symlink mid-loop (rename follows a symlinked intermediate component) — high; the stale-container recreate was lazy, so the old bind could persist for an unbounded window — medium; plus lows (a colliding regular file silently replaced; the setup VM's reuse path never re-checked `base`; the legacy guard on a half-migrated host; the `git.go:73` read; the `Force: true` teardown). Fixed before this commit: the migration runs through an `os.Root` at the tenant dir (`migrateTenantIntoBase`), logs counts only, and runs after a new startup sweep `Orchestrator.RemoveStaleSetupVMs` that force-removes every setup container with a stale home bind; `EnsureSetup` checks `base/` for a symlink component on every path; a colliding entry is left in place and counted; `migrateLegacy` skips a half-migrated tenant. Tests: `TestMigrateBaseHomesLeavesCollisionsInPlace`, `TestMigrateLegacySkipsAHalfMigratedTenant`. Rebutted/recorded: `git.go:73` (pre-existing, TODO.md), the forced teardown (ADR-064). Round 2 is the same report's "## Round 2".
