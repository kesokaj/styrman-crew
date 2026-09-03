---
owner: reviewer
date: 2026-09-03
---
# Review — harness phase 2 (migration + entrypoint) (2026-09-03)

Range reviewed: `git diff -- image/entrypoint.sh` (the harness-seeding block, lines ~110-137) plus the new untracked file `image/configs/harness-migrate.sh`.

## Verdict: APPROVE

Nothing broken. The migration predicate is correctly idempotent (stamp-exists/manifest-absent), the move list matches everything `v1.19.1`'s `claude/setup.sh` and `antigravity/setup.sh` actually wrote (cross-checked file-by-file, including every `backup()`-produced `.bak.*` sibling and the retired-command/retired-agent backups, all of which land inside directories the helper moves wholesale), `mv -T` is used correctly for files, directories, and dangling symlinks (`[ ! -e ] && [ ! -L ]` correctly keeps dangling symlinks in scope), and the `if migrate && install; then stamp` shape in `entrypoint.sh` guarantees the version stamp is written only on full success — a failed `sh-agent install` can leave the old harness already backed up and the new one partially written, but never with the stamp updated, so the next boot's predicate (stamp present, manifest absent) correctly retries and the retry is a safe no-op for whatever already moved.

## Findings

No high/medium findings. Two low-severity/low-confidence observations, neither blocking:

- **[low / confidence: low]** `image/entrypoint.sh:133-135` — a single `WARN: agent harness deploy reported errors (continuing)` covers both a `harness-migrate.sh` failure and a `sh-agent install` failure; an operator diagnosing a stuck reseed has to inspect the home (does a `harness-backup-*` dir exist? does `~/.config/sh-agent/state.json` exist?) to tell which step failed. Not a correctness bug — the stamp-write gating is still safe either way — just a debuggability gap worth a follow-up log line if it ever becomes an operational pain point.
- **[low / confidence: low]** `image/entrypoint.sh:122` — the comment's specific "measured 0.02 s into an empty home" figure isn't corroborated in `.agents/docs/testing/2026-09-03-verify-harness-phase2.md`, which measured ~559 ms for the same `install` call but via `docker run` (full container-startup overhead included, not comparable). This doesn't put R1 (<5 s target, <15 s hard limit) at risk either way, but the number as stated can't be verified from the trail that exists.

## Criteria

1. Dead code — pass. The ADR-047 PATH-trim workaround (mktemp copy, trimmed-PATH `runuser`, its ~25-line comment) is removed cleanly along with the old `setup.sh` invocation; nothing orphaned.
2. Flow tracing — pass. Traced `entrypoint.sh` → `harness-migrate.sh` → `sh-agent install --repo /styrman/harness --home $HOME`; confirmed via `internal/harness/plan.go`/`apply.go`/`state.go` that `state.json` is written only after every planned file succeeds, so the stamp/manifest ordering in the entrypoint can't produce a half-migrated home with a written stamp. Confirmed `install` never shells out to `agy`/`claude` (only `git`/`diff`/`go`, and only `go`/`git pull` on the `update` path, not `install`), so dropping the PATH trim is safe as the comment claims.
3. Standards conformance — pass. `set -euo pipefail`, consistent quoting, `nullglob` scoped tightly around the one loop that needs it and unset immediately after; shellcheck (per the project's own verification log) reports no findings on either changed file.
4. Directory layout — pass. `image/configs/harness-migrate.sh` sits beside the other root-copied config templates (`profile-motd.sh`, `styrman-credentials.sh`, `tmux.conf`) that `image/Dockerfile`'s `COPY configs/ /styrman/configs/` already ships wholesale — no Dockerfile change needed, consistent with existing placement.
5. Simplicity (DIY-except-security) — pass. Pure bash using `mv`/`mkdir -p`/glob, no new dependency; no auth/crypto/session surface touched.
6. Decoupling — pass. The styrman-side migration workaround lives at the call site (`image/configs/`), not inside the vendored `image/harness/` mirror, consistent with the ADR-047 precedent the code comments cite; the mirror stays a verbatim, diffable copy of `styrman-harness`.
