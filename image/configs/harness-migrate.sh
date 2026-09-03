#!/usr/bin/env bash
# harness-migrate.sh — move the artifacts of the PREVIOUS image-seeded agent
# harness (devshell_prompt/harness, shipped v1.15.0 – v1.19.x) out of a project
# home before sh-agent installs the current one (styrman-harness, ADR-062).
#
# Runs as the project user with HOME set, from image/entrypoint.sh, ahead of
# `sh-agent install`. Styrman-side by design (ADR-047): the harness mirror
# itself knows nothing about what came before it.
#
# Predicate: ~/.styrman/.harness-version exists (the stamp every previous
# version wrote) and ~/.config/sh-agent/state.json does not (sh-agent has
# never installed into this home). Both true → this home was seeded by the
# old harness and never migrated.
#
# Action: MOVE, never delete (operator directive 2026-09-03), every path the
# old claude/setup.sh and antigravity/setup.sh wrote — inventoried from those
# scripts — into ~/.styrman/harness-backup-<ts>/, keeping the path relative
# to $HOME. Whole harness-owned trees move (the old harness rsync'd skills/
# with --delete and re-seeded agents/ on every version bump, so anything a
# user dropped in there was already at its mercy; the backup keeps it
# recoverable). Nothing else in the home is touched: not ~/.claude/projects,
# history or credentials, not ~/.gemini/antigravity-cli (agy's own state and
# the broker-delivered token, R3), not other plugins, not ~/projects.
#
# Idempotent: after a successful `sh-agent install` the manifest exists and
# the predicate is false; after a failed one the paths are already gone and
# a re-run moves nothing (the backup dir is only created when something moves).
# Exit 1 when a path could not be moved (the caller still installs; the
# stamp is withheld so the WARN repeats until the operator looks).
set -euo pipefail

HOME_DIR="${HOME:?HOME must be set}"
STAMP="${HOME_DIR}/.styrman/.harness-version"
MANIFEST="${HOME_DIR}/.config/sh-agent/state.json"

log() { echo "[harness-migrate] $*"; }

if [ ! -e "${STAMP}" ]; then
    log "no previous harness stamp — nothing to migrate"
    exit 0
fi
if [ -e "${MANIFEST}" ]; then
    log "sh-agent manifest present — already migrated"
    exit 0
fi

BACKUP="${HOME_DIR}/.styrman/harness-backup-$(date +%Y%m%d-%H%M%S)"

# Fixed paths the previous harness wrote, relative to $HOME.
PATHS=(
    .claude/CLAUDE.md
    .claude/settings.json
    .claude/skills
    .claude/agents
    .claude/commands
    .claude/statusline.sh
    .claude/format-hook.sh
    .claude/feedback-capture.sh
    .gemini/GEMINI.md
    .gemini/settings.json
    .gemini/format-hook.sh
    .gemini/config/skills
    .gemini/config/plugins/harness
    .gemini/config/global_workflows
    .agents/bin
)
# The `.bak.<timestamp>` copies its own backup() left beside each file on
# every re-seed. Only the directories that STAY need a glob: the copies inside
# .claude/agents, .claude/commands and .agents/bin move with their tree.
BAK_GLOBS=(
    ".claude/*.bak.*"
    ".gemini/*.bak.*"
    ".gemini/config/*.bak.*"
)

# Names reach the container log, which the ops panel shows: strip control
# characters so a user-chosen file name cannot forge a log line.
plain() { printf '%s' "$1" | tr -d '\000-\037\177'; }

moved=0
failed=0
move() {
    local rel="$1" src="${HOME_DIR}/$1" dst="${BACKUP}/$1"
    if [ ! -e "${src}" ] && [ ! -L "${src}" ]; then
        return 0
    fi
    # Never abort the inventory on one unmovable path (a chmod'ed dir): move
    # the rest, report, and let the caller decide — the exit status says so.
    if mkdir -p "$(dirname "${dst}")" && mv -T "${src}" "${dst}"; then
        log "moved $(plain "${rel}")"
        moved=$((moved + 1))
    else
        log "WARN: could not move $(plain "${rel}") — left in place"
        failed=$((failed + 1))
    fi
}

for p in "${PATHS[@]}"; do
    move "${p}"
done
shopt -s nullglob
for g in "${BAK_GLOBS[@]}"; do
    for f in "${HOME_DIR}"/${g}; do
        move "${f#"${HOME_DIR}"/}"
    done
done
shopt -u nullglob

if [ "${moved}" -eq 0 ] && [ "${failed}" -eq 0 ]; then
    log "previous harness stamp present but nothing left to move"
else
    log "moved ${moved} path(s) to ${BACKUP}; ${failed} left in place"
fi
exit $(( failed == 0 ? 0 : 1 ))
