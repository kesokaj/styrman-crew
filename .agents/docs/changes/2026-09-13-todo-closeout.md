---
owner: changelog
date: 2026-09-13
---
# TODO close-out 2026-09-13 (date: 2026-09-13)

## Summary

The whole-project audit (2026-09-13) examined 31 surfaces and identified two security-relevant lows and ten cosmetic/low-severity follow-ups from the 2026-09-10 plan. The operator approved all recommendations for strike or fix. Two items are fixed in v1.30.1 PATCH: `deploy/firewall/styrman-firewall.sh` (missing-`xt_physdev` condition now degrades via `warn()`, reaching the exit code), and `image/entrypoint.sh` (harness version and stamp path passed as arguments to `sh -c`, not interpolated). A new `image/bump-pins.sh` script refreshes ADR-069's pinned versions and rewrites Dockerfile ARG lines on demand. The remaining ten items are struck with reasons recorded in ADR-070; the open TODO is now empty per ADR-045's standing rule.

## Type

task

## Files changed

- `.agents/docs/architecture/DECISIONS.md` — ADR-070 appended
- `TODO.md` — Inbox cleared; one explanatory line added
- `deploy/firewall/styrman-firewall.sh` — missing-`xt_physdev` condition routed through `warn()`
- `image/entrypoint.sh` — harness version and stamp path passed as `sh -c` arguments
- `image/bump-pins.sh` — new script to refresh pinned versions and update Dockerfiles

## Agents dispatched

none

## Verification

Recorded in the release gate report.

## Requirements

n/a
