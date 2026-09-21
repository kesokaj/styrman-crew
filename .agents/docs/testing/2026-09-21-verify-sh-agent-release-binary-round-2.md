---
owner: verifier
date: 2026-09-21
---
# Verification — sh-agent release binary, round 2 (review/security fixes)   (2026-09-21)

Range reviewed: working tree (dirty, ~202 changed paths) at HEAD 589f424, vs
predecessor's run recorded in
`.agents/docs/testing/2026-09-21-verify-sh-agent-release-binary.md` (Status: PASS).
Delta named by dispatch: `image/entrypoint.sh` (harness_home_newer, timeout 5
manifest read, "kept" pass line), `deploy/update/styrman-update` (TRUST_UID=0,
hook `--check`), `deploy/host-setup.sh`, `deploy/styrman-ctl`,
`deploy/verify-commit.sh`, `.agents/docs/testing/styrman/ensure-test-image.sh`
(token handling), `.agents/docs/testing/styrman/integration.sh` (one new
check), two Go test files.

## Status: PASS

Scope: complete — all six dispatched steps ran. `testing/guide.md` was not
checked/written this round (out of scope for this dispatch; predecessor's
run is the project's normal verification path and this is a follow-up delta
check on named files only).

## Commands
1. `gofmt -l internal cmd` → exit 0, no output (clean).
   `go vet ./...` → exit 0, no output.
   `go test -race -count=1 ./...` (root module, backgrounded to
   `.agents/scratch/verify-round2/go-test-race.txt`) → exit 0. All 11
   tested packages report `ok` (cmd/styrman, internal/config,
   conversation, credbroker, credstore, files, gitrepo, history, httpapi,
   identity, links); `cmd/wsprobe` has no test files. 0 `FAIL` lines.
   counts: not applicable per-subtest (run without `-v`, so Go prints only
   package-level `ok`/`FAIL`, not `--- PASS/FAIL/SKIP` lines); 11 of 11
   tested packages `ok`, 0 `FAIL`.
2. `go test -count=1 -run 'TestEntrypointHarnessHomeNewer|TestStyrmanUpdateRetiresHarnessUnits|TestStyrmanCtlInstallPlan' -v ./internal/config/`
   → exit 0. All three named tests appear as `--- PASS`:
   `TestEntrypointHarnessHomeNewer` (0.03s), `TestStyrmanCtlInstallPlan`
   (0.04s), `TestStyrmanUpdateRetiresHarnessUnits` (0.03s).
   counts: pass 3, fail 0, skip 0 at 589f424+dirty for this command.
   vs previous: none (this filtered invocation is new to this round).
3. `bash -n` on all 7 named scripts (image/entrypoint.sh,
   deploy/update/styrman-update, deploy/host-setup.sh, deploy/styrman-ctl,
   deploy/verify-commit.sh, .agents/docs/testing/styrman/ensure-test-image.sh,
   .agents/docs/testing/styrman/integration.sh) → exit 0 for every file (no
   syntax errors).
   `shellcheck -S error <same 7 files>` (shellcheck 0.10.0) → exit 0, no
   output.
4. Tier 2 was re-run by the main agent, not by me. Read (not re-run)
   `.agents/scratch/tier2b.log`: final line `passed=112 failed=0 skipped=3
   aborted=0` — matches the expected `passed=112 failed=0 skipped=3
   aborted=0` exactly. `grep -c 'check "' .agents/docs/testing/styrman/integration.sh`
   → 115, matching the dispatch's "115 checks" note. Found the exact line
   `PASS the left-alone boot logs 'kept', not a new 'deployed' line` at
   tier2b.log:152, and the preceding `PASS boot log says the newer home was
   left alone (no install, ADR-074)` and `PASS manifest planted newer than
   the image (v99.0.0), stamp cleared` — the never-downgrade guard's
   integration coverage is present and green.
