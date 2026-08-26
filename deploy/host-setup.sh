#!/usr/bin/env bash
# styrman host provisioner (R11 / ADR-010).
#
# Turns a fresh Debian 13 host — baremetal or a GCP VM with nested
# virtualization — into a styrman host: Docker CE, Kata Containers (runtime
# "mvm"), the daemon.json Kata needs (incl. time-namespaces off, ADR-009),
# host tuning (KSM), and the styrman state dirs. Idempotent: safe to re-run;
# every step checks before it changes anything.
#
# Usage:  sudo ./host-setup.sh [--user <name>]
#         --user: the operator account that runs the styrman server and owns
#                 the shared home (default: $SUDO_USER).
set -euo pipefail

KATA_VERSION="${KATA_VERSION:-3.31.0}"
STATE_DIR="${STYRMAN_STATE_DIR:-/var/lib/styrman}"
OPERATOR="${SUDO_USER:-}"

while [[ $# -gt 0 ]]; do
    case "$1" in
        --user) OPERATOR="$2"; shift 2 ;;
        *) echo "unknown flag: $1"; exit 1 ;;
    esac
done
[[ $(id -u) -eq 0 ]] || { echo "run as root (sudo)"; exit 1; }
[[ -n "${OPERATOR}" ]] || { echo "pass --user <operator account>"; exit 1; }
id "${OPERATOR}" >/dev/null

log() { echo "==> $*"; }

# ─── 0. Sanity: virtualization ──────────────────────────────────────────────
if [[ ! -e /dev/kvm ]]; then
    if grep -qE 'vmx|svm' /proc/cpuinfo; then
        log "WARNING: /dev/kvm missing but CPU supports virt — check BIOS/modules"
    else
        echo "FATAL: no hardware virtualization (need baremetal or GCP VM with nested virt:"
        echo "  gcloud compute instances create ... --enable-nested-virtualization)"
        exit 1
    fi
fi

# ─── 1. Docker CE ───────────────────────────────────────────────────────────
if ! command -v docker >/dev/null; then
    log "installing Docker CE"
    curl -fsSL https://get.docker.com | sh
else
    log "Docker present: $(docker --version)"
fi
usermod -aG docker "${OPERATOR}"

# ─── 2. Kata Containers → /opt/kata, runtime 'mvm' ──────────────────────────
if [[ ! -x /opt/kata/bin/kata-runtime ]]; then
    log "installing Kata ${KATA_VERSION}"
    curl -fsSL "https://github.com/kata-containers/kata-containers/releases/download/${KATA_VERSION}/kata-static-${KATA_VERSION}-amd64.tar.xz" \
        | tar -xJ -C /
else
    log "Kata present: $(/opt/kata/bin/kata-runtime version | head -1)"
fi
# containerd resolves runtimeType io.containerd.mvm.v2 → containerd-shim-mvm-v2 on PATH.
if [[ ! -x /usr/local/bin/containerd-shim-mvm-v2 ]]; then
    ln -sf /opt/kata/bin/containerd-shim-kata-v2 /usr/local/bin/containerd-shim-mvm-v2
fi

# ─── 3. daemon.json: mvm runtime + time-namespaces off (ADR-009) ────────────
# Docker is only restarted when the config actually changed — a re-run on a
# live host must not kill running project microVMs (idempotence).
log "merging /etc/docker/daemon.json"
DAEMON_CHANGED=$(python3 - <<'PYEOF'
import json, os, shutil, tempfile
path = "/etc/docker/daemon.json"
cfg = {}
if os.path.exists(path):
    with open(path) as f:
        cfg = json.load(f)
changed = False
runtimes = cfg.setdefault("runtimes", {})
if runtimes.get("mvm") != {"runtimeType": "io.containerd.mvm.v2"}:
    runtimes["mvm"] = {"runtimeType": "io.containerd.mvm.v2"}
    changed = True
features = cfg.setdefault("features", {})
# Docker >= 29.5 enables time namespaces by default; Kata <= 3.31 rejects
# them (kata-containers#13080).
if features.get("time-namespaces") is not False:
    features["time-namespaces"] = False
    changed = True
if changed:
    # Copy (not move) the backup, then write via temp + atomic rename so a
    # failed write can never leave the host without a daemon.json.
    if os.path.exists(path):
        shutil.copy2(path, path + ".bak-styrman")
    fd, tmp = tempfile.mkstemp(dir=os.path.dirname(path))
    with os.fdopen(fd, "w") as f:
        json.dump(cfg, f, indent=4, sort_keys=True)
    os.replace(tmp, path)
print("yes" if changed else "no")
PYEOF
)
if [[ "${DAEMON_CHANGED}" == *yes* ]]; then
    log "daemon.json updated (backup: daemon.json.bak-styrman) — restarting docker"
    systemctl restart docker
else
    log "daemon.json already correct — not restarting docker"
fi

# ─── 4. Host tuning: KSM (merges identical microVM pages) ───────────────────
if [[ -w /sys/kernel/mm/ksm/run ]]; then
    echo 1 > /sys/kernel/mm/ksm/run
    cat > /etc/tmpfiles.d/styrman-ksm.conf <<'EOF'
w /sys/kernel/mm/ksm/run - - - - 1
w /sys/kernel/mm/ksm/sleep_millisecs - - - - 100
EOF
    log "KSM enabled (persisted via tmpfiles.d)"
fi

# ─── 5. State dirs ──────────────────────────────────────────────────────────
mkdir -p "${STATE_DIR}/home/projects"
chown -R "${OPERATOR}:${OPERATOR}" "${STATE_DIR}"
log "state dir ready: ${STATE_DIR} (owner ${OPERATOR})"

# ─── 5b. Deployment .env: computed identity values ──────────────────────────
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ ! -f "${SCRIPT_DIR}/.env" && -f "${SCRIPT_DIR}/.env.example" ]]; then
    OP_UID=$(id -u "${OPERATOR}")
    OP_GID=$(id -g "${OPERATOR}")
    DOCKER_GID=$(getent group docker | cut -d: -f3)
    sed -e "s/^STYRMAN_UID=.*/STYRMAN_UID=${OP_UID}/" \
        -e "s/^STYRMAN_GID=.*/STYRMAN_GID=${OP_GID}/" \
        -e "s/^DOCKER_GID=.*/DOCKER_GID=${DOCKER_GID}/" \
        -e "s/^STYRMAN_USER=.*/STYRMAN_USER=${OPERATOR}/" \
        "${SCRIPT_DIR}/.env.example" > "${SCRIPT_DIR}/.env"
    chown "${OPERATOR}:${OPERATOR}" "${SCRIPT_DIR}/.env"
    chmod 600 "${SCRIPT_DIR}/.env"
    log "created deploy/.env with computed identity — fill in the secrets section"
fi

# ─── 6. Verify ──────────────────────────────────────────────────────────────
log "verifying Kata microVM boots..."
docker pull -q debian:trixie >/dev/null
GUEST=$(docker run --runtime mvm --rm debian:trixie uname -r)
log "OK — guest kernel ${GUEST}, host kernel $(uname -r)"
log "host setup complete. Next: bash deploy/home-init.sh && go run ./cmd/styrman/"
