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
# Pin Docker too (like Kata) so a fresh install is reproducible; override to
# match/upgrade. The convenience script reads $VERSION.
DOCKER_VERSION="${DOCKER_VERSION:-29.7.2}"
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
    log "installing Docker CE ${DOCKER_VERSION}"
    curl -fsSL https://get.docker.com | VERSION="${DOCKER_VERSION}" sh
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
    if [[ -z "${EXPECT}" ]]; then
        # Fail CLOSED (M15): extracting an unverified tarball as root into / is a
        # code-execution vector on a MITM'd/tampered asset. Refuse rather than
        # warn-and-continue; the operator can pin KATA_SHA256 to proceed offline.
        echo "FATAL: could not obtain the Kata ${KATA_VERSION} SHA256 — refusing to extract an unverified tarball as root. Set KATA_SHA256 to the expected digest." >&2
        rm -f "${KATA_TMP}"
        exit 1
    fi
    ACTUAL=$(sha256sum "${KATA_TMP}" | awk '{print $1}')
    [[ "${ACTUAL}" == "${EXPECT}" ]] || { echo "FATAL: Kata tarball checksum mismatch (expected ${EXPECT}, got ${ACTUAL})"; rm -f "${KATA_TMP}"; exit 1; }
    log "Kata tarball checksum verified"
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
# Project isolation is IPv4-only (the firewall matches sdb-* v4 traffic), so an
# IPv6-enabled Docker bridge could bypass it — keep IPv6 off at the daemon.
for _k in ("ipv6", "ip6tables"):
    if cfg.get(_k) is not False:
        cfg[_k] = False
        changed = True
# Keep running project microVMs alive across a daemon restart.
if cfg.get("live-restore") is not True:
    cfg["live-restore"] = True
    changed = True
# Bound container log growth so a chatty guest can't fill the disk.
if cfg.get("log-driver") != "json-file":
    cfg["log-driver"] = "json-file"
    changed = True
_want_log = {"max-size": "50m", "max-file": "3"}
if cfg.get("log-opts") != _want_log:
    cfg["log-opts"] = _want_log
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

# ─── 3b. Project→host + project→LAN isolation (R12, ADR-029/036) ────────────
# A project microVM must reach the INTERNET but nothing private: not the host,
# not the LAN, not other projects, not cloud metadata. Two paths guard this,
# both keyed on the bridge INTERFACE `-i sdb-+` — styrman names project bridges
# sdb-<hash> (config.BridgeNameFor) precisely so this matches; Docker's default
# br-<hash> names would slip past it (the 2026-08-29 leak: full host + LAN).
#   INPUT       → STYRMAN-IN  : traffic addressed to the host itself.
#   DOCKER-USER → STYRMAN-FWD : traffic ROUTED onward; runs first in FORWARD.
# The wildcard lives only on the chain JUMPS; the drop/return logic sits in
# dedicated chains so the self-heal timer can re-apply everything by
# INSERT-IF-MISSING and never delete a live rule — no window where isolation is
# briefly absent (the old delete-then-insert had one every 60s). A physdev
# RETURN exempts genuinely same-bridge L2 traffic so a project keeps reaching
# the credential broker + port-preview proxy on its OWN bridge (both are the
# server's IPs there); with bridge-nf-call-iptables=1 that traffic DOES traverse
# DOCKER-USER, so without the exemption the broker is silently dropped (the
# 2026-08-29 creds-broken regression).
cat > /usr/local/sbin/styrman-firewall.sh <<'FWEOF'
#!/usr/bin/env bash
# Keep project microVMs (bridges sdb-*) isolated from host / LAN / other projects
# / cloud metadata while leaving the internet AND the same-bridge broker + proxy
# reachable (R12, ADR-029/036). Self-heal-safe: every rule is INSERT-IF-MISSING,
# never delete-then-insert, so the 60s timer re-runs open no unprotected window.
# The `-i sdb-+` wildcard appears only on the chain jumps (matched textually via
# -S, which renders it stably); the chains' own rules carry no wildcard, so `-C`
# is reliable for them.
set -eu

modprobe xt_physdev 2>/dev/null || true   # same-bridge exemption below needs it

