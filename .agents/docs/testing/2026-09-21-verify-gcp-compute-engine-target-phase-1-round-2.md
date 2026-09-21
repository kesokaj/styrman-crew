---
owner: verifier
date: 2026-09-21
---
# Verification — gcp-compute-engine-target-phase-1-round-2   (2026-09-21)

Range reviewed: live working tree (uncommitted, base 44a88e2+dirty), since
round 1 (`.agents/docs/testing/2026-09-21-verify-gcp-compute-engine-target-phase-1.md`,
PASS): `deploy/gcp/gcp.env` renamed `deploy/gcp/vm.conf`; `deploy/gcp/lib.sh`
gained `SSH_ARGS`/`ssh_reachable`/`ssh_hint` + IAP tunnelling; `deploy/gcp/vm.sh`
and `deploy/gcp/install.sh` use them; `internal/config/gcp_scripts_test.go`
gained `TestGCPSSHUnreachableIsNamed` (11 `TestGCP*` tests total);
`deploy/host-setup.sh` step 2 (Kata) changed to `.tar.zst` asset,
`tar --zstd`, digest from pinned `KATA_SHA256_3_31_0` or the GitHub release
API via python3.

## Status: PASS

Scope: complete for the dispatched checklist (10 items) — Tier 1 unit
commands, the change's own script syntax/lint checks, the caddy ingress
submodule tests, a rename/ignore check, a binary-content guard, and a
read-only Kata-digest cross-check against the real GitHub release API.
`.agents/docs/testing/guide.md` exists (written round 1) and was followed.
Tier 2+ (integration, e2e, vulncheck, a real gcloud run) out of scope, as in
round 1 — listed under Not examined.

## Commands

1. `go vet ./...` → exit 0
2. `go test ./... -count=1` → exit 0; 15 packages `ok`, 0 `FAIL`; full output
   `.agents/scratch/verify-gcp-round2-gotest.txt`
3. `go test ./internal/config/ -run 'TestGCP' -count=1 -v` → exit 0; 11
   `--- PASS`, 0 FAIL/SKIP:
   TestGCPVMCreateNeedsYesWithoutTerminal, TestGCPVMCreatePlan,
   TestGCPVMDeleteRefusesWithoutConfirm, TestGCPVMRejectsBadNames,
   TestGCPInstallPlan, TestGCPSSHUnreachableIsNamed,
   TestGCPInstallCloneHelperShape, TestGCPInstallRefusesLooseEnvFile,
   TestGCPInstallNeedsRunningVM, TestGCPInstallNeedsKVM,
   TestGCPVMStatusExitCodes — matches the dispatch's expected count (was 10
   in round 1, +1 `TestGCPSSHUnreachableIsNamed` for the new
   `ssh_reachable`/`ssh_hint` behaviour)
4. `bash -n deploy/gcp/vm.sh deploy/gcp/install.sh deploy/gcp/lib.sh
   deploy/host-setup.sh deploy/verify-commit.sh` → exit 0
5. `shellcheck -S warning deploy/gcp/*.sh deploy/host-setup.sh` → exit 0, no
   output
6. `(cd deploy/caddy/ingress && go test ./... -count=1)` → exit 0;
   `ok  github.com/spgo/devbox/caddy/ingress  2.024s`
7. `git check-ignore -v deploy/gcp/vm.conf` → exit 1, no output (not
   ignored, as expected — `vm.conf` does not match the `*.env` ignore
   rule the old `gcp.env` name did). `ls deploy/gcp/` →
   `install.sh lib.sh vm.conf vm.sh` — `gcp.env` is gone, `vm.conf` present.
8. `git diff --numstat deploy/host-setup.sh deploy/verify-commit.sh` →
   `25  6  deploy/host-setup.sh` / `3  3  deploy/verify-commit.sh` — both
   text (no `-` in either column). `file deploy/gcp/* internal/config/gcp_scripts_test.go`
   → all report "... text" (Bourne-Again shell script / Unicode text, UTF-8
   text / Nim source code, Unicode text, UTF-8 text for the `.go` file —
   `file`'s heuristic mislabels the Go source as "Nim" by content sniffing,
   but it is still UTF-8 text, not "data"/binary); none binary.
