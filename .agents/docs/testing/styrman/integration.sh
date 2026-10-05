#!/usr/bin/env bash
# styrman integration test — exercises the full lifecycle against real
# Docker + Kata: create project → terminals → WebSocket I/O → transcripts →
# file explorer → ports/preview → git → persistence → cleanup.
#
# Prereqs: styrman-runtime image built (image/), Kata runtime 'mvm' working.
# Uses an isolated state dir and port; safe to run alongside a real install.
#
# Default image tag is styrman-runtime:test, NOT styrman-runtime:dev — on
# this host, deploy/.env sets STYRMAN_IMAGE=styrman-runtime:dev, so :dev is
# what production actually boots. Building/tagging :dev here would re-stamp
# the production image (it happened during the 2026-09-01 audit). Pass an
# explicit tag as $1 to target something else.
#
# Usage: bash .agents/docs/testing/styrman/integration.sh [image-tag]
set -euo pipefail

IMAGE="${1:-styrman-runtime:test}"
# A stale image is rebuilt before the first check (BUG-026); FATAL on failure.
bash "$(dirname "${BASH_SOURCE[0]}")/ensure-test-image.sh" "${IMAGE}"
# Overridable, and REFUSED when already bound: on 2026-09-11 a hand-walk's
# local server sat on 18080, this suite's own server could not bind, and the
# readiness probe below was answered by the OTHER server — 90 checks ran
# green against the wrong target (BUG-040). A green run against the wrong
# server is worse than a red one.
PORT="${STYRMAN_ITEST_PORT:-18080}"
# Fail CLOSED: without `ss` the old pipeline was simply false and the guard
# passed (review finding 2026-09-13); capture first, then grep, so a failing
# `ss` is a FATAL of its own rather than a silent pass.
command -v ss >/dev/null 2>&1 || { echo "FATAL: ss (iproute2) is required to prove port ${PORT} is free" >&2; exit 1; }
bound="$(ss -ltn 2>/dev/null | awk '{print $4}')" || { echo "FATAL: ss failed; cannot prove port ${PORT} is free" >&2; exit 1; }
if printf '%s\n' "${bound}" | grep -qE "[:.]${PORT}$"; then
    echo "FATAL: port ${PORT} is already bound — another server would answer this suite's health probe (set STYRMAN_ITEST_PORT to pick another)" >&2
    exit 1
fi
ME="$(id -un)"        # auth-none mode acts as the OS user
ID=""                 # project UUID, captured after create
CN=""                 # container name styrman-<uuid>
# On DISK, never /tmp: /tmp is tmpfs on this host, and a project home on
# tmpfs pays nothing for a sync — on 2026-10-02 the boot-budget check below
# read 3.26 s for an image whose harness install costs 12.8 s on ext4 (the
# same run on /var/tmp read 16.16 s and failed). Production homes are on
# disk, so the budget is measured there (ADR-078; plan 2026-10-02, D12).
STATE="$(mktemp -d /var/tmp/styrman-itest-XXXX)"
STATE_FS="$(stat -f -c %T "${STATE}")"
REPO="$(cd "$(dirname "$0")/../../../.." && pwd)"
# ADR-067's network-mode checks read JSON fields; fail loudly up front rather
# than letting every one of them compare against an empty string.
command -v jq >/dev/null 2>&1 || { echo "FATAL: jq is required (network-mode checks)" >&2; exit 1; }
API="http://127.0.0.1:${PORT}"
PASS=0; FAIL=0
ABORTED=1   # cleared by the last line; a set -e abort must never report green

say()  { echo -e "\n\033[1;34m=== $* ===\033[0m"; }
ok()   { echo -e "\033[32mPASS\033[0m $*"; PASS=$((PASS+1)); }
bad()  { echo -e "\033[31mFAIL\033[0m $*"; FAIL=$((FAIL+1)); }
skip() { echo -e "\033[33mSKIP\033[0m $*"; SKIP=$((SKIP+1)); }
check(){ if eval "$2"; then ok "$1"; else bad "$1"; fi; }
SKIP=0
# wait_seed — block, bounded, until this boot's harness step is over. Since
# ADR-078 the harness's file install runs AFTER ready, in the background
# (about 13 s on a disk-backed home), and the entrypoint writes
# /run/styrman/harness-done last on every path. Every harness assertion that
# follows a boot calls this first. It never fails by itself: a marker that
# does not appear shows as the FAIL of the checks that follow, and the first
# boot asserts the marker outright.
wait_seed() {
    for _ in $(seq 1 150); do
        docker exec "${CN}" test -f /run/styrman/harness-done 2>/dev/null && return 0
        sleep 1
    done
    return 0
}

cleanup() {
    say "cleanup"
    kill "${SERVER_PID:-}" 2>/dev/null || true
    [ -n "${CN}" ] && docker rm -f "${CN}" 2>/dev/null || true
    [ -n "${PUBCN:-}" ] && docker rm -f "${PUBCN}" 2>/dev/null || true
    [ -n "${ID}" ] && docker network rm "sdb-${ID}" 2>/dev/null || true
    sudo rm -rf "${STATE}" 2>/dev/null || rm -rf "${STATE}" || true
    echo "passed=${PASS} failed=${FAIL} skipped=${SKIP} aborted=${ABORTED}"
    # An EXIT trap's status does NOT become the script's exit status (audit
    # finding: failing runs exited 0) — exit explicitly from the trap. A
    # run that never reached the end (set -e) is a failure too (release
    # review 2026-09-04: a truncated run reported green).
    exit $(( FAIL == 0 && ABORTED == 0 ? 0 : 1 ))
}
trap cleanup EXIT

say "build"
cd "${REPO}"
go build -o "${STATE}/styrman" ./cmd/styrman/
go build -o "${STATE}/wsprobe" ./cmd/wsprobe/

say "start server (state=${STATE}, port=${PORT})"
mkdir -p "${STATE}/home/projects"   # legacy layout: exercises the migration
STYRMAN_STATE_DIR="${STATE}" STYRMAN_IMAGE="${IMAGE}" STYRMAN_ADDR="127.0.0.1:${PORT}" \
    STYRMAN_UPDATE_STATUS_FILE="${STATE}/status/updater.json" \
    "${STATE}/styrman" > "${STATE}/server.log" 2>&1 &
SERVER_PID=$!
for _ in $(seq 1 20); do
    curl -fsS "${API}/api/health" >/dev/null 2>&1 && break
    sleep 0.5
done
check "health endpoint" "curl -fsS ${API}/api/health | grep -q ok"
check "embedded SPA served" "curl -fsS ${API}/ | grep -q '<title>crew</title>'"
check "SPA deep-link fallback" "curl -fsS ${API}/p/x/1 | grep -q '<title>crew</title>'"
check "unknown API path is a JSON 404" \
    "[ \$(curl -s -o /dev/null -w '%{http_code}' ${API}/api/nope) = 404 ]"

say "create project (boots microVM, git init)"
T0=$(date +%s)
OUT=$(curl -fsS -X POST "${API}/api/projects" -d '{"name":"itest","gitInit":true}')
T1=$(date +%s)
echo "created in $((T1-T0))s: ${OUT}"
ID=$(echo "${OUT}" | grep -oE '"id":"[0-9a-f-]{36}"' | cut -d'"' -f4)
CN="styrman-${ID}"
check "create returns immediately (async boot)" "echo '${OUT}' | grep -q '\"state\":\"creating\"'"
check "project got a UUID" "[ -n '${ID}' ]"
for _ in $(seq 1 60); do
    docker exec "${CN}" test -f /run/styrman/ready 2>/dev/null && break
    sleep 1
