# Handoff — 2026-09-03 — harness migration: scope A done, scope B planned (image-seeded harness → sh-agent)

Session ID: 77314da2 (run from the styrman-harness checkout, working on this repo)
Project: /home/kesokaj/workspace/styrman-crew
Branch: main

## Context

The operator moved their agent harness to the rebuilt `styrman-harness` (28 skills, 7 agents, `sh-agent` installer, no hooks or scripts) and wants this project on it in two senses: the repo's own dev harness (scope A) and the harness the runtime image seeds into every project microVM (scope B, `image/harness/`).

## Completed

- Scope A, commit `4cb99e2`: `CLAUDE.md` = `@AGENTS.md`; `.agents/{rules,skills,agents,commands,workflows,scratch}`; `.agents/docs/bugs/INDEX.md`; `.agents/docs/deploy/{local,remote,gcp}.md` filled from the existing deploy docs (GCP not applicable); `.claude/*` symlinks; `.gitignore` lines; `AGENTS.md` `## Deploy`. Verified headless: the import chain and the global context load. Changelog `changes/2026-09-03-harness-migration-scope-a.md`.
- Scope B recon by `explorer`: `/home/kesokaj/workspace/styrman-crew/.agents/docs/plans/2026-09-03-recon-image-harness.md`.
- Scope B feature plan, draft: `/home/kesokaj/workspace/styrman-crew/.agents/docs/plans/2026-09-03-image-harness-replacement.md` — decisions D1–D7, five phases with task boxes and verification, e2e criteria, three open questions.

## Open / In Progress

- Scope B has not started: the plan needs its cold-reader check (`reviewer`, the plan file only) and the operator's answers to its three open questions (D1 delivery model, D3 backup-vs-delete, the release version and `MINIMAL` handling) before Phase 1.
- Known traps from the recon: `internal/httpapi/setup.go:248` hardcodes the old agy agents path; ADR-047's PATH trim exists only for the old `agy -p` self-check; Tier 5 tests are manual and assert old-harness artifacts; private-repo clone from inside a VM is unproven — the plan avoids depending on it.
- Operator-owned items from the previous handoff (`2026-09-02-clone-fix-and-backlog-clearance.md`) are unchanged.

## Key Decisions

- Nothing deleted in scope A; `.claude/agent-memory/` stays tracked as before.
- Scope B recommended design: keep ADR-046's mirror model with the styrman-harness checkout at a recorded commit, build `sh-agent` in the Dockerfile, seed with `sh-agent install --repo /styrman/harness --home "$HOME_DIR"`, migrate old homes by moving artifacts to `~/.styrman/harness-backup-<ts>/`, no `sh-agent update` inside VMs.

## Corrections

- None in this repo this session.

## Agent notes (unverified)

- `sh-agent install` makes no network or CLI calls, so the R1 boot budget should hold without the PATH trim; measure before relying on it.
- The image's `agy` is installed by its upstream script at build time; the new rules path (`~/.gemini/config/rules/AGENTS.md`) is verified with agy 1.1.25 on the operator's Linux box, not yet inside the image.

## Environment State

- Branch: main; scope A pushed; this handoff and the two scope B plan files are the pending commit.
- Running processes: none. No containers were started this session.
- The operator's real home on this box has the new harness installed (`sh-agent status` no drift).

## Artifacts to Read

- `/home/kesokaj/workspace/styrman-crew/.agents/docs/plans/2026-09-03-harness-migration.md` — the two-scope checklist
- `/home/kesokaj/workspace/styrman-crew/.agents/docs/plans/2026-09-03-image-harness-replacement.md` — the scope B plan (draft)
- `/home/kesokaj/workspace/styrman-crew/.agents/docs/plans/2026-09-03-recon-image-harness.md` — recon with file:line evidence
- `/home/kesokaj/workspace/styrman-crew/.agents/docs/architecture/DECISIONS.md` — ADR-046, ADR-047 (superseded in part by the plan's ADR-062)

## Suggested Skills

- `feature` — scope B is a feature; run its cold-reader check on the plan before any code
- `golang` — `internal/httpapi/setup.go` change and its test
- `docker` — the Dockerfile builder stage and the dev image build
- `bash` — the entrypoint block and the migration helper

## Next Steps

1. Read the scope B plan; dispatch `reviewer` for its cold-reader check with the plan file only; fix gaps.
2. Get the operator's answers to the plan's three open questions; record them in the plan (and ADR-062 when Phase 1 starts).
3. Phase 1 → Phase 5 as checkpoints, one commit and changelog entry per phase; e2e criteria before done; `/preflight` for the release.
