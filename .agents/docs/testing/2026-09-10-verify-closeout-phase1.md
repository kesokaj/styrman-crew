---
owner: verifier
date: 2026-09-10
---
# Verification — closeout Phase 1 (2026-09-10)

Range reviewed: uncommitted working tree (HEAD 0378825) — Phase 1 of
`.agents/docs/plans/2026-09-10-todo-closeout.md`, Interfaces items C, D, 5,
7, E, F, G.

## Status: PASS

Scope: gofmt, go vet, go test -race (all packages), darwin/arm64 cross
build, numstat check, test-count deltas per changed test file, legacy
symbol grep, and per-item coverage trace — all ran and complete for Phase
1. Integration/e2e suites intentionally NOT run (per dispatch, deferred to
Phase 6 by the main thread). Vendored `deploy/caddy/ingress` module also
not run (out of scope for this dispatch's file list).

## Commands

- `gofmt -l internal/ cmd/` → no output (clean).
- `go vet ./...` → no output (clean).
- `go test ./... -count=1 -race` → all packages `ok`, no failures:
  `cmd/styrman`, `internal/config`, `internal/conversation`,
  `internal/credbroker`, `internal/credstore`, `internal/files`,
  `internal/gitrepo`, `internal/history`, `internal/httpapi`,
  `internal/identity`, `internal/links`, `internal/orchestrator`,
  `internal/project`, `internal/terminal`, `internal/userstore`.
  `cmd/wsprobe`, `web` — no test files (pre-existing, unrelated to this
  change).
- `GOOS=darwin GOARCH=arm64 go build ./...` → succeeds, no errors.
- `git diff --numstat` → all changed files show clean `+/-` integer
  columns, no `-` (binary) column on any path:
  ```
  12  0  .agents/docs/deploy/remote.md
  0  68  cmd/styrman/main.go
  0  88  cmd/styrman/migrate_test.go
  0  17  internal/config/config.go
  75  9  internal/httpapi/git.go
  29 12  internal/httpapi/network.go
  62  0  internal/httpapi/network_test.go
  25 11  internal/httpapi/setup.go
  96  4  internal/httpapi/setup_pubkey_test.go
  47 24  internal/orchestrator/orchestrator.go
  ```
  (`internal/httpapi/gitconfig_test.go` is new/untracked, not in `git diff`.)

## Test-count deltas (BUG-005 check)

- `cmd/styrman/migrate_test.go`: 6 → 4. Removed:
  `TestMigrateWorkspaces`, `TestEnsureReposRefusesUnmigrated` — expected,
  paired with deletion of `migrateWorkspaces`/`LegacyWorkspaceExists`
  (item F). Remaining 4 (`TestMigrateBaseHomes`,
  `TestMigrateLegacyHomeLandsInBase`,
  `TestMigrateBaseHomesLeavesCollisionsInPlace`,
  `TestMigrateLegacySkipsAHalfMigratedTenant`) present and pass.
- `internal/httpapi/network_test.go`: 19 → 21 (net add of 2, matches item
  C's two new tests).
- `internal/httpapi/setup_pubkey_test.go`: 1 → 3 (net add of 2, matches
  items 5/7).
- `internal/httpapi/gitconfig_test.go`: new file, 1 test
  (`TestGitUserFromConfig`, item G).
- `grep -rn 'LegacyWorkspaceExists\|migrateWorkspaces' cmd internal` →
  empty. No dangling references to the deleted code.

## Coverage trace per Interfaces item

- **C** (network reply after lock release / no-decision 500) →
  `TestNetworkReplyIsWrittenAfterTheLockIsReleased` (network_test.go:756),
  `TestNetworkNoDecisionIsA500` (network_test.go:783). Both present, pass.
- **D** (tool details echoes only a public key) →
  `TestToolDetailsEchoesOnlyAPublicKey` (setup_pubkey_test.go:75). Present,
  passes.
- **5/7** (public-key line formatting / comment sanitisation) →
  `TestPublicKeyLine` (setup_pubkey_test.go:15),
  `TestPublicKeyLineSanitisesTheComment` (setup_pubkey_test.go:52). Both
  present, pass.
- **G** (git user from config) → `TestGitUserFromConfig`
  (gitconfig_test.go:9). Present, passes.
- **E** (two-sample unmark + Warn) → NO unit test in this change. The plan
  itself (`2026-09-10-todo-closeout.md` line 324) explicitly defers E's
  unit test to Phase 5, gated on a Docker fake not yet built. This is a
  documented gap, not an omission — reported as such, not a FAIL, since
  the plan scopes it out of Phase 1.
- **F** (deletion of `migrateWorkspaces`/`LegacyWorkspaceExists`) →
  code and its two tests removed together (see test-count delta above);
  no leftover references; remaining 4 migrate tests still cover the
  surviving migration paths.

## Coverage of changed code

- `internal/httpapi/network.go` — covered by `network_test.go`
  (existing + 2 new tests).
- `internal/httpapi/setup.go` — covered by `setup_pubkey_test.go`
  (existing + 2 new tests).
- `internal/httpapi/git.go` — covered by new `gitconfig_test.go`
  (`TestGitUserFromConfig`) plus pre-existing git tests in the package
  (package-level `go test` passed).
- `internal/orchestrator/orchestrator.go` — item E (Docker interaction
  changes) has NO unit test yet; gap tracked above, matches plan's own
  Phase 5 deferral.
- `internal/config/config.go`, `cmd/styrman/main.go` — net deletions only
  (config/flag surface removed alongside item F); no new test surface to
  cover; `cmd/styrman` package tests still pass.
- `cmd/styrman/migrate_test.go` — 2 tests removed with the code they
  tested (item F); remaining 4 tests pass.
- `.agents/docs/deploy/remote.md` — docs-only change, no test applicable.

## Not examined

- `deploy/caddy/ingress` module tests (separate Go module per AGENTS.md;
  not in this dispatch's file list, not run).
- Integration suite (`.agents/docs/testing/styrman/integration.sh`) and
  browser e2e (`web/e2e/run.sh`) — explicitly excluded by the dispatch,
  deferred to Phase 6 by the main thread.
- Frontend checks (`web/` tsc/lint/vitest) — no `web/` files in this
  change set, not run.
- `govulncheck` (`deploy/vulncheck.sh`) — not requested, not run.

## Environment

Linux 6.12.107+deb13-amd64, repo at
`/home/kesokaj/workspace/styrman-crew`, branch `main`, HEAD `0378825`.
Go toolchain per `go.mod` (root module). `go test -race` used the host's
native race detector support (linux/amd64); darwin/arm64 check was
cross-compile only (`go build`, no test execution, no Kata microVM
needed).
