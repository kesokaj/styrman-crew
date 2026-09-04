# Roadmap

<!-- Division of labor: ROADMAP.md = where the project is going; TODO.md = small deferred items; .agents/docs/plans/ = how. -->

Last reviewed: 2026-09-03

Empty by design past "Now" (operator directive 2026-08-30: "I want nothing
in todo and roadmap — start fresh with new things"). See `TODO.md` for the
matching empty-slate Inbox and **ADR-045**
(`.agents/docs/architecture/DECISIONS.md`) for how the previous roadmap was
closed out. History lives in `.agents/docs/changes/` and git.

## Now

**v1.20.0 shipped 2026-09-04** (`.agents/docs/changes/2026-09-04-v1.20.0-harness-replacement-and-session-expiry.md`): the harness migration is complete — scope A (the repo's own layout) and scope B (the image seeds `sh-agent` from a `styrman-harness` mirror, ADR-062) — plus BUG-001 and eight-hour sessions (ADR-063). Left for the operator: the manual Tier 5 step 4 (`claude -p` / `agy` inside a fresh project on production), and the decisions queued in `TODO.md` — the un-nesting ADR for project homes (the durable fix behind BUG-002/003), the upstream `sh-agent` items, and the ingress lows.

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