done
check "microVM became ready in the background" "docker exec ${CN} test -f /run/styrman/ready"
check "microVM kernel differs from host" \
    "[ \"\$(docker exec ${CN} uname -r)\" != \"\$(uname -r)\" ]"
check "workspace dir on host" "[ -d '${STATE}/projects/${ME}/${ID}' ]"
check "git repo initialized" "docker exec ${CN} test -d /home/$(id -un)/projects/itest/.git"
check "in-guest dockerd runs overlay2" \
    "docker exec ${CN} docker info --format '{{.Driver}}' | grep -q overlay2"

say "agent harness (ADR-074: the image's sh-agent release binary, installed write-once into the home)"
wait_seed
check "harness step finished after ready (harness-done marker, ADR-078)" \
    "docker exec ${CN} test -f /run/styrman/harness-done"
GH="/home/${ME}"
# The stamp is version+release (ADR-074): read once from the guest, as the
# entrypoint composes it, and compared verbatim below and after the recreate.
HREL="$(docker exec "${CN}" sh -c '/styrman/bin/sh-agent version | cut -d" " -f2' 2>/dev/null || true)"
HID="$(docker exec "${CN}" sh -c 'printf "%s+%s" "$(cat /styrman/VERSION)" "$(/styrman/bin/sh-agent version | cut -d" " -f2)"' 2>/dev/null || true)"
check "image sh-agent is a release binary (version is a canonical vX.Y.Z)" \
    "printf '%s' '${HREL}' | grep -Eq '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$'"
# install copies the running binary into the home: that copy is what the
# shell resolves (~/.local/bin is first on PATH, skel/bashrc) and what
# `sh-agent update` later replaces. Same release as the image's on a fresh seed.
check "home sh-agent (~/.local/bin) is the image's release" \
    "[ \"\$(docker exec ${CN} runuser -u ${ME} -- env HOME=${GH} ${GH}/.local/bin/sh-agent version)\" = 'sh-agent ${HREL}' ]"
# The manifest names the release the harness came from — the shape the
# entrypoint's never-downgrade guard reads (embedded:vX.Y.Z, a path would be
# a pre-ADR-074 install).
check "manifest records the embedded release (state.json repo = embedded:${HREL})" \
    "docker exec ${CN} runuser -u ${ME} -- jq -e '.repo == \"embedded:${HREL}\"' ${GH}/.config/sh-agent/state.json >/dev/null"
check "sh-agent reports no drift in the project home" \
    "docker exec ${CN} runuser -u ${ME} -- env HOME=${GH} ${GH}/.local/bin/sh-agent status --home ${GH} >/dev/null"
check "CLAUDE.md is the two-line import stub" \
    "docker exec ${CN} grep -qx '@AGENTS.md' ${GH}/.claude/CLAUDE.md"
# The seeded roster equals what the binary carries: the counts come from the
# binary's own `list` and from the directories, never from a constant.
check "skills and agy agents seeded equal the binary's inventory" \
    "n=\"\$(docker exec ${CN} /styrman/bin/sh-agent list | sed -n 's/^### Skills (\\([0-9]*\\))$/\\1/p')\" && a=\"\$(docker exec ${CN} /styrman/bin/sh-agent list | sed -n 's/^### Subagents (\\([0-9]*\\))$/\\1/p')\" && [ -n \"\$n\" ] && [ -n \"\$a\" ] && [ \"\$(docker exec ${CN} sh -c 'ls ${GH}/.claude/skills | wc -l')\" = \"\$n\" ] && [ \"\$(docker exec ${CN} sh -c 'ls ${GH}/.gemini/config/agents | wc -l')\" = \"\$a\" ]"
# The boot log's own pass line names the release (of install's stdout only
# its settings.json lines reach the boot log, so the pass line is still the
# in-guest evidence that install succeeded this boot). Both values are
# captured first and length-checked so a failed read cannot leave a
# truncated pattern that still matches. `grep -F … >/dev/null`, never
# `grep -q` (BUG-057).
check "boot log records the harness deployed from the image's sh-agent release" \
    "v=\"\$(docker exec ${CN} cat /styrman/VERSION)\" && [ -n \"\$v\" ] && [ -n '${HREL}' ] && docker logs ${CN} 2>&1 | grep -F \"agent harness deployed (version \$v, sh-agent ${HREL};\" >/dev/null"
# ADR-072 (BUG-061): the harness now WRITES one file outside its own
# directories — ~/.claude/settings.json with the attribution key — and the
# entrypoint logs the installer's line about it; both are asserted, and the
# old "settings.json absent" clause is gone with the behaviour it described.
check "boot log carries the installer's settings: line for the created file (ADR-072)" \
    "docker logs ${CN} 2>&1 | grep -F \"settings: ${GH}/.claude/settings.json created with\" >/dev/null"
check "no previous-harness artifacts (shell gates, GEMINI.md)" \
    "! docker exec ${CN} sh -c 'test -e ${GH}/.agents/bin || test -e ${GH}/.gemini/GEMINI.md'"
check "settings.json holds exactly the attribution key, trailers off (ADR-072)" \
    "docker exec ${CN} runuser -u ${ME} -- jq -e '.attribution.commit == \"\" and .attribution.pr == \"\" and (keys == [\"attribution\"])' ${GH}/.claude/settings.json >/dev/null"

say "browser (ADR-068: one headless Chromium per project, MCP-registered for both agents)"
# The registration is the seeding block's third step; both files are the
# tools' OWN MCP registries, not settings.json (ADR-062 D8 still holds).
# The harness stamp is what makes the next boot skip re-seeding; its write
# passes the id as an argument (audit 2026-09-13), so it must equal the
# image's VERSION plus the binary's release exactly (ADR-074) — a quoting
# slip would show here as a mismatch or a missing stamp.
check "harness stamp equals version+release after boot" \
    "v=\"\$(docker exec ${CN} cat ${GH}/.styrman/.harness-version)\" && [ -n \"\$v\" ] && [ \"\$v\" = '${HID}' ]"
check "browser MCP server registered for claude and agy" \
    "docker exec ${CN} sh -c 'jq -e \".mcpServers[\\\"styrman-browser\\\"].command == \\\"/styrman/bin/styrman-browser\\\"\" ${GH}/.claude.json >/dev/null && jq -e \".mcpServers[\\\"styrman-browser\\\"].command == \\\"/styrman/bin/styrman-browser\\\" and .mcpServers[\\\"styrman-browser\\\"].disabled == false\" ${GH}/.gemini/config/mcp_config.json >/dev/null'"
# As the project user with its HOME, exactly how the server (Phase 2) and the
# MCP verb invoke it. 9229 is Chromium's own loopback listener (what in-guest
# clients use); the socat door on 9222 admits only the server's bridge
# address, so loopback is refused there by design. Chromium's own sandbox
# stays on (no --no-sandbox).
check "styrman-browser start answers /json/version with the sandbox on" \
    "docker exec ${CN} runuser -u ${ME} -- env HOME=${GH} /styrman/bin/styrman-browser start >/dev/null && docker exec ${CN} curl -fsS http://127.0.0.1:9229/json/version | grep -q '\"Browser\": \"Chrome/' && ! docker exec ${CN} sh -c 'tr \"\\0\" \" \" < /proc/\$(cat ${GH}/.styrman/browser/pid)/cmdline' | grep -q -- --no-sandbox"
