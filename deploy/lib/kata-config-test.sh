#!/usr/bin/env bash
# deploy/lib/kata-config-test.sh — exercises deploy/lib/kata-config.sh against
# a scratch prefix it creates itself (ADR-052): no root, no /opt/kata, no
# /usr/local/bin. internal/config/kata_config_script_test.go runs it under
# `go test ./...`; by hand:
#
#   bash deploy/lib/kata-config-test.sh
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=kata-config.sh
source "${HERE}/kata-config.sh"
PASS=0; FAIL=0
ok()  { echo "PASS $*"; PASS=$((PASS + 1)); }
bad() { echo "FAIL $*"; FAIL=$((FAIL + 1)); }
expect() { # <name> <want> <got>
    if [[ "$2" == "$3" ]]; then ok "$1"; else bad "$1: want '$2', got '$3'"; fi
}

SCRATCH="$(mktemp -d)"
trap 'rm -rf "${SCRATCH}"' EXIT

PREFIX="${SCRATCH}/kata"
mkdir -p "${PREFIX}/bin" "${PREFIX}/share/defaults/kata-containers" "${SCRATCH}/bin"
printf '#!/bin/sh\n' > "${PREFIX}/bin/containerd-shim-kata-v2"
chmod 0755 "${PREFIX}/bin/containerd-shim-kata-v2"
WANT="#!/bin/bash
KATA_CONF_FILE=${PREFIX}/share/defaults/kata-containers/configuration-clh.toml \\
    exec ${PREFIX}/bin/containerd-shim-kata-v2 \"\$@\""

# ── Wrapper: four starting states, each run twice ────────────────────────────
wrapper_case() { # <name> <setup-fn> <first-run-want>
    local shim="${SCRATCH}/bin/shim-$1"
    "$2" "${shim}"
    expect "wrapper/$1: first run" "$3" "$(kata_shim_wrapper "${shim}" "${PREFIX}")"
    expect "wrapper/$1: second run" unchanged "$(kata_shim_wrapper "${shim}" "${PREFIX}")"
    if [[ -f "${shim}" && ! -L "${shim}" ]]; then ok "wrapper/$1: regular file"; else bad "wrapper/$1: not a regular file"; fi
    expect "wrapper/$1: mode" 755 "$(stat -c %a "${shim}")"
    expect "wrapper/$1: content" "${WANT}" "$(cat "${shim}")"
}
setup_missing() { :; }
setup_symlink() { ln -s "${PREFIX}/bin/containerd-shim-kata-v2" "$1"; }
setup_other()   { printf '#!/bin/bash\nexec /somewhere/else "$@"\n' > "$1"; chmod 0755 "$1"; }
setup_correct() { printf '%s\n' "${WANT}" > "$1"; chmod 0755 "$1"; }
setup_badmode() { printf '%s\n' "${WANT}" > "$1"; chmod 0644 "$1"; }
wrapper_case missing setup_missing changed
wrapper_case symlink setup_symlink changed
wrapper_case other   setup_other   changed
wrapper_case correct setup_correct unchanged
wrapper_case badmode setup_badmode changed
# The symlink case must replace the LINK, never write through it.
expect "wrapper/symlink: target untouched" '#!/bin/sh' "$(cat "${PREFIX}/bin/containerd-shim-kata-v2")"

# No Kata shim under the prefix: refuse, write nothing.
got="$(kata_shim_wrapper "${SCRATCH}/bin/shim-noprefix" "${SCRATCH}/nowhere")" && rc=0 || rc=$?
expect "wrapper/no kata: exit status" 1 "${rc}"
expect "wrapper/no kata: message" "missing: ${SCRATCH}/nowhere/bin/containerd-shim-kata-v2" "${got}"
if [[ ! -e "${SCRATCH}/bin/shim-noprefix" ]]; then ok "wrapper/no kata: nothing written"; else bad "wrapper/no kata: file written"; fi

# ── Reclaim: false, already true, key missing, file missing ─────────────────
TOML="${PREFIX}/share/defaults/kata-containers/configuration-clh.toml"
reclaim_toml() { # <value> — a fragment shaped like the packaged file
    printf '[hypervisor.clh]\n# Reclaim guest freed memory.\nreclaim_guest_freed_memory = %s\n\n# reclaim_guest_freed_memory = false (a comment, never edited)\nenable_hugepages = false\n' "$1"
}
reclaim_toml false > "${TOML}"
reclaim_toml true > "${SCRATCH}/want.toml"
expect "reclaim/false: first run" changed "$(kata_clh_reclaim "${TOML}")"
expect "reclaim/false: second run" unchanged "$(kata_clh_reclaim "${TOML}")"
if diff -q "${TOML}" "${SCRATCH}/want.toml" >/dev/null; then ok "reclaim/false: only the key changed"; else bad "reclaim/false: file differs: $(diff "${TOML}" "${SCRATCH}/want.toml" | tr '\n' ' ')"; fi

reclaim_toml true > "${TOML}"
expect "reclaim/true: first run" unchanged "$(kata_clh_reclaim "${TOML}")"
expect "reclaim/true: second run" unchanged "$(kata_clh_reclaim "${TOML}")"

printf '[hypervisor.clh]\nenable_hugepages = false\n' > "${TOML}"
cp "${TOML}" "${SCRATCH}/nokey.toml"
got="$(kata_clh_reclaim "${TOML}")" && rc=0 || rc=$?
expect "reclaim/no key: exit status" 1 "${rc}"
expect "reclaim/no key: message" "missing: reclaim_guest_freed_memory in ${TOML}" "${got}"
if diff -q "${TOML}" "${SCRATCH}/nokey.toml" >/dev/null; then ok "reclaim/no key: file untouched"; else bad "reclaim/no key: file changed"; fi

got="$(kata_clh_reclaim "${SCRATCH}/absent.toml")" && rc=0 || rc=$?
expect "reclaim/no file: exit status" 1 "${rc}"

# No temp file is left beside either target.
leftover="$(find "${SCRATCH}" -name '*.??????' | wc -l)"
expect "no temp files left" 0 "${leftover}"

echo "kata-config: ${PASS} passed, ${FAIL} failed"
[[ "${FAIL}" -eq 0 ]]
