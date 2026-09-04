# Exec interface: testable git ARGV   (date: 2026-09-04)

## Summary

`internal/httpapi` gained a narrow `guestRuntime` interface to decouple handler tests from the orchestrator's concrete type, enabling unit tests of the ARGV security decisions in git.go. The interface carries exactly two methods: `Status(ctx, user, project)` and `Exec(ctx, containerID, spec)`. Fifteen call sites (8 `s.orch.Exec`, 7 `s.orch.Status`) now route through it. The orchestrator's other ~15 methods stay on the concrete `orch` field — this is a seam for the exec path, not an abstraction over the orchestrator.

26 handler tests added in `internal/httpapi/guestexec_test.go` with a `fakeGuest`. What they exist for is the ARGV: `add -A` not `commit -a`; `--end-of-options` immediately before the branch on the switch path and NOT on the create path (where `-b` takes its argument as a value); `-u` on stash push; `--all` but never `--prune` on fetch; `--ff-only` on pull; the log limit clamped to 1..200; `git show --format= --patch` for a commit diff with its exit code checked (404 on an unknown revision) while the sibling `--no-index` branch still deliberately ignores exit 1, which means "files differ". Also: rejected branch names and bad commit hashes reach the guest ZERO times; reads report a stopped VM as 200 with a state field while mutations refuse; and format/check round trips including the STYRMAN_OK/STYRMAN_ERR sentinel.

**Important:** an Exec seam does NOT close all three v1.22.0 test gaps. It closes two — the six git handlers, and the `checkFile`/`formatFile` item. BUG-003's residual is a different seam: `EnsureRunning` has no unit test "because the orchestrator has no fake DOCKER client". The task to add a `dockerAPI` interface and fake is now in TODO.md at its true scope: that seam is ~166 lines with 19 methods to stub, so it is a separate task.

## The interface

A new `guestRuntime` type in `internal/httpapi/guestexec.go` (8 lines) with the contract: the ContainerID that Status returns belongs to the given (user, project) pair, and is the only thing binding a later Exec to the right tenant's VM. This contract was implicit under the concrete orchestrator; now it is stated.

## The tests

`internal/httpapi/guestexec_test.go`: 26 unit tests, each with a fresh `fakeGuest` derived from a (user, project) pair. The fake records which (user, project) pair each lookup asked about, and can fail. All 15 routing sites pass their original args unchanged (a pure receiver rename), so every test is a behaviour-preserving assertion of the existing git-handler ARGV choices. Tests cover:

- **git add:** `-A` flag present, no alternatives
- **git commit:** `-q` + message, no `--allow-empty` (prevents accidental create-tree-from-scratch commits)
- **git branch:** rejection via `IsBadBranchName` reaches guest zero times; `-m <old> <new>` routes through the handler, not below
- **git checkout:** `-q` + tracking setup on new branch; `-b <name>` REJECTS bad names immediately before constructing the command (bad names, unlike hashes, must not reach the guest at all)
- **git stash:** `push -u -q -m <msg>` (the `-u` includes untracked files); `pop -q`; list / drop
- **git fetch:** `--all -q` (the `--all` is unconditional, `--prune` is explicitly never added); error reporting
- **git pull:** `--ff-only -q` (fast-forward only, rejecting merge commits)
- **git log:** limit clamped to `[]int64{1, 200}` before constructing the command; `--no-walk --format=%H` for the commit hash list
- **git show:** the commit-diff branch uses `--format= --patch` and checks the exit code (404 if the revision is unknown); the `--no-index` diff branch deliberately ignores exit 1 (files differ)
- **git config:** read-only, no guest exec
- **checkFile/formatFile:** the sentinel round trip, STYRMAN_OK / STYRMAN_ERR
- **Stopped VM:** reads return 200 with state=stopped; mutations reject with 503
- **Tenant binding:** the fake counts executions and tracks (user, project) lookups; an Exec against the wrong tenant fails

## Security review findings (all fixed)

Two dispatches, both on the diff:

**reviewer (APPROVE WITH NITS, `.agents/docs/reviews/2026-09-04-review-guest-seam.md`):** verified the routing is behaviour-preserving at all 15 sites (a pure receiver rename, identical args, errors and control flow) and checked all tests line-by-line for vacuousness, finding none. One nit: the fake's mutex was justified by a background-boot race that never happens (the test config sets no Image, so `createProject` early-returns). Comment corrected to say the lock is precautionary and NOT evidence the concurrent path is covered.

**security (FINDINGS, `.agents/docs/reviews/2026-09-04-security-guest-seam.md`):** all fixed. The medium one is the notable one: the file header claimed handler tests for "every endpoint that reaches into a project's microVM" when only 4 of the 15 routed sites were covered — and the two uncovered CREDENTIAL sites are exactly what the seam was sent for security review over. A claimed-covered-but-uncovered exec path is how a regression on a credential read ships green. The header now states the true scope and lists all eleven uncovered sites by name so the gap stays visible. Also fixed: the interface now states its tenant-binding CONTRACT; the fake now derives its container id from (user, project) and records which pair each lookup asked about, instead of ignoring both; the Status method can now fail, so the error path is tested; and "nothing reached git" assertions became `execCount() == 0`, strictly stronger than "no git <sub> ran".

## Files changed

- `internal/httpapi/guestexec.go` — new; defines `guestRuntime` interface (two methods) and `fakeGuest` stub (28 lines), recording (user, project) lookups and exec attempts; tenant binding via derived container ID
- `internal/httpapi/guestexec_test.go` — new; 26 handler tests of git ARGV choices: branch/stash/commit/log/checkout/fetch/pull/show commands, bad-name rejection, sentinel round trip, stopped VM behaviour, tenant isolation, fake error path
- `internal/httpapi/httpapi.go` — `Server` gained a `guest guestRuntime` field, assigned from `orch` in `New()`. Fifteen call sites changed from `s.orch.Status(…)` to `s.guest.Status(…)` and from `s.orch.Exec(…)` to `s.guest.Exec(…)`. No logic change.
- `internal/httpapi/git.go` — no change; tests now cover the ARGV
- `internal/httpapi/files.go` — no change; tests now cover the checkFile/formatFile sentinel
- `.agents/docs/reviews/2026-09-04-review-guest-seam.md` — reviewer log, APPROVE WITH NITS, 1 nit on the false-concurrent-path claim
- `.agents/docs/reviews/2026-09-04-security-guest-seam.md` — security log, FINDINGS with fixes: claimed-covered-but-uncovered exec gap, contract statement, fake isolation

## Agents dispatched

reviewer (APPROVE WITH NITS; line-by-line vacuousness check, one nit corrected), security (FINDINGS, all fixed: coverage gap, contract, fake tenant isolation, error path).

## Verification

- `go test ./... -run guestexec` runs 26 new tests, all passing
- `go test ./...` (full suite) clean, no flakes, 0 new panics
- `go vet ./...` clean
- `gofmt` check clean (no control bytes: BUG-007)
- Frontend: `(cd web && npm run check && npm run lint && npm test)` unchanged: tsc -b clean, oxlint clean, vitest 69/69 passing
- `(cd deploy/caddy/ingress && go test ./...)` clean (vendored module)
- `docker build -t styrman-runtime:dev --build-arg STYRMAN_VERSION=dev-$(date +%s) image/` succeeds and boots the shell (loopback-only `-listen :7777` path, no auth)

## Requirements

n/a

## Related

- ADR-057 (git ARGV security decisions; these tests codify them)
- v1.22.0 release gate findings: three separate places noted the same missing-test gap (git handlers, checkFile/formatFile, dockerAPI interface). This seam closes two. BUG-003 (`.agents/docs/bugs/BUG-003-guest-paths-not-testable.md`): the orchestrator's concrete type and its 19 docker-related methods block testing the symlink guard on bind sources (`projectHomeIsReal`/`realBelow` in `EnsureRunning`). A `dockerAPI` interface task is deferred to TODO.md with true scope.
- TODO.md: one-liner on the dockerAPI seam work