# Server side (Phase 2): the DevTools ports are owner-only through the relay —
# never listed (the Ports panel is where every share/link starts), never
# previewed. A bare exec'd start (no STYRMAN_BROWSER_PEER) leaves the browser
# loopback-only by design, so 9229 is the port that must be listening and
# hidden here; 9222 is covered by the routes check below, where the SERVER
# starts it with the peer.
check "ports list omits the browser's 9229 while it listens" \
    "docker exec ${CN} runuser -u ${ME} -- env HOME=${GH} /styrman/bin/styrman-browser start >/dev/null && docker exec ${CN} sh -c 'ss -ltnH | grep -q \":9229 \"' && ! curl -fsS ${API}/api/projects/${ID}/ports | jq -e '.ports[] | select(.port == 9222 or .port == 9229)' >/dev/null"
check "preview proxy refuses the browser port with a 400" \
    "[ \"\$(curl -s -o /dev/null -w '%{http_code}' ${API}/proxy/${ID}/9222/)\" = 400 ] && [ \"\$(curl -s -o /dev/null -w '%{http_code}' -X PUT -H 'Content-Type: application/json' -d '{\"emails\":[]}' ${API}/api/projects/${ID}/ports/9222/shares)\" = 400 ]"
# The server's start carries STYRMAN_BROWSER_PEER (its own bridge address),
# so this is also where the 9222 forwarder appears — and it too stays off
# the ports list.
check "browser status/start/stop routes answer for the owner (9222 up, hidden)" \
    "curl -fsS -X POST ${API}/api/projects/${ID}/browser/start | grep -q '\"running\":true' && docker exec ${CN} sh -c 'ss -ltnH | grep -q \":9222 \"' && ! curl -fsS ${API}/api/projects/${ID}/ports | jq -e '.ports[] | select(.port == 9222)' >/dev/null && curl -fsS ${API}/api/projects/${ID}/browser | grep -q '\"version\":\"Chrome/' && [ \"\$(curl -s -o /dev/null -w '%{http_code}' -X POST ${API}/api/projects/${ID}/browser/stop)\" = 204 ] && curl -fsS ${API}/api/projects/${ID}/browser | grep -q '\"running\":false'"
# `start` is what every relay connect runs, so it must be idempotent: a
# second server-run start finds the forwarder it started (by the listening
# socket's inode in the recorded pid's fds) and leaves it alone — the pid
# file does not change. A regression to "never up" respawns socat on every
# connect and drops the other panes (round-4 review, 2026-09-13).
check "a second start keeps the running forwarder (socat pid unchanged)" \
    "curl -fsS -X POST ${API}/api/projects/${ID}/browser/start >/dev/null && p1=\"\$(docker exec ${CN} cat ${GH}/.styrman/browser/socat.pid)\" && [ -n \"\$p1\" ] && curl -fsS -X POST ${API}/api/projects/${ID}/browser/start >/dev/null && [ \"\$(docker exec ${CN} cat ${GH}/.styrman/browser/socat.pid)\" = \"\$p1\" ] && [ \"\$(curl -s -o /dev/null -w '%{http_code}' -X POST ${API}/api/projects/${ID}/browser/stop)\" = 204 ]"
check "styrman-browser stop ends it (status reports not running)" \
    "docker exec ${CN} runuser -u ${ME} -- env HOME=${GH} /styrman/bin/styrman-browser stop && docker exec ${CN} runuser -u ${ME} -- env HOME=${GH} /styrman/bin/styrman-browser status | grep -q '\"running\":false'"

say "shell skel (v1.22.0: GOPATH is \$HOME/.go, seeded write-once per file)"
check "seeded .bash_exports carries the new GOPATH" \
    "docker exec ${CN} grep -qx 'export GOPATH=\"\$HOME/.go\"' ${GH}/.bash_exports"
# The value a shell actually resolves — proves the seed AND the sourcing chain
# (.profile -> .bashrc -> .bash_exports), not just the file's bytes.
# MUST be `-lic`, not `-lc`: .bashrc returns early for a non-interactive shell
# (`case $- in *i*) ;; *) return ;; esac`, skel/bashrc:7), so a plain `-lc`
# never reaches .bash_exports and reports an empty GOPATH. A styrman terminal
# is an interactive login shell under tmux, which `-lic` reproduces.
check "login shell resolves GOPATH to \$HOME/.go" \
    "[ \"\$(docker exec ${CN} runuser -u ${ME} -- bash -lic 'echo \$GOPATH' 2>/dev/null)\" = '${GH}/.go' ]"
# ~/.go/bin joins PATH only once it exists (the `[[ -d ]]` guard in bashrc:24),
# so create it FIRST and read PATH from a SECOND shell — doing both in one
# shell tests nothing, the guard has already run by then.
check "\$HOME/.go/bin joins PATH once it exists" \
    "docker exec ${CN} runuser -u ${ME} -- mkdir -p ${GH}/.go/bin && docker exec ${CN} runuser -u ${ME} -- bash -lic 'echo \$PATH' 2>/dev/null | tr : '\\n' | grep -qx '${GH}/.go/bin'"
say "in-project tool updates (ADR-077: claude into ~/.npm-global, agy in /opt/agy)"
# Claude's updater installs with npm into the prefix skel/bash_exports names
# (NPM_CONFIG_PREFIX=$HOME/.npm-global). The entrypoint creates that tree on
# every boot: absent, Claude reports the prefix unwritable and turns its
# updater off, and skel/bashrc leaves ~/.npm-global/bin off PATH.
check "~/.npm-global/bin and lib exist, user-writable, bin on a login shell's PATH" \
    "docker exec ${CN} runuser -u ${ME} -- sh -c 'test -w ${GH}/.npm-global/bin && test -w ${GH}/.npm-global/lib' && docker exec ${CN} runuser -u ${ME} -- bash -lic 'echo \$PATH' 2>/dev/null | tr : '\\n' | grep -qx '${GH}/.npm-global/bin'"
# What Claude itself says, from an INTERACTIVE login shell (see the GOPATH
# check above for why -lic). `Auto-updates:` is the positive control: a
# doctor that printed nothing must not read as "no warning".
DOCTOR="$(docker exec "${CN}" runuser -u "${ME}" -- env HOME="${GH}" bash -lic 'timeout 60 claude doctor </dev/null' 2>&1 || true)"
check "claude doctor reports auto-updates usable (no unwritable-prefix warning)" \
    "printf '%s\\n' \"\${DOCTOR}\" | grep -F 'Auto-updates:' >/dev/null && ! printf '%s\\n' \"\${DOCTOR}\" | grep -F 'auto-update: npm global folder' >/dev/null"
# agy replaces its own binary in the directory it really lives in (it
# resolves symlinks), and refuses when that directory is not writable.
check "project user can write agy's directory; /usr/local/bin/agy resolves into it" \
    "docker exec ${CN} runuser -u ${ME} -- sh -c 'd=\"\$(dirname \"\$(readlink -f /usr/local/bin/agy)\")\" && [ \"\$d\" = /opt/agy ] && test -w \"\$d\" && [ \"\$(readlink -f \"\$(command -v agy)\")\" = /opt/agy/agy ]'"
# ADR-080 (security review 2026-10-02): that directory is the project user's
# ALONE. The first cut shipped it 0777, and any other uid in the VM could
# then swap the binary the user runs next. `runuser -u nobody -- true` is the
# positive control: a runuser that cannot run at all must not read as "nobody
# cannot write". The last clause covers what is IN the directory too: nothing
# under /opt/agy may be group- or other-writable, the binary included
# (security review round 2: the directory's mode says nothing about a
# binary that could be overwritten in place).
check "agy's directory is owner-only: the project user's, mode 755, not writable by another uid" \
    "[ \"\$(docker exec ${CN} stat -c '%U %a' /opt/agy)\" = '${ME} 755' ] && docker exec ${CN} runuser -u nobody -- true && ! docker exec ${CN} runuser -u nobody -- test -w /opt/agy && docker exec ${CN} test -x /opt/agy/agy && [ -z \"\$(docker exec ${CN} find /opt/agy -perm /022)\" ]"