ip4="$(command -v iptables)"
ensure_chain() { "$ip4" -nL "$1" >/dev/null 2>&1 || "$ip4" -N "$1"; }
ensure_rule()  { local c="$1"; shift; "$ip4" -C "$c" "$@" 2>/dev/null || "$ip4" -A "$c" "$@"; }
ensure_jump()  { # parent iface target — textual presence test is wildcard-safe
    "$ip4" -S "$1" 2>/dev/null | grep -qF -- "-i $2 -j $3" || "$ip4" -I "$1" -i "$2" -j "$3"
}

# One-time migration off the pre-chain direct rules from earlier versions.
"$ip4" -D INPUT -i sdb-+ -j DROP 2>/dev/null || true
for net in 10.0.0.0/8 172.16.0.0/12 192.168.0.0/16 169.254.0.0/16; do
    "$ip4" -D DOCKER-USER -i sdb-+ -d "$net" -j DROP 2>/dev/null || true
done

# Host itself: a project bridge may send it nothing.
ensure_chain STYRMAN-IN
ensure_rule  STYRMAN-IN -j DROP
ensure_jump  INPUT sdb-+ STYRMAN-IN

# Routed egress: RETURN genuinely same-bridge L2 first (broker + proxy on the
# project's OWN bridge), then drop every private range. Other projects live in
# 172.16/12 too, so cross-project stays blocked; public internet falls through.
ensure_chain STYRMAN-FWD
ensure_rule  STYRMAN-FWD -m physdev --physdev-is-bridged -j RETURN 2>/dev/null \
    || echo "styrman-firewall: WARN xt_physdev unavailable — broker may be blocked" >&2
for net in 10.0.0.0/8 172.16.0.0/12 192.168.0.0/16 169.254.0.0/16; do
    ensure_rule STYRMAN-FWD -d "$net" -j DROP
done
ensure_jump  DOCKER-USER sdb-+ STYRMAN-FWD

# IPv6: this isolation is IPv4. Don't silently rely on the host's ipv6.disable
# boot flag — turn IPv6 off on each project bridge, and best-effort mirror the
# drops so a future IPv6-enabled bridge cannot bypass isolation.
for ifp in /sys/class/net/sdb-*; do
    [ -e "$ifp" ] || continue
    sysctl -qw "net.ipv6.conf.${ifp##*/}.disable_ipv6=1" 2>/dev/null || true
done
if command -v ip6tables >/dev/null 2>&1; then
    ip6="$(command -v ip6tables)"
    "$ip6" -nL STYRMAN-IN6 >/dev/null 2>&1 || "$ip6" -N STYRMAN-IN6 2>/dev/null || true
    "$ip6" -C STYRMAN-IN6 -j DROP 2>/dev/null || "$ip6" -A STYRMAN-IN6 -j DROP 2>/dev/null || true
    "$ip6" -S INPUT 2>/dev/null | grep -qF -- "-i sdb-+ -j STYRMAN-IN6" \
        || "$ip6" -I INPUT -i sdb-+ -j STYRMAN-IN6 2>/dev/null || true
    "$ip6" -nL STYRMAN-FWD6 >/dev/null 2>&1 || "$ip6" -N STYRMAN-FWD6 2>/dev/null || true
    for net6 in fc00::/7 fe80::/10; do
        "$ip6" -C STYRMAN-FWD6 -d "$net6" -j DROP 2>/dev/null || "$ip6" -A STYRMAN-FWD6 -d "$net6" -j DROP 2>/dev/null || true
    done
    "$ip6" -S FORWARD 2>/dev/null | grep -qF -- "-i sdb-+ -j STYRMAN-FWD6" \
        || "$ip6" -I FORWARD -i sdb-+ -j STYRMAN-FWD6 2>/dev/null || true
