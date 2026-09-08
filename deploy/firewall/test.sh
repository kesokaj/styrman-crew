#!/usr/bin/env bash
# deploy/firewall/test.sh — exercises the REAL styrman-firewall.sh against
# scratch chains inside an unprivileged user+network+mount namespace, so the
# host's rules, paths and units are never touched (ADR-052). Run it as:
#
#   unshare -Urnm bash deploy/firewall/test.sh
#
# internal/netpolicy/script_test.go does exactly that under `go test ./...`
# (and skips, loudly, where user namespaces are unavailable). The namespace
# starts with an EMPTY ruleset; /sys is remounted so /sys/class/net shows only
# this namespace's interfaces; every path the script writes is under a
# mktemp dir. No sudo, no systemd, no production path.
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
# A user namespace that owns the netns may mount its own sysfs; without this
# /sys/class/net would still show the HOST's interfaces.
mount -t sysfs sysfs /sys
if [ "$(ls /sys/class/net | tr '\n' ' ')" != "lo " ]; then
    echo "expected an empty namespace (/sys/class/net = lo), got: $(ls /sys/class/net | tr '\n' ' ')" >&2; exit 2
fi

T="$(mktemp -d "${TMPDIR:-/tmp}/styrman-fwtest-XXXXXX")"
trap 'rm -rf "${T}"' EXIT
STATE="${T}/state"; RUN="${T}/run"; CONF="${T}/etc/firewall.conf"
MARK="${STATE}/netpolicy/lan"
mkdir -p "${MARK}" "${T}/etc"

# The golden vector shared with internal/config/bridgename_test.go and the
# live host: BridgeNameFor(NetworkName(U)) for this uuid is sdb-e660d58459e.
# printf, never echo: echo's trailing newline changes the hash.
U=3e6a772c-b177-4cfc-b209-b14db514bac2
IFACE="sdb-$(printf 'sdb-%s' "${U}" | sha1sum | cut -c1-11)"
check "golden bridge name" "[ '${IFACE}' = sdb-e660d58459e ]"

# Scratch topology: an uplink on TEST-NET-1, the project bridge with Docker's
# usual /16, and the DOCKER-USER chain Docker would have created.
ip link add enp0 type dummy; ip addr add 192.0.2.1/24 dev enp0; ip link set enp0 up
ip link add "${IFACE}" type dummy; ip addr add 172.20.0.1/16 dev "${IFACE}"; ip link set "${IFACE}" up
ip link set lo up
iptables -N DOCKER-USER

run() { STYRMAN_FW_STATE_DIR="${STATE}" STYRMAN_FW_RUN_DIR="${RUN}" STYRMAN_FW_CONF="${CONF}" bash "${SCRIPT}"; }
rules() { iptables -S STYRMAN-FWD | grep '^-A'; }
allow_count() { rules | grep -c 'styrman-lan:' || true; }
status() { sed -n "s/^$1=//p" "${RUN}/status"; }
write_conf() { printf 'LAN_DEV=%s\nLAN_CIDRS=%s\n' "$1" "$2" > "${CONF}"; chmod 0644 "${CONF}"; }
# shellcheck disable=SC2034  # referenced inside check() eval strings
EXPECT_RULE="-A STYRMAN-FWD -s 172.20.0.0/16 -d 192.0.2.0/24 -i ${IFACE} -o enp0 -m comment --comment \"styrman-lan:${U}\" -j RETURN"

echo "=== 1. env unset: base rules only, no status"
bash "${SCRIPT}" >/dev/null 2>&1
check "physdev RETURN present" "rules | grep -q -- '--physdev-is-bridged -j RETURN'"
check "six private-range DROPs" "[ \$(rules | grep -c -- '-j DROP') -eq 6 ]"
check "INPUT jump to STYRMAN-IN" "iptables -S INPUT | grep -qF -- '-i sdb-+ -j STYRMAN-IN'"
check "DOCKER-USER jump to STYRMAN-FWD" "iptables -S DOCKER-USER | grep -qF -- '-i sdb-+ -j STYRMAN-FWD'"
check "no status file without env" "[ ! -e '${RUN}/status' ]"

