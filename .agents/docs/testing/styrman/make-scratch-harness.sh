#!/usr/bin/env bash
# make-scratch-harness.sh <outdir> <image> [<tag>] — a scratch harness
# channel (ADR-073) for Tier 2 and for hand runs, built the way
# internal/config/styrman_harness_test.go builds its fixture: <outdir>/origin
# is a git repo initialised from image/harness/ (minus crew's own COMMIT,
# SHA256SUMS, VENDORED.md and .agents), <tag> (default v1.0.0) is signed on
# it with a scratch ed25519 key whose public half is the one line of
# <outdir>/allowed_signers, <outdir>/clone is the channel's own clone, and
# `styrman-update harness` runs in test mode (STYRMAN_UPDATE_TEST=1, every
# path under <outdir>, unelevated, the smoke in <image>) against
# <outdir>/{state,clone,content}. A second call with a NEW tag on an
# existing <outdir> appends one line to the origin's README.md, signs the
# tag and runs the channel again — the swap Tier 2 exercises between a stop
# and a start. The last stdout line is the content dir's `current` path
# (what STYRMAN_HARNESS_DIR's `current` resolves to); everything else goes
# to stderr. Needs git, ssh-keygen, docker and the image's pinned Go builder
# image locally (the export builds sh-agent offline in it).
set -euo pipefail
OUT="${1:?usage: make-scratch-harness.sh <outdir> <image> [<tag>]}"
IMAGE="${2:?usage: make-scratch-harness.sh <outdir> <image> [<tag>]}"
TAG="${3:-v1.0.0}"
case "${OUT}" in /*) ;; *) echo "make-scratch-harness: <outdir> must be absolute" >&2; exit 2 ;; esac
case "${OUT}" in /|/etc|/etc/*|/var/lib/styrman|/var/lib/styrman/*|/opt|/opt/*) echo "make-scratch-harness: refusing a production path" >&2; exit 2 ;; esac
REPO="$(cd "$(dirname "$0")/../../../.." && pwd)"
MIRROR="${REPO}/image/harness"
SCRIPT="${REPO}/deploy/update/styrman-update"
ORIGIN="${OUT}/origin"; CLONE="${OUT}/clone"; CONTENT="${OUT}/content"; STATE="${OUT}/state/update"; SCRATCH_HOME="${OUT}/home"
export HOME="${SCRATCH_HOME}" GIT_CONFIG_GLOBAL="${SCRATCH_HOME}/gitconfig" GIT_CONFIG_NOSYSTEM=1
export STYRMAN_UPDATE_TEST=1 STYRMAN_UPDATE_STATE_DIR="${STATE}" STYRMAN_HARNESS_REPO_DIR="${CLONE}" \
    STYRMAN_HARNESS_DIR="${CONTENT}" STYRMAN_DEPLOY_DIR="${REPO}" STYRMAN_HARNESS_SMOKE_IMAGE="${IMAGE}" \
    STYRMAN_ALLOWED_SIGNERS="${OUT}/allowed_signers"
KEY="${SCRATCH_HOME}/.ssh/id_scratch"
g() { git -c user.name=t -c user.email=t@example.test -c gpg.format=ssh -c "user.signingkey=${KEY}.pub" "$@"; }
if [ ! -d "${ORIGIN}/.git" ]; then
    mkdir -p "${ORIGIN}" "${SCRATCH_HOME}/.ssh"
    chmod 700 "${SCRATCH_HOME}/.ssh"
    tar -C "${MIRROR}" --exclude=./.agents --exclude=./COMMIT --exclude=./SHA256SUMS --exclude=./VENDORED.md -cf - . | tar -C "${ORIGIN}" -xf -
    ssh-keygen -q -t ed25519 -N '' -f "${KEY}" >&2
    # verify_tag refuses a group/world-writable signers file even in test mode.
    ( umask 077; printf 't@example.test %s\n' "$(cat "${KEY}.pub")" > "${OUT}/allowed_signers" )
    g -C "${ORIGIN}" init -q -b main
    g -C "${ORIGIN}" add -A
    g -C "${ORIGIN}" commit -q -m "scratch harness base"
else
    printf '\n<!-- scratch %s -->\n' "${TAG}" >> "${ORIGIN}/README.md"
    g -C "${ORIGIN}" add -A
    g -C "${ORIGIN}" commit -q -m "scratch ${TAG}"
fi
g -C "${ORIGIN}" tag -s "${TAG}" -m "styrman-harness ${TAG}"
[ -d "${CLONE}/.git" ] || git clone -q "${ORIGIN}" "${CLONE}" >&2
bash "${SCRIPT}" harness >&2
[ -L "${CONTENT}/current" ] || { echo "make-scratch-harness: the channel did not deliver ${TAG} (no ${CONTENT}/current)" >&2; exit 1; }
GOT="$(cat "${CONTENT}/current/repo/TAG")"
[ "${GOT}" = "${TAG}" ] || { echo "make-scratch-harness: current is ${GOT}, not ${TAG}" >&2; exit 1; }
echo "${CONTENT}/current"
