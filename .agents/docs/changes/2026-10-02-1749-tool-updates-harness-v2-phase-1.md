---
owner: main
date: 2026-10-02
---
# In-project tool updates, harness v2.3.2 behind ready — Phase 1   (2026-10-02)

Phase 1 of `plans/2026-10-02-tool-updates-freeze-harness-v2.md`. The operator
asked for three things — the runtime image on current versions and a look at
why `claude`/`agy` cannot be updated inside a project; a fix for crew freezing
until the window is reloaded; harness v2.3.2 in the image. This phase is the
image; the freeze is Phase 2.

## What changed

**Pins** (`image/Dockerfile`, `deploy/Dockerfile.server`): Node 22.23.3,
Claude Code 2.1.287, OpenTofu 1.13.1, yq 4.54.1, chrome-devtools-mcp 1.10.1,
both Debian base digests (`image/bump-pins.sh --apply` for the ones it reads;
OpenTofu's and yq's sums are the publishers' GitHub release-asset digests, and
OpenTofu's also matches its published `SHA256SUMS`). Go 1.27.1 and hadolint
2.15.1 were current.

**claude and agy update themselves in a project** (ADR-077, BUG-080). `agy
update` could not write beside its binary in root's `/usr/local/bin`; Claude's
updater had no npm prefix to write to, because `~/.npm-global` did not exist in
a fresh home. agy now lives at `/opt/agy/agy` (the directory 0777) behind the
`/usr/local/bin/agy` symlink, and the entrypoint creates
`~/.npm-global/{bin,lib}` on every non-minimal boot.

**Harness v2.3.2, installed after ready** (ADR-078). The delivery is ADR-074's,
unchanged: `ARG HARNESS_VERSION=v2.3.2`, the signed release binary. The
operator's link was the release's zip; the same release carries the binary the
image already verifies against the pinned signer, so the zip was not used. The
v2 harness is 1,788 files and `sh-agent install` syncs each one: 12.8 s in a
microVM on a disk-backed home, where v1.6.0 took 0.02 s. In front of
`/run/styrman/ready` that measured 16.16 s and 16.40 s boot-to-ready against
the 15 s hard limit. The entrypoint now runs the migration, the never-downgrade
decision and the two MCP registrations before ready, as before, and the
install after it, in the background, under a 120 s cap;
`/run/styrman/harness-done` marks the end of the harness step. Boot-to-ready
on disk afterwards: 3.01 s and 3.20 s.

**The integration suite measures on disk** (BUG-082, plan D12). Its state dir
was under `/tmp`, tmpfs on this host, where the same 16-second boot read
3.26 s and the suite passed. It is under `/var/tmp` now, the boot-budget check
names the filesystem it measured on and fails on tmpfs, `wait_seed` waits for
`harness-done` before harness assertions, and six checks were added (121).

**The bridge answers a ping** (ADR-079, the server half; plan D17).
`{"type":"ping"}` → `{"type":"pong"}`, written under a 5 s bound, and a ping
is not activity for the idle reaper. The page that uses it is Phase 2.
`wsprobe -control` sends a text frame for the suite's check.

**`styrman-ctl install` writes an optional key the answers file carries**
(BUG-081, plan D18). Found by this phase's Tier 1: two Go tests had been red
since v1.39.0, and one was right — `CLOUDFLARE_API_TOKEN` left the must-fill
list in `167f605`, the loop that fills `.env` reads that list only, and a
token in the `--env-file` was dropped without a word. `OPTIONAL_FILL` writes
it when given, validated, never prompted for. The other red test asserted GNU
`stat` text in a file that had been made portable on purpose; it now asserts
the difference.

## Files
- `image/Dockerfile`, `deploy/Dockerfile.server`, `image/entrypoint.sh`
- `internal/terminal/bridge.go`, `internal/config/config.go` (a comment), `cmd/wsprobe/main.go`
- `deploy/styrman-ctl`, `deploy/gcp/install.sh` (two comments)
- tests: `internal/config/entrypoint_seed_order_test.go`, `internal/config/styrman_ctl_optional_fill_test.go`, `internal/terminal/control_test.go` (new); `internal/config/gcp_scripts_test.go`, `internal/config/styrman_ctl_test.go` (edited); `.agents/docs/testing/styrman/integration.sh`
- docs: `specs/styrman/runtime-image.md`, `specs/styrman/deploy.md`, `specs/styrman/terminal-protocol.md`, `testing/styrman/README.md`, `deploy/gcp.md`, `image/README.md`, `README.md`, `deploy/README.md`
- records: ADR-077, ADR-078, ADR-079; BUG-080 to BUG-084; the plan and its cold-read report

## Agents dispatched
- `reviewer`, three times, as the plan's cold reader (plan only): `reviews/2026-10-02-review-plan-tool-updates-freeze-harness-v2.md`. Round 3 (the cap): `implementation ready`, `design needs revision` on three narrow notes; the operator chose "Proceed with all three (Recommended)", recorded as D12, D14, D15.
- `documentation` (the eight docs above), `bugs` twice (BUG-080 to BUG-082; BUG-083, BUG-084).
- `verifier`: `testing/2026-10-02-1744-verify-tool-updates-freeze-harness-v2-phase-1.md`, `## Status: PASS`, over the working tree on top of `1fe28a0`.
- Code review and security review of the diff run once over the whole change, at the plan's Phase 3.

## Verification
The verifier's log, every command bare with its exit status:
- `go build ./...`, `go vet ./...` → 0; `go test -v -count=1 ./...` → 0 (pass 392, fail 0, skip 1); the Caddy ingress module → 0 (pass 60).
- `web`: `npm run check` → 0, `npm run lint` → 0, `npm test` → 0 (15 files, 178 tests).
- `docker build -f deploy/Dockerfile.server` → 0; the runtime image rebuilt by the suite's `ensure-test-image.sh`.
- `bash .agents/docs/testing/styrman/integration.sh styrman-runtime:test` → 0: `passed=118 failed=0 skipped=3 aborted=0` (the three SKIPs are the host-firewall previews); `PASS boot to ready within R1's 15 s hard limit (measured 3.23s on ext2/ext3; soft target 5 s)` — `stat` names ext4 that way.
- Go test names: 330 at `1fe28a0`, 334 in the tree; no `func Test` line removed.
- Not run in this phase: Tier 4 (no web source changed), `vulncheck.sh`, `verify-commit.sh`.

Red before green, by hand (logs in `.agents/scratch/`, not committed):
- The suite with all new checks, against the image before the entrypoint and
  agy changes and the server before the pong: the five checks this phase
  exists for failed, and the boot-budget check failed at 16.40 s. That run did
  not finish: the main agent edited `integration.sh` while bash was still
  executing it, and it ended in a syntax error with `aborted=1` after 107
  passes (BUG-083). The six failures are in the part that ran; the rest of the
  suite's passes come from the later full runs.
- `TestStyrmanCtlInstallPlan` and `TestGCPLibEnvGetMatchesStyrmanCtl` fail in
  a worktree of `1fe28a0` and pass at `v1.38.0`.
- Mutations, each restored from a saved copy: `harness_finish &` without the
  `&`, and the marker-clearing line without `harness-done`, each fail
  `TestEntrypointInstallsHarnessAfterReady`; `classifyControl` returning
  activity for a ping, or no reply, each fail `TestClassifyControl`; the
  optional key's `validate … || die` line removed fails
  `TestStyrmanCtlInstallOptionalKeyInvalidStops`.

Hand run, inside a real booted project of the rebuilt image (a locally run
server, scratch state under `/var/tmp`, `docker exec` as the project user in
an interactive login shell): with agy 1.2.2 and claude 2.1.270 planted, `agy
update` → `Update successful!`, 1.2.14 in `/opt/agy`; `claude update` →
`Successfully updated from 2.1.270 to version 2.1.287`, and a new login shell
ran 2.1.287 from `~/.npm-global` with `claude doctor` printing `No
installation issues found`. The planted agy was owned by the project user,
not root as the image ships it; the root-owned case was seen once, in a probe
of the 1.38.0 image with the same directory mode.

Not covered: the `agy`/`claude` downloads in any suite; `harness_finish`'s
failure branches (a timed-out install) in a booted VM; the idle reaper
actually stopping a project under a pinging tab (the unit test proves a ping
is not counted).

## Learnings for the local host
- The shared path changed in two places a fresh host exercises:
  `styrman-ctl install` (the optional key) and the runtime image build. The
  install change is proven by its dry-run tests only — fresh-host path
  unexercised: no new VM was created for this phase.
- This host's auto-updater has been failing since before v1.39.0 reached it:
  `/etc/styrman/gh-token` is rejected by GitHub, so the host still runs
  1.38.0 (BUG-084). The operator installs a fresh token; no root step was run
  by the agent.
