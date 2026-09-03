---
owner: main
date: 2026-09-03
---
# Plan — migrate styrman-crew to the current harness (2026-09-03)

Two scopes, confirmed by the operator 2026-09-03.

## Scope A — this repository's own dev harness (layout only, nothing to delete)

Inventory: `AGENTS.md`, `REQUIREMENTS.md`, `ROADMAP.md`, `TODO.md`, `README.md`, and `.agents/docs/{architecture,changes,handoffs,plans,reviews,specs,testing}` already exist; no `GEMINI.md`, no hooks, no old scripts, no `.agents/workflows/`; `.claude/agent-memory/` is tracked in git (kept as is). Old-harness wording survives only in append-only history (`testing/`, `changes/`) and in `ROADMAP.md` "Next", which describes the product's seeded harness (Scope B) and stays until B lands.

- [x] `CLAUDE.md` = `@AGENTS.md`
- [x] `.agents/{rules,skills,agents,commands,workflows,scratch}/.gitkeep`
- [x] `.agents/docs/bugs/INDEX.md` (template)
- [x] `.agents/docs/deploy/{local,remote,gcp}.md` filled from `AGENTS.md`, `deploy/README.md`, `specs/styrman/{deploy,versioning}.md`; gcp not applicable
- [x] `.claude/{skills,agents,commands,rules}` → `../.agents/*` symlinks
- [x] `.gitignore`: `.claude/worktrees/`, `.agents/scratch/*`, `!.agents/scratch/.gitkeep` (inserted above the `!.env.example` last-match rule)
- [x] `AGENTS.md` `## Deploy` section pointing at the guides
- [x] Verify: `go build ./... && go vet ./...`; a headless Claude Code session in the repo sees `CLAUDE.md` → `AGENTS.md` and the project layer through the symlinks
- [x] Changelog entry, commit by name, push
- [x] Handoff with a cold-reader check (fresh session resumed from it 2026-09-03; its findings applied)

## Scope B — the harness the runtime image seeds into every project home (feature)

Today `image/harness/` ships the previous harness (constitution `shared/AGENTS.md`, 29 skills, `shared/scripts/{trail-check,resume-context,format-hook,...}.sh`, `claude/settings.json` with hooks, `antigravity/GEMINI.md`, plugins), seeded and re-seeded by `image/entrypoint.sh` keyed on `STYRMAN_VERSION`. It is to be replaced by the current harness (`styrman-harness`: `sh-agent`, 28 skills, 7 agents, no hooks, no scripts, `~/.gemini/config/rules/AGENTS.md`).

- [x] Recon brief: `.agents/docs/plans/2026-09-03-recon-image-harness.md` (`explorer`, 2026-09-03)
- [x] Feature plan with phases and task boxes (the `feature` skill): `2026-09-03-image-harness-replacement.md`; design decisions stated for the operator: how the image obtains the harness (pinned snapshot at build vs clone at boot; private-repo access via the credential broker), re-seeding an existing home, the Antigravity rules path inside the VM, retiring hooks/scripts the product documents, test tiers (integration "Tier 5" harness checks, e2e)
- [x] Cold-reader check of the plan (`reviewer`, 2026-09-03: `reviews/2026-09-03-review-plan-image-harness-replacement.md`, 3 medium / 4 low, all folded into the plan with the operator's answers)
- [ ] Phases as checkpoints; e2e before done; `ROADMAP.md` "Next" (harness assertions) folded in or closed
