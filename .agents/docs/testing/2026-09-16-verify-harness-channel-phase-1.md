---
owner: verifier
date: 2026-09-16
---
# Verification — harness-channel-phase-1   (2026-09-16)

Range reviewed: 567b81f..worktree

## Status: PASS

Scope: complete — all six requested steps ran against the working tree (deploy/update/styrman-update, deploy/styrman-ctl, deploy/host-setup.sh, the two new units, the two new Go test files) on this host, which has docker, both pinned images, and all shell tools.

## Commands

1. `go build ./...` → exit 0.
   `go vet ./...` → exit 0.
   `gofmt -l internal/ cmd/` → empty output, exit 0.

2. `go test ./internal/config/ -run 'StyrmanHarness|StyrmanCtl' -count=1 -v`
   → exit 0. Output: `.agents/scratch/verify-harness-channel-phase-1/go-test-1.txt`.
   counts: pass 18, fail 0, skip 0 at 567b81f+dirty for `go test ./internal/config/ -run 'StyrmanHarness|StyrmanCtl' -count=1 -v`.
   vs previous: none (first run of this pattern in `testing/`).
   Quoted lines:
   ```
   --- PASS: TestStyrmanHarnessChannel (7.89s)
   --- PASS: TestStyrmanCtlUpdateSetupHarnessPlan (0.08s)
   --- PASS: TestStyrmanCtlUpdatePlan (0.30s)
   --- PASS: TestStyrmanCtlUpdateSetupPlan (0.42s)
   ```
   No `--- SKIP` line anywhere in the output — `docker`, `ssh-keygen`, `bash`, `git`,
   `diff`, `sha256sum` and both images (`styrman-runtime:test`,
   `golang:1.25.13-trixie@sha256:6be3...` — the `harness-builder` FROM pin in
   `image/Dockerfile`) were present, so `TestStyrmanHarnessChannel` ran for real
   (walk-down, alarms, downgrade refusal, smoke, swap) rather than skipping.
   Every other `Test.*(StyrmanHarness|StyrmanCtl).*` the pattern matches also
   PASSed (18/18); full list in the scratch log.

3. `go test -race ./... -count=1` (whole module) → exit 0 (background run;
   log `.agents/scratch/verify-harness-channel-phase-1/go-test-race.txt`,
   15 `ok` lines, 0 `FAIL`/panic lines, tail: `ok github.com/kesokaj/styrman-crew/internal/userstore 1.010s`, `web [no test files]`).
   `(cd deploy/caddy/ingress && go test ./... -count=1)` → exit 0:
   `ok github.com/spgo/devbox/caddy/ingress 2.024s`.

4. `bash -n deploy/update/styrman-update` → exit 0.
   `bash -n deploy/styrman-ctl` → exit 0.
   `bash -n deploy/host-setup.sh` → exit 0.
   `shellcheck -S warning deploy/update/styrman-update deploy/styrman-ctl deploy/host-setup.sh` → no output, exit 0.
   `systemd-analyze verify deploy/update/styrman-harness-update.service deploy/update/styrman-harness-update.timer` → no output, exit 0 (systemd 257 on this host; no unknown-key warnings appeared).

