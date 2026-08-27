#!/usr/bin/env bash
# styrman e2e harness: launches its OWN server on host loopback (auth=none,
# isolated state dir) and runs the Playwright checks against it. Self-hosting
# is deliberate — production has no auth-bypass listener, and a host-loopback
# server is unreachable from project microVMs (2026-08-27 security review).
#
# Usage: bash web/e2e/run.sh [image-tag]
set -euo pipefail

IMAGE="${1:-styrman-runtime:dev}"
PORT="${STYRMAN_E2E_PORT:-18090}"
STATE="$(mktemp -d /tmp/styrman-e2e-XXXX)"
REPO="$(cd "$(dirname "$0")/../.." && pwd)"
API="http://127.0.0.1:${PORT}"
PID=""

cleanup() {
    kill "${PID:-}" 2>/dev/null || true
    # Remove any project VM + network the run created (isolated state dir, so
    # these UUIDs are ours alone).
    for c in $(docker ps -aq --filter "label=styrman.managed=true" \
               --filter "status=running" 2>/dev/null); do
        env=$(docker inspect "$c" --format '{{range .Config.Env}}{{println .}}{{end}}' 2>/dev/null)
        echo "$env" | grep -q "STYRMAN_STATE_DIR=${STATE}" && docker rm -f "$c" >/dev/null 2>&1 || true
    done
    sudo rm -rf "${STATE}" 2>/dev/null || rm -rf "${STATE}" || true
}
trap cleanup EXIT

echo "== building server =="
( cd "${REPO}" && go build -o "${STATE}/styrman" ./cmd/styrman/ )

echo "== starting host server (state=${STATE}, port=${PORT}) =="
STYRMAN_STATE_DIR="${STATE}" STYRMAN_IMAGE="${IMAGE}" STYRMAN_ADDR="127.0.0.1:${PORT}" \
    "${STATE}/styrman" > "${STATE}/server.log" 2>&1 &
PID=$!
for i in $(seq 1 30); do
    curl -fsS "${API}/api/health" >/dev/null 2>&1 && break
    sleep 0.5
done
curl -fsS "${API}/api/health" >/dev/null 2>&1 || { echo "server failed to start"; cat "${STATE}/server.log"; exit 1; }

echo "== running Playwright checks =="
node "${REPO}/web/e2e/run.mjs" --base "${API}"
