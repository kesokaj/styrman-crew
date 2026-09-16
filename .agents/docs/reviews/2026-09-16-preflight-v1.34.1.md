---
owner: main
date: 2026-09-16
---
# Preflight — v1.34.1 (2026-09-16)

Range reviewed: v1.34.0..c26a723 (plus the release commit)

Release: **PATCH** v1.34.1 — fixes released behaviour only (BUG-063):
stopping a project now switches its container's restart policy to `no`
before the guest's own shutdown, and every attach or start switches it back
to `unless-stopped`, so Docker no longer restarts the clean exit and then
hard-kills the rebooted VM. No endpoint, flag, image or deploy step changes;
`web/` untouched. Commits in the range: 51fcfe2 (v1.34.0 verification
records), 976b565 (handoff), c26a723 (the fix, under the bug skill with its
own rows). No pre-gate commit: the tree was clean when the gate started.

## VERDICT: SHIP

No blocking condition: verifier runs green over the range (`deploy/verify-commit.sh
c26a723` "safe to tag", Tier 2 `passed=103 failed=0 skipped=3`); security
FINDINGS with no high and a `## Not examined` naming no release-diff path
(every path read); reviewer APPROVE WITH NITS on the fix commit; docs lens
APPROVE; no REQUIREMENTS.md violation; no project agent shadows a harness
agent.

## Findings (FAIL-first)

None blocking.

- **[low / high — FIXED in the release commit]** reviewer #1 and #2: the
  fix's changes entry carried two insertion counts taken before the last
  edits (35→38 for `orchestrator.go`, 13→12 for `integration.sh`) and said
  "a NotFound answer returns nil" without "on the stop path"; a
  `## Correction` section is appended to `changes/2026-09-16-bug-063-fix.md`
  (append-only file).
- **[low / medium — FIXED by evidence]** security #1: nothing asserted that
  a running Kata guest's sizing survives the policy update (moby forwards
  the all-zero `Resources` to the shim). Probed on the throwaway project
  `verify-v1-34-0` on the production host: `docker inspect` `NanoCpus`
  `Memory` `RestartPolicy` read `0 0 unless-stopped` before, `0 0 no`
  after `docker update --restart=no`, `0 0 unless-stopped` after the
  restore; inside the guest `nproc` 1 and `free -m` total 1986 identical at
  all three points; `/run/styrman/ready` present throughout
  (`.agents/scratch/preflight-v1.34.1/sizing-probe.txt`).
- **[low / high — REBUTTED]** security #2: the stop-path update is warn-only
  and a rejected update is invisible above the log while the API still
  answers `stopped`. The stop itself still completes (`ContainerStop`
  follows and the container ends stopped — `TestStopProceedsWhenTheRestartPolicyUpdateFails`);
  a rejected update only reinstates the one-extra-boot race for that stop,
  and the Warn is its visibility. Surfacing it in the API would be a new
  field; noted in the handoff, not a defect of the fix.
- **[low / high — carried]** security #3: `npm audit` in `web/` reports one
  high (js-yaml, via the TanStack start plugin), build-time only, absent
  from `web/dist/client`; recorded by the v1.33.0 gate the same way
  ("pre-existing, carried"); `web/` is untouched since.
- **[low / low]** security #4: see the report; nothing to act on for this
  release.
- **[informational]** reviewer #3: the stop path now makes one shim
  round-trip under the project lock before the exec — the same hang class
  as the existing first step, not a new one.
- **[medium / high — carried, accepted]** `bash deploy/vulncheck.sh` exits 3:
  GO-2026-4887 and GO-2026-4883 in `github.com/docker/docker@v28.5.2`,
  no fix available, the pair the script's header accepts; `image/harness`
  clean. `go.mod`/`go.sum` untouched.

## Review rows

- Code, the fix commit (976b565..c26a723, opus):
  `.agents/docs/reviews/2026-09-16-review-976b565-c26a723-code.md` —
  APPROVE WITH NITS; it reverted both `ContainerUpdate` blocks in a scratch
  copy and saw all four new tests fail for the defect's reason, and
  mutation-checked the failure-branch test. The fix as a working tree:
  `2026-09-16-review-worktree-bug-063-code.md` — round 1 REQUEST CHANGES
  (the restore lived only in the start branch), round 2 APPROVE WITH NITS,
  `## Outcome` written. 51fcfe2 and 976b565 are trail only.
