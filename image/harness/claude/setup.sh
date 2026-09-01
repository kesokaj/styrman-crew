#!/usr/bin/env bash
set -euo pipefail

# =============================================================================
# claude/setup.sh — Deploy the harness for Claude Code.
#
# Source: ../shared/   (single source of truth, AGENTS.md framework)
# Target: ~/.claude/
#
# Deploys:
#   shared/AGENTS.md   → ~/.claude/CLAUDE.md
#   shared/skills/     → ~/.claude/skills/      (includes the /audit, /retro, … workflow skills)
#   claude/commands/   → ~/.claude/commands/    (overlay; Claude-specific extras, if any)
#   shared/scripts/{resume-context.sh,trail-check.sh} → ~/.agents/bin/  (vendor-neutral shell gates)
#
# Also refreshes the in-repo CLAUDE.md copy from shared/AGENTS.md so it stays
# in sync with the source of truth for inspection.
#
# Usage:
#   ./claude/setup.sh              # Deploy everything
#   ./claude/setup.sh --dry-run    # Show what would happen without changes
# =============================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
HARNESS_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
SHARED_DIR="${HARNESS_DIR}/shared"

CLAUDE_HOME="${HOME}/.claude"
CC_SKILLS="${CLAUDE_HOME}/skills"
CC_COMMANDS="${CLAUDE_HOME}/commands"
CC_AGENTS="${CLAUDE_HOME}/agents"

DRY_RUN=false

while [[ $# -gt 0 ]]; do
    case "$1" in
        --dry-run) DRY_RUN=true; shift ;;
        *)
            echo "Unknown option: $1"
            echo "Usage: ./claude/setup.sh [--dry-run]"
            exit 1
            ;;
    esac
done

log() { echo "[claude] $*"; }

run() {
    if [[ "$DRY_RUN" == true ]]; then
        log "[dry-run] $*"
    else
        log "$*"
        bash -c "$*"
    fi
}

# Back up an existing file/dir before it is overwritten (REQUIREMENTS.md §Security:
# "Installer must back up existing dotfiles before overwriting").
backup() {
    local target="$1"
    if [[ -e "$target" ]]; then
        run "cp -a '$target' '${target}.bak.$(date +%Y%m%d-%H%M%S)'"
    fi
}

# ---------------------------------------------------------------------------
# Step 1: Refresh the in-repo CLAUDE.md copy from the shared source
# ---------------------------------------------------------------------------
log "=== Refreshing in-repo CLAUDE.md from shared/AGENTS.md ==="

if [[ ! -f "${SHARED_DIR}/AGENTS.md" ]]; then
    echo "Error: ${SHARED_DIR}/AGENTS.md not found"
    exit 1
fi

run "cp '${SHARED_DIR}/AGENTS.md' '${SCRIPT_DIR}/CLAUDE.md'"
log "✅ shared/AGENTS.md → claude/CLAUDE.md"

# ---------------------------------------------------------------------------
# Step 2: Deploy global CLAUDE.md
# ---------------------------------------------------------------------------
log "=== Deploying global CLAUDE.md ==="

run "mkdir -p '${CLAUDE_HOME}'"
backup "${CLAUDE_HOME}/CLAUDE.md"
run "cp '${SCRIPT_DIR}/CLAUDE.md' '${CLAUDE_HOME}/CLAUDE.md'"
log "✅ claude/CLAUDE.md → ${CLAUDE_HOME}/CLAUDE.md"

# ---------------------------------------------------------------------------
# Step 3: Deploy skills
# ---------------------------------------------------------------------------
log "=== Deploying skills ==="

if [[ ! -d "${SHARED_DIR}/skills" ]]; then
    echo "Error: ${SHARED_DIR}/skills/ not found"
    exit 1
fi

run "mkdir -p '${CC_SKILLS}'"
run "rsync -av --delete '${SHARED_DIR}/skills/' '${CC_SKILLS}/'"
log "✅ shared/skills/ → ${CC_SKILLS}/"

