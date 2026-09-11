---
owner: verifier
date: 2026-09-11
---
# Verification — styrman-ctl Phase 3 (edc4858)

Range reviewed: edc4858 (single commit), verified via `git worktree add .agents/scratch/verify-phase3 edc4858` — NOT the dirty working tree (which carries uncommitted Phase 4 edits to `deploy/styrman-ctl`).

## Status: PASS

Scope: complete — shellcheck, go vet, targeted config test suite, full race test suite, six real read-only dry-run checks against the production host, and test-count/function-diff discipline all ran.

## Commands

1. `shellcheck deploy/styrman-ctl` → exit 0, no findings.
2. `go vet ./...` → exit 0, no findings.
3. `go test -race -count=1 -run StyrmanCtl -v ./internal/config/ 2>&1 | grep -E '^(--- |ok|FAIL)'` → 11/11 PASS:
   `TestStyrmanCtlHelpAndUnknownVerb`, `TestStyrmanCtlStatusJSON`, `TestStyrmanCtlStatusNothingInstalled`, `TestStyrmanCtlValidate`, `TestStyrmanCtlEnvSet`, `TestStyrmanCtlGuardPath`, `TestStyrmanCtlInstallPlan`, `TestStyrmanCtlInstallRefusals`, `TestStyrmanCtlUpdateSetupPlan`, `TestStyrmanCtlEditPlan`, `TestStyrmanCtlUpdatePlan` — `ok  github.com/kesokaj/styrman-crew/internal/config  2.998s`.
4. `go test -race -count=1 ./...` → all 15 packages `ok` (cmd/styrman, internal/config, conversation, credbroker, credstore, files, gitrepo, history, httpapi, identity, links, orchestrator, project, terminal, userstore); `cmd/wsprobe` and `web` report "no test files" (pre-existing, out of scope for this commit). No FAIL.
5. Real read-only dry-runs on the production host (`dave`), run from the edc4858 worktree:
   - `sudo -n deploy/styrman-ctl edit --dry-run STYRMAN_IDLE_TIMEOUT=9h` → exit 0. Output contained `+ env_set /opt/styrman/deploy/.env STYRMAN_IDLE_TIMEOUT` and `+ env STYRMAN_VERSION=1.27.1 docker compose -f /opt/styrman/deploy/docker-compose.yml --env-file /opt/styrman/deploy/.env up -d` (re-applies at the deployed version, 1.27.1, from /opt/styrman) — matches expectation.
   - `sudo -n deploy/styrman-ctl update --dry-run` → exit 0. Output contained `+ systemctl start styrman-update.service` followed by `+ journalctl -u styrman-update.service -n 20 --no-pager` — matches expectation.
   - `sudo -n deploy/styrman-ctl update --redeploy --dry-run` → exit 0. Output contained `+ env STYRMAN_UPDATE_STATE_DIR=/etc/styrman/update STYRMAN_DEPLOY_DIR=/opt/styrman STYRMAN_GH_TOKEN_FILE=/etc/styrman/gh-token STYRMAN_ALLOWED_SIGNERS=/etc/styrman/allowed_signers /usr/local/bin/styrman-update --force` — matches expectation exactly (all inputs named, `--force`).
   - Nothing changed: `sudo -n stat -c '%Y' /opt/styrman/deploy/.env` = `1788288805` before and after (identical). `systemctl is-enabled styrman-update.timer` = `enabled` both times. `docker ps --format '{{.Names}} {{.Status}}'` = `styrman-caddy Up 24 hours` / `styrman-server Up 24 hours (healthy)` unchanged before and after all three dry-runs.
6. Test-count discipline:
   - `grep -c '^func Test' internal/config/styrman_ctl_test.go` → `11`.
   - `git show --stat edc4858 -- '*_test.go'` → `internal/config/styrman_ctl_test.go | 142 ++++++++++++++++++++++++++++++++++-- ` — 136 insertions, 6 deletions.
   - Function-level diff (`git show edc4858^:...` vs current, sorted `^func Test` lines): before had 9 functions, after has 11 — the only differences are two ADDED functions, `TestStyrmanCtlEditPlan` and `TestStyrmanCtlUpdatePlan`. No test function was removed.
   - Content of the 6 deleted lines (`git show edc4858 -- internal/config/styrman_ctl_test.go`, lines 111-113 of the old file): a stale assertion block inside the pre-existing update test —
     ```
     // Plain update is still the Phase 3 stub.
     if out, code := ctl(t, e, "update", "--dry-run"); code == 0 || !strings.Contains(out, "not implemented") {
         t.Fatalf("plain update: %d %s", code, out)
     }
     ```
     This matches the dispatch's expectation: plain `update` is no longer a stub in Phase 3, so the old "not implemented" assertion was removed as part of extending the same test function, not by deleting a test function. No BUG-005-style silent deletion.

## Coverage of changed code

`git show --stat edc4858` touches `deploy/styrman-ctl` (script) and `internal/config/styrman_ctl_test.go` (tests) only. The test file is the covering test for the script; both are exercised by step 3/4 above. No other source files changed in this commit.

## Environment

- OS: Linux 6.12.107+deb13-amd64 (production host `dave`).
- Go: per repo `go.mod` toolchain, via `go test -race`.
- shellcheck: system-installed, invoked directly.
- Worktree: `.agents/scratch/verify-phase3`, detached HEAD at edc4858; removed after verification (`git worktree remove`).
- Docker: `styrman-server:1.27.1` / `styrman-caddy:1.27.1` healthy throughout, unaffected by any dry-run.