# R1 boot budget, from the container's own clock: the gap between its start
# and the entrypoint's ready line. The quick half of the harness seed (the
# migration and the two MCP registrations) sits on that path — a blocking
# call there once cost 60 s (ADR-047), and the v2 harness's file install
# cost 13 s there until ADR-078 moved it behind ready.
# Measured on the state dir's filesystem, named in the label (stat calls
# ext4 "ext2/ext3"); on tmpfs the check FAILS rather than passing a budget
# it cannot see (plan 2026-10-02, D12).
BOOT_AT="$(docker inspect -f '{{.State.StartedAt}}' "${CN}")"
READY_AT="$(docker logs -t "${CN}" 2>&1 | grep -m1 'project .* ready' | cut -d' ' -f1 || true)"
BOOT_GAP="$(awk "BEGIN{printf \"%.2f\", $(date -d "${READY_AT:-1970-01-01T00:00:00Z}" +%s.%N) - $(date -d "${BOOT_AT}" +%s.%N)}")"
if [ "${STATE_FS}" = tmpfs ]; then
    bad "boot to ready within R1's 15 s hard limit (state dir on tmpfs: the boot budget cannot be measured)"
else
    check "boot to ready within R1's 15 s hard limit (measured ${BOOT_GAP}s on ${STATE_FS}; soft target 5 s)" \
        "awk 'BEGIN{exit !(${BOOT_GAP} > 0 && ${BOOT_GAP} < 15)}'"
fi
# The harness's file install (1,788 files, synced one by one) runs AFTER
# ready (ADR-078): the ready line must precede the deployed line. docker's
# -t stamps are fixed-width RFC 3339, so they compare as strings; both are
# length-checked so a missing line cannot compare as "earlier".
DEPLOYED_AT="$(docker logs -t "${CN}" 2>&1 | grep -m1 -F 'agent harness deployed (version' | cut -d' ' -f1 || true)"
check "ready line precedes the agent harness deployed line (install runs after ready)" \
    "[ -n '${READY_AT}' ] && [ -n '${DEPLOYED_AT}' ] && [[ '${READY_AT}' < '${DEPLOYED_AT}' ]]"

say "terminals (durable records, ADR-016)"
NEW=$(curl -fsS -X POST "${API}/api/projects/${ID}/terminals" -d '{"title":"First terminal"}')
check "record created with title" "echo '${NEW}' | grep -q '\"title\":\"First terminal\"'"
curl -fsS -X PATCH "${API}/api/projects/${ID}/terminals/1" -d '{"title":"Renamed"}' -o /dev/null
check "rename persisted" \
    "curl -fsS ${API}/api/projects/${ID}/terminals | grep -q '\"title\":\"Renamed\"'"
check "attach to unknown terminal is refused" \
    "[ \$(curl -s -o /dev/null -w '%{http_code}' ${API}/ws/term/${ID}/99) = 404 ]"

say "terminal I/O over WebSocket"
MARK="itest-$$-mark"
OUT=$("${STATE}/wsprobe" -url "ws://127.0.0.1:${PORT}/ws/term/${ID}/1?cols=120&rows=30" \
    -send "echo ${MARK}; pwd\r" -wait 5s || true)
check "shell echoes marker" "echo '${OUT}' | grep -q '${MARK}'"
# Liveness (ADR-079): a ping control frame is answered with a pong text
# frame, which wsprobe prints as [text:…].
PONG=$("${STATE}/wsprobe" -url "ws://127.0.0.1:${PORT}/ws/term/${ID}/1?cols=120&rows=30" \
    -control '{"type":"ping"}' -wait 3s || true)
check "a ping text frame is answered with a pong" \
    "printf '%s' \"\${PONG}\" | grep -F '[text:{\"type\":\"pong\"}]' >/dev/null"
check "cwd is the workspace" "echo '${OUT}' | grep -q 'projects/itest'"
check "terminal reported live" \
    "curl -fsS ${API}/api/projects/${ID}/terminals | grep -q '\"live\":true'"

say "tmux session survives disconnect"
"${STATE}/wsprobe" -url "ws://127.0.0.1:${PORT}/ws/term/${ID}/1" \
    -send "STYRMAN_TEST_VAR=${MARK}\r" -wait 2s >/dev/null || true
OUT=$("${STATE}/wsprobe" -url "ws://127.0.0.1:${PORT}/ws/term/${ID}/1" \
    -send "echo VAR=\$STYRMAN_TEST_VAR\r" -wait 4s || true)
check "reattach sees same shell state" "echo '${OUT}' | grep -q 'VAR=${MARK}'"

say "transcripts (history)"
sleep 1
check "transcript listed" "curl -fsS ${API}/api/projects/${ID}/history | grep -q '\"id\":\"conv-1\"'"
check "transcript captured the command" \
    "curl -fsS ${API}/api/projects/${ID}/history/conv-1 | grep -q '${MARK}'"
check "invalid transcript id refused" \
    "[ \$(curl -s -o /dev/null -w '%{http_code}' '${API}/api/projects/${ID}/history/..%2Fetc') != 200 ]"

say "file explorer"
# A real entry, not just the key: an empty "entries":[] passed this check
# while every project listing was blank (release e2e 2026-09-04, BUG-004).
check "project listing works (shows .git)" "curl -fsS '${API}/api/projects/${ID}/files' | grep -q '\"name\":\".git\"'"
check "home listing works (per-project, shows .styrman)" "curl -fsS '${API}/api/projects/${ID}/home/files' | grep -q '\"name\":\".styrman\"'"
check "path traversal refused" \
    "[ \$(curl -s -o /dev/null -w '%{http_code}' '${API}/api/projects/${ID}/files?path=../../etc') = 400 ]"
curl -fsS -X PUT "${API}/api/projects/${ID}/file?path=note.txt" \
    -d '{"content":"written by the explorer"}' -o /dev/null
check "file written and read back" \
    "curl -fsS '${API}/api/projects/${ID}/file?path=note.txt' | grep -q 'written by the explorer'"
# ADR-027: workspaces live in per-project storage, not the home
check "file visible on the host" "grep -q explorer '${STATE}/projects/${ME}/${ID}/note.txt'"
check "download serves the file" \
    "curl -fsS '${API}/api/projects/${ID}/download?path=note.txt' | grep -q explorer"

say "ports + preview proxy"
docker exec -d -u "$(id -un)" "${CN}" sh -c 'cd /tmp && echo "<h1>preview</h1>" > i.html && python3 -m http.server 4100 >/dev/null 2>&1'
sleep 2
check "port detected" "curl -fsS ${API}/api/projects/${ID}/ports | grep -q 4100"
# The preview proxy needs the TEST server (a host process) to reach the VM's
# bridge IP. On a hardened host the v1.9.0 firewall DROPs host→bridge traffic
# (production is unaffected — its containerized server sits on each bridge),
# so probe reachability once and SKIP the proxy-path checks when blocked.
VMIP=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "${CN}" 2>/dev/null || true)
HOSTFW=""
if [ -z "${VMIP}" ]; then
    bad "firewall probe: could not resolve the VM IP (not a firewall skip)"
