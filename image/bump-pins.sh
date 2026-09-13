#!/usr/bin/env bash
# bump-pins — refresh the runtime image's pinned versions and checksums
# (ADR-069) from the publishers' own sources, so a bump is one command and
# never a hand-copied hash.
#
#   bash image/bump-pins.sh            # print the ARG lines / digests that would change
#   bash image/bump-pins.sh --apply    # rewrite them in image/Dockerfile (and the
#                                      # base-image digests in both Dockerfiles)
#
# Sources: go.dev/dl (latest stable, its published sha256 for linux-amd64),
# nodejs.org/dist (latest of the pinned major, SHASUMS256.txt), the npm
# registry (@anthropic-ai/claude-code latest — npm publishes no separate sum;
# the version alone is pinned), and `docker manifest inspect` for the amd64
# digest of each FROM tag. Antigravity (`agy`) is not pinned by design
# (ADR-069) and is not touched.
#
# Trust model, stated plainly: TLS to each publisher plus the publisher's own
# published sums. Nothing here verifies a signature (Node's SHASUMS256.txt.asc
# is not checked). What a bump proves is that the ARCHIVE the build fetches
# matches what the publisher listed, and that a later change upstream fails
# the build loudly. After --apply: `docker build image/` (or
# `.agents/docs/testing/styrman/ensure-test-image.sh styrman-runtime:test`),
# then commit the two Dockerfiles together.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
IMG="${ROOT}/image/Dockerfile"
SRV="${ROOT}/deploy/Dockerfile.server"
APPLY=0
[ "${1:-}" = "--apply" ] && APPLY=1

for tool in curl jq docker sed npm awk; do
    command -v "${tool}" >/dev/null 2>&1 || { echo "bump-pins: ${tool} is required" >&2; exit 1; }
done

current() { sed -n "s/^ARG $1=\(.*\)$/\1/p" "${IMG}" | head -1; }
# Every publisher value is shape-checked BEFORE it goes anywhere near sed: a
# hostile or malformed response must not become a sed program (security
# review, 2026-09-13).
is_version() { [[ "$1" =~ ^[0-9]+\.[0-9]+(\.[0-9]+)?$ ]]; }
is_sha256()  { [[ "$1" =~ ^[0-9a-f]{64}$ ]]; }
is_digest()  { [[ "$1" =~ ^sha256:[0-9a-f]{64}$ ]]; }

fail_read=0
go_json="$(curl -fsSL 'https://go.dev/dl/?mode=json' || true)"
GO_VERSION="$(printf '%s' "${go_json}" | jq -r '.[0].version // empty' 2>/dev/null | sed 's/^go//')"
GO_SHA256="$(printf '%s' "${go_json}" | jq -r '.[0].files[]? | select(.os=="linux" and .arch=="amd64" and .kind=="archive") | .sha256' 2>/dev/null | head -1)"
is_version "${GO_VERSION}" && is_sha256 "${GO_SHA256}" || { echo "bump-pins: go.dev/dl gave no usable version/sum" >&2; fail_read=1; }

NODE_MAJOR="$(current NODE_VERSION | cut -d. -f1)"
NODE_VERSION="$(curl -fsSL https://nodejs.org/dist/index.json 2>/dev/null | jq -r --arg m "v${NODE_MAJOR}." '[.[] | select(.version|startswith($m))][0].version // empty' 2>/dev/null | sed 's/^v//')"
NODE_SHA256=""
if is_version "${NODE_VERSION}"; then
    NODE_SHA256="$(curl -fsSL "https://nodejs.org/dist/v${NODE_VERSION}/SHASUMS256.txt" 2>/dev/null | awk -v f="node-v${NODE_VERSION}-linux-x64.tar.xz" '$2==f {print $1}' | head -1)"
fi
is_version "${NODE_VERSION}" && is_sha256 "${NODE_SHA256}" || { echo "bump-pins: nodejs.org gave no usable version/sum for the v${NODE_MAJOR} line" >&2; fail_read=1; }

CLAUDE_CODE_VERSION="$(npm view @anthropic-ai/claude-code version 2>/dev/null || true)"
is_version "${CLAUDE_CODE_VERSION}" || { echo "bump-pins: npm gave no usable @anthropic-ai/claude-code version" >&2; fail_read=1; }

