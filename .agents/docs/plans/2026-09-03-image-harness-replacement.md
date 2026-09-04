---
owner: main
date: 2026-09-03
status: shipped in v1.20.0 (2026-09-04); manual Tier 5 step 4 pending on the operator — cold-read by `reviewer` 2026-09-03 (`reviews/2026-09-03-review-plan-image-harness-replacement.md`, 3 medium / 4 low, all resolved below); the operator's answers to the open questions recorded 2026-09-03
---
# Plan — replace the image-seeded harness with sh-agent (styrman-harness)

Scope as read: the runtime image stops shipping the previous harness (`image/harness/`: constitution, 29 skills, shell gates, hooks, `settings.json`, `GEMINI.md`, agy plugin) and seeds the current one — `sh-agent install` from the `styrman-harness` repo (28 skills, 7 agents, `~/.gemini/config/rules/AGENTS.md`, no hooks, no scripts, no settings files) — into every project home, on first boot and on version bump, migrating homes that carry the old one. The settings UI, the tests, and the docs follow. Confirmed by the operator 2026-09-03 ("Yes. We are going to replace that as well." and, mid-session: "the new harness should fully replace the old. ALL old occurrences of the old harness need to go. Even settings.json files for gemini and claude"). Recon: `2026-09-03-recon-image-harness.md`.

## Operator's answers (2026-09-03), applied throughout

1. D1: mirror at a recorded commit (not a submodule).
2. D3: move the old artifacts to `~/.styrman/harness-backup-<ts>/`, never delete — and the old `settings.json` files of both vendors go with them (whole files, not a key strip).
3. The release is a minor bump (`v1.20.0`); `MINIMAL` projects do not get the harness (unchanged: the setup VM runs `STYRMAN_MINIMAL=1`).
4. D5: repoint the settings roster at a seeded project home and filter `*.bak.*`.

No reason was found not to follow any of them. One consequence to state: with no `settings.json` seeded, Claude Code inside a project runs in its default permission mode (the old file set `permissions.defaultMode: auto`, `attribution` empty, `effortLevel: high`); the operator's own machine runs the new harness with exactly that — no `permissions` block, no `GEMINI.md`, no `~/.gemini/settings.json` — so this is the known baseline, not a regression to hide.

## Decisions