9. Kata digest cross-check: `grep -n '^KATA_SHA256_3_31_0=' deploy/host-setup.sh`
   → `20:KATA_SHA256_3_31_0=68c2786a0b97023f62f3eca02dc868b78a794e5469d4ddd6cc9e0bd4a7212b0b`.
   `gh api repos/kata-containers/kata-containers/releases/tags/3.31.0 --jq
   '.assets[] | select(.name=="kata-static-3.31.0-amd64.tar.zst") |
   .digest'` → `sha256:68c2786a0b97023f62f3eca02dc868b78a794e5469d4ddd6cc9e0bd4a7212b0b`.
   The two hex strings match exactly (case-insensitive-identical, both
   lowercase). The pinned constant is current for the 3.31.0 release's
   amd64 `.tar.zst` asset.

## Root cause
(n/a — no failures)

## Coverage of changed code

- `deploy/gcp/vm.conf` (renamed from `gcp.env`) — data/env file, no script
  logic; the rename itself is exercised by item 7 (ignore-rule check) and
  item 8 (text check); referencing scripts covered below.
- `deploy/gcp/lib.sh` (new `SSH_ARGS`/`ssh_reachable`/`ssh_hint`, IAP
  tunnelling) — covered by `bash -n`/`shellcheck` (items 4/5) plus indirect
  exercise via `vm.sh`/`install.sh` under every `TestGCP*` test; the new
  `ssh_reachable`/`ssh_hint` unreachable-SSH path is covered directly by
  the new `TestGCPSSHUnreachableIsNamed` (item 3). The IAP tunnelling
  branch itself (`gcloud compute start-iap-tunnel` invocation) is not
  exercised by any test — it only runs against a live GCP project; see Not
  examined.
- `deploy/gcp/vm.sh`, `deploy/gcp/install.sh` (updated to use the new
  lib.sh helpers) — covered by the full `TestGCP*` suite (item 3) plus
  `bash -n`/`shellcheck` (items 4/5); the ssh-reachability integration
  specifically by `TestGCPSSHUnreachableIsNamed`.
- `internal/config/gcp_scripts_test.go` (new `TestGCPSSHUnreachableIsNamed`)
  — the test file itself; exercised by running it (item 3), confirmed a
  text file (item 8).
- `deploy/host-setup.sh` step 2 (Kata `.tar.zst`/`tar --zstd`/digest
  source) — covered by `bash -n`/`shellcheck` (items 4/5, syntax/lint only)
  and the read-only digest cross-check (item 9), which confirms the pinned
  constant is correct today; no test in this repo runs `host-setup.sh`
  step 2 end-to-end (it needs a fresh host without `/opt/kata`) — see Not
  examined, same gap as round 1.
- `deploy/verify-commit.sh` — unchanged in this round's diff beyond what
  round 1 already covered (`bash -n`, item 4); not run end-to-end here
  either, same as round 1.

No changed source file in this round's diff lacks a covering
syntax/lint/unit check; the gaps below are all live-network/live-host paths
that were also out of scope in round 1.

## Not examined

- The real `gcloud`/`gh`/`ssh` execution paths of `vm.sh`, `install.sh`,
  `lib.sh`'s IAP tunnelling, and `host-setup.sh`'s Kata download-and-verify
  step against a live GCP project or a fresh host without `/opt/kata` — all
  ten (now eleven) `TestGCP*` tests are dry-run/plan-mode or early-refusal
  tests, per round 1's log; that gap is unchanged in this round.
- `deploy/host-setup.sh` step 2's fresh-host code path specifically: it
  only runs when `/opt/kata` is absent, i.e. the next real GCP bring-up
  after this release — not exercised by any test in this repo today, and
  item 9 only validated the pinned digest constant, not the download/verify
  code path that consumes it.
- Tier 2 integration (`.agents/docs/testing/styrman/integration.sh`),
  browser e2e (`web/e2e/run.sh`), and `bash deploy/vulncheck.sh` — out of
  scope for this dispatch, as in round 1.
- `deploy/verify-commit.sh` end-to-end (the full release-gate script) — not
  run in either round; only its `bash -n` syntax check.

## Environment

Linux 6.12.107+deb13-amd64. `go version` not separately captured this round
(unchanged from round 1's environment, no toolchain edits in this diff).
`shellcheck` and `gh` invoked from PATH successfully (items 5 and 9), so
both are present and working. `git rev-parse --short HEAD` at run time:
44a88e2 (dirty — 16 modified/untracked paths per `git status --short`, as
listed in the pre-flight gitStatus context).
