#!/usr/bin/env bash
# deploy/firewall/install.sh — install or converge the host half of project
# network isolation (R12, ADR-029/036/067). Idempotent and narrow: the
# firewall script and its two units, nothing else.
#
# It also REMOVES what ADR-065/066 installed and ADR-067 deleted — the .path
# unit, the spool tmpfs mount, its tmpfiles entry, the run dir and the service
# drop-in — because an upgrading host still carries them and a leftover .path
# unit pointing at a deleted spool is a unit that fails on every boot. The
# operator's /etc/styrman/firewall.conf is deliberately LEFT ALONE: this
# script does not edit an operator-owned file, and its now-unread keys are
# inert.
#
# Usage (root):
#   sudo bash deploy/firewall/install.sh [--dry-run]
#   Advanced (root tests, ADR-052): --bin-dir --unit-dir --tmpfiles-dir
#        override the fixed system paths.
set -euo pipefail
shopt -s inherit_errexit

BIN_DIR=/usr/local/sbin
UNIT_DIR=/etc/systemd/system
TMPFILES_DIR=/etc/tmpfiles.d
RUN_DIR=/run/styrman-firewall
DRY=0

log() { echo "styrman-firewall-install: $*"; }
die() { echo "styrman-firewall-install: ERROR $*" >&2; exit 1; }
run() { if [ "${DRY}" = 1 ]; then echo "+ $*"; else "$@"; fi; }

while [ $# -gt 0 ]; do
    case "$1" in
        --bin-dir) BIN_DIR="$2"; shift 2 ;;
        --unit-dir) UNIT_DIR="$2"; shift 2 ;;
        --tmpfiles-dir) TMPFILES_DIR="$2"; shift 2 ;;
        --run-dir) RUN_DIR="$2"; shift 2 ;;
        --dry-run) DRY=1; shift ;;
        # Accepted and ignored so an operator's muscle memory, a stale runbook
        # or an old host_converge invocation does not fail the install. They
        # configured the LAN allowance machinery ADR-067 deleted.
        --state-dir|--lan-dev|--lan-cidr|--lan-allow-host|--lan-allow-published-ports|--ingress-bridge)
            log "note: $1 is obsolete since ADR-067 (the mode is the bridge name, not a firewall rule) - ignoring"
            shift 2 ;;
        --i-accept-sshd-passwords)
            log "note: $1 is obsolete since ADR-067 - ignoring"
            shift ;;
        *) echo "unknown flag: $1" >&2; exit 2 ;;
    esac
done

[ "${DRY}" = 1 ] || [ "$(id -u)" = 0 ] || die "run as root (sudo), or pass --dry-run"
SRC="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
for f in styrman-firewall.sh styrman-firewall.service styrman-firewall.timer; do
    [ -f "${SRC}/${f}" ] || die "missing ${SRC}/${f}"
done
bash -n "${SRC}/styrman-firewall.sh"

# ─── 1. Script + units ──────────────────────────────────────────────────────
log "installing ${BIN_DIR}/styrman-firewall.sh"
run install -m 0755 -o root -g root "${SRC}/styrman-firewall.sh" "${BIN_DIR}/styrman-firewall.sh"
for f in styrman-firewall.service styrman-firewall.timer; do
    run install -m 0644 -o root -g root "${SRC}/${f}" "${UNIT_DIR}/${f}"
done

# ─── 2. Converge away the ADR-065/066 host artifacts ────────────────────────
# Order matters: stop and disable before deleting the unit files, or systemd
# keeps a running unit whose fragment is gone.
PATH_UNIT="${UNIT_DIR}/styrman-firewall.path"
MOUNT_UNIT_NAME=""
if command -v systemd-escape >/dev/null 2>&1; then
    MOUNT_UNIT_NAME="$(systemd-escape -p --suffix=mount "${RUN_DIR}/spool")"
fi
if [ -e "${PATH_UNIT}" ]; then
    log "removing the obsolete styrman-firewall.path (ADR-067: no spool to watch)"
    run systemctl disable --now styrman-firewall.path || true
    run rm -f "${PATH_UNIT}"
fi
if [ -n "${MOUNT_UNIT_NAME}" ] && [ -e "${UNIT_DIR}/${MOUNT_UNIT_NAME}" ]; then
    log "removing the obsolete spool tmpfs mount ${MOUNT_UNIT_NAME}"
    run systemctl disable --now "${MOUNT_UNIT_NAME}" || true
    run rm -f "${UNIT_DIR}/${MOUNT_UNIT_NAME}"
fi
if [ -e "${TMPFILES_DIR}/styrman-firewall.conf" ]; then
    log "removing the obsolete tmpfiles entry for ${RUN_DIR}"
    run rm -f "${TMPFILES_DIR}/styrman-firewall.conf"
fi
# The service drop-in only ever carried STYRMAN_FW_STATE_DIR, which the script
# no longer reads.
DROPIN="${UNIT_DIR}/styrman-firewall.service.d/override.conf"
if [ -e "${DROPIN}" ]; then
    log "removing the obsolete service drop-in ${DROPIN}"
    run rm -f "${DROPIN}"
    run rmdir --ignore-fail-on-non-empty "$(dirname "${DROPIN}")"
fi
# The run dir is a plain root-owned directory once its mount is gone. Remove
# it by name, non-recursively: anything unexpected inside is left for a human.
if [ -d "${RUN_DIR}" ]; then
    log "removing ${RUN_DIR} (no longer used)"
    run rm -f "${RUN_DIR}/status" "${RUN_DIR}/lock"
    run rmdir "${RUN_DIR}/spool" 2>/dev/null || true
    run rmdir "${RUN_DIR}" 2>/dev/null || log "note: ${RUN_DIR} not empty - left in place"
fi
if [ -e /etc/styrman/firewall.conf ]; then
    log "note: /etc/styrman/firewall.conf is no longer read by anything (ADR-067). Left untouched."
fi

# ─── 3. Activate ────────────────────────────────────────────────────────────
run systemctl daemon-reload
# The oneshot exits 1 on a DEGRADED host (a missing xt_physdev, a tagged
# DROP it refused to delete — every WARN reaches its exit path). That must
# not abort this installer before the units are enabled (BUG-035's shape;
# the updater's host_converge tolerates it the same way): log and go on.
run systemctl start styrman-firewall.service \
    || log "WARN: styrman-firewall.service reported a degraded state (journalctl -u styrman-firewall) — continuing so the units are enabled"
# Enabled as a boot-time unit too: the base rules land once right after
# docker.service instead of at the timer's first tick (the service's
# [Install] section; ADR-065 follow-up (f), closed 2026-09-10).
run systemctl enable styrman-firewall.service
run systemctl enable --now styrman-firewall.timer
if [ "${DRY}" = 0 ]; then
    log "STYRMAN-FWD:"
    iptables -S STYRMAN-FWD 2>/dev/null | sed 's/^/    /' || true
    log "STYRMAN-IN:"
    iptables -S STYRMAN-IN 2>/dev/null | sed 's/^/    /' || true
    log "done. Verify: systemctl status styrman-firewall.timer"
else
    log "dry run complete - nothing written"
fi
