---
owner: verifier
date: 2026-09-04
---
# Verification — unnest-project-homes Phase 1   (2026-09-04)

Range reviewed: working tree (uncommitted) vs HEAD (0da855e)

## Status: PASS

Scope: Phase 1 only (config/migration/orchestrator/httpapi wiring + static
audit); integration.sh and e2e explicitly excluded per dispatch (Phase 2).

## Commands

- `gofmt -l internal/ cmd/` → empty (clean).
- `go vet ./...` → no output, exit 0.
- `go test ./... -race -count=1` → all packages `ok`, including
  `cmd/styrman`, `internal/config`, `internal/orchestrator`,
  `internal/httpapi`. No failures.
- `go test ./internal/config/ ./cmd/styrman/ ./internal/orchestrator/ -run 'HomeLayout|Migrate|SetupHomeStale|RealBelow|ProjectHomeIsReal' -v -count=1`:
  - `TestHomeLayoutKeepsGuestMountsApart` — PASS (internal/config)
  - `TestMigrateBaseHomes` — PASS (cmd/styrman)
  - `TestMigrateLegacyHomeLandsInBase` — PASS (cmd/styrman)
  - `TestRealBelowRefusesSymlinkComponents` — PASS (internal/orchestrator)
  - `TestProjectHomeIsRealAllowsMissingRefusesLinks` — PASS (internal/orchestrator)
  - `TestSetupHomeStale` — PASS (internal/orchestrator)

## Bite proof (stash / test / pop, always confirmed restored)

- (a) `git stash push -- internal/config/config.go` →
  `go test ./internal/config/ -run HomeLayout` →
  `internal/config/config_test.go:182:14: c.TenantDir undefined (type *Config has no field or method TenantDir)` — build failed, as required.
  `git stash pop` → `git diff --stat -- internal/config/config.go` showed
  `12 ++++++++++--` (full diff restored).
- (b) `git stash push -- cmd/styrman/main.go` →
  `go test ./cmd/styrman/ -run Migrate` →
  `cmd/styrman/migrate_test.go:70:2: undefined: migrateBaseHomes` /
  `:102:2: undefined: migrateBaseHomes` — build failed, as required.
  `git stash pop` → `git diff --stat -- cmd/styrman/main.go` showed
  `69 +++++++++++++++++++++++++++++++++++++++++++++++++----` (full diff restored).

## Static audit

`grep -rn 'UserHome(' internal cmd | grep -v _test` — every hit is the
setup-VM base home, unchanged callers, all correct:
- `orchestrator.go:587,588,612,620` (EnsureSetup: stale check + realBelow
  target + homePath) — base home, correct.
- `httpapi/explorer.go:443`, `httpapi/credentials.go:263`,
  `httpapi/setup.go:34,127,214`, `httpapi/git.go:73` — setup-VM base home,
  correct.
- `main.go:183,232,459` (migrateBaseHomes / legacy workspace lookup) —
  base home, correct.
- `config.go:357,372` — definition of `UserHome` and `ProjectHome`'s
  internal use of it (base/projects/... convenience path), correct.
No hit found where the tenant dir was meant but `UserHome` was used instead.

`grep -rn 'HomesDir' internal cmd | grep -v _test` — every hit builds the
tenant dir (`HomesDir/<user>`) or `p/<uuid>` under it, never `base/`
directly: `server.go:126` (trust root), `orchestrator.go:989-994` (root
scan), `main.go:37,147,366,384,435` (dir listing / OpenRoot), `config.go`
definitions, `credentials.go:262` (`HomesDir/<user>/p/*`). None reach into
`base/` without going through `TenantDir`/`UserHome`.

`grep -n 'TenantDir(' internal/orchestrator/orchestrator.go
internal/httpapi/server.go` confirms the required call sites use
`TenantDir`: `EnsureRunning` (lines 226, 272, 275, 282), `EnsureSetup`'s
confined base creation (606, 609, 612), and `server.go createProject`
(727, 728).

## Coverage of changed code

- `internal/config/config.go` accessors (`TenantDir`, `UserHome`,
  `ProjectHome`) — `TestHomeLayoutKeepsGuestMountsApart`
  (internal/config/config_test.go).
- `cmd/styrman/main.go` `migrateLegacy`'s new `MkdirAll` (tenant dir) —
  `TestMigrateLegacyHomeLandsInBase`.
- `cmd/styrman/main.go` `migrateBaseHomes` — `TestMigrateBaseHomes`
  exercises move (alice), skip-already-migrated (bob), refuse-planted-
  base-symlink (carol), skip-symlinked-tenant-entry (dave), and a second
  invocation for idempotency, all in one test.
- `internal/orchestrator/orchestrator.go` `setupHomeStale` —
  `TestSetupHomeStale`.
- `internal/orchestrator/orchestrator.go` `realBelow` / confined `base`
  creation — `TestRealBelowRefusesSymlinkComponents`,
  `TestProjectHomeIsRealAllowsMissingRefusesLinks`
  (internal/orchestrator/realbelow_test.go).
- `internal/orchestrator/orchestrator.go` `EnsureSetup`'s stale-bind
  recreate path and `EnsureRunning`'s `TenantDir` call sites, and
  `internal/httpapi/server.go createProject`'s `TenantDir` use — **no unit
  test**: there is no fake Docker client in this repo to drive
  `EnsureSetup`/`EnsureRunning` end-to-end. This wiring is covered instead
  by Phase 2's fixture (integration.sh / e2e), explicitly out of scope for
  this Phase 1 verification run.
- `.agents/rules/guest-writable-paths.md` — documentation-only change, no
  test applicable.

## Environment

OS: Linux 6.12.107+deb13-amd64. Go: `go version` not captured verbatim in
this run but matches the repo's pinned toolchain used by `go build`/`go
test` above, both of which succeeded. Docker/Colima: not exercised (no
Docker-backed test was run in this scope). Vendored Caddy ingress module
(`deploy/caddy/ingress`) not touched by this change set and not run here —
out of scope, root `go test ./...` does not cover it per AGENTS.md, and no
files under that module appear in the diff.
