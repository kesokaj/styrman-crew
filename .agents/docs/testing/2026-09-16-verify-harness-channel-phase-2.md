---
owner: verifier
date: 2026-09-16
---
# Verification — harness-channel-phase-2   (2026-09-16)

Range reviewed: 518c5c0..worktree

## Status: PASS

Scope: working-tree diff since 518c5c0 — internal/config/config.go,
internal/orchestrator/orchestrator.go, deploy/docker-compose.yml (changed);
internal/config/harnessdir_test.go, internal/orchestrator/harness.go,
internal/orchestrator/harness_test.go (new). Plan Phase 2 task boxes and
Checkpoint (`.agents/docs/plans/2026-09-16-harness-channel.md`) reviewed
against; all six required steps ran to completion.

## Commands

1. `go build ./...` → exit 0.
   `go vet ./...` → exit 0.
   `gofmt -l internal/ cmd/` → printed nothing, exit 0.

2. `go test ./internal/config/ -run HarnessDir -count=1 -v` → exit 0.
   ```
   === RUN   TestHarnessDirFromEnv
   --- PASS: TestHarnessDirFromEnv (0.00s)
   PASS
   ok  	github.com/kesokaj/styrman-crew/internal/config	0.003s
   ```
   `go test ./internal/orchestrator/ -run 'Harness|CreateBinds' -count=1 -v` → exit 0.
   ```
   --- PASS: TestHarnessBindsNameTheUnresolvedCurrentPath (0.00s)
   --- PASS: TestHarnessBindsAbsentChannelIsNotExist (0.00s)
   --- PASS: TestHarnessBindsDisabledIsNilNil (0.00s)
   --- PASS: TestHarnessBindsRefuseAnUntrustedChannel (0.00s)
       --- PASS: TestHarnessBindsRefuseAnUntrustedChannel/current_is_a_real_directory (0.00s)
       --- PASS: TestHarnessBindsRefuseAnUntrustedChannel/no_sh-agent_binary (0.00s)
       --- PASS: TestHarnessBindsRefuseAnUntrustedChannel/no_COMMIT (0.00s)
       --- PASS: TestHarnessBindsRefuseAnUntrustedChannel/content_dir_is_a_symlink (0.00s)
       --- PASS: TestHarnessBindsRefuseAnUntrustedChannel/world-writable_export (0.00s)
       --- PASS: TestHarnessBindsRefuseAnUntrustedChannel/world-writable_content_dir (0.00s)
       --- PASS: TestHarnessBindsRefuseAnUntrustedChannel/current_escapes_the_content_dir (0.00s)
   --- PASS: TestHarnessChannelStateReadsTagAndCommit (0.00s)
   --- PASS: TestEnsureRunningRecreatesAStoppedContainerToAddTheHarnessBinds (0.00s)
   --- PASS: TestEnsureRunningRecreatesAStoppedContainerToDropTheHarnessBindsWhenTheChannelIsAbsent (0.00s)
   --- PASS: TestEnsureRunningLeavesARunningContainerWithoutTheHarnessBinds (0.00s)
   --- PASS: TestCreateBindsTheChannelForProjectsNotForTheSetupVM (0.00s)
   PASS
   ok  	github.com/kesokaj/styrman-crew/internal/orchestrator	0.005s
   ```
   Note: this run matched `-run 'Harness|CreateBinds'` and also picked up
   `TestEnsureRunningLeavesAStoppedContainerWhenTheChannelIsUntrusted`
   implicitly (name contains no "Harness"/"CreateBinds" substring — it did
   NOT run in this invocation; it did run under the full-package `-race`
   pass below, where it appears as part of `ok internal/orchestrator`, PASS
   implied by no FAIL line). All 8 top-level tests + 7 subtests named by the
   `-run` filter show `--- PASS`; zero `--- FAIL`, zero `--- SKIP`.
   counts: pass 15 (8 top-level + 7 subtests), fail 0, skip 0 at 518c5c0+dirty
   for `go test ./internal/orchestrator/ -run 'Harness|CreateBinds' -count=1 -v`.
   vs previous: none (new test file, no earlier counts line for this command).

3. `go test -race ./... -count=1` (run in background, log at
   `.agents/scratch/verify-harness-channel-phase-2-race.log`) → all packages
   `ok`, zero `FAIL` lines (checked `grep -i "^FAIL"`, no hits), including
   `internal/config` (14.765s) and `internal/orchestrator` (11.115s).
   `(cd deploy/caddy/ingress && go test ./... -count=1)` → exit 0,
   `ok github.com/spgo/devbox/caddy/ingress 2.017s`.

4. `STYRMAN_VERSION=0.0.0 STYRMAN_UID=1000 STYRMAN_GID=1000 DOCKER_GID=999 docker compose -f deploy/docker-compose.yml config` → exit 0. Rendered
   styrman service env line 77: `STYRMAN_HARNESS_DIR: /var/lib/styrman/harness`.

## Coverage of changed code

- `internal/config/config.go` (HarnessDir from `STYRMAN_HARNESS_DIR`) —
  covered by `internal/config/harnessdir_test.go` (`TestHarnessDirFromEnv`).
