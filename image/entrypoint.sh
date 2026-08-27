#!/usr/bin/env bash
# styrman-runtime entrypoint — runs as root inside the Kata microVM.
# Responsibilities: guest tuning, user creation (matching host UID/GID, spike
# S2), per-project DinD on a loop-mounted ext4 image (ADR-008, spike S3),
# keyring plumbing for agy (spike S4), then park as a reaped PID-1 child.
set -uo pipefail

USERNAME="${STYRMAN_USER:?STYRMAN_USER not set}"
UID_="${STYRMAN_UID:?STYRMAN_UID not set}"
GID_="${STYRMAN_GID:?STYRMAN_GID not set}"
PROJECT="${STYRMAN_PROJECT:?STYRMAN_PROJECT not set}"           # project UUID
PROJECT_DIR="${STYRMAN_PROJECT_DIR:-$PROJECT}"                  # workspace dir name
DISK_GB="${STYRMAN_DOCKER_DISK_GB:-20}"
HOME_DIR="/home/${USERNAME}"

log() { echo "[styrman] $*"; }

# /run is NOT a tmpfs in this container — a marker from the previous boot
# survives docker stop/start and would let waitReady pass while dockerd and
# the keyring are still coming up (review finding). Clear it first thing.
mkdir -p /run/styrman
rm -f /run/styrman/ready

# ─── Guest tuning (Kata mounts /proc/sys and /sys/fs/cgroup read-only) ──────
mount -o remount,rw /proc/sys 2>/dev/null || true
cat /etc/resolv.conf.styrman > /etc/resolv.conf 2>/dev/null || true

# cgroup v2 nesting for DinD (spike S3: without this dockerd dies on
# 'mkdir /sys/fs/cgroup/init: Read-only file system').
if [ -f /sys/fs/cgroup/cgroup.controllers ]; then
    mount -o remount,rw /sys/fs/cgroup 2>/dev/null || true
    mkdir -p /sys/fs/cgroup/init
    xargs -rn1 < /sys/fs/cgroup/cgroup.procs > /sys/fs/cgroup/init/cgroup.procs 2>/dev/null || :
    sed -e 's/ / +/g' -e 's/^/+/' < /sys/fs/cgroup/cgroup.controllers \
        > /sys/fs/cgroup/cgroup.subtree_control 2>/dev/null || true
fi

# Loop devices for the project docker image mount.
for i in $(seq 0 7); do
    [ -b "/dev/loop$i" ] || mknod -m660 "/dev/loop$i" b 7 "$i" 2>/dev/null || true
done

# ─── User: must match the host owner of the shared home (spike S2) ──────────
if ! id "${USERNAME}" &>/dev/null; then
    groupadd -g "${GID_}" "${USERNAME}" 2>/dev/null || true
    useradd -u "${UID_}" -g "${GID_}" -M -d "${HOME_DIR}" -s /bin/bash "${USERNAME}"
fi
echo "${USERNAME} ALL=(ALL) NOPASSWD: ALL" > "/etc/sudoers.d/${USERNAME}"
chmod 440 "/etc/sudoers.d/${USERNAME}"
usermod -aG docker "${USERNAME}" 2>/dev/null || true

# Minimal mode (system VMs, e.g. the hidden onboarding container): no
# workspace, no DinD — just the user, keyring, and a shell.
MINIMAL="${STYRMAN_MINIMAL:-}"

# Project-scoped dirs inside the shared home (write-discipline, ADR-003).
if [ -z "${MINIMAL}" ]; then
    runuser -u "${USERNAME}" -- mkdir -p "${HOME_DIR}/projects/${PROJECT_DIR}" "${HOME_DIR}/.dockerd" 2>/dev/null || true
fi

# ─── Keyring plumbing for agy (spike S4) ────────────────────────────────────
# The password lands in a root-then-user file; a profile.d snippet starts a
# session dbus + unlocked gnome-keyring on first login shell.
mkdir -p /run/styrman
printf '%s' "${STYRMAN_KEYRING_PW:-styrman}" > /run/styrman/keyring-pw
chown "${UID_}:${GID_}" /run/styrman/keyring-pw
chmod 400 /run/styrman/keyring-pw
unset STYRMAN_KEYRING_PW
cp /styrman/configs/profile-keyring.sh /etc/profile.d/styrman-keyring.sh
cp /styrman/configs/profile-motd.sh /etc/profile.d/styrman-motd.sh

# tmux defaults (hidden status bar, login shells) unless the user has their own.
if [ ! -f "${HOME_DIR}/.tmux.conf" ]; then
    runuser -u "${USERNAME}" -- cp /styrman/configs/tmux.conf "${HOME_DIR}/.tmux.conf" 2>/dev/null || true
