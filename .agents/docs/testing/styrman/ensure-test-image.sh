#!/usr/bin/env bash
# ensure-test-image.sh <tag> — rebuild the suites' runtime image when its
# sources are newer than the image, so a stale tag can never turn into false
# failures that read as source regressions (BUG-026: the :test tag went stale
# against image/skel and cost three integration checks). Both suites source
# this before their first check. Never touches :dev — that is what production
# boots (deploy/.env); the tag is always the caller's.
#
#   STYRMAN_TEST_IMAGE_NO_REBUILD=1   skip the check entirely (fast iteration)
#
# A failed build is FATAL: the caller must not run one check against an image
# it could not refresh.
set -euo pipefail
TAG="${1:?usage: ensure-test-image.sh <image-tag>}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)"
# Never the image production boots: deploy/.env's STYRMAN_IMAGE (on this host
# styrman-runtime:dev). Rebuilding it here re-stamps what real projects run —
# the 2026-09-01 audit finding the suites' :test default exists to prevent
# (security review of the close-out's phase 3).
PROD_TAG="$( { sed -n 's/^STYRMAN_IMAGE=//p' "${ROOT}/deploy/.env" 2>/dev/null || true; } | tr -d '"' | head -1)"
case "${TAG}" in
    "${PROD_TAG:-styrman-runtime:dev}"|styrman-runtime:dev|*:latest)
        echo "FATAL: ensure-test-image: refusing to rebuild ${TAG} — that is a production tag, not a test tag" >&2
        exit 1 ;;
esac
if [ "${STYRMAN_TEST_IMAGE_NO_REBUILD:-}" = "1" ]; then
    echo "ensure-test-image: skipped (STYRMAN_TEST_IMAGE_NO_REBUILD=1) — ${TAG} as is"
    exit 0
fi
created="$(docker image inspect --format '{{.Created}}' "${TAG}" 2>/dev/null || true)"
reason=""
if [ -z "${created}" ]; then
    reason="image ${TAG} is missing"
else
    # Newest mtime under image/ (the harness mirror's own .git excluded);
    # compare as epoch seconds so this works with GNU date.
    img_epoch="$(date -d "${created}" +%s)"
    newest="$(find "${ROOT}/image" -path '*/.git' -prune -o -type f -printf '%T@\n' 2>/dev/null | sort -n | tail -1 | cut -d. -f1)"
    if [ -n "${newest}" ] && [ "${newest}" -gt "${img_epoch}" ]; then
        reason="image/ has files newer ($(date -d @"${newest}" +%FT%T)) than ${TAG} (${created%%.*})"
    fi
fi
if [ -z "${reason}" ]; then
    echo "ensure-test-image: ${TAG} is current (built ${created%%.*})"
    exit 0
fi
echo "ensure-test-image: rebuilding ${TAG}: ${reason}"
if ! docker build -t "${TAG}" --build-arg "STYRMAN_VERSION=test-$(date +%s)" "${ROOT}/image" >"${ROOT}/.agents/scratch/ensure-test-image.log" 2>&1; then
    echo "FATAL: ensure-test-image: docker build of ${TAG} failed — see .agents/scratch/ensure-test-image.log" >&2
    exit 1
fi
echo "ensure-test-image: ${TAG} rebuilt"
