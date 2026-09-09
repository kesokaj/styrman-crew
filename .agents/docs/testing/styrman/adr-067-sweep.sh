#!/usr/bin/env bash
# adr-067-sweep.sh — the completion check for ADR-067's deletions.
#
# One grep per deleted identifier across the whole tree. This exists because
# hand-curating a "surfaces touched" list FAILED: three cold-reader rounds
# found eleven items, nine of them the same class — a consumer of a deleted
# identifier that was not in the plan's table. Generating the list instead
# found eight more surfaces those three rounds had missed, including
# deploy/verify-commit.sh, which verified a unit file the change deletes and
# would have failed the release gate itself.
#
# Run from the repo root. Exits non-zero on any hit, listing it.
#
#   bash .agents/docs/testing/styrman/adr-067-sweep.sh
set -uo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." || exit 2

fail=0
# report takes the matches as an ARGUMENT, never on stdin. On stdin it would
# have to sit on the right of a pipe, which bash runs in a subshell, so
# `fail=1` would be set in a child and lost — the script printed "zero hits"
# while listing hits on its first run. Same family as BUG-010.
report() {
    local id="$1" out="$2"
    [ -n "${out}" ] || return 0
    echo "STILL PRESENT: ${id}"
    printf '%s\n' "${out}" | sed 's/^/    /'
    fail=1
}

# Everything below excludes this script itself (it necessarily names every
# identifier) and comment-only lines (the code that replaced a mechanism often
# explains what it replaced — BUG-020's fix names lanChangedAt on purpose).
SELF="adr-067-sweep.sh"
nocomments() { grep -vE "^[^:]+:[0-9]+: *(//|#|\*|/\*)"; }

# Go. `lanAccess` is deliberately absent: it is the one identifier that
# SURVIVES (Project.LanAccess, the API body, RunOpts.Lan).
for id in netpolicy 's\.netpol' 'NetPolicy()' NetPolicyDir lanFields hostFirewall \
          hostFirewallView ingressBridgeMismatch lanFor LanExposed kickLAN \
          pruneHostReachingCreds ingressBr BridgeNameForNetwork LanChecker \
          isHostReachingFile IsHostReaching LanEverEnabled LanChangedAt \
          RestoreLanAccess STYRMAN_FIREWALL_DIR FirewallRunDir LanState \
          LanMessage MinScriptVersion; do
    report "${id} (go)" "$(grep -rn "${id}" --include=*.go . 2>/dev/null | grep -v "${SELF}" | nocomments || true)"
done

# Web. Comments may discuss the history (BUG-020's explanation names
# lanChangedAt), so only CODE counts: strip anything after the first // or *.
for id in HostFirewall hostFirewall lanState lanMessage lanChangedAt LanState credentialImpact; do
    report "${id} (web)" "$(grep -rn "${id}" web/src web/e2e 2>/dev/null | grep -v node_modules | nocomments || true)"
done

# Shell, units, compose. The firewall script and its test legitimately name
# the styrman-lan* tags: that is the one-shot cleanup that REMOVES them.
report "shell/units" "$(grep -rn 'STYRMAN_FIREWALL_DIR\|LAN_CIDRS\|LAN_ALLOW\|lan_allow\|STYRMAN_FW_VERSION\|styrman-firewall\.path' \
    --include=*.sh --include=*.yml --include=*.service --include=*.timer --include=*.path \
    --include=styrman-update . 2>/dev/null \
    | grep -v "${SELF}" \
    | grep -v '^\./deploy/firewall/styrman-firewall\.sh:' \
    | grep -v '^\./deploy/firewall/test\.sh:' \
    | grep -v '^\./deploy/firewall/install\.sh:' \
    | nocomments || true)"

if [ "${fail}" -eq 0 ]; then
    echo "ADR-067 sweep: zero hits"
else
    echo "ADR-067 sweep: FAILED — the identifiers above outlived their mechanism" >&2
fi
exit "${fail}"
