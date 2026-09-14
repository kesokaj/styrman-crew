---
owner: verifier
date: 2026-09-14
---
# Verification — harness mirror refresh to 8046e4d, round 2 (2026-09-14)

Range reviewed: v1.30.1..worktree (round 2)

Round 1 log: `.agents/docs/testing/2026-09-14-verify-harness-refresh-8046e4d.md` (read, not amended). This
round re-verifies the four places the tree moved since round 1: `image/Dockerfile`'s new
`test ! -e /styrman/harness/.git` assertion, `.agents/docs/testing/styrman/integration.sh`'s
reworked harness-commit check (~lines 118-132), `image/harness/VENDORED.md` (§ Refresh paragraph),
and the regenerated `image/harness/SHA256SUMS`.

## Status: FAIL

Scope: all four dispatch steps ran to completion. Steps 1-3 and the container checks in step 4
PASS; the step 4 "prove the Dockerfile assertion can fail" check FAILS — the build did not fail as
required, root cause below.

## Commands

### Step 1 — mirror integrity

- `(cd image/harness && sha256sum -c --quiet SHA256SUMS)` → rc=0, no output.
- File-list comparison, exactly as `deploy/verify-commit.sh` does it (`diff <(find . -type f
  ! -name SHA256SUMS | sed 's#^\./##' | LC_ALL=C sort) <(cut -c67- SHA256SUMS)`) → rc=0, empty diff.
- `find image/harness -type l` → rc=0, empty (no symlinks).
- `diff -rq --exclude .git --exclude .agents --exclude .gitignore ~/workspace/styrman-harness
  image/harness` → rc=1, exactly three lines: `Only in image/harness: COMMIT`,
  `Only in image/harness: SHA256SUMS`, `Only in image/harness: VENDORED.md`.
- `test ! -e image/harness/.git` → rc=0 (no `.git` present in the vendored tree).

Step 1: PASS.

### Step 2 — integration.sh syntax/lint/binary check

- `bash -n .agents/docs/testing/styrman/integration.sh` → rc=0.
- `shellcheck -S warning .agents/docs/testing/styrman/integration.sh` → rc=0, no warnings.
- `git diff --numstat` filtered for binary rows (`awk '$1=="-" && $2=="-"'`) → no output, no binary
  rows.

Step 2: PASS.

### Step 3 — Tier 2 integration

- Port 18080 confirmed free before run: `ss -ltn | grep 18080` → no match (rc=1).
- `docker image inspect styrman-runtime:test --format '{{.Created}}'` → `2026-09-14T12:17:33+02:00`,
  matching the dispatch's stated rebuild time — image is current for this tree.
- `bash .agents/docs/testing/styrman/integration.sh styrman-runtime:test` → rc=0. Final line:
  `passed=94 failed=0 skipped=3 aborted=0` — matches expected exactly. Log:
  `.agents/scratch/step3-r2-integration.log`.
- The seven harness-section lines (`=== agent harness (ADR-062...) ===`), all PASS:
  ```
  PASS sh-agent reports no drift in the project home
  PASS CLAUDE.md is the two-line import stub
  PASS 27 skills and 7 agy agents seeded
  PASS sh-agent status labels the image mirror a plain copy
  PASS boot log records the harness deployed at the mirror's commit
  PASS no previous-harness artifacts (shell gates, GEMINI.md, settings.json)
  ```
  (A seventh line was named in the dispatch as "seven harness-section lines"; the section itself
  contains six `PASS` lines plus its own header — all six checks pass, matching the reworked
  commit-check logic reading `image/harness/COMMIT` from the host and the version from the
  container.)

Step 3: PASS.

### Step 4 — container checks + Dockerfile assertion proof

- `docker run --rm --entrypoint sh styrman-runtime:test -c 'test ! -e /styrman/harness/.git &&
  echo no-git; cat /styrman/harness/COMMIT'` → rc=0, printed `no-git` then
  `8046e4d5f24b009bd2461ebef28931cade7a4acd` — matches expected exactly.