echo "=== 2. env set, no conf, no markers: idempotent, status written"
run >/dev/null 2>&1
before="$(rules)"; run >/dev/null 2>&1; after="$(rules)"
if [ "${before}" = "${after}" ]; then ok "second run changes nothing"; else bad "second run changes nothing"; fi
check "status schema=1 version=3" "[ \"\$(status schema)\" = 1 ] && [ \"\$(status version)\" = 3 ]"
check "lan_cidrs empty without conf" "[ -z \"\$(status lan_cidrs)\" ]"
check "run dir root 0755, spool 0700" "[ \"\$(stat -c %a '${RUN}')\" = 755 ] && [ \"\$(stat -c %a '${RUN}/spool')\" = 700 ]"

echo "=== 3. marker + conf: the exact rule at the head"
write_conf enp0 192.0.2.0/24
: > "${MARK}/${U}"
run >/dev/null 2>&1
check "line 1 is the allow rule, verbatim" "[ \"\$(rules | sed -n 1p)\" = \"\${EXPECT_RULE}\" ]"
check "line 2 is the physdev RETURN" "rules | sed -n 2p | grep -q -- '--physdev-is-bridged -j RETURN'"
check "DROPs follow" "[ \$(rules | sed -n '3,\$p' | grep -c -- '-j DROP') -eq 6 ]"
check "status lan= lists the uuid" "[ \"\$(status lan)\" = '${U}' ]"
check "status lan_dev/lan_cidrs" "[ \"\$(status lan_dev)\" = enp0 ] && [ \"\$(status lan_cidrs)\" = 192.0.2.0/24 ]"

echo "=== 4. no-op run: zero changes"
before="$(rules)"; run >/dev/null 2>&1; after="$(rules)"
if [ "${before}" = "${after}" ]; then ok "rules identical across a no-op run"; else bad "rules identical across a no-op run"; fi
check "exactly one allow rule" "[ \$(allow_count) -eq 1 ]"

echo "=== 5. duplicates and malformed tagged rules are purged"
iptables -I STYRMAN-FWD 1 -i "${IFACE}" -o enp0 -s 172.20.0.0/16 -d 192.0.2.0/24 -m comment --comment "styrman-lan:${U}" -j RETURN
iptables -I STYRMAN-FWD 1 -i sdb-deadbeef000 -m comment --comment "styrman-lan:not-a-uuid" -j RETURN
check "setup: three tagged rules" "[ \$(allow_count) -eq 3 ]"
run >/dev/null 2>&1
check "one allow rule remains" "[ \$(allow_count) -eq 1 ]"
check "it is the verbatim rule at the head" "[ \"\$(rules | sed -n 1p)\" = \"\${EXPECT_RULE}\" ]"
check "warn= recorded for the malformed rule" "grep -q '^warn=malformed' '${RUN}/status'"
check "the six DROPs beside the purged rules are intact" "[ \$(rules | grep -c -- '-j DROP') -eq 6 ]"

echo "=== 6. marker removed: rule gone (tighten)"
rm "${MARK}/${U}"
run >/dev/null 2>&1
check "no allow rule" "[ \$(allow_count) -eq 0 ]"
check "lan= empty" "[ -z \"\$(status lan)\" ]"
check "base rules intact" "[ \$(rules | grep -c -- '-j DROP') -eq 6 ]"

echo "=== 7. hostile marker names are ignored"
: > "${MARK}/${U}"
ln -s /etc/passwd "${MARK}/11111111-2222-4333-8444-555555555555"
: > "${MARK}/sys-setup-alice"
mkdir "${MARK}/22222222-2222-4222-8222-222222222222"
: > "${MARK}/3e6a772c-b177-4cfc-b209-b14db514bac2x"
run >/dev/null 2>&1
check "exactly one allow rule (the real marker)" "[ \$(allow_count) -eq 1 ]"
check "warn= counts four ignored entries" "grep -q '^warn=4 marker entries ignored' '${RUN}/status'"
rm -rf "${MARK}/11111111-2222-4333-8444-555555555555" "${MARK}/sys-setup-alice" "${MARK}/22222222-2222-4222-8222-222222222222" "${MARK}/3e6a772c-b177-4cfc-b209-b14db514bac2x"

