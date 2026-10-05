---
owner: bugs
date: 2026-10-05
id: BUG-090
status: fixed
area: internal/orchestrator (Destroy / force-remove)
---
# BUG-090: async project delete failed, Docker refused `docker rm -f` of a Kata container as "is zombie"

## Symptom
Tier 2 (`integration.sh`) while verifying per-project resources (ADR-083, plan
`.agents/docs/plans/2026-10-05-project-resources.md`). Runs 1 and 3 failed
`container gone (async delete)`, `workspace wiped`, `project gone from API`
(passed=139 failed=3); run 2 passed (142/0); run 4 aborted (aborted=1) at the
persistence section's bare `docker rm -f "${CN}"`:
`could not kill container: container c9308185ed26 PID 1312994 is zombie and can
not be killed. Use the --init option when creating containers ...`.
These checks had passed in every earlier recorded run (e.g. v1.29.0, v1.30.0 logs).

## Repro
Intermittent; seen only inside the full suite. Hand timings of `docker rm -f` on
full-size VMs (idle, 1.5 GiB and 20 GiB touched) were all 0.04-1 s. Manual replays
of the suite's resources and LAN steps deleted in 4 s, twice.

## Root cause
A Kata container's PID as Docker records it is its cloud-hypervisor process
(`docker inspect -f '{{.State.Pid}}'`, checked on dave). A force-remove landing
while the VM is exiting, before containerd-shim-kata-v2 reaps that process, is
refused with "is zombie" instead of waiting. `Orchestrator.Destroy` force-removed
once and returned the error, so the async delete failed (record kept with deleteError).
Why now: since ADR-083 project VMs are sized to the whole host (16 vCPU, ~60 GiB on
dave); the exit window evidently got long enough to hit. NOT proven: hand timings
did not reproduce the window.
Ruled out: delete blocked on the project lock; teardown time of a large VM.

## Fix
`forceRemove(ctx, name)` in `internal/orchestrator/orchestrator.go` retries a
force-remove every 2 s (`zombieRetryEvery`) while the error contains "is zombie",
until ctx ends; any other error is returned at once. Used by `Destroy`,
`DestroySetup`, and the setup-VM removals for a changed guest user, a moved home and
stale setup VMs. The suite's `docker rm -f` in the BUG-062 persistence step retries
the same way (30 x 2 s).
Tests: `internal/orchestrator/destroy_zombie_test.go` —
TestDestroyRetriesAContainerWhoseVMIsStillExiting (red before the fix),
TestDestroyReturnsOtherRemoveErrorsAtOnce, TestDestroyGivesUpWhenTheContextEnds.
`go test -race ./...` exit 0 after.

## Prevention
A Docker call that removes or kills a RUNNING Kata container waits out the VM's exit
(retry on "is zombie", bounded by ctx), never one shot; a test that removes a
running project container by hand retries the same way.

## Related
BUG-063 (restart policy across stop), BUG-088 (a request racing a stop): same family,
an operation meeting a VM mid-exit. ADR-083.
