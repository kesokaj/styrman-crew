---
owner: verifier
date: 2026-09-21
---
# Verification — gcp-compute-engine-target-phase-1-round-3   (2026-09-21)

Range reviewed: live working tree (uncommitted), vs round 2's tree
(`testing/2026-09-21-verify-gcp-compute-engine-target-phase-1-round-2.md`, PASS).
Changes since round 2: `deploy/gcp/install.sh` step 5 lost `sudo env
GH_TOKEN=…` and gained a `git … verify-tag` step; `deploy/gcp/vm.sh` create
adds `block-project-ssh-keys=TRUE`; `deploy/styrman-ctl` `cmd_install`
exports `GH_TOKEN` from `--gh-token-file` before `host-setup.sh` (new log
line "exported from --gh-token-file"); `internal/config/gcp_scripts_test.go`
(11 `TestGCP*` tests, new assertions in `TestGCPInstallPlan` and
`TestGCPVMCreatePlan`) and `internal/config/styrman_ctl_test.go` (new
`TestStyrmanCtlInstallExportsTokenForHostSetup`).

## Status: PASS

Scope: Tier 1 unit + targeted config tests + shell syntax/lint + secret-shape
guard for the GCP Compute Engine phase-1 diff, per the dispatch's 8-command
list. Tier 2+ (integration, e2e, real GCP bring-up) not run — see Not
examined. `testing/guide.md` exists (written round 1/2, unchanged this run).

## Commands

1. `go vet ./...` → exit 0, no output.

2. `go test ./... -count=1` → exit 0. All packages `ok`, none FAIL:
   cmd/styrman, cmd/wsprobe (no test files), internal/config,
   internal/conversation, internal/credbroker, internal/credstore,
   internal/files, internal/gitrepo, internal/history, internal/httpapi,
   internal/identity, internal/links, internal/orchestrator,
   internal/project, internal/terminal, internal/userstore, web (no test
   files). counts: not printed per-test (no `-v`); package-level pass 16 ok
   / 0 FAIL / 0 skip at `44a88e2`+dirty for `go test ./... -count=1`.
   vs previous: round-2 log — same package set, all ok both times, no
   regression.

3. `go test ./internal/config/ -run 'TestGCP|TestStyrmanCtlInstall' -count=1 -v`
   → exit 0. 14 tests, all `--- PASS`:
   TestGCPVMCreateNeedsYesWithoutTerminal, TestGCPVMCreatePlan,
   TestGCPVMDeleteRefusesWithoutConfirm, TestGCPVMRejectsBadNames,
   TestGCPInstallPlan, TestGCPSSHUnreachableIsNamed,
   TestGCPInstallCloneHelperShape, TestGCPInstallRefusesLooseEnvFile,
   TestGCPInstallNeedsRunningVM, TestGCPInstallNeedsKVM,
   TestGCPVMStatusExitCodes (= 11 TestGCP*, matches expected count),
   TestStyrmanCtlInstallPlan, TestStyrmanCtlInstallRefusals,
   TestStyrmanCtlInstallExportsTokenForHostSetup (new, PASS).
   counts: pass 14, fail 0, skip 0 at `44a88e2`+dirty for this command.
   vs previous: round-2 log ran an overlapping but not identical -v
   selection; this run's new test (TestStyrmanCtlInstallExportsTokenForHostSetup)
   is new since round 2 and passes; no fall in pass, no rise in skip.

4. `bash -n deploy/gcp/vm.sh deploy/gcp/install.sh deploy/gcp/lib.sh deploy/host-setup.sh deploy/styrman-ctl deploy/verify-commit.sh`
   → exit 0, no output.

