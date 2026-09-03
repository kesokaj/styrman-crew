---
owner: verifier
date: 2026-09-03
---
# Verification — image-harness-replacement Phase 1   (2026-09-03)

Range reviewed: working tree (uncommitted) — `image/Dockerfile` diff, `image/harness/` replaced with a mirror of
`~/workspace/styrman-harness` @ `6893e2218414617da2bb88259b905ca3882b2659` + `image/harness/COMMIT` and
`image/harness/VENDORED.md`. Untracked `image/configs/harness-migrate.sh` and any `image/entrypoint.sh` change
(Phase 2) explicitly out of scope and not touched.

## Status: PASS

Scope: complete for the six steps named — build, sh-agent list/version, in-image tree check, host-vs-mirror diff,
hadolint (unavailable, reported), repo baseline go build/vet/test. Integration.sh and e2e suite intentionally not run.

## Commands

1. `docker build -t styrman-runtime:test --build-arg STYRMAN_VERSION=test-1788469242 image/`
   → succeeded, `real 0m1.9s` (fully layer-cached from a prior identical build; `harness-builder` stage present and
   executed as step `[harness-builder 4/4] RUN ... go build ... ./cmd/sh-agent` — cache hit, not skipped). Final image
   `sha256:6f3c1e56716cebd3ab3ded9d16edf8d7baf187132e866a16122fab2906ee1f95` tagged `styrman-runtime:test`.

2. `docker run --rm --entrypoint /styrman/bin/sh-agent styrman-runtime:test list --repo /styrman/harness`
   → printed the full skills/agents table, header lines `### Skills (28)` and `### Subagents (7)` present, 28 skill
   rows and 7 agent rows counted in output. Matches requirement.
   `docker run --rm --entrypoint /styrman/bin/sh-agent styrman-runtime:test version` → `sh-agent (devel)`.

3. `docker run --rm --entrypoint sh styrman-runtime:test -c 'cat /styrman/harness/COMMIT; test -e /styrman/harness/setup.sh && echo OLD || echo gone; ls /styrman/harness/skills | wc -l; ls /styrman/harness/agents | wc -l'`
   → `6893e2218414617da2bb88259b905ca3882b2659` / `gone` / `29` / `7`.
   `git -C ~/workspace/styrman-harness rev-parse HEAD` → `6893e2218414617da2bb88259b905ca3882b2659` — matches.
   29 skills entries = 28 skill dirs + `VENDORED.md`; 7 agents — both as specified.

4. `diff -rq --exclude .git --exclude .agents --exclude .gitignore ~/workspace/styrman-harness image/harness`
   → exit 1, output exactly:
   ```
   Only in image/harness: COMMIT
   Only in image/harness: VENDORED.md
   ```
   Matches the required exact output.

5. `hadolint image/Dockerfile` → not available (no `hadolint` binary on host `$PATH`); `hadolint/hadolint` image not
   present locally (`docker images` had no match) and was not pulled per instructions. Reported as not available,
   no findings recorded.

6. At repo root:
   - `go build ./...` → exit 0, no output.
   - `go vet ./...` → exit 0, no output.
   - `go test ./...` → exit 0. `ok` for `cmd/styrman`, `internal/config`, `internal/conversation`, `internal/credbroker`,
     `internal/credstore`, `internal/files`, `internal/gitrepo`, `internal/history`, `internal/httpapi` (0.130s),
     `internal/identity`, `internal/links`, `internal/orchestrator`, `internal/project`, `internal/terminal`,
     `internal/userstore`; `cmd/wsprobe` and `web` report `[no test files]` (pre-existing, unaffected by this phase).
   - Integration.sh and web/e2e suite deliberately not run per task scope.

## Coverage of changed code

Phase 1 changes are entirely Dockerfile build wiring and a vendored-mirror data replacement (no Go/TS source under
test by `go test`/`npm test`); the applicable verification for this phase is the container-build and in-image checks
in steps 1–4 above, all of which passed. No source files were changed that would require unit-test coverage.

## Environment

- OS: Linux 6.12.107+deb13-amd64 (host)
- Go: go1.25.0 linux/amd64 (host toolchain); harness-builder stage inside Docker uses `golang:1.25.13-trixie` per
  Dockerfile pin.
- Docker: build ran natively on host Docker (no Colima; this is a Linux dev box per AGENTS.md).
- hadolint: not installed, and its Docker image not present locally — skipped per instructions (do not pull).
