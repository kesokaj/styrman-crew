---
owner: verifier
date: 2026-10-02
---
# Verification — tool-updates-freeze-harness-v2-phase-1   (2026-10-02)

Range reviewed: 1fe28a0..working tree (uncommitted, 22 files modified + untracked)

## Status: PASS

Scope: complete for the dispatched tiers (1, server image build, 2); Tier 4, vulncheck, verify-commit not run (out of scope). Guide testing/guide.md exists.

## Commands
- 1. `go build ./...` → exit 0
- 2. `go vet ./...` → exit 0
- 3. `go test -v -count=1 ./...` → exit 0; counts: pass 392, fail 0, skip 1 at 1fe28a0+dirty; vs previous: none
- 4. `(cd deploy/caddy/ingress && go test -v -count=1 ./...)` → exit 0; pass 60, fail 0, skip 0
- 5. `(cd web && npm run check)` → exit 0
- 6. `(cd web && npm run lint)` → exit 0 (oxlint)
- 7. `(cd web && npm test)` → exit 0; 15 files, 178 tests passed
- 8. `docker build -f deploy/Dockerfile.server -t styrman-server:verify-phase1 ...` → exit 0; `docker rmi` exit 0
- 9. `bash .agents/docs/testing/styrman/integration.sh styrman-runtime:test` → exit 0; `passed=118 failed=0 skipped=3 aborted=0` (3 SKIPs = host-firewall previews). Boot line: `PASS boot to ready within R1's 15 s hard limit (measured 3.23s on ext2/ext3; soft target 5 s)`. Also PASS: agy dir writable/resolves; ready line precedes harness deployed line; ping answered with pong. Image rebuild ran via ensure-test-image.sh.

## Test count
`go test ./... -list '.*'` Test names: 334 (HEAD 330 per dispatch). `git diff HEAD -U0 -- '*_test.go'` shows no removed `func Test` line; test files diff: 24 insertions, 5 deletions (edits inside gcp_scripts_test.go, styrman_ctl_test.go). New untracked test files: entrypoint_seed_order_test.go, styrman_ctl_optional_fill_test.go, control_test.go.

## Not examined
Tier 4 e2e, vulncheck, verify-commit not in scope. Skip 1 in go test not compared to a previous log (none). wsprobe -control has no unit test (covered only via Tier 2 ping check). Step 8 tree check: git status identical to step 1 apart from this log.