# ---------------------------------------------------------------------------
# Step 4: Slash commands.
# The five global workflows migrated into shared/skills/ (a skill at
# skills/<name>/SKILL.md provides /<name>, same as commands/<name>.md did), so
# retire the previously deployed copies to avoid duplicate command definitions.
# Claude-specific extras in claude/commands/ (if any) still overlay.
# ---------------------------------------------------------------------------
log "=== Slash commands (migrated into skills) ==="

MIGRATED_COMMANDS=(audit.md distill.md init-project.md research.md retro.md)
for cmd in "${MIGRATED_COMMANDS[@]}"; do
    if [[ -f "${CC_COMMANDS}/${cmd}" ]]; then
        backup "${CC_COMMANDS}/${cmd}"
        run "rm '${CC_COMMANDS}/${cmd}'"
        log "  retired commands/${cmd} (now a skill)"
    fi
done

# Overlay Claude-specific commands (if any)
if [[ -d "${SCRIPT_DIR}/commands" ]] && [[ -n "$(ls -A "${SCRIPT_DIR}/commands" 2>/dev/null)" ]]; then
    run "mkdir -p '${CC_COMMANDS}'"
    run "rsync -av '${SCRIPT_DIR}/commands/' '${CC_COMMANDS}/'"
    log "✅ claude/commands/ → ${CC_COMMANDS}/ (overlay)"
else
    log "ℹ️  No Claude-specific commands to overlay"
fi

# ---------------------------------------------------------------------------
# Step 5: Deploy global settings.json (merged into any existing file so local
# customization — model overrides, env, extra permissions — survives redeploys;
# backs up the existing file first)
# ---------------------------------------------------------------------------
log "=== Deploying settings.json ==="

if [[ -f "${SCRIPT_DIR}/settings.json" ]]; then
    run "mkdir -p '${CLAUDE_HOME}'"
    if [[ -f "${CLAUDE_HOME}/settings.json" ]]; then
        log "Merging settings.json with existing config to preserve local customization..."
        if [[ "$DRY_RUN" == true ]]; then
            log "[dry-run] Merge '${SCRIPT_DIR}/settings.json' into '${CLAUDE_HOME}/settings.json' via merge-settings.py"
        else
            backup "${CLAUDE_HOME}/settings.json"
            python3 "${SHARED_DIR}/scripts/merge-settings.py" \
                "${CLAUDE_HOME}/settings.json" "${SCRIPT_DIR}/settings.json" "${CLAUDE_HOME}/settings.json.tmp"
            run "mv '${CLAUDE_HOME}/settings.json.tmp' '${CLAUDE_HOME}/settings.json'"
            log "✅ settings.json merged successfully"
        fi
    else
        run "cp '${SCRIPT_DIR}/settings.json' '${CLAUDE_HOME}/settings.json'"
        log "✅ claude/settings.json → ${CLAUDE_HOME}/settings.json"
    fi
else
    log "ℹ️  No claude/settings.json to deploy"
fi

# ---------------------------------------------------------------------------
# Step 6: Deploy subagents (no --delete: preserve any user-added agents).
# The flip side of skipping --delete: agents the harness once shipped but has
# since retired would linger on installed machines forever, so they are removed
# explicitly (backup first) — same pattern as MIGRATED_COMMANDS above.
# ---------------------------------------------------------------------------
log "=== Deploying subagents ==="

RETIRED_AGENTS=(testing.md)
for agent in "${RETIRED_AGENTS[@]}"; do
    if [[ -f "${CC_AGENTS}/${agent}" ]]; then
        backup "${CC_AGENTS}/${agent}"
        run "rm '${CC_AGENTS}/${agent}'"
        log "  retired agents/${agent} (no longer shipped by the harness)"
    fi
done

if [[ -d "${SCRIPT_DIR}/agents" ]] && [[ -n "$(ls -A "${SCRIPT_DIR}/agents" 2>/dev/null)" ]]; then
    run "mkdir -p '${CC_AGENTS}'"
    run "rsync -av '${SCRIPT_DIR}/agents/' '${CC_AGENTS}/'"
    log "✅ claude/agents/ → ${CC_AGENTS}/"
else
    log "ℹ️  No subagents to deploy"
fi

