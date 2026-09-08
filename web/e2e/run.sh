#!/usr/bin/env bash
# styrman e2e harness: launches its OWN server on host loopback (auth=none,
# isolated state dir) and runs the Playwright checks against it. Self-hosting
# is deliberate — production has no auth-bypass listener, and a host-loopback
# server is unreachable from project microVMs (2026-08-27 security review).
#
# Usage: bash web/e2e/run.sh [image-tag]
set -euo pipefail

# Default to the TEST tag, not :dev — deploy/.env points STYRMAN_IMAGE at
# styrman-runtime:dev, so defaulting there boots real project microVMs against
# production's image (and the integration tier's prereq used to REBUILD it,
# re-stamping what production boots — audit finding 2026-09-01).
IMAGE="${1:-styrman-runtime:test}"
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
               2>/dev/null); do
        env=$(docker inspect "$c" --format '{{range .Config.Env}}{{println .}}{{end}}' 2>/dev/null)
        echo "$env" | grep -q "STYRMAN_STATE_DIR=${STATE}" && docker rm -f "$c" >/dev/null 2>&1 || true
    done
    sudo rm -rf "${STATE}" 2>/dev/null || rm -rf "${STATE}" || true
}
trap cleanup EXIT

# NOTE: this rewrites tracked files — web/dist/client gets fresh content-hashed
# asset names on every run, so `git status` is dirty afterwards. That is the
# build output, not stray edits; stage it with `git add -A -- web/dist`.
#
# Build the SPA FIRST. The server embeds web/dist/client (web/embed.go), so
# building only the Go binary silently tests whatever assets were lying in
# dist from a previous build — a frontend change would appear to "fail" while
# the real fix sat unbuilt (hit for real on 2026-09-01). Skip with
# STYRMAN_E2E_SKIP_WEB=1 when iterating on server-only changes.
if [ -z "${STYRMAN_E2E_SKIP_WEB:-}" ]; then
    echo "== building web assets =="
    ( cd "${REPO}/web" && npm run build >/dev/null ) \
        || { echo "FATAL: web build failed"; exit 1; }
fi

echo "== building server =="
( cd "${REPO}" && go build -o "${STATE}/styrman" ./cmd/styrman/ )

echo "== starting host server (state=${STATE}, port=${PORT}) =="
# STYRMAN_FIREWALL_DIR at a scratch dir (ADR-065): a host-process test
# server must never read the production reconciler's status or kick its
# spool; with no status file every project reports "unavailable", which is
# the branch the e2e checks exercise.
STYRMAN_STATE_DIR="${STATE}" STYRMAN_IMAGE="${IMAGE}" STYRMAN_ADDR="127.0.0.1:${PORT}" \
    STYRMAN_FIREWALL_DIR="${STATE}/fw" \
    "${STATE}/styrman" > "${STATE}/server.log" 2>&1 &
PID=$!
for i in $(seq 1 30); do
    curl -fsS "${API}/api/health" >/dev/null 2>&1 && break
    sleep 0.5
done
curl -fsS "${API}/api/health" >/dev/null 2>&1 || { echo "server failed to start"; cat "${STATE}/server.log"; exit 1; }

echo "== running Playwright checks =="
node "${REPO}/web/e2e/run.mjs" --base "${API}"
