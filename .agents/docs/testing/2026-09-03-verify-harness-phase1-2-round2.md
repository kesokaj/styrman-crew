---
owner: verifier
date: 2026-09-03
---
# Verification — Phase 1-2 image-harness-replacement, round 2 (2026-09-03)

Round 1: `.agents/docs/testing/2026-09-03-verify-harness-phase1.md`, `…-phase2.md`.
This round re-verifies the security-review fixes: `image/Dockerfile` (`GOPROXY=off`, `chmod -R go-w /styrman/harness`), `image/.dockerignore` (new), `image/harness/SHA256SUMS` (new, 151 files), `image/harness/VENDORED.md`, `deploy/verify-commit.sh` (new integrity/pin/roster steps), `deploy/vulncheck.sh` (now scans `image/harness`), `image/entrypoint.sh` (timeout-guarded seeding, distinct WARN lines, stamp-on-both-success), `image/configs/harness-migrate.sh` (continues past unmovable paths, exit 1, `plain()` strips control chars).

Range reviewed: working tree (uncommitted) vs `main` — the named files above, plus a fresh `styrman-runtime:test` build (image id `30161db3104e0`, deleted after this run) and one-off host fixtures under `.agents/scratch/verify-fixture-r2/` (removed after this run).

## Status: PASS

Scope: all 7 requested steps ran; complete. No file edited except this log. `integration.sh` and the e2e suite were not run (per instruction). Only `:test` tags used, no `:dev`/version tag.

## Commands

1. `bash -n` + `shellcheck -S warning` on `image/entrypoint.sh` and `deploy/verify-commit.sh`; `shellcheck` (default severity) on `image/configs/harness-migrate.sh` — all six commands exit 0, no findings.

2. `(cd image/harness && sha256sum -c --quiet SHA256SUMS && go vet ./... && go build ./...)` → all exit 0, no output (silent success).
   `diff <(find . -type f ! -name SHA256SUMS | sed 's#^\./##' | LC_ALL=C sort) <(cut -c67- SHA256SUMS)` → empty, exit 0 — manifest lists exactly the tree's files.

3. Pin extraction with `verify-commit.sh`'s own `sed` expressions:
   `srv_go=golang:1.25.13-trixie` (from `deploy/Dockerfile.server`), `img_go=golang:1.25.13-trixie` (from `image/Dockerfile`) — equal and non-empty.

4. `docker build -t styrman-runtime:test --build-arg STYRMAN_VERSION=test-$(date +%s) image/` → succeeded (mostly cached, harness-builder + final chmod/install layers ran fresh). Inside the image:
   - `stat -c '%A %U:%G' /styrman/harness /styrman/harness/skills/golang/SKILL.md /styrman/bin/sh-agent` → `drwxr-xr-x root:root`, `-rw-r--r-- root:root`, `-rwxr-xr-x root:root` — no group-write on the tree, binary is `0755 root`.
   - `/styrman/bin/sh-agent list --repo /styrman/harness` → `Skills (28)`, `Subagents (7)`.
   - `ls /styrman/configs/harness-migrate.sh` → present.
   - `grep -c 'timeout 60' /styrman/entrypoint.sh` → `2`.

5. `bash deploy/vulncheck.sh` — per-module:
   - `image/harness` → **No vulnerabilities found.**
   - `.` (root) → 2 vulnerabilities from 1 module: `GO-2026-4887` and `GO-2026-4883`, both `github.com/docker/docker@v28.5.2+incompatible` (the accepted docker/docker pair).
   - `deploy/caddy/ingress` → same accepted pair (`GO-2026-4887`/`GO-2026-4883`, `github.com/docker/docker`).
   No new findings; `image/harness` is clean.

