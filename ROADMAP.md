# Roadmap

<!-- Division of labor: ROADMAP.md = where the project is going; TODO.md = small deferred items; .agents/docs/plans/ = how. -->

Last reviewed: 2026-09-03

Empty by design past "Now" (operator directive 2026-08-30: "I want nothing
in todo and roadmap — start fresh with new things"). See `TODO.md` for the
matching empty-slate Inbox and **ADR-045**
(`.agents/docs/architecture/DECISIONS.md`) for how the previous roadmap was
closed out. History lives in `.agents/docs/changes/` and git.

## Now

Harness migration, operator-confirmed 2026-09-03: scope A (the repo's own harness layout) done; scope B (the image seeds `sh-agent` from a `styrman-harness` mirror, ADR-062) implemented through Phase 4 in four commits on 2026-09-03 — plan `.agents/docs/plans/2026-09-03-image-harness-replacement.md`. Open before the `v1.20.0` release (Phase 5, `/preflight`): the operator's call after the Phase 3 security review did not converge in three rounds (`reviews/2026-09-03-security-harness-phase3.md`; every finding is fixed, the residual is stated in BUG-003), the token-lifetime fix (approved 2026-09-04, ships in this release). The `REQUIREMENTS.md` R5 wording was updated with the operator's approval on 2026-09-04 (ADR-062 amendment). The "fail to fetch" bug is fixed (BUG-001) and ships with that release.

## Next

Closed 2026-09-03 (ADR-062): `integration.sh` now asserts the seeded harness
inside a booted project (`sh-agent status` no drift, the two-line
`CLAUDE.md`, 28 skills / 7 agents, no previous-harness artifacts) and bounds
boot→ready by R1's 15 s hard limit from the container's own timestamps — the
"no automated assertion that a seeded agent harness is correct" item and its
boot-latency gap. Tier 5 in `.agents/docs/testing/styrman/README.md` keeps
only what a script cannot judge.

## Later / Ideas

<!-- Deliberately empty — awaiting operator input. Add items here as they arise, don't invent them. -->
