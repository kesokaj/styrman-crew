---
owner: main
date: 2026-09-03
status: draft — awaiting cold-reader check and the operator's decisions D1–D3
---
# Plan — replace the image-seeded harness with sh-agent (styrman-harness)

Scope as read: the runtime image stops shipping the previous harness (`image/harness/`: constitution, 29 skills, shell gates, hooks, `GEMINI.md`, agy plugin) and seeds the current one — `sh-agent install` from the `styrman-harness` repo (28 skills, 7 agents, `~/.gemini/config/rules/AGENTS.md`, no hooks, no scripts) — into every project home, on first boot and on version bump, migrating homes that carry the old one. The settings UI, the tests, and the docs follow. Confirmed by the operator 2026-09-03 ("Yes. We are going to replace that as well."). Recon: `2026-09-03-recon-image-harness.md`.

## Decisions (D1–D3 need the operator; D4–D7 are proposed as consequences)

- **D1 Delivery — recommended: keep ADR-046's model.** `image/harness/` becomes a verbatim mirror of the styrman-harness checkout at a recorded commit (`image/harness/COMMIT`), refreshed by hand from `~/workspace/styrman-harness` (rsync, excluding `.git/`, `.agents/`, `.gitignore`), and the Dockerfile builds `sh-agent` from it in a builder stage. Alternatives: a git submodule at the pinned SHA (git-native pin, but every clone including the auto-updater's `/opt/styrman` must recurse); `git clone` at boot (rejected: needs private-repo credentials on first boot, when no token exists, and violates R1's boot budget).
- **D2 Seeding.** `image/entrypoint.sh` runs `sh-agent install --repo /styrman/harness --home "$HOME_DIR"` as the project user, keyed on the existing `~/.styrman/.harness-version` stamp. `install` is a file copier: no network, no CLI shell-outs, so the ADR-047 PATH trim (there to dodge a 60 s `agy -p` self-check) is retired, and boot time is measured against R1.
- **D3 Migration of homes seeded by the old harness** (stamp present and older than the first version carrying this change): move, never delete, into `~/.styrman/harness-backup-<timestamp>/`: `~/.claude/CLAUDE.md` (the old constitution — `sh-agent` never overwrites a `CLAUDE.md` it did not write, so the two-line import cannot land otherwise), `~/.gemini/GEMINI.md`, `~/.agents/bin/`, `~/.gemini/config/plugins/harness/`, `~/.claude/{format-hook.sh,feedback-capture.sh,statusline.sh}`, `~/.claude/commands/*` from the old set, and the three skills the new harness dropped (`research`, `grill-me`, `skill-repair`) from both vendors. Strip the `hooks` and `statusLine` keys the old `merge-settings.py` added from `~/.claude/settings.json`; every other key stays. User-created files are never touched. The helper lives in `image/` (call site, per ADR-047), not in the mirror.
- **D4 No `sh-agent update` inside VMs.** Updates arrive with the image, as today (ADR-046 unchanged on delivery). `sh-agent status` inside a project is the self-check the tests use.
- **D5 Settings UI.** `internal/httpapi/setup.go` `toolDetails`: agy agents are read from `.gemini/config/agents/*/agent.md` (not `plugins/harness/agents`); Claude paths unchanged.
- **D6 Tests.** Tier 5 stops being manual: `integration.sh` asserts, inside a booted project, `sh-agent status` exit 0 with no drift, the absence of `~/.agents/bin`, `~/.gemini/GEMINI.md`, and a `hooks` key, and a boot-latency bound — the `ROADMAP.md` "Next" item.
- **D7 Records.** ADR-062 supersedes ADR-046's mirror *source* and ADR-047's PATH trim; R5 (identical defaults for every project) stands; `versioning.md` records the harness commit per release.

## Phase 1 — vendor and build (files and Dockerfile only)

- [ ] `image/harness/` replaced by the mirror: `context/`, `skills/`, `agents/`, `commands/`, `rules/`, `cmd/`, `internal/`, `go.mod`, `README.md`, plus `COMMIT` (the styrman-harness SHA) and a rewritten `VENDORED.md` (provenance, refresh procedure, the `diff -rq` check against the checkout at that SHA)
- [ ] `image/Dockerfile`: builder stage `go build -o /out/sh-agent ./cmd/sh-agent` from the mirror; runtime gets `/styrman/bin/sh-agent` and `/styrman/harness/`; `COPY harness/` line and comment updated
- [ ] Old `image/harness/{setup.sh,shared/,claude/,antigravity/}` removed (the backup is git)
- Verification: `docker build` at a dev version succeeds; `docker run --rm --entrypoint /styrman/bin/sh-agent <img> list --repo /styrman/harness` shows 28 skills, 7 agents; `diff -rq` against the checkout is empty

## Phase 2 — seeding and migration (entrypoint)

- [ ] `image/entrypoint.sh` harness block: `runuser -u "$USERNAME" -- env HOME="$HOME_DIR" /styrman/bin/sh-agent install --repo /styrman/harness --home "$HOME_DIR"`, stamp written on success, non-fatal and logged on failure, ADR-047 PATH trim and its comment removed; `~/.local/bin` on the user's PATH (check the login shell profile the image seeds)
- [ ] `image/configs/harness-migrate.sh` (python3 or sh): runs before `install` when the stamp exists and predates the cut-over version; moves the D3 list into `~/.styrman/harness-backup-<ts>/`, strips the two settings keys, logs each move
- [ ] `MINIMAL` mode still skips the whole block
- Verification: clean home → stamp written, `sh-agent status --home` no drift, `~/.claude/CLAUDE.md` is the two-line stub, boot within R1 (timed against the current image); old-home fixture (a home seeded by the current image in a throwaway container) → backup dir holds the old files, `settings.json` has no `hooks`, user files untouched, second boot is a no-op

## Phase 3 — server

- [ ] `internal/httpapi/setup.go` `toolDetails`: agy `Agents` from `.gemini/config/agents` (directory names); test in `internal/httpapi/` with a fixture home
- Verification: `go test ./internal/httpapi/ -race`; the settings page shows 7 agents for both tools on a dev project

## Phase 4 — tests and docs

- [ ] `.agents/docs/testing/styrman/integration.sh`: the D6 assertions (docker exec into the created project; count in `README.md` updated)
- [ ] `.agents/docs/testing/styrman/README.md` Tier 5 rewritten around `sh-agent status`; Tier 4 note on the private-repo clone left as history
- [ ] `image/README.md`, `README.md` (harness paragraph), `AGENTS.md` (Setup Commands comment "the version keys harness re-seeding" stays true)
- [ ] `.agents/docs/architecture/DECISIONS.md` ADR-062; `.agents/docs/specs/styrman/versioning.md` harness-commit pin per release; `ROADMAP.md` "Next" harness-assertion item closed; changelog
- Verification: `integration.sh` green with the new count; docs cold-read by the `documentation` agent

## Phase 5 — e2e and release

- [ ] Dev image built and run locally; new project created; `docker exec` → `sh-agent status` no drift; `claude -p` inside the project answers the context question (first heading, 28 skills, `/grilling`); `agy` after the operator signs in (manual, recorded)
- [ ] Upgrade path on a real existing dev project (stop, start on the new image) → migration evidence
- [ ] `/preflight` for the release; version chosen by the operator (minor bump proposed)
- Verification: e2e criteria below; `deploy/verify-commit.sh` on the release commit

## E2E criteria

Golden path: a new project on the new image reports ready within R1; inside it `sh-agent status` says no drift, `~/.claude/CLAUDE.md` is the two-line import, `ls ~/.claude/skills | wc -l` is 28, `ls ~/.gemini/config/agents | wc -l` is 7, and `claude -p` quotes `# AGENTS.md — global agent context`.
Edge: a project home seeded by v1.19.x restarts on the new image: the old constitution, `GEMINI.md`, shell gates, and hook scripts are under `~/.styrman/harness-backup-*`, `settings.json` has no `hooks` key, `~/projects/` and the user's own dotfiles are byte-identical before and after.

## Open questions for the operator (block Phase 1)

1. D1: mirror at a recorded commit (recommended) or a git submodule?
2. D3: is moving the old artifacts to `~/.styrman/harness-backup-<ts>/` acceptable, or delete outright?
3. The version that carries this (minor bump, e.g. v1.20.0?) and whether `MINIMAL` projects should also get the new harness.
