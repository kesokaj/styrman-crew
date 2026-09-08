#!/usr/bin/env bash
# styrman-firewall.sh — project microVM network isolation (R12) plus the
# per-project local-network exception (ADR-065). Installed to
# /usr/local/sbin by deploy/firewall/install.sh; run by styrman-firewall.timer
# every 60 s and by styrman-firewall.path whenever the spool is non-empty.
#
# Two sections, in this order:
#
#   BASE — keep every project bridge (sdb-*) isolated from the host, the LAN,
#   other projects, cloud metadata and the tailnet while leaving the internet
#   and the same-bridge broker + port-preview proxy reachable (ADR-029/036).
#   Self-heal-safe: every rule is INSERT-IF-MISSING, never delete-then-insert,
#   so the 60 s timer re-runs open no unprotected window. This is the logic
#   that lived as a heredoc in deploy/host-setup.sh until ADR-065; the one
#   change is `-w 5` on every iptables call (security review). It runs FIRST
#   and touches nothing a lower-privileged uid controls, so nothing that uid
#   does can delay R12 enforcement.
#
#   LAN — reconcile the per-project allowances: one head-of-chain RETURN in
#   STYRMAN-FWD per (project marker, configured LAN prefix). The ONLY input
#   from the non-root server is the NAME of an empty marker file; root decides
#   what "local network" means from a root-owned conf, validated on every run.
#   These tagged RETURNs are the one class of rule this script deletes on
#   demand (marker gone = rule gone; tighten before loosen); the base rules
#   keep the never-delete promise. A no-op run performs reads only.
#
# Paths come from the environment with NO defaults (ADR-052): the systemd
# unit pins the production values; a hand-run or a test that does not set
# them applies the BASE section only and says so. Writes go only to
# $STYRMAN_FW_RUN_DIR (root-owned, outside the state dir); the state dir is
# only ever READ.
#
#   STYRMAN_FW_STATE_DIR   /var/lib/styrman   — markers at <dir>/netpolicy/lan/<uuid>
#   STYRMAN_FW_RUN_DIR     /run/styrman-firewall — spool/, status, lock
#   STYRMAN_FW_CONF        /etc/styrman/firewall.conf — LAN_DEV=, LAN_CIDRS=
#   STYRMAN_FW_PATH_UNIT   styrman-firewall.path — revived (with backoff) after
#                          a healthy run; unset = never touch systemd (tests)
#
# STYRMAN_FW_VERSION is written into the status file; the server refuses to
# enable LAN access when it is below internal/netpolicy.MinScriptVersion.
STYRMAN_FW_VERSION=3
set -eu
# Command substitutions inherit errexit too: a failure inside $(...) must
# abort the run like any other (a swallowed mktemp failure hid a broken
# status write during review).
shopt -s inherit_errexit

start_ns="$(date +%s%N)"
log()  { echo "styrman-firewall: $*"; }
WARNS=()
# warn: journal + status. note: status only (a standing condition such as a
# missing conf must not write a journal line every minute).
warn() { echo "styrman-firewall: WARN $*" >&2; WARNS+=("$*"); }
note() { WARNS+=("$*"); }

ip4="$(command -v iptables)"
# -w 5: wait for a concurrent netlink writer (dockerd) instead of failing the
# whole run under set -e (review).
ipt() { "$ip4" -w 5 "$@"; }

# ─── BASE: isolation rules (ADR-029/036), unchanged but for -w ───────────────
# The `-i sdb-+` wildcard appears only on the chain jumps (matched textually
# via -S, which renders it stably); the chains' own rules carry no wildcard,
# so `-C` is reliable for them.
modprobe xt_physdev 2>/dev/null || true   # same-bridge exemption below needs it

ensure_chain() { ipt -nL "$1" >/dev/null 2>&1 || ipt -N "$1"; }
ensure_rule()  { local c="$1"; shift; ipt -C "$c" "$@" 2>/dev/null || ipt -A "$c" "$@"; }
ensure_jump()  { # parent iface target — textual presence test is wildcard-safe
    ipt -S "$1" 2>/dev/null | grep -qF -- "-i $2 -j $3" || ipt -I "$1" -i "$2" -j "$3"
}

# One-time migration off the pre-chain direct rules from earlier versions.
ipt -D INPUT -i sdb-+ -j DROP 2>/dev/null || true
for net in 10.0.0.0/8 172.16.0.0/12 192.168.0.0/16 169.254.0.0/16; do
    ipt -D DOCKER-USER -i sdb-+ -d "$net" -j DROP 2>/dev/null || true
