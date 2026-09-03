# Harness replacement, phase 2: seeding and migration     (date: 2026-09-03)

## Summary

`image/entrypoint.sh` harness block rewritten to seed the new harness and migrate old homes. Keyed on `/styrman/VERSION` vs `~/.styrman/.harness-version`; runs `image/configs/harness-migrate.sh` then `/styrman/bin/sh-agent install --repo /styrman/harness --home $HOME` as the project user, each under `timeout 20`, with `GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1`. Install succeeds even after partial migration; the stamp is written only when both succeed. Two distinct WARN lines on errors; ADR-047's 20-line PATH trim and its comment removed (install makes no CLI or network call, measured 0.02 s on host, ~0.5 s through `docker run`). New `image/configs/harness-migrate.sh` (bash, `set -euo pipefail`) moves every path the v1.19.1 `claude/setup.sh` and `antigravity/setup.sh` wrote — including both vendors' `settings.json` as whole files per operator directive — into `~/.styrman/harness-backup-<YYYYmmdd-HHMMSS>/`, never deletes, continues past an unmovable path (exit 1 at end), strips control characters from logged names, idempotent (second run: predicate false, no-op). MINIMAL projects still skip the block.

## Type

feature, phase 2 of 5

## Files changed

- `image/entrypoint.sh` — rewritten harness block: stamp check, timeout-wrapped migrate + install, two distinct WARN lines, stamp written on success; ADR-047 PATH trim and 20-line comment removed
- `image/configs/harness-migrate.sh` — new: D3 predicate (stamp exists, no `~/.config/sh-agent/state.json`); moves `.claude/{CLAUDE.md,settings.json,skills,agents,commands,statusline.sh,format-hook.sh,feedback-capture.sh}`, `.gemini/{GEMINI.md,settings.json,format-hook.sh,config/skills,config/plugins/harness,config/global_workflows}`, `.agents/bin`, every `*.bak.*` sibling of the above; logs each move; idempotent

## Agents dispatched

verifier (`.agents/docs/testing/2026-09-03-verify-harness-phase2.md` PASS; round 2 PASS), reviewer (`.agents/docs/reviews/2026-09-03-review-harness-phase2.md` APPROVE, 2 low: distinct WARN lines and timing claim — both fixed), security (`.agents/docs/reviews/2026-09-03-security-harness-phase2.md`: round 1 = 1 medium / 5 low — no timeout ahead of `ready`, git config honouring user `~/.gitconfig`, `set -e` aborting inventory, unsanitized names in log; round 2 = closed after fixes; newlow items: 60 s vs 90 s timeouts, symlinked `~/.styrman`, `Install` return on first error — remaining items upstream in TODO.md).

## Verification

All on the host, no VM:
- Clean home: `HOME=$H docker run --rm -v "$H:/h" --entrypoint /styrman/bin/sh-agent styrman-runtime:dev install --repo /styrman/harness --home /h` → `$H/.claude/CLAUDE.md` is two-line stub, 28 skills in `~/.claude/skills`, 7 agy agents, `sh-agent status --repo image/harness --home $H` no drift
- Old-home fixture seeded by v1.19.1 setup + stamp → `HOME=$H2 bash image/configs/harness-migrate.sh` + install → every D3 path under `~/.styrman/harness-backup-*/`, none left in home, planted user files byte-identical, second run is no-op
- Unmovable dir (`chmod 500 ~/.claude`) → moves the rest, two WARNs, exit 1
- Newline in a name → one clean log line (control characters stripped)
- Boot→ready: 1.69 s measured in integration run (within R1's 15 s hard limit)

## Requirements

R1 (boot budget) measured; R3 (credentials, `~/.gemini/antigravity-cli`, `~/projects` never moved) untouched.

## Related

- ADR-062 (D2–D3: seeding model and migration)
- Plan: `.agents/docs/plans/2026-09-03-image-harness-replacement.md`
