#!/usr/bin/env bash
# styrman-runtime entrypoint — runs as root inside the Kata microVM.
# Responsibilities: guest tuning, user creation (matching host UID/GID, spike
# S2), per-project DinD on a loop-mounted ext4 image (ADR-008, spike S3),
# keyring plumbing for agy (spike S4), then park as a reaped PID-1 child.
set -uo pipefail

USERNAME="${STYRMAN_USER:?STYRMAN_USER not set}"
UID_="${STYRMAN_UID:?STYRMAN_UID not set}"
GID_="${STYRMAN_GID:?STYRMAN_GID not set}"
PROJECT="${STYRMAN_PROJECT:?STYRMAN_PROJECT not set}"
DISK_GB="${STYRMAN_DOCKER_DISK_GB:-20}"
HOME_DIR="/home/${USERNAME}"

log() { echo "[styrman] $*"; }

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

# Project-scoped dirs inside the shared home (write-discipline, ADR-003).
runuser -u "${USERNAME}" -- mkdir -p "${HOME_DIR}/projects/${PROJECT}" "${HOME_DIR}/.dockerd" 2>/dev/null || true

# ─── Keyring plumbing for agy (spike S4) ────────────────────────────────────
# The password lands in a root-then-user file; a profile.d snippet starts a
# session dbus + unlocked gnome-keyring on first login shell.
mkdir -p /run/styrman
printf '%s' "${STYRMAN_KEYRING_PW:-styrman}" > /run/styrman/keyring-pw
chown "${UID_}:${GID_}" /run/styrman/keyring-pw
chmod 400 /run/styrman/keyring-pw
unset STYRMAN_KEYRING_PW
cp /styrman/configs/profile-keyring.sh /etc/profile.d/styrman-keyring.sh

# tmux defaults (hidden status bar, login shells) unless the user has their own.
if [ ! -f "${HOME_DIR}/.tmux.conf" ]; then
    runuser -u "${USERNAME}" -- cp /styrman/configs/tmux.conf "${HOME_DIR}/.tmux.conf" 2>/dev/null || true
fi

# ─── Per-project Docker daemon (ADR-004 + ADR-008) ──────────────────────────
IMG="${HOME_DIR}/.dockerd/${PROJECT}.img"
if [ ! -f "${IMG}" ]; then
    log "creating ${DISK_GB}G sparse docker image ${IMG}"
    runuser -u "${USERNAME}" -- truncate -s "${DISK_GB}G" "${IMG}"
    mkfs.ext4 -q "${IMG}"
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
for i in $(seq 1 30); do
    if docker info &>/dev/null; then
        chmod 666 /var/run/docker.sock 2>/dev/null || true
        log "dockerd ready (${STORAGE_DRIVER} on ${DATA_ROOT})"
        break
    fi
    sleep 1
done

# ─── Ready ──────────────────────────────────────────────────────────────────
touch /run/styrman/ready
log "project ${PROJECT} ready (user=${USERNAME} uid=${UID_})"

# tini (PID 1) reaps; this process just parks. Terminal sessions arrive via
# docker exec (ADR-002) and never depend on this shell.
exec sleep infinity