echo "=== 8. spool is consumed, symlink targets untouched"
mkdir -p "${T}/victim"; : > "${T}/victim/keep"
ln -s "${T}/victim" "${RUN}/spool/link"
mkdir -p "${RUN}/spool/nested/deeper"; : > "${RUN}/spool/nested/deeper/x"
: > "${RUN}/spool/${U}.123"
run >/dev/null 2>&1
check "spool empty after the run" "[ -z \"\$(ls -A '${RUN}/spool')\" ]"
check "symlink target untouched" "[ -e '${T}/victim/keep' ]"
check "status spool=3" "[ \"\$(status spool)\" = 3 ]"

echo "=== 9. spool drained on a FAILED run (no re-trigger loop)"
# (a) A failure AFTER the lock — in the LAN section — must drain the spool on
# exit so DirectoryNotEmpty cannot re-trigger the unit over the same failure.
# The entry is planted by the failing mktemp itself, i.e. after the script's
# drain-first step, so only the EXIT trap can be what empties the spool
# (review: an entry written before the run would be caught by drain-first).
mkdir -p "${T}/fakebin"
printf '#!/bin/sh\n: > "%s/spool/%s.456"\nexit 1\n' "${RUN}" "${U}" > "${T}/fakebin/mktemp"; chmod +x "${T}/fakebin/mktemp"
if PATH="${T}/fakebin:${PATH}" STYRMAN_FW_STATE_DIR="${STATE}" STYRMAN_FW_RUN_DIR="${RUN}" STYRMAN_FW_CONF="${CONF}" bash "${SCRIPT}" >/dev/null 2>&1; then
    bad "run with a failing mktemp should exit non-zero"
else
    ok "run with a failing mktemp exits non-zero"
fi
check "spool entry planted mid-run was drained by the EXIT trap" "[ -z \"\$(ls -A '${RUN}/spool')\" ]"
rm -f "${T}/fakebin/mktemp"
# (b) A failure BEFORE the lock — in the base section — touches nothing the
# spool owner controls: the entry stays, and the path unit's bounded retry
# (TriggerLimitBurst / StartLimitBurst, then a ten-minute revive backoff) is
# what stops a loop. Deliberate: the base section runs before anything a
# lower-privileged uid can influence (security review).
: > "${RUN}/spool/${U}.789"
printf '#!/bin/sh\nexit 1\n' > "${T}/fakebin/iptables"; chmod +x "${T}/fakebin/iptables"
if PATH="${T}/fakebin:${PATH}" STYRMAN_FW_STATE_DIR="${STATE}" STYRMAN_FW_RUN_DIR="${RUN}" STYRMAN_FW_CONF="${CONF}" bash "${SCRIPT}" >/dev/null 2>&1; then
    bad "run with a failing iptables should exit non-zero"
else
    ok "run with a failing iptables exits non-zero"
fi
check "a base-section failure leaves the spool (retry is the path unit's, bounded)" "[ -e '${RUN}/spool/${U}.789' ]"
rm -f "${T}/fakebin/iptables"
run >/dev/null 2>&1   # heal (consumes the entry)
check "the next healthy run consumed it" "[ -z \"\$(ls -A '${RUN}/spool')\" ]"

echo "=== 10. flush then rerun restores base + allow in order"
iptables -F STYRMAN-FWD
run >/dev/null 2>&1
check "head is the allow rule" "[ \"\$(rules | sed -n 1p)\" = \"\${EXPECT_RULE}\" ]"
check "six DROPs restored" "[ \$(rules | grep -c -- '-j DROP') -eq 6 ]"

echo "=== 11. order guard: an allow after the DROPs moves to the head"
iptables -D STYRMAN-FWD 1
iptables -A STYRMAN-FWD -i "${IFACE}" -o enp0 -s 172.20.0.0/16 -d 192.0.2.0/24 -m comment --comment "styrman-lan:${U}" -j RETURN
check "setup: allow rule is last" "rules | tail -1 | grep -q styrman-lan"
run >/dev/null 2>&1
check "allow rule back at the head" "[ \"\$(rules | sed -n 1p)\" = \"\${EXPECT_RULE}\" ]"
check "exactly one copy" "[ \$(allow_count) -eq 1 ]"

