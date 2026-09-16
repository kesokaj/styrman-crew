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
STATE="$(mktemp -d /tmp/styrman-itest-XXXX)"
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

say "harness channel (ADR-073: a scratch channel, signed and delivered the production way)"
# make-scratch-harness.sh runs `styrman-update harness` in test mode against
# ${STATE}/harness-channel: a signed v1.0.0 on a scratch origin built from
# image/harness/, exported, smoked in ${IMAGE}, swapped in as content/current.
# The server below mounts it into the project (STYRMAN_HARNESS_DIR); the
# harness section asserts the guest sees the channel, not the image's copy,
# and the stop+delete section swaps it to v1.0.1 while the VM is stopped.
HARNESS_OUT="${STATE}/harness-channel"
HARNESS_CUR="$(bash "${REPO}/.agents/docs/testing/styrman/make-scratch-harness.sh" "${HARNESS_OUT}" "${IMAGE}" 2>"${STATE}/harness-channel.log" | tail -1 || true)"
check "scratch channel delivered v1.0.0 (make-scratch-harness.sh; log: ${STATE}/harness-channel.log)" \
    "[ -L '${HARNESS_CUR}' ] && [ \"\$(cat '${HARNESS_CUR}/repo/TAG')\" = v1.0.0 ]"
HARNESS_C1="$(cat "${HARNESS_CUR}/repo/COMMIT" 2>/dev/null || true)"

say "start server (state=${STATE}, port=${PORT})"
mkdir -p "${STATE}/home/projects"   # legacy layout: exercises the migration
STYRMAN_STATE_DIR="${STATE}" STYRMAN_IMAGE="${IMAGE}" STYRMAN_ADDR="127.0.0.1:${PORT}" \
    STYRMAN_HARNESS_DIR="${HARNESS_C1:+${HARNESS_OUT}/content}" \
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
check "server start logged the harness channel (tag v1.0.0, its commit)" \
    "grep -q '\"msg\":\"harness channel\"' ${STATE}/server.log && grep -q '\"tag\":\"v1.0.0\"' ${STATE}/server.log && grep -q '\"commit\":\"${HARNESS_C1}\"' ${STATE}/server.log"

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

say "agent harness (ADR-062/ADR-073: sh-agent seeded write-once from the mounted channel)"
GH="/home/${ME}"
# The stamp is version+commit (ADR-073): read once from the guest, as the
# entrypoint composes it, and compared verbatim below and after the recreate.
HID="$(docker exec "${CN}" sh -c 'printf "%s+%s" "$(cat /styrman/VERSION)" "$(cat /styrman/harness/COMMIT)"' 2>/dev/null || true)"
check "guest /styrman/harness is the channel's export (COMMIT equals the host's current)" \
    "[ -n '${HARNESS_C1}' ] && [ \"\$(docker exec ${CN} cat /styrman/harness/COMMIT)\" = '${HARNESS_C1}' ]"
check "guest /styrman/harness is read-only (a :ro bind, even for root)" \
    "! docker exec ${CN} touch /styrman/harness/.probe 2>/dev/null"
# The :ro is enforced on the HOST side of the virtiofs share, not only as a
# guest mount flag: guest root (CAP_SYS_ADMIN, apparmor off) can flip the
# guest's mount to rw, and the write still fails — and nothing lands in the
# host's export (security review 2026-09-16; the rw control: the same touch
# on a :rw bind succeeds).
check "guest cannot write the channel even after remounting it rw (host-side ro, R12)" \
    "docker exec ${CN} sh -c 'mount -o remount,rw /styrman/harness 2>/dev/null; ! touch /styrman/harness/.probe 2>/dev/null' && ! test -e '${HARNESS_OUT}/content/current/repo/.probe'"
check "guest sh-agent is the channel's binary (sha256 equals content/current/bin/sh-agent)" \
    "[ \"\$(docker exec ${CN} sha256sum /styrman/bin/sh-agent | cut -d' ' -f1)\" = \"\$(sha256sum '${HARNESS_OUT}/content/current/bin/sh-agent' | cut -d' ' -f1)\" ]"
check "no .git in the mounted harness (a plain copy, as the mirror was)" \
    "! docker exec ${CN} test -e /styrman/harness/.git"
# The two `source channel tag` log greps (here and after the swap) are
# diagnostics of the entrypoint's contract: the guest owns that stdout, so the
# EVIDENCE that the channel was mounted is the neighbouring checks — the
# guest's COMMIT equals the host's current, the binary's sha256, no .git
# (security review 2026-09-16).
check "boot log names the channel as the harness source (tag v1.0.0)" \
    "docker logs ${CN} 2>&1 | grep -F 'source channel tag v1.0.0' >/dev/null"
check "sh-agent reports no drift in the project home" \
    "docker exec ${CN} runuser -u ${ME} -- env HOME=${GH} /styrman/bin/sh-agent status --repo /styrman/harness --home ${GH} >/dev/null"
check "CLAUDE.md is the two-line import stub" \
    "docker exec ${CN} grep -qx '@AGENTS.md' ${GH}/.claude/CLAUDE.md"
check "27 skills and 7 agy agents seeded" \
    "[ \"\$(docker exec ${CN} sh -c 'ls ${GH}/.claude/skills | wc -l')\" = 27 ] && [ \"\$(docker exec ${CN} sh -c 'ls ${GH}/.gemini/config/agents | wc -l')\" = 7 ]"
