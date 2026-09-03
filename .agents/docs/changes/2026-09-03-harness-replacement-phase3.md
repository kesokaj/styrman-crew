# Harness replacement, phase 3: server roster and security boundary     (date: 2026-09-03)

## Summary

Server roster (D5): `internal/httpapi/setup.go` `rosterHome` now picks the user's newest project whose home has `.config/sh-agent/state.json` (base home never seeded); settings roster (Skills, Agents, Commands) read from it. Agy Agents listed from `.gemini/config/agents/*/agent.md`. `listNames` skips dot-files and old harness re-seed backups (anchored to `<file>.bak.<YYYYmmdd-HHMMSS>`). Readiness flags still read base home. Tests in `internal/httpapi/setup_roster_test.go` (newest seeded home wins over newer unseeded, backup filter, legitimately named `a.bak.tool.md` listed, dot-files skipped, agy agents by directory, empty without seeded project). Frontend: `web/src/routes/settings.tsx` `rosterOf` adds "N skills · M subagents" to claude and agy card detail (first time rendered); `web/dist/client` rebuilt.

Security (three rounds): HIGH — `internal/files.openRoot` called `os.OpenRoot(root)` by path, which follows symlinks on the root itself, exposing any host path or cross-tenant home when a guest planted a link at `p/<uuid>`. Fixed: `files.Trust(bases…)` wraps each root with `os.OpenRoot(<base>/<tenant>)` + `Root.OpenRoot(rest)`, refusing host and cross-tenant links, allowing relative in-tenant links, refusing absolute. Registered in `httpapi.New` for `HomesDir` and `StateDir/projects`. MEDIUM — unbounded `ReadDir` on guest-writable dirs (20 s poll); capped `ListNames` at 1000. Round 2 exposed NEW pre-existing HIGHs via the same invariant: transcript janitor (root, 6 h) opened `p/<uuid>` by raw path; orchestrator passed raw project-home path to Docker after `os.MkdirAll` through symlinks. Fixed: janitor lists/opens through `os.OpenRoot` + root API + `fs.Stat` (caught `d.Info()` Go 1.25 regression); orchestrator gained `realBelow` check on create + `projectHomeIsReal` on every `EnsureRunning` path. Tests: `internal/files/trust_test.go` (`TestTrustedRootRefusesAPlantedSymlink`, `TestListNamesIsCapped`), `internal/orchestrator/realbelow_test.go` (`TestRealBelowRefusesSymlinkComponents`, `TestProjectHomeIsRealAllowsMissingRefusesLinks`), `cmd/styrman/prune_test.go` (janitor path isolation). Round 3: START branch re-resolved symlinked bind source; fixed with `projectHomeIsReal` check post-MkdirAll. Residual: check-to-Docker window in `EnsureRunning` (no fake client for unit test); proposed durable fix (un-nest homes, ADR for operator) in TODO.md. Class rule graduated to `.agents/rules/guest-writable-paths.md`.

## Type

feature + security, phase 3 of 5

## Files changed

- `internal/httpapi/setup.go` — `rosterHome` picks newest seeded project home; settings read from it; `listNames` skips dots and `.bak.*` anchored backups; agy agents from `.gemini/config/agents/*/agent.md`
- `internal/httpapi/setup_roster_test.go` — new tests: seeded-home precedence, backup filter, legitimate `a.bak.tool.md` listed, dot-files hidden, agy agent discovery, empty without seeded project
- `internal/files/` — new `Trust` class wrapping root per tenant; `openRoot("")` fails closed; `ListNames` capped at 1000; `Stat`/`ReadDir` via trusted root
- `internal/files/trust_test.go` — new: planted symlink refused, cap enforced
- `internal/orchestrator/` — `realBelow` utility; `projectHomeIsReal` checks pre- and post-MkdirAll on `EnsureRunning` paths
- `internal/orchestrator/realbelow_test.go` — new: symlink components refused, missing home allowed
- `cmd/styrman/main.go` — `pruneTranscripts` uses `os.OpenRoot` + root API + `fs.Stat`
- `cmd/styrman/prune_test.go` — new: janitor confined within home
- `.agents/rules/guest-writable-paths.md` — new: class rule on symlink boundaries
- `web/src/routes/settings.tsx` — `rosterOf` renders "N skills · M subagents" on roster cards
- `web/dist/client` — rebuilt

## Agents dispatched

verifier (`.agents/docs/testing/2026-09-03-verify-harness-phase3.md` PASS; round 2 `.agents/docs/testing/2026-09-03-verify-harness-phase3-round2.md` PASS, live probe skipped—production runner), reviewer (`.agents/docs/reviews/2026-09-03-review-harness-phase3.md` APPROVE; rosterOf nit on term "subagents" kept), security (three rounds: R1 HIGH `openRoot` symlink traversal + MEDIUM unbounded `ReadDir`, R2 HIGH janitor and orchestrator raw-path exposure, R3 HIGH START re-resolve; rule converged per `.agents/docs/reviews/2026-09-03-security-harness-phase3*.md`; residual check-to-Docker window noted in TODO.md), bugs (BUG-002 `files.openRoot` symlink exposure, BUG-003 janitor + orchestrator raw paths).

## Verification

- `cd internal/files && go test ./…` — trust tests pass (planted symlink refused, cross-tenant refused, cross-host refused, legitimate `a.bak.tool.md` listed)
- `cd internal/orchestrator && go test ./…` — realbelow tests pass
- `cd cmd/styrman && go test ./…` — prune test confines janitor
- `internal/httpapi/setup_roster_test.go` passes: seeded home selected, backup filter, agy agent discovery
- Server starts without error; `GET /api/settings` renders roster with skill and subagent counts
- Frontend builds; `web/dist/client` updated

## Requirements

R3 (per-project homes, credentials, isolation) tightened: tenancy boundary now enforced at host level for symlink resolution and Docker bind sources. R12 (per-project network isolation) unaffected. Proposed durable fix requires operator decision (ADR on home nesting).

## Related

- ADR-062 (D1–D8, decisions and consequences; D5 updated with roster design)
- Plan: `.agents/docs/plans/2026-09-03-image-harness-replacement.md`
- Security records: `.agents/docs/reviews/2026-09-03-security-harness-phase3.md`, `…-round2.md`, `…-round3.md`
- Bug records: `.agents/docs/bugs/BUG-002-*.md`, `BUG-003-*.md`
- Rule: `.agents/rules/guest-writable-paths.md`