- Security, the range (v1.34.0..c26a723, opus):
  `.agents/docs/reviews/2026-09-16-security-v1.34.0-c26a723.md` — FINDINGS,
  three low and one low/low; `## Not examined` names no diff path. The
  fix's own row: `2026-09-16-security-worktree-bug-063.md` (two low, one
  rebutted with a comment in the suite, one fixed by a test; Outcome
  written); it verified the all-zero `Resources` do not reset the guest's
  persisted limits in moby v28.5.2.
- Verification: `.agents/docs/testing/2026-09-16-verify-v1.34.1-release.md`
  (`Range reviewed: v1.34.0..c26a723`). Its runs, read by the main agent
  from `.agents/scratch/verify-v1.34.1-release/`: `gofmt -l`, `go vet`,
  `go build`, `shellcheck` — no output; root `go test ./... -v -count=1`
  354 PASS / 0 FAIL / 1 SKIP with `TestStopTurnsTheRestartPolicyOffBeforeTheGuestExits`,
  `TestEnsureRunningTurnsTheRestartPolicyBackOnBeforeStart`,
  `TestStopProceedsWhenTheRestartPolicyUpdateFails` and
  `TestEnsureRunningRestoresTheRestartPolicyOnARunningContainer` PASS;
  ingress 57/0/0; mirror 79/0/1, `sh-agent list` 27/7; vulncheck the
  accepted pair only; `deploy/verify-commit.sh c26a723` full run → `OK:
  commit c26a72383056 builds and tests clean in isolation — safe to tag`,
  `runtime image seeds 27 skills, 7 subagents (styrman-harness 4aa19be)`;
  Tier 2 once → `passed=103 failed=0 skipped=3 aborted=0` with "stop left
  the container exited 0, no restart, policy off (BUG-063)" and "start
  switched the restart policy back on (BUG-063)" PASS. The log's `## Status`
  line still read "IN PROGRESS (Tier 2 running in background)" when this
  report was written — the agent's bounded wait loop had not released it,
  the third such stall today; the log is committed as the agent leaves it,
  and the outputs above are its own files. The fix's own row:
  `testing/2026-09-16-verify-bug-063.md` — PASS (root 350/0/1, Tier 2
  103/0/3 on the tree before the hoist).
- Docs lens: `.agents/docs/reviews/2026-09-16-review-v1.34.0-c26a723-docs.md`
  — APPROVE, no findings (its one note: guide.md says "since v1.34.1"
  before the tag exists — this release).
- Shadow agents: none.

## Verification observed by the main agent (bare, status read)

- Unit tests: the first two BUG-063 tests failed before the fix
  (`after Stop: running=false restarts=1 — the daemon restarted the clean
  exit`, taking the 12 s wait as the live stop did; `after Start:
  running=true policy="no"`) and pass after it; `go test ./internal/orchestrator/
  -count=1` ok after every edit.
- Tier 2, two bare runs: `.agents/scratch/bug-063/integration-run1.txt`
  (before the hoist) and `integration-run2.txt` (the final tree), each
  `passed=103 failed=0 skipped=3 aborted=0`, rc 0, both BUG-063 checks PASS.
- Live probes on the throwaway project (the operator's projects untouched):
  a running Kata container accepted `docker update --restart=no` and back
  with the guest still ready; the sizing probe above.
- Live host before the tag: `styrman-server:1.34.0` healthy, deployed
  v1.34.0 (sha c1f7e31), `styrman-update.timer` active; projects:
  `verify-v1-34-0` running on 1.34.0, `google` stopped (the BUG-063
  casualty, on its 1.33.0 container). Docker Hub anonymous budget 100/100 at
  the start of the gate.

## Trail freshness

- Newest `changes/` entry `2026-09-16-bug-063-fix.md` describes the fix
  commit; the release entry is written in the release commit. Pass.
- No `date:` in the future under `.agents/docs`. Pass.
- Append-only: no past entry modified in the range; the fix entry gains an
  appended Correction in the release commit (4 lines added, 0 deleted). Pass.
- `ROADMAP.md` and `TODO.md` untouched since v1.34.0, both headers-only.
  Report-only.

## What was not checked

- Browser e2e (`web/` untouched; the frontend trio ran inside
  `verify-commit.sh`); Tier 5 (mirror unchanged).
- The live Stop path on the production host at v1.34.1 — the post-release
  check below, on the throwaway project.
- A rejected `ContainerUpdate` from a real Kata shim (only the fake's).
- The release commit itself: verified by `deploy/verify-commit.sh` on it
  after it exists (recorded below).