# The mirror has no .git, so the harness installs it as a plain copy: never a
# `go build` in a root-owned tree, never a git call (harness BUG-015, fixed in
# 2c8990e). `status` labels it so; the boot log's own pass line names the
# mirror commit (of install's stdout only its settings.json lines reach the
# boot log, so the pass line is still the in-guest evidence that install
# succeeded this boot). The commit it must
# name is the one THIS tree pins, read from the host file — not from the
# container, which would compare a constant with itself (review 2026-09-14);
# ensure-test-image is what makes the image and the tree the same commit.
# Both values are captured first and length-checked so a failed read cannot
# leave a truncated pattern that still matches.
# `grep -F … >/dev/null`, never `grep -q`: under this script's pipefail a -q
# that exits on the first line leaves the writer with SIGPIPE (141) and the
# pipeline red on a matching output (BUG-057).
check "sh-agent status labels the mounted harness a plain copy" \
    "docker exec ${CN} runuser -u ${ME} -- env HOME=${GH} /styrman/bin/sh-agent status --repo /styrman/harness --home ${GH} | grep -F '/styrman/harness (plain copy, not its own git checkout; update is refused)' >/dev/null"
check "boot log records the harness deployed at the channel's commit" \
    "c=\"\$(cut -c1-7 '${HARNESS_OUT}/content/current/repo/COMMIT')\" && v=\"\$(docker exec ${CN} cat /styrman/VERSION)\" && [ \${#c} = 7 ] && [ -n \"\$v\" ] && docker logs ${CN} 2>&1 | grep -F \"agent harness deployed (version \$v, styrman-harness \$c\" >/dev/null"
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
# image's VERSION plus the mounted harness's COMMIT exactly (ADR-073) — a
# quoting slip would show here as a mismatch or a missing stamp.
check "harness stamp equals version+commit after boot" \
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
# R1 boot budget, from the container's own clock: the gap between its start
# and the entrypoint's ready line. The harness seed sits on that path — a
# blocking call there once cost 60 s (ADR-047) and nothing asserted it.
BOOT_AT="$(docker inspect -f '{{.State.StartedAt}}' "${CN}")"
READY_AT="$(docker logs -t "${CN}" 2>&1 | grep -m1 'project .* ready' | cut -d' ' -f1 || true)"
BOOT_GAP="$(awk "BEGIN{printf \"%.2f\", $(date -d "${READY_AT:-1970-01-01T00:00:00Z}" +%s.%N) - $(date -d "${BOOT_AT}" +%s.%N)}")"
check "boot to ready within R1's 15 s hard limit (measured ${BOOT_GAP}s; soft target 5 s)" \
    "awk 'BEGIN{exit !(${BOOT_GAP} > 0 && ${BOOT_GAP} < 15)}'"

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
docker rm -f "${CN}" >/dev/null
curl -fsS -X POST "${API}/api/projects/${ID}/start" >/dev/null
OUT=$("${STATE}/wsprobe" -url "ws://127.0.0.1:${PORT}/ws/term/${ID}/1" \
    -send "cat ~/projects/itest/proof.txt\r" -wait 4s || true)
check "file survives container recreate" "echo '${OUT}' | grep -q persisted"
# ADR-031: the docker disk lives in the PER-PROJECT home now
check "docker.img persisted" "[ -f '${STATE}/homes/${ME}/p/${ID}/.dockerd/${ID}.img' ]"
check "transcript survives recreate" \
    "curl -fsS ${API}/api/projects/${ID}/history/conv-1 | grep -q '${MARK}'"
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
# ADR-073, plan Risk (b): Docker re-resolves a bind SOURCE on every start,
# so a channel swapped while the VM is stopped is what the next start mounts
# — no recreate, the container keeps its binds on the unresolved `current`.
HARNESS_CUR2="$(bash "${REPO}/.agents/docs/testing/styrman/make-scratch-harness.sh" "${HARNESS_OUT}" "${IMAGE}" v1.0.1 2>>"${STATE}/harness-channel.log" | tail -1 || true)"
HARNESS_C2="$(cat "${HARNESS_OUT}/content/current/repo/COMMIT" 2>/dev/null || true)"
check "scratch channel swapped current to v1.0.1 while the VM is stopped (a new commit)" \
    "[ -L '${HARNESS_CUR2}' ] && [ \"\$(cat '${HARNESS_OUT}/content/current/repo/TAG')\" = v1.0.1 ] && [ -n '${HARNESS_C2}' ] && [ '${HARNESS_C2}' != '${HARNESS_C1}' ]"
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
check "restarted VM mounts the swapped channel (guest COMMIT is v1.0.1's, Docker re-resolved current)" \
    "[ \"\$(docker exec ${CN} cat /styrman/harness/COMMIT)\" = '${HARNESS_C2}' ] && docker exec ${CN} cat /styrman/harness/TAG | grep -qx v1.0.1"
check "the swap re-seeded the home from the new tag (boot log: source channel tag v1.0.1)" \
    "docker logs ${CN} 2>&1 | grep -F 'source channel tag v1.0.1' >/dev/null"
# The BUG-062 undo above left the home without a stamp or a settings.json:
# this boot seeds again and the file comes back with the key (ADR-072).
check "re-seed after the BUG-062 undo recreates settings.json with the key" \
    "docker logs ${CN} 2>&1 | grep -F \"settings: ${GH}/.claude/settings.json created with\" >/dev/null && docker exec ${CN} runuser -u ${ME} -- jq -e '.attribution.commit == \"\" and .attribution.pr == \"\"' ${GH}/.claude/settings.json >/dev/null"
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