done

# Host itself: a project bridge may send it nothing.
ensure_chain STYRMAN-IN
ensure_rule  STYRMAN-IN -j DROP
ensure_jump  INPUT sdb-+ STYRMAN-IN

# Routed egress: RETURN genuinely same-bridge L2 first (broker + proxy on the
# project's OWN bridge), then drop every private range. Other projects live in
# 172.16/12 too, so cross-project stays blocked; public internet falls through.
# The per-project LAN allowances (below) are inserted AHEAD of all of these.
ensure_chain STYRMAN-FWD
ensure_rule  STYRMAN-FWD -m physdev --physdev-is-bridged -j RETURN 2>/dev/null \
    || echo "styrman-firewall: WARN xt_physdev unavailable — broker may be blocked" >&2
# 100.64.0.0/10 is CGNAT — where Tailscale/headscale put their tailnet, so a
# project could otherwise reach the operator's whole private mesh; 198.18.0.0/15
# is the benchmark range some VPNs and container fabrics route real traffic on.
# Both were missing from the original RFC1918 list (2026-09-01 audit).
for net in 10.0.0.0/8 172.16.0.0/12 192.168.0.0/16 169.254.0.0/16 \
           100.64.0.0/10 198.18.0.0/15; do
    ensure_rule STYRMAN-FWD -d "$net" -j DROP
done
ensure_jump  DOCKER-USER sdb-+ STYRMAN-FWD

# IPv6: this isolation is IPv4. Don't silently rely on the host's ipv6.disable
# boot flag — turn IPv6 off on each project bridge, and best-effort mirror the
# drops so a future IPv6-enabled bridge cannot bypass isolation.
for ifp in /sys/class/net/sdb-*; do
    [ -e "$ifp" ] || continue
    sysctl -qw "net.ipv6.conf.${ifp##*/}.disable_ipv6=1" 2>/dev/null || true
done
if command -v ip6tables >/dev/null 2>&1; then
    ip6="$(command -v ip6tables)"
    "$ip6" -nL STYRMAN-IN6 >/dev/null 2>&1 || "$ip6" -N STYRMAN-IN6 2>/dev/null || true
    "$ip6" -C STYRMAN-IN6 -j DROP 2>/dev/null || "$ip6" -A STYRMAN-IN6 -j DROP 2>/dev/null || true
    "$ip6" -S INPUT 2>/dev/null | grep -qF -- "-i sdb-+ -j STYRMAN-IN6" \
        || "$ip6" -I INPUT -i sdb-+ -j STYRMAN-IN6 2>/dev/null || true
    "$ip6" -nL STYRMAN-FWD6 >/dev/null 2>&1 || "$ip6" -N STYRMAN-FWD6 2>/dev/null || true
    for net6 in fc00::/7 fe80::/10; do
        "$ip6" -C STYRMAN-FWD6 -d "$net6" -j DROP 2>/dev/null || "$ip6" -A STYRMAN-FWD6 -d "$net6" -j DROP 2>/dev/null || true
    done
    "$ip6" -S FORWARD 2>/dev/null | grep -qF -- "-i sdb-+ -j STYRMAN-FWD6" \
        || "$ip6" -I FORWARD -i sdb-+ -j STYRMAN-FWD6 2>/dev/null || true
fi

# ─── LAN prelude: paths, guards, lock, spool ─────────────────────────────────
STATE="${STYRMAN_FW_STATE_DIR:-}"
RUN="${STYRMAN_FW_RUN_DIR:-}"
CONF="${STYRMAN_FW_CONF:-}"
PATH_UNIT="${STYRMAN_FW_PATH_UNIT:-}"
if [ -z "${STATE}" ] || [ -z "${RUN}" ] || [ -z "${CONF}" ]; then
    log "per-project LAN reconcile skipped: STYRMAN_FW_STATE_DIR/RUN_DIR/CONF unset (the unit sets them) — base rules only"
    exit 0
fi
lan_active=0
spool_seen=0

