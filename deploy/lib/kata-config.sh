# shellcheck shell=bash
# deploy/lib/kata-config.sh — the two Kata host settings styrman depends on,
# as functions sourced by host-setup.sh. Each takes its target path as an
# argument so deploy/lib/kata-config-test.sh can point it at a scratch prefix
# (ADR-052), prints `changed` or `unchanged` on success, and prints the reason
# and returns non-zero when it cannot do its job.
#
# Both settings were hand-made on the first host and never captured here, so
# a fresh host (the GCP VM) silently ran QEMU instead of Cloud Hypervisor
# (BUG-089). They are checked by CONTENT, not by "the path exists": a guard on
# existence is what hid the drift.

# kata_shim_wrapper <shim-path> <kata-prefix>
# containerd resolves runtimeType io.containerd.mvm.v2 to
# containerd-shim-mvm-v2 on PATH. A plain symlink to containerd-shim-kata-v2
# makes Kata load configuration.toml, which upstream points at QEMU; the
# wrapper pins Cloud Hypervisor, the hypervisor every sizing measurement was
# taken on (ADR-083). Replacing a symlink with the wrapper only affects
# sandboxes started afterwards.
kata_shim_wrapper() {
    local shim="$1" prefix="$2" want tmp
    if [[ ! -x "${prefix}/bin/containerd-shim-kata-v2" ]]; then
        echo "missing: ${prefix}/bin/containerd-shim-kata-v2"
        return 1
    fi
    want="#!/bin/bash
KATA_CONF_FILE=${prefix}/share/defaults/kata-containers/configuration-clh.toml \\
    exec ${prefix}/bin/containerd-shim-kata-v2 \"\$@\""
    if [[ -f "${shim}" && ! -L "${shim}" && "$(stat -c %a "${shim}")" == 755 \
          && "$(cat "${shim}")" == "${want}" ]]; then
        echo unchanged
        return 0
    fi
    # Same directory, then rename: containerd never sees a half-written
    # shim, and rename replaces a symlink itself rather than its target.
    tmp="$(mktemp "${shim}.XXXXXX")" || return 1
    printf '%s\n' "${want}" > "${tmp}" && chmod 0755 "${tmp}" && mv -f "${tmp}" "${shim}" \
        || { rm -f "${tmp}"; echo "failed: writing ${shim}"; return 1; }
    echo changed
}

# kata_clh_reclaim <toml-path>
# reclaim_guest_freed_memory = true lets Cloud Hypervisor take back memory a
# guest has freed (virtio-balloon free page reporting). Projects are sized to
# the whole host by default (ADR-083), so without it every VM keeps whatever
# it once touched. Kata reads its config per sandbox start: no restart.
# A toml without the key is an error, not a no-op: a Kata release that
# renamed it would otherwise leave reclaim off with "unchanged" printed.
kata_clh_reclaim() {
    local toml="$1" tmp
    if [[ ! -f "${toml}" ]]; then
        echo "missing: ${toml}"
        return 1
    fi
    if grep -qE '^reclaim_guest_freed_memory = true$' "${toml}"; then
        echo unchanged
        return 0
    fi
    if ! grep -qE '^reclaim_guest_freed_memory = false$' "${toml}"; then
        echo "missing: reclaim_guest_freed_memory in ${toml}"
        return 1
    fi
    tmp="$(mktemp "${toml}.XXXXXX")" || return 1
    sed 's/^reclaim_guest_freed_memory = false$/reclaim_guest_freed_memory = true/' "${toml}" > "${tmp}" \
        && chmod --reference="${toml}" "${tmp}" && chown --reference="${toml}" "${tmp}" \
        && mv -f "${tmp}" "${toml}" \
        || { rm -f "${tmp}"; echo "failed: writing ${toml}"; return 1; }
    echo changed
}