else
    # NOT a bare statement: under set -e a failing curl outside a condition
    # aborts the whole suite (it did — the probe's exit 28 killed the run).
    RC=0
    curl -m 3 -so /dev/null "http://${VMIP}:4100/i.html" || RC=$?
    case "${RC}" in
        0) HOSTFW="" ;;
        28) HOSTFW=1 ;; # timeout = the v1.9.0 DROP — the only firewall signature
        *) bad "firewall probe: curl exit ${RC} (in-guest server dead? NOT the firewall)" ;;
    esac
fi
if [ -n "${HOSTFW}" ]; then
    skip "pretty URL proxies to the app (host firewall blocks host→bridge; prod unaffected)"
    skip "legacy proxy URL still works (host firewall)"
else
    check "pretty URL proxies to the app" \
        "curl -fsS ${API}/p/${ID}/1/port/4100/i.html | grep -q preview"
    check "legacy proxy URL still works" \
        "curl -fsS ${API}/proxy/${ID}/4100/i.html | grep -q preview"
fi
check "bare URL redirects to trailing slash" \
    "[ \$(curl -s -o /dev/null -w '%{http_code}' ${API}/p/${ID}/1/port/4100) = 301 ]"

say "public TTL preview links (1.1 foundation)"
LINK_TOK=$(curl -fsS -X POST "${API}/api/projects/${ID}/ports/4100/links" -d '{"ttl":"1h"}' | python3 -c 'import sys,json;print(json.load(sys.stdin)["token"])' 2>/dev/null || true)
check "link minted" "[ -n \"${LINK_TOK}\" ]"
if [ -n "${HOSTFW}" ]; then
    skip "public link serves the app (host firewall blocks host→bridge; prod unaffected)"
else
    check "public link serves the app (no auth)" \
        "curl -fsS ${API}/link/${LINK_TOK}/i.html | grep -q preview"
fi
check "link listed for the project" \
    "curl -fsS ${API}/api/projects/${ID}/links | grep -q ${LINK_TOK}"
check "bad token 404s" \
    "[ \$(curl -s -o /dev/null -w '%{http_code}' ${API}/link/deadbeef/) = 404 ]"
check "revoke deletes the link" \
    "[ \$(curl -s -o /dev/null -w '%{http_code}' -X DELETE ${API}/api/projects/${ID}/links/${LINK_TOK}) = 204 ]"
check "revoked link no longer serves" \
    "[ \$(curl -s -o /dev/null -w '%{http_code}' ${API}/link/${LINK_TOK}/i.html) = 404 ]"

# Per-project network mode (ADR-067). The mode IS the bridge interface name
# the server picks at `docker network create`, so everything below is checked
# against Docker itself rather than against a root status file: there is no
# root half to stub, and no scratch firewall dir to point anywhere.
say "network mode (ADR-067)"
NET="sdb-${ID}"
bridge_of() { docker network inspect "$1" --format '{{index .Options "com.docker.network.bridge.name"}}' 2>/dev/null; }

check "a new project defaults to sandbox" \
    "[ \$(curl -s ${API}/api/projects/${ID}/network | jq -r .lanAccess) = false ]"
check "its bridge carries the sdb- prefix (matched by the firewall drops)" \
    "bridge_of ${NET} | grep -q '^sdb-'"
check "the running container is on that bridge" \
    "[ -e /sys/class/net/\$(bridge_of ${NET}) ]"
check "admin may enable" \
    "[ \$(curl -s -o /dev/null -w '%{http_code}' -X PUT -H 'content-type: application/json' -d '{\"lanAccess\":true}' ${API}/api/projects/${ID}/network) = 200 ]"
check "the record now says LAN" \
    "[ \$(curl -s ${API}/api/projects/${ID}/network | jq -r .lanAccess) = true ]"
# The PUT rebuilds the project synchronously, so by the time it returns the
# bridge must already have the other prefix. This is the whole feature.
check "the bridge was rebuilt with the lan- prefix" \
    "bridge_of ${NET} | grep -q '^lan-'"
# config.BridgeNameFor hashes the NETWORK NAME alone and prepends the prefix
# afterwards; the first draft of this check hashed "sdb-${NET}" and so compared
# against a name the server never computes.
check "the suffix is unchanged (same project, same identity in ip link)" \
    "[ \"\$(bridge_of ${NET} | cut -c5-)\" = \"\$(printf '%s' ${NET} | sha1sum | cut -c1-11)\" ]"
# DOCKER's OWN rules name every bridge in full — DOCKER, DOCKER-BRIDGE,
# DOCKER-CT, DOCKER-FORWARD — and one of them
# (`-A DOCKER ! -i <br> -o <br> -j DROP`) IS the cross-project isolation this
# design leans on. So "no rule names it" is false by construction and the
# first draft of this check asserted it anyway. The real claim is narrower and
# is the whole feature: no rule that jumps into a STYRMAN-* chain matches a
# lan- interface. Styrman's jumps key on the `sdb-+` wildcard; a lan- bridge is
# outside it.
if sudo -n iptables -S >/dev/null 2>&1; then
    check "no styrman jump matches the lan- bridge" \
        "! sudo -n iptables -S | grep STYRMAN | grep -q -- '-i lan-'"
    check "docker still isolates the lan- bridge from other bridges" \
        "sudo -n iptables -S DOCKER | grep -q -- \"! -i \$(bridge_of ${NET}) -o \$(bridge_of ${NET}) -j DROP\""
else
    skip "styrman-jump and docker-isolation rule checks (iptables not readable via sudo -n)"
    skip "docker isolation rule check"
fi
check "the project still runs after the rebuild" \
    "[ \$(curl -s ${API}/api/projects/${ID} | jq -r .state) = running ]"
check "its home survived the rebuild" \
    "docker exec ${CN} test -d /home/\$(curl -s ${API}/api/settings | jq -r .terminalUsername)/projects"

check "disable returns 200" \
    "[ \$(curl -s -o /dev/null -w '%{http_code}' -X PUT -H 'content-type: application/json' -d '{\"lanAccess\":false}' ${API}/api/projects/${ID}/network) = 200 ]"
check "the bridge is back to sdb-" "bridge_of ${NET} | grep -q '^sdb-'"
check "a no-op PUT reports rebuilt=false" \
    "[ \$(curl -s -X PUT -H 'content-type: application/json' -d '{\"lanAccess\":false}' ${API}/api/projects/${ID}/network | jq -r .rebuilt) = false ]"
check "malformed body is 400" \
    "[ \$(curl -s -o /dev/null -w '%{http_code}' -X PUT -H 'content-type: application/json' -d '{}' ${API}/api/projects/${ID}/network) = 400 ]"
check "unknown project is 404" \
    "[ \$(curl -s -o /dev/null -w '%{http_code}' ${API}/api/projects/00000000-0000-4000-8000-000000000000/network) = 404 ]"

# Per-project resources (ADR-083). Kata adds a container's limits to its own
# base VM (1 vCPU, 2 GiB) and caps the workload at exactly the limit through
# the guest cgroup, so the guest shows `nproc` = setting + 1 while cpu.max
# reads the setting. "Unlimited" is the whole host as `docker info` reports
# it. Every resize rebuilds the container, so this section sits after the
# network-mode section and restores unlimited at its end.
say "resources (ADR-083)"
RES="${API}/api/projects/${ID}/resources"
HOSTCPU="$(docker info -f '{{.NCPU}}')"
HOSTMEM_KB="$(( $(docker info -f '{{.MemTotal}}') / 1024 ))"
put_res() { curl -s -X PUT -H 'content-type: application/json' -d "$1" "${RES}"; }
put_code() { curl -s -o /dev/null -w '%{http_code}' -X PUT -H 'content-type: application/json' -d "$1" "${RES}"; }
guest() { docker exec "${CN}" sh -c "$1" 2>/dev/null; }
disk_bytes() { guest 'df -B1 --output=size /var/lib/docker-project | tail -1' | tr -d ' '; }

