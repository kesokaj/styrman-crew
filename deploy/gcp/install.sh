#!/usr/bin/env bash
# deploy/gcp/install.sh — install styrman on the VM deploy/gcp/vm.sh created,
# by the SAME path the NUC uses: clone this repo at a release tag on the VM,
# stage the three secret files there, run `styrman-ctl install --yes …`
# (host-setup.sh, the six .env keys, runtime image, stack, auto-updater),
# verify, and remove the staged secrets. Plan
# .agents/docs/plans/2026-09-21-gcp-compute-engine-target.md, ADR-075.
#
#   deploy/gcp/install.sh [--dry-run] [--secrets-dir D] [--version X.Y.Z] [--user U]
#
# --secrets-dir (default ~/.config/styrman/gcp, mode 700) holds three files,
# each mode 600 and owned by you:
#   env              the six must-fill keys of deploy/.env.example (BASE_DOMAIN,
#                    GOOGLE_CLIENT_ID, GOOGLE_CLIENT_SECRET, CLOUDFLARE_API_TOKEN,
#                    ADMIN_EMAIL, STYRMAN_ALLOWED_EMAILS) + optional STYRMAN_HOSTNAME
#   gh-token         a fine-grained GitHub token that reads this repository AND
#                    kesokaj/styrman-harness (the image build, ADR-074)
#   allowed_signers  one line: <git email> <key type> <public key> of the release signer
# --version defaults to this checkout's newest release tag (never a sha, ADR-053);
# --user to your local login name (what gcloud compute ssh logs in as).
# Secret VALUES never appear in output or in a local argv: the files travel
# by scp; the token reaches git on the VM through a file-reading helper.
set -euo pipefail
shopt -s inherit_errexit

# shellcheck disable=SC2034  # read by lib.sh's die()
PROG=install.sh
# shellcheck source=lib.sh
. "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib.sh"
VM="${GCP_LIB_DIR}/vm.sh"
REPO="$(cd "${GCP_LIB_DIR}/../.." && pwd)"

usage() { sed -n '/^#   deploy\/gcp\/install.sh/,/^set -euo/p' "${BASH_SOURCE[0]}" | grep '^#' | sed 's/^# \{0,3\}//'; }

SECRETS="${HOME}/.config/styrman/gcp"
VERSION=""
USER_NAME="${USER:-$(id -un)}"
while [ $# -gt 0 ]; do
    case "$1" in
        --dry-run) DRY=1; shift ;;
        --secrets-dir) [ $# -ge 2 ] || die "--secrets-dir needs a path"; SECRETS="$2"; shift 2 ;;
        --version) [ $# -ge 2 ] || die "--version needs X.Y.Z"; VERSION="$2"; shift 2 ;;
        --user) [ $# -ge 2 ] || die "--user needs a name"; USER_NAME="$2"; shift 2 ;;
        help|-h|--help) usage; exit 0 ;;
        *) usage; die "unknown argument $1" ;;
    esac
done

# ─── Inputs, validated before anything is called ────────────────────────────
[[ "${USER_NAME}" =~ ^[a-z_][a-z0-9_-]{0,31}$ ]] || die "--user '${USER_NAME}' is not a login name"
if [ -z "${VERSION}" ]; then
    VERSION="$(git -C "${REPO}" describe --tags --abbrev=0 2>/dev/null || true)"
    VERSION="${VERSION#v}"