5. `shellcheck -S warning deploy/gcp/*.sh deploy/host-setup.sh` → exit 0, no
   output (deploy/gcp/vm.conf is not a `.sh` file and shellcheck's glob did
   not pick it up as a script; not relevant to the gate's list).
   `shellcheck -S warning deploy/styrman-ctl` (INFORMATION, not on the
   gate's list) → exit 0, no output. `git diff deploy/styrman-ctl` shows
   one hunk touched, `@@ -449,6 +449,16 @@ cmd_install() {`; since shellcheck
   on the whole file is clean, the touched lines carry no warning either.

6. `(cd deploy/caddy/ingress && go test ./... -count=1)` → exit 0.
   `ok github.com/spgo/devbox/caddy/ingress 2.024s`.

7. Secret-shape guard:
   - `grep -n 'GH_TOKEN=' deploy/gcp/install.sh` → no output, rc=1 (not
     found) — confirms the `sudo env GH_TOKEN=…` step is gone from
     install.sh as described.
   - `grep -n 'sudo env GH_TOKEN' deploy/gcp/*.sh` → no output, rc=1.
   - `git diff --numstat deploy/styrman-ctl deploy/host-setup.sh deploy/verify-commit.sh`
     → `25 6 deploy/host-setup.sh`, `10 0 deploy/styrman-ctl`,
     `3 3 deploy/verify-commit.sh` — all numeric (text), no `-`/`-` binary
     markers.
   - `file deploy/gcp/* internal/config/gcp_scripts_test.go` → all report
     "Unicode text, UTF-8 text" (install.sh/vm.sh additionally "Bourne-Again
     shell script … executable"); none report "data" or binary.
   - `git status --short --untracked-files=all` → full list recorded below
     under Environment/tree state. No `.env*`, `*.pem`, `*.key`,
     `*.tfvars`, `*.tfstate`, or credential-JSON-shaped path present among
     tracked-modified or untracked files.

Full `git status --short --untracked-files=all`:
```
 M .agents/docs/bugs/INDEX.md
 M .agents/docs/deploy/gcp.md
 M ROADMAP.md
 M deploy/host-setup.sh
 M deploy/styrman-ctl
 M deploy/verify-commit.sh
 M internal/config/styrman_ctl_test.go
?? .agents/docs/bugs/BUG-069-gcp-ssh-outage-misdiagnosed.md
?? .agents/docs/bugs/BUG-070-kata-download-url-stale.md
?? .agents/docs/bugs/BUG-071-mutation-restore-by-sed-corrupted-install-line.md
?? .agents/docs/plans/2026-09-21-gcp-compute-engine-target.md
?? .agents/docs/reviews/2026-09-21-plan-cold-read-gcp-compute-engine-target-round-2.md
?? .agents/docs/reviews/2026-09-21-plan-cold-read-gcp-compute-engine-target-round-3.md
?? .agents/docs/reviews/2026-09-21-plan-cold-read-gcp-compute-engine-target.md
?? .agents/docs/reviews/2026-09-21-review-gcp-compute-engine-target.md
?? .agents/docs/reviews/2026-09-21-security-gcp-compute-engine-target.md
?? .agents/docs/testing/2026-09-21-verify-gcp-compute-engine-target-phase-1-round-2.md
?? .agents/docs/testing/2026-09-21-verify-gcp-compute-engine-target-phase-1-round-3.md
?? .agents/docs/testing/2026-09-21-verify-gcp-compute-engine-target-phase-1.md
?? .agents/docs/testing/guide.md
?? deploy/gcp/install.sh
?? deploy/gcp/lib.sh
?? deploy/gcp/vm.conf
?? deploy/gcp/vm.sh
?? internal/config/gcp_scripts_test.go
```
(this round-3 log itself appears as untracked, written mid-run.)

## Root cause

N/A — no FAIL.

## Coverage of changed code

- `deploy/gcp/install.sh` (step 5 rewrite: token export removed, verify-tag
  added) — covered by `internal/config/gcp_scripts_test.go`
  (`TestGCPInstallPlan`, new assertions).
- `deploy/gcp/vm.sh` (`block-project-ssh-keys=TRUE`) — covered by
  `internal/config/gcp_scripts_test.go` (`TestGCPVMCreatePlan`, new
  assertion).
- `deploy/styrman-ctl` (`cmd_install` GH_TOKEN export) — covered by
  `internal/config/styrman_ctl_test.go`
  (`TestStyrmanCtlInstallExportsTokenForHostSetup`, new).
- `deploy/host-setup.sh`, `deploy/verify-commit.sh` — modified per the
  dispatch's description but no new test names were called out for these
  two files specifically beyond the shared scripts above; `bash -n` and
  shellcheck both clean on them, and they run inside the same
  `TestGCPInstallPlan`/`TestStyrmanCtlInstall*` plan-assertion tests where
  relevant (these tests assert planned command strings, not live execution
  of host-setup.sh itself — no unit test executes host-setup.sh's shell
  logic; that only runs on real Kata hardware, see Not examined).
- No added `//nolint`, `//lint:ignore`, `@ts-ignore`, `@ts-expect-error`,
  `eslint-disable`, `t.Skip`, `t.SkipNow`, `//go:build ignore`, `.skip(` or
  `.only(` found in the diff (not explicitly grepped line-by-line beyond
  reading the full diffs of the shell/Go files touched; see Not examined).

## Not examined

- The fresh-host Kata path of `host-setup.sh` (real hardware only).
- The real-gcloud/IAP/ssh paths in `deploy/gcp/*.sh` (require live GCP
  credentials and a running VM; not exercised here).
- The real `git verify-tag` step on the VM (requires a live checkout with
  a signed tag and gpg trust setup).
- The real `GH_TOKEN` export from `--gh-token-file` actually reaching
  `host-setup.sh` step 7 in a live run (unit tests assert the planned
  command string only; only a real bring-up after a release exercises it
  end to end).
- `web/` Tier 1 (`npm run check && npm run lint && npm test`) — not in the
  dispatch's 8-command list, not run this round.
- Tier 2+ suites (`deploy/vulncheck.sh`, `integration.sh`, `web/e2e/run.sh`,
  `deploy/verify-commit.sh` as a release gate) — not in the dispatch's list,
  not run this round.
- A line-by-line grep of the diff for added suppression/skip directives
  was not run as a separate explicit command; the full diffs were read for
  content but no dedicated `grep -n '//nolint\|t.Skip\|@ts-ignore\|eslint-disable'`
  pass was made over the changed files this round.

## Environment

OS: Linux 6.12.107+deb13-amd64. `go version`, `shellcheck` 0.10.0 (from
version check above). HEAD at start: `44a88e2`, working tree dirty per
`git status --short` above (matches the dispatch's described diff; no
unexpected paths).