drain_spool() {
    [ -d "${RUN}/spool" ] || return 0
    # find -P never follows symlinks; -delete is depth-first, so a nested
    # directory entry goes too. Entry NAMES are never interpreted.
    find "${RUN}/spool" -mindepth 1 -delete 2>/dev/null || true
}
# On a FAILED exit the spool is drained so DirectoryNotEmpty cannot re-trigger
# the unit in a tight loop over the same failure (the 60 s timer retries).
# On a healthy exit entries that arrived mid-run are LEFT, so the path unit
# re-triggers and the next run picks up their markers: no toggle is lost.
on_exit() {
    local rc=$?
    if [ "${rc}" -ne 0 ] && [ "${lan_active}" = 1 ]; then drain_spool; fi
}
trap 'exit 143' TERM
trap on_exit EXIT

# The run dir must be absolute, outside the (operator-owned) state dir, and a
# root-owned directory not writable by group/others — the bar every other
# root-written path in this project meets. The spool below it is owned by the
# state dir's owner (the server's uid) so the non-root server can drop trigger
# files; its PARENT stays root-owned so that uid cannot swap `spool` for a
# symlink and redirect root's delete. install.sh mounts a 1 MiB / 1024-inode
# tmpfs on the spool so that uid can never fill /run through it (review).
prepare_run_dir() {
    case "${RUN}" in /*) ;; *) warn "run dir ${RUN} is not absolute — LAN section skipped"; return 1 ;; esac
    case "${RUN}/" in "${STATE}"/*) warn "run dir ${RUN} is inside the state dir — LAN section skipped"; return 1 ;; esac
    if [ -L "${RUN}" ]; then warn "run dir ${RUN} is a symlink — LAN section skipped"; return 1; fi
    [ -d "${RUN}" ] || install -d -m 0755 -o root -g root "${RUN}"
    if [ "$(stat -c %u "${RUN}")" != "0" ] || [ -n "$(find "${RUN}" -maxdepth 0 -perm /022)" ]; then
        warn "run dir ${RUN} is not a root-owned directory without group/world write — LAN section skipped"; return 1
    fi
    if [ ! -d "${STATE}" ] || [ -L "${STATE}" ]; then warn "state dir ${STATE} is not a directory — LAN section skipped"; return 1; fi
    if [ -L "${RUN}/spool" ]; then warn "${RUN}/spool is a symlink — LAN section skipped"; return 1; fi
    install -d -m 0700 -o "$(stat -c %u "${STATE}")" -g "$(stat -c %g "${STATE}")" "${RUN}/spool"
    return 0
}

prepare_run_dir || exit 0
exec 9>"${RUN}/lock"
# Blocking, bounded: a unit run waits for a hand-run instead of exiting with
# the spool still full (which would re-trigger immediately).
if ! flock -w 25 9; then
    warn "lock ${RUN}/lock busy for 25 s — LAN section skipped this run"
    exit 0
fi
lan_active=1
spool_seen="$(find "${RUN}/spool" -mindepth 1 -maxdepth 1 2>/dev/null | wc -l | tr -d ' ')"
drain_spool

# ─── LAN: per-project allowances (ADR-065) ───────────────────────────────────
UUID_RE='^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'
CIDR_RE='^([0-9]{1,3}\.){3}[0-9]{1,3}/[0-9]{1,2}$'
# Exactly how `iptables -S` renders our rule (iptables 1.8.11 nf_tables;
# pinned by deploy/firewall/test.sh). A well-formed rule is fully described
# by its five captures, so it is DELETED BY SPEC — never by a rule number that
# another root writer could shift between the read and the delete (review).
# Anything carrying our tag that does not match is not ours and is removed by
# number, after re-reading the chain and confirming the exact line is still
# at that number.
RULE_RE='^-A STYRMAN-FWD -s ([0-9./]+) -d ([0-9./]+) -i (sdb-[0-9a-f]{11}) -o ([A-Za-z0-9_.-]{1,15}) -m comment --comment "styrman-lan:([0-9a-f-]{36})" -j RETURN$'

LAN_DEV=""
LAN_CIDRS=()
CONF_ERROR=""
PENDING=()
ERRORS=()
declare -A DESIRED=()      # "iface dev src dst uuid" → 1
declare -A WANT_COUNT=()   # uuid → number of (uuid, cidr) rules wanted
declare -A SEEN=()         # key → occurrences in the chain
declare -A PURGE=()        # key → 1: delete every instance by spec
declare -A APPLIED_COUNT=()
MALFORMED=()               # exact -S lines to delete by (re-checked) number
added=0; removed=0; ignored=0
lan_list=""

conf_fail() { CONF_ERROR="$1"; LAN_CIDRS=(); }
is_docker_bridge_name() {
    case "$1" in docker0|sdb-*) return 0 ;; esac
    [[ "$1" =~ ^br-[0-9a-f]{12}$ ]]
}
ip2int() { local IFS=.; set -- $1; echo $(( ($1 << 24) | ($2 << 16) | ($3 << 8) | $4 )); }
# overlaps <a-int> <a-prefix> <b-int> <b-prefix>
overlaps() { local p; p=$(( $2 < $4 ? $2 : $4 )); [ $(( $1 >> (32 - p) )) -eq $(( $3 >> (32 - p) )) ]; }

validate_dev() {
    local dev="$1"
    if ! [[ "${dev}" =~ ^[A-Za-z0-9_.-]{1,15}$ ]]; then conf_fail "LAN_DEV missing or invalid"; return 1; fi
    if [ ! -e "/sys/class/net/${dev}" ]; then conf_fail "LAN_DEV ${dev} does not exist"; return 1; fi
    if is_docker_bridge_name "${dev}"; then conf_fail "LAN_DEV ${dev} is a Docker bridge"; return 1; fi
    # A bridged uplink (br0 on a KVM host) is legitimate; only warn.
    if [ -e "/sys/class/net/${dev}/bridge" ]; then note "LAN_DEV ${dev} is a kernel bridge - fine for a bridged uplink, wrong for a container bridge"; fi
    LAN_DEV="${dev}"
    return 0
}

validate_cidrs() {
    local raw="${1//,/ }" tok a b c d pre ipi deny d_ip d_pre rcidr rdev
    local -a deny_cidrs=(172.16.0.0/12 169.254.0.0/16 100.64.0.0/10 198.18.0.0/15 127.0.0.0/8 0.0.0.0/8 224.0.0.0/3)
    local -a out=()
    # plus the connected route of every Docker bridge present (future pools too)
    while read -r rcidr rdev; do
        [ -n "${rcidr}" ] || continue
        [[ "${rcidr}" == */* ]] || rcidr="${rcidr}/32"
        if is_docker_bridge_name "${rdev}"; then deny_cidrs+=("${rcidr}"); fi
    done < <(ip -o -4 route show scope link 2>/dev/null | awk '{print $1, $3}')
    for tok in ${raw}; do
        if ! [[ "${tok}" =~ ^([0-9]{1,3})\.([0-9]{1,3})\.([0-9]{1,3})\.([0-9]{1,3})/([0-9]{1,2})$ ]]; then
            conf_fail "LAN_CIDRS: ${tok} is not a.b.c.d/n"; return 1
        fi
        a=${BASH_REMATCH[1]}; b=${BASH_REMATCH[2]}; c=${BASH_REMATCH[3]}; d=${BASH_REMATCH[4]}; pre=${BASH_REMATCH[5]}
        if [ "$a" -gt 255 ] || [ "$b" -gt 255 ] || [ "$c" -gt 255 ] || [ "$d" -gt 255 ]; then conf_fail "LAN_CIDRS: ${tok} has an octet above 255"; return 1; fi
        if [ "${pre}" -lt 16 ] || [ "${pre}" -gt 30 ]; then conf_fail "LAN_CIDRS: ${tok} prefix must be /16../30"; return 1; fi
        ipi=$(( (a << 24) | (b << 16) | (c << 8) | d ))
        # iptables would silently normalise 192.168.86.7/24 to .0/24 and the
        # desired/current comparison would churn every run.
        if [ $(( ipi & ((1 << (32 - pre)) - 1) )) -ne 0 ]; then conf_fail "LAN_CIDRS: ${tok} has host bits set"; return 1; fi
        for deny in "${deny_cidrs[@]}"; do
            d_ip="$(ip2int "${deny%/*}")"; d_pre="${deny#*/}"
            if overlaps "${ipi}" "${pre}" "${d_ip}" "${d_pre}"; then conf_fail "LAN_CIDRS: ${tok} overlaps ${deny}"; return 1; fi
        done
        out+=("${tok}")
    done
    if [ ${#out[@]} -eq 0 ]; then note "LAN_CIDRS empty - no local-network prefix configured"; return 0; fi
    LAN_CIDRS=("${out[@]}")
    return 0
}

read_conf() {
    local line key val dev="" cidrs=""
    if [ ! -e "${CONF}" ]; then note "conf ${CONF} missing - no local-network prefix configured (deploy/firewall/install.sh writes it)"; return 0; fi
    if [ -L "${CONF}" ] || [ ! -f "${CONF}" ] || [ "$(stat -c %u "${CONF}")" != "0" ] || [ -n "$(find "${CONF}" -maxdepth 0 -perm /022)" ]; then
        conf_fail "conf ignored: must be a root-owned regular file not writable by group/others"; return 0
    fi
    while IFS= read -r line || [ -n "${line}" ]; do
        case "${line}" in ''|'#'*) continue ;; esac
        if [[ "${line}" =~ ^(LAN_DEV|LAN_CIDRS|LAN_ALLOW_HOST|LAN_ALLOW_PUBLISHED_PORTS)=([A-Za-z0-9./\ ,_-]*)$ ]]; then
            key="${BASH_REMATCH[1]}"; val="${BASH_REMATCH[2]}"
            case "${key}" in
                LAN_DEV) dev="${val}" ;;
                LAN_CIDRS) cidrs="${val}" ;;
                # Reserved for a later ADR (ADR-065 D3): parsed as 0, never acted on.
                *) if [ -n "${val}" ] && [ "${val}" != "0" ]; then note "conf: ${key} is reserved and ignored"; fi ;;
            esac
        else
            conf_fail "conf: unparseable line (only LAN_DEV= and LAN_CIDRS= are accepted)"; return 0
        fi
    done < "${CONF}"
    validate_dev "${dev}" || return 0
    validate_cidrs "${cidrs}" || return 0
}