fi
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
    # Ingress origin secret (URL-safe: no +/= that need shell/sed escaping).
    INGRESS_TOKEN=$(openssl rand -hex 32 2>/dev/null || head -c32 /dev/urandom | od -An -tx1 | tr -d ' \n')
    JWT_KEY=$(openssl rand -hex 32 2>/dev/null || head -c32 /dev/urandom | od -An -tx1 | tr -d " \n")
    sed -e "s/^STYRMAN_UID=.*/STYRMAN_UID=${OP_UID}/" \
        -e "s/^STYRMAN_GID=.*/STYRMAN_GID=${OP_GID}/" \
        -e "s/^DOCKER_GID=.*/DOCKER_GID=${DOCKER_GID}/" \
        -e "s/^STYRMAN_USER=.*/STYRMAN_USER=${OPERATOR}/" \
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
# Versioned tag, never :latest (operator rule, no mutable tags): the version is
# the newest release tag sans "v" — the SAME string compose interpolates into
# STYRMAN_IMAGE=styrman-runtime:${STYRMAN_VERSION}, so what this step builds is
# exactly what `STYRMAN_VERSION=<that> docker compose up` will boot. A clone
# with no tags gets a unique setup-<timestamp> tag rather than a mutable one.
# `|| true` INSIDE the substitution: set -o pipefail is inherited by the
# subshell, so a tagless or .git-less tree would otherwise abort the whole
# provisioning script (exit 128, stderr suppressed) and make the fallback
# below unreachable (review finding, measured).
SETUP_VERSION="$(cd "${REPO_ROOT}" && { git describe --tags --abbrev=0 2>/dev/null || true; } | sed 's/^v//')"
[ -n "${SETUP_VERSION}" ] || SETUP_VERSION="setup-$(date +%Y%m%d%H%M)"
RUNTIME_IMAGE="${STYRMAN_IMAGE:-styrman-runtime:${SETUP_VERSION}}"
if ! docker image inspect "${RUNTIME_IMAGE}" >/dev/null 2>&1; then
    log "building runtime image ${RUNTIME_IMAGE} (a few minutes)"
    # STYRMAN_VERSION stamps /styrman/VERSION, which keys the write-once
    # harness seeding in each home — without it every build stamps 0.0.0 and
    # existing homes never re-seed (audit finding).
    docker build -t "${RUNTIME_IMAGE}" \
        --build-arg STYRMAN_VERSION="${SETUP_VERSION}" \
        "${REPO_ROOT}/image" \
        || { echo "FATAL: runtime image build failed"; exit 1; }
else
    log "runtime image ${RUNTIME_IMAGE} present"
fi

# (Step 8, the shared-home seeding, was removed: since ADR-030/031 the base
# image carries the dotfiles + agent harness and the entrypoint seeds each
# PER-PROJECT home write-once — there is no shared home to initialize.)

# ─── 8c. Tag-gated auto-update (operator request 2026-09-01) ────────────────
# Installs the updater + timer, and creates the DEDICATED deploy clone it
# operates on. The clone is deliberately NOT this repo: production must never
# be built from the operator's working tree, or an auto `git checkout <tag>`
# would stomp in-progress work. Like the backup timer, this is installed but
# only ENABLED once its prerequisite (the clone's own deploy/.env) exists.
install -m 755 "${SCRIPT_DIR}/update/styrman-update" /usr/local/bin/styrman-update
install -m 644 "${SCRIPT_DIR}/update/styrman-update.service" /etc/systemd/system/
install -m 644 "${SCRIPT_DIR}/update/styrman-update.timer" /etc/systemd/system/
systemctl daemon-reload

STYRMAN_DEPLOY_DIR="${STYRMAN_DEPLOY_DIR:-/opt/styrman}"
if [ ! -d "${STYRMAN_DEPLOY_DIR}/.git" ]; then
    ORIGIN="$(git -C "${REPO_ROOT}" remote get-url origin 2>/dev/null || true)"
    # root has no SSH agent/key of its own, and this repo is private, so an
    # ssh:// clone here fails unless a deploy key is installed first. Try, but
    # treat failure as an expected, explained outcome rather than a stray WARN.
    if [ -n "${ORIGIN}" ]; then
        # NO auto-clone over SSH here, and NO ssh-keyscan seeding.
        #
        # The updater fetches as root and BUILDS what it fetches as root, so its
        # known_hosts is a trust anchor. Seeding it with ssh-keyscan is
        # unauthenticated trust-on-first-use: it moves that window to
        # provisioning time rather than closing it, and pairing it with
        # `StrictHostKeyChecking=accept-new` meant a failed scan left an empty
        # file that the clone then TOFU-populated anyway (security review, twice).
        # The operator seeds known_hosts from the forge's PUBLISHED fingerprints
        # and clones by hand — the instructions below print the exact commands.
        echo "==> deploy clone not created automatically (see the auto-update notes below)"
    fi
