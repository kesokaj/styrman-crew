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

# ─── 3b. Project→host + project→LAN isolation (R12, ADR-029/036/065) ────────
# Moved to §5c (deploy/firewall/install.sh): the firewall script, its timer
# and path units, the run dir, the marker dir and the operator's LAN
# definition. It runs after §5 because the installer needs the state dir to
# exist and belong to the operator (the server's uid owns the marker dir).

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
# Refuse a symlink at any of the three roots rather than chown through it
# (security review 2026-09-08: chown/chmod dereference, mkdir -p traverses).
for d in "${STATE_DIR}" "${STATE_DIR}/homes" "${STATE_DIR}/projects"; do
    if [[ -L "${d}" ]]; then echo "FATAL: ${d} is a symlink — refusing to provision through it"; exit 1; fi
done
mkdir -p "${STATE_DIR}/homes" "${STATE_DIR}/projects"
chown -h "${OPERATOR}:${OPERATOR}" "${STATE_DIR}" "${STATE_DIR}/homes" "${STATE_DIR}/projects"
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

# ─── 5c. Project→host + project→LAN isolation (R12, ADR-029/036/065) ────────
# A project microVM must reach the INTERNET but nothing private: not the host,
# not the LAN, not other projects, not cloud metadata — except the LAN, for a
# project an admin toggles on (ADR-065). Both halves are root-owned host
# state outside the container stack: /usr/local/sbin/styrman-firewall.sh (base
# isolation + per-project reconcile), styrman-firewall.timer (60 s self-heal),
# styrman-firewall.path (applies a toggle within a second), the run dir
# /run/styrman-firewall, the server-written marker dir under the state dir,
# and — written only if absent — /etc/styrman/firewall.conf, the operator's
# definition of "local network", discovered from the default route and
# printed. deploy/firewall/install.sh is idempotent and narrow; the
# auto-updater's host_converge re-applies the script and units from every
# verified release tag, so this is the last time a firewall change needs a
# hand-run of host-setup.sh (the 2026-08-30 → 2026-09-02 drift, ADR-065).
if STYRMAN_STATE_DIR="${STATE_DIR}" bash "${SCRIPT_DIR}/firewall/install.sh"; then
    log "project isolation active + self-healing (styrman-firewall.timer 60 s, styrman-firewall.path on change)"
else
    log "WARN: deploy/firewall/install.sh failed — projects stay isolated only if a previous script is installed; run: sudo bash ${SCRIPT_DIR}/firewall/install.sh"
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
# would stomp in-progress work. Installed always, but only ENABLED once its
# prerequisite (the clone's own deploy/.env) exists.
install -m 755 "${SCRIPT_DIR}/update/styrman-update" /usr/local/bin/styrman-update
install -m 644 "${SCRIPT_DIR}/update/styrman-update.service" /etc/systemd/system/
install -m 644 "${SCRIPT_DIR}/update/styrman-update.timer" /etc/systemd/system/
systemctl daemon-reload

STYRMAN_DEPLOY_DIR="${STYRMAN_DEPLOY_DIR:-/opt/styrman}"
if [ ! -d "${STYRMAN_DEPLOY_DIR}/.git" ]; then
    ORIGIN="$(git -C "${REPO_ROOT}" remote get-url origin 2>/dev/null || true)"
    # The repo is private and root authenticates with a fine-grained GitHub
    # token the operator must mint in the GitHub UI first (no SSH key on the
    # host — operator decision 2026-09-01) — so no auto-clone here; the
    # instructions below print the exact commands.
    if [ -n "${ORIGIN}" ]; then
        echo "==> deploy clone not created automatically (see the auto-update notes below)"
    fi
fi

# Best-effort https rendering of this repo's origin for the instructions
# below (an ssh-form origin is rewritten; the updater clones over HTTPS).
HTTPS_ORIGIN="$(printf '%s' "${ORIGIN:-}" | sed -E 's#^git@([^:]+):#https://\1/#')"
[ -n "${HTTPS_ORIGIN}" ] || HTTPS_ORIGIN="https://github.com/<owner>/<repo>.git"

if [ -d "${STYRMAN_DEPLOY_DIR}/.git" ] && [ -f "${STYRMAN_DEPLOY_DIR}/deploy/.env" ]; then
    systemctl enable --now styrman-update.timer
    echo "==> auto-update: enabled (checks every 5min for a new vX.Y.Z tag)"
else
    cat <<AU
==> auto-update INSTALLED but NOT enabled. Three prerequisites, all root-only:

    1. A deploy CLONE at ${STYRMAN_DEPLOY_DIR} (production must never build
       from your working tree — an auto checkout would stomp your edits).
       The repo is private and the host holds NO SSH key (operator decision
       2026-09-01): auth is a FINE-GRAINED GitHub token, scoped to this ONE
       repository with Contents: Read-only. Mint it in the GitHub UI
       (Settings → Developer settings → Fine-grained tokens: Repository
       access = only this repo; Permissions = Contents: Read-only; pick an
       expiry you will honor — the updater fails loudly when it lapses):

         sudo install -d -m 700 /etc/styrman
         # paste the token when prompted — read -s: no echo, so it lands in
         # neither shell history nor terminal scrollback/transcripts:
         sudo bash -c 'umask 077; read -rs t; printf "%s" "\$t" > /etc/styrman/gh-token'
         # HTTPS origin — the token never goes in the URL; the updater feeds
         # it via a credential helper reading that root-only file:
         sudo git -c credential.helper= \\
              -c 'credential.https://github.com.helper=!f() { echo username=x-access-token; printf "password=%s\\n" "\$(cat /etc/styrman/gh-token)"; }; f' \\
              clone ${HTTPS_ORIGIN} ${STYRMAN_DEPLOY_DIR}
         sudo chown -R root:root ${STYRMAN_DEPLOY_DIR}

    2. That clone's OWN deploy/.env (gitignored, so a fresh clone has none):

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
       ALLOWLIST. A bare \`git verify-tag\` exits 0 for a good-but-UNTRUSTED
       signature, so with GPG any key ever imported into root's keyring would
       pass and git never compares signer to tagger. The updater therefore
       requires SSH signing with an allowed-signers file and refuses to run
       without one:

         printf '%s %s\n' "<your-git-email>" "\$(cat ~/.ssh/id_ed25519.pub)" \\
           | sudo tee /etc/styrman/allowed_signers
         sudo chmod 644 /etc/styrman/allowed_signers   # root-owned, not group/world-writable
         # NOTE: no \`git config --global\` needed — the updater reads this FIXED
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

# (Step 8b, the restic→GCS host backup layer, was REMOVED — operator decision
# 2026-09-01: no backup layer at all; durability is git. Users commit and
# push; a stale host is rebuilt from this script + the repo. If an old host
# still carries styrman-backup.{service,timer}, disable and delete them.)
# /etc/styrman itself stays: the auto-updater's allowed_signers, gh-token,
# and run-state live there.
install -d -m 700 /etc/styrman

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