# DESIRED: one rule per (uuid marker, LAN prefix). The interface name is
# recomputed from the uuid exactly as config.BridgeNameFor does — the marker's
# name is the whole grammar; its contents are never read. The marker tree
# (netpolicy/ and netpolicy/lan/) must be real directories owned by the state
# dir's owner or root; a symlink anywhere in it yields no allowances at all.
# Residual: between these checks and the glob the owning uid could still
# swap the directory, but root reads only NAMES here and that uid can create
# any name directly, so nothing is gained by the swap.
collect_desired() {
    local parent="${STATE}/netpolicy" markers="${STATE}/netpolicy/lan" f name iface subnet c owner d
    owner="$(stat -c %u "${STATE}")"
    for d in "${parent}" "${markers}"; do
        if [ ! -e "${d}" ]; then note "marker dir ${markers} missing (the server creates it) - no allowances"; return 0; fi
        if [ -L "${d}" ] || [ ! -d "${d}" ]; then warn "${d} is not a real directory - no allowances"; return 0; fi
        if [ "$(stat -c %u "${d}")" != "${owner}" ] && [ "$(stat -c %u "${d}")" != "0" ]; then warn "${d} is not owned by the state dir's owner or root - no allowances"; return 0; fi
    done
    shopt -s nullglob
    for f in "${markers}"/*; do
        name="${f##*/}"
        if ! [[ "${name}" =~ ${UUID_RE} ]] || [ -L "${f}" ] || [ ! -f "${f}" ]; then ignored=$((ignored + 1)); continue; fi
        iface="sdb-$(printf 'sdb-%s' "${name}" | sha1sum | cut -c1-11)"
        if [ ! -e "/sys/class/net/${iface}" ]; then PENDING+=("${name}:no-bridge"); continue; fi
        subnet="$(ip -o -4 route show dev "${iface}" scope link 2>/dev/null | awk '{print $1; exit}')"
        if ! [[ "${subnet}" =~ ${CIDR_RE} ]]; then PENDING+=("${name}:no-subnet"); continue; fi
        for c in "${LAN_CIDRS[@]}"; do DESIRED["${iface} ${LAN_DEV} ${subnet} ${c} ${name}"]=1; done
        WANT_COUNT["${name}"]=${#LAN_CIDRS[@]}
    done
    shopt -u nullglob
    if [ "${ignored}" -gt 0 ]; then note "${ignored} marker entries ignored (not a plain file named by a uuid)"; fi
}

