#!/usr/bin/env bash
# deploy/firewall/install.sh — install or converge the host half of project
# network isolation + the per-project LAN exception (ADR-065). Idempotent and
# narrow: the firewall script, its three units, the tmpfiles entry for the
# run dir, the marker directory, and — ONLY IF ABSENT — the operator's LAN
# definition. host-setup.sh §3b delegates here; the auto-updater's
# host_converge installs the script and units from the verified tag but never
# writes firewall.conf, so what "local network" means stays a decision the
# operator saw once.
#
# Usage (root):
#   sudo bash deploy/firewall/install.sh [--state-dir DIR] [--lan-dev DEV]
#        [--lan-cidr CIDR]... [--dry-run]
#   Advanced (root tests, ADR-052): --bin-dir --unit-dir --tmpfiles-dir
#        --etc-dir override the fixed system paths.
set -euo pipefail

STATE_DIR="${STYRMAN_STATE_DIR:-/var/lib/styrman}"
BIN_DIR=/usr/local/sbin
UNIT_DIR=/etc/systemd/system
TMPFILES_DIR=/etc/tmpfiles.d
ETC_DIR=/etc/styrman
RUN_DIR=/run/styrman-firewall
LAN_DEV=""
LAN_CIDRS=()
ALLOW_HOST=""
DRY=0

while [ $# -gt 0 ]; do
    case "$1" in
        --state-dir) STATE_DIR="$2"; shift 2 ;;
        --lan-dev) LAN_DEV="$2"; shift 2 ;;
        --lan-cidr) LAN_CIDRS+=("$2"); shift 2 ;;
        # ADR-066. Given explicitly it is also applied to an EXISTING conf —
        # the one thing this installer changes in place, because opting in to
        # host reachability is meant to be a deliberate human act and the conf
        # is otherwise never rewritten once present.
        --lan-allow-host)
            case "$2" in 0|1) ALLOW_HOST="$2" ;; *) die "--lan-allow-host takes 0 or 1" ;; esac
            shift 2 ;;
        --bin-dir) BIN_DIR="$2"; shift 2 ;;
        --unit-dir) UNIT_DIR="$2"; shift 2 ;;
        --tmpfiles-dir) TMPFILES_DIR="$2"; shift 2 ;;
        --etc-dir) ETC_DIR="$2"; shift 2 ;;
        --dry-run) DRY=1; shift ;;
        *) echo "unknown flag: $1" >&2; exit 2 ;;
    esac
done

log() { echo "==> $*"; }
die() { echo "FATAL: $*" >&2; exit 1; }
run() { if [ "${DRY}" = 1 ]; then echo "+ $*"; else "$@"; fi; }

[ "${DRY}" = 1 ] || [ "$(id -u)" = 0 ] || die "run as root (sudo), or pass --dry-run"
SRC="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
for f in styrman-firewall.sh styrman-firewall.service styrman-firewall.timer styrman-firewall.path; do
    [ -f "${SRC}/${f}" ] || die "missing ${SRC}/${f}"
done
bash -n "${SRC}/styrman-firewall.sh"
[ -d "${STATE_DIR}" ] || die "state dir ${STATE_DIR} does not exist (host-setup.sh creates it)"
OWNER_UID="$(stat -c %u "${STATE_DIR}")"
OWNER_GID="$(stat -c %g "${STATE_DIR}")"
[ "${OWNER_UID}" != "0" ] || die "state dir ${STATE_DIR} is root-owned; host-setup.sh chowns it to the operator, and the server runs as that uid"

# ─── 1. Script + units ──────────────────────────────────────────────────────
log "installing ${BIN_DIR}/styrman-firewall.sh (version $(sed -n 's/^STYRMAN_FW_VERSION=\([0-9]*\)$/\1/p' "${SRC}/styrman-firewall.sh"))"
run install -m 0755 -o root -g root "${SRC}/styrman-firewall.sh" "${BIN_DIR}/styrman-firewall.sh"
for f in styrman-firewall.service styrman-firewall.timer styrman-firewall.path; do
    run install -m 0644 -o root -g root "${SRC}/${f}" "${UNIT_DIR}/${f}"
done
# The unit pins /var/lib/styrman; a non-default state dir gets a drop-in that
# survives a reinstall of the unit file.
DROPIN_DIR="${UNIT_DIR}/styrman-firewall.service.d"
if [ "${STATE_DIR}" != "/var/lib/styrman" ]; then
    log "non-default state dir: drop-in ${DROPIN_DIR}/override.conf"
    if [ "${DRY}" = 1 ]; then
        echo "+ write ${DROPIN_DIR}/override.conf: Environment=STYRMAN_FW_STATE_DIR=${STATE_DIR}"
    else
        install -d -m 0755 -o root -g root "${DROPIN_DIR}"
        printf '[Service]\nEnvironment=STYRMAN_FW_STATE_DIR=%s\n' "${STATE_DIR}" > "${DROPIN_DIR}/override.conf.new.$$"
        chmod 0644 "${DROPIN_DIR}/override.conf.new.$$"
        mv -fT "${DROPIN_DIR}/override.conf.new.$$" "${DROPIN_DIR}/override.conf"
    fi
