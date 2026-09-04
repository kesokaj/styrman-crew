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
PORT=18080
ME="$(id -un)"        # auth-none mode acts as the OS user
ID=""                 # project UUID, captured after create
CN=""                 # container name styrman-<uuid>
STATE="$(mktemp -d /tmp/styrman-itest-XXXX)"
REPO="$(cd "$(dirname "$0")/../../../.." && pwd)"
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

say "agent harness (ADR-062: sh-agent seeded write-once from the image)"
GH="/home/${ME}"
check "sh-agent reports no drift in the project home" \
    "docker exec ${CN} runuser -u ${ME} -- env HOME=${GH} /styrman/bin/sh-agent status --repo /styrman/harness --home ${GH} >/dev/null"
check "CLAUDE.md is the two-line import stub" \
    "docker exec ${CN} grep -qx '@AGENTS.md' ${GH}/.claude/CLAUDE.md"
check "28 skills and 7 agy agents seeded" \
    "[ \"\$(docker exec ${CN} sh -c 'ls ${GH}/.claude/skills | wc -l')\" = 28 ] && [ \"\$(docker exec ${CN} sh -c 'ls ${GH}/.gemini/config/agents | wc -l')\" = 7 ]"
check "no previous-harness artifacts (shell gates, GEMINI.md, settings.json)" \
    "! docker exec ${CN} sh -c 'test -e ${GH}/.agents/bin || test -e ${GH}/.gemini/GEMINI.md || test -e ${GH}/.claude/settings.json'"

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
docker rm -f "${CN}" >/dev/null
curl -fsS -X POST "${API}/api/projects/${ID}/start" >/dev/null
OUT=$("${STATE}/wsprobe" -url "ws://127.0.0.1:${PORT}/ws/term/${ID}/1" \
    -send "cat ~/projects/itest/proof.txt\r" -wait 4s || true)
check "file survives container recreate" "echo '${OUT}' | grep -q persisted"
# ADR-031: the docker disk lives in the PER-PROJECT home now
check "docker.img persisted" "[ -f '${STATE}/homes/${ME}/p/${ID}/.dockerd/${ID}.img' ]"
check "transcript survives recreate" \
    "curl -fsS ${API}/api/projects/${ID}/history/conv-1 | grep -q '${MARK}'"

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
curl -fsS -X DELETE "${API}/api/projects/${ID}?wipe=true" -o /dev/null
# Delete is ASYNC since v1.3 (202 + background: destroy container, THEN wipe,
# THEN remove the record). Wait for the LAST step — the record leaving the
# API list — so the wipe + record-removal checks below don't race the tail.
for _ in $(seq 1 60); do
    curl -fsS "${API}/api/projects" | grep -q "${ID}" || break
    sleep 2
done
check "container gone (async delete)" "! docker ps -a --format '{{.Names}}' | grep -q ${CN}"
check "workspace wiped" "[ ! -d '${STATE}/projects/${ME}/${ID}' ]"
check "project gone from API" "! curl -fsS ${API}/api/projects | grep -q ${ID}"
ABORTED=0
