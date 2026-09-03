# Roadmap — styrman-harness

<!-- Division of labor: ROADMAP.md = where the project is going; TODO.md = small deferred items; .agents/docs/plans/ = how. -->

Last reviewed: 2026-09-03

## Now

Real use. The rebuild is verified on the Linux box for both CLIs (every skill and agent, sandbox and real home — `.agents/docs/testing/`), and installed in the operator's real home there. What is left is the Mac: `sh-agent update`, then one interactive start of each CLI to confirm `~/.gemini/config/rules/AGENTS.md`, the seven agents, and the 28 skills load (checklist: `.agents/docs/plans/2026-09-03-rebuild.md`, section 12). No tuning before two weeks of use (ADR-009).

## Next

- After two weeks: `/retro` over the feedback log; fold accepted proposals into `context/`, skills, or agents; count agent dispatches from the changelogs (ADR-009 cost check).
- Re-run the skill matrix (`.agents/docs/testing/skill-matrix.md`) after any change to `context/`, `agents/`, or a workflow skill.

## Later / Ideas

- A `sh-agent sandbox` subcommand if re-running the matrix by hand becomes routine (ADR-010 leaves the door open).
- Nothing else scheduled. A rejected option is a decision, not a plan — see `REQUIREMENTS.md` Non-goals.