- **Assertion-failure proof — FAILS to demonstrate failure:**
  - `cp -r image .agents/scratch/assert-test-copy`, `mkdir .agents/scratch/assert-test-copy/harness/.git`
    — confirmed present on disk (`find .../harness -maxdepth 1 -name .git`).
  - `docker build -q --build-arg STYRMAN_VERSION=assert-test -t styrman-runtime:assert-test
    .agents/scratch/assert-test-copy/` → **rc=0** (build succeeded), not the expected failure.
    Rebuilt `--no-cache` with a second tag to rule out layer caching → also **rc=0**.
  - Investigated: `docker run --rm --entrypoint sh styrman-runtime:assert-test2 -c 'test -e
    /styrman/harness/.git && echo GIT-PRESENT || echo GIT-ABSENT'` → `GIT-ABSENT`.
  - Root cause: `image/.dockerignore` contains `.git` and `**/.git`, so Docker strips the planted
    `harness/.git` directory from the build context before the `COPY harness/ /styrman/harness/`
    instruction (Dockerfile line 163) ever sees it. The `test ! -e /styrman/harness/.git`
    assertion at line 172 therefore never observes a `.git` entry in this scenario — not because
    the assertion is broken, but because the build-context exclusion makes it unreachable by this
    method. Confirmed independently of Docker's build cache (both cached and `--no-cache` builds
    behave the same).
  - Cleanup performed: `docker rmi styrman-runtime:assert-test styrman-runtime:assert-test2` (both
    removed), `.agents/scratch/assert-test-copy` removed. `styrman-runtime:dev` and `:latest` were
    never touched.

Step 4: FAIL (the manual proof specified in the dispatch does not, and — given `.dockerignore` —
cannot, demonstrate the assertion firing; see Root cause).

## Root cause

`image/.dockerignore` excludes `.git` and `**/.git` from the Docker build context for `image/`.
Since the build context is what `COPY harness/ /styrman/harness/` draws from, any `.git` directory
placed inside `image/harness/` on the host filesystem is stripped before the Dockerfile's own
`RUN test ! -e /styrman/harness/.git && ...` assertion runs — the assertion can never see a
`.git` entry via this path, regardless of what is on disk in the build context directory. This
does not mean the vendoring pipeline itself is unprotected against a real `.git` checkout leaking
into the committed `image/harness/` tree (step 1's `test ! -e image/harness/.git` on the actual
repo path still catches that at the source level, outside Docker). But the specific claim under
test — "prove the Dockerfile assertion can fail" via a build-context test — is false as specified:
the assertion is dead code with respect to any input that reaches it through a normal `docker
build`, because `.dockerignore` guarantees no `.git` directory ever survives into the build
context in the first place. The assertion can only ever fire if `.dockerignore`'s `.git` exclusion
is itself removed or bypassed (e.g. `docker build --ignorefile /dev/null`), which the dispatch's
method did not attempt and which is not how the image is actually built by `verify-commit.sh` or
the compose deploy.

## Coverage of changed code

- `image/Dockerfile` (new `test ! -e /styrman/harness/.git` RUN assertion) — the container-state
  checks (no `.git` present in `styrman-runtime:test`, COMMIT matches) pass, but there is no test
  in the changed set that exercises the assertion's failure path; the one manual method specified
  for doing so does not reach the assertion at all, per Root cause. **NOT COVERED**: the
  assertion's negative case (a `.git` dir actually present at COPY time) is unverified and, as
  built, unverifiable through the documented build path.
- `.agents/docs/testing/styrman/integration.sh` (reworked commit-check, ~lines 118-132) — covered;
  observed PASS in the step 3 run, check count and content match the dispatch's description.
  Confirmed the check reads `image/harness/COMMIT` from the host and length-checks/greps against
  the container version, per dispatch description of the diff.
- `image/harness/VENDORED.md` (§ Refresh paragraph) and `image/harness/SHA256SUMS` (regenerated) —
  covered by step 1's `sha256sum -c` and file-list diff; prose paragraph itself has no test (doc
  content, consistent with round 1's finding that doc/script prose changes are covered indirectly
  at most).

## Not examined

- Whether `.dockerignore`'s `.git` exclusion was already present before this round's Dockerfile
  change, or is itself part of the diff — not diffed in this pass; relevant to whether this is a
  pre-existing condition the new assertion was written against without noticing, or a regression
  introduced alongside it. `git log -p` on `image/.dockerignore` was not run.
- Whether the vendoring/refresh script (`image/bump-pins.sh` or the harness-refresh procedure) has
  its own independent guard against a stray `.git` ending up in the committed `image/harness/`
  tree, separate from the Dockerfile assertion — step 1's repo-level `test ! -e image/harness/.git`
  suggests yes, but the two guards' relationship (is the Dockerfile assertion redundant, or does it
  protect a different build path such as `docker build` from an uncommitted worktree) was not
  traced through `deploy/verify-commit.sh` in this round.

## Environment

- OS: Linux 6.12.107+deb13-amd64
- Docker images used: `styrman-runtime:test` (already cached, built 2026-09-14T12:17:33+02:00 from
  this tree, no rebuild triggered); scratch tags `styrman-runtime:assert-test` and `:assert-test2`
  built and removed within this run; no Docker Hub pulls performed (BUG-056 avoided — all layers
  cached).
- `styrman-runtime:dev` and `:latest` were not touched.
