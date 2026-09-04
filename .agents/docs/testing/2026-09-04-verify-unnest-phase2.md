---
owner: verifier
date: 2026-09-04
---
# Verification — unnest-project-homes Phase 2   (2026-09-04)

Range reviewed: v1.20.0..worktree

## Status: PASS

Scope: complete — image build, integration.sh, e2e run, go build/vet/test all ran on the live working tree against `styrman-runtime:test`; a concurrent upgrade fixture (port 18099, `.agents/scratch/unnest-*`) was left untouched.

## Commands

- `docker build -t styrman-runtime:test --build-arg STYRMAN_VERSION=test-<ts> image/` → all layers CACHED, `naming to docker.io/library/styrman-runtime:test` done, real 2.3s.
- `bash .agents/docs/testing/styrman/integration.sh styrman-runtime:test` → `passed=62 failed=0 skipped=3 aborted=0` (matches expected 62/0/3/0), real 2m4.9s.
  - Two listing checks: `project listing works (shows .git)`, `home listing works (per-project, shows .styrman)` — both PASS.
  - Five harness checks: `sh-agent reports no drift in the project home`, `CLAUDE.md is the two-line import stub`, `28 skills and 7 agy agents seeded`, `no previous-harness artifacts (shell gates, GEMINI.md, settings.json)`, `boot to ready within R1's 15 s hard limit (measured 1.72s; soft target 5 s)` — all PASS.
  - The 3 SKIP were the expected host-firewall-blocked host→bridge preview checks (prod unaffected).
  - Isolated state dir `/tmp/styrman-itest-lVBd`: confirmed removed after the run (`ls` → No such file or directory). Did not catch it mid-run to inspect `<state>/homes/<user>/p/<uuid>` vs `<state>/homes/<user>/base` layout directly (the run completed in the background before a mid-run inspection could be scheduled); the suite's own PASS lines for "workspace dir on host" and "sh-agent reports no drift in the project home" indicate the layout resolved correctly, but this is not a direct filesystem observation.
- `bash web/e2e/run.sh styrman-runtime:test` → `passed=61 failed=0 skipped=2` (matches expected 61/0/2), state dir `/tmp/styrman-e2e-WhYx`, no failures.
  - `git status --short -- web/dist` → only `web/dist/client/_shell.html` modified (known build-id drift); restored with `git checkout -- web/dist/client/_shell.html`; `git status --short -- web/dist` clean after.
- `go build ./...` → clean, real 0.4s.
- `go vet ./...` → clean, real 0.2s.
- `go test ./... -race -count=1` → all packages `ok` (cmd/styrman, internal/config, conversation, credbroker, credstore, files, gitrepo, history, httpapi, identity, links, orchestrator, project, terminal, userstore; cmd/wsprobe and web have no test files), real 1.9s.
- `docker ps --format '{{.Names}}\t{{.Status}}' | grep styrman`:
  - Before integration.sh: `styrman-sys-setup-kesokaj` (Up 41s, fixture), `styrman-c45858be-...` (Up 44s, fixture project), `styrman-caddy` (Up 53m), `styrman-server` (Up 53m healthy), `styrman-64ac08e4-...` (Up 9h, operator's project).
  - After integration.sh / before e2e: `styrman-sys-setup-kesokaj` (Up 45s, recreated — fixture churn), `styrman-caddy` (Up 56m), `styrman-server` (Up 56m healthy), `styrman-64ac08e4-...` (Up 9h) — production set unchanged.
  - After e2e: `styrman-caddy` (Up 59m), `styrman-server` (Up 59m healthy), `styrman-64ac08e4-...` (Up 9h) — production set unchanged; fixture's setup container had exited/rotated by this point (fixture churn, not touched by this run).

## Coverage of changed code

Working tree at time of this run was clean per `git status` (Phase 1's changes were already committed as of `0da855e`; no Phase-2-specific code diff exists yet beyond docs/plan). This run verifies Phase 1's `base`-layout change end-to-end via the full integration + e2e suites plus the unit/race gate, per the Phase 2 gate item "integration.sh and web/e2e/run.sh green on styrman-runtime:test."

## Environment

Linux 6.12.107+deb13-amd64, Go 1.25.0, Docker (styrman-runtime:test built from cache), Node/Playwright via web/e2e/run.sh (chromium_headless_shell-1234).
