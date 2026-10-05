---
owner: verifier
date: 2026-10-05
---
# Verification — project-resources-phase-2   (2026-10-05)

Range reviewed: 58ddcd3..working tree (uncommitted)

## Status: PASS

Scope: dispatch's steps 1-8 ran; Tier 2 (integration.sh) and e2e deliberately not run, so the entrypoint disk grow and real Kata sizing are unexercised. Not `complete`: guide not re-read/updated this run (existing testing/styrman/README.md used as is).

## Commands
- 1. `go build ./...` → exit 0
- 2. `go vet ./...` → exit 0
- 3. `go test -race -count=1 ./...` → exit 0; 16 packages ok, 0 FAIL (output .agents/scratch/verify-project-resources-phase-2/go-test-race.txt)
- 4. `go test -count=1 -v ./internal/... ./cmd/...` → exit 0; counts: top-level pass 401, fail 0, skip 1 (TestHelperListener) at 58ddcd3+dirty; (474 incl. subtests); vs previous: HEAD worktree same command, exit 0, top-level pass 366, fail 0, skip 1 (+35 passes, explained: new resources/removed_env/store_resources tests); detached worktree removed (worktree remove exit 0; the pre-existing wt-next worktree is not mine)
- 5. `(cd deploy/caddy/ingress && go test -count=1 ./...)` → exit 0; ok github.com/spgo/devbox/caddy/ingress
- 6. `bash -n image/entrypoint.sh` → exit 0; `shellcheck -S warning image/entrypoint.sh` → exit 0 (no output)
- 7. `gofmt -l internal cmd` → exit 0, no output
- 8. `grep -rn 'orchestrator.RunOpts{' internal/ cmd/ --include='*.go' | grep -v _test` → exactly one hit: internal/httpapi/server.go:109, inside runOptsFor
- 7. Suppressions added in diff (nolint, t.Skip, .only etc.): none.

## Coverage of changed code
- internal/orchestrator/resources.go - resources_test.go, resources_start_test.go
- orchestrator.go, docker.go - those tests plus adapted fakedocker_test.go, seam_test.go
- internal/config/config.go - removed_env_test.go
- internal/project/store.go - store_resources_test.go
- internal/httpapi/resources.go, server.go, network.go - resources_test.go
- image/entrypoint.sh - NOT covered by any unit test (syntax/shellcheck only); disk grow covered only by Tier 2 (Phase 4), not run. Reported as a gap, not a FAIL: shell, no unit tier.

## Not examined
Tier 2 integration.sh and web e2e; real Kata sizing and the entrypoint's disk grow; vulncheck, frontend checks; per-test mapping beyond file-level.

## Environment
Linux 6.12 amd64, bash; Go per go.mod toolchain; no Docker/Colima use. Tree check (step 8): only new path vs start is this log.
