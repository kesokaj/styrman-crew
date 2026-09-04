# Roadmap

<!-- Division of labor: ROADMAP.md = where the project is going; TODO.md = small deferred items; .agents/docs/plans/ = how. -->

Last reviewed: 2026-09-03

Empty by design past "Now" (operator directive 2026-08-30: "I want nothing
in todo and roadmap — start fresh with new things"). See `TODO.md` for the
matching empty-slate Inbox and **ADR-045**
(`.agents/docs/architecture/DECISIONS.md`) for how the previous roadmap was
closed out. History lives in `.agents/docs/changes/` and git.

## Now

**v1.21.0 shipped 2026-09-04** (`.agents/docs/changes/2026-09-04-v1.21.0-unnest-project-homes.md`): the setup VM's home is `homes/<user>/base` and no guest mount contains the project homes (ADR-064); production's base home migrated at the deploy. Before it, v1.20.0 (the harness migration end to end, BUG-001, eight-hour sessions). Nothing is in flight; the next items are the operator's picks from `TODO.md` (the legacy `migrateWorkspaces` path, the `git.go:73` read, the upstream `sh-agent` items, the ingress lows).

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