5. Test-count comparison, `go test -count=1 -list '.*' ./internal/config/ | grep -c '^Test'`:
   working tree (now) = 36; HEAD 589f424 via detached worktree
   (`.agents/scratch/wt-head*`, added and removed cleanly) = 39.
   Disappeared (present at HEAD, gone in the working tree — all from three
   fully-deleted test files, `internal/config/harnessdir_test.go`,
   `styrman_ctl_harness_test.go`, `styrman_harness_test.go`, per the removed
   harness-channel mechanism, ADR-073 superseded): `TestHarnessDirFromEnv`,
   `TestStyrmanCtlHarnessOriginIsGated`,
   `TestStyrmanCtlStatusLatchedHarnessTagIsDegraded`,
   `TestStyrmanCtlUpdateSetupHarnessPlan`, `TestStyrmanHarnessChannel`. All
   five delete together with the harness-channel code paths they tested
   (git status shows the matching `image/harness/`,
   `deploy/update/styrman-harness-update.{service,timer}` deletions) — this
   matches the dispatch's "deleted on purpose" framing, though the dispatch
   named only two of the five files by test name; the other three
   (`TestHarnessDirFromEnv`, `TestStyrmanCtlHarnessOriginIsGated`,
   `TestStyrmanCtlStatusLatchedHarnessTagIsDegraded`) are the remainder of
   the same three fully-deleted files and are the same deletion, not a
   separate loss.
   Appeared: `TestEntrypointHarnessHomeNewer`,
   `TestStyrmanUpdateRetiresHarnessUnits` — exactly the two the dispatch
   named. `TestStyrmanCtlInstallPlan` (used in step 2) exists at both HEAD
   and now — unchanged by this round, not new.

## Root cause  (FAIL only)
N/A — no failures this round.

## Coverage of changed code
- `image/entrypoint.sh` `harness_home_newer` (new function, "kept" pass
  line, timeout-5 manifest read) — covered directly by
  `internal/config/entrypoint_guard_test.go`'s
  `TestEntrypointHarnessHomeNewer` (extracts the function by its shell
  boundaries and runs it under bash with 9 cases including the security
  review's "unknown image never wins" cases), AND end-to-end by Tier 2's
  never-downgrade sequence (steps around tier2b.log:145-153, the exact
  "kept" line asserted).
- `deploy/update/styrman-update` `TRUST_UID=0` default and the retire-hook's
  `--check` — covered by `internal/config/styrman_update_test.go`'s
  `TestStyrmanUpdateRetiresHarnessUnits` (asserts `--check` reports "would
  remove", changes no unit/state file, and calls no systemctl).
- `.agents/docs/testing/styrman/integration.sh` new check ("the left-alone
  boot logs 'kept', not a new 'deployed' line") — covered by its own
  execution in the Tier 2 run read in step 4 (PASS).
- `deploy/host-setup.sh`, `deploy/styrman-ctl`, `deploy/verify-commit.sh`,
  `.agents/docs/testing/styrman/ensure-test-image.sh` — GH_TOKEN / gh auth
  token / gh-token-file handling (the "one rule every build site follows":
  GH_TOKEN, else gh auth token or a token file, else stop, token passed as
  a docker build --secret and unset from the environment afterward) is
  **NOT covered by any test that exercises the token-selection logic or the
  docker build invocation itself.** Its only coverage this round is
  `bash -n` (parses) and `shellcheck -S error` (no error-level lint
  findings), plus, for `deploy/styrman-ctl` specifically, the
  `TestStyrmanCtlInstallPlan` dry-run plan test — which does not exercise
  the token/build-secret code paths in verify-commit.sh or
  ensure-test-image.sh at all. This gap is reported, not hidden, matching
  the dispatch's own framing of it as expected.

## Not examined
- `testing/guide.md` presence/currency — out of scope for this delta
  dispatch; not checked or written this round.
- No frontend (`web/`) changes were in the named delta; not run.
- The vendored Caddy ingress module (`deploy/caddy/ingress`) was not in the
  named delta and was not re-tested this round.
- `govulncheck` (`deploy/vulncheck.sh`) was not run this round — not named
  in the dispatch.
- The actual docker-build execution path for the token-secret handling
  (would require a real `gh` token / docker build) was not run — see
  Coverage section above.

## Environment
OS: Linux 6.12.107+deb13-amd64. shellcheck 0.10.0. HEAD 589f424, tree dirty
(~202 paths, matches the dispatch's described pending release changes).
Go module: root (`github.com/kesokaj/styrman-crew`).