check "a new project records the operator's Docker disk default (20)" \
    "[ \"\$(jq -r --arg id ${ID} '.projects[] | select(.id == \$id) | .diskGB' '${STATE}/users/${ME}/projects.json')\" = 20 ]"
check "GET reports unlimited, disk 20, nothing pending, the host's limits" \
    "curl -fsS ${RES} | jq -e --argjson c ${HOSTCPU} '.cpus == 0 and .memoryGiB == 0 and .diskGB == 20 and .pending == false and .limits.maxCpus == \$c and .limits.minDiskGB == 20 and .limits.maxDiskGB == 500' >/dev/null"
check "unlimited: the guest sees every host CPU" \
    "[ \"\$(guest nproc)\" = ${HOSTCPU} ]"
check "unlimited: the guest's MemTotal is at least 97% of the host's" \
    "[ \"\$(guest \"awk '/MemTotal/{print \\\$2}' /proc/meminfo\")\" -ge \$(( HOSTMEM_KB * 97 / 100 )) ]"
DISK0="$(disk_bytes)"
check "PUT 2 CPUs / 4 GiB / 21 GB on a running project rebuilds it" \
    "[ \"\$(put_res '{\"cpus\":2,\"memoryGiB\":4,\"diskGB\":21}' | jq -r .rebuilt)\" = true ]"
check "the guest cgroup caps the workload at 2 CPUs" \
    "[ \"\$(guest 'cat /sys/fs/cgroup/cpu.max')\" = '200000 100000' ]"
check "the guest cgroup caps memory at 4 GiB" \
    "[ \"\$(guest 'cat /sys/fs/cgroup/memory.max')\" = 4294967296 ]"
check "the guest shows 3 CPUs (the setting plus Kata's own)" \
    "[ \"\$(guest nproc)\" = 3 ]"
check "the Docker disk grew by 1 GiB at boot (within 2%)" \
    "d=\$(( \$(disk_bytes) - DISK0 )); [ \"\$d\" -ge \$(( 1073741824 * 98 / 100 )) ] && [ \"\$d\" -le \$(( 1073741824 * 102 / 100 )) ]"
check "the Docker disk cannot shrink (20 after 21 is 400)" \
    "[ \"\$(put_code '{\"cpus\":2,\"memoryGiB\":4,\"diskGB\":20}')\" = 400 ]"
check "more CPUs than the host is 400" \
    "[ \"\$(put_code \"{\\\"cpus\\\":\$((HOSTCPU + 1)),\\\"memoryGiB\\\":0,\\\"diskGB\\\":21}\")\" = 400 ]"
check "a missing field is 400" \
    "[ \"\$(put_code '{\"cpus\":2}')\" = 400 ]"

curl -fsS -X POST "${API}/api/projects/${ID}/stop" >/dev/null
check "PUT on a stopped project only records (rebuilt=false)" \
    "[ \"\$(put_res '{\"cpus\":3,\"memoryGiB\":4,\"diskGB\":21}' | jq -r .rebuilt)\" = false ]"
curl -fsS -X POST "${API}/api/projects/${ID}/start" >/dev/null
check "its next start runs at the new size (cpu.max 3)" \
    "[ \"\$(guest 'cat /sys/fs/cgroup/cpu.max')\" = '300000 100000' ]"
CID1="$(docker inspect -f '{{.Id}}' "${CN}")"
curl -fsS -X POST "${API}/api/projects/${ID}/stop" >/dev/null
curl -fsS -X POST "${API}/api/projects/${ID}/start" >/dev/null
check "an unchanged project is not recreated across stop/start (same container)" \
    "[ \"\$(docker inspect -f '{{.Id}}' ${CN})\" = \"${CID1}\" ]"
check "stop/start keeps HostConfig at 3 CPUs / 4 GiB" \
    "[ \"\$(docker inspect -f '{{.HostConfig.NanoCpus}} {{.HostConfig.Memory}}' ${CN})\" = '3000000000 4294967296' ]"
# The attach path's restart-policy update on a LIVE sandbox carries all-zero
# Resources (security review 2026-09-16, low): flip the policy the way a
# half-failed stop leaves it, then a start call re-asserts it on the running
# VM — the guest's limits must be untouched.
docker update --restart no "${CN}" >/dev/null
curl -fsS -X POST "${API}/api/projects/${ID}/start" >/dev/null
check "a live restart-policy update leaves the size alone (policy back on, cgroup and HostConfig unchanged)" \
    "[ \"\$(docker inspect -f '{{.HostConfig.RestartPolicy.Name}} {{.HostConfig.NanoCpus}} {{.HostConfig.Memory}}' ${CN})\" = 'unless-stopped 3000000000 4294967296' ] && [ \"\$(guest 'cat /sys/fs/cgroup/cpu.max')\" = '300000 100000' ] && [ \"\$(guest 'cat /sys/fs/cgroup/memory.max')\" = 4294967296 ]"
check "back to unlimited rebuilds the running project" \
    "[ \"\$(put_res '{\"cpus\":0,\"memoryGiB\":0,\"diskGB\":21}' | jq -r .rebuilt)\" = true ]"
check "unlimited again: every host CPU" \
    "[ \"\$(guest nproc)\" = ${HOSTCPU} ]"
check "the record keeps the grown disk (21)" \
    "[ \"\$(curl -fsS ${RES} | jq -r .diskGB)\" = 21 ]"
# The grow's RETRY branch (review 2026-10-05, finding 1): a boot whose
# truncate succeeded but whose resize2fs did not leaves the FILE at the new
# size and the filesystem at the old one. Plant exactly that on the stopped
# project's image from the host, raise the record to match the file, and the
# next boot must still grow the filesystem — a gate on the file's size would
# skip it.
IMGF="${STATE}/homes/${ME}/p/${ID}/.dockerd/${ID}.img"
curl -fsS -X POST "${API}/api/projects/${ID}/stop" >/dev/null
# The image lives in the guest-writable home: truncate only a regular file
# that is not a link (realpath equal to its path), only while the VM is
# stopped, and never create one (-c) (security addendum 2026-10-05, low 1).
check "retry setup: the image file is 22 GiB, its filesystem still 21" \
    "! docker ps --format '{{.Names}}' | grep -qx ${CN} && [ -f '${IMGF}' ] && [ ! -L '${IMGF}' ] && [ \"\$(realpath '${IMGF}')\" = '${IMGF}' ] && truncate -c -s 22G '${IMGF}' && [ \"\$(stat -c %s '${IMGF}')\" = 23622320128 ]"
check "a PUT of 22 GB on the stopped project records it" \
    "[ \"\$(put_res '{\"cpus\":0,\"memoryGiB\":0,\"diskGB\":22}' | jq -r .diskGB)\" = 22 ]"
curl -fsS -X POST "${API}/api/projects/${ID}/start" >/dev/null
check "the next boot grows the filesystem to the file (2 GiB over the start, within 2%)" \
    "d=\$(( \$(disk_bytes) - DISK0 )); [ \"\$d\" -ge \$(( 2147483648 * 98 / 100 )) ] && [ \"\$d\" -le \$(( 2147483648 * 102 / 100 )) ]"


# Git side panel is a real in-guest client now (the host-side /diff route and the
# checkpoint machinery were removed as a guest→server RCE vector — ADR-039).
say "git (in-guest client)"
check "git status reports the running repo" \
    "curl -fsS ${API}/api/projects/${ID}/git/status | grep -q '\"repo\":true'"
