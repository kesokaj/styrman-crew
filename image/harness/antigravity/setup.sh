#!/usr/bin/env bash
set -euo pipefail

# =============================================================================
# antigravity/setup.sh — Deploy the harness for the Antigravity CLI (agy).
#
# Source: ../shared/ + ../claude/agents/  (Claude Code is the source of truth;
#         everything Antigravity-specific is derived at deploy time)
# Target: ~/.gemini/
#
# Paths verified against agy-cli 1.1.11 (the CLI rebuilt ~/.gemini in 2026-08):
#   Global rules : ~/.gemini/GEMINI.md                    (loaded as always-on rules)
#   Skills       : ~/.gemini/config/skills/               (global discovery root)
#   Workflows    : ~/.gemini/config/global_workflows/     (slash commands)
#   Plugins      : ~/.gemini/config/plugins/              (agents + hooks bundle)
#   CLI settings : ~/.gemini/antigravity-cli/settings.json — NOT touched by this
#                  script. It is agy's own state (auth, trust, UI prefs) with a
#                  schema unrelated to the old ~/.gemini/settings.json, which
#                  agy no longer reads. Hooks now ship via the harness plugin's
#                  hooks.json instead of a settings overlay.
#
# Deploys:
#   shared/AGENTS.md            → ~/.gemini/GEMINI.md
#   shared/skills/              → ~/.gemini/config/skills/   (skills double as slash commands)
#   claude/agents/*.md          → antigravity/plugins/harness/agents/ (derived)
#   antigravity/plugins/        → ~/.gemini/config/plugins/
#   shared/scripts/format-hook.sh → ~/.gemini/format-hook.sh
#   shared/scripts/{resume-context.sh,trail-check.sh} → ~/.agents/bin/  (vendor-neutral
#                  shell gates — Antigravity runs them as skill steps, Claude as hooks)
#
# Usage:
#   ./antigravity/setup.sh              # Deploy everything
#   ./antigravity/setup.sh --dry-run    # Show what would happen without changes
# =============================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
HARNESS_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
SHARED_DIR="${HARNESS_DIR}/shared"
CLAUDE_AGENTS_DIR="${HARNESS_DIR}/claude/agents"

GEMINI_HOME="${HOME}/.gemini"
AG_CONFIG="${GEMINI_HOME}/config"
AG_SKILLS="${AG_CONFIG}/skills"
AG_WORKFLOWS="${AG_CONFIG}/global_workflows"
AG_PLUGINS="${AG_CONFIG}/plugins"

DRY_RUN=false

while [[ $# -gt 0 ]]; do
    case "$1" in
        --dry-run) DRY_RUN=true; shift ;;
        *)
            echo "Unknown option: $1"
            echo "Usage: ./antigravity/setup.sh [--dry-run]"
            exit 1
            ;;
    esac
done

log() { echo "[antigravity] $*"; }

run() {
    if [[ "$DRY_RUN" == true ]]; then
        log "[dry-run] $*"
    else
        log "$*"
        bash -c "$*"
    fi
}

# Back up an existing file/dir before it is overwritten (REQUIREMENTS.md §Security).
backup() {
    local target="$1"
    if [[ -e "$target" ]]; then
        run "cp -a '$target' '${target}.bak.$(date +%Y%m%d-%H%M%S)'"
    fi
}

# ---------------------------------------------------------------------------
# Step 1: Refresh the in-repo GEMINI.md copy from the shared source
# ---------------------------------------------------------------------------
log "=== Refreshing in-repo GEMINI.md from shared/AGENTS.md ==="

if [[ ! -f "${SHARED_DIR}/AGENTS.md" ]]; then
    echo "Error: ${SHARED_DIR}/AGENTS.md not found"
    exit 1
fi

run "cp '${SHARED_DIR}/AGENTS.md' '${SCRIPT_DIR}/GEMINI.md'"
log "✅ shared/AGENTS.md → antigravity/GEMINI.md"

# ---------------------------------------------------------------------------
# Step 2: Deploy global GEMINI.md
# ---------------------------------------------------------------------------
log "=== Deploying global GEMINI.md ==="

run "mkdir -p '${GEMINI_HOME}'"
backup "${GEMINI_HOME}/GEMINI.md"
run "cp '${SCRIPT_DIR}/GEMINI.md' '${GEMINI_HOME}/GEMINI.md'"
log "✅ antigravity/GEMINI.md → ${GEMINI_HOME}/GEMINI.md"

# ---------------------------------------------------------------------------
# Step 3: Deploy skills
# ---------------------------------------------------------------------------
log "=== Deploying skills ==="

