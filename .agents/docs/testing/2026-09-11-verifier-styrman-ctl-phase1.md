---
owner: verifier
date: 2026-09-11
---
# Verification — styrman-ctl Phase 1   (2026-09-11)

Range reviewed: HEAD (092c7ef69c1d6d4da6ce1710f66e892cdd0b49e8) — "styrman-ctl, phase 1: one front door for the deploy scripts — skeleton, --dry-run harness, .env helpers with per-key validation, path guard, status (human + --json), Go test harness with fake tools under a scratch prefix"

## Status: PASS

Scope: complete — all steps in the dispatch ran, on the production host, read-only.

## Commands

1. `shellcheck deploy/styrman-ctl` → exit 0, no output.
2. `go vet ./...` → exit 0, no output.
3. `go test -race -count=1 -v ./internal/config/` → exit 0, `ok github.com/kesokaj/styrman-crew/internal/config 3.253s`. StyrmanCtl tests (6, matches expected count):
   - TestStyrmanCtlHelpAndUnknownVerb — PASS
   - TestStyrmanCtlStatusJSON — PASS
   - TestStyrmanCtlStatusNothingInstalled — PASS
   - TestStyrmanCtlValidate — PASS
   - TestStyrmanCtlEnvSet — PASS
   - TestStyrmanCtlGuardPath — PASS
4. `go test -race -count=1 ./...` → exit 0, all packages `ok` (cmd/styrman, internal/config, conversation, credbroker, credstore, files, gitrepo, history, httpapi, identity, links, orchestrator, project, terminal, userstore; cmd/wsprobe and web report "no test files").
5. `deploy/styrman-ctl help` (own user, no sudo) → exit 0, printed usage/menu text.
   `deploy/styrman-ctl frobnicate` → exit 1, `styrman-ctl: ERROR unknown verb frobnicate (try: help)`.
6. `sudo -n deploy/styrman-ctl status` → exit 0. Human output:
   ```
   styrman-ctl status  (host dave, 2026-09-11 14:40)
     server     healthy   styrman-server:1.27.1 (styrman-caddy:1.27.1)   https://crew.styrman.xyz
     deployed   v1.27.1    /etc/styrman/update/deployed-version, sha 4686792dd99a6d5d47837ad0cba3e5133b398e07
     updater    enabled/active (timer), static/inactive (service)   clone /opt/styrman at 4686792 (v1.27.1)
     firewall   enabled/inactive (service), enabled/active (timer)   chains STYRMAN-FWD (7 rules) STYRMAN-IN (1)
     projects   0 running microVMs, 1 project dirs, /var/lib/styrman 174M
     host       Docker 29.7.2, Kata 3.31.0 (mvm), operator kesokaj
     verdict    OK
   ```
   `sudo -n deploy/styrman-ctl status --json | python3 -m json.tool` → exit 0, valid JSON parsed cleanly, `"exit": 0`, `"installed": true`.

   Cross-checks (all match the status output exactly):
   - `sudo -n docker inspect -f '{{.Config.Image}}' styrman-server` → `styrman-server:1.27.1` — matches `server.image`.
   - `systemctl is-enabled styrman-update.timer` → `enabled` — matches `updater.timer` = `enabled/active`.
   - `sudo -n cat /etc/styrman/update/deployed-version` → `v1.27.1` — matches `deployed.version`.

7. Test-count discipline:
   - `grep -c '^func Test' internal/config/styrman_ctl_test.go` → `6`, matches expected.
   - `git show --stat HEAD -- '*_test.go'` → only `internal/config/styrman_ctl_test.go | 280 ++++++++++++++++++++++++++++++++++++` (1 file changed, 280 insertions(+), 0 deletions). No deletions anywhere in test files.
   - `git diff --numstat HEAD~1 HEAD -- internal/config/styrman_ctl_test.go` → `280 0`, confirming pure addition.
   - `git log --diff-filter=A -- internal/config/styrman_ctl_test.go` confirms this commit is the file's first appearance (new file, not an overwrite — satisfies the BUG-005 writing-files rule).

install/update/edit/uninstall were NOT run (still stubs per plan; dispatch explicitly excluded them). Nothing on the host was changed — only `status`, `status --json`, `help`, and an invalid-verb call were exercised, all read-only per `deploy/styrman-ctl`'s own design.

## Root cause (FAIL only)

N/A — no failures.

## Coverage of changed code

- `deploy/styrman-ctl` — covered by `internal/config/styrman_ctl_test.go` (6 tests: help/unknown-verb, status JSON, status nothing-installed, env validate, env set, path guard) plus `shellcheck` for shell-lint coverage.
- `internal/config/styrman_ctl_test.go` — the test file itself; new, additive only (see test-count discipline above).
- `.agents/docs/plans/2026-09-11-installer-ctl.md`, `deploy/README.md`, `.agents/docs/testing/2026-09-11-styrman-ctl-phase1-skeleton-status.md`, `.agents/docs/testing/2026-09-11-verify-styrman-ctl-phase1.md` — docs/plan changes, not code; no test coverage expected.

## Environment

- Host: production host `dave` (crew.styrman.xyz), run from repo root `/home/kesokaj/workspace/styrman-crew` on branch `main` at commit `092c7ef69c1d6d4da6ce1710f66e892cdd0b49e8`.
- Go: as configured in repo toolchain (build/test succeeded; version not separately queried).
- Docker: 29.7.2. Kata runtime: mvm 3.31.0.
- `sudo -n` used for read-only `status`/`status --json`/`docker inspect`/`cat deployed-version` per dispatch; no install/update/edit/uninstall verbs invoked, no host state changed.
- Pre-existing unrelated working-tree modifications noted at session start (`.claude/agent-memory/documentation/MEMORY.md`, `.claude/agent-memory/documentation/project_styrman_docs.md`) — untouched by this verification, not part of the reviewed commit.
