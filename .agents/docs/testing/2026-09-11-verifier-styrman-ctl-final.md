---
owner: verifier
date: 2026-09-11
---
# Verification — styrman-ctl final state (e34bf37)

Range reviewed: 108c4e4..e34bf37 (final of four review rounds)

## Status: PASS

Scope: complete — all 6 requested verification tiers ran, including real
read-only production-host commands on `dave`.

## Pre-flight

`git log -1 HEAD` = `e34bf37` (working tree already at the target commit —
no worktree needed). `git diff e34bf37 -- deploy/styrman-ctl
internal/config/styrman_ctl_test.go` = empty (both files match the commit
exactly). Working tree was NOT fully clean: `.agents/docs/bugs/INDEX.md`,
`.claude/agent-memory/documentation/MEMORY.md`,
`.claude/agent-memory/documentation/project_styrman_docs.md` modified and
`.agents/docs/bugs/BUG-041-piped-verification-guarded-commit.md` untracked
— none of these are `deploy/styrman-ctl` or the test file, so they do not
affect this verification; noted, not fixed (see Not examined).

## Commands

1. `shellcheck deploy/styrman-ctl` → exit 0, no output. PASS
2. `go vet ./...` → exit 0, no output. PASS
3. `go test -race -count=1 -run StyrmanCtl -v ./internal/config/` → exit 0,
   `ok github.com/kesokaj/styrman-crew/internal/config 3.965s`, 15 PASS:
   TestStyrmanCtlHelpAndUnknownVerb, TestStyrmanCtlStatusJSON,
   TestStyrmanCtlStatusNothingInstalled, TestStyrmanCtlValidate,
   TestStyrmanCtlEnvSet, TestStyrmanCtlGuardPath, TestStyrmanCtlInstallPlan,
   TestStyrmanCtlInstallRefusals, TestStyrmanCtlUpdateSetupPlan,
   TestStyrmanCtlEditPlan, TestStyrmanCtlUpdatePlan,
   TestStyrmanCtlUninstallPlans, TestStyrmanCtlUninstallConfirmations,
   TestStyrmanCtlOverridesGatedOutsideHarness,
   TestStyrmanCtlDisplayValuesArePrintable. PASS
4. `go test -race -count=1 ./...` → exit 0, all 15 packages `ok`
   (cmd/styrman, internal/config, conversation, credbroker, credstore,
   files, gitrepo, history, httpapi, identity, links, orchestrator,
   project, terminal, userstore; 2 packages "no test files"). PASS
5. Real read-only production host (`dave`) commands, each run bare, exit
   status read directly:
   - `sudo -n deploy/styrman-ctl status` → exit 0, verdict OK.
   - `sudo -n deploy/styrman-ctl install --dry-run --version 1.27.1
     --skip-auto-update` → exit 0.
   - `sudo -n deploy/styrman-ctl edit --dry-run
     STYRMAN_IDLE_TIMEOUT=9h` → exit 0.
   - `sudo -n deploy/styrman-ctl update --dry-run` → exit 0.
   - `sudo -n deploy/styrman-ctl uninstall --level 2 --dry-run` → exit 0;
     plan contains `+ iptables -D DOCKER-USER -i sdb-+ -j STYRMAN-FWD` and
     `+ iptables -D INPUT -i sdb-+ -j STYRMAN-IN`, and `+ rm -rf --
     /etc/styrman` then `+ rm -rf -- /opt/styrman`; confirm word
     `uninstall`.
   - `sudo -n deploy/styrman-ctl uninstall --level 3 --dry-run` → exit 0;
     plan ends `+ du -sh /var/lib/styrman` then `+ rm -rf --
     /var/lib/styrman`, confirm word `dave`.
   - `STYRMAN_ETC_DIR=/tmp/x sudo -nE deploy/styrman-ctl status` → exit 1,
     stderr `styrman-ctl: ERROR STYRMAN_ETC_DIR is a test seam, not a
     production knob (unset it)`.
   All 6 dry-run/status commands matched the expected exit codes and
   expected output lines exactly. PASS

   Nothing changed, before vs. after (identical in both snapshots):
   - `docker ps` styrman-server/styrman-caddy: both `Up 25 hours
     (healthy)`/`Up 25 hours` before and after.
   - `systemctl is-enabled styrman-firewall.timer styrman-update.timer`:
     `enabled` / `enabled`, both runs.
   - `sudo -n ls /etc/styrman`: `allowed_signers firewall.conf gh-token
     update`, both runs.
   - `sudo -n stat -c %Y /opt/styrman/deploy/.env`: `1788288805`, both
     runs (mtime unchanged).
   - `docker images -q | wc -l`: `88` before and after (diff empty).

6. `grep -c '^func Test' internal/config/styrman_ctl_test.go` → `15`.
   `git diff --stat 108c4e4..e34bf37 -- '*_test.go'` → `78 insertions(+), 6
   deletions(-)` in `internal/config/styrman_ctl_test.go`. Confirmed no
   removed test function: `108c4e4` had 13 `^func Test` functions vs `15`
   at `e34bf37` — net addition of 2 test functions, the 6 deleted lines
   are edits inside existing test bodies, not removals of a `func Test`
   declaration. PASS

## Coverage of changed code

`deploy/styrman-ctl` (shell) is covered by
`internal/config/styrman_ctl_test.go`'s 15 Go-driven scenario tests plus
`shellcheck`; both ran clean above.

## Environment

Host `dave` (production), user `kesokaj`, passwordless `sudo -n` available.
Linux 6.12.107+deb13-amd64. Go toolchain per repo `go.mod`. Docker 29.7.2,
Kata 3.31.0 (mvm). `styrman-server`/`styrman-caddy` both `1.27.1`, deployed
version confirmed matching (sha `4686792`).

## Not examined

- Did not investigate or fix the pre-existing working-tree modifications
  to `.agents/docs/bugs/INDEX.md`/`.claude/agent-memory/*` or the
  untracked `BUG-041-piped-verification-guarded-commit.md` — out of scope
  for this verification (neither touches `deploy/styrman-ctl` or its
  test), flagged for the dispatching agent to reconcile.
- Did not run `go test -race -count=1 ./...` for the vendored Caddy
  ingress module (`deploy/caddy/ingress`) — not requested in this task's
  step list.
