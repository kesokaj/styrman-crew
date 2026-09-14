---
owner: verifier
date: 2026-09-14
---
# Verification — harness mirror refresh to 8046e4d (2026-09-14)

Range reviewed: v1.30.1..worktree

## Status: PASS

Scope: all five verification steps in the dispatch ran to completion (setup commands, mirror integrity, Tier 2 integration, manual mirror-refresh checks, shellcheck/bash -n/binary check).

## Commands

### Step 1 — AGENTS.md § Setup Commands

- `go test ./...` → rc=0, all packages `ok`. Log: `.agents/scratch/step1-gotest.log`
- `(cd deploy/caddy/ingress && go test ./...)` → rc=0, `ok github.com/spgo/devbox/caddy/ingress`
- `go vet ./...` → rc=0, no output
- `bash deploy/vulncheck.sh` → rc=3 (govulncheck "called vulnerability" exit, per its own header comment — this is a report, not a hard gate). Scanned three modules inside the pinned `golang:1.25.13-trixie@sha256:6be3...` builder image (image was already cached locally, no Docker Hub pull):
  - `.` (root module): 2 vulnerabilities — GO-2026-4887 and GO-2026-4883, both in `github.com/docker/docker@v28.5.2+incompatible`, both "Fixed in: N/A". These are the two accepted-per-release daemon-side findings the script's header names.
  - `deploy/caddy/ingress`: same 2 findings (GO-2026-4887, GO-2026-4883), same module, same reason.
  - `image/harness`: **"No vulnerabilities found."** — scanned clean.
- `(cd web && npm run check)` → rc=0 (`tsc -b`, no errors)
- `(cd web && npm run lint)` → rc=0 (`oxlint`, no errors)
- `(cd web && npm test)` → rc=0 — `Test Files 15 passed (15)`, `Tests 178 passed (178)`

Step 1: PASS.

### Step 2 — mirror integrity (deploy/verify-commit.sh's own checks, run against the working tree)