- `internal/orchestrator/orchestrator.go` (recreate-for-harness block,
  create-spec `harnessBinds` field, EnsureSetup no-binds) — covered by
  `internal/orchestrator/harness_test.go`
  (`TestEnsureRunningRecreatesAStoppedContainerToAddTheHarnessBinds`,
  `TestEnsureRunningRecreatesAStoppedContainerToDropTheHarnessBindsWhenTheChannelIsAbsent`,
  `TestEnsureRunningLeavesAStoppedContainerWhenTheChannelIsUntrusted`,
  `TestEnsureRunningLeavesARunningContainerWithoutTheHarnessBinds`,
  `TestCreateBindsTheChannelForProjectsNotForTheSetupVM`).
- `internal/orchestrator/harness.go` (new) — covered by
  `internal/orchestrator/harness_test.go`
  (`TestHarnessBindsNameTheUnresolvedCurrentPath`,
  `TestHarnessBindsAbsentChannelIsNotExist`, `TestHarnessBindsDisabledIsNilNil`,
  `TestHarnessBindsRefuseAnUntrustedChannel` with 7 subtests,
  `TestHarnessChannelStateReadsTagAndCommit`); `logHarnessChannel` is NOT
  directly unit-tested (no test found calling it) — it is a thin logging
  wrapper around `harnessChannelState`, which is covered.
- `deploy/docker-compose.yml` (new `STYRMAN_HARNESS_DIR` env line) — no
  Go test tier applies; verified by `docker compose … config` in step 4
  above (rendered output, not a test).

No suppressions (`//nolint`, `//lint:ignore`, `t.Skip`, `.only(`, etc.) found
in the changed/new files (grepped the diff manually while reading).

## Positive control (mechanism is new, not pre-existing)

- `git show 518c5c0:internal/orchestrator/orchestrator.go | grep -c harnessBinds` → `0`.
- `git show 518c5c0:internal/config/config.go | grep -c HarnessDir` → `0`.
  Confirms `harnessBinds`/`HarnessDir` do not exist at 518c5c0; the whole
  mechanism under test is new in this working tree.
- `internal/orchestrator/harness_test.go` itself carries no in-file
  red/green comment. The red-before/green-after claim (7 FAIL before the
  code, green after) is recorded instead in the PLAN
  (`.agents/docs/plans/2026-09-16-harness-channel.md`, Phase 2, second task
  box: "red before the code (7 FAIL), green after"). Dispatch asked to read
  that comment IN the test file; it is not there — noted as a documentation
  location mismatch, not a code defect (the plan's line is credible and the
  positive control above independently establishes the mechanism's novelty).

## Read-only checks (orchestrator.go, harness.go)

- **Recreate rule fires only for a STOPPED container**: confirmed.
  `internal/orchestrator/orchestrator.go:352` —
  `if err == nil && (insp.State == nil || !insp.State.Running) {` gates the
  entire harness-recreate block (lines 337–368); a running container is
  left untouched (comment at 344–347 and test
  `TestEnsureRunningLeavesARunningContainerWithoutTheHarnessBinds` agree).
- **Create path passes the same binds**: confirmed.
  `internal/orchestrator/orchestrator.go:348` computes `hbinds` once, and
  the project create spec at line 469 sets `harnessBinds: hbinds` — the
  identical value the recreate-decision logic (352–368) used, so a
  recreate and a fresh create carry the same binds. `createSpec.harnessBinds`
  field declared at `internal/orchestrator/orchestrator.go:946`; consumed
  in the bind-list builder at line 1111 (`return append(b, spec.harnessBinds...)`)
  inside `create` (func at line 1020).
- **EnsureSetup's create passes no harnessBinds**: confirmed.
  `internal/orchestrator/orchestrator.go:795` — the setup VM's
  `o.create(ctx, createSpec{...})` call (function starts at 733, this
  create call at 795) has no `harnessBinds:` key in its literal, so the
  field defaults to Go's zero value (`nil` slice) — no harness binds ever
  reach the setup VM. Matches test
  `TestCreateBindsTheChannelForProjectsNotForTheSetupVM`.

## Not examined

- Phase 3 (entrypoint / integration tier) — out of scope per dispatch, not
  run.
- `web/e2e/run.sh` and `.agents/docs/testing/styrman/integration.sh` — not
  run; dispatch scoped this verification to build/vet/gofmt, the two named
  unit-test invocations, `go test -race ./...` (whole module + caddy
  ingress module), and `docker compose config`, none of which include the
  Tier 2 integration or browser e2e suites.
- `logHarnessChannel`'s actual log output (server-start INFO/WARN line) was
  read in source but not exercised by a running server in this pass — no
  test found that captures its output directly; it is a thin wrapper over
  already-tested `harnessChannelState`.
- The plan's Checkpoint box (`- [ ] Checkpoint`) is unticked in the working
  tree; ticking it is not a verifier action.

## Environment

Linux 6.12.107+deb13-amd64. `go` toolchain per go.mod (not separately
queried — build/vet/test all succeeded). Docker Compose CLI used only for
`config` (render), no containers built or run. No Colima (Linux host).
