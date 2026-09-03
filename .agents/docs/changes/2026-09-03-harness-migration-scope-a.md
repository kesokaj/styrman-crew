# Harness migration, scope A: the repo's own agent layout on the current harness     (date: 2026-09-03)

## Summary

The dev harness the operator uses on this repo moved to the current `styrman-harness` layout (28 skills, 7 agents, `sh-agent`, no hooks). Nothing was deleted: the repo had no `GEMINI.md`, hooks, scripts, or `.agents/workflows/`; old-harness wording survives only in append-only history and in `ROADMAP.md` "Next", which describes the product's seeded harness (scope B, a separate feature — plan in `.agents/docs/plans/2026-09-03-harness-migration.md`).

Added: `CLAUDE.md` (`@AGENTS.md`); `.agents/{rules,skills,agents,commands,workflows,scratch}`; `.agents/docs/bugs/INDEX.md`; `.agents/docs/deploy/{local,remote,gcp}.md` filled from `AGENTS.md` Setup Commands, `deploy/README.md`, and `specs/styrman/{deploy,versioning}.md` (GCP: not applicable, self-hosted); `.claude/{skills,agents,commands,rules}` symlinks into `.agents/`; `.gitignore` lines for `.claude/worktrees/` and `.agents/scratch/`; an `AGENTS.md` `## Deploy` section pointing at the guides. `.claude/agent-memory/` stays tracked as before.

## Type

task

## Files changed

- `CLAUDE.md`, `AGENTS.md`, `.gitignore`
- `.agents/{rules,skills,agents,commands,workflows,scratch}/.gitkeep`, `.claude/{skills,agents,commands,rules}` (symlinks)
- `.agents/docs/bugs/INDEX.md`, `.agents/docs/deploy/{local,remote,gcp}.md`, `.agents/docs/plans/2026-09-03-harness-migration.md`

## Agents dispatched

`explorer` (recon of the image-seeded harness for scope B; brief under `plans/`). Docs and layout only; no source changed, no review needed.

## Verification

`go build ./... && go vet ./...` clean. A headless Claude Code session in the repo reported `CLAUDE.md` → `AGENTS.md`, the three deploy guides from the `## Deploy` section, and the global context (28 skills, 7 subagents) loaded alongside.

## Requirements

None touched.
