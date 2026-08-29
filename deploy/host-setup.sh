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
#                 the per-user home tree (default: $SUDO_USER).
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
# python3 is used to merge daemon.json; fail early with a clear message.
command -v python3 >/dev/null || { echo "FATAL: python3 is required (apt-get install -y python3)"; exit 1; }

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
    KATA_URL="https://github.com/kata-containers/kata-containers/releases/download/${KATA_VERSION}/kata-static-${KATA_VERSION}-amd64.tar.xz"
    KATA_TMP="$(mktemp)"
    curl -fsSL "${KATA_URL}" -o "${KATA_TMP}"
    # Verify against the release's published SHA256 before extracting into / as
    # root — a tampered/MITM'd asset would otherwise write arbitrary files.
    # Pin via KATA_SHA256 env; otherwise fetch the sums file from the release.
    EXPECT="${KATA_SHA256:-}"
    if [[ -z "${EXPECT}" ]]; then
        EXPECT=$(curl -fsSL "https://github.com/kata-containers/kata-containers/releases/download/${KATA_VERSION}/kata-static-${KATA_VERSION}-amd64.tar.xz.sha256sum" 2>/dev/null | awk '{print $1}')
    fi
    if [[ -n "${EXPECT}" ]]; then
        ACTUAL=$(sha256sum "${KATA_TMP}" | awk '{print $1}')
        [[ "${ACTUAL}" == "${EXPECT}" ]] || { echo "FATAL: Kata tarball checksum mismatch (expected ${EXPECT}, got ${ACTUAL})"; rm -f "${KATA_TMP}"; exit 1; }
        log "Kata tarball checksum verified"
    else
        log "WARNING: could not obtain Kata SHA256 — extracting UNVERIFIED (set KATA_SHA256 to enforce)"
    fi
    tar -xJf "${KATA_TMP}" -C /
    rm -f "${KATA_TMP}"
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

# ─── 3b. Project→host + project→LAN isolation (R12, ADR-029) ────────────────
# A project microVM must reach the INTERNET but nothing private: not the host,
# not the LAN, not other projects, not cloud metadata. Two paths guard this:
#   INPUT       — traffic addressed to the host itself (its gateway IS the host).
#   DOCKER-USER — traffic ROUTED onward (FORWARD) to a private network; runs
#                 first in FORWARD and survives Docker restarts.
# Both match `-i sdb-+`, the bridge INTERFACE name — styrman names project
# bridges sdb-<hash> (config.BridgeNameFor) precisely so this matches; Docker's
# default br-<hash> names would slip past it (the 2026-08-29 leak: full host +
# LAN reachable). Return traffic (into the bridge, -o sdb-+) and the port-preview
# proxy (server→project) are unaffected; the broker is same-bridge L2.
cat > /usr/local/sbin/styrman-firewall.sh <<'FWEOF'
#!/usr/bin/env bash
# Project microVMs (bridges sdb-*) reach the internet but NOT the host, the LAN,
# other projects, or cloud metadata (R12/ADR-029). Idempotent via delete-then-
# insert: iptables-nft's `-C` check is unreliable with the `sdb-+` wildcard (it
# can report a missing rule as present), so we never rely on it. Safe to re-run,
# which the self-heal timer does every 60s.
set -eu
add() {  # chain, then rule spec — delete any existing copy, then (re)insert.
    local chain="$1"; shift
    iptables -D "$chain" "$@" 2>/dev/null || true
    iptables -I "$chain" "$@"
}
# Host itself (INPUT), then private networks + link-local/metadata routed onward
# (FORWARD via DOCKER-USER). 172.16/12 also covers other project/system bridges
# (cross-project isolation); a project's own bridge + broker are same-subnet L2,
# never routed, so allowed.
add INPUT -i sdb-+ -j DROP
for net in 10.0.0.0/8 172.16.0.0/12 192.168.0.0/16 169.254.0.0/16; do
    add DOCKER-USER -i sdb-+ -d "$net" -j DROP
done
FWEOF
chmod 0755 /usr/local/sbin/styrman-firewall.sh
cat > /etc/systemd/system/styrman-firewall.service <<'EOF'
[Unit]
Description=styrman project→host/LAN network isolation (R12/ADR-029)
After=docker.service
Wants=docker.service

[Service]
Type=oneshot
ExecStart=/usr/local/sbin/styrman-firewall.sh
EOF
# A timer re-applies the (idempotent) rules every minute — self-healing, so a
# Docker daemon restart, a firewall-manager reload, or a manual flush can't leave
# the VMs exposed for more than ~1 min. The rules use the `sdb-+` wildcard, so
# NEW project bridges are covered without any per-network rule.
cat > /etc/systemd/system/styrman-firewall.timer <<'EOF'
[Unit]
Description=Re-apply styrman network isolation rules (self-heal)

[Timer]
OnBootSec=15s
OnUnitActiveSec=60s
AccuracySec=5s

