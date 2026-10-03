---
owner: verifier
date: 2026-10-03
---
# Verification — release-ad5dbf6   (2026-10-03)

Range reviewed: v1.41.0..ad5dbf6

## Status: PASS

Scope: complete for the dispatched steps 1-8 (guide .agents/docs/testing/guide.md exists). Outputs under .agents/scratch/verify-release-ad5dbf6/.

## Commands
- 1. `git status --porcelain --untracked-files=all` at start -> empty (rc=0).
- 2. `deploy/verify-commit.sh ad5dbf6` -> rc=0; "OK: commit ad5dbf624947 builds and tests clean in isolation — safe to tag." Runtime image carries sh-agent v2.3.2. No Docker Hub 429.
- 3. `bash deploy/vulncheck.sh` -> rc=3; only GO-2026-4883 and GO-2026-4887 appear (accepted state).
- 4a. `go test ./internal/config/ -run 'TestStyrmanUpdate|TestStyrmanCtl' -count=1 -v` -> rc=0; counts: pass 36 (incl. subtests), fail 0, skip 0 at ad5dbf6; 15 `TestStyrmanUpdateStatus*` PASS, none SKIP. vs previous: none.
- 4b. `go test -race ./internal/orchestrator/ -run TestListeningPorts -count=3 -v` -> rc=0; pass 24 over 3 runs (8 per run), fail 0, skip 0; ok 8.3s.
- 5. `bash .agents/docs/testing/styrman/integration.sh styrman-runtime:test` -> rc=0; `passed=122 failed=0 skipped=3 aborted=0` (as expected).
- 6. `bash web/e2e/run.sh` (run after Tier 2 finished; no integration/e2e process running beforehand) -> rc=0; `passed=99 failed=0 skipped=2` (as expected). `git status --short web/dist` empty before and after.
- 7. Suppression scan of `git diff v1.41.0..ad5dbf6 -- . ':!.agents/docs'` (350 lines): one added skip, internal/config/styrman_update_status_test.go (diff line 112) `t.Skip("no en_US.UTF-8 locale on this host to run under")`, guarded by a `locale -a` check with the reason on the line; it did not skip on this host. Acceptable. No nolint, .skip(, .only(, @ts-ignore, oxlint-disable added.
- 8. End `git status --porcelain --untracked-files=all` -> only untracked: .agents/docs/reviews/2026-10-03-review-v1.41.0-ad5dbf6-code.md, .agents/docs/reviews/2026-10-03-security-v1.41.0-ad5dbf6.md (other agents' reports) and this log. No source/test/config path changed.

## Coverage of changed code
- deploy/update/styrman-update — internal/config/styrman_update_status_test.go
- internal/orchestrator/ports.go, orchestrator.go — internal/orchestrator/ports_lifecycle_test.go (+ existing TestListeningPorts)
- README.md, ROADMAP.md — docs

## Not examined
Fresh-host path (deploy/gcp) not exercised; full `go test ./...` beyond what verify-commit.sh ran was not repeated separately; govulncheck's two accepted findings not assessed beyond ID match.

## Environment
Linux 6.12.111 (Debian 13), Kata runtime host (NUC), Docker plain.