5. Coverage map (changed/new files → covering test):
   - `deploy/update/styrman-update` (harness mode: `harness_select`, `harness_alarm`, `harness_export`, `harness_smoke`, swap, state files, test-mode guards) — covered by `internal/config/styrman_harness_test.go::TestStyrmanHarnessChannel` (drives the real script end to end via `exec.Command`, scratch dirs, scratch signing key) and, for the `styrman-ctl`-invoked planning path, `internal/config/styrman_ctl_harness_test.go::TestStyrmanCtlUpdateSetupHarnessPlan`.
   - `deploy/styrman-ctl` (`setup_auto_update` harness-token prompt, `/opt/styrman-harness` clone, content dir, timer enable, `status` harness line) — covered by `TestStyrmanCtlUpdateSetupHarnessPlan`; the existing `TestStyrmanCtlUpdatePlan`/`TestStyrmanCtlUpdateSetupPlan` still pass unchanged, evidence the refactor (adding the third token param) did not disturb the crew-only paths.
   - `deploy/host-setup.sh` (creates `/var/lib/styrman/harness` content dir) — **NOT covered by any test tier reached in this run** (root-only, no unit/integration test invokes it). This is a documented gap, not a hidden one: the plan's own verify clause for this box is manual (`styrman-ctl --dry-run update --setup` + a live-host `sudo -n styrman-ctl status` after deploy), scheduled for release/e2e, not Phase-1 automated tests.
   - `deploy/update/styrman-harness-update.service` / `.timer` — covered by `systemd-analyze verify` (syntax/semantics) and, structurally, by a grep confirming `self_install`'s file list names both (`deploy/update/styrman-update:748`) and `host_converge` enables the timer unconditionally on `-f`/`-d` existence, not gated on a `changed` flag (`deploy/update/styrman-update:871-873` — BUG-035 compliant; comment at 865-869 cites BUG-035 by name). No dedicated Go/shell test exercises `self_install`/`host_converge` end to end for the harness unit pair in this run — same documented gap as host-setup.sh (root-only, no test tier).
   - `internal/config/styrman_harness_test.go`, `internal/config/styrman_ctl_harness_test.go` — new test files, self-covering (they are the tests).

   Test count: `git show HEAD:internal/config/styrman_update_test.go | grep -c '^func Test'` → 1 (unchanged in the working tree — `git diff --stat HEAD -- internal/config/styrman_update_test.go` is empty). Working-tree `internal/config/*_test.go` total: 36 funcs across 8 files (`config_test.go` 9, `ipforward_test.go` 2, `bridgename_test.go` 4, `styrman_ctl_harness_test.go` 1 [new], `styrman_harness_test.go` 1 [new], `styrman_ctl_test.go` 16, `styrman_update_test.go` 1 [unchanged], `firewall_script_test.go` 2). Net: +2 new tests, 0 removed.

6. Positive control: `git show HEAD:deploy/update/styrman-update > .agents/scratch/.../updater.head.sh; STYRMAN_UPDATE_TEST=1 bash <that file> harness --check` → exit 2, stderr `unknown flag: harness` — confirms the `harness` verb did not exist at HEAD (567b81f) and the new test in step 2 is exercising genuinely new code, not a no-op. Scratch file deleted afterward (`rm -f .agents/scratch/verify-harness-channel-phase-1/updater.head.sh`); confirmed gone.

## Suppressions added (diff scan)
`git diff 567b81f -- deploy/ internal/config/` (plus a status check on the two new untracked files) for `nolint|lint:ignore|t.Skip|go:build ignore|.only(|.skip(`: the only hits are the two `t.Skip`f calls in the new `styrman_harness_test.go` (lines 26, 42), both carrying a reason on the same line (missing tool / missing image) — not bare skips. No `.only(`, no unreasoned suppression.

## Coverage of changed code
See §5 above. One uncovered surface, documented as expected: `deploy/host-setup.sh`'s new content-dir creation lines — root-only, no automated test tier in Phase 1 (the plan's own verify clause defers this box to a live-host `--dry-run` + post-deploy `status` check).

## Not examined
- The plan's other verify clauses for this box that are explicitly manual/live-host (`styrman-ctl --dry-run update --setup`, `sudo -n styrman-ctl status` on the deployed host) — out of scope for a working-tree verification run, no host was deployed to.
- Phases 2–4 of the plan (server mounts, entrypoint, integration Tier 2, docs/release) — out of scope per the dispatch (Phase 1 only); their source files (`internal/config` `HarnessDir`, `internal/orchestrator` binds, `image/entrypoint.sh`, `integration.sh`) are either absent from this diff or untouched.
- `go test ./... -count=1` without `-race` was not run separately; the `-race` run (step 3) supersedes it and is the stronger check.

## Environment
Linux 6.12.107+deb13-amd64, host `dave`. `go version` inferred from module (not queried directly, build/vet/test all succeeded so toolchain is compatible). Docker 29.7.2 (Colima not applicable — native Linux Docker). `systemd-analyze` 257-class (no unknown-key warnings). `shellcheck` 0.10.0. Images present locally: `styrman-runtime:test`, `golang:1.25.13-trixie@sha256:6be3daa12135c38c177e2fbbf38ef2ad000d09b34bfa9f73dd7ca785bff134a5`.