fi

if [ -d "${STYRMAN_DEPLOY_DIR}/.git" ] && [ -f "${STYRMAN_DEPLOY_DIR}/deploy/.env" ]; then
    systemctl enable --now styrman-update.timer
    echo "==> auto-update: enabled (checks every 5min for a new vX.Y.Z tag)"
else
    cat <<AU
==> auto-update INSTALLED but NOT enabled. Three prerequisites, all root-only:

    1. A deploy CLONE at ${STYRMAN_DEPLOY_DIR} (production must never build
       from your working tree — an auto checkout would stomp your edits).
       The repo is private and root has no key, so give it a READ-ONLY deploy
       key first (add the .pub to the repo's Deploy keys on GitHub):

         sudo install -d -m 700 /etc/styrman
         sudo ssh-keygen -t ed25519 -N "" -C styrman-deploy -f /etc/styrman/deploy-key
         cat /etc/styrman/deploy-key.pub      # add to GitHub → Settings → Deploy keys
         # Pin the host key from GitHub's PUBLISHED fingerprints — never
         # ssh-keyscan, which is unauthenticated trust-on-first-use:
         curl -s https://api.github.com/meta | \
           jq -r '.ssh_keys[] | "github.com " + .' | sudo tee /etc/styrman/known_hosts
         # then CHECK the printed keys against https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/githubs-ssh-key-fingerprints
         sudo GIT_SSH_COMMAND="ssh -i /etc/styrman/deploy-key -o IdentitiesOnly=yes -o StrictHostKeyChecking=yes -o UserKnownHostsFile=/etc/styrman/known_hosts" \
              git clone ${ORIGIN:-<origin>} ${STYRMAN_DEPLOY_DIR}
         sudo chown -R root:root ${STYRMAN_DEPLOY_DIR}

    2. That clone's OWN deploy/.env (gitignored, so a fresh clone has none) (gitignored, so a fresh clone has none):

         sudo cp ${SCRIPT_DIR}/.env ${STYRMAN_DEPLOY_DIR}/deploy/.env
         sudo chmod 600 ${STYRMAN_DEPLOY_DIR}/deploy/.env

    Then enable it:

         # dry run, changes nothing. The state dir is explicit ON PURPOSE
         # (ADR-052): the script has no default, so a test or hand run can
         # never touch production state by omission — the systemd unit is
         # the only place the production path is pinned.
         sudo STYRMAN_UPDATE_STATE_DIR=/etc/styrman/update styrman-update --check
         sudo systemctl enable --now styrman-update.timer

    3. SIGN your release tags. The updater builds and runs the tag AS ROOT, so
       it refuses unsigned ones — otherwise anyone who can push a tag gets root
       on this host. Annotated + signed:

         git tag -s v1.16.0 -m "styrman v1.16.0" && git push origin v1.16.0

       The VERIFYING side is what the updater enforces, and it needs an
       ALLOWLIST. A bare `git verify-tag` exits 0 for a good-but-UNTRUSTED
       signature, so with GPG any key ever imported into root's keyring would
       pass and git never compares signer to tagger. The updater therefore
       requires SSH signing with an allowed-signers file and refuses to run
       without one:

         printf '%s %s\n' "<your-git-email>" "$(cat ~/.ssh/id_ed25519.pub)" \
           | sudo tee /etc/styrman/allowed_signers
         sudo chmod 644 /etc/styrman/allowed_signers   # root-owned, not group/world-writable
         # NOTE: no `git config --global` needed — the updater reads this FIXED
         # path, because systemd gives the unit no $HOME and ProtectHome=yes
         # blanks /root, so root's gitconfig is unreadable under the timer.
         # and on your side: git config gpg.format ssh; git config user.signingkey ~/.ssh/id_ed25519.pub

       Signing the tag is not enough on its own.
       To accept unsigned tags anyway — a deliberate risk — put
       STYRMAN_UPDATE_REQUIRE_SIGNED=0 in /etc/styrman/update.env.

    Pushing a SIGNED vX.Y.Z tag then deploys it within ~5 minutes; pre-release
    tags (-rc1, -beta) are ignored by design.
AU
fi

# ─── 8b. Host-layer backups (restic → GCS, R11 / ADR-044 DR layer) ──────────
# Install the script + systemd units from the repo. The timer is enabled only
# once the operator has provisioned the two root-only secrets (SA key + restic
# password) — until then this prints instructions instead of half-working.
install -d -m 700 /etc/styrman
install -m 755 "${SCRIPT_DIR}/backup/styrman-host-backup" /usr/local/bin/styrman-host-backup
install -m 644 "${SCRIPT_DIR}/backup/styrman-backup.service" /etc/systemd/system/
install -m 644 "${SCRIPT_DIR}/backup/styrman-backup.timer" /etc/systemd/system/
systemctl daemon-reload
if command -v restic >/dev/null 2>&1; then :; else apt-get update -qq && apt-get install -y restic; fi
if [ -f /etc/styrman/backup-sa.json ] && [ -f /etc/styrman/restic-pw ]; then
    systemctl enable --now styrman-backup.timer
    echo "==> host backups: daily restic timer enabled"
else
    cat <<'BK'
==> host backups INSTALLED but NOT enabled — provision the secrets first
    (the script's baked-in default is the ORIGINAL deployment's bucket,
    gs:styrman-crew-backups-driven-eft-h4it:/ in GCP project
    driven-eft-h4it — on any other host FIRST write /etc/styrman/backup.env
    with your own GOOGLE_PROJECT= and RESTIC_REPOSITORY=gs:<your-bucket>:/):
    1. Create the GCS bucket + a service account with objectAdmin on that
       bucket only; download its key to /etc/styrman/backup-sa.json
       (root:root 600).
    2. openssl rand -base64 24 > /etc/styrman/restic-pw && chmod 600 /etc/styrman/restic-pw
       SAVE THAT PASSWORD in a password manager: without it the backups
       are unreadable after host loss.
    3. Initialize the repository (adjust -r for your bucket):
       sudo env GOOGLE_PROJECT=<project> \
         GOOGLE_APPLICATION_CREDENTIALS=/etc/styrman/backup-sa.json \
         RESTIC_PASSWORD_FILE=/etc/styrman/restic-pw \
         restic -r gs:<bucket>:/ init
    4. systemctl enable --now styrman-backup.timer
BK
fi

# ─── 9. Next steps (secrets can't be auto-filled) ────────────────────────────
cat <<NEXT

==> host setup complete. To bring styrman up (production, Caddy + OAuth):
    0. LOG OUT AND BACK IN first (or run \`newgrp docker\`) — the docker
       group this script just added you to is NOT active in this shell,
       and every docker command below fails without it.
    1. Edit ${SCRIPT_DIR}/.env — fill GOOGLE_CLIENT_ID/SECRET, CLOUDFLARE_API_TOKEN,
       STYRMAN_ALLOWED_EMAILS, BASE_DOMAIN, ADMIN_EMAIL. (Identity + secrets
       like STYRMAN_INGRESS_TOKEN / STYRMAN_JWT_KEY are already generated.)
    2. Deploy at the release version (NO v prefix, NO mutable tags — the same
       string names all three images; see .agents/docs/specs/styrman/versioning.md):
         cd ${SCRIPT_DIR} \\
           && ver=\$(git -C .. describe --tags --abbrev=0 | sed 's/^v//') \\
           && docker build -t styrman-runtime:\$ver --build-arg STYRMAN_VERSION=\$ver ../image \\
           && STYRMAN_VERSION=\$ver docker compose up -d --build

    For local testing without OAuth: the integration suite and web/e2e/run.sh
    self-host a server on 127.0.0.1 — they need a Go toolchain (e2e also
    Node + Playwright) which THIS script does not install, and default to
    image styrman-runtime:dev (pass the tag step 7 above built as the arg on a
    host provisioned only by this script). Prereqs:
    .agents/docs/testing/styrman/README.md. No auth-bypass listener in prod.
NEXT