if [[ ! -d "${SHARED_DIR}/skills" ]]; then
    echo "Error: ${SHARED_DIR}/skills/ not found"
    exit 1
fi

run "mkdir -p '${AG_SKILLS}'"
run "rsync -av --delete '${SHARED_DIR}/skills/' '${AG_SKILLS}/'"
log "✅ shared/skills/ → ${AG_SKILLS}/"

# ---------------------------------------------------------------------------
# Step 4: Retire the legacy global_workflows deploy target.
# Workflows migrated into shared/skills/ (skills act as slash commands in both
# vendors), so ~/.gemini/config/global_workflows/ — previously fully managed by
# this script via rsync --delete — is removed after a backup.
# ---------------------------------------------------------------------------
if [[ -d "${AG_WORKFLOWS}" ]]; then
    log "=== Retiring legacy global_workflows (migrated into skills) ==="
    backup "${AG_WORKFLOWS}"
    run "rm -rf '${AG_WORKFLOWS}'"
    log "✅ ${AG_WORKFLOWS}/ backed up and removed"
fi

# ---------------------------------------------------------------------------
# Step 5: Derive plugin agents from claude/agents (Claude is the source).
# derive-agy-agent.py keeps name/description, adds subagent/mainAgent flags,
# maps Claude model aliases to agy tiers, and rewrites skill paths.
# ---------------------------------------------------------------------------
log "=== Deriving plugin agents from claude/agents ==="

if [[ ! -d "${CLAUDE_AGENTS_DIR}" ]]; then
    echo "Error: ${CLAUDE_AGENTS_DIR}/ not found (Claude agents are the source)"
    exit 1
fi

AG_PLUGIN_AGENTS="${SCRIPT_DIR}/plugins/harness/agents"
if [[ "$DRY_RUN" == true ]]; then
    log "[dry-run] Derive ${AG_PLUGIN_AGENTS}/*.md from ${CLAUDE_AGENTS_DIR}/*.md via derive-agy-agent.py"
