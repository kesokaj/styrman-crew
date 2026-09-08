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
ALLOW_PUB=""
INGRESS_BRIDGE=""
ACCEPT_SSH_PASSWORDS=0
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
        --lan-allow-published-ports)
            case "$2" in 0|1) ALLOW_PUB="$2" ;; *) die "--lan-allow-published-ports takes 0 or 1" ;; esac
            shift 2 ;;
        --ingress-bridge) INGRESS_BRIDGE="$2"; shift 2 ;;
        # Deliberate override for a host whose sshd is not on the uplink, or
        # whose operator accepts the risk knowingly. Never a default.
        --i-accept-sshd-passwords) ACCEPT_SSH_PASSWORDS=1; shift ;;
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

# R12 states it as a REQUIREMENT: a deployment that lets projects reach the
# host must not accept password authentication on the uplink. With
# LAN_ALLOW_HOST=1 an agent inside a project can open connections to this
# machine's sshd, and a sudo-group login on a NOPASSWD host is root. The
# deploy guide said so and nothing enforced it (security review 2026-09-08) —
# a precondition no code checks is a wish. This checks it.
require_sshd_hardened() {
    local pw
    [ "${ACCEPT_SSH_PASSWORDS}" = 1 ] && { log "WARNING: --i-accept-sshd-passwords given; skipping the R12 sshd precondition"; return 0; }
    command -v sshd >/dev/null 2>&1 || { log "no sshd binary on this host - precondition not applicable"; return 0; }
    pw="$(sshd -T 2>/dev/null | sed -n 's/^passwordauthentication //p' | head -1)"
    case "${pw}" in
        no) log "sshd: passwordauthentication no - R12 precondition met" ;;
        "") die "cannot read sshd's effective config to check the LAN_ALLOW_HOST precondition (R12). Fix that, or pass --i-accept-sshd-passwords if sshd is not reachable from the LAN." ;;
        *)  die "REFUSING --lan-allow-host 1: sshd accepts password authentication (passwordauthentication ${pw}).
    R12 forbids that on a host projects can reach: an agent in a LAN-enabled
    project could guess its way to a shell, and a sudo-group login is root.
    Harden first - the five-step sequence, with a second session held open, is
    in .agents/docs/deploy/remote.md - then re-run this. To override
    deliberately (sshd not exposed to the LAN), pass --i-accept-sshd-passwords." ;;
    esac
}

if [ "${ALLOW_HOST}" = 1 ]; then require_sshd_hardened; fi

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

if [ "${ALLOW_PUB}" = 1 ]; then derive_ingress_bridge; fi

# ─── 4. LAN definition — written ONLY IF ABSENT ─────────────────────────────
# Docker names a bridge `br-` + the first 12 hex of the network id, so the
# ingress bridge is DERIVED, never typed. Naming the wrong one would leave the
# real ingress inside the LAN-reachable set, which is the opposite of what the
# key is for — so a failure to derive is fatal, exactly as it is for LAN_DEV.
derive_ingress_bridge() {
    local id
    [ -n "${INGRESS_BRIDGE}" ] && return 0
    command -v docker >/dev/null 2>&1 \
        || die "--lan-allow-published-ports 1 needs the ingress bridge: no docker CLI, pass --ingress-bridge br-<12 hex>"
    id="$(docker network inspect styrman-ingress --format '{{.Id}}' 2>/dev/null || true)"
    case "${id}" in
        [0-9a-f]*) ;;
        *) die "cannot resolve the styrman-ingress network: pass --ingress-bridge br-<12 hex>" ;;
    esac
    INGRESS_BRIDGE="br-$(printf '%s' "${id}" | cut -c1-12)"
    [ -e "/sys/class/net/${INGRESS_BRIDGE}" ] \
        || die "derived ingress bridge ${INGRESS_BRIDGE} does not exist - is the ingress running?"
    log "derived ingress bridge ${INGRESS_BRIDGE} from the styrman-ingress network"
}


CONF="${ETC_DIR}/firewall.conf"
UPDATES=()
[ -n "${ALLOW_HOST}" ] && UPDATES+=("LAN_ALLOW_HOST=${ALLOW_HOST}")
[ -n "${ALLOW_PUB}" ] && UPDATES+=("LAN_ALLOW_PUBLISHED_PORTS=${ALLOW_PUB}")
[ "${ALLOW_PUB}" = 1 ] && UPDATES+=("INGRESS_BRIDGE=${INGRESS_BRIDGE}")

if [ -e "${CONF}" ] && [ ${#UPDATES[@]} -gt 0 ]; then
    # Targeted in-place update of ONLY the named keys, preserving everything
    # else. This is the one thing the installer changes in an existing conf:
    # the file is otherwise never rewritten once present, and opting in to
    # either allowance is meant to be a deliberate act rather than a hand-edit.
    [ -L "${CONF}" ] && die "${CONF} is a symlink"
    [ "$(stat -c %u "${CONF}")" = "0" ] || die "${CONF} is not root-owned"
    new_conf="$(cat "${CONF}")"
    for kv in "${UPDATES[@]}"; do
        k="${kv%%=*}"
        log "setting ${kv} in ${CONF}"
        if printf '%s\n' "${new_conf}" | grep -q "^${k}="; then
            new_conf="$(printf '%s\n' "${new_conf}" | sed "s|^${k}=.*|${kv}|")"
        else
            new_conf="$(printf '%s\n%s' "${new_conf}" "${kv}")"
        fi
    done
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
    # The optional key is its own line, built BEFORE the printf. It used to be
    # inlined as ${VAR:+...$'\n'}, and $'...' is not ANSI-C quoting inside
    # double quotes — a fresh install wrote a literal backslash-n into the
    # conf, whose grammar then rejected the line and closed EVERY allowance on
    # that host (release review, 2026-09-08).
    INGRESS_LINE=""
    [ -n "${INGRESS_BRIDGE}" ] && INGRESS_LINE="INGRESS_BRIDGE=${INGRESS_BRIDGE}"
    body="$(printf '# written by deploy/firewall/install.sh %s - edit, then: sudo systemctl start styrman-firewall.service\n# LAN_DEV: the uplink device; LAN_CIDRS: space-separated IPv4 prefixes (/16../30) a LAN-enabled\n# project may reach. Validated on every run; an invalid value closes every allowance (fail closed).\n# LAN_ALLOW_HOST (ADR-066): 1 lets a LAN-enabled project reach the HOST itself, sshd included -\n#   a deployment that sets it MUST NOT accept password authentication on the uplink. Default 0.\n# LAN_ALLOW_PUBLISHED_PORTS (ADR-066): 1 lets a LAN-enabled project reach host-published Docker\n#   ports. Needs INGRESS_BRIDGE, which excludes the ingress from the reachable set. Default 0.\nLAN_DEV=%s\nLAN_CIDRS=%s\nLAN_ALLOW_HOST=%s\nLAN_ALLOW_PUBLISHED_PORTS=%s\n%s\n' "$(date +%F)" "${LAN_DEV}" "${LAN_CIDRS[*]}" "${ALLOW_HOST:-0}" "${ALLOW_PUB:-0}" "${INGRESS_LINE}")"
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