fi
[[ "${VERSION}" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || die "no release version: pass --version X.Y.Z (a commit sha is never a version, ADR-053)"

# private_file / private_dir: the checks styrman-ctl's private_file makes.
private_file() {
    local f="$1" mode owner pmode
    [ -L "${f}" ] && die "${f}: a symlink is refused"
    [ -f "${f}" ] || die "${f}: no such file (see: deploy/gcp/install.sh help)"
    owner="$(stat -c %U "${f}" 2>/dev/null || echo ?)"
    [ "${owner}" = "$(id -un)" ] || die "${f} is owned by ${owner}, not you"
    mode="$(stat -c %a "${f}" 2>/dev/null || echo 777)"
    [ "$(( 8#${mode} & 8#077 ))" = 0 ] || die "${f} is readable by group or others (mode ${mode}); chmod 600 it first"
    pmode="$(stat -c %a "$(dirname -- "${f}")" 2>/dev/null || echo 777)"
    [ "$(( 8#${pmode} & 8#077 ))" = 0 ] || die "$(dirname -- "${f}") must be mode 700 (it is ${pmode}); a secret file must not sit in a shared directory"
}
[ -L "${SECRETS}" ] && die "${SECRETS}: a symlink is refused"
[ -d "${SECRETS}" ] || die "${SECRETS}: no such directory — create it (mkdir -m 700) with env, gh-token and allowed_signers (deploy/gcp/install.sh help)"
ENV_FILE="${SECRETS}/env"; TOKEN_FILE="${SECRETS}/gh-token"; SIGNERS_FILE="${SECRETS}/allowed_signers"
private_file "${ENV_FILE}"; private_file "${TOKEN_FILE}"; private_file "${SIGNERS_FILE}"

env_get() { local v; v="$(grep -E "^$2=" "$1" | tail -n1 || true)"; printf '%s' "${v#*=}"; }
for key in BASE_DOMAIN GOOGLE_CLIENT_ID GOOGLE_CLIENT_SECRET CLOUDFLARE_API_TOKEN ADMIN_EMAIL STYRMAN_ALLOWED_EMAILS; do
    [ -n "$(env_get "${ENV_FILE}" "${key}")" ] || die "${ENV_FILE}: ${key} is empty or missing"
done
BASE_DOMAIN="$(env_get "${ENV_FILE}" BASE_DOMAIN)"
HOSTNAME_KEY="$(env_get "${ENV_FILE}" STYRMAN_HOSTNAME)"; HOSTNAME_KEY="${HOSTNAME_KEY:-styrman}"
[[ "${BASE_DOMAIN}" =~ ^[a-z0-9.-]+$ ]] || die "${ENV_FILE}: BASE_DOMAIN '${BASE_DOMAIN}' is not a hostname"
[[ "${HOSTNAME_KEY}" =~ ^[a-z0-9-]+$ ]] || die "${ENV_FILE}: STYRMAN_HOSTNAME '${HOSTNAME_KEY}' is not a label"
[ "$(grep -c '' "${SIGNERS_FILE}")" -le 1 ] || die "${SIGNERS_FILE}: one line expected"

# The VM clones this checkout's origin over https (a fork installs itself);
# only a plain https URL reaches the remote git (the styrman-ctl rule).
ORIGIN="$(git -C "${REPO}" remote get-url origin 2>/dev/null || true)"
[ -n "${ORIGIN}" ] || die "this checkout has no git remote 'origin'"
ORIGIN="$(printf '%s' "${ORIGIN}" | sed -E 's#^git@([^:]+):#https://\1/#; s#^ssh://git@([^/]+)/#https://\1/#; s#\.git$##')"
[[ "${ORIGIN}" =~ ^https://[a-z0-9.-]+/[A-Za-z0-9._/-]+$ ]] || die "origin ${ORIGIN} is not a plain https URL"
ORIGIN_HOST="${ORIGIN#https://}"; ORIGIN_HOST="https://${ORIGIN_HOST%%/*}"

load_gcp_env
need_gcloud

# ─── 1. The VM must be running, with KVM ────────────────────────────────────
log "install: ${GCP_INSTANCE} in ${GCP_ZONE} — styrman ${VERSION} from ${ORIGIN}, user ${USER_NAME}"
if "${VM}" status >/dev/null 2>&1; then rc=0; else rc=$?; fi
case "${rc}" in
    0|1) ;;
    2) die "no instance ${GCP_INSTANCE}: run deploy/gcp/vm.sh create first" ;;
    3) die "${GCP_INSTANCE} exists but is not running: run deploy/gcp/vm.sh start first" ;;
    *) die "deploy/gcp/vm.sh status failed (${rc})" ;;
esac
ssh_reachable || die "ssh to ${GCP_INSTANCE} failed — $(ssh_hint)"
# host-setup.sh only WARNS when /dev/kvm is missing but the CPU shows vmx:
# refuse here, before anything is staged.
vm_ssh 'test -e /dev/kvm' >/dev/null 2>&1 \
    || die "no /dev/kvm on ${GCP_INSTANCE}: nested virtualization is off or unsupported for GCP_MACHINE_TYPE=${GCP_MACHINE_TYPE} — recreate with a supported Intel type (vm.sh delete --confirm ${GCP_INSTANCE}; GCP_MACHINE_TYPE=… vm.sh create)"

# ─── 2. Tools the clone and styrman-ctl need ────────────────────────────────
log "install: git, curl, python3 on the VM"
run "${VM}" ssh -- 'sudo apt-get update -q && sudo apt-get install -y -q git curl python3'

# ─── 3. Stage the secrets (removed again whatever happens below) ────────────
STAGED=0
cleanup() {
    local rc=$?
    if [ "${STAGED}" = 1 ]; then
        log "install: removing the staged secrets from the VM"
        run "${VM}" ssh -- 'rm -rf "$HOME/.styrman-install"' \
            || warn "the staged secrets are STILL ON THE VM: remove them with  deploy/gcp/vm.sh ssh -- 'rm -rf \"\$HOME/.styrman-install\"'"
    fi
    exit "${rc}"
}
trap cleanup EXIT INT TERM HUP
log "install: staging env, gh-token, allowed_signers → ~/.styrman-install on the VM"
run "${VM}" ssh -- 'install -d -m 700 "$HOME/.styrman-install"'
STAGED=1
run gcloud compute scp "${ENV_FILE}" "${TOKEN_FILE}" "${SIGNERS_FILE}" "${GCP_INSTANCE}:.styrman-install/" "${GCP_ARGS[@]}" "${ZONE_ARGS[@]}" "${SSH_ARGS[@]}" --quiet
run "${VM}" ssh -- 'chmod 600 "$HOME"/.styrman-install/*'