check "git status sees the new file (untracked)" \
    "curl -fsS ${API}/api/projects/${ID}/git/status | grep -q 'note.txt'"
check "git diff of the untracked file shows its content" \
    "curl -fsS \"${API}/api/projects/${ID}/git/diff?path=note.txt&untracked=true\" | grep -q 'note'"

say "settings"
check "settings served" "curl -fsS ${API}/api/settings | grep -q '\"authMode\"'"

# Updater status (ADR-081): the server reads ONE file, named by
# STYRMAN_UPDATE_STATUS_FILE (set on the server above), and shows an admin
# what is in it — this server runs with auth off, where the caller is one.
# The plant's times are NOW: a literal would be more than an hour old and
# read "stale", not "failing".
US_FILE="${STATE}/status/updater.json"
check "settings: no updater status file, no updater key" \
    "curl -fsS ${API}/api/settings | jq -e 'has(\"updater\") | not' >/dev/null"
mkdir -p "${STATE}/status"
US_NOW="$(date -u +%FT%TZ)"
printf '{"result":"failing","code":"fetch-auth","checkedAt":"%s","since":"%s","deployed":"v1.38.0","latest":""}\n' "${US_NOW}" "${US_NOW}" > "${US_FILE}"
check "settings: a failing updater status reaches an admin, code and all" \
    "curl -fsS ${API}/api/settings | jq -e '.updater.state == \"failing\" and .updater.code == \"fetch-auth\" and .updater.deployed == \"v1.38.0\"' >/dev/null"
printf 'not json\n' > "${US_FILE}"
check "settings: an updater status that cannot be read is \"unknown\", never ok" \
    "curl -fsS ${API}/api/settings | jq -e '.updater.state == \"unknown\" and (.updater | has(\"code\") | not)' >/dev/null"
rm -f "${US_FILE}"

say "persistence across container destruction (R3/R5)"
"${STATE}/wsprobe" -url "ws://127.0.0.1:${PORT}/ws/term/${ID}/1" \
    -send "echo persisted > ~/projects/itest/proof.txt\r" -wait 2s >/dev/null || true
sleep 1
# BUG-062: a settings.json the installer cannot judge — here a directory in
# its place — is reported on the installer's stdout with a `note: ` prefix,
# and the entrypoint must carry that line into the boot log (the key is not
# written then, and only this line says so). Removing the stamp makes the
# seed run again on the recreate below (the home persists, nothing else
# changes); the plant is undone after the check so the stop/start section
# proves the file comes back with the key. Checked, not bare (see the LAN
# section): a failed setup must read as one FAIL, not abort the suite.
check "BUG-062 setup: stamp removed, a directory planted in settings.json's place" \
    "docker exec ${CN} runuser -u ${ME} -- sh -c 'rm -f ${GH}/.styrman/.harness-version ${GH}/.claude/settings.json && mkdir ${GH}/.claude/settings.json'"
# Bounded retry: a force-remove that lands while the Kata VM is exiting is
# refused "PID … is zombie" until the shim reaps it — the same window the
# orchestrator's forceRemove waits out (2026-10-05); a bare rm aborted the run.
for _ in $(seq 1 30); do docker rm -f "${CN}" >/dev/null 2>&1 && break; sleep 2; done
# Without this the recreate checks below could pass on a container that was
# never removed, had the retries run out (review of f507278, low).
check "the container was removed before the recreate (zombie window waited out)" \
    "! docker ps -a --format '{{.Names}}' | grep -qx ${CN}"
curl -fsS -X POST "${API}/api/projects/${ID}/start" >/dev/null
OUT=$("${STATE}/wsprobe" -url "ws://127.0.0.1:${PORT}/ws/term/${ID}/1" \
    -send "cat ~/projects/itest/proof.txt\r" -wait 4s || true)
check "file survives container recreate" "echo '${OUT}' | grep -q persisted"
# ADR-031: the docker disk lives in the PER-PROJECT home now
check "docker.img persisted" "[ -f '${STATE}/homes/${ME}/p/${ID}/.dockerd/${ID}.img' ]"
check "transcript survives recreate" \
    "curl -fsS ${API}/api/projects/${ID}/history/conv-1 | grep -q '${MARK}'"
# The recreate above re-seeds (the stamp was removed): its install is in the
# background, so wait before reading what it logs and stamps — and before the
# undo below removes the stamp a still-running install would write back.
wait_seed
check "boot log carries the installer's note: line for a settings.json it could not judge (BUG-062)" \
    "docker logs ${CN} 2>&1 | grep -F \"note: ${GH}/.claude/settings.json is not a regular file, so the attribution setting could not be checked\" >/dev/null"
check "the seed still completed around the refused key (stamp rewritten)" \
    "docker exec ${CN} runuser -u ${ME} -- sh -c 'test \"\$(cat ${GH}/.styrman/.harness-version)\" = \"${HID}\"'"
check "BUG-062 undo: plant removed, stamp removed for one more seed" \
    "docker exec ${CN} runuser -u ${ME} -- sh -c 'rmdir ${GH}/.claude/settings.json && rm -f ${GH}/.styrman/.harness-version'"

say "terminal pause + close-all"
curl -fsS -X POST "${API}/api/projects/${ID}/terminals" -d '{"title":"second"}' -o /dev/null
"${STATE}/wsprobe" -url "ws://127.0.0.1:${PORT}/ws/term/${ID}/2" -send "echo hi\r" -wait 2s >/dev/null || true
curl -fsS -X POST "${API}/api/projects/${ID}/terminals/2/pause" -o /dev/null -w ''
check "paused terminal keeps record, loses session" \
    "curl -fsS ${API}/api/projects/${ID}/terminals | grep -q '\"id\":\"2\"' && ! curl -fsS ${API}/api/projects/${ID}/terminals | grep -q '\"id\":\"2\",[^}]*\"live\":true'"
curl -fsS -X DELETE "${API}/api/projects/${ID}/terminals" -o /dev/null
check "close-all removed every terminal" \
    "[ \$(curl -fsS ${API}/api/projects/${ID}/terminals | grep -o '\"id\"' | wc -l) = 0 ]"
curl -fsS -X POST "${API}/api/projects/${ID}/terminals" -d '{}' -o /dev/null

say "multi-tenancy + uuid identity"
check "legacy home migrated to per-user layout" "[ -d '${STATE}/homes/${ME}' ] && [ ! -d '${STATE}/home' ]"
check "container named styrman-<uuid>" "docker ps --format '{{.Names}}' | grep -q \"^${CN}\$\""
check "per-user metadata store" "[ -f '${STATE}/users/${ME}/projects.json' ]"
curl -fsS -X PATCH "${API}/api/projects/${ID}" -d '{"name":"itest-renamed"}' -o /dev/null
check "friendly rename sticks, dir unchanged" \
    "curl -fsS ${API}/api/projects/${ID} | grep -q '\"name\":\"itest-renamed\"' && curl -fsS ${API}/api/projects/${ID} | grep -q '\"dir\":\"itest\"'"
NEW2=$(curl -s -X POST "${API}/api/projects" -d '{"name":"itest-renamed","gitInit":false}' -o /dev/null -w '%{http_code}')
check "live duplicate name refused" "[ \"${NEW2}\" = 409 ]"

