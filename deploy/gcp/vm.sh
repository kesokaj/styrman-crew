#!/usr/bin/env bash
# deploy/gcp/vm.sh — the styrman GCP VM's lifecycle through gcloud (plan
# .agents/docs/plans/2026-09-21-gcp-compute-engine-target.md, ADR-075).
#
#   deploy/gcp/vm.sh [--dry-run] [--yes] create      reserve the static IP + create the VM (nested virt on); no-op when it exists
#   deploy/gcp/vm.sh [--dry-run] status             exit 0 running+healthy, 1 running but styrman degraded/not installed,
#                                                   2 no instance, 3 instance exists but is not RUNNING
#   deploy/gcp/vm.sh [--dry-run] ssh [-- CMD…]      interactive shell, or one command
#   deploy/gcp/vm.sh [--dry-run] stop | start       pay for the disk only while stopped
#   deploy/gcp/vm.sh [--dry-run] delete --confirm <GCP_INSTANCE>   instance, boot disk and static IP — irreversible
#   deploy/gcp/vm.sh help
#
# Defaults come from vm.conf beside this script; a GCP_* variable already in
# the environment wins. The VM itself is disposable (durability is git +
# deploy/gcp/install.sh, as on the NUC). --dry-run prints every command that
# would change something as "+ cmd" and runs none of them; reads still run so
# the plan is the real one. --yes answers create's cost confirmation; delete
# is gated by --confirm alone.
set -euo pipefail
shopt -s inherit_errexit

PROG=vm.sh
# shellcheck source=lib.sh
. "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib.sh"

usage() {
    sed -n '2,/^set -euo/p' "${BASH_SOURCE[0]}" | grep -E '^#( {3}deploy/gcp/vm\.sh| {3,}[0-9])' | sed 's/^# \{0,3\}//'
}

CONFIRM_GIVEN=""
VERB=""
ARGS=()
while [ $# -gt 0 ]; do
    case "$1" in
        --dry-run) DRY=1; shift ;;
        --yes) YES=1; shift ;;
        --confirm) [ $# -ge 2 ] || die "--confirm needs a word"; CONFIRM_GIVEN="$2"; shift 2 ;;
        --) shift; ARGS+=("$@"); break ;;
        -*) die "unknown flag $1 (see: deploy/gcp/vm.sh help)" ;;
        *) if [ -z "${VERB}" ]; then VERB="$1"; else ARGS+=("$1"); fi; shift ;;
    esac
done
[ -n "${VERB}" ] || { usage; exit 1; }
case "${VERB}" in help|-h|--help) usage; exit 0 ;; esac

load_gcp_env
need_gcloud

cmd_create() {
    local st
    if st="$(instance_state)"; then
        log "instance ${GCP_INSTANCE} already exists in ${GCP_ZONE}: ${st//$'\t'/ } — nothing to do"
        return 0
    fi
    # The one prompt --yes answers: creating bills from the first minute. A
    # dry run obeys the same rule so it shows exactly what an unattended run
    # would do.
    if [ "${YES}" != 1 ]; then
        has_tty || die "no terminal to confirm on: pass --yes to create unattended (billed while running)"
        local a
        read -r -p "Create ${GCP_INSTANCE} (${GCP_MACHINE_TYPE}, ${GCP_BOOT_DISK_SIZE} ${GCP_BOOT_DISK_TYPE}) in ${GCP_ZONE}: billed while running (about USD 250-300/month for n4-standard-8) — continue? [y/N] " a < /dev/tty
        [[ "${a}" =~ ^[yY] ]] || die "aborted, nothing created"
    fi
    assert_no_public_ssh
    if gcloud compute addresses describe "${GCP_ADDRESS}" "${GCP_ARGS[@]}" --region="${GCP_REGION}" --format='value(address)' >/dev/null 2>&1; then
        log "address ${GCP_ADDRESS} already reserved in ${GCP_REGION}"
    else
        log "reserving external address ${GCP_ADDRESS} in ${GCP_REGION}"
        run gcloud compute addresses create "${GCP_ADDRESS}" "${GCP_ARGS[@]}" --region="${GCP_REGION}" --network-tier=PREMIUM
    fi
    log "creating ${GCP_INSTANCE} (${GCP_MACHINE_TYPE}, nested virtualization on, no service account)"
    run gcloud compute instances create "${GCP_INSTANCE}" "${GCP_ARGS[@]}" "${ZONE_ARGS[@]}" \
        --machine-type="${GCP_MACHINE_TYPE}" --enable-nested-virtualization \
        --image-family="${GCP_IMAGE_FAMILY}" --image-project="${GCP_IMAGE_PROJECT}" \
        --boot-disk-size="${GCP_BOOT_DISK_SIZE}" --boot-disk-type="${GCP_BOOT_DISK_TYPE}" \
        --subnet="${GCP_SUBNET}" --address="${GCP_ADDRESS}" --tags="${GCP_TAGS}" \
        --no-service-account --no-scopes \
        --shielded-secure-boot --shielded-vtpm --shielded-integrity-monitoring \
        --metadata=enable-oslogin=FALSE,block-project-ssh-keys=TRUE --labels=app=styrman
    if [ "${DRY}" = 1 ]; then
        echo "+ (dry run) would wait for ssh and print the external IP"
        return 0
    fi
    log "waiting for ssh (up to 5 minutes; gcloud adds this machine's key to the INSTANCE's metadata on first use — project-wide keys are blocked)"
    local i
    for i in $(seq 1 30); do
        if ssh_reachable; then
            st="$(instance_state)"
            log "ready: ${GCP_INSTANCE} ${st//$'\t'/ } — next: deploy/gcp/install.sh"
            return 0
        fi
        [ "${i}" -lt 30 ] && sleep 10
    done
    die "ssh never came up: gcloud compute instances get-serial-port-output ${GCP_INSTANCE} ${GCP_ARGS[*]} ${ZONE_ARGS[*]}"
}

