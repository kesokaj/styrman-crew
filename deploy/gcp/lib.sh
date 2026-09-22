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

# --- secrets and env files -------------------------------------------------
# These are the hardened versions from deploy/styrman-ctl. install.sh used to
# carry its own forks; env_get's had lost the is_key guard and matched with
# `grep -E "^$2="`, interpolating the key into a regex, and private_file's had
# drifted in two of five checks (2026-09-22 review). styrman-ctl cannot source
# this file -- it is the general host tool and this lib is GCP-specific -- so
# the duplication is deliberate, and internal/config/gcp_scripts_test.go
# asserts the two copies still agree rather than trusting them to.

is_key() { [[ "$1" =~ ^[A-Z][A-Z0-9_]*$ ]]; }

env_get() { # env_get FILE KEY → value ('' if absent); the LAST assignment wins, like compose
    local file="$1" key="$2"
    is_key "${key}" || die "not a key: ${key}"
    [ -f "${file}" ] || return 0
    # The key travels as an awk VARIABLE and is matched literally — never
    # interpolated into a program (security review: a key with a slash
    # changed a sed program; with GNU sed's e flag that is command execution).
    awk -v k="${key}" 'index($0, k "=") == 1 { v = substr($0, length(k) + 2) } END { if (v != "") print v }' "${file}"
}

# private_file FILE [OWNER_POLICY] [PARENT_MASK]
#
# OWNER_POLICY is `self` (the invoking user only, the default here) or
# `root-or-self` (also root or ${SUDO_USER}). styrman-ctl needs the latter
# because it runs under sudo; these scripts run as the operator, so they do
# not. That difference is deliberate, not drift.
#
# PARENT_MASK is the octal mask the parent directory's mode must not
# intersect. 022 -- not writable by group or other -- is the check that
# actually protects the file: a readable directory does not expose a 600 file,
# a writable one lets it be REPLACED. 077 additionally demands 700 and suits a
# directory that exists only to hold secrets, which is why these scripts pass
# it for ~/.config/styrman/gcp; styrman-ctl uses 022 because /etc/styrman is
# legitimately 755.
private_file() {
    local f="$1" owner_policy="${2:-self}" parent_mask="${3:-077}" mode owner pmode
    [ -L "${f}" ] && die "${f}: a symlink is refused"
    [ -f "${f}" ] || die "${f}: no such file (see: deploy/gcp/install.sh help)"
    owner="$(stat -c %U "${f}" 2>/dev/null || echo ?)"
    case "${owner_policy}" in
        root-or-self)
            [ "${owner}" = root ] || [ "${owner}" = "${SUDO_USER:-}" ] || [ "${owner}" = "$(id -un)" ] \
                || die "${f} is owned by ${owner}, not root or ${SUDO_USER:-you}" ;;
        self)
            [ "${owner}" = "$(id -un)" ] || die "${f} is owned by ${owner}, not you" ;;
        *) die "private_file: unknown owner policy ${owner_policy}" ;;
    esac
    mode="$(stat -c %a "${f}" 2>/dev/null || echo 777)"
    [ "$(( 8#${mode} & 8#077 ))" = 0 ] || die "${f} is readable by group or others (mode ${mode}); chmod 600 it first"
    pmode="$(stat -c %a "$(dirname -- "${f}")" 2>/dev/null || echo 777)"
    [ "$(( 8#${pmode} & 8#${parent_mask} ))" = 0 ] \
        || die "$(dirname -- "${f}") is too permissive (mode ${pmode}, must not intersect ${parent_mask}); a secret file must not sit in a shared directory"
}

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