say "stop + delete (wipe)"
curl -fsS -X POST "${API}/api/projects/${ID}/stop" >/dev/null
check "container stopped" "! docker ps --format '{{.Names}}' | grep -q ${CN}"
# BUG-063: Stop switches the restart policy off before the guest's own exit,
# so the daemon does not restart the clean exit — the container ends exited
# 0 with no restart on record, and its policy reads "no" while stopped. The
# old ordering left RestartCount 1 (a second boot) and, on the live host, an
# exit 255 after the real stop hard-killed the rebooted VM. Exit code and
# RestartCount are the BEHAVIOUR (the daemon did not restart); the policy
# name is the persisted HostConfig, what a host reboot reads — the check
# after the boot=1 start below asserts that persisted value only.
check "stop left the container exited 0, no restart, policy off (BUG-063)" \
    "[ \"\$(docker inspect -f '{{.State.ExitCode}} {{.RestartCount}} {{.HostConfig.RestartPolicy.Name}}' ${CN})\" = '0 0 no' ]"
check "terminals still listed while stopped" \
    "curl -fsS ${API}/api/projects/${ID}/terminals | grep -q '\"title\"'"
# Sticky stop (v1.11.0): an automatic reconnect (no boot=1) must be answered
# with close 4002 and must NOT boot the VM back; boot=1 (user intent) boots.
TID=$(curl -fsS "${API}/api/projects/${ID}/terminals" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
OUT=$("${STATE}/wsprobe" -url "ws://127.0.0.1:${PORT}/ws/term/${ID}/${TID}" -wait 3s || true)
check "stopped project refuses auto-reconnect (close 4002)" "echo '${OUT}' | grep -q 'close:4002'"
check "VM stayed stopped after refused reconnect" "! docker ps --format '{{.Names}}' | grep -q ${CN}"
OUT=$("${STATE}/wsprobe" -url "ws://127.0.0.1:${PORT}/ws/term/${ID}/${TID}?cols=120&rows=30&boot=1" \
    -send "echo revived\r" -wait 8s || true)
check "boot=1 reconnect boots the stopped VM" "echo '${OUT}' | grep -q revived"
check "VM running again after boot=1" "docker ps --format '{{.Names}}' | grep -q ${CN}"
check "start switched the restart policy back on (BUG-063)" \
    "[ \"\$(docker inspect -f '{{.HostConfig.RestartPolicy.Name}}' ${CN})\" = unless-stopped ]"
# The BUG-062 undo above left the home without a stamp or a settings.json:
# this boot seeds again and the file comes back with the key (ADR-072).
# The install is in the background (ADR-078): wait for it, both for this
# check and so the stop below does not cut it short.
wait_seed
check "re-seed after the BUG-062 undo recreates settings.json with the key" \
    "docker logs ${CN} 2>&1 | grep -F \"settings: ${GH}/.claude/settings.json created with\" >/dev/null && docker exec ${CN} runuser -u ${ME} -- jq -e '.attribution.commit == \"\" and .attribution.pr == \"\"' ${GH}/.claude/settings.json >/dev/null"
# ADR-074 never-downgrade (plan D4): a home whose manifest names a NEWER
# release than the image carries — what `sh-agent update` inside the project
# leaves behind — is left alone by the next boot: no install, the binary and
# manifest untouched, the stamp rewritten so it is not retried every boot.
# Planted on the HOST as the owner while the VM is stopped (the shape the
# Phase 1 smoke proved against the real binary's output), then restored.
HOSTHOME="${STATE}/homes/${ME}/p/${ID}"
curl -fsS -X POST "${API}/api/projects/${ID}/stop" >/dev/null
check "stopped again for the never-downgrade case" "! docker ps --format '{{.Names}}' | grep -q ${CN}"
SHA_BEFORE="$(sha256sum "${HOSTHOME}/.local/bin/sh-agent" | cut -d' ' -f1)"
DEPLOYED_BEFORE="$(docker logs "${CN}" 2>&1 | grep -cF 'agent harness deployed (version' || true)"
sed -i "s/\"repo\": \"embedded:${HREL}\"/\"repo\": \"embedded:v99.0.0\"/" "${HOSTHOME}/.config/sh-agent/state.json"
rm -f "${HOSTHOME}/.styrman/.harness-version"
check "manifest planted newer than the image (v99.0.0), stamp cleared" \
    "grep -q '\"repo\": \"embedded:v99.0.0\"' '${HOSTHOME}/.config/sh-agent/state.json' && [ ! -e '${HOSTHOME}/.styrman/.harness-version' ]"
OUT=$("${STATE}/wsprobe" -url "ws://127.0.0.1:${PORT}/ws/term/${ID}/${TID}?cols=120&rows=30&boot=1" \
    -send "echo revived2\r" -wait 8s || true)
check "boot=1 boots the VM for the never-downgrade case" "echo '${OUT}' | grep -q revived2"
check "boot log says the newer home was left alone (no install, ADR-074)" \
    "docker logs ${CN} 2>&1 | grep -F 'agent harness: home has sh-agent v99.0.0 (sh-agent update), image carries ${HREL}; left alone' >/dev/null"
# The pass line of a skipped install says "kept", never "deployed": the
# count of deployed lines is what it was before this boot (code review
# 2026-09-21, finding 3). The pass line and the stamp come from the
# background step (ADR-078), so wait for it.
wait_seed
check "the left-alone boot logs 'kept', not a new 'deployed' line" \
    "docker logs ${CN} 2>&1 | grep -F 'agent harness kept (version' >/dev/null && [ \"\$(docker logs ${CN} 2>&1 | grep -cF 'agent harness deployed (version')\" = '${DEPLOYED_BEFORE}' ]"
check "home sh-agent and manifest untouched by the boot" \
    "[ \"\$(sha256sum '${HOSTHOME}/.local/bin/sh-agent' | cut -d' ' -f1)\" = '${SHA_BEFORE}' ] && grep -q '\"repo\": \"embedded:v99.0.0\"' '${HOSTHOME}/.config/sh-agent/state.json'"
check "stamp rewritten after the left-alone boot (not retried every boot)" \
    "[ \"\$(cat '${HOSTHOME}/.styrman/.harness-version')\" = '${HID}' ]"
sed -i "s/\"repo\": \"embedded:v99.0.0\"/\"repo\": \"embedded:${HREL}\"/" "${HOSTHOME}/.config/sh-agent/state.json"
# Re-enable LAN mode right before the delete so the destroy path is
# exercised on a LAN project, not only a sandboxed one.
# Checked, not bare: under `set -euo pipefail` a 503 here aborted the whole
# suite and five later checks were never run at all — worse than one FAIL,
# because a lost check looks like a check that passed.
check "re-enable accepted before the delete" \
    "curl -fsS -X PUT '${API}/api/projects/${ID}/network' -d '{\"lanAccess\":true}' -o /dev/null"
check "the record says LAN before the delete" \
    "[ \$(curl -s ${API}/api/projects/${ID}/network | jq -r .lanAccess) = true ]"
curl -fsS -X DELETE "${API}/api/projects/${ID}?wipe=true" -o /dev/null
# Delete is ASYNC since v1.3 (202 + background: destroy container, THEN wipe,
# THEN remove the record). Wait for the LAST step — the record leaving the
# API list — so the wipe + record-removal checks below don't race the tail.
for _ in $(seq 1 60); do
    curl -fsS "${API}/api/projects" | grep -q "${ID}" || break
    sleep 2
done
# ADR-067 leaves nothing beside the record to leak: no marker, no rule, and
# the bridge goes with the network Destroy removes.
check "delete left no netpolicy tree behind" "[ ! -e '${STATE}/netpolicy' ]"
check "container gone (async delete)" "! docker ps -a --format '{{.Names}}' | grep -q ${CN}"
check "workspace wiped" "[ ! -d '${STATE}/projects/${ME}/${ID}' ]"
check "project gone from API" "! curl -fsS ${API}/api/projects | grep -q ${ID}"
ABORTED=0