- **D1 Delivery — mirror at a recorded commit (ADR-046's model, new source).** `image/harness/` is a verbatim copy of the `styrman-harness` checkout at the SHA recorded in `image/harness/COMMIT`, refreshed by hand from `~/workspace/styrman-harness` with `rsync -a --delete --exclude .git --exclude .agents --exclude .gitignore --exclude /COMMIT --exclude /VENDORED.md ~/workspace/styrman-harness/ image/harness/` (the nested `.gitignore` is excluded so it cannot ignore files inside this repo; the two anchored excludes are styrman's own files in the mirror dir). The Dockerfile builds `sh-agent` from the mirror in a builder stage. Rejected: a submodule (every clone incl. the updater's `/opt/styrman` must recurse); `git clone` at boot (private repo, no credentials on first boot, R1 budget).
- **D2 Seeding.** `image/entrypoint.sh` runs `runuser -u "$USERNAME" -- env HOME="$HOME_DIR" /styrman/bin/sh-agent install --repo /styrman/harness --home "$HOME_DIR"`, keyed on the existing `~/.styrman/.harness-version` stamp. What `install` does (verified in the styrman-harness source 2026-09-03): copies files, writes `~/.config/sh-agent/state.json` (the manifest) and a copy of itself to `~/.local/bin/sh-agent`; it runs `git rev-parse` in the repo dir to record the commit and discards the error when there is no `.git` (the mirror has none, so the manifest's `commit` is empty — `image/harness/COMMIT` is the record; `sh-agent status` prints a blank commit, accepted); it never invokes `agy`, `claude`, or the network. Measured on the host: 256 files into an empty home in 0.02 s. The ADR-047 PATH trim (there to dodge the old `agy -p` self-check) is therefore retired; boot is still timed against R1. `~/.local/bin` is already on the interactive PATH (`image/skel/bashrc:23`).
- **D3 Migration of homes seeded by the old harness.** Predicate: `~/.styrman/.harness-version` exists and `~/.config/sh-agent/state.json` does not (seeded, never by sh-agent) — no version-string comparison. Action: move, never delete, into `~/.styrman/harness-backup-<YYYYmmdd-HHMMSS>/` keeping the path relative to `$HOME`, everything the old `image/harness/{claude,antigravity}/setup.sh` wrote (inventoried 2026-09-03 from those scripts):
  - `.claude/CLAUDE.md` (the old constitution — `sh-agent` never overwrites a `CLAUDE.md` it did not write, so the two-line import cannot land otherwise), `.claude/settings.json` (whole file; it carried `hooks`, `statusLine`, `permissions`, `attribution`, `effortLevel`, `theme`, `verbose`, `fileCheckpointingEnabled`, `cleanupPeriodDays`), `.claude/skills/`, `.claude/agents/`, `.claude/commands/`, `.claude/statusline.sh`, `.claude/format-hook.sh`, `.claude/feedback-capture.sh`
  - `.gemini/GEMINI.md`, `.gemini/settings.json` (if present), `.gemini/format-hook.sh`, `.gemini/config/skills/`, `.gemini/config/plugins/harness/`, `.gemini/config/global_workflows/` (if present)
  - `.agents/bin/`
  - every `*.bak.*` sibling the old `backup()` left in `.claude/`, `.claude/agents/`, `.claude/commands/`, `.gemini/`, `.agents/bin/`
  Whole harness-owned trees move (not a file list inside them): the old harness rsync'd `skills/` with `--delete` and re-seeded `agents/` on every version bump, so anything a user dropped in there was already at the harness's mercy; the backup keeps it recoverable. Never touched: anything else under `~/.claude/` (history, `projects/`, credentials), `~/.gemini/antigravity-cli/` (agy's own state and the broker-delivered OAuth token, R3), other plugins under `~/.gemini/config/plugins/`, `~/projects/`, the user's dotfiles. The two skills the new harness dropped (`grill-me`, `skill-repair`) need no special case: they go with `.claude/skills/` and `.gemini/config/skills/`. The helper lives in `image/configs/harness-migrate.sh` (call site, per ADR-047), not in the mirror; it logs each move and is idempotent (second boot: predicate false, no-op).
- **D4 No `sh-agent update` inside VMs.** Updates arrive with the image, as today (ADR-046 unchanged on delivery). `sh-agent status` inside a project is the self-check the tests use.
- **D5 Settings UI.** `internal/httpapi/setup.go` `toolDetails`: the roster (`Skills`, `Agents`, `Commands`) is read from the user's most recently created project whose home has `~/.config/sh-agent/state.json` (the base home is never seeded — `TODO.md` Inbox item "Settings harness roster is permanently empty"); when no project qualifies the fields stay empty as today. agy `Agents` come from `.gemini/config/agents/*/agent.md` (directory names), not `plugins/harness/agents`. `listNames` ignores `*.bak.*`. `Commands` will read 0 (the harness ships none; `/name` skills are the commands) — the UI copy must not imply otherwise.
- **D6 Tests.** Tier 5 stops being manual: `integration.sh` asserts, inside a booted project, `sh-agent status` exit 0 with no drift, the absence of `~/.agents/bin`, `~/.gemini/GEMINI.md`, `~/.claude/settings.json`, and a boot-latency bound — the `ROADMAP.md` "Next" item.
- **D7 Records.** ADR-062 supersedes ADR-046's mirror *source* and ADR-047's PATH trim; R5 (identical defaults for every project) stands; `versioning.md` records the harness commit per release.
- **D8 No styrman-owned settings files.** Neither vendor gets a `settings.json` from the image (operator directive 2026-09-03). Nothing in `image/` seeds one.

## Phase 1 — vendor and build (files and Dockerfile only)

- [x] `image/harness/` replaced by the mirror (D1 rsync command): `context/`, `skills/`, `agents/`, `commands/`, `rules/`, `cmd/`, `internal/`, `go.mod`, the harness repo's own `README.md`/`AGENTS.md`/`CLAUDE.md`/`REQUIREMENTS.md`/`ROADMAP.md`/`TODO.md` (verbatim; not read by anything here), plus `COMMIT` (the styrman-harness SHA) and a rewritten `VENDORED.md` (provenance, the refresh command, the `diff -rq` check against the checkout at that SHA, the blank-commit note from D2)
- [x] `image/Dockerfile`: builder stage `FROM golang:<same pin as deploy/Dockerfile.server> AS harness-builder` → `go build -o /out/sh-agent ./cmd/sh-agent` from the mirror; runtime gets `/styrman/bin/sh-agent` and `/styrman/harness/`; `COPY harness/` line and comment updated
- [x] Old `image/harness/{setup.sh,shared/,claude/,antigravity/,README.md}` removed (the backup is git: `git show v1.19.1:image/harness/...`)
- Verification: `docker build -t styrman-runtime:dev --build-arg STYRMAN_VERSION=dev-$(date +%s) image/` succeeds; `docker run --rm --entrypoint /styrman/bin/sh-agent styrman-runtime:dev list --repo /styrman/harness` shows 28 skills, 7 agents; `diff -rq --exclude .git --exclude .agents --exclude .gitignore ~/workspace/styrman-harness image/harness` reports only `COMMIT` and `VENDORED.md`

## Phase 2 — seeding and migration (entrypoint)

- [x] `image/entrypoint.sh` harness block: when the stamp differs from `/styrman/VERSION`: run `image/configs/harness-migrate.sh` (as the user, `HOME` set), then the D2 `sh-agent install`; stamp written on success; non-fatal and logged on failure; ADR-047 PATH trim and its comment removed
- [x] `image/configs/harness-migrate.sh` (bash; `set -euo pipefail`; input `HOME`): the D3 predicate and move list, one `mv` per path into the timestamped backup dir, a log line per move, exit 0 when nothing to do
- [x] `MINIMAL` mode still skips the whole block (operator answer 3)
- Verification, all on the host, no VM (the real upgrade is Phase 5):
  - clean home: `H=$(mktemp -d .agents/scratch/home.XXXX)`; `docker run --rm -v "$H:/h" -e HOME=/h --entrypoint /styrman/bin/sh-agent styrman-runtime:dev install --repo /styrman/harness --home /h` → `$H/.claude/CLAUDE.md` is the two-line stub, `ls $H/.claude/skills | wc -l` = 28, `ls $H/.gemini/config/agents | wc -l` = 7, `sh-agent status --repo image/harness --home $H` no drift
  - old-home fixture: `git archive v1.19.1 image/harness | tar -x -C .agents/scratch/` then `HOME=$H2 bash .agents/scratch/image/harness/setup.sh` (this is exactly what the old entrypoint ran, as the user with `HOME` set; needs bash, rsync, python3 — all on the host) and `mkdir -p $H2/.styrman && echo 1.19.1 > $H2/.styrman/.harness-version`; then `HOME=$H2 bash image/configs/harness-migrate.sh` followed by the clean-home `install` above against `$H2` → `$H2/.styrman/harness-backup-*/` holds every D3 path that existed, `$H2/.claude/settings.json` and `$H2/.gemini/GEMINI.md` and `$H2/.agents/bin` are gone from the home, a user file planted beforehand (`$H2/.claude/projects/keep.txt`, `$H2/.gemini/antigravity-cli/keep.txt`) is byte-identical, `sh-agent status` no drift; a second `harness-migrate.sh` run is a no-op
  - boot latency: Phase 5 (real VM), compared with `.agents/docs/testing/styrman/README.md` Tier 5 3a (~1 s today)

## Phase 3 — server

- [x] `internal/httpapi/setup.go` `toolDetails`: roster source per D5 (the qualifying project's home), agy `Agents` from `.gemini/config/agents` (directory names), `listNames` ignores `*.bak.*`; test in `internal/httpapi/` with a fixture home holding `.claude/agents/{a.md,b.md.bak.20260901-065226}`, `.claude/skills/{x,y}/SKILL.md`, `.gemini/config/agents/{a,c}/agent.md`, and a `.config/sh-agent/state.json`
- [x] Settings page copy: `Commands` reads 0 by design (D5) — either hide a zero count or label it; the frontend change is small
- Verification: `go test ./internal/httpapi/ -race`; the settings page shows 7 agents for both tools on a dev project with at least one seeded project

## Phase 4 — tests and docs

- [x] `.agents/docs/testing/styrman/integration.sh`: the D6 assertions (docker exec into the created project; count in `README.md` updated)
- [x] `.agents/docs/testing/styrman/README.md` Tier 5 rewritten around `sh-agent status`; Tier 4 note on the private-repo clone left as history
- [x] `image/README.md`, `README.md` (harness paragraph), `AGENTS.md` (Setup Commands comment "the version keys harness re-seeding" stays true; R5's "vendored from `devshell_prompt/harness`" wording is in `REQUIREMENTS.md` — immutable without the operator: propose the one-line edit "vendored from `styrman-harness`" at Phase 4 and apply only on approval, with ADR-062)
- [x] `.agents/docs/architecture/DECISIONS.md` ADR-062 (D1–D8); `.agents/docs/specs/styrman/versioning.md` harness-commit pin per release; `ROADMAP.md` "Next" harness-assertion item closed; `TODO.md` Inbox item "Settings harness roster is permanently empty" ticked (Phase 3 is its fix); changelog
- Verification: `integration.sh` green with the new count; docs cold-read by the `documentation` agent

## Phase 5 — e2e and release

- [~] Dev image built and run locally; new project created; `docker exec` → `sh-agent status` no drift; boot latency recorded (1.69 s, `testing/2026-09-03-e2e-harness-upgrade-path.md`); `claude -p` / `agy` inside a project: manual, needs the operator's sign-in — pending on the released version
- [x] Upgrade path on a real existing dev project (stop, start on the new image) → migration evidence (backup dir listing, `settings.json` gone, user files intact) — `testing/2026-09-03-e2e-harness-upgrade-path.md`
- [x] `/preflight` for `v1.20.0` — shipped 2026-09-04 (`reviews/2026-09-04-preflight-v1.20.0.md`, tag `v1.20.0` on `1a172db`, deployed by the auto-updater 200 s after the push)
- Verification: e2e criteria below; `deploy/verify-commit.sh` on the release commit

## E2E criteria

Golden path: a new project on the new image reports ready within R1; inside it `sh-agent status` says no drift, `~/.claude/CLAUDE.md` is the two-line import, `ls ~/.claude/skills | wc -l` is 28, `ls ~/.gemini/config/agents | wc -l` is 7, no `~/.claude/settings.json`, no `~/.gemini/GEMINI.md`, no `~/.agents/bin`, and `claude -p` quotes `# AGENTS.md — global agent context`.
Edge: a project home seeded by v1.19.x restarts on the new image: every D3 path is under `~/.styrman/harness-backup-*`, none remain in the home, `~/projects/`, `~/.gemini/antigravity-cli/`, and the user's own dotfiles are byte-identical before and after, and a second restart changes nothing.

## Resolved review findings (2026-09-03)

Medium: D3 skill list corrected (only `grill-me`, `skill-repair` dropped; `research` stays — and the whole-tree move makes the list moot); D3 settings keys — the whole file moves (operator); contradictions removed (decisions are decided, open questions closed). Low: the `TODO.md` item is named, not numbered; D2 states the `git rev-parse` shell-out; the old-home fixture has literal commands; Phase 4 ticks the roster item.

## Progress notes (2026-09-03)

- Phases 1–3 implemented and gated (verifier rounds 1–2, reviewer, security rounds 1–2 — reports under `testing/` and `reviews/` named `…-harness-phase*`). Security added to the plan's scope: `image/harness/SHA256SUMS` + `deploy/verify-commit.sh` mirror/runtime-image/pin checks, `deploy/vulncheck.sh` third module, `GOPROXY=off`, `image/.dockerignore`, `chmod -R go-w`, `timeout 60` + `GIT_CONFIG_GLOBAL=/dev/null` on the seeding calls, a helper that continues past an unmovable path, and — BUG-002 — `files.Trust` tenant confinement for every root under a bind-mounted home.
- Phase 4: docs written (`runtime-image.md`, `image/README.md`, `versioning.md`, `ROADMAP.md`, `TODO.md`, ADR-062); `documentation` cold-read dispatched; `integration.sh` run against `styrman-runtime:dev` in progress. `REQUIREMENTS.md` R5 wording ("vendored from `devshell_prompt/harness`") awaits the operator's approval: proposed edit — "The agent harness (a mirror of `styrman-harness` at `image/harness/COMMIT`, installed by `sh-agent`)".
