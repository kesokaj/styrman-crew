#!/usr/bin/env bash
# styrman-firewall.sh — project microVM network isolation (R12). Installed to
# /usr/local/sbin by deploy/firewall/install.sh; run by styrman-firewall.timer
# every 60 s.
#
# ONE job, since ADR-067: keep every SANDBOX project bridge (sdb-*) isolated
# from the host, the LAN, other projects, cloud metadata and the tailnet,
# while leaving the internet and the same-bridge broker + port-preview proxy
# reachable (ADR-029/036). Self-heal-safe: every rule is INSERT-IF-MISSING,
# never delete-then-insert, so the 60 s timer re-runs open no unprotected
# window.
#
# What this script no longer does, and why. ADR-065/066 gave it a second
# section that reconciled per-project LAN allowances from marker files the
# server wrote, driven by a spool, reporting through a status file, with a
# version handshake against the server. ADR-067 deleted all of it: the mode is
# now the bridge INTERFACE NAME, which the server picks at `docker network
# create`. A LAN project's bridge is `lan-<hash>`, so the `-i sdb-+` jumps
# below simply do not match it and it behaves like any other Docker container.
# Nothing here needs to know which projects those are, and root parses no
# input from the server at all any more.
#
# Cross-project isolation does NOT depend on this script: Docker writes
# `-A DOCKER ! -i <br> -o <br> -j DROP` per bridge, which is what enforces
# R12's cross-project guarantee for both modes.
#
# Paths come from the environment with NO defaults (ADR-052). Only the
# cleanup below reads one, and it writes nothing outside iptables.
set -eu
# Command substitutions inherit errexit too: a failure inside $(...) must
# abort the run like any other (a swallowed mktemp failure hid a broken
# status write during review).
shopt -s inherit_errexit

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

# Host itself: a project bridge may send it nothing.
ensure_chain STYRMAN-IN
ensure_rule  STYRMAN-IN -j DROP
ensure_jump  INPUT sdb-+ STYRMAN-IN

# Routed egress: RETURN genuinely same-bridge L2 first (broker + proxy on the
# project's OWN bridge), then drop every private range. Other projects live in
# 172.16/12 too, so cross-project stays blocked; public internet falls through.
# The per-project LAN allowances (below) are inserted AHEAD of all of these.
ensure_chain STYRMAN-FWD
# Through warn(), not a bare echo: without xt_physdev the same-bridge RETURN
# is missing and the private-range DROP below swallows the credential broker
# and the port preview for EVERY sandbox project — a degraded state that must
# reach the exit path like every other one here, not read as green in
# systemd (audit 2026-09-13, BUG-034's family).
ensure_rule  STYRMAN-FWD -m physdev --physdev-is-bridged -j RETURN 2>/dev/null \
    || warn "xt_physdev unavailable — the same-bridge RETURN is missing, so the credential broker and port preview are blocked for sandbox projects"
# 100.64.0.0/10 is CGNAT — where Tailscale/headscale put their tailnet, so a
# project could otherwise reach the operator's whole private mesh; 198.18.0.0/15
# is the benchmark range some VPNs and container fabrics route real traffic on.
# Both were missing from the original RFC1918 list (2026-09-01 audit).
for net in 10.0.0.0/8 172.16.0.0/12 192.168.0.0/16 169.254.0.0/16 \
           100.64.0.0/10 198.18.0.0/15; do
    ensure_rule STYRMAN-FWD -d "$net" -j DROP
done
ensure_jump  DOCKER-USER sdb-+ STYRMAN-FWD

# One-time migration off the pre-chain direct rules from earlier versions,
# LAST: the replacement chains and jumps above are already in place, so a
# first run on a pre-chain host never has a window with the old rules gone
# and the new ones not yet inserted. (They ran first until the 2026-09-08
# release review pointed out that this file's own never-delete-then-insert
# promise did not hold for its one deleting step.)
ipt -D INPUT -i sdb-+ -j DROP 2>/dev/null || true
for net in 10.0.0.0/8 172.16.0.0/12 192.168.0.0/16 169.254.0.0/16; do
    ipt -D DOCKER-USER -i sdb-+ -d "$net" -j DROP 2>/dev/null || true
done

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

# ─── One-shot cleanup: ADR-065/066 leftovers ─────────────────────────────────
# A host upgrading from v1.23/v1.24 may still carry tagged allowance rules
# this script no longer writes. Remove them by FULL CONTENT MATCH, never by
# rule number: an externally-writable ordered list shifts under a position
# captured in an earlier read (BUG-013). Refuse to delete a DROP whatever its
# comment says — the base rules keep the never-delete promise, and a tagged
# DROP would mean something has gone very wrong.
#
# Idempotent and silent once clean, so it stays safely in the timer path.
# The rule is matched by a STRICT whole-line regex and rebuilt from the
# captured groups, never by re-splitting arbitrary text: `iptables -S` quotes
# the comment, so naive word-splitting hands iptables a token with literal
# quotes (it fails), and a comment containing spaces would let a planted rule
# inject argv. The args group forbids `"` for that reason, so no second quoted
# match can hide inside it.
TAG_RE='^-A (STYRMAN-FWD|STYRMAN-IN) ([-A-Za-z0-9 ./+_]*) -m comment --comment "(styrman-lan|styrman-lanhost|styrman-lanpub):([0-9a-f-]{36})" -j RETURN$'
for chain in STYRMAN-FWD STYRMAN-IN; do
    ipt -nL "${chain}" >/dev/null 2>&1 || continue
    while IFS= read -r line; do
        case "${line}" in
            *"styrman-lan:"*|*"styrman-lanhost:"*|*"styrman-lanpub:"*) ;;
            *) continue ;;
        esac
        if [[ "${line}" == *" -j DROP" ]]; then
            warn "refusing to delete a tagged DROP in ${chain}: ${line}"
            continue
        fi
        if [[ ! "${line}" =~ ${TAG_RE} ]]; then
            warn "tagged rule in ${chain} does not match the ADR-065/066 shape - leaving it: ${line}"
            continue
        fi
        # shellcheck disable=SC2206  # deliberate word splitting of a group the regex constrained
        args=( ${BASH_REMATCH[2]} )
        if ipt -D "${BASH_REMATCH[1]}" "${args[@]}" \
               -m comment --comment "${BASH_REMATCH[3]}:${BASH_REMATCH[4]}" -j RETURN 2>/dev/null; then
            log "removed a stale ADR-065/066 allowance from ${chain}"
        else
            warn "could not remove a stale allowance from ${chain}: ${line}"
        fi
    done < <(ipt -S "${chain}" 2>/dev/null || true)
done

# The conf keys ADR-065/066 used (LAN_DEV, LAN_CIDRS, LAN_ALLOW_HOST,
# LAN_ALLOW_PUBLISHED_PORTS, INGRESS_BRIDGE) are no longer read by anything.
# They are left in place deliberately: this script does not edit an
# operator-owned file, and an unread key is inert. install.sh says so.

if [ "${#WARNS[@]}" -gt 0 ]; then
    exit 1
fi
exit 0