cmd_status() {
    local st status ip
    if ! st="$(instance_state)"; then
        echo "no instance ${GCP_INSTANCE} in ${GCP_ZONE} (${GCP_PROJECT}) — deploy/gcp/vm.sh create"
        return 2
    fi
    status="${st%%$'\t'*}"; ip="${st#*$'\t'}"
    echo "instance ${GCP_INSTANCE}  ${status}  ${ip:-no external IP}  (${GCP_ZONE}, ${GCP_PROJECT})"
    [ "${status}" = RUNNING ] || return 3
    if ! ssh_reachable; then
        echo "ssh to ${GCP_INSTANCE} unreachable — $(ssh_hint)"
        return 1
    fi
    # styrman-ctl is never installed into PATH: it lives in the deploy clone
    # the updater maintains, and in the operator's checkout. Probing
    # /usr/local/bin/styrman-ctl reported "not installed" on a host that was
    # serving (BUG-073) — ask the host which copy it has.
    local ctl
    ctl="$(vm_ssh 'for p in /opt/styrman/deploy/styrman-ctl "$HOME/styrman-crew/deploy/styrman-ctl" /usr/local/bin/styrman-ctl; do [ -x "$p" ] && { echo "$p"; break; }; done' 2>/dev/null | tr -d '\r' | head -n1)"
    if [ -z "${ctl}" ]; then
        echo "styrman not installed on ${GCP_INSTANCE} — deploy/gcp/install.sh"
        return 1
    fi
    # The remote's answer reaches a sudo command line and is re-parsed by the
    # remote shell, so what is accepted must contain no shell metacharacter.
    # The two fixed paths are compared literally; the home one is the only
    # variable part and its username is bounded to a POSIX-ish class --
    # `case`'s old /home/*/... arm accepted a `*`, which matches `/`, a
    # space, a quote and `$(` alike (security review 2026-09-22).
    case "${ctl}" in
        /opt/styrman/deploy/styrman-ctl|/usr/local/bin/styrman-ctl) ;;
        *)
            [[ "${ctl}" =~ ^/root/styrman-crew/deploy/styrman-ctl$ ]] ||
            [[ "${ctl}" =~ ^/home/[a-z_][a-z0-9_-]{0,31}/styrman-crew/deploy/styrman-ctl$ ]] ||
                die "${GCP_INSTANCE} answered with an unexpected styrman-ctl path: ${ctl}"
            ;;
    esac
    if vm_ssh "sudo ${ctl} status"; then return 0; fi
    return 1
}

cmd_delete() {
    local st
    if ! st="$(instance_state)"; then
        log "no instance ${GCP_INSTANCE} in ${GCP_ZONE}; the address ${GCP_ADDRESS} is released if still reserved"
    else
        log "instance ${GCP_INSTANCE} is ${st%%$'\t'*}"
    fi
    echo "This removes the instance ${GCP_INSTANCE}, its boot disk (every project home, workspace and user store on it) and releases ${GCP_ADDRESS}. Irreversible."
    if [ -n "${CONFIRM_GIVEN}" ]; then
        [ "${CONFIRM_GIVEN}" = "${GCP_INSTANCE}" ] || { echo "${PROG}: --confirm ${CONFIRM_GIVEN} does not match ${GCP_INSTANCE} — aborted, nothing changed" >&2; return 2; }
    else
        has_tty || { echo "${PROG}: no terminal to confirm on; pass --confirm ${GCP_INSTANCE} to delete unattended — nothing changed" >&2; return 2; }
        local typed
        read -r -p "Type ${GCP_INSTANCE} to continue: " typed < /dev/tty
        [ "${typed}" = "${GCP_INSTANCE}" ] || { echo "${PROG}: aborted, nothing changed" >&2; return 2; }
    fi
    if [ -n "${st}" ]; then
        run gcloud compute instances delete "${GCP_INSTANCE}" "${GCP_ARGS[@]}" "${ZONE_ARGS[@]}" --quiet
    fi
    if gcloud compute addresses describe "${GCP_ADDRESS}" "${GCP_ARGS[@]}" --region="${GCP_REGION}" --format='value(address)' >/dev/null 2>&1; then
        run gcloud compute addresses delete "${GCP_ADDRESS}" "${GCP_ARGS[@]}" --region="${GCP_REGION}" --quiet
    fi
    log "deleted"
}

case "${VERB}" in
    create) cmd_create ;;
    status) cmd_status ;;
    ssh)    if [ "${DRY}" = 1 ]; then
                if [ ${#ARGS[@]} -gt 0 ]; then
                    printf '+ gcloud compute ssh %s %s %s %s --quiet --command=%q\n' "${GCP_INSTANCE}" "${GCP_ARGS[*]}" "${ZONE_ARGS[*]}" "${SSH_ARGS[*]}" "${ARGS[*]}"
                else
                    printf '+ gcloud compute ssh %s %s %s %s --quiet\n' "${GCP_INSTANCE}" "${GCP_ARGS[*]}" "${ZONE_ARGS[*]}" "${SSH_ARGS[*]}"
                fi
            else vm_ssh "${ARGS[@]}"; fi ;;
    stop)   run gcloud compute instances stop "${GCP_INSTANCE}" "${GCP_ARGS[@]}" "${ZONE_ARGS[@]}" ;;
    start)  run gcloud compute instances start "${GCP_INSTANCE}" "${GCP_ARGS[@]}" "${ZONE_ARGS[@]}" ;;
    delete) cmd_delete ;;
    *) usage; die "unknown verb ${VERB}" ;;
esac
