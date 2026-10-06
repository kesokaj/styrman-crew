---
owner: verifier
date: 2026-10-06
---
# Verification — harness-v2.4.1   (2026-10-06)

Range reviewed: 927eb72..working tree (uncommitted; image/Dockerfile + docs)

## Status: PASS

Scope: Tier 2, build/vet/go test run; Tier 3 controls read only (not re-run); Caddy module, web, vulncheck, e2e not run (not requested; no Go/web source changed). Guide: .agents/docs/testing/styrman/README.md (project's testing guide).

## Commands
- 1. `pwd` -> /home/kesokaj/workspace/styrman-crew; `git status --short --untracked-files=all` -> 7 modified: DECISIONS.md, specs/deploy.md, specs/runtime-image.md, testing/styrman/README.md, README.md, image/Dockerfile, image/README.md (no Go/web source changed)
- 4. Tier 2: `bash .agents/docs/testing/styrman/integration.sh > .agents/scratch/verify-harness-v241/integration.txt 2>&1` -> rc=0. Log head: "ensure-test-image: rebuilding styrman-runtime:test: image/ has files newer (2026-10-06T07:48:56) than styrman-runtime:test (2026-10-05T16:01:51)" then "rebuilt" (image now created 2026-10-06T07:49:45, from the working-tree Dockerfile via the script's GH token secret). Summary line: `passed=146 failed=0 skipped=3 aborted=0`.
  - counts: pass 146, fail 0, skip 3 at 927eb72+dirty for integration.sh; vs README count line (`passed=146 failed=0 skipped=3`, 149 checks): identical, no difference. Skips are the 3 documented firewall auto-SKIPs (pretty URL proxy, legacy proxy URL, public link).
  - Image under test: `docker run --rm --entrypoint /styrman/bin/sh-agent styrman-runtime:test version` -> `sh-agent v2.4.1`.
- 4. `go build ./...` -> rc=0
- 4. `go vet ./...` -> rc=0
- 4. `go test ./...` > gotest.txt -> rc=0; 17 packages `ok`, no FAIL lines (not run with -v; per-test counts not captured)
- Tier 3 (read, not re-run), .agents/scratch/:
  - harness-build.out: `PASS-BUILD rc=0`, image `sh-agent v2.4.1`, list shows Skills (36), Subagents (8), Rules (1). Build of the real Dockerfile passes.
  - harness-build.sh: must-fail control passes `--build-arg HARNESS_SHA256=000...0`; harness-build.out: `MUSTFAIL-BUILD rc=1`.
  - harness-build-fail.log: rc=1 at stage-0 step 8/14 (the harness RUN), `ERROR: ... exit code: 1`; echoed step shows the zero-hash version of `grep -qxF "<hash>  sh-agent-linux-amd64" SHA256SUMS` as first failing candidate (the log does not name which command exited 1; the grep and sha256sum steps both fail with the zero hash).
  - harness-build-oldver.log: same RUN step, rc=1, with version `v2.4.0` and a different hash (3031e27e...) in the echoed step; fails at the same step (the log does not say which assertion, e.g. the SHA256SUMS header or the version test, tripped).
  - Limit: control logs show the step failed, not which assertion; the author's script for oldver is not in scratch (only harness-build.sh for the zero-hash one).
- 7. Coverage: no Go/web source changed; only image/Dockerfile (covered by Tier 2 integration build + harness checks) and docs. No suppressions added (diff has none in code).
- 8. Tree: source/test/config unchanged since step 1 (same 7 modified paths). New untracked, other agents' artifacts, listed as seen: .agents/docs/reviews/2026-10-06-review-harness-sha256-pin.md, .agents/docs/reviews/2026-10-06-security-harness-sha256-pin.md; plus this log.

## Coverage of changed code
- image/Dockerfile (harness RUN step) - exercised by the Tier 2 image rebuild and the harness checks; must-fail controls read (Tier 3).
- docs - no tests applicable.

## Not examined
Caddy module tests, web tests, vulncheck, Tier 4 e2e, a fresh re-run of the Tier 3 controls, the content of the review/security artifacts, the docs prose.

## Environment
Linux 6.12.111 (amd64), Docker native, Kata runtime mvm; Go per go.mod.
