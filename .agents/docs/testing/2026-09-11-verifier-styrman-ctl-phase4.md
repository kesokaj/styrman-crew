---
owner: verifier
date: 2026-09-11
---
# Verification — styrman-ctl Phase 4 + Phase 3 review fixes (2026-09-11)

Range reviewed: commit 108c4e4 (HEAD at time of run; tree already at this commit,
only unrelated `.claude/agent-memory/` files modified — no worktree needed).

## Status: PASS

Scope: complete — all six requested steps ran, including the five read-only
production dry-runs and before/after state proofs.

## Commands

1. `shellcheck deploy/styrman-ctl` → exit 0, no findings. PASS
2. `go vet ./...` → exit 0, no findings. PASS
3. `go test -race -count=1 -run StyrmanCtl -v ./internal/config/ 2>&1 | grep -E '^(--- |ok|FAIL)'` →
   13 `--- PASS` lines (TestStyrmanCtlHelpAndUnknownVerb, StatusJSON,
   StatusNothingInstalled, Validate, EnvSet, GuardPath, InstallPlan,
   InstallRefusals, UpdateSetupPlan, EditPlan, UpdatePlan, UninstallPlans,
   UninstallConfirmations) + `ok  .../internal/config  3.374s`. PASS
4. `go test -race -count=1 ./...` → all 15 testable packages `ok`
   (cmd/styrman, internal/config, conversation, credbroker, credstore,
   files, gitrepo, history, httpapi, identity, links, orchestrator,
   project, terminal, userstore); `cmd/wsprobe` and `web` report
   `[no test files]` (expected, non-Go/no tests). Exit 0. PASS
5. Production host dry-runs (all read-only, exit codes as expected):
   - `sudo -n deploy/styrman-ctl status` → verdict **OK**, exit 0. PASS
   - `install --dry-run --version 1.27.1 --skip-auto-update` → prints plan
     ending "install: done. Log out and back in...", exit 0. PASS
   - `edit --dry-run STYRMAN_IDLE_TIMEOUT=9h` → prints env_set + recreate
     plan + status block, exit 0. PASS
   - `update --dry-run` → prints `systemctl start styrman-update.service` +
     journalctl + status block, exit 0. PASS
   - `uninstall --level 3 --dry-run` → full plan ends with
     `+ rm -rf -- /var/lib/styrman` and
     `==> uninstall: dry run — nothing executed (the word to type for level 3 is: dave)`,
     exit 0. PASS
   - `uninstall --level 1 --dry-run --confirm nope` → prints plan then
     `styrman-ctl: ERROR --confirm nope does not match the required word — aborted, nothing changed`,
     exit 1, no command executed. PASS
   - Nothing changed, proven directly:
     - `docker ps` for styrman-server/styrman-caddy: `Up 24 hours (healthy)` /
       `Up 24 hours` identical before and after.
     - `systemctl is-enabled styrman-firewall.timer styrman-update.timer`:
       `enabled` / `enabled`, identical before and after.
     - `sudo -n ls /etc/styrman /opt/styrman/deploy/.env`: identical listing
       (`allowed_signers firewall.conf gh-token update`, and the `.env` file)
       before and after.
     - `docker images | wc -l`: `89` before and after (unchanged).
6. Test-count discipline:
   - `grep -c '^func Test' internal/config/styrman_ctl_test.go` → `13`. PASS
   - `git diff --stat edc4858..108c4e4 -- '*_test.go'` → one file,
     190 insertions / 13 deletions; `git diff ... | grep -E '^-func Test'`
     is empty (no `func Test...` line removed). The 13 deleted lines are the
     documented stale stub assertion (`uninstall --dry-run` "not implemented"
     check) plus a few reflowed lines inside continuing test bodies
     (askpass/credential-helper/fetch assertions), not removed tests. PASS

## Coverage of changed code

- `deploy/styrman-ctl` — covered by `internal/config/styrman_ctl_test.go`
  (13 `TestStyrmanCtl*` functions exercising help, status, validate, env,
  guard-path, install/update/edit/uninstall plans and refusals/confirmations)
  and by the live read-only dry-run exercise above on the actual production
  host.

## Environment

- Host: production styrman host (`dave`), used strictly read-only via
  `sudo -n` (`status` and `--dry-run` verbs only; no non-dry-run verb was
  ever invoked; no source was edited).
- Go 1.25.0 linux/amd64. `shellcheck` from `/usr/bin/shellcheck`.
- Docker 29.7.2, Kata 3.31.0 (mvm) per `styrman-ctl status`.
- Deployed version at time of run: v1.27.1, sha 4686792.