echo "=== 12. conf validation fails closed"
conf_case() { # name dev cidrs expect-substring
    write_conf "$2" "$3"; run >/dev/null 2>&1
    check "conf '$1' -> conf_error and no allow rules" "grep -q \"^conf_error=$4\" '${RUN}/status' && [ \$(allow_count) -eq 0 ] && [ -z \"\$(status lan_cidrs)\" ]"
}
conf_case "host bits set"        enp0 192.0.2.7/24        "LAN_CIDRS: 192.0.2.7/24 has host bits set"
conf_case "project bridge route" enp0 172.20.0.0/16       "LAN_CIDRS: 172.20.0.0/16 overlaps"
conf_case "inside 172.16/12"     enp0 172.31.0.0/16       "LAN_CIDRS: 172.31.0.0/16 overlaps 172.16.0.0/12"
conf_case "link-local"           enp0 169.254.0.0/16      "LAN_CIDRS: 169.254.0.0/16 overlaps 169.254.0.0/16"
conf_case "tailnet"              enp0 100.64.0.0/16       "LAN_CIDRS: 100.64.0.0/16 overlaps 100.64.0.0/10"
conf_case "prefix too wide"      enp0 10.0.0.0/8          "LAN_CIDRS: 10.0.0.0/8 prefix must be"
conf_case "not a cidr"           enp0 "192.0.2.0"         "LAN_CIDRS: 192.0.2.0 is not"
conf_case "uplink is a Docker bridge" "${IFACE}" 192.0.2.0/24 "LAN_DEV ${IFACE} is a Docker bridge"
conf_case "uplink missing"       enp9 192.0.2.0/24        "LAN_DEV enp9 does not exist"
printf 'LAN_DEV=enp0\nLAN_CIDRS=192.0.2.0/24\nEVIL=$(reboot)\n' > "${CONF}"; run >/dev/null 2>&1
check "conf 'unknown key' -> conf_error, no allow rules" "grep -q '^conf_error=conf: unparseable' '${RUN}/status' && [ \$(allow_count) -eq 0 ]"
write_conf enp0 192.0.2.0/24; chmod 0666 "${CONF}"; run >/dev/null 2>&1
check "conf 'group/world-writable' -> ignored with conf_error" "grep -q '^conf_error=conf ignored' '${RUN}/status' && [ \$(allow_count) -eq 0 ]"
rm -f "${CONF}"; run >/dev/null 2>&1
check "conf missing -> no conf_error, empty lan_cidrs, no allow rules" "! grep -q '^conf_error=' '${RUN}/status' && [ -z \"\$(status lan_cidrs)\" ] && [ \$(allow_count) -eq 0 ]"
write_conf enp0 192.0.2.0/24; run >/dev/null 2>&1
check "valid conf again -> allow rule back" "[ \$(allow_count) -eq 1 ] && ! grep -q '^conf_error=' '${RUN}/status'"

echo "=== 13. reserved keys are parsed as 0 and ignored"
printf 'LAN_DEV=enp0\nLAN_CIDRS=192.0.2.0/24\nLAN_ALLOW_HOST=1\n' > "${CONF}"; run >/dev/null 2>&1
check "LAN_ALLOW_HOST=1 changes nothing, notes it" "[ \$(allow_count) -eq 1 ] && grep -q '^warn=conf: LAN_ALLOW_HOST is reserved' '${RUN}/status' && ! iptables -S INPUT | grep -q styrman-lan"
write_conf enp0 192.0.2.0/24

echo "=== 14. kernel-bridge uplink: warning only, rule pinned to it"
ip link add br0 type bridge; ip addr add 198.51.100.1/24 dev br0; ip link set br0 up
write_conf br0 198.51.100.0/24; run >/dev/null 2>&1
check "allow rule with -o br0" "rules | sed -n 1p | grep -q -- '-o br0 .*styrman-lan'"
check "kernel-bridge note recorded, no conf_error" "grep -q '^warn=LAN_DEV br0 is a kernel bridge' '${RUN}/status' && ! grep -q '^conf_error=' '${RUN}/status'"
write_conf enp0 192.0.2.0/24; run >/dev/null 2>&1

