---
owner: main
date: 2026-09-04
---
# Preflight — v1.21.0 (2026-09-04)

Range reviewed: v1.20.0..f8e6989 (the release commit that carries this report and the release entry is docs-only on top of f8e6989; `deploy/verify-commit.sh` runs again on the tagged commit before the tag).

## VERDICT: SHIP

Found during the gate, worst first, and its state now:

1. **[medium / confidence: high] FIXED — two live tests had been overwritten.** The Phase 1 test file (`f3882ac`) replaced `cmd/styrman/migrate_test.go`, dropping `TestMigrateWorkspaces` and `TestEnsureReposRefusesUnmigrated` — the only cover for the ADR-027 workspace migration and the R13 unmigrated-workspace guard, whose input path this very change re-pointed. Three review rounds and a green suite did not notice (a deleted test cannot fail). Restored verbatim in `f8e6989`; all seven tests pass; BUG-005 with a prevention rule (`.agents/rules/writing-files.md`). Raised by the release reviewer and, independently, the release security pass.
2. **[medium / confidence: high] FIXED — the startup sweep of stale setup VMs ran with a 60 s Docker budget ahead of the listener**, the shape that once hung `/api/health` after a reboot; now 10 s (`f8e6989`).
3. **[medium / confidence: medium] RECORDED, not a regression — `migrateWorkspaces`** (the pre-ADR-027 workspace move) still resolves its source with `EvalSymlinks` and renames by path, and that source now sits under the setup VM's live mount. It fires only for a legacy workspace, of which this host has none. `TODO.md`.
4. **[low] FIXED — a failed rename in `migrateBaseHomes` dropped its error**; the first cause is now logged under "base home migration incomplete" (`f8e6989`).
5. **[low] RECORDED — `git.go:73`** reads `base/.gitconfig` by path (`TODO.md`, pre-existing).

Phase 1's own security review blocked in round 1 (a rename by path could follow a `base` swapped mid-loop by a still-running old setup VM; the recreate was lazy) and cleared in round 2 after the migration moved onto an `os.Root` at the tenant dir and a startup sweep force-removes stale setup VMs first (`reviews/2026-09-04-security-unnest-phase1.md`). The release pass confirms BUG-003's check-to-mount residual — accepted in v1.20.0 — is closed by construction: `base/` and `p/<uuid>` are siblings under a tenant dir mounted nowhere.

## Rows

- **Reviewer:** `reviews/2026-09-04-review-v1.21.0.md` (opus) over the source no earlier report saw — approve with nits, the mediums fixed above; `reviews/2026-09-04-review-unnest-phase1.md` (approve, 2 lows fixed); the plan's cold-read `reviews/2026-09-04-review-plan-unnest-project-homes.md` (three rounds to ready).
- **Security:** `reviews/2026-09-04-security-v1.21.0.md` (non-blocking; asked that the tag land on the commit with the restored tests — it does); `reviews/2026-09-04-security-unnest-phase1.md` (2 rounds). `deploy/vulncheck.sh`: the accepted docker/docker pair only, `image/harness` clean; `npm audit`: 0.
- **Verification:** `testing/2026-09-04-verify-v1.21.0.md` PASS on `3e2f125` (go/web gates, `verify-commit.sh` OK, integration 62/0/3/0 with boot→ready 1.63 s, e2e 61/0/2, production untouched); `f8e6989` changed a timeout, a log line, and tests — `go test ./... -race` green on it; `verify-commit.sh` is run again on the tagged commit. `testing/2026-09-04-verify-unnest-phase{1,2}.md`; the upgrade fixture `testing/2026-09-04-e2e-unnest-upgrade-fixture.md` (three runs: old layout migrated, stale setup VM swept, guest cannot see `p/`, project boots).

## Trail freshness

- Newest `changes/` entry describes the release work: pass (`2026-09-04-v1.21.0-gate-fixes.md`, then the release entry).
- No future `date:`: pass. `changes/` and `DECISIONS.md` append-only since v1.20.0: pass (0 deletions; ADR-064 appended). `ROADMAP.md`/`TODO.md` touched: yes.

## Docs freshness

ADR-064; `AGENTS.md`, `README.md` (Status → v1.21.0), `.agents/docs/specs/styrman/{deploy,api,guide}.md` updated; the `documentation` agent's cold-read found nothing else stale. `REQUIREMENTS.md` unchanged (R3, R12 hold).

## Pre-gate commits on the branch

0da855e (plan), f3882ac (Phase 1), 3e2f125 (Phase 2), f8e6989 (gate fixes) — all pushed.

## What was not checked

- The startup sweep against a slow or absent Docker daemon was reasoned about (10 s budget, then the migration proceeds), not exercised.
- A downgrade to v1.20.x after the migration (one-way; stated in ADR-064) was not exercised.
- The deploy runs through the auto-updater from the signed tag; the live check (health, the version in the container tag, a setup VM bound to `base/` on production) is reported in the session after the push.