elif [ -e "${DROPIN_DIR}/override.conf" ]; then
    log "removing stale drop-in ${DROPIN_DIR}/override.conf (state dir is the default again)"
    run rm -f "${DROPIN_DIR}/override.conf"
fi

# ─── 2. Run dir at boot (tmpfiles) + now ────────────────────────────────────
# The parent is root-owned so the server's uid cannot swap `spool` for a
# symlink; the spool itself belongs to that uid so the server can write it.
TMPFILES_CONF="${TMPFILES_DIR}/styrman-firewall.conf"
log "tmpfiles: ${TMPFILES_CONF} (spool owner ${OWNER_UID}:${OWNER_GID})"
if [ "${DRY}" = 1 ]; then
    printf '+ write %s:\n  d %s 0755 root root -\n  d %s/spool 0700 %s %s -\n' "${TMPFILES_CONF}" "${RUN_DIR}" "${RUN_DIR}" "${OWNER_UID}" "${OWNER_GID}"
else
    printf 'd %s 0755 root root -\nd %s/spool 0700 %s %s -\n' "${RUN_DIR}" "${RUN_DIR}" "${OWNER_UID}" "${OWNER_GID}" > "${TMPFILES_CONF}.new.$$"
    chmod 0644 "${TMPFILES_CONF}.new.$$"
    mv -fT "${TMPFILES_CONF}.new.$$" "${TMPFILES_CONF}"
fi
run systemd-tmpfiles --create "${TMPFILES_CONF}"
# The spool is the one place the server's uid writes on the root tmpfs. Bound
# it structurally (security review): its own tiny tmpfs, 1 MiB and 1024
# inodes, mounted at boot before docker.service. Generated here (uid/gid are
# host-specific) and never touched by host_converge. NOTE: a running
# styrman-server container bound /run/styrman-firewall before this mount
# existed and keeps seeing the plain directory underneath — restart it once
# (the next release deploy does) so its trigger files land on the tmpfs.
MOUNT_UNIT="${UNIT_DIR}/$(systemd-escape -p --suffix=mount "${RUN_DIR}/spool")"
log "spool tmpfs: ${MOUNT_UNIT} (1M, 1024 inodes, ${OWNER_UID}:${OWNER_GID})"
MOUNT_BODY="$(printf '[Unit]\nDescription=styrman firewall spool (bounded tmpfs written by the server uid, consumed by root)\nDefaultDependencies=no\nConflicts=umount.target\nBefore=umount.target local-fs.target\nAfter=systemd-tmpfiles-setup-dev.service\n\n[Mount]\nWhat=tmpfs\nWhere=%s/spool\nType=tmpfs\nOptions=size=1M,nr_inodes=1024,mode=0700,uid=%s,gid=%s,nosuid,nodev,noexec\n\n[Install]\nWantedBy=local-fs.target\n' "${RUN_DIR}" "${OWNER_UID}" "${OWNER_GID}")"
if [ "${DRY}" = 1 ]; then
    printf '+ write %s:\n' "${MOUNT_UNIT}"; printf '%s\n' "${MOUNT_BODY}" | sed 's/^/    /'
else
    printf '%s\n' "${MOUNT_BODY}" > "${MOUNT_UNIT}.new.$$"
    chmod 0644 "${MOUNT_UNIT}.new.$$"
    mv -fT "${MOUNT_UNIT}.new.$$" "${MOUNT_UNIT}"
fi

# ─── 3. Marker directory — deliberately NOT created here ────────────────────
# The server creates <STATE_DIR>/netpolicy/lan itself at boot (cmd/styrman,
# 0750) as its own uid, inside its own state dir. Root used to create and
# chown it here, and the security review of 2026-09-08 showed that guard
# could only ever be narrowed, not closed: STATE_DIR is owned by that same
# uid, so it can swap a path component between root's check and root's
# chown/chmod (both dereference), and between one loop iteration and the
# next. The fix is to take root out of the path entirely — nothing is lost,
# because the server makes the directory with the right owner by
# construction, and the reconciler already treats an absent marker dir as
# "no allowances" and says so in its status (BUG-009).