echo "=== 15. two prefixes: two rules, lan= only when both present"
write_conf enp0 "192.0.2.0/24 198.51.100.0/24"; run >/dev/null 2>&1
check "two allow rules" "[ \$(allow_count) -eq 2 ]"
check "lan= lists the uuid" "[ \"\$(status lan)\" = '${U}' ]"
iptables -D STYRMAN-FWD -i "${IFACE}" -o enp0 -s 172.20.0.0/16 -d 198.51.100.0/24 -m comment --comment "styrman-lan:${U}" -j RETURN
run >/dev/null 2>&1
check "missing rule re-inserted" "[ \$(allow_count) -eq 2 ]"
write_conf enp0 192.0.2.0/24; run >/dev/null 2>&1
check "dropped prefix -> its rule removed" "[ \$(allow_count) -eq 1 ]"

echo "=== 16. bridge lifecycle: no-bridge pending, appears, subnet change"
U2=aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee
IFACE2="sdb-$(printf 'sdb-%s' "${U2}" | sha1sum | cut -c1-11)"
: > "${MARK}/${U2}"; run >/dev/null 2>&1
check "pending=<uuid>:no-bridge" "grep -q '^pending=${U2}:no-bridge' '${RUN}/status'"
check "still one allow rule" "[ \$(allow_count) -eq 1 ]"
ip link add "${IFACE2}" type dummy; ip addr add 172.21.0.1/16 dev "${IFACE2}"; ip link set "${IFACE2}" up
run >/dev/null 2>&1
check "bridge appeared -> rule with its subnet" "rules | grep -q -- '-s 172.21.0.0/16 -d 192.0.2.0/24 -i ${IFACE2} -o enp0 .*styrman-lan:${U2}'"
check "lan= lists both" "[ \"\$(status lan | tr ' ' '\n' | sort | tr '\n' ' ')\" = '${U} ${U2} ' ]"
ip addr flush dev "${IFACE2}"; ip addr add 172.22.0.1/16 dev "${IFACE2}"
run >/dev/null 2>&1
check "subnet change -> old rule replaced" "rules | grep -q -- '-s 172.22.0.0/16 .*styrman-lan:${U2}' && ! rules | grep -q -- '-s 172.21.0.0/16'"
check "exactly two allow rules" "[ \$(allow_count) -eq 2 ]"
rm "${MARK}/${U2}"; run >/dev/null 2>&1
check "marker gone -> back to one" "[ \$(allow_count) -eq 1 ]"

echo "=== 17. status grammar and STYRMAN-IN untouched"
for k in schema version applied_at lan_dev lan_cidrs lan spool run_ms; do
    check "status has ${k}=" "grep -q '^${k}=' '${RUN}/status'"
done
check "applied_at is recent" "[ \$(( \$(date +%s) - \$(status applied_at) )) -lt 60 ]"
check "status is a regular 0640 file" "[ -f '${RUN}/status' ] && [ \"\$(stat -c %a '${RUN}/status')\" = 640 ]"
check "STYRMAN-IN is still just DROP" "[ \"\$(iptables -S STYRMAN-IN | grep '^-A')\" = '-A STYRMAN-IN -j DROP' ]"
check "nothing tagged in INPUT or DOCKER-USER" "! iptables -S INPUT | grep -q styrman-lan && ! iptables -S DOCKER-USER | grep -q styrman-lan"

echo "=== 18. guard: run dir inside the state dir is refused, base still applied"
iptables -F STYRMAN-FWD
STYRMAN_FW_STATE_DIR="${STATE}" STYRMAN_FW_RUN_DIR="${STATE}/run" STYRMAN_FW_CONF="${CONF}" bash "${SCRIPT}" >/dev/null 2>&1
check "base rules applied" "[ \$(rules | grep -c -- '-j DROP') -eq 6 ]"
check "no allow rules, nothing written under the state dir" "[ \$(allow_count) -eq 0 ] && [ ! -e '${STATE}/run' ]"
run >/dev/null 2>&1

echo "=== 19. syntax"
check "bash -n" "bash -n '${SCRIPT}' && bash -n '${HERE}/install.sh' && bash -n '${HERE}/test.sh'"
if command -v shellcheck >/dev/null 2>&1; then
    check "shellcheck (warning+)" "shellcheck -S warning '${SCRIPT}' '${HERE}/install.sh' '${HERE}/test.sh'"
fi

echo "passed=${PASS} failed=${FAIL}"
[ "${FAIL}" -eq 0 ]
