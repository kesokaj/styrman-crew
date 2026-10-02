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
# A stale image is rebuilt before the first check (BUG-026); FATAL on failure.
bash "$(dirname "${BASH_SOURCE[0]}")/../../.agents/docs/testing/styrman/ensure-test-image.sh" "${IMAGE}"
PORT="${STYRMAN_E2E_PORT:-18090}"
STATE="$(mktemp -d /tmp/styrman-e2e-XXXX)"
# Epoch seconds at start. cleanup() removes only networks created AFTER this,
# which is what keeps it away from production's (created long ago) without
# needing a label the orchestrator does not set.
RUN_START="$(date -u +%s)"
# The mount-source format for cleanup()'s `docker inspect`, defined HERE and
# not inline in the trap: a single-quoted format string with a literal
# newline inside `$( )` inside an EXIT trap is exactly the shape BUG-038's
# unexplained "unexpected EOF while looking for matching `'" named, and a
# variable defined at the top is parsed once, where `bash -n` can see it.
MOUNT_SOURCES_FMT=$'{{range .Mounts}}{{.Source}}\n{{end}}'
REPO="$(cd "$(dirname "$0")/../.." && pwd)"
API="http://127.0.0.1:${PORT}"
PID=""

cleanup() {
    kill "${PID:-}" 2>/dev/null || true
    # Remove any project VM + network the run created (isolated state dir, so
    # these UUIDs are ours alone).
    for c in $(docker ps -aq --filter "label=styrman.managed=true" \
               2>/dev/null); do
        # Match on the container's MOUNT SOURCES, not its env. This loop used
        # to keep a container only if its env carried
        # `STYRMAN_STATE_DIR=${STATE}` — a variable the orchestrator never
        # sets on a project container (its env is STYRMAN_USER, _UID, _GID,
        # _CRED_BROKER, _PROJECT, _PROJECT_DIR, _DOCKER_DISK_GB and nothing
        # else). So the filter never matched, and this safety net had never
        # once removed anything: a run that ABORTED before the UI cleanup
        # section leaked its microVM, silently, because a green run is tidied
        # by the server instead when the suite deletes the project.
        #
        # Every project container binds its home and workspace from under its
        # server's StateDir, so a source under "${STATE}/" is exactly the
        # property "belongs to THIS run's isolated server" — which is what the
        # env check was reaching for. Production's containers mount from
        # /var/lib/styrman and can never match a mktemp'd ${STATE}.
        #
        # `|| continue`, and not for tidiness: set -e is in force inside an
        # EXIT trap too, so a failing command substitution in an ASSIGNMENT
        # aborts the trap where it stands. A container listed a moment ago by
        # `docker ps -aq` and already gone by now (this run's own project, the
        # server deletes it asynchronously) made `docker inspect` fail, which
        # killed cleanup BEFORE the rm below — leaving /tmp/styrman-e2e-* dirs
        # behind AND handing the shell an exit status of 1 after a run that
        # reported failed=0 (BUG-024).
        srcs=$(docker inspect "$c" --format "${MOUNT_SOURCES_FMT}" 2>/dev/null) || continue
        echo "$srcs" | grep -q "^${STATE}/" && docker rm -f "$c" >/dev/null 2>&1 || true
    done
    # Networks outlive their containers here. The suite creates several
    # projects (the main one, a deliberately-failing clone, a succeeding
    # clone) and does not delete all of them, and a project whose clone failed
    # can leave a network with no container ever attached. Each one holds a
    # /16 out of Docker's default pool, and TODO.md already carries the risk
    # that a 16th bridge lands in 192.168/16 — so leaking these per run is not
    # cosmetic.
    #
    # RETRIED, because the server deletes projects ASYNCHRONOUSLY: at trap
    # time a network can still have its container attached, and a single pass
    # skipped exactly those and leaked them once the container went away a
    # moment later. Bounded at ~12 s so cleanup cannot hang.
    #
    # The guard that protects production is the TIMESTAMP — its network
    # predates this run. `docker network rm` refuses a network that is still
    # in use, so an early attempt is harmless and simply retries.
    #
    # `.Created.Unix`, not `.Created`: Go renders the bare field as
    # "2026-09-09 16:48:14.959530866 +0200 CEST", which GNU date refuses (an
    # offset AND a zone abbreviation), so parsing it in the shell silently
    # `continue`d on every network and the sweep did nothing at all.
    for _ in 1 2 3 4 5 6; do
        left=0
        for n in $(docker network ls --format '{{.Name}}' 2>/dev/null \
                   | grep -E '^(sdb|lan)-' || true); do
            created=$(docker network inspect "$n" \
                      --format '{{.Created.Unix}}' 2>/dev/null) || continue
            [ "${created}" -ge "${RUN_START}" ] || continue
            docker network rm "$n" >/dev/null 2>&1 || left=1
        done
        [ "${left}" = 0 ] && break
        sleep 2
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
# Updater status (ADR-081): exported, so the server below and run.mjs (which
# plants the file for its Settings check) read the SAME path.
export STYRMAN_UPDATE_STATUS_FILE="${STATE}/status/updater.json"
STYRMAN_STATE_DIR="${STATE}" STYRMAN_IMAGE="${IMAGE}" STYRMAN_ADDR="127.0.0.1:${PORT}" \
    "${STATE}/styrman" > "${STATE}/server.log" 2>&1 &
PID=$!
for i in $(seq 1 30); do
    curl -fsS "${API}/api/health" >/dev/null 2>&1 && break
    sleep 0.5
done
curl -fsS "${API}/api/health" >/dev/null 2>&1 || { echo "server failed to start"; cat "${STATE}/server.log"; exit 1; }

echo "== running Playwright checks =="
# No firewall fixture since ADR-067: the network mode is the bridge interface
# name the server picks, so the browser checks exercise the real thing against
# the real server — there is no root half to fake.
# Take the suite's own status and exit with it deliberately. Letting the
# script fall off the end here makes the EXIT trap's last command decide the
# exit code, which is how a green run came to report failure (see cleanup).
status=0
node "${REPO}/web/e2e/run.mjs" --base "${API}" || status=$?
exit "${status}"
