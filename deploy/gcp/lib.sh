# shellcheck shell=bash
# deploy/gcp/lib.sh — shared by vm.sh and install.sh (never run directly):
# logging, the run() dry-run seam, vm.conf loading and validation, gcloud
# lookup, and the instance/ssh helpers. Same shape as deploy/styrman-ctl.

log()  { echo "==> $*"; }
warn() { echo "==> WARNING: $*" >&2; }
die()  { echo "${PROG}: ERROR $*" >&2; exit 1; }
# Every side effect goes through run(): --dry-run prints and does nothing.
run()  { if [ "${DRY}" = 1 ]; then echo "+ $*"; else "$@"; fi; }
has_tty() { ( : < /dev/tty ) 2>/dev/null; }

DRY=0
# shellcheck disable=SC2034  # set here, read by vm.sh's create
YES=0
GCP_LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# load_gcp_env: vm.conf sets a key only when it is unset, so a GCP_* value
# already in the environment wins (a plain `source` would clobber it).
load_gcp_env() {
    local file="${GCP_LIB_DIR}/vm.conf" line key val
    [ -f "${file}" ] || die "${file}: no such file"
    while IFS= read -r line || [ -n "${line}" ]; do
        line="${line%%#*}"
        line="${line%"${line##*[![:space:]]}"}"
        [ -n "${line}" ] || continue
        key="${line%%=*}"; val="${line#*=}"
        [[ "${key}" =~ ^GCP_[A-Z_]+$ ]] || die "vm.conf: not a GCP_* assignment: ${line}"
        [ -n "${!key:-}" ] || printf -v "${key}" '%s' "${val}"
    done < "${file}"
    # Every value reaches a gcloud command line: validate before any call.
    local name
    for name in GCP_INSTANCE GCP_ADDRESS; do
        [[ "${!name:-}" =~ ^[a-z]([-a-z0-9]{0,61}[a-z0-9])?$ ]] || die "${name}='${!name:-}' is not a valid GCP resource name"
    done
    for name in GCP_PROJECT GCP_ZONE GCP_SUBNET GCP_MACHINE_TYPE GCP_BOOT_DISK_TYPE GCP_IMAGE_FAMILY GCP_IMAGE_PROJECT; do
        [[ "${!name:-}" =~ ^[a-z0-9-]+$ ]] || die "${name}='${!name:-}' is not a valid value (lowercase letters, digits, dashes)"
    done
    [[ "${GCP_TAGS:-}" =~ ^[a-z0-9,-]+$ ]] || die "GCP_TAGS='${GCP_TAGS:-}' is not a comma-separated tag list"
    [[ "${GCP_BOOT_DISK_SIZE:-}" =~ ^[0-9]+GB$ ]] || die "GCP_BOOT_DISK_SIZE='${GCP_BOOT_DISK_SIZE:-}' must look like 200GB"
    # shellcheck disable=SC2034  # read by vm.sh (addresses live in the zone's region)
    GCP_REGION="${GCP_ZONE%-*}"
    GCP_ARGS=(--project="${GCP_PROJECT}")
    ZONE_ARGS=(--zone="${GCP_ZONE}")
    [[ "${GCP_SSH_VIA_IAP:-1}" =~ ^[01]$ ]] || die "GCP_SSH_VIA_IAP must be 0 or 1"
    SSH_ARGS=(); [ "${GCP_SSH_VIA_IAP:-1}" = 0 ] || SSH_ARGS=(--tunnel-through-iap)
}

# need_gcloud: on PATH, else the SDK's default install location.
need_gcloud() {
    command -v gcloud >/dev/null 2>&1 && return 0
    [ -x "${HOME}/google-cloud-sdk/bin/gcloud" ] && { PATH="${PATH}:${HOME}/google-cloud-sdk/bin"; return 0; }
    die "gcloud not found on PATH or at ~/google-cloud-sdk/bin (install the SDK and run gcloud auth login — the gcloud-setup skill)"
}

# instance_state → "STATUS<TAB>EXTERNAL_IP" (IP may be empty); fails when
# the instance does not exist.
instance_state() {
    gcloud compute instances describe "${GCP_INSTANCE}" "${GCP_ARGS[@]}" "${ZONE_ARGS[@]}" \
        --format='value(status,networkInterfaces[0].accessConfigs[0].natIP)' 2>/dev/null
}

# vm_ssh [CMD]: no CMD → an interactive shell; CMD → run that one command
# string on the VM (gcloud adds this machine's key to project metadata on
# first use). A read, never wrapped in run(): callers that mutate wrap it.
vm_ssh() {
    if [ $# -eq 0 ]; then
        gcloud compute ssh "${GCP_INSTANCE}" "${GCP_ARGS[@]}" "${ZONE_ARGS[@]}" "${SSH_ARGS[@]}" --quiet
    else
        gcloud compute ssh "${GCP_INSTANCE}" "${GCP_ARGS[@]}" "${ZONE_ARGS[@]}" "${SSH_ARGS[@]}" --quiet \
            --ssh-flag='-o ConnectTimeout=15' --command="$*"
    fi
}

# ssh_reachable: one `true` over ssh — the check every diagnosis starts with,
# so an unreachable VM is never reported as "not installed" or "no KVM"
# (the first bring-up did exactly that, BUG record in the plan).
ssh_reachable() { vm_ssh true >/dev/null 2>&1; }
ssh_hint() { echo "gcloud compute ssh ${GCP_INSTANCE} ${GCP_ARGS[*]} ${ZONE_ARGS[*]} ${SSH_ARGS[*]} --troubleshoot"; }
