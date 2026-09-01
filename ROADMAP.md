# Roadmap

<!-- Division of labor: ROADMAP.md = where the project is going; TODO.md = small deferred items; .agents/docs/plans/ = how. -->

Last reviewed: 2026-09-01

Empty by design past "Now" (operator directive 2026-08-30: "I want nothing
in todo and roadmap — start fresh with new things"). See `TODO.md` for the
matching empty-slate Inbox and **ADR-045**
(`.agents/docs/architecture/DECISIONS.md`) for how the previous roadmap was
closed out. History lives in `.agents/docs/changes/` and git.

## Now

<!-- Deliberately empty — awaiting operator input. Add items here as they arise, don't invent them. -->

## Next

No automated assertion that a seeded agent harness is correct. Neither
`.agents/docs/testing/styrman/integration.sh` nor the Playwright e2e suite
(`web/e2e/`) checks the harness deployed into a project home — not the
constitution, skills, subagents, nor the `~/.agents/bin/` shell gates.
Every check for the v1.15.0 release was derived ad hoc from
`image/entrypoint.sh` and recorded only as manual steps ("Tier 5" in
`.agents/docs/testing/styrman/README.md`). Related: `integration.sh` polls
readiness in a `seq 1 60` loop with no latency assertion, which is why a
60-second regression on the container boot path (fixed in v1.15.0 — it
violated REQUIREMENTS.md R1 by 4x) went undetected. A boot-latency
assertion plus a handful of harness assertions in `integration.sh` would
catch this class of regression automatically.

## Later / Ideas

<!-- Deliberately empty — awaiting operator input. Add items here as they arise, don't invent them. -->
