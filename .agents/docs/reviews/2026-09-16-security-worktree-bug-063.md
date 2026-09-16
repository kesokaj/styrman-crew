---
owner: security
date: 2026-09-16
---
# Security review — worktree-bug-063   (2026-09-16)

Range reviewed: working tree vs 976b565 — the five changed files and bug063_test.go

Blobs pinned (the tree moved twice during this review — see "Tree moved"):
`orchestrator.go` f161ed3, `docker.go` 763a009, `fakedocker_test.go` df94b10,
`seam_test.go` 7130d0c, `bug063_test.go` f152698, `integration.sh` b9eaf38.

## Verdict: FINDINGS (two low, both assurance-grade; nothing tenant-exploitable)

No weakness in this change set is reachable by a project user or across
tenants. Both new `ContainerUpdate` calls are server-side, addressed by a
server-derived container name (`"styrman-"+<project UUID>`) or the inspected
container ID — exactly the values the pre-existing `ContainerStop`,
`ContainerStart` and `ContainerRemove` calls already use, so the seam addition
reaches no container it could not already stop or destroy. They carry a
restart policy and a zero `Resources`, which moby translates to an empty
resource set (verified below, so the guest's Kata sizing is not reset), and
both sit inside the per-project lock, behind the existing authz path and after
the `projectHomeIsReal` symlink guard (orchestrator.go:341, still ahead of the
switch at :344). What remains is measurement: the stop-path update is
warn-only, and its only detector is a live-Kata Tier 2 assertion made through
`docker inspect`, which reads persisted state that moby can leave disagreeing
with the live restart monitor — so the check can report the opposite of the
behaviour, and no unit test covers the failure path even though the fake
already has the hook for it.

## Findings
- **[low / confidence: medium]** `.agents/docs/testing/styrman/integration.sh:450` and `:465` — both new checks read `docker inspect … {{.HostConfig.RestartPolicy.Name}}`, i.e. **persisted** state. In moby (read at `$GOMODCACHE/github.com/docker/docker@v28.5.2+incompatible/daemon/update.go`), `daemon.update` calls `ctr.UpdateMonitor(newPolicy)` *before* the task update, and the deferred `restoreConfig` rollback on a failed task update restores only `ctr.HostConfig` — the live restart monitor keeps the new policy. Persisted config and live monitor can therefore disagree in either direction until the daemon restarts and rebuilds the monitor from the persisted config. Practically the checks can report the opposite of the behaviour: a stop whose update half-failed behaves correctly (monitor "no") while inspect prints `unless-stopped` and the check fails, and the symmetric case can pass while the monitor is wrong. This is the BUG-034 shape — instrumentation that may not measure the thing it names. Remediation: assert the behaviour once (after the boot=1 revive, `kill -TERM 1` in the guest and check the VM comes back by itself), or state in the check's comment that it asserts the persisted policy only.
- **[low / confidence: high]** `internal/orchestrator/orchestrator.go:1226-1232` — the stop-path update is warn-only, and the component that must honour it is the Kata shim (`runtime mvm`), not runc: on a running container moby routes the update through `tsk.UpdateResources` into the shim, and if the shim rejects it moby rolls the host config back to `unless-stopped` and returns an error. styrman logs a warning and proceeds, and BUG-063's race is exactly as before. The comment says so honestly, and Tier 2's `0 0 no` assertion is the detector — but that detector lives **only** in the live-Kata suite: no unit test covers the failure path, though `fakeDocker` already has the injection hook (`f.fail[method]` in `record`, fakedocker_test.go:79-88). Remediation: one test setting `fd.fail["ContainerUpdate"]` asserting Stop still stops with the warning as the only consequence, so "a failed update leaves the old race for this one stop, nothing worse" is tested rather than asserted. The same test pins the start path's failure mode, whose residual is that an update that keeps failing is retried on every attach (self-healing and bounded — one write per attach while broken — but worth being deliberate about).

## Verified negatives (checked, not findings)
- **Zero `Resources` in `UpdateConfig` do not reset the guest's sizing.** Project VMs are created with `NanoCPUs`/`Memory` from config (orchestrator.go:1067-1070) and Kata sizes the guest from exactly those, so a resource reset here would be a cross-tenant DoS lever. Read in moby v28.5.2 (`daemon/update_linux.go`): for an all-zero `Resources`, `toContainerdResources` emits `nil` CPU, `nil` Memory, `nil` BlockIO, and `getPidsLimit` returns `nil` (`daemon/daemon_unix.go:125`); the persisted merge keeps only non-zero request fields. Nothing is unset. Caveat: the SDK in `go.mod` is v28.5.2 while a code comment cites live Engine 29.7.2 — a divergence there would be a daemon regression, not a styrman one.
- **No amplification on the hot attach path.** `ensureFor` → `wsBoot` → `EnsureRunning` runs on every terminal WebSocket attach (`internal/httpapi/server.go:262-278`, :301), including automatic reconnects, so an unconditional write there would have let an owner drive one host-daemon write + one shim round-trip per reconnect (the family the `ipCache` memo at orchestrator.go:49-52 exists for) and would have made an attach depend on that VM's runtime shim answering while holding the project lock. The reviewed blob guards the write with the inspect already in hand (`insp.HostConfig == nil || insp.HostConfig.RestartPolicy.Name != want.Name`, :356), so the steady state issues no update at all; a nil `HostConfig` falls through to the write, which is the fail-safe direction.
- **The setup VM's `noRestart` is untouched.** The ephemeral onboarding VM is `config.SetupContainer(user)` and starts through its own path (orchestrator.go:707-728), which makes no `ContainerUpdate` call — the "reap, don't resurrect" policy (`no`, orchestrator.go:763) cannot be flipped to `unless-stopped` by the new code, so an abandoned setup VM still cannot survive a reboot.
- **No reordering of a security guard.** `projectHomeIsReal` (:341) still runs before the switch (:344) and therefore before the new update (:356); the recreate branches set `err = errdefs.NotFound(...)`, so the update is never issued against a container that is about to be rebuilt.
- **`errdefs.IsNotFound → return nil` in `stopLocked` is behaviour-preserving**: the pre-change path reached the same `nil` via `ContainerStop`'s own NotFound check, and `stopLocked` has no post-stop cleanup that the early return would skip.
- **Seam widening is bounded.** `dockerAPI` (docker.go:24-33) gains one method; the interface is package-private, the production implementation is `*client.Client`, and only the two call sites use it. The fake's implementation records the call and mutates its own map — no host reach. `plant`'s new `restart: "unless-stopped"` makes the fake match a real project container rather than an empty policy, which is the safer default for the other seam tests (and, with the :356 guard, is what keeps them from recording a spurious update).
- **Tier 2 additions fail closed.** A missing container makes `docker inspect` fail, the substitution yields the empty string, and `[ "" = '0 0 no' ]` is false — the check fails rather than passing vacuously. `${CN}` is script-derived; no external input reaches the `-f` template.
- `go vet ./internal/orchestrator/` — run bare, exit 0, status read before this line was written.

## Tree moved during the review (both earlier findings resolved in the reviewed blob)
1. First pass read `orchestrator.go` at 45ebe02, where the restore sat inside the `!insp.State.Running` branch. Recorded then as the worst finding: a Stop that half-failed with the container still **running** left it pinned at `no`, and nothing ever restored it — the project would silently stop coming back after a host reboot. The tree changed to c2f9821, which makes the restore unconditional for every existing container and adds `TestEnsureRunningRestoresTheRestartPolicyOnARunningContainer` (bug063_test.go:83-105) for exactly that case. **Resolved.**
2. Second pass recorded the cost of that fix: an unconditional Docker **write** plus a runtime-shim round-trip on every attach of a running container, inside the project lock. The tree changed again to f161ed3, adding the inspect-based skip at :356 and matching the comment to it. **Resolved** (see "No amplification" above).

Both are recorded here rather than dropped, because the report is the only
record that the two states existed and why they were unsafe. Everything else
in this report describes the pinned blobs listed at the top. Four docs files
(`bugs/BUG-063-*.md`, `bugs/INDEX.md`, `specs/styrman/guide.md`,
`testing/styrman/README.md`) and two other agents' artifacts also appeared in
the tree during the review; they are outside this dispatch's scope.

## Checked
- **Secrets**: none added, none logged — the new `logger.Warn` calls carry the project name and the Docker error only; no home path, no broker material, no token.
- **Injection**: no SQL/command/template/path construction in the Go change; the two new shell checks interpolate only script-local `${CN}` into a `docker inspect -f` template. No new parsing boundary and no new external input.
- **Authz/authn**: no handler, guard or middleware touched; both call sites are reached only through already-authorized, owner-scoped paths (`stopProject`/`startProject`/`ensureFor`, all behind `getProjectFor`) and run under the per-project lock their callers already hold. No client-supplied identifier reaches Docker: the container name is derived from the store's project UUID.
- **Crypto**: nothing cryptographic in the diff — no primitive, token format, session or auth flow implemented or composed.
- **Dependencies**: `go.mod`/`go.sum` untouched (absent from `git status`); the only new symbol is `container.UpdateConfig` from the already-vendored Docker SDK. Per the dispatch, `govulncheck`/`npm audit` were **not** run — no dependency moved, and the project's path for that is the pinned-builder `deploy/vulncheck.sh`.
- **GCP surface**: none. Self-hosted platform (`.agents/docs/deploy/gcp.md`: not applicable); no IAM, service account, `--allow-unauthenticated`, or Secret Manager surface in the diff.

## Not examined
- `go test ./internal/orchestrator/...` was **not** run (report-only dispatch); only `go vet` was. The three tests in `bug063_test.go` were read, not executed, and the verifier's record `.agents/docs/testing/2026-09-16-verify-bug-063.md` was not read.
- Kata's own `Update` handling (`containerd-shim-kata-v2`) was not read; the findings reason from moby's side only, and the in-code claim "probed on a Kata VM 2026-09-16" was not reproduced.
- moby was read at the `go.mod` version (v28.5.2) in the module cache, not at the Engine version running on the live host.
- `internal/httpapi/server.go` was read at 240-312, 915-960 and 1120-1165 only; `internal/httpapi/network.go` only at 206/228 — the mode-toggle handler's full body, `provision.go`, and the terminal bridge's reconnect/backoff policy were not read end to end.
- `internal/orchestrator/orchestrator.go` was read at 1-80, 180-420, 700-790, 1180-1290, 1520-1560 plus the current diff hunks — the remainder (`Exec`, `stopMarked`'s full body, `projectBusy`, the reaper loop, `Destroy`) was not read end to end.
- `fakedocker_test.go` and `seam_test.go` were read as diffs plus surrounding hunks and the `record`/`callsTo` helpers; the rest of the fake was not read in full.
- `.agents/docs/testing/styrman/integration.sh` was read only at the diff hunk and its neighbours; the `check` helper's implementation and quoting were inferred from surrounding checks, not read.
- The four docs files that changed mid-review, and the code-review and verifier artifacts that appeared beside this one, were not reviewed (out of scope).

## Outcome

- Finding 1 (low) — rebutted for the stop check, fixed as wording for the start check: "stop left the container exited 0, no restart, policy off" asserts the behaviour (`State.ExitCode` 0 and `RestartCount` 0 — the daemon did not restart the clean exit) and reads the persisted policy as its third value; the check after the boot=1 start asserts the persisted `HostConfig.RestartPolicy.Name` only, which is what a host reboot reads — the comment above the stop check in `.agents/docs/testing/styrman/integration.sh` now says exactly that. Proving the live monitor would need a host reboot or a guest kill inside the suite; not done.
- Finding 2 (low) — fixed: `TestStopProceedsWhenTheRestartPolicyUpdateFails` (bug063_test.go) covers the two failure branches with the fake's `fail["ContainerUpdate"]` hook — a rejected update logs and the stop still reaches Docker; a NotFound answer returns nil with no TERM and no stop. The degraded path (update rejected, TERM taken, one restart) is the pre-fix behaviour the first test reproduced before the fix. The live detector stays the Tier 2 assertion.
- Bare re-checks after these edits: `go test ./internal/orchestrator/ -count=1` ok; Tier 2 on the final tree `.agents/scratch/bug-063/integration-run2.txt` (see the changes entry). Round 1 closed 2026-09-16.
