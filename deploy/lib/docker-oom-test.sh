#!/usr/bin/env bash
# deploy/lib/docker-oom-test.sh — exercises deploy/lib/docker-oom.sh against a
# scratch dir it creates itself (ADR-052): no root, no /etc/systemd.
# internal/config/docker_oom_script_test.go runs it under `go test ./...`.
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=docker-oom.sh
source "${HERE}/docker-oom.sh"
PASS=0; FAIL=0
ok()  { echo "PASS $*"; PASS=$((PASS + 1)); }
bad() { echo "FAIL $*"; FAIL=$((FAIL + 1)); }
expect() { if [[ "$2" == "$3" ]]; then ok "$1"; else bad "$1: want '$2', got '$3'"; fi; }

SCRATCH="$(mktemp -d)"
trap 'rm -rf "${SCRATCH}"' EXIT
WANT="$(docker_oom_dropin_content)"

case_() { # <name> <setup-fn> <first-run-want>
    local p="${SCRATCH}/$1/docker.service.d/50-styrman-oom.conf"
    "$2" "${p}"
    expect "$1: first run" "$3" "$(docker_oom_dropin "${p}")"
    expect "$1: second run" unchanged "$(docker_oom_dropin "${p}")"
    if [[ -f "${p}" && ! -L "${p}" ]]; then ok "$1: regular file"; else bad "$1: not a regular file"; fi
    expect "$1: content" "${WANT}" "$(cat "${p}")"
}
s_missing() { :; }
s_other()   { mkdir -p "$(dirname "$1")"; printf '[Service]\nOOMScoreAdjust=-500\n' > "$1"; }
s_correct() { mkdir -p "$(dirname "$1")"; printf '%s\n' "${WANT}" > "$1"; }
s_symlink() { mkdir -p "$(dirname "$1")"; printf 'target\n' > "${SCRATCH}/target"; ln -s "${SCRATCH}/target" "$1"; }
case_ missing s_missing changed
case_ other   s_other   changed
case_ correct s_correct unchanged
case_ symlink s_symlink changed
expect "symlink: its target untouched" target "$(cat "${SCRATCH}/target")"

# The settings the fix depends on, by value.
grep -qx 'OOMScoreAdjust=-999' <<<"${WANT}" && ok "drop-in sets OOMScoreAdjust=-999" || bad "OOMScoreAdjust=-999 missing"
grep -qx 'StartLimitIntervalSec=0' <<<"${WANT}" && ok "drop-in lifts the start limit" || bad "StartLimitIntervalSec=0 missing"
leftover="$(find "${SCRATCH}" -name '*.??????' | wc -l)"
expect "no temp files left" 0 "${leftover}"

echo "docker-oom: ${PASS} passed, ${FAIL} failed"
[[ "${FAIL}" -eq 0 ]]
