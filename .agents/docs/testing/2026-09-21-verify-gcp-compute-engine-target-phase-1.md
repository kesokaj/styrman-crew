---
owner: verifier
date: 2026-09-21
---
# Verification — gcp-compute-engine-target-phase-1   (2026-09-21)

Range reviewed: live working tree (uncommitted) — new files `deploy/gcp/gcp.env`,
`deploy/gcp/lib.sh`, `deploy/gcp/vm.sh`, `deploy/gcp/install.sh`,
`internal/config/gcp_scripts_test.go`; edit to `deploy/verify-commit.sh` lines
168-171 adding `deploy/gcp/*.sh` to its `bash -n` and shellcheck lists.

## Status: PASS

Scope: Tier 1 unit commands plus the change's own script syntax/lint checks
and a test-count guard; Tier 2+ (integration, e2e, vulncheck) not run —
out of scope for this dispatch (Phase 1 dry-run scripts only, no cloud
calls). `.agents/docs/testing/guide.md` written this run (was absent).

## Commands

- `go vet ./...` → exit 0
- `go test ./... -count=1` → exit 0; all 15 packages `ok` (no FAIL); full output .agents/scratch/verify-gcp-compute-engine-target-phase-1-gotest2.txt
- `go test ./internal/config/ -run 'TestGCP' -count=1 -v` → exit 0; 10 tests, all `--- PASS`, no FAIL/SKIP: TestGCPVMCreateNeedsYesWithoutTerminal, TestGCPVMCreatePlan, TestGCPVMDeleteRefusesWithoutConfirm, TestGCPVMRejectsBadNames, TestGCPInstallPlan, TestGCPInstallCloneHelperShape, TestGCPInstallRefusesLooseEnvFile, TestGCPInstallNeedsRunningVM, TestGCPInstallNeedsKVM, TestGCPVMStatusExitCodes
- `bash -n deploy/gcp/vm.sh deploy/gcp/install.sh deploy/gcp/lib.sh deploy/verify-commit.sh` → exit 0
- `shellcheck -S warning deploy/gcp/*.sh` → exit 0, no output
- `(cd deploy/caddy/ingress && go test ./... -count=1)` → exit 0; `ok  github.com/spgo/devbox/caddy/ingress  2.025s`
- Test-count guard: `git stash list` empty; `git diff --stat` shows only `.agents/docs/deploy/gcp.md` (128 changed), `ROADMAP.md` (3 changed), `deploy/verify-commit.sh` (6 changed) as modified tracked files — no other tracked source file touched; `git diff --numstat deploy/verify-commit.sh` → `3\t3\tdeploy/verify-commit.sh` (3 insertions/3 deletions, matching the two edited lines' before/after, no unrelated deletion); `file deploy/gcp/*.sh internal/config/gcp_scripts_test.go` → all report "Bourne-Again shell script"/"Unicode text, UTF-8 text" or "ASCII text", none "data"

## Root cause
(n/a — no failures)

## Coverage of changed code

- `deploy/gcp/vm.sh` — covered by `internal/config/gcp_scripts_test.go` (TestGCPVMCreateNeedsYesWithoutTerminal, TestGCPVMCreatePlan, TestGCPVMDeleteRefusesWithoutConfirm, TestGCPVMRejectsBadNames, TestGCPVMStatusExitCodes) plus `bash -n` and `shellcheck -S warning`
- `deploy/gcp/install.sh` — covered by `internal/config/gcp_scripts_test.go` (TestGCPInstallPlan, TestGCPInstallCloneHelperShape, TestGCPInstallRefusesLooseEnvFile, TestGCPInstallNeedsRunningVM, TestGCPInstallNeedsKVM) plus `bash -n` and `shellcheck -S warning`
- `deploy/gcp/lib.sh` — covered indirectly (sourced by vm.sh/install.sh under every test above) plus `bash -n` and `shellcheck -S warning`; no test targets it directly by name
- `deploy/gcp/gcp.env` — data/env file, no script logic; syntax n/a, not shellchecked (correctly, per the change's own list — only `*.sh` files)
- `internal/config/gcp_scripts_test.go` — the test file itself; exercised by running it (item 3)
- `deploy/verify-commit.sh` lines 168-171 — the two-line addition of `deploy/gcp/*.sh` to its own `bash -n`/shellcheck lists is exercised structurally by items 4 and 5 reproducing what that addition causes `verify-commit.sh` to run, but `verify-commit.sh` itself was NOT run end-to-end in this dispatch (that is the full release-gate script, out of scope for a Phase-1 script-addition check); its own test suite is `bash -n` (item 4, passes) and no unit test covers the list-membership change directly — a plain grep confirms `deploy/gcp/*.sh` appears in both the `bash -n` and shellcheck loops (visible in the diff, not independently asserted by a test)

## Not covered (explicitly, per dispatch item 8)

All ten `TestGCP*` tests in `internal/config/gcp_scripts_test.go` are DRY-RUN / plan-mode tests (they assert the printed `+ (dry run) would ...` plan or an early refusal, using a fake `gcloud`/`ssh` stub or no real network access at all). None of the following code paths — which only execute when a script is run for real against a live GCP project — are exercised by any test in this repository today:

- `vm.sh create`'s ssh-readiness wait loop (`vm.sh:83-93`, up to 5 minutes polling `vm_ssh true` until it succeeds or `die`s)
- The real `sudo styrman-ctl status` relay over ssh (`vm.sh:105-109`)
- `install.sh` step 6's curl retry/redirect probe against the live app (`install.sh:156-161`)
- `install.sh`'s `cleanup` trap on a REAL failure mid-run (`trap cleanup EXIT INT TERM HUP`, `install.sh:123`) — the trap's registration is structurally present (bash -n/shellcheck pass) but no test forces a failure partway through a real run to observe the trap firing and actually removing `~/.styrman-install` on a live VM
- The real `gcloud compute instances create/delete`, `gcloud compute scp`, and `gcloud compute addresses create/delete` calls themselves (only their argument construction is checked, via the dry-run plan output)

These are explicitly named in this dispatch as Phase 2's cloud-smoke scope, not a FAIL here.

## Not examined

- Tier 2 integration (`.agents/docs/testing/styrman/integration.sh`), Tier 4 e2e (`web/e2e/run.sh`), `bash deploy/vulncheck.sh`, and `deploy/verify-commit.sh` run end-to-end — all out of scope for this dispatch (Phase 1: script syntax/lint/unit only, no cloud calls, no full release gate)
- The frontend suite (`cd web && npm run check && npm run lint && npm test`) — not touched by this change set (no `web/` files in the diff) and not run
- Real-network paths of `deploy/gcp/vm.sh` and `deploy/gcp/install.sh` — see "Not covered" above

## Environment

Linux 6.12.107+deb13-amd64, working tree at commit 44a88e2 + uncommitted
changes listed above.
