#!/usr/bin/env bash
# vulncheck — run govulncheck for BOTH Go modules inside the SAME pinned Go
# image the server is built with, so the security gate measures the toolchain
# that ships, not whatever `go` the host happens to have (2026-09-02 security
# delta: the host's go1.25.0 reported 31 stdlib findings the shipped
# golang:1.25.13 build never had). Read-only source mount; the module cache
# persists in a named volume so re-runs are fast.
#
# Exit status is govulncheck's: 3 when a CALLED vulnerability exists in either
# module, 0 when clean, anything else = the scan itself failed. This is a
# REPORT for the security row, not a hard build gate: docker/docker carries
# daemon-side findings with no fix available (GO-2026-4887/4883), which the
# reviewer records as accepted per release.
#
# Usage: deploy/vulncheck.sh            (from the repo root)
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# Single source of truth: the builder line of the server Dockerfile.
IMAGE="$(sed -n 's/^FROM \(golang:[^ ]*\) AS builder$/\1/p' "${ROOT}/deploy/Dockerfile.server")"
[ -n "${IMAGE}" ] || { echo "vulncheck: cannot find the golang builder image in deploy/Dockerfile.server" >&2; exit 2; }
# Pinned tool version (operator rule: nothing floats).
GOVULN="golang.org/x/vuln/cmd/govulncheck@v1.7.0"
rc=0
r=0
# Shadow the one secret-bearing file in the tree so a container that downloads
# and runs a tool never sees the ingress token — but ONLY when it exists.
# Bind-mounting /dev/null over a path that is not in the tree fails in runc
# with exit 125, which made this script unrunnable on any clean checkout: a
# release gate that cannot run on CI or a fresh clone is worse than no gate
# (audit 2026-09-02). A checkout with no deploy/.env has no secret to hide.
ENV_SHADOW=()
if [ -f "${ROOT}/deploy/.env" ]; then
    ENV_SHADOW=(-v /dev/null:/src/deploy/.env:ro)
fi
for mod in . deploy/caddy/ingress; do
    echo "==> govulncheck ${mod} (in ${IMAGE})"
    # `go install` then run the binary — NOT `go run`, which collapses
    # govulncheck's exit 3 ("called vulnerability") into its own exit 1 and
    # makes a finding indistinguishable from a broken scan (review).
    # GOTOOLCHAIN=local: scan with the image's Go, never auto-switch to
    # whatever a go directive asks for — the stdlib measured must be the
    # stdlib shipped.
    # The repo is mounted read-only, and the one secret-bearing file in it
    # (the gitignored deploy/.env) is shadowed with /dev/null — a container
    # that downloads and runs a tool must never see the ingress token
    # (security review).
    docker run --rm \
        -v "${ROOT}:/src:ro" "${ENV_SHADOW[@]}" -w "/src/${mod}" \
        -v styrman-vulncheck-cache:/go/pkg/mod \
        -v styrman-vulncheck-gocache:/root/.cache \
        -e GOTOOLCHAIN=local \
        "${IMAGE}" sh -c "go install ${GOVULN} && govulncheck ./..." || r=$?
    # Worst wins across modules: a broken scan (anything but 0/3) must never be
    # masked by a later clean or found module, and a finding (3) never by a
    # later clean one (review).
    case "${r:-0}" in
        0) ;;
        3) [ "${rc}" -eq 0 ] && rc=3 ;;
        *) rc="${r}" ;;
    esac
    r=0
done
exit "${rc}"
