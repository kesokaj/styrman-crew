#!/usr/bin/env bash
# styrman Phase 1 integration test — exercises the full lifecycle against real
# Docker + Kata: create project → terminal I/O over WebSocket → persistence
# across container destruction → conversation management → cleanup.
#
# Prereqs: styrman-runtime image built (image/), Kata runtime 'mvm' working.
# Uses an isolated state dir and port; safe to run alongside a real install.
#
# Usage: bash .agents/docs/testing/styrman/integration.sh [image-tag]
set -euo pipefail

IMAGE="${1:-styrman-runtime:dev}"
PORT=18080
STATE="$(mktemp -d /tmp/styrman-itest-XXXX)"
REPO="$(cd "$(dirname "$0")/../../../.." && pwd)"
PASS=0; FAIL=0

say()  { echo -e "\n\033[1;34m=== $* ===\033[0m"; }
ok()   { echo -e "\033[32mPASS\033[0m $*"; PASS=$((PASS+1)); }
bad()  { echo -e "\033[31mFAIL\033[0m $*"; FAIL=$((FAIL+1)); }
check(){ if eval "$2"; then ok "$1"; else bad "$1"; fi; }

cleanup() {
    say "cleanup"
    kill "${SERVER_PID:-}" 2>/dev/null || true
    docker rm -f styrman-itest 2>/dev/null || true
    docker network rm sdb-itest 2>/dev/null || true
    sudo rm -rf "${STATE}" 2>/dev/null || rm -rf "${STATE}" || true
    echo "passed=${PASS} failed=${FAIL}"
    [[ ${FAIL} -eq 0 ]]
}
trap cleanup EXIT

say "build"
cd "${REPO}"
go build -o "${STATE}/styrman" ./cmd/styrman/
go build -o "${STATE}/wsprobe" ./cmd/wsprobe/

say "start server (state=${STATE}, port=${PORT})"
mkdir -p "${STATE}/home/projects"
STYRMAN_STATE_DIR="${STATE}" STYRMAN_IMAGE="${IMAGE}" STYRMAN_ADDR="127.0.0.1:${PORT}" \
    "${STATE}/styrman" > "${STATE}/server.log" 2>&1 &
SERVER_PID=$!
for i in $(seq 1 20); do
    curl -fsS "http://127.0.0.1:${PORT}/api/health" >/dev/null 2>&1 && break
    sleep 0.5
done
check "health endpoint" "curl -fsS http://127.0.0.1:${PORT}/api/health | grep -q ok"
check "embedded SPA served" "curl -fsS http://127.0.0.1:${PORT}/ | grep -q '<title>styrman</title>'"
check "SPA deep-link fallback" "curl -fsS http://127.0.0.1:${PORT}/p/x/1 | grep -q '<title>styrman</title>'"

say "create project (boots microVM, git init)"
T0=$(date +%s)
OUT=$(curl -fsS -X POST "http://127.0.0.1:${PORT}/api/projects" \
    -d '{"name":"itest","agent":"claude","gitInit":true}')
T1=$(date +%s)
echo "created in $((T1-T0))s: ${OUT}"
check "project created running" "echo '${OUT}' | grep -q '\"state\":\"running\"'"
check "microVM kernel differs from host" \
    "[ \"\$(docker exec styrman-itest uname -r)\" != \"\$(uname -r)\" ]"
check "workspace dir on host" "[ -d '${STATE}/home/projects/itest' ]"
check "git repo initialized" "docker exec styrman-itest test -d /home/$(id -un)/projects/itest/.git"
check "in-guest dockerd runs overlay2" \
    "docker exec styrman-itest docker info --format '{{.Driver}}' | grep -q overlay2"

say "terminal I/O over WebSocket (plain-shell terminal, tmux term-1)"
MARK="itest-$$-mark"
OUT=$("${STATE}/wsprobe" -url "ws://127.0.0.1:${PORT}/ws/term/itest/1?kind=shell&cols=120&rows=30" \
    -send "echo ${MARK}; pwd\r" -wait 5s || true)
