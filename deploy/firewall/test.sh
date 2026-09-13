#!/usr/bin/env bash
# deploy/firewall/test.sh — exercises the REAL styrman-firewall.sh against
# scratch chains inside an unprivileged user+network+mount namespace, so the
# host's rules, paths and units are never touched (ADR-052). Run it as:
#
#   unshare -Urnm bash deploy/firewall/test.sh
#
# internal/config/firewall_script_test.go does exactly that under
# `go test ./...` (and skips, loudly, where user namespaces are unavailable).
# The namespace starts with an EMPTY ruleset; /sys is remounted so
# /sys/class/net shows only this namespace's interfaces. No sudo, no systemd,
# no production path.
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCRIPT="${HERE}/styrman-firewall.sh"
export PATH="/usr/sbin:/sbin:${PATH}"
PASS=0; FAIL=0
ok()  { echo "PASS $*"; PASS=$((PASS + 1)); }
bad() { echo "FAIL $*"; FAIL=$((FAIL + 1)); }
check() { if eval "$2"; then ok "$1"; else bad "$1"; fi; }

[ "$(id -u)" = 0 ] || { echo "not root inside the namespace — run under: unshare -Urnm bash $0" >&2; exit 2; }
# DEFINITIVE barrier before anything is mounted or changed (ADR-052, security
# review): this must be a network namespace other than the host's. A real-root
# run outside unshare sees pid 1's netns as its own and stops here.
if [ "$(readlink /proc/self/ns/net 2>/dev/null)" = "$(readlink /proc/1/ns/net 2>/dev/null)" ]; then
    echo "REFUSING: this is the host's network namespace — run under: unshare -Urnm bash $0" >&2; exit 2
fi
mount -t sysfs sysfs /sys
if [ "$(ls /sys/class/net | tr '\n' ' ')" != "lo " ]; then
    echo "expected an empty namespace (/sys/class/net = lo), got: $(ls /sys/class/net | tr '\n' ' ')" >&2; exit 2
fi

# The golden vector shared with internal/config/bridgename_test.go and the
# live host: BridgeNameFor(NetworkName(U), false) is sdb-e660d58459e, and with
# lan=true it is lan-e660d58459e — same suffix, different prefix (ADR-067).
# printf, never echo: echo's trailing newline changes the hash.
U=3e6a772c-b177-4cfc-b209-b14db514bac2
SUFFIX="$(printf 'sdb-%s' "${U}" | sha1sum | cut -c1-11)"
IFACE="sdb-${SUFFIX}"
LANIF="lan-${SUFFIX}"
check "golden sandbox bridge name" "[ '${IFACE}' = sdb-e660d58459e ]"
check "golden lan bridge name" "[ '${LANIF}' = lan-e660d58459e ]"

# Scratch topology: an uplink on TEST-NET-1, a SANDBOX project bridge and a
# LAN project bridge with Docker's usual /16s, and the DOCKER-USER chain
# Docker would have created.
ip link add enp0 type dummy; ip addr add 192.0.2.1/24 dev enp0; ip link set enp0 up
ip link add "${IFACE}" type dummy; ip addr add 172.20.0.1/16 dev "${IFACE}"; ip link set "${IFACE}" up
ip link add "${LANIF}" type dummy; ip addr add 172.21.0.1/16 dev "${LANIF}"; ip link set "${LANIF}" up
ip link set lo up
iptables -N DOCKER-USER

# The script exits 1 whenever it has WARNS (a missing xt_physdev included);
# the assertions are the check, so a warning exit must not abort the suite
# under set -e — but the status is KEPT in RUNRC and asserted where it
# matters (review 2026-09-13: `|| true` alone made the exit status
# unobservable in both directions).
RUNRC=0
# shellcheck disable=SC2034  # read inside the check strings the helper evals
run() { local rc=0; bash "${SCRIPT}" || rc=$?; RUNRC=${rc}; return 0; }
rules() { iptables -S STYRMAN-FWD | grep '^-A'; }
in_rules() { iptables -S STYRMAN-IN | grep '^-A'; }
tagged_count() { { rules; in_rules; } | grep -c 'styrman-lan' || true; }

echo "=== 1. base rules"
run >/dev/null 2>&1
check "a clean run exits 0 (no WARNS)" "[ \"\${RUNRC}\" = 0 ]"
check "physdev RETURN present" "rules | grep -q -- '--physdev-is-bridged -j RETURN'"
check "six private-range DROPs" "[ \$(rules | grep -c -- '-j DROP') -eq 6 ]"
check "STYRMAN-IN is a bare DROP" "[ \"\$(in_rules)\" = '-A STYRMAN-IN -j DROP' ]"
check "INPUT jump to STYRMAN-IN" "iptables -S INPUT | grep -qF -- '-i sdb-+ -j STYRMAN-IN'"
check "DOCKER-USER jump to STYRMAN-FWD" "iptables -S DOCKER-USER | grep -qF -- '-i sdb-+ -j STYRMAN-FWD'"