# rule_args <key> — the argv of a well-formed rule, rebuilt from its captures
# (all regex-validated: iface, device, two CIDRs, uuid).
rule_args() {
    local iface dev src dst uuid
    read -r iface dev src dst uuid <<<"$1"
    ARGS=(-i "${iface}" -o "${dev}" -s "${src}" -d "${dst}" -m comment --comment "styrman-lan:${uuid}" -j RETURN)
}

# CURRENT: walk `-S` once. A well-formed key is kept only when it occurs
# exactly once, before the first DROP, and is desired; otherwise every
# instance is purged by spec and, if desired, re-inserted at the head.
scan_current() {
    local n=0 first_drop=0 line key
    while IFS= read -r line; do
        case "${line}" in "-A STYRMAN-FWD "*) ;; *) continue ;; esac
        n=$((n + 1))
        if [ "${first_drop}" -eq 0 ] && [[ "${line}" == *" -j DROP" ]]; then first_drop=${n}; fi
        case "${line}" in *styrman-lan:*) ;; *) continue ;; esac
        if [[ "${line}" =~ ${RULE_RE} ]]; then
            key="${BASH_REMATCH[3]} ${BASH_REMATCH[4]} ${BASH_REMATCH[1]} ${BASH_REMATCH[2]} ${BASH_REMATCH[5]}"
            SEEN["${key}"]=$(( ${SEEN[${key}]:-0} + 1 ))
            if [ "${SEEN[${key}]}" -gt 1 ]; then
                PURGE["${key}"]=1; warn "duplicate allow rule for project ${BASH_REMATCH[5]} - purged and re-inserted"
            elif [ "${first_drop}" -ne 0 ] && [ "${n}" -gt "${first_drop}" ]; then
                PURGE["${key}"]=1; warn "allow rule for project ${BASH_REMATCH[5]} found after a DROP - re-inserted at the head"
            elif [ -z "${DESIRED[${key}]:-}" ]; then
                PURGE["${key}"]=1; log "lan OFF project=${BASH_REMATCH[5]} iface=${BASH_REMATCH[3]} dst=${BASH_REMATCH[2]}"
            fi
        else
            MALFORMED+=("${line}"); warn "malformed styrman-lan rule removed"
        fi
    done < <(ipt -S STYRMAN-FWD 2>/dev/null || true)
}