check "shell echoes marker" "echo '${OUT}' | grep -q '${MARK}'"
check "cwd is the workspace" "echo '${OUT}' | grep -q 'projects/itest'"
check "terminal listed via API" \
    "curl -fsS http://127.0.0.1:${PORT}/api/projects/itest/terminals | grep -q '\"id\":\"1\"'"

say "tmux session survives disconnect"
"${STATE}/wsprobe" -url "ws://127.0.0.1:${PORT}/ws/term/itest/1?kind=shell" \
    -send "STYRMAN_TEST_VAR=${MARK}\r" -wait 2s >/dev/null || true
OUT=$("${STATE}/wsprobe" -url "ws://127.0.0.1:${PORT}/ws/term/itest/1?kind=shell" \
    -send "echo VAR=\$STYRMAN_TEST_VAR\r" -wait 4s || true)
check "reattach sees same shell state" "echo '${OUT}' | grep -q 'VAR=${MARK}'"

say "conversations API (durable records, ADR-011)"
NEW=$(curl -fsS -X POST "http://127.0.0.1:${PORT}/api/projects/itest/conversations" -d '{"title":"First chat"}')
check "record created with title" "echo '${NEW}' | grep -q '\"title\":\"First chat\"'"
check "agent session UUID pinned" "echo '${NEW}' | grep -qE '\"agentSession\":\"[0-9a-f-]{36}\"'"
curl -fsS -X PATCH "http://127.0.0.1:${PORT}/api/projects/itest/conversations/1" -d '{"title":"Renamed"}' -o /dev/null
CONVS=$(curl -fsS "http://127.0.0.1:${PORT}/api/projects/itest/conversations")
check "rename persisted" "echo '${CONVS}' | grep -q '\"title\":\"Renamed\"'"
# Attach the agent conversation (claude launches; we just verify liveness).
"${STATE}/wsprobe" -url "ws://127.0.0.1:${PORT}/ws/term/itest/1?cols=120&rows=30" -wait 4s >/dev/null || true
CONVS=$(curl -fsS "http://127.0.0.1:${PORT}/api/projects/itest/conversations")
check "conversation live after attach" "echo '${CONVS}' | grep -q '\"live\":true'"
curl -fsS -X DELETE "http://127.0.0.1:${PORT}/api/projects/itest/conversations/1" -o /dev/null
CONVS=$(curl -fsS "http://127.0.0.1:${PORT}/api/projects/itest/conversations")
check "record deleted" "! echo '${CONVS}' | grep -q '\"id\":\"1\"'"

say "persistence across container destruction (R3/R5)"
"${STATE}/wsprobe" -url "ws://127.0.0.1:${PORT}/ws/term/itest/9?kind=shell" \
    -send "echo persisted > ~/projects/itest/proof.txt\r" -wait 2s >/dev/null || true
sleep 1
docker rm -f styrman-itest >/dev/null
curl -fsS -X POST "http://127.0.0.1:${PORT}/api/projects/itest/start" >/dev/null
OUT=$("${STATE}/wsprobe" -url "ws://127.0.0.1:${PORT}/ws/term/itest/9?kind=shell" \
    -send "cat ~/projects/itest/proof.txt\r" -wait 4s || true)
check "file survives container recreate" "echo '${OUT}' | grep -q persisted"
check "docker.img persisted" "[ -f '${STATE}/home/.dockerd/itest.img' ]"

say "stop + delete (wipe)"
curl -fsS -X POST "http://127.0.0.1:${PORT}/api/projects/itest/stop" >/dev/null
check "container stopped" "! docker ps --format '{{.Names}}' | grep -q styrman-itest"
curl -fsS -X DELETE "http://127.0.0.1:${PORT}/api/projects/itest?wipe=true" -o /dev/null
check "container gone" "! docker ps -a --format '{{.Names}}' | grep -q styrman-itest"
check "workspace wiped" "[ ! -d '${STATE}/home/projects/itest' ]"
check "project gone from API" "! curl -fsS http://127.0.0.1:${PORT}/api/projects | grep -q itest"