echo "=== 2. ADR-067: the jumps match the sandbox prefix and NOT the lan prefix"
# This is the whole feature expressed as a rule-level assertion. The jumps are
# wildcard `sdb-+`, so a lan-<hash> interface is reached by nothing here — and
# a sandbox one must always be.
check "no rule anywhere names the lan- interface" \
    "! iptables -S | grep -q -- '${LANIF}'"
check "the sdb- wildcard covers the sandbox iface" \
    "[ \"\$(iptables -S INPUT | grep -c -- '-i sdb-+')\" = 1 ]"
check "no rule names a lan- wildcard" "! iptables -S | grep -q -- 'lan-+'"

echo "=== 3. idempotence and self-heal"
before="$(rules)"; run >/dev/null 2>&1; after="$(rules)"
if [ "${before}" = "${after}" ]; then ok "second run changes nothing"; else bad "second run changes nothing"; fi
iptables -F STYRMAN-FWD
run >/dev/null 2>&1
check "flushed chain is rebuilt" "[ \$(rules | grep -c -- '-j DROP') -eq 6 ]"
check "physdev RETURN restored" "rules | grep -q -- '--physdev-is-bridged -j RETURN'"
iptables -D INPUT -i sdb-+ -j STYRMAN-IN
run >/dev/null 2>&1
check "deleted INPUT jump is reinserted" "iptables -S INPUT | grep -qF -- '-i sdb-+ -j STYRMAN-IN'"

echo "=== 4. ADR-065/066 leftovers are cleaned up"
# A host upgrading from v1.23/v1.24 carries these. All three families, in both
# chains, plus one in the middle of the chain rather than at the head.
iptables -I STYRMAN-FWD 1 -s 172.20.0.0/16 -d 192.0.2.0/24 -i "${IFACE}" -o enp0 \
    -m comment --comment "styrman-lan:${U}" -j RETURN
iptables -A STYRMAN-FWD -s 172.20.0.0/16 -d 192.0.2.0/24 -i "${IFACE}" \
    -m comment --comment "styrman-lanpub:${U}" -j RETURN
iptables -I STYRMAN-IN 1 -s 172.20.0.0/16 -d 192.0.2.1/32 -i "${IFACE}" \
    -m comment --comment "styrman-lanhost:${U}" -j RETURN
check "setup: three tagged rules planted" "[ \$(tagged_count) -eq 3 ]"
run >/dev/null 2>&1
check "all three are removed" "[ \$(tagged_count) -eq 0 ]"
check "the six DROPs are intact" "[ \$(rules | grep -c -- '-j DROP') -eq 6 ]"
check "STYRMAN-IN is a bare DROP again" "[ \"\$(in_rules)\" = '-A STYRMAN-IN -j DROP' ]"
run >/dev/null 2>&1
check "cleanup is idempotent" "[ \"\${RUNRC}\" = 0 ] && [ \$(tagged_count) -eq 0 ]"

echo "=== 5. the cleanup refuses to delete a DROP"
# A tagged DROP would mean something has gone badly wrong; deleting rules by
# content must still never remove one (BUG-013's neighbouring hazard).
iptables -I STYRMAN-FWD 1 -d 10.0.0.0/8 -m comment --comment "styrman-lan:${U}" -j DROP
run >/dev/null 2>&1
check "a run with a refused DROP exits 1 (the WARN reaches the exit path)" "[ \"\${RUNRC}\" = 1 ]"
check "the tagged DROP survives" \
    "rules | grep -q -- '--comment \"styrman-lan:${U}\" -j DROP'"
# `{ ...; } || true` before the pipe: the script deliberately exits 1 when it
# has warnings, and `set -o pipefail` would otherwise fail this check even
# though grep matched.
check "and the run reports a warning" "{ run 2>&1 || true; } | grep -q 'refusing to delete a tagged DROP'"
iptables -D STYRMAN-FWD -d 10.0.0.0/8 -m comment --comment "styrman-lan:${U}" -j DROP

echo "=== 6. IPv6 mirror"
if command -v ip6tables >/dev/null 2>&1; then
    run >/dev/null 2>&1
    check "STYRMAN-IN6 drops" "ip6tables -S STYRMAN-IN6 2>/dev/null | grep -q -- '-j DROP'"
    check "INPUT6 jump on the sdb- wildcard" "ip6tables -S INPUT 2>/dev/null | grep -qF -- '-i sdb-+ -j STYRMAN-IN6'"
    check "no ip6 rule names the lan- interface" "! ip6tables -S 2>/dev/null | grep -q -- '${LANIF}'"
else
    echo "SKIP ip6tables unavailable"
fi

echo
echo "PASS=${PASS} FAIL=${FAIL}"
[ "${FAIL}" -eq 0 ]
