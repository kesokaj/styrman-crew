#!/usr/bin/env bash
set -euo pipefail

# =============================================================================
# setup.sh — Deploy the harness for one or more AI coding agents.
#
# Source of truth: shared/ (AGENTS.md framework — vendor-neutral)
#
# Targets:
#   antigravity → ~/.gemini/
#   claude      → ~/.claude/
#
# Usage:
#   ./setup.sh                       # Deploy for both Antigravity and Claude Code
#   ./setup.sh --target antigravity  # Deploy only for Antigravity
#   ./setup.sh --target claude       # Deploy only for Claude Code
#   ./setup.sh --dry-run             # Show what would happen without changes
# =============================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

DRY_RUN=false
TARGETS=()

while [[ $# -gt 0 ]]; do
    case "$1" in
        --dry-run) DRY_RUN=true; shift ;;
        --target)
            shift
            case "$1" in
                antigravity|claude) TARGETS+=("$1") ;;
                *) echo "Unknown target: $1 (expected: antigravity | claude)"; exit 1 ;;
            esac
            shift
            ;;
        -h|--help)
            sed -n '4,17p' "${BASH_SOURCE[0]}"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            echo "Usage: ./setup.sh [--target antigravity|claude] [--dry-run]"
            exit 1
            ;;
    esac
done

# Default: deploy to both
if [[ ${#TARGETS[@]} -eq 0 ]]; then
    TARGETS=(antigravity claude)
fi

log() { echo "[harness] $*"; }

# ---------------------------------------------------------------------------
# Dispatch to per-vendor setup scripts
# ---------------------------------------------------------------------------
for target in "${TARGETS[@]}"; do
    log ""
    log "############################################"
    log "# Deploying target: ${target}"
    log "############################################"
    SCRIPT="${SCRIPT_DIR}/${target}/setup.sh"
    if [[ ! -x "${SCRIPT}" ]]; then
        echo "Error: ${SCRIPT} not found or not executable"
        exit 1
    fi
    if [[ "$DRY_RUN" == true ]]; then
        "${SCRIPT}" --dry-run
    else
        "${SCRIPT}"
    fi
done

log ""
log "=== All targets deployed ==="
