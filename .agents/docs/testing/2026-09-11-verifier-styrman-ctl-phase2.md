---
owner: verifier
date: 2026-09-11
---
# Verification — styrman-ctl Phase 2 (2026-09-11)

Commit verified: c334efdfb7877bd62f3666486e4409f7c283597e
"styrman-ctl, phase 2: install (host-setup + guided .env + bring-up at a
version + auto-update offer) and update --setup (token, clone under a
one-shot askpass, .env copy, allowed_signers, timer); 3 dry-run plan tests;
Phase 1 review nits closed"

Range reviewed: HEAD (c334efd) working tree at time of run — matched `git log -1`.

## Status: PASS

Scope: complete — all six requested checks ran (shellcheck, go vet, targeted
config tests, full test suite, two real dry-runs against the production host,
test-count discipline).

## Commands

1. `shellcheck deploy/styrman-ctl` → no output, exit 0. PASS
2. `go vet ./...` → no output, exit 0. PASS
3. `go test -race -count=1 -run StyrmanCtl -v ./internal/config/ 2>&1 | grep -E '^(--- |ok|FAIL)'` →
   9× `--- PASS`, `ok github.com/kesokaj/styrman-crew/internal/config 2.502s`.
   Tests: TestStyrmanCtlHelpAndUnknownVerb, TestStyrmanCtlStatusJSON,
   TestStyrmanCtlStatusNothingInstalled, TestStyrmanCtlValidate,
   TestStyrmanCtlEnvSet, TestStyrmanCtlGuardPath, TestStyrmanCtlInstallPlan,
   TestStyrmanCtlInstallRefusals, TestStyrmanCtlUpdateSetupPlan. Matches
   expected 9 PASS. PASS
4. `go test -race -count=1 ./...` → all packages `ok` (cmd/styrman,
   internal/config, conversation, credbroker, credstore, files, gitrepo,
   history, httpapi, identity, links, orchestrator (21.6s), project,
   terminal, userstore); `cmd/wsprobe` and `web` report `[no test files]`
   (pre-existing, not part of this change). Exit 0. PASS
5. Production-host dry-runs (`sudo -n`, read-only):
   - `deploy/styrman-ctl install --dry-run --version 1.27.1 --skip-auto-update`
     → exit 0. Contained all three required lines verbatim:
     `+ bash /home/kesokaj/workspace/styrman-crew/deploy/host-setup.sh --user kesokaj`,
     `+ docker build -t styrman-runtime:1.27.1 --build-arg STYRMAN_VERSION=1.27.1 /home/kesokaj/workspace/styrman-crew/image`,
     `+ env STYRMAN_VERSION=1.27.1 docker compose -f /home/kesokaj/workspace/styrman-crew/deploy/docker-compose.yml --env-file /home/kesokaj/workspace/styrman-crew/deploy/.env up -d --build`.
     PASS
   - Created scratch files `.agents/scratch/gh-token-verify.txt` (`ghp_verifierFAKE`)
     and `.agents/scratch/allowed-signers-verify.txt`
     (`me@example.test ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIExample`).
   - `deploy/styrman-ctl update --setup --dry-run --gh-token-file .agents/scratch/gh-token-verify.txt --allowed-signers-file .agents/scratch/allowed-signers-verify.txt`
     → exit 0, plan ended with `+ systemctl start styrman-update.service`
     (preceded by `+ systemctl enable --now styrman-update.timer`). PASS
   - `grep -c verifierFAKE <captured output>` → `0` matches (grep exit 1 =
     no match) — token value never appeared in dry-run output. PASS

   Proof nothing changed:
   - `git status --porcelain` before and after: identical — only
     pre-existing unrelated modifications
     (`.claude/agent-memory/documentation/MEMORY.md`,
     `.../project_styrman_docs.md`, present before this run started) plus
     the two new scratch files under `.agents/scratch/` (gitignored, not
     tracked). No tracked repo file changed by the dry-runs.
   - `systemctl is-enabled styrman-update.timer`: `enabled` before, `enabled`
     after — unchanged.
   - `sudo -n stat -c '%U %a %Y' /etc/styrman/gh-token /etc/styrman/allowed_signers`:
     `root 600 1788328341` and `root 644 1788288688` before AND after —
     identical mtimes, confirming the dry-run wrote nothing to these files.

6. Test-count discipline:
   - `grep -c '^func Test' internal/config/styrman_ctl_test.go` → `9`,
     matches expected count.
   - `git show --stat HEAD -- '*_test.go'` → `internal/config/styrman_ctl_test.go
     | 174 +++++++++++++++++++++++++++++++++++- | 1 file changed, 173
     insertions(+), 1 deletion(-)` — net addition only, no removed test
     file, deletion count (1) consistent with a closing-brace/whitespace
     edit at the tail of an appended block, not a lost test (test count of
     9 confirmed independently in step 3).

## Coverage of changed code

Commit touches `deploy/styrman-ctl` (install + update --setup verbs) and
`internal/config/styrman_ctl_test.go` (+173 lines, 3 new dry-run plan
tests: TestStyrmanCtlInstallPlan, TestStyrmanCtlInstallRefusals,
TestStyrmanCtlUpdateSetupPlan, run and passing above). No other source
files changed in this commit per `git show --stat HEAD`.

## Environment

OS: Linux 6.12.107+deb13-amd64 (production host, same box `sudo -n` dry-runs
ran against). Go: per repo `go.mod` toolchain (build succeeded). Docker/
Colima: not exercised — both dry-runs stopped short of any real `docker`,
`git`, or `systemctl` mutation by design (`--dry-run`), confirmed via the
before/after mtime and `is-enabled` checks above.

## Overall verdict: PASS

All six steps passed with no deviations. No fixes were made; none were
needed.