# vm_ssh [CMD]: no CMD → an interactive shell; CMD → run that ONE command
# STRING on the VM — every argument is flattened into it, so a caller builds
# the string itself rather than passing a word list. gcloud puts this
# machine's key in the INSTANCE's metadata (create sets
# block-project-ssh-keys=TRUE, so project-wide keys are not used). A read,
# never wrapped in run(): callers that mutate wrap it.
vm_ssh() {
    if [ $# -eq 0 ]; then
        gcloud compute ssh "${GCP_INSTANCE}" "${GCP_ARGS[@]}" "${ZONE_ARGS[@]}" "${SSH_ARGS[@]}" --quiet
    else
        gcloud compute ssh "${GCP_INSTANCE}" "${GCP_ARGS[@]}" "${ZONE_ARGS[@]}" "${SSH_ARGS[@]}" --quiet \
            --ssh-flag='-o ConnectTimeout=15' --command="$*"
    fi
}

# assert_no_public_ssh: "no public port 22" is a property of the PROJECT, not
# of this script — omitting the `ssh` tag hides the VM from a tag-targeted
# rule, but a rule with NO target tags (a default VPC's `default-allow-ssh`)
# reaches every instance anyway. Refuse to create into such a project rather
# than claim a posture the project does not have (security review
# 2026-09-22). The check never fails SILENTLY: when it cannot run — no
# firewall list, no python3, a filter that errors — it refuses and names the
# override, because a quiet pass here is the failure this check exists to
# prevent (BUG-034's family; review round 2, 2026-09-22).
assert_no_public_ssh() {
    if [ "${GCP_ALLOW_PUBLIC_SSH:-0}" = 1 ]; then
        warn "GCP_ALLOW_PUBLIC_SSH=1 — NOT checking whether this project exposes ssh to the internet"
        return 0
    fi
    local json out rc exposed
    json="$(gcloud compute firewall-rules list "${GCP_ARGS[@]}" --format=json 2>/dev/null)" || json=""
    [ -n "${json}" ] || die "cannot list this project's firewall rules, so 'no public port 22' is unverified for ${GCP_INSTANCE} — grant compute.firewalls.list, or re-run with GCP_ALLOW_PUBLIC_SSH=1 to create without the check"
    # The filter prints one line: OK, then the names of the rules that reach
    # THIS instance on port 22 from the internet. Its exit status and that
    # sentinel are both read — a traceback or a missing python3 must not read
    # as "nothing exposed".
    if out="$(printf '%s' "${json}" | python3 -c '
import json, sys
tags = set(t for t in sys.argv[1].split(",") if t)
PUBLIC = {"0.0.0.0/0", "::/0"}
TCP = {"tcp", "all", "6"}          # the API accepts the protocol number too
def reaches_22(rule):
    for a in rule.get("allowed") or []:
        if str(a.get("IPProtocol", "")).lower() not in TCP:
            continue
        ports = a.get("ports")
        if not ports:                      # no ports key = every port
            return True
        for p in ports:
            lo, _, hi = str(p).partition("-")
            try:
                if int(lo) <= 22 <= int(hi or lo):
                    return True
            except ValueError:
                continue
    return False
names = []
for r in json.load(sys.stdin):
    if (r.get("direction") or "INGRESS").upper() != "INGRESS" or r.get("disabled"):
        continue
    if not (PUBLIC & set(r.get("sourceRanges") or [])):
        continue
    if r.get("targetServiceAccounts"):     # this instance runs with none
        continue
    targets = r.get("targetTags")
    if targets and not (set(targets) & tags):
        continue                           # aimed at other instances
    if reaches_22(r):
        names.append(str(r.get("name") or "?"))
print("OK " + " ".join(names))' "${GCP_TAGS}" 2>/dev/null)"; then rc=0; else rc=$?; fi
    case "${rc}:${out}" in
        0:OK*) ;;
        *) die "the ssh-exposure check could not run (python3 exit ${rc}); refusing rather than assuming this project is closed — re-run with GCP_ALLOW_PUBLIC_SSH=1 to create without the check" ;;
    esac
    exposed="${out#OK}"; exposed="${exposed# }"
    [ -z "${exposed}" ] || die "this project has a firewall rule that opens ssh to the internet for instances like ${GCP_INSTANCE} (rule: ${exposed}). It carries no 'ssh' tag and ssh goes through IAP, so port 22 is meant to be closed — delete or retarget that rule, or, if it does not reach this instance's network, re-run with GCP_ALLOW_PUBLIC_SSH=1."
}

# ssh_reachable: one `true` over ssh — the check every diagnosis starts with,
# so an unreachable VM is never reported as "not installed" or "no KVM"
# (the first bring-up did exactly that, BUG record in the plan).
ssh_reachable() { vm_ssh true >/dev/null 2>&1; }
ssh_hint() { echo "gcloud compute ssh ${GCP_INSTANCE} ${GCP_ARGS[*]} ${ZONE_ARGS[*]} ${SSH_ARGS[*]} --troubleshoot"; }
