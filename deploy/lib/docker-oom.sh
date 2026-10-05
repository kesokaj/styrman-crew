# shellcheck shell=bash
# deploy/lib/docker-oom.sh — dockerd's place in the OOM order, as a function
# sourced by host-setup.sh (ADR-085). Takes the drop-in path as an argument so
# deploy/lib/docker-oom-test.sh can point it at a scratch dir (ADR-052);
# prints `changed` or `unchanged`, or the reason and a non-zero status.
#
# Project VMs run at oom_score_adj -999 and are sized to the whole host
# (ADR-083). Docker's own unit runs dockerd at -500, so under memory pressure
# dockerd died before any VM, and its unit stops restarting it after three
# kills in a minute: the server stayed up but could no longer stop or resize
# anything (security review of v1.42.1, finding 2). At -999 dockerd ties with
# the VMs and, being far smaller, is not the one picked; no start limit means
# systemd keeps bringing it back if it is.

docker_oom_dropin_content() {
    cat <<'UNIT'
# styrman (ADR-085): dockerd ranks with the project VMs in the OOM order and
# is always restarted. Written by host-setup.sh — edits here are overwritten.
[Unit]
StartLimitIntervalSec=0

[Service]
OOMScoreAdjust=-999
UNIT
}

# docker_oom_dropin <drop-in-path>
docker_oom_dropin() {
    local path="$1" dir tmp want
    dir="$(dirname "${path}")"
    want="$(docker_oom_dropin_content)"
    if [[ -f "${path}" && ! -L "${path}" && "$(cat "${path}")" == "${want}" ]]; then
        echo unchanged
        return 0
    fi
    mkdir -p "${dir}" || { echo "failed: mkdir ${dir}"; return 1; }
    tmp="$(mktemp "${path}.XXXXXX")" || return 1
    printf '%s\n' "${want}" > "${tmp}" && chmod 0644 "${tmp}" && mv -f "${tmp}" "${path}" \
        || { rm -f "${tmp}"; echo "failed: writing ${path}"; return 1; }
    echo changed
}