# ---------------------------------------------------------------------------
# Step 7: Deploy hook + statusline scripts
#   - statusline.sh is Claude-only (Antigravity has no statusline)
#   - format-hook.sh is shared (deployed to both vendors from shared/scripts/)
# ---------------------------------------------------------------------------
log "=== Deploying hook + statusline scripts ==="

run "mkdir -p '${CLAUDE_HOME}'"
if [[ -f "${SCRIPT_DIR}/scripts/statusline.sh" ]]; then
    backup "${CLAUDE_HOME}/statusline.sh"
    run "cp '${SCRIPT_DIR}/scripts/statusline.sh' '${CLAUDE_HOME}/statusline.sh'"
    run "chmod +x '${CLAUDE_HOME}/statusline.sh'"
    log "✅ claude/scripts/statusline.sh → ${CLAUDE_HOME}/statusline.sh"
fi
if [[ -f "${SHARED_DIR}/scripts/format-hook.sh" ]]; then
    backup "${CLAUDE_HOME}/format-hook.sh"
    run "cp '${SHARED_DIR}/scripts/format-hook.sh' '${CLAUDE_HOME}/format-hook.sh'"
    run "chmod +x '${CLAUDE_HOME}/format-hook.sh'"
    log "✅ shared/scripts/format-hook.sh → ${CLAUDE_HOME}/format-hook.sh"
fi
# Learning-loop capture hook (Claude-only for now: the UserPromptSubmit event is
# verified for Claude Code; no confirmed Antigravity equivalent yet)
if [[ -f "${SHARED_DIR}/scripts/feedback-capture.sh" ]]; then
    backup "${CLAUDE_HOME}/feedback-capture.sh"
    run "cp '${SHARED_DIR}/scripts/feedback-capture.sh' '${CLAUDE_HOME}/feedback-capture.sh'"
    run "chmod +x '${CLAUDE_HOME}/feedback-capture.sh'"
    log "✅ shared/scripts/feedback-capture.sh → ${CLAUDE_HOME}/feedback-capture.sh"
fi

# ---------------------------------------------------------------------------
# Step 8: Deploy shell gates to ~/.agents/bin/ — the vendor-neutral home that
# hooks (settings.json on Claude Code) and skill steps (Antigravity) reference.
# Scripts stay in shared/scripts/ as the source of truth.
# ---------------------------------------------------------------------------
log "=== Deploying shell gates to ~/.agents/bin ==="

AGENTS_BIN="${HOME}/.agents/bin"
run "mkdir -p '${AGENTS_BIN}'"
for gate in resume-context.sh trail-check.sh; do
    if [[ -f "${SHARED_DIR}/scripts/${gate}" ]]; then
        backup "${AGENTS_BIN}/${gate}"
        run "cp '${SHARED_DIR}/scripts/${gate}' '${AGENTS_BIN}/${gate}'"
        run "chmod +x '${AGENTS_BIN}/${gate}'"
        log "✅ shared/scripts/${gate} → ${AGENTS_BIN}/${gate}"
    else
        log "⚠️  shared/scripts/${gate} missing — hooks reference ${AGENTS_BIN}/${gate}; rerun setup once it exists"
    fi
done

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
echo ""
log "=== Done ==="
log ""
log "Deployed:"
log "  Global:    ${CLAUDE_HOME}/CLAUDE.md"
log "  Skills:    ${CC_SKILLS}/  (workflow skills provide the slash commands below)"
log "  Gates:     ${AGENTS_BIN}/  (resume-context.sh, trail-check.sh — wired via settings.json hooks)"
log ""
log "Available slash commands:"
log "  /audit          — 4-step code audit protocol"
log "  /distill        — Extract skills from repeated patterns"
log "  /research       — Research & spec protocol (full + light)"
log "  /retro          — Session retrospective (learning loop)"
log "  /init-project   — Scaffold a new project with AGENTS.md + .agents/"
log "  /handoff        — Summarize session for agent-to-agent handoff"
log "  /preflight      — Release gate (reviewer + security + verifier + trail freshness)"
log ""
log "To scaffold a new project, open Claude Code and run: /init-project"