# ─── 4. LAN definition — written ONLY IF ABSENT ─────────────────────────────
CONF="${ETC_DIR}/firewall.conf"
if [ -e "${CONF}" ] && [ -n "${ALLOW_HOST}" ]; then
    # Targeted in-place update of the ONE key, preserving everything else.
    [ -L "${CONF}" ] && die "${CONF} is a symlink"
    [ "$(stat -c %u "${CONF}")" = "0" ] || die "${CONF} is not root-owned"
    log "setting LAN_ALLOW_HOST=${ALLOW_HOST} in ${CONF}"
    if grep -q '^LAN_ALLOW_HOST=' "${CONF}"; then
        new_conf="$(sed "s/^LAN_ALLOW_HOST=.*/LAN_ALLOW_HOST=${ALLOW_HOST}/" "${CONF}")"
    else
        new_conf="$(cat "${CONF}"; printf 'LAN_ALLOW_HOST=%s\n' "${ALLOW_HOST}")"
    fi
    printf '%s\n' "${new_conf}" | sed 's/^/    /'
    if [ "${DRY}" = 0 ]; then
        printf '%s\n' "${new_conf}" > "${CONF}.new.$$"
        chmod 0644 "${CONF}.new.$$"; chown root:root "${CONF}.new.$$"
        mv -fT "${CONF}.new.$$" "${CONF}"
    fi
elif [ -e "${CONF}" ]; then
    log "keeping existing ${CONF}:"
    sed 's/^/    /' "${CONF}"
    grep -q '^LAN_ALLOW_HOST=1' "${CONF}" \
        || log "note: the host itself is NOT reachable from projects; opt in with --lan-allow-host 1 (ADR-066)"
else
    if [ -z "${LAN_DEV}" ]; then
        LAN_DEV="$(ip -4 route show default 2>/dev/null | awk '{for (i = 1; i <= NF; i++) if ($i == "dev") { print $(i + 1); exit }}')"
        [ -n "${LAN_DEV}" ] || die "no IPv4 default route: pass --lan-dev DEV --lan-cidr CIDR explicitly"
        log "discovered uplink ${LAN_DEV} (device of the default route)"
    fi
    [ -e "/sys/class/net/${LAN_DEV}" ] || die "LAN_DEV ${LAN_DEV} does not exist"
    if [ ${#LAN_CIDRS[@]} -eq 0 ]; then
        while read -r cidr; do
            case "${cidr}" in */*) LAN_CIDRS+=("${cidr}") ;; esac
        done < <(ip -4 -o route show dev "${LAN_DEV}" scope link 2>/dev/null | awk '{print $1}')
        [ ${#LAN_CIDRS[@]} -gt 0 ] || die "no connected IPv4 prefix on ${LAN_DEV}: pass --lan-cidr CIDR explicitly"
        log "discovered local prefix(es) on ${LAN_DEV}: ${LAN_CIDRS[*]}"
    fi
    log "writing ${CONF}:"
    body="$(printf '# written by deploy/firewall/install.sh %s - edit, then: sudo systemctl start styrman-firewall.service\n# LAN_DEV: the uplink device; LAN_CIDRS: space-separated IPv4 prefixes (/16../30) a LAN-enabled\n# project may reach. Validated on every run; an invalid value closes every allowance (fail closed).\n# LAN_ALLOW_HOST (ADR-066): 1 lets a LAN-enabled project reach the HOST itself, sshd included -\n#   a deployment that sets it MUST NOT accept password authentication on the uplink. Default 0.\n# Reserved (parsed as 0): LAN_ALLOW_PUBLISHED_PORTS - needs its own ADR.\nLAN_DEV=%s\nLAN_CIDRS=%s\nLAN_ALLOW_HOST=%s\n' "$(date +%F)" "${LAN_DEV}" "${LAN_CIDRS[*]}" "${ALLOW_HOST:-0}")"
    printf '%s\n' "${body}" | sed 's/^/    /'
    if [ "${DRY}" = 0 ]; then
        # Create only when absent; never re-mode an existing directory (the
        # updater once chmod 0700'd /tmp this way — styrman-update:139-152).
        [ -d "${ETC_DIR}" ] || install -d -m 0700 -o root -g root "${ETC_DIR}"
        if [ -L "${ETC_DIR}" ] || [ "$(stat -c %u "${ETC_DIR}")" != "0" ] || [ -n "$(find "${ETC_DIR}" -maxdepth 0 -perm /022)" ]; then
            die "${ETC_DIR} must be a root-owned directory not writable by group/others"
        fi
        printf '%s\n' "${body}" > "${CONF}.new.$$"
        chmod 0644 "${CONF}.new.$$"
        chown root:root "${CONF}.new.$$"
        mv -fT "${CONF}.new.$$" "${CONF}"
    fi
fi

# ─── 5. Activate ────────────────────────────────────────────────────────────
run systemctl daemon-reload
run systemctl enable --now "$(basename "${MOUNT_UNIT}")"
run systemctl start styrman-firewall.service
run systemctl enable --now styrman-firewall.timer styrman-firewall.path
if [ "${DRY}" = 0 ]; then
    log "STYRMAN-FWD (head):"
    iptables -S STYRMAN-FWD | head -12 | sed 's/^/    /'
    log "status (${RUN_DIR}/status):"
    sed 's/^/    /' "${RUN_DIR}/status" 2>/dev/null || echo "    (not written yet - check: journalctl -u styrman-firewall.service)"
    log "done. Verify: systemctl status styrman-firewall.timer styrman-firewall.path"
else
    log "dry run complete - nothing written"
fi
