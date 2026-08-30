#!/usr/bin/env bash
# styrman integration test — exercises the full lifecycle against real
# Docker + Kata: create project → terminals → WebSocket I/O → transcripts →
# file explorer → ports/preview → git → persistence → cleanup.
#
# Prereqs: styrman-runtime image built (image/), Kata runtime 'mvm' working.
# Uses an isolated state dir and port; safe to run alongside a real install.
#
# Usage: bash .agents/docs/testing/styrman/integration.sh [image-tag]
set -euo pipefail

IMAGE="${1:-styrman-runtime:dev}"
PORT=18080
ME="$(id -un)"        # auth-none mode acts as the OS user
ID=""                 # project UUID, captured after create
CN=""                 # container name styrman-<uuid>
STATE="$(mktemp -d /tmp/styrman-itest-XXXX)"
REPO="$(cd "$(dirname "$0")/../../../.." && pwd)"
API="http://127.0.0.1:${PORT}"
PASS=0; FAIL=0

say()  { echo -e "\n\033[1;34m=== $* ===\033[0m"; }
ok()   { echo -e "\033[32mPASS\033[0m $*"; PASS=$((PASS+1)); }
bad()  { echo -e "\033[31mFAIL\033[0m $*"; FAIL=$((FAIL+1)); }
check(){ if eval "$2"; then ok "$1"; else bad "$1"; fi; }

cleanup() {
    say "cleanup"
    kill "${SERVER_PID:-}" 2>/dev/null || true
    [ -n "${CN}" ] && docker rm -f "${CN}" 2>/dev/null || true
    [ -n "${ID}" ] && docker network rm "sdb-${ID}" 2>/dev/null || true
    sudo rm -rf "${STATE}" 2>/dev/null || rm -rf "${STATE}" || true
    echo "passed=${PASS} failed=${FAIL}"
    # An EXIT trap's status does NOT become the script's exit status (audit
    # finding: failing runs exited 0) — exit explicitly from the trap.
    exit $(( FAIL == 0 ? 0 : 1 ))
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
for i in $(seq 1 20); do
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
for i in $(seq 1 60); do
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
check "project listing works" "curl -fsS '${API}/api/projects/${ID}/files' | grep -q entries"
check "home listing works (per-project)" "curl -fsS '${API}/api/projects/${ID}/home/files' | grep -q entries"
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
check "pretty URL proxies to the app" \
    "curl -fsS ${API}/p/${ID}/1/port/4100/i.html | grep -q preview"
check "legacy proxy URL still works" \
    "curl -fsS ${API}/proxy/${ID}/4100/i.html | grep -q preview"
check "bare URL redirects to trailing slash" \
    "[ \$(curl -s -o /dev/null -w '%{http_code}' ${API}/p/${ID}/1/port/4100) = 301 ]"

say "public TTL preview links (1.1 foundation)"
LINK_TOK=$(curl -fsS -X POST "${API}/api/projects/${ID}/ports/4100/links" -d '{"ttl":"1h"}' | python3 -c 'import sys,json;print(json.load(sys.stdin)["token"])' 2>/dev/null || true)
check "link minted" "[ -n \"${LINK_TOK}\" ]"
check "public link serves the app (no auth)" \
    "curl -fsS ${API}/link/${LINK_TOK}/i.html | grep -q preview"
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
curl -fsS -X DELETE "${API}/api/projects/${ID}?wipe=true" -o /dev/null
# Delete is ASYNC since v1.3 (202 + background: destroy container, THEN wipe,
# THEN remove the record). Wait for the LAST step — the record leaving the
# API list — so the wipe + record-removal checks below don't race the tail.
for i in $(seq 1 60); do
    curl -fsS "${API}/api/projects" | grep -q "${ID}" || break
    sleep 2
done
check "container gone (async delete)" "! docker ps -a --format '{{.Names}}' | grep -q ${CN}"
check "workspace wiped" "[ ! -d '${STATE}/projects/${ME}/${ID}' ]"
check "project gone from API" "! curl -fsS ${API}/api/projects | grep -q ${ID}"
