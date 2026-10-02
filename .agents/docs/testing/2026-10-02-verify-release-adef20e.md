---
owner: verifier
date: 2026-10-02
---
# Verification — release-adef20e   (2026-10-02)

Range reviewed: v1.40.0..adef20e

## Status: PASS

Scope: complete (steps 1-8 as dispatched; guide.md exists, not modified)

## Commands
- 1. `git status --porcelain --untracked-files=all` → exit 0; empty (clean). `git status --short web/dist` before: empty.
- 2. `deploy/verify-commit.sh adef20e` → exit 0; 'OK: commit adef20e89ca9 builds and tests clean in isolation — safe to tag.' (output .agents/scratch/verify-release-adef20e/vc.txt)
- 3. `bash deploy/vulncheck.sh` → exit 3; only GO-2026-4883 and GO-2026-4887 appear (accepted state).
- 4. `go test ./internal/config/ -run 'TestStyrmanUpdate|TestStyrmanCtl' -count=1 -v` → exit 0; counts: pass 34, fail 0, skip 0 at adef20e for that command (13 TestStyrmanUpdateStatus* PASS); vs previous: none
- 4. `go test -race ./internal/orchestrator/ -run TestListeningPorts -count=1 -v` → exit 0; counts: pass 7, fail 0, skip 0 at adef20e; vs previous: none
- 5. `bash .agents/docs/testing/styrman/integration.sh styrman-runtime:test` → exit 0; passed=122 failed=0 skipped=3 aborted=0
- 6. `bash web/e2e/run.sh` (after Tier 2 finished; no concurrent suite) → exit 0; passed=99 failed=0 skipped=2. `git status --short web/dist` before and after: empty (rebuild changed nothing).
- 7. Added skips in v1.40.0..adef20e (excluding web/dist, .agents/docs): two `t.Skip`, each with reason on its line: "no bash on this host" (diff line 491), "runs the root refusal; meaningless as root" (diff line 899). No nolint/.only/@ts-ignore/oxlint-disable.
- 8. Final status: only untracked .agents/docs/reviews/2026-10-02-review-624808d-adef20e-{code,docs}.md and 2026-10-02-security-624808d-adef20e.md (other agents' reports) plus this log. No source/test/config path changed.

## Coverage of changed code
Not mapped file-by-file (release gate; suites above are the project's tiers).

## Not examined
Per-file coverage mapping; full `go test ./...` and web npm tests ran inside verify-commit.sh (not separately counted here); Tier 3 and GCP fresh-host path unexercised.

## Environment
Linux 6.12 amd64, Docker plain, Kata mvm host.
