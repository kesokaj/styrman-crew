#!/usr/bin/env bash
# Verify a COMMIT, not the working tree.
#
# Why this exists: v1.18.0 was tagged from a working tree that built while the
# COMMIT did not — `bootFailed` was extracted into a new file that never got
# staged, so the pushed tree called a function it did not define. Every local
# gate passed, because every local gate ran against the working tree. The
# auto-updater built from the tag, failed, and rolled production back.
#
# Run this before signing a release tag. It checks out the commit into a
# detached worktree under /tmp and builds THAT — so anything unstaged,
# untracked, or forgotten is absent, exactly as it will be for the updater.
#
# Usage: deploy/verify-commit.sh [commit-ish]   (default: HEAD)
set -euo pipefail

REF="${1:-HEAD}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SHA="$(git -C "${ROOT}" rev-parse --verify "${REF}^{commit}")"
WT="$(mktemp -d "${TMPDIR:-/tmp}/styrman-verify-XXXXXX")"
IMG="styrman-verify-$$:tmp"

cleanup() {
    git -C "${ROOT}" worktree remove --force "${WT}" >/dev/null 2>&1 || true
    rm -rf "${WT}" 2>/dev/null || true
    docker rmi "${IMG}" >/dev/null 2>&1 || true
}
trap cleanup EXIT

echo "==> verifying commit ${SHA:0:12} (${REF}) in a detached worktree"
git -C "${ROOT}" worktree add -q --detach "${WT}" "${SHA}"

# Anything the working tree has but the commit does not is invisible from here
# — which is the entire point.
echo "==> go build"; ( cd "${WT}" && go build ./... )
echo "==> go vet";   ( cd "${WT}" && go vet ./... )
echo "==> go test -race"; ( cd "${WT}" && go test -race -count=1 ./... )

# The vendored ingress is a SEPARATE Go module: the root `./...` never reaches
# it, so a break there ships silently (TODO 2026-09-01, closed 2026-09-02).
echo "==> vendored ingress module (separate go.mod)"
( cd "${WT}/deploy/caddy/ingress" && go vet ./... && go test ./... )

# The Go binary EMBEDS web/dist (web/embed.go), and web/dist is committed. So a
# frontend change whose build output was never committed ships a server whose
# UI silently predates its API. Rebuild from the commit's own web/src and
# compare: vite's content hashes are derived from content, so a matching build
# produces identical filenames (TODO 2026-09-01; hit for real 2026-09-02).
if [ -z "${SKIP_WEB_FRESHNESS:-}" ] && command -v npm >/dev/null 2>&1; then
    echo "==> web/dist freshness (rebuild and compare)"
    committed="$(cd "${WT}/web/dist" && find . -type f | sort)"
    ( cd "${WT}/web" && npm ci --no-audit --no-fund >/dev/null 2>&1 && npm run build >/dev/null 2>&1 )
    rebuilt="$(cd "${WT}/web/dist" && find . -type f | sort)"
    if [ "${committed}" != "${rebuilt}" ]; then
        echo "FAIL: committed web/dist does not match a build of this commit's web/src." >&2
        echo "      Run 'cd web && npm run build' and commit web/dist." >&2
        diff <(printf '%s\n' "${committed}") <(printf '%s\n' "${rebuilt}") | head -20 >&2
        exit 1
    fi
    echo "    committed assets match a fresh build"
fi

echo "==> gofmt"
fmt="$(cd "${WT}" && gofmt -l internal/ cmd/)"
[ -z "${fmt}" ] || { echo "gofmt would reformat:"; echo "${fmt}"; exit 1; }

if [ -z "${SKIP_WEB_FRESHNESS:-}" ] && command -v npm >/dev/null 2>&1; then
    echo "==> frontend check / lint / unit tests"
    ( cd "${WT}/web" && npm run check >/dev/null && npm run lint >/dev/null && npm test >/dev/null )
fi

# The build the auto-updater actually runs. This is what failed for v1.18.0.
echo "==> server image (the build the updater runs)"
( cd "${WT}" && docker build -q -f deploy/Dockerfile.server \
    --build-arg STYRMAN_VERSION=verify -t "${IMG}" . >/dev/null )

echo
echo "OK: commit ${SHA:0:12} builds and tests clean in isolation — safe to tag."