fi

# ─── Terminal transcripts (history) ─────────────────────────────────────────
# Every tmux session pipes its output to a log in the shared home, so the
# record of what the agents and the operator did outlives the container.
# The path is project-specific, but ~/.tmux.conf is shared across projects —
# hence a per-container include file that the shared config sources.
LOG_DIR="${HOME_DIR}/.styrman/logs/${PROJECT}"
runuser -u "${USERNAME}" -- mkdir -p "${LOG_DIR}"
mkdir -p /etc/styrman
cat > /etc/styrman/tmux-project.conf <<TEOF
set-hook -g after-new-session 'pipe-pane -o "cat >> ${LOG_DIR}/#{session_name}.log"'
set-hook -g after-new-window 'pipe-pane -o "cat >> ${LOG_DIR}/#{session_name}.log"'
# Track terminal bells (agents ring BEL when waiting for input) so the UI
# can badge unattended terminals; flags clear when the session is viewed.
set -g monitor-bell on
set -g bell-action any
set -g visual-bell off
TEOF
# Ensure the shared tmux.conf sources it (idempotent; survives user edits).
if ! grep -q 'styrman/tmux-project.conf' "${HOME_DIR}/.tmux.conf" 2>/dev/null; then
    echo 'source-file -q /etc/styrman/tmux-project.conf' >> "${HOME_DIR}/.tmux.conf"
fi

# ─── Per-project Docker daemon (ADR-004 + ADR-008) ──────────────────────────
if [ -n "${MINIMAL}" ]; then
    touch /run/styrman/ready
    log "system container ${PROJECT} ready (minimal mode, user=${USERNAME})"
    exec sleep infinity
fi

IMG="${HOME_DIR}/.dockerd/${PROJECT}.img"
if [ ! -f "${IMG}" ]; then
    # Build at a temp path and mv into place only once mkfs succeeded, so an
    # interrupted first boot can't leave a half-made image that silently
    # diverts the next boot to the vfs fallback (review finding).
    log "creating ${DISK_GB}G sparse docker image ${IMG}"
    if runuser -u "${USERNAME}" -- truncate -s "${DISK_GB}G" "${IMG}.tmp" && mkfs.ext4 -q "${IMG}.tmp"; then
        mv "${IMG}.tmp" "${IMG}"
    else
        log "WARN: docker image creation failed, removing partial file"
        rm -f "${IMG}.tmp"
    fi
fi
mkdir -p /var/lib/docker-project
if mount -o loop "${IMG}" /var/lib/docker-project; then
    STORAGE_DRIVER="overlay2"
    DATA_ROOT="/var/lib/docker-project"
else
    # Fallback (ADR-008): vfs directly on the virtiofs home. Slow but works.
    log "WARN: loop mount failed, falling back to vfs storage driver"
    STORAGE_DRIVER="vfs"
    DATA_ROOT="${HOME_DIR}/.dockerd/${PROJECT}"
    mkdir -p "${DATA_ROOT}"
fi

mkdir -p /etc/docker
cat > /etc/docker/daemon.json <<EOF
{
  "data-root": "${DATA_ROOT}",
  "storage-driver": "${STORAGE_DRIVER}",
  "dns": ["8.8.8.8", "1.1.1.1"],
  "ipv6": false,
  "ip6tables": false,
  "bip": "10.64.0.1/16",
  "default-address-pools": [{"base": "10.128.0.0/16", "size": 26}],
  "log-driver": "json-file",
  "log-opts": {"max-size": "10m", "max-file": "2"}
}
EOF
rm -f /var/run/docker.pid
dockerd > /var/log/dockerd.log 2>&1 &
DOCKER_UP=false
for i in $(seq 1 30); do
    if docker info &>/dev/null; then
        DOCKER_UP=true
        log "dockerd ready (${STORAGE_DRIVER} on ${DATA_ROOT})"
        break
    fi
    sleep 1
done
if [ "${DOCKER_UP}" != "true" ]; then
    # Exit non-zero so the orchestrator's waitReady fails with a real error
    # instead of handing out a project whose docker is broken (R2).
    log "FATAL: dockerd did not become ready; last log lines:"
    tail -5 /var/log/dockerd.log || true
    exit 1
fi

# ─── Ready (only after everything above actually succeeded) ────────────────
touch /run/styrman/ready
log "project ${PROJECT} ready (user=${USERNAME} uid=${UID_})"

# tini (PID 1) reaps; this process just parks. Terminal sessions arrive via
# docker exec (ADR-002) and never depend on this shell.
exec sleep infinity