# ─── 4. The checkout on the VM, at the release tag ──────────────────────────
# The token reaches git the way styrman-ctl's setup_auto_update does it: a
# helper scoped to the origin's host, every inherited helper cleared, and
# the token-file PATH on the env line of the one git call — never
# interpolated into the snippet the helper's shell parses (security review
# 2026-09-16). The helper is single-quoted for the VM's shell.
sq() { local q="'" bs='\'; printf "'%s'" "${1//$q/$q$bs$q$q}"; }
HELPER="credential.${ORIGIN_HOST}.helper=!f() { echo username=x-access-token; printf 'password=%s\\n' \"\$(cat \"\${STYRMAN_GH_TOKEN_FILE}\")\"; }; f"
GIT_ENV='env GIT_TERMINAL_PROMPT=0 STYRMAN_GH_TOKEN_FILE="$HOME/.styrman-install/gh-token"'
GIT_C="git -c credential.helper= -c $(sq "${HELPER}")"
if vm_ssh 'test -d "$HOME/styrman-crew/.git"' >/dev/null 2>&1; then
    # reset --hard + clean: `checkout` alone keeps local edits and untracked
    # files, so the verify-tag below would vouch for the tag object while the
    # tree that runs as root is something else (review 2026-09-22).
    log "install: ~/styrman-crew exists on the VM — fetching tags, resetting it to v${VERSION}"
    run "${VM}" ssh -- "${GIT_ENV} ${GIT_C} -C \"\$HOME/styrman-crew\" fetch --tags && git -C \"\$HOME/styrman-crew\" reset -q --hard \"v${VERSION}\" && git -C \"\$HOME/styrman-crew\" clean -qfdx"
else
    log "install: cloning ${ORIGIN} at v${VERSION} → ~/styrman-crew on the VM"
    run "${VM}" ssh -- "${GIT_ENV} ${GIT_C} clone --branch v${VERSION} -- ${ORIGIN} \"\$HOME/styrman-crew\""
fi

# ─── 5. The tag is signed by the release signer, then styrman-ctl install ───
# Only a release the staged allowed_signers vouches for runs on the VM — the
# updater's rule (ADR-050/051), applied to the first install too.
log "install: verifying the v${VERSION} tag signature against the staged allowed_signers"
# gpg.format and gpg.ssh.program are pinned on the command line: the checkout
# is writable by the VM user, and a repo-local config could otherwise point
# the verifier at another program (security review 2026-09-22).
run "${VM}" ssh -- "git -C \"\$HOME/styrman-crew\" -c gpg.format=ssh -c gpg.ssh.program=ssh-keygen -c gpg.ssh.allowedSignersFile=\"\$HOME/.styrman-install/allowed_signers\" verify-tag \"v${VERSION}\""
# The token reaches host-setup.sh's image build through --gh-token-file:
# styrman-ctl exports it in its own process, so no value is ever on a
# command line sudo logs (security review 2026-09-21).
log "install: sudo styrman-ctl install --yes on the VM (host-setup.sh, image build, stack, auto-updater — 10-20 minutes)"
run "${VM}" ssh -- "cd \"\$HOME/styrman-crew\" && sudo ./deploy/styrman-ctl install --yes --user ${USER_NAME} --version ${VERSION} --env-file \"\$HOME/.styrman-install/env\" --gh-token-file \"\$HOME/.styrman-install/gh-token\" --allowed-signers-file \"\$HOME/.styrman-install/allowed_signers\""

# ─── 6. Verify from here ────────────────────────────────────────────────────
URL="https://${HOSTNAME_KEY}.${BASE_DOMAIN}/"
log "install: verifying ${URL} answers with the sign-in redirect (the ingress may take a few minutes to obtain its certificate)"
if [ "${DRY}" = 1 ]; then
    run curl -sS -o /dev/null -w '%{http_code} %{redirect_url}\n' "${URL}"
else
    for i in $(seq 1 18); do
        out="$(curl -sS -o /dev/null -w '%{http_code} %{redirect_url}' --max-time 15 "${URL}" 2>/dev/null || true)"
        case "${out}" in
            302*"//auth.${BASE_DOMAIN}/"*) break ;;
        esac
        [ "${i}" -lt 18 ] && sleep 10
    done
    case "${out}" in
        302*"//auth.${BASE_DOMAIN}/"*) ;;
        *) die "${URL} did not redirect to auth.${BASE_DOMAIN} after 3 minutes (last: '${out:-no answer}'); the stack may still be starting — deploy/gcp/vm.sh status, and docker logs styrman-caddy on the VM" ;;
    esac
fi
log "install: done — ${URL} (Google sign-in via https://auth.${BASE_DOMAIN}; the OAuth client must list https://auth.${BASE_DOMAIN}/oauth2/google/authorization-code-callback as a redirect URI)"