[Install]
WantedBy=timers.target
EOF
systemctl daemon-reload
# Apply once immediately, then let the timer keep it applied.
systemctl start styrman-firewall.service 2>/dev/null || true
systemctl enable --now styrman-firewall.timer 2>/dev/null \
    && log "project isolation active + self-healing (styrman-firewall.timer, 60s)" \
    || log "WARN: could not enable styrman-firewall.timer — apply /usr/local/sbin/styrman-firewall.sh manually"

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
# Current layout (ADR-027/031): homes/<user>/p/<uuid> per-project homes,
# projects/<user>/<uuid> workspaces — both created lazily by the server; only
# the roots are needed here. chown is NON-recursive: a re-run must not walk
# every project home/workspace/sparse docker.img (idempotence).
mkdir -p "${STATE_DIR}/homes" "${STATE_DIR}/projects"
chown "${OPERATOR}:${OPERATOR}" "${STATE_DIR}" "${STATE_DIR}/homes" "${STATE_DIR}/projects"
log "state dir ready: ${STATE_DIR} (owner ${OPERATOR})"

# ─── 5b. Deployment .env: computed identity values ──────────────────────────
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ ! -f "${SCRIPT_DIR}/.env" && -f "${SCRIPT_DIR}/.env.example" ]]; then
    OP_UID=$(id -u "${OPERATOR}")
    OP_GID=$(id -g "${OPERATOR}")
    DOCKER_GID=$(getent group docker | cut -d: -f3)
    [[ -n "${DOCKER_GID}" ]] || { echo "FATAL: docker group not found — is Docker installed?"; exit 1; }
    # Generate a real keyring password instead of shipping the literal default,
    # which would leave every user's agy OAuth token unprotected at rest (R10).
    KEYRING_PW=$(openssl rand -base64 24 2>/dev/null || head -c18 /dev/urandom | base64)
    # Ingress origin secret (URL-safe: no +/= that need shell/sed escaping).
    INGRESS_TOKEN=$(openssl rand -hex 32 2>/dev/null || head -c32 /dev/urandom | od -An -tx1 | tr -d ' \n')
    JWT_KEY=$(openssl rand -hex 32 2>/dev/null || head -c32 /dev/urandom | od -An -tx1 | tr -d " \n")
    sed -e "s/^STYRMAN_UID=.*/STYRMAN_UID=${OP_UID}/" \
        -e "s/^STYRMAN_GID=.*/STYRMAN_GID=${OP_GID}/" \
        -e "s/^DOCKER_GID=.*/DOCKER_GID=${DOCKER_GID}/" \
        -e "s/^STYRMAN_USER=.*/STYRMAN_USER=${OPERATOR}/" \
        -e "s#^STYRMAN_KEYRING_PW=.*#STYRMAN_KEYRING_PW=${KEYRING_PW}#" \
        -e "s/^STYRMAN_INGRESS_TOKEN=.*/STYRMAN_INGRESS_TOKEN=${INGRESS_TOKEN}/" \
        -e "s/^STYRMAN_JWT_KEY=.*/STYRMAN_JWT_KEY=${JWT_KEY}/" \
        "${SCRIPT_DIR}/.env.example" > "${SCRIPT_DIR}/.env"
    chown "${OPERATOR}:${OPERATOR}" "${SCRIPT_DIR}/.env"
    chmod 600 "${SCRIPT_DIR}/.env"
    log "created deploy/.env with computed identity — fill in the secrets section"
fi

# ─── 6. Verify Kata ──────────────────────────────────────────────────────────
log "verifying Kata microVM boots..."
docker pull -q debian:trixie >/dev/null
GUEST=$(docker run --runtime mvm --rm debian:trixie uname -r)
log "OK — guest kernel ${GUEST}, host kernel $(uname -r)"

# ─── 7. Build the runtime image (image-only; no secrets needed) ──────────────
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
RUNTIME_IMAGE="${STYRMAN_IMAGE:-styrman-runtime:latest}"
if ! docker image inspect "${RUNTIME_IMAGE}" >/dev/null 2>&1; then
    log "building runtime image ${RUNTIME_IMAGE} (a few minutes)"
    # STYRMAN_VERSION stamps /styrman/VERSION, which keys the write-once
    # harness seeding in each home — without it every build stamps 0.0.0 and
    # existing homes never re-seed (audit finding).
    docker build -t "${RUNTIME_IMAGE}" \
        --build-arg STYRMAN_VERSION="$(cd "${REPO_ROOT}" && git describe --tags --always 2>/dev/null || date +%Y%m%d%H%M)" \
        "${REPO_ROOT}/image" \
        || { echo "FATAL: runtime image build failed"; exit 1; }
else
    log "runtime image ${RUNTIME_IMAGE} present"
fi

# (Step 8, the shared-home seeding, was removed: since ADR-030/031 the base
# image carries the dotfiles + agent harness and the entrypoint seeds each
# PER-PROJECT home write-once — there is no shared home to initialize.)

# ─── 9. Next steps (secrets can't be auto-filled) ────────────────────────────
cat <<NEXT

==> host setup complete. To bring styrman up (production, Caddy + OAuth):
    1. Edit ${SCRIPT_DIR}/.env — fill GOOGLE_CLIENT_ID/SECRET, CLOUDFLARE_API_TOKEN,
       STYRMAN_ALLOWED_EMAILS, BASE_DOMAIN. (Identity + secrets like
       STYRMAN_INGRESS_TOKEN / STYRMAN_JWT_KEY / STYRMAN_KEYRING_PW are already
       generated.)
    2. cd ${SCRIPT_DIR} && docker compose up -d --build

    For local testing without OAuth (no ingress): run the integration suite
    (.agents/docs/testing/styrman/integration.sh) or web/e2e/run.sh — both
    self-host a server on 127.0.0.1. There is no auth-bypass listener in prod.
NEXT
