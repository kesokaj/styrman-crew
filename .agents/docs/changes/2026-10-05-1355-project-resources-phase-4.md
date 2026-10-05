---
owner: main
date: 2026-10-05
---
# Per-project resources, Phase 4: end to end, BUG-090, review fixes, docs   (2026-10-05)

Plan: `.agents/docs/plans/2026-10-05-project-resources.md` (Phase 4).

- **BUG-090 fixed: an async delete sometimes failed.**
  - Docker refused the force-remove with "PID … is zombie and can not be
    killed". A Kata container's PID is its cloud-hypervisor process, and a
    remove that lands while the VM is exiting, before the shim reaps it, is
    refused instead of waited on.
  - New `forceRemove` in `internal/orchestrator/orchestrator.go` retries every
    2 s while the error says "is zombie", until ctx ends; any other error
    returns at once. `Destroy`, `DestroySetup` and the setup-VM removals (guest
    changed, home moved, stale) use it.
  - Tests in `destroy_zombie_test.go` (3); the retry test was red before the fix.
  - Tier 2 found it: runs 1 and 3 failed the three delete checks, and run 4
    aborted at the suite's own `docker rm -f`, which now retries the same way.
- **Code review finding 1 fixed:** the entrypoint's Docker-disk grow could
  never retry once `truncate` had succeeded, because it compared the FILE size.
  - It now compares the filesystem size (`dumpe2fs -h`) and truncates only
    when the file is smaller.
  - Checked in a Kata guest: a plain grow; the finding's trigger, a file at 3G
    with a 2G filesystem, now grows; at size it is a no-op; a smaller request
    never shrinks.
- **Tier 2** (`integration.sh`): a "resources (ADR-083)" section of 20 checks
  after the network-mode section, run against real Kata. Run 5:
  `passed=142 failed=0 skipped=3`.
- **e2e** (`web/e2e/run.mjs`): two Resources-dialog checks at the end, each
  closing the dialog in a `finally`. `passed=101 failed=0 skipped=2`.
- **Docs** (`documentation` agent): `api.md` (the two endpoints; the removed
  env vars), `deploy.md` (sizing, the post-release `host-setup.sh` re-run,
  measurements), `guide.md` (the dialog, the renamed setting), `gcp.md` (the
  re-run on the VM) and `AGENTS.md` (an Architecture bullet). The testing
  README's count lines are updated.
- **Reviews:**
  - `reviewer`: `.agents/docs/reviews/2026-10-05-review-project-resources.md`,
    APPROVE WITH NITS. Finding 1 fixed; 2 (no per-user disk quota) is the
    operator's choice, recommended defer; 3 (QEMU hosts) rebutted under plan D28.
  - `security`: `.agents/docs/reviews/2026-10-05-security-project-resources.md`,
    2 medium and 3 low. Both mediums restate operator decisions: the whole host
    by default (the operator's words), and reclaim needing the manual re-run
    (D28). Outcomes are in each report.

Agents: `documentation`, `reviewer`, `security`, `bugs` (BUG-090). Diff of the
fixes read by me.

## Learnings for the local host
dave's memory reclaim stays off until `host-setup.sh` is re-run after the
release (root, the operator's permission per request).