# Base images: the amd64 digest behind each FROM tag (the tag stays, the digest moves).
digest_of() { docker manifest inspect "$1" 2>/dev/null | jq -r '.manifests[]? | select(.platform.architecture=="amd64" and .platform.os=="linux") | .digest' 2>/dev/null | head -1; }
declare -A DIGEST
for f in "${IMG}" "${SRV}"; do
    while IFS= read -r ref; do
        tag="${ref%%@*}"
        [ -n "${DIGEST[${tag}]:-}" ] && continue
        d="$(digest_of "${tag}")"
        if is_digest "${d}"; then DIGEST["${tag}"]="${d}"; else echo "bump-pins: no amd64 digest for ${tag} (docker manifest inspect)" >&2; fail_read=1; fi
    done < <(sed -n 's/^FROM \([^ ]*\).*$/\1/p' "${f}")
done

[ "${fail_read}" = 0 ] || { echo "bump-pins: could not read every publisher source — nothing changed" >&2; exit 1; }

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
    cur="$(grep -ho "^FROM ${tag}@sha256:[0-9a-f]*" "${IMG}" "${SRV}" | head -1 | sed 's/.*@//')"
    report "FROM ${tag}" "${cur:-<no digest>}" "${DIGEST[${tag}]}"
done

[ "${changed}" = 1 ] || { echo "bump-pins: everything is current"; exit 0; }
[ "${APPLY}" = 1 ] || { echo "bump-pins: run again with --apply to rewrite the ARG lines and digests"; exit 0; }

# Rewrite temp copies, prove every expected line is present, then move both
# into place together — never a half-rewritten tree (review 2026-09-13).
tmp_img="$(mktemp)"; tmp_srv="$(mktemp)"
trap 'rm -f "${tmp_img}" "${tmp_srv}"' EXIT
cp "${IMG}" "${tmp_img}"; cp "${SRV}" "${tmp_srv}"
sed -i "s|^ARG GO_VERSION=.*|ARG GO_VERSION=${GO_VERSION}|; s|^ARG GO_SHA256=.*|ARG GO_SHA256=${GO_SHA256}|; s|^ARG NODE_VERSION=.*|ARG NODE_VERSION=${NODE_VERSION}|; s|^ARG NODE_SHA256=.*|ARG NODE_SHA256=${NODE_SHA256}|; s|^ARG CLAUDE_CODE_VERSION=.*|ARG CLAUDE_CODE_VERSION=${CLAUDE_CODE_VERSION}|" "${tmp_img}"
for tag in "${!DIGEST[@]}"; do
    # A FROM with a digest gets it replaced; a bare FROM (tag only, optionally
    # "AS name") gets one added.
    sed -i "s|^FROM ${tag}@sha256:[0-9a-f]*|FROM ${tag}@${DIGEST[${tag}]}|; s|^FROM ${tag}\( AS .*\)\?$|FROM ${tag}@${DIGEST[${tag}]}\1|" "${tmp_img}" "${tmp_srv}"
done
for want in "ARG GO_VERSION=${GO_VERSION}" "ARG GO_SHA256=${GO_SHA256}" "ARG NODE_VERSION=${NODE_VERSION}" "ARG NODE_SHA256=${NODE_SHA256}" "ARG CLAUDE_CODE_VERSION=${CLAUDE_CODE_VERSION}"; do
    grep -qxF -- "${want}" "${tmp_img}" || { echo "bump-pins: rewrite did not land: ${want} — nothing changed" >&2; exit 1; }
done
for tag in "${!DIGEST[@]}"; do
    grep -qsF -- "FROM ${tag}@${DIGEST[${tag}]}" "${tmp_img}" "${tmp_srv}" || { echo "bump-pins: digest rewrite did not land for ${tag} — nothing changed" >&2; exit 1; }
done
mv -f "${tmp_img}" "${IMG}"; mv -f "${tmp_srv}" "${SRV}"
trap - EXIT
echo "bump-pins: applied — now build the image to prove the sums, then commit image/Dockerfile and deploy/Dockerfile.server together"