else
    mkdir -p "${AG_PLUGIN_AGENTS}"
    rm -f "${AG_PLUGIN_AGENTS}"/*.md
    for src in "${CLAUDE_AGENTS_DIR}"/*.md; do
        name="$(basename "$src")"
        python3 "${SHARED_DIR}/scripts/derive-agy-agent.py" "$src" "${AG_PLUGIN_AGENTS}/${name}"
        log "  derived agents/${name}"
    done
    log "✅ claude/agents/ → antigravity/plugins/harness/agents/ (derived copies)"
fi

# ---------------------------------------------------------------------------
# Step 6: Deploy plugins (agents + format hook registration via hooks.json)
# Ownership rule: ${AG_PLUGINS}/harness/ is fully harness-owned, so it syncs
# with --delete (a derived agent deleted from the repo must not resurrect on
# the next deploy). Everything else under ${AG_PLUGINS}/ may be user-installed
# plugins, so any non-harness repo content syncs WITHOUT --delete. Currently
# harness/ is the only subdir this repo ships.
# ---------------------------------------------------------------------------
log "=== Deploying plugins ==="

if [[ -d "${SCRIPT_DIR}/plugins/harness" ]]; then
    run "mkdir -p '${AG_PLUGINS}/harness'"
    run "rsync -av --delete '${SCRIPT_DIR}/plugins/harness/' '${AG_PLUGINS}/harness/'"
    log "✅ antigravity/plugins/harness/ → ${AG_PLUGINS}/harness/ (harness-owned, --delete)"
else
    log "ℹ️  No harness plugin to deploy"
fi

OTHER_PLUGINS="$(find "${SCRIPT_DIR}/plugins" -mindepth 1 -maxdepth 1 ! -name harness 2>/dev/null || true)"
if [[ -n "${OTHER_PLUGINS}" ]]; then
    run "mkdir -p '${AG_PLUGINS}'"
    run "rsync -av --exclude 'harness' '${SCRIPT_DIR}/plugins/' '${AG_PLUGINS}/'"
    log "✅ antigravity/plugins/ (non-harness) → ${AG_PLUGINS}/ (no --delete)"
fi

# ---------------------------------------------------------------------------
# Step 7: Deploy shared format-hook script (referenced by the plugin hooks.json)
# ---------------------------------------------------------------------------
log "=== Deploying format-hook script ==="

if [[ -f "${SHARED_DIR}/scripts/format-hook.sh" ]]; then
    run "mkdir -p '${GEMINI_HOME}'"
    backup "${GEMINI_HOME}/format-hook.sh"
    run "cp '${SHARED_DIR}/scripts/format-hook.sh' '${GEMINI_HOME}/format-hook.sh'"
    run "chmod +x '${GEMINI_HOME}/format-hook.sh'"
    log "✅ shared/scripts/format-hook.sh → ${GEMINI_HOME}/format-hook.sh"
fi

# ---------------------------------------------------------------------------
# Step 8: Deploy shell gates to ~/.agents/bin/ — the vendor-neutral home that
# skill steps (Antigravity) and hooks (Claude Code settings.json) reference.
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
        log "⚠️  shared/scripts/${gate} missing — skill steps reference ${AGENTS_BIN}/${gate}; rerun setup once it exists"
    fi
done

# ---------------------------------------------------------------------------
# Step 9: Remove artifacts of older harness layouts that agy no longer reads
# (never deletes user data — only files this harness itself deployed)
# ---------------------------------------------------------------------------
if [[ -f "${GEMINI_HOME}/settings.json" ]] && grep -q '"_comment": "Minimal settings overlay for the agent harness' "${GEMINI_HOME}/settings.json" 2>/dev/null; then
    log "=== Retiring stale ~/.gemini/settings.json (agy-cli no longer reads it) ==="
    backup "${GEMINI_HOME}/settings.json"
    run "rm '${GEMINI_HOME}/settings.json'"
    log "✅ stale settings.json backed up and removed"
fi

# ---------------------------------------------------------------------------
# Summary + verification
# ---------------------------------------------------------------------------
echo ""
log "=== Done ==="
log ""
log "Deployed:"
log "  Global rules: ${GEMINI_HOME}/GEMINI.md"
log "  Skills:       ${AG_SKILLS}/  (includes workflow skills: /audit /distill /handoff /research /retro /init-project /preflight)"
log "  Plugins:      ${AG_PLUGINS}/harness/ (agents + hooks.json)"
log "  Format hook:  ${GEMINI_HOME}/format-hook.sh"
log "  Gates:        ${AGENTS_BIN}/  (resume-context.sh, trail-check.sh — run as skill steps)"

if [[ "$DRY_RUN" != true ]] && command -v agy >/dev/null 2>&1; then
    log ""
    log "Verification:"
    # Derived agents are subagent-only (mainAgent: false), so `agy agents` —
    # which lists primary-agent candidates — won't show them. The plugin
    # validator confirms they parse and register; invoke_subagent uses them.
    AGENT_LINE="$(agy plugin validate "${AG_PLUGINS}/harness" 2>/dev/null | grep -E 'agents' | head -1 | sed 's/^[[:space:]]*//')"
    EXPECTED_AGENTS="$(find "${CLAUDE_AGENTS_DIR}" -maxdepth 1 -name '*.md' 2>/dev/null | wc -l | tr -d ' ')"
    log "  plugin agents: ${AGENT_LINE:-validation failed} (expected ${EXPECTED_AGENTS} processed)"
    # `agy -p "/skills"` is quota-free as of agy-cli 1.1.11 (no agent turn).
    # Records are "name<TAB>description"; wrapped description lines don't match.
    # Two checks from one invocation: (a) total record count — pattern allows
    # underscores in names ([a-z0-9_-], literal tab via $'\t' for BSD grep);
    # (b) each deployed skill dir name looked up in the output, which is immune
    # to wrapped-line miscounts. Both reported.
    TAB=$'\t'
    SKILLS_OUTPUT="$(agy -p "/skills" 2>/dev/null || true)"
    SKILL_COUNT="$(printf '%s\n' "${SKILLS_OUTPUT}" | grep -cE "^[a-z0-9][a-z0-9_-]*${TAB}" || true)"
    EXPECTED_SKILLS=0
    FOUND_SKILLS=0
    MISSING_SKILLS=""
    for skill_dir in "${SHARED_DIR}"/skills/*/; do
        [[ -d "${skill_dir}" ]] || continue
        skill_name="$(basename "${skill_dir}")"
        EXPECTED_SKILLS=$((EXPECTED_SKILLS + 1))
        if printf '%s\n' "${SKILLS_OUTPUT}" | grep -q "^${skill_name}[[:space:]]"; then
            FOUND_SKILLS=$((FOUND_SKILLS + 1))
        else
            MISSING_SKILLS="${MISSING_SKILLS} ${skill_name}"
        fi
    done
    log "  agy skills: ${SKILL_COUNT} records listed; ${FOUND_SKILLS}/${EXPECTED_SKILLS} deployed skills present by name (builtins add a few records)"
    if [[ -n "${MISSING_SKILLS}" ]]; then
        log "  ⚠️  not found in /skills output:${MISSING_SKILLS} — inspect with: agy -p \"/skills\""
    fi
fi