- `(cd image/harness && sha256sum -c --quiet SHA256SUMS)` → rc=0, no output (all hashes match)
- `(cd image/harness && go vet ./...)` → rc=0, no output
- File-list comparison (the check actually at deploy/verify-commit.sh lines 92-96; the dispatch's cited "lines 33-40" is stale — the real check moved down the file):
  `diff <(find . -type f ! -name SHA256SUMS | sed 's#^\./##' | LC_ALL=C sort) <(cut -c67- SHA256SUMS)` → empty diff (no extras/missing).
- `find image/harness -type l` → empty (no symlinks).
- `diff -rq --exclude .git --exclude .agents --exclude .gitignore ~/workspace/styrman-harness image/harness` → rc=1, exactly three lines:
  ```
  Only in image/harness: COMMIT
  Only in image/harness: SHA256SUMS
  Only in image/harness: VENDORED.md
  ```
- `cat image/harness/COMMIT` = `8046e4d5f24b009bd2461ebef28931cade7a4acd`; `git -C ~/workspace/styrman-harness rev-parse HEAD` = `8046e4d5f24b009bd2461ebef28931cade7a4acd` — equal.
- `(cd image/harness && go run ./cmd/sh-agent list | grep -E '^### ')` → rc=0, prints `### Skills (27)` and `### Subagents (7)`.

**Harness test suite finding (reported, not a project FAIL):**
- `(cd image/harness && go test ./...)` → rc=1: `--- FAIL: TestDryRunNeverBuilds` in `cmd/sh-agent`, package `internal/harness` ok. Failure message: `dry-run output missing "[dry-run] the binary would be rebuilt from"` — actual output was `[dry-run] plain copy, not its own git checkout: the running binary would be installed as is`.
- `(cd ~/workspace/styrman-harness && go test ./...)` → rc=0, both packages `ok`.
- Root cause: `TestDryRunNeverBuilds` assumes it runs inside its own git checkout (where dry-run reports "would be rebuilt from"); the vendored mirror under `image/harness` is a plain copy with no `.git`, so the same binary correctly reports the plain-copy path instead and the upstream test's assumption does not hold there. This is a property of the vendored test suite's environment assumption, not a defect in this project's change — confirmed by reading `deploy/verify-commit.sh`: its harness-mirror check (lines 88-106) runs only `sha256sum -c`, `go vet ./...`, the file-list diff, the symlink `find`, and a builder-pin check — it never runs `go test` inside `image/harness`, so this failing test is not on the project's gate path.

Step 2: PASS (mirror integrity, file list, symlinks, commit pin, list output all match expected). The `go test ./...` finding above is recorded as an observation about the vendored suite's own environment assumption, not a project FAIL.

### Step 3 — Tier 2 integration

- Check-count comparison: `git show HEAD:.agents/docs/testing/styrman/integration.sh | grep -c '^check '` = 90 (before); working tree `grep -c '^check ' .agents/docs/testing/styrman/integration.sh` = 92 (after) — two checks added, matching the two named harness checks.
- Port 18080 confirmed free before run (`ss -ltn | grep 18080` → nothing).
- `bash .agents/docs/testing/styrman/integration.sh styrman-runtime:test` → rc=0. Printed summary: `passed=94 failed=0 skipped=3 aborted=0` — matches the README's stated green-run expectation exactly.
- The two new harness-section lines both printed `PASS`:
  ```
  PASS sh-agent status labels the image mirror a plain copy
  PASS boot log records the harness deployed at the mirror's commit
  ```
  (both under `=== agent harness (ADR-062: sh-agent seeded write-once from the image) ===`)

Step 3: PASS. Log: `.agents/scratch/step3-integration.log`.

### Step 4 — manual mirror-refresh checks (README's "Manual, on every mirror refresh" list, items 1 and 2)

- `docker run --rm --entrypoint /styrman/bin/sh-agent styrman-runtime:test list --repo /styrman/harness | grep -E '^### '` → `### Skills (27)`, `### Subagents (7)`.
- `docker run --rm --entrypoint /styrman/bin/sh-agent styrman-runtime:test install --dry-run --repo /styrman/harness --home /check | head -1` → `[dry-run] plain copy, not its own git checkout: the running binary would be installed as is` — matches expected exactly.
- `docker run --rm --entrypoint sh styrman-runtime:test -c 'cat /styrman/harness/COMMIT; echo; grep -c "GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM" /styrman/entrypoint.sh'` → printed `8046e4d5f24b009bd2461ebef28931cade7a4acd` then `0`. The container's overall exit code was 1 because `grep -c` with a zero count itself exits 1 (last command in the `sh -c` string) — this is `grep`'s documented no-match behaviour, not a failure of the check; both printed values match what the dispatch names as expected.

Step 4: PASS.

### Step 5 — shellcheck / bash -n / binary check

- `bash -n` on `image/entrypoint.sh`, `.agents/docs/testing/styrman/integration.sh`, `deploy/verify-commit.sh` → rc=0 for all three.
- `shellcheck -S warning` on the same three files → rc=0 for all three, no warnings.
- `git diff --numstat HEAD` → 64 changed files, none with `-\t-` (binary) columns; `awk` filter for binary rows found nothing.

Step 5: PASS.

## Coverage of changed code

- `image/entrypoint.sh` (dropped `GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1` from the install line, since install on a plain-copy mirror never calls git) — covered by: the step 4 manual check (grep count 0 on the entrypoint), and by integration.sh's boot-time checks that actually exercise this code path (`sh-agent reports no drift in the project home`, `27 skills and 7 agy agents seeded`, the two new harness checks, `no previous-harness artifacts`).
- Two new integration.sh checks (`sh-agent status labels the image mirror a plain copy`, `boot log records the harness deployed at the mirror's commit`) — self-covering (they are the tests); both observed PASS in the step 3 run against `styrman-runtime:test` (image rebuilt 11:52 today from this tree).
- `image/harness/` mirror refresh (54 modified, 2 deleted, 5 untracked files) — covered by SHA256SUMS/file-list/symlink checks (step 2), by `go vet` (clean), and by the vendored `go test ./...` — NOT run as part of any gate (confirmed absent from `deploy/verify-commit.sh`'s harness-mirror check) and observed to fail on `TestDryRunNeverBuilds` for the environment reason above; that failure is pre-existing to the vendoring model (also fails the same way whenever the mirror is a plain copy) and is not something this refresh could have introduced or fixed.
- `deploy/verify-commit.sh`, `.agents/docs/testing/styrman/integration.sh`, `image/README.md`, `.agents/docs/specs/styrman/runtime-image.md`, `README.md`, `.agents/docs/testing/styrman/README.md` (doc/script prose and version-line changes) — no dedicated test; covered indirectly by shellcheck/bash -n (script syntax) and by the fact that integration.sh's own check count and PASS lines were directly observed to match the doc's stated numbers.

Not identified as uncovered: nothing in the changed-file list needed test coverage it did not get, except the vendored harness's own `TestDryRunNeverBuilds`, which is an upstream test-environment mismatch outside this project's gate, not an untested code path in this change.

## Environment

- OS: Linux 6.12.107+deb13-amd64
- Go: pinned builder image `golang:1.25.13-trixie@sha256:6be3daa12135c38c177e2fbbf38ef2ad000d09b34bfa9f73dd7ca785bff134a5` for vulncheck; host `go` used for the rest (versions not separately queried this run)
- Node: `npm` project in `web/` — versions not separately queried this run
- Docker: images used were already cached locally (`golang:1.25.13-trixie@...`, `styrman-runtime:test` built 2026-09-14 11:52:32 +0200); no Docker Hub pulls performed (BUG-056 avoided)
- `styrman-runtime:test` was rebuilt at 11:52 today from this tree, per dispatch instructions; `STYRMAN_TEST_IMAGE_NO_REBUILD` was not set.