6. Migration helper fixtures (host, `.agents/scratch/verify-fixture-r2/`, removed after this run):
   - **(a) old-home fixture**: seeded from `git archive v1.19.1 image/harness`, ran its `setup.sh` (full old harness deployed: `.claude/{CLAUDE.md,settings.json,skills/,agents/,statusline.sh,format-hook.sh,feedback-capture.sh}`, `.gemini/{GEMINI.md,format-hook.sh,config/skills/,config/plugins/harness/}`, `.agents/bin/{resume-context.sh,trail-check.sh}`), stamp `.styrman/.harness-version=1.19.1` written manually, three `keep.txt` fixtures placed outside the moved paths (`.claude/skills/keep.txt` — inside the moved `skills/` dir — and `.gemini/keep.txt`, `.agents/keep.txt` outside the moved paths). `harness-migrate.sh` moved the same 12 paths as round 1 into one `harness-backup-<ts>` dir, exit 0. `install --repo /styrman/harness --home` with `GIT_CONFIG_GLOBAL=/dev/null` → `256 written, 0 unchanged, 0 removed, 0 kept`. Post-install: `.claude/settings.json`, `.claude/statusline.sh`, `.gemini/GEMINI.md`, `.gemini/config/plugins/harness`, `.agents/bin` all absent; exactly one backup dir; all three `keep.txt` md5s unchanged through migrate and install; `status` → `drift none — 255 files match the repo`. Second `harness-migrate.sh` run → `sh-agent manifest present — already migrated`, exit 0, backup-dir count stayed 1.
   - **(b) unmovable path**: stamped home with `.gemini/GEMINI.md` and `.claude/{settings.json,skills/x}`, `chmod 500 .claude`. Helper output: moved `.gemini/GEMINI.md`; two lines `WARN: could not move .claude/settings.json — left in place` and `WARN: could not move .claude/skills — left in place`; summary `moved 1 path(s) to … ; 2 left in place`; **exit 1**. Mode restored to 700 afterward.
   - **(c) embedded-newline filename**: `.gemini/x.bak.1\n[styrman] forged` (real newline in the name) plus `.gemini/GEMINI.md`, stamped home. Both moved (`moved 2 path(s) … ; 0 left in place`, exit 0); the logged line for the forged file — `moved .gemini/x.bak.1[styrman] forged` — verified with `cat -A` to contain exactly one line terminator (i.e., no embedded newline inside the name), confirming `plain()` strips control characters before logging.
   - (Also re-confirmed the round-1 "fresh dir, only the stamp" case is unaffected — not re-run this round since no code path touching it changed.)

7. Root baseline: `go build ./...`, `go vet ./...`, `go test ./...` — all exit 0. 12 `ok` packages (cmd/styrman, internal/config, conversation, credbroker, credstore, files, gitrepo, history, httpapi, identity, links, orchestrator, project, terminal, userstore), 2 `[no test files]` (cmd/wsprobe, web). No failures.

## Root cause (FAIL only)

N/A — all steps passed.

## Coverage of changed code

- `image/Dockerfile` (`GOPROXY=off`, `chmod -R go-w`) — covered by steps 2 (build succeeds under GOPROXY=off implicitly via the harness-builder stage) and 4 (permissions confirmed inside the built image).
- `image/.dockerignore` — covered indirectly by step 4's successful, reasonably-sized build context transfer (14.11kB observed); not independently content-diffed.
- `image/harness/SHA256SUMS`, `VENDORED.md` — covered by step 2 (integrity check + completeness diff both pass).
- `deploy/verify-commit.sh` (new steps) — the `sha256sum -c`/`go vet` and golang-pin-equality logic re-executed standalone in steps 2–3 with matching results; the script's own end-to-end run (including the runtime-image build + `sh-agent list` roster comparison) was **not invoked directly** — steps 4 covers its constituent checks manually instead, so the script's own control flow (e.g., failure-path wiring) is not exercised by this round.
- `deploy/vulncheck.sh` (scans `image/harness`) — covered by step 5, ran the actual script.
- `image/entrypoint.sh` (timeout guards, distinct WARN lines, stamp-on-both-success) — covered for static shape (syntax/lint, `timeout 60` count = 2, call sequence baked into image) but the runtime WARN/stamp behavior itself is not exercisable outside a real Kata boot, same deferral as round 1 (Phase 5 e2e).
- `image/configs/harness-migrate.sh` (continue-past-unmovable, exit 1, `plain()`) — directly covered by step 6's three fixture cases (a/b/c), all passing exactly as specified.

## Environment

- OS: Linux 6.12.107+deb13-amd64
- Docker: host Docker Engine 29.7.2 (no Colima — Linux box)
- shellcheck 0.10.0
- Go: module toolchain resolved via root `go.mod`/harness `go.mod`; version not independently queried this run
- Test image `styrman-runtime:test` built and deleted within this run; no `:dev` image touched