# delete_malformed: by number, but only after re-reading the chain and
# confirming the exact line is still at that number — the only way to name a
# rule whose text we do not trust enough to replay.
delete_malformed() {
    local target line n
    for target in "${MALFORMED[@]}"; do
        n=0
        while IFS= read -r line; do
            case "${line}" in "-A STYRMAN-FWD "*) ;; *) continue ;; esac
            n=$((n + 1))
            if [ "${line}" = "${target}" ]; then
                if ipt -D STYRMAN-FWD "${n}"; then removed=$((removed + 1)); else ERRORS+=("rule ${n}:delete failed"); fi
                break
            fi
        done < <(ipt -S STYRMAN-FWD 2>/dev/null || true)
    done
}

reconcile() {
    local key
    # Delete FIRST (tighten before loosen): every instance of a purged key,
    # by spec — a spec delete can only ever remove a rule that IS the rule.
    for key in "${!PURGE[@]}"; do
        rule_args "${key}"
        while ipt -C STYRMAN-FWD "${ARGS[@]}" 2>/dev/null; do
            if ipt -D STYRMAN-FWD "${ARGS[@]}"; then removed=$((removed + 1)); else ERRORS+=("${key##* }:delete failed"); break; fi
        done
    done
    delete_malformed
    # Insert desired rules that are not (or no longer) present, at position 1:
    # ahead of the physdev RETURN and every DROP. `-C` first: a bare -I duplicates.
    for key in "${!DESIRED[@]}"; do
        if [ -n "${SEEN[${key}]:-}" ] && [ -z "${PURGE[${key}]:-}" ]; then continue; fi
        rule_args "${key}"
        if ipt -C STYRMAN-FWD "${ARGS[@]}" 2>/dev/null; then continue; fi
        if ipt -I STYRMAN-FWD 1 "${ARGS[@]}"; then
            added=$((added + 1)); log "lan ON project=${key##* } rule=${key% *}"
        else
            ERRORS+=("${key##* }:${key#* * * }:insert failed")
        fi
    done
}

