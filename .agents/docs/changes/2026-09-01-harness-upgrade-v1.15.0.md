# Vendored agent harness re-sync — Global Agent Constitution + gates (v1.15.0)

Date: 2026-09-01. The vendored agent harness at `image/harness/` was
re-synced verbatim from its source of truth `~/workspace/devshell_prompt/harness`
(upstream commit `6132b97`, "harness: rehaul the agent spine, vendor skills,
fix shell env"), and the runtime image rebuilt with `STYRMAN_VERSION=1.15.0`
so every project home re-seeds on next boot via the existing write-once
marker (`~/.styrman/.harness-version` keyed on `/styrman/VERSION` —
ADR-030/R5). `image/harness/VENDORED.md`, the one styrman-only file kept
through the sync, was rewritten with the sync date, upstream commit, and the
two new facts below (multi-home deploy target, retirement-on-reseed
behavior).

## Files changed

- `image/harness/**` — 20 modified, 39 added, 25 deleted (136 → 150 files;
  136 − 25 + 39 = 150). Correction: an earlier version of this entry said
  "19 modified, 19 added, 25 deleted", which was wrong and self-contradictory
  (it didn't reconcile to 150). Re-measured: modified = `git status
  --porcelain -- image/harness | grep -c '^ M'` = 20; added files =
  `git ls-files --others --exclude-standard image/harness | wc -l` = 39 (the
  original "19" was a count of untracked *entries*, i.e. new directories, not
  files). No Go, web, or deploy source changed inside `image/harness/`
  itself; see the entrypoint change below for the one styrman-side source
  edit this release also carries.
- `image/harness/VENDORED.md` — rewritten: sync date, upstream commit hash,
  agent/skill roster, and (added after the independent review) the exact
  refresh command:
  `rsync -a --delete --exclude /VENDORED.md ~/workspace/devshell_prompt/harness/ image/harness/`.
  The exclude must be anchored (`/VENDORED.md`, not `VENDORED.md`) — unanchored,
  it would also skip upstream's unrelated `shared/skills/VENDORED.md`, and
  without the exclude at all, `--delete` wipes this file. Also fixed a
  wording bug: "so a harness downgrade of a home only happens on a version
  bump" read as the opposite of what was meant — the paragraph is about
  re-seeding/retiring dropped content on upgrade, not downgrading. Reworded
  to "change"/"re-seed".
- `image/entrypoint.sh` — the harness-seeding invocation (the block that
  shells out to `bash "${HSRC}/setup.sh"`) now runs with an explicit
  `PATH=/usr/bin:/bin:/usr/sbin:/sbin` instead of inheriting the default
  PATH. See "Post-review finding and fix" below — this is the one source
  change in this release and it lives in the entrypoint invocation, not
  inside the vendored `image/harness/` tree.

## Substance of the upgrade (old → new)

- Constitution `shared/AGENTS.md`: 62-line "Global Development Standards" →
  69-line "Global Agent Constitution"; adds the Resume Protocol and the
  Definition of Done dispatch table.
- Subagents 4 → 6: `testing` retired, replaced by `verifier`; `security` and
  `explorer` added; `changelog`, `documentation`, `reviewer` retained.
- New shell gates deployed to `~/.agents/bin/`: `resume-context.sh` (Claude
  `SessionStart` hook) and `trail-check.sh` (Claude `Stop` hook — blocks a
  stop when source changed but today's `changes/` entry is missing).
  Antigravity invokes the same two scripts as explicit skill steps instead
  of hooks. In HOOK MODE, both exit 0 silently outside a git repo or a
  project with no `changes/` dir. Correction: an earlier version of this
  entry said this was true of both gates unconditionally — it is not.
  `trail-check.sh --check` (the mode `/preflight` and `/handoff` invoke,
  not the `Stop`-hook mode) prints and exits 1 outside a git repo
  (`trail-check.sh:147-151`).
- Skills 23 → 28: added preflight, review-standards, grilling, grill-me,
  codebase-design, find-skills, anti-ui-slop; removed skill-creator,
  code-review.
- Claude `settings.json` now carries 4 hook events (`SessionStart`,
  `PostToolUse`, `UserPromptSubmit`, `Stop`) and is merged into any existing
  file, not overwritten.
- The harness now deploys to THREE home locations: `~/.claude`, `~/.gemini`,
  and `~/.agents/bin/` (the last is the vendor-neutral gate location shared
  by both CLIs).

## Verification

- Clean-container deploy test (empty home, `styrman-runtime:dev`):
  `setup.sh` exit 0; 69-line `CLAUDE.md` and `GEMINI.md`; 28 skills vendored
  to each; 6 agents; both gates present in `~/.agents/bin/`;
  `settings.json` valid JSON; statusline/format-hook executable.
- Real re-seed on a live project VM whose home was stamped 1.13.1 with the
  OLD harness (4 agents incl. `testing.md`, 23 skills, 62-line `CLAUDE.md`):
  the orchestrator's image-upgrade path
  (`internal/orchestrator/orchestrator.go:205-224`) recreated the stopped
  container on the new image; log shows `[styrman] agent harness deployed
  (version 1.15.0)`; marker → 1.15.0; `CLAUDE.md` → 69 lines; 28 skills; 6
  agents with `testing.md` retired to a `.bak`; `~/.agents/bin/` present;
  `settings.json` hooks = SessionStart/PostToolUse/UserPromptSubmit/Stop;
  `skill-creator` removed.
- Both gates run inside the guest against a real project repo: exit 0.
- Regression: `go build ./cmd/styrman/` OK, `go vet ./...` OK, `go test
  ./...` all packages ok.
- Post-review addition: cold-create timing on a real newly created project
  (created, timed, deleted) — container start → `project ... ready` =
  2.69s, harness deployed at 1.63s in. See "Post-review finding and fix"
  below for the full before/after measurements; this is the test the
  original verification pass was missing.

## Known side effects (honest log)

- `rsync --delete` on `~/.claude/skills/` means any skill a project user
  added by hand is removed on re-seed. Pre-existing harness behavior
  (unchanged by this sync), but it fires again for every project on first
  boot after this version bump.
- Each re-seed leaves `*.bak.<timestamp>` copies in the guest home
  (`CLAUDE.md`, `settings.json`, `statusline.sh`, the retired `testing.md`,
  and a duplicate pair of the gates because both vendor setups deploy them).
  Harmless litter, one set per version bump.
- Correction: an earlier version of this entry described the Antigravity
  setup's `agy -p "/skills"` self-check as printing a harmless "not found in
  /skills output" warning for all 28 skills when `agy` is unauthenticated,
  and called it cosmetic. That was wrong — see "Post-review finding and fix"
  below. It was not a warning, it was a 60-second hard block on the
  container boot path, on every newly created project.

## Post-review finding and fix (agy self-check blocked boot for 60s)

An independent review (`.agents/docs/reviews/2026-09-01-review-harness-vendor-sync-v1.15.0.md`,
verdict REQUEST CHANGES) found that the harness deploy, which runs
synchronously on the boot path ahead of `/run/styrman/ready`, ends with
`antigravity/setup.sh` shelling out to `agy -p "/skills"` as a self-check,
guarded only by `command -v agy`. That call hard-blocks for a fixed **60s**
whenever the home has no Antigravity OAuth token — which is EVERY newly
created project, because the token is materialized by
`/etc/profile.d/zz-styrman-credentials.sh`, a login shell that has not run
yet at entrypoint time.

- Measured before the fix: `bash /styrman/harness/setup.sh` against an empty
  home = **61s**; the isolated call `agy -p "/skills"` alone = 61s, rc=1;
  repeated with `--network none` → still 60s, confirming an internal timeout
  rather than a network stall.
- Impact: this violates REQUIREMENTS.md **R1** ("new project → agent prompt
  visible < 5s target, < 15s hard limit") by 4x, and contradicts ADR-045's
  stated "~5–10s Kata cold boot". `waitReady`'s 90s budget
  (`orchestrator.go:303,578`) absorbs the delay so nothing fails outright —
  it just silently blows the SLA on every new project.
- Why the earlier verification missed it: the one live re-seed used as
  evidence in this entry's "Verification" section ran against project
  `928e709a`, whose home already carried a valid agy token from its 1.13.1
  era — the fast path (~6.5s). No test in the original verification pass
  exercised a home with no token, which is the actual common case for new
  projects.
- Fix (`image/entrypoint.sh`, styrman-side only): the harness-seeding call
  now runs with `PATH=/usr/bin:/bin:/usr/sbin:/sbin`, trimmed of
  `/usr/local/bin` where `agy` lives, so `command -v agy` fails and
  `antigravity/setup.sh` skips the self-check entirely. Everything
  `setup.sh` actually needs — `bash`, `rsync`, `python3`, `git`, `jq` — lives
  in `/usr/bin`. This does NOT edit anything inside `image/harness/`, so
  ADR-046's verbatim-mirror rule holds; the workaround lives entirely in the
  entrypoint's invocation of the vendored script.
- Why not `timeout`: the self-check is the LAST step of
  `antigravity/setup.sh`. A `timeout` wrapped around the whole seeding call
  would kill the run before `claude/setup.sh` gets to execute, so the
  write-once marker would never be written — that would re-trigger a full
  re-deploy on every single boot, forever, which is worse than the 60s it
  would be trying to fix.
- Measured after the fix, clean-container deploy (empty home,
  `styrman-runtime:dev`): the harness seeding block itself = **0s** measured
  delta; the deploy is otherwise byte-identical to what was already
  verified — 69-line `CLAUDE.md`/`GEMINI.md`, 28 skills vendored to each
  vendor home, 6 agents each, both gates present in `~/.agents/bin/`,
  `settings.json` valid JSON, zero errors in the log.
- End-to-end proof on a real, newly created project (created, timed, then
  deleted): container start → `project ... ready` = **2.69s**, with the
  harness reported deployed at 1.63s in. This is now inside R1's < 5s
  target / < 15s hard limit, and restores ADR-045's "~5–10s Kata cold boot"
  claim.
- Honesty note: this 60s block was PRE-EXISTING upstream `setup.sh`
  behavior, not something introduced by this sync — the v1.15.0 re-sync
  re-armed it for every project going forward. The earlier verification in
  this entry missed it because the one live re-seed it relied on used a
  home that already carried a valid agy token from its 1.13.1 era (~6.5s,
  the fast case), not a freshly created, unauthenticated home.

See ADR-046 for the decision that this tree is refreshed as a verbatim
mirror, never edited in place, with a `STYRMAN_VERSION` bump as the only
delivery path, and ADR-047 (its amendment) for where styrman-side
workarounds for upstream harness behavior belong.
