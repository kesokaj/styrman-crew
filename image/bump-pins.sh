#!/usr/bin/env bash
# bump-pins — refresh the runtime image's pinned versions and checksums
# (ADR-069) from the publishers' own sources, so a bump is one command and
# never a hand-copied hash.
#
#   bash image/bump-pins.sh            # print the ARG lines that would change
#   bash image/bump-pins.sh --apply    # rewrite them in image/Dockerfile (and the
#                                      # base-image digests in both Dockerfiles)
#
# Sources: go.dev/dl (latest stable, its published sha256 for linux-amd64),
# nodejs.org/dist (latest of the pinned major, SHASUMS256.txt),
# npm (@anthropic-ai/claude-code latest), and `docker manifest inspect`
# for the amd64 digest of each FROM tag. Antigravity (`agy`) is not pinned
# by design (ADR-069) and is not touched. After --apply: `docker build image/`
# (or `.agents/docs/testing/styrman/ensure-test-image.sh styrman-runtime:test`)
# proves the new sums, then commit the two Dockerfiles together.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
IMG="${ROOT}/image/Dockerfile"
SRV="${ROOT}/deploy/Dockerfile.server"
APPLY=0
[ "${1:-}" = "--apply" ] && APPLY=1

for tool in curl jq docker sed; do
    command -v "${tool}" >/dev/null 2>&1 || { echo "bump-pins: ${tool} is required" >&2; exit 1; }
done

current() { sed -n "s/^ARG $1=\(.*\)$/\1/p" "${IMG}" | head -1; }

# Go: latest stable and its published sum for the linux-amd64 archive.
go_json="$(curl -fsSL 'https://go.dev/dl/?mode=json')"
GO_VERSION="$(printf '%s' "${go_json}" | jq -r '.[0].version' | sed 's/^go//')"
GO_SHA256="$(printf '%s' "${go_json}" | jq -r '.[0].files[] | select(.os=="linux" and .arch=="amd64" and .kind=="archive") | .sha256')"

# Node: newest release of the major the Dockerfile pins, from its SHASUMS256.txt.
NODE_MAJOR="$(current NODE_VERSION | cut -d. -f1)"
NODE_VERSION="$(curl -fsSL https://nodejs.org/dist/index.json | jq -r --arg m "v${NODE_MAJOR}." '[.[] | select(.version|startswith($m))][0].version' | sed 's/^v//')"
NODE_SHA256="$(curl -fsSL "https://nodejs.org/dist/v${NODE_VERSION}/SHASUMS256.txt" | awk -v f="node-v${NODE_VERSION}-linux-x64.tar.xz" '$2==f {print $1}')"

# Claude Code: npm's latest.
CLAUDE_CODE_VERSION="$(npm view @anthropic-ai/claude-code version 2>/dev/null)"

[ -n "${GO_VERSION}" ] && [ -n "${GO_SHA256}" ] && [ -n "${NODE_VERSION}" ] && [ -n "${NODE_SHA256}" ] && [ -n "${CLAUDE_CODE_VERSION}" ] \
    || { echo "bump-pins: could not read every publisher source — nothing changed" >&2; exit 1; }

# Base images: the amd64 digest behind each FROM tag (the tag stays, the digest moves).
digest_of() { docker manifest inspect "$1" 2>/dev/null | jq -r '.manifests[] | select(.platform.architecture=="amd64" and .platform.os=="linux") | .digest' | head -1; }
declare -A DIGEST
for f in "${IMG}" "${SRV}"; do
    while IFS= read -r ref; do
        tag="${ref%%@*}"
        [ -n "${DIGEST[${tag}]:-}" ] || DIGEST["${tag}"]="$(digest_of "${tag}")"
    done < <(sed -n 's/^FROM \([^ ]*\).*$/\1/p' "${f}")
done

changed=0
report() { # name current new
    if [ "$2" != "$3" ]; then echo "  $1: $2 -> $3"; changed=1; else echo "  $1: $2 (current)"; fi
}
echo "bump-pins: publisher state $(date -u +%FT%TZ)"
report GO_VERSION "$(current GO_VERSION)" "${GO_VERSION}"
report GO_SHA256 "$(current GO_SHA256)" "${GO_SHA256}"
report NODE_VERSION "$(current NODE_VERSION)" "${NODE_VERSION}"
report NODE_SHA256 "$(current NODE_SHA256)" "${NODE_SHA256}"
report CLAUDE_CODE_VERSION "$(current CLAUDE_CODE_VERSION)" "${CLAUDE_CODE_VERSION}"
for tag in "${!DIGEST[@]}"; do
    [ -n "${DIGEST[${tag}]}" ] || { echo "  ${tag}: digest unavailable (docker manifest inspect failed)"; continue; }
    cur="$(grep -ho "^FROM ${tag}@sha256:[0-9a-f]*" "${IMG}" "${SRV}" | head -1 | sed 's/.*@//')"
    report "FROM ${tag}" "${cur:-<no digest>}" "${DIGEST[${tag}]}"
done

[ "${changed}" = 1 ] || { echo "bump-pins: everything is current"; exit 0; }
[ "${APPLY}" = 1 ] || { echo "bump-pins: run again with --apply to rewrite the ARG lines and digests"; exit 0; }

sed -i "s/^ARG GO_VERSION=.*/ARG GO_VERSION=${GO_VERSION}/; s/^ARG GO_SHA256=.*/ARG GO_SHA256=${GO_SHA256}/; s/^ARG NODE_VERSION=.*/ARG NODE_VERSION=${NODE_VERSION}/; s/^ARG NODE_SHA256=.*/ARG NODE_SHA256=${NODE_SHA256}/; s/^ARG CLAUDE_CODE_VERSION=.*/ARG CLAUDE_CODE_VERSION=${CLAUDE_CODE_VERSION}/" "${IMG}"
for tag in "${!DIGEST[@]}"; do
    [ -n "${DIGEST[${tag}]}" ] || continue
    sed -i "s#^FROM ${tag}@sha256:[0-9a-f]*#FROM ${tag}@${DIGEST[${tag}]}#" "${IMG}" "${SRV}"
done
echo "bump-pins: applied — now build the image to prove the sums, then commit image/Dockerfile and deploy/Dockerfile.server together"
