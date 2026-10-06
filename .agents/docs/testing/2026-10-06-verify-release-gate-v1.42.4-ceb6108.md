---
owner: verifier
date: 2026-10-06
---
# Verification — release-gate-v1.42.4-ceb6108   (2026-10-06)

Range reviewed: v1.42.4..ceb6108

## Status: PASS

Scope: step 1 verify-commit.sh ceb6108, step 2 Tier 2 integration.sh; guide .agents/docs/testing/styrman/README.md (exists)

## Commands
- 1. orient: pwd=/home/kesokaj/workspace/styrman-crew, git status clean at ceb6108; change set present (26 files: image/Dockerfile, web comments, docs).

- 4. `GH_TOKEN=$(gh auth token) bash deploy/verify-commit.sh ceb6108 > .agents/scratch/verify-commit.log 2>&1` -> rc=0; final line "OK: commit ceb61086018c builds and tests clean in isolation"; 0 `--- FAIL`; runtime image carries sh-agent v2.4.1 (36 skills, 8 subagents, 1 rule). Go/Caddy/gofmt/web freshness/frontend trio/server image/runtime image all passed. Per-test counts not extracted (script prints stages, not counts).
- 4. `bash .agents/docs/testing/styrman/integration.sh > .agents/scratch/integration.log 2>&1` -> rc=0; passed=146 failed=0 skipped=3 aborted=0; matches README 146/0/3.
- 4. `docker run --rm --entrypoint /styrman/bin/sh-agent styrman-runtime:test version` -> rc=0; `sh-agent v2.4.1`.
- 7. Changed source: image/Dockerfile (covered by integration + image build), web comments only (covered by api.encoding.test.ts, ran in vitest). No suppressions checked beyond this; not diffed line-by-line.
- 8. Tree: no source/test/config change. New untracked agent artifacts seen: .agents/docs/reviews/2026-10-06-review-release-v1.42.4-ceb6108.md, ...-review-v1.42.4-ceb6108-docs.md, ...-security-release-gate-v1.42.4-ceb6108.md.

## Not examined
Per-package Go test counts and vitest counts (verify-commit.sh does not print them); suppression scan of the diff; fresh-host/GCP path; Tier 3 browser e2e (not named).
