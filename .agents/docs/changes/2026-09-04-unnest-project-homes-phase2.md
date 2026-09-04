# Un-nest project homes — phase 2     (v1.21.0)

## Summary

Feature + security, phase 2 of 2: documentation of the layout change (ADR-064), upgrade fixture verification, and integration/e2e suites green. The setup VM's home has moved from `HomesDir/<user>` to `HomesDir/<user>/base`; this entry closes Phase 1's test gates and confirms the upgrade path works: v1.20.0 state migrates 10 entries into `base/`, the setup VM detects its stale mount and recreates, the guest sees only its own home, and projects boot within R1.

## Changes

### Documentation (AGENTS.md, deploy.md, guide.md, api.md, README.md)

ADR-064 appended to `.agents/docs/architecture/DECISIONS.md`. Multi-tenant paragraph in `AGENTS.md` restated (the setup VM's home moved to `HomesDir/<user>/base`). Per-project-homes paragraph in `.agents/docs/specs/styrman/deploy.md` updated (the tenant dir is server-only; harvest sentence clarified). Architecture diagram and "Where things live" table in `.agents/docs/specs/styrman/guide.md` added row `homes/<user>/base` (setup VM home). `README.md` Status line updated for v1.21.0. Cold-read by `documentation` agent fixed: `GET /api/settings/tools` in `.agents/docs/specs/styrman/api.md` now names `base/` and adds server-only parent entry to the state-dir layout diagram.

### Verification artifacts

Upgrade fixture (`testing/2026-09-04-e2e-unnest-upgrade-fixture.md`, PASS): v1.20.0 server state with setup and project VMs; new server migrated 10 entries into `base/`, left `p/` untouched, read the setup-done marker from `base/`, and recreated the setup VM with the base bind. Guest inside setup VM cannot see `p/`.

Integration suite (`integration.sh` on `styrman-runtime:test`: passed=62 failed=0 skipped=3 aborted=0, boot→ready 1.72 s) and e2e suite (`web/e2e/run.sh`: passed=61 failed=0 skipped=2) — `testing/2026-09-04-verify-unnest-phase2.md`, PASS: unchanged — project creation, file visibility, and setup status working.

## Type

docs + e2e, phase 2 of 2

## Files changed

- `.agents/docs/architecture/DECISIONS.md` — ADR-064 appended
- `AGENTS.md` — multi-tenant paragraph restated for `homes/<user>/base`
- `.agents/docs/specs/styrman/deploy.md` — per-project-homes paragraph, harvest note
- `.agents/docs/specs/styrman/guide.md` — architecture diagram, "Where things live" table added `base/` row
- `.agents/docs/specs/styrman/api.md` — `GET /api/settings/tools` path names, state-dir layout diagram updated (cold-read fix)
- `README.md` — Status line for v1.21.0

## Agents dispatched

documentation (cold-read `.agents/docs/specs/styrman/api.md` and others), verifier (`.agents/docs/testing/2026-09-04-verify-unnest-phase2.md`), changelog (this entry).

## Verification

- ADR-064 reviewed and appended (captures the architecture decision).
- All doc surfaces (AGENTS.md, deploy.md, guide.md, api.md, README.md) reflect the new layout: setup VM home at `base/`, tenant dir server-only, project homes untouched at `p/<uuid>`.
- Upgrade fixture passes: v1.20.0 state → v1.21.0 server → 10 entries migrated into `base/`, setup VM recreated with new bind, guest mount is `base/` only, project boots within R1 (boot time logged).
- Integration suite (65 checks) green: project creation, file scope, setup status.
- E2E suite green: fresh onboarding path works.
- Phase 1 test gates closed: `.agents/docs/testing/2026-09-04-verify-unnest-phase1.md` (PASS), `.agents/docs/reviews/2026-09-04-review-unnest-phase1.md` (approve, 2 lows fixed: rule file closing paragraph, distinct log line and test for colliding entry in `migrateBaseHomes`), `.agents/docs/reviews/2026-09-04-security-unnest-phase1.md` (in flight).

## Requirements

R3 (per-project homes), R12 (isolation), and R1 (boot budget) unchanged. Layout architecture closed the check-to-mount window (BUG-002/003); v1.20.0 guards remain. No API, JSON, or env change.

## Related

- Plan: `.agents/docs/plans/2026-09-04-unnest-project-homes.md`
- Phase 1: `.agents/docs/changes/2026-09-04-unnest-project-homes-phase1.md`
- ADR-064: the architecture decision
- BUG-002, BUG-003, BUG-004: prior vulnerabilities the layout change closes