# APPLIED: what the kernel holds after the reconcile — one read; a uuid is
# reported only when EVERY one of its (uuid, cidr) rules is present.
count_applied() {
    local line key uuid
    while IFS= read -r line; do
        [[ "${line}" =~ ${RULE_RE} ]] || continue
        key="${BASH_REMATCH[3]} ${BASH_REMATCH[4]} ${BASH_REMATCH[1]} ${BASH_REMATCH[2]} ${BASH_REMATCH[5]}"
        [ -n "${DESIRED[${key}]:-}" ] || continue
        uuid="${BASH_REMATCH[5]}"
        APPLIED_COUNT["${uuid}"]=$(( ${APPLIED_COUNT[${uuid}]:-0} + 1 ))
    done < <(ipt -S STYRMAN-FWD 2>/dev/null || true)
    for uuid in "${!WANT_COUNT[@]}"; do
        if [ "${WANT_COUNT[${uuid}]}" -gt 0 ] && [ "${APPLIED_COUNT[${uuid}]:-0}" -eq "${WANT_COUNT[${uuid}]}" ]; then
            lan_list+="${uuid} "
        elif [ "${WANT_COUNT[${uuid}]}" -gt 0 ]; then
            ERRORS+=("${uuid}:${APPLIED_COUNT[${uuid}]:-0} of ${WANT_COUNT[${uuid}]} rules present")
        fi
    done
}

# The status file names every LAN-enabled project and the LAN prefix: root
# 0640 with the state dir's group, so the server reads it and other local
# uids do not (review).
run_ms=0
write_status() {
    local t p e w
    run_ms=$(( ($(date +%s%N) - start_ns) / 1000000 ))
    t="$(mktemp "${RUN}/.status.XXXXXX")"
    {
        echo "schema=1"
        echo "version=${STYRMAN_FW_VERSION}"
        echo "applied_at=$(date +%s)"
        echo "lan_dev=${LAN_DEV}"
        echo "lan_cidrs=${LAN_CIDRS[*]:-}"
        echo "lan=${lan_list% }"
        for p in "${PENDING[@]}"; do echo "pending=${p}"; done
        for e in "${ERRORS[@]}"; do echo "error=${e}"; done
        if [ -n "${CONF_ERROR}" ]; then echo "conf_error=${CONF_ERROR}"; fi
        for w in "${WARNS[@]}"; do echo "warn=${w}"; done
        echo "spool=${spool_seen}"
        echo "run_ms=${run_ms}"
    } | tr -c 'A-Za-z0-9 ._:/=\n-' '_' > "${t}"
    chown "root:$(stat -c %g "${STATE}")" "${t}"
    chmod 0640 "${t}"
    mv -fT "${t}" "${RUN}/status"
}

read_conf
collect_desired
scan_current
reconcile
count_applied
write_status

# A TriggerLimit or StartLimit hit FAILS the path unit and it stops watching;
# a boot-time job deletion leaves it merely inactive. Revive either after a
# healthy run — at most once per ten minutes, so a uid that floods the spool
# buys itself one burst per revive, not a burst per timer tick (review).
# Never touched when the unit name is unset (tests).
if [ -n "${PATH_UNIT}" ] && [[ "${PATH_UNIT}" =~ ^[A-Za-z0-9@._-]+\.path$ ]]; then
    if ! systemctl is-active --quiet "${PATH_UNIT}" 2>/dev/null; then
        now_s="$(date +%s)"
        last_revive="$(cat "${RUN}/.revived" 2>/dev/null || echo 0)"
        [[ "${last_revive}" =~ ^[0-9]+$ ]] || last_revive=0
        if [ $(( now_s - last_revive )) -ge 600 ]; then
            if systemctl start "${PATH_UNIT}" 2>/dev/null; then
                log "revived ${PATH_UNIT}"; echo "${now_s}" > "${RUN}/.revived"
            else
                warn "could not start ${PATH_UNIT}"
            fi
        else
            note "${PATH_UNIT} inactive - next revive attempt in $(( 600 - (now_s - last_revive) ))s"
        fi
    fi
fi

# One journal line per run, and only when something changed or warned; quiet
# runs leave their evidence in the status file (applied_at, run_ms).
if [ "${added}" -gt 0 ] || [ "${removed}" -gt 0 ] || [ ${#ERRORS[@]} -gt 0 ] || [ -n "${CONF_ERROR}" ]; then
    log "lan desired=${#DESIRED[@]} added=${added} removed=${removed} errors=${#ERRORS[@]} dev=${LAN_DEV:-none} cidrs=${LAN_CIDRS[*]:-none} spool=${spool_seen} run_ms=${run_ms}${CONF_ERROR:+ conf_error=${CONF_ERROR}}"
fi
exit 0
