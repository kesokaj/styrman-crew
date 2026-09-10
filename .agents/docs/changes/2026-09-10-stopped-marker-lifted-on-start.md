---
owner: main
date: 2026-09-10
---
# Stopped marker lifted immediately after container start

The "deliberately stopped" marker (`o.stopped`, BUG-030) is now deleted immediately after `ContainerStart` succeeds, rather than only after a successful 90-second readiness wait. This closes a case where a start that succeeded but whose readiness wait timed out (a slow cold boot) left the marker set on a VM that came up anyway, causing every terminal auto-reconnect to be refused with close 4002 "project stopped" until someone clicked Start again.

## The fix

**Before:** `ensureRunningLocked` deleted the marker only after `waitReady` completed successfully. A start that succeeded but whose readiness wait timed out would leave the marker set on a running VM.

**After:** The marker is now deleted immediately after the `ContainerStart` call succeeds, within `ensureRunningLocked` and before `waitReady`. The marker's one job is to prevent auto-reconnects from booting a VM someone deliberately stopped; a start that has already happened makes it stale regardless of readiness.

## Files changed

- `internal/orchestrator/orchestrator.go` — `ensureRunningLocked` (+10/−3): marker deletion moved from after `waitReady` to immediately after the `ContainerStart` switch.

## Verification

- Verifier log: `.agents/docs/testing/2026-09-10-verify-stopped-marker.md` (gofmt/vet/go test -race, darwin build).
- Integration suite on real Kata run by main: all checks pass (result recorded in commit message).
- 13-line diff, reviewed by main (below subagent threshold).

## Coverage gap

No orchestrator-package unit test for this branch exists (BUG-003 residual: no fake Docker client). The integration suite exercises the path end-to-end; unit isolation remains deferred.

## Agents dispatched

verifier, changelog
