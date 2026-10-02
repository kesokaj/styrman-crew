#!/usr/bin/env bash
# styrman-runtime entrypoint — runs as root inside the Kata microVM.
# Responsibilities: guest tuning, user creation (matching host UID/GID, spike
# S2), per-project DinD on a loop-mounted ext4 image (ADR-008, spike S3),
# then park as a reaped PID-1 child.
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
# survives docker stop/start and would let waitReady pass while dockerd is
# still coming up (review finding). Clear it first thing. harness-done (the
# end of this boot's harness step, ADR-078) is a per-boot marker too.
mkdir -p /run/styrman
rm -f /run/styrman/ready /run/styrman/harness-done

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
# A reserved name (root, daemon, …) already exists at a different uid, so
# useradd was skipped and every docker exec would run as the WRONG uid on a
# home owned by ${UID_} — silent, total breakage. Fail loudly instead (I11).
if [ "$(id -u "${USERNAME}")" != "${UID_}" ]; then
    log "FATAL: terminal username '${USERNAME}' maps to uid $(id -u "${USERNAME}"), not ${UID_} — pick a non-reserved name"
    exit 1
fi
echo "${USERNAME} ALL=(ALL) NOPASSWD: ALL" > "/etc/sudoers.d/${USERNAME}"
chmod 440 "/etc/sudoers.d/${USERNAME}"
usermod -aG docker "${USERNAME}" 2>/dev/null || true

# agy updates itself by replacing its binary in the directory it lives in
# (ADR-077), so that directory belongs to the project user — and to nobody
# else (ADR-080): world-writable, any other uid in the VM could swap the
# binary the user runs next. The DIRECTORY only: replacing a file needs
# write access to its directory, not to the file, and leaving the 220 MB
# binary root-owned keeps this a metadata change instead of a copy-up.
# Before the minimal-mode branch on purpose: the setup VM runs agy too.
chown "${UID_}:${GID_}" /opt/agy 2>/dev/null \
    || log "WARN: could not hand /opt/agy to ${USERNAME} — \`agy update\` will not work in this VM"

# Self-heal home ownership: if the home was ever missing at mount time,
# Docker auto-creates the bind source owned by root, which makes every write
# by the guest fail (git config, ssh-keygen, gh). Claim the mount point for
# the guest uid. Non-recursive so existing user files keep their ownership.
chown "${UID_}:${GID_}" "${HOME_DIR}" 2>/dev/null || true

# Minimal mode (system VMs, e.g. the hidden onboarding container): no
# workspace, no DinD — just the user and a shell.
MINIMAL="${STYRMAN_MINIMAL:-}"

# Standard home dirs — the home is now PER-PROJECT and starts empty
# (ADR-031), so the image seeds the skeleton. ~/projects/<dir> is the
# workspace bind mount (mkdir is a no-op on it); ~/.dockerd stays in the home.
# ~/.npm-global/{bin,lib} is the npm prefix skel/bash_exports names
# (NPM_CONFIG_PREFIX): Claude's own updater installs there, and while the
# tree did not exist Claude reported the prefix unwritable and switched its
# updater off, and skel/bashrc left ~/.npm-global/bin off PATH (ADR-077).
# Created on EVERY boot, so homes that predate this get it too.
if [ -z "${MINIMAL}" ]; then
    runuser -u "${USERNAME}" -- mkdir -p \
        "${HOME_DIR}/projects/${PROJECT_DIR}" "${HOME_DIR}/.dockerd" \
        "${HOME_DIR}/.config" "${HOME_DIR}/.cache" "${HOME_DIR}/.local/bin" \
        "${HOME_DIR}/.npm-global/bin" "${HOME_DIR}/.npm-global/lib" \
        2>/dev/null || true
fi
runuser -u "${USERNAME}" -- mkdir -p "${HOME_DIR}/.ssh" 2>/dev/null || true
chmod 700 "${HOME_DIR}/.ssh" 2>/dev/null || true

# Default shell dotfiles from the image (ADR-031), seeded write-once as the
# user. A per-project home starts empty, so this is what makes it a usable
# shell. bash_secrets is 0600 (R10). Never clobbers a file the user edited.
if [ -d /styrman/skel ]; then
    for f in bashrc profile bash_aliases bash_exports bash_secrets npmrc; do
        dst="${HOME_DIR}/.${f}"
        if [ ! -e "${dst}" ]; then
            runuser -u "${USERNAME}" -- cp "/styrman/skel/${f}" "${dst}" 2>/dev/null || true
            [ "${f}" = "bash_secrets" ] && chmod 600 "${dst}" 2>/dev/null || true
        fi
    done
fi

# ─── Login-shell profile.d snippets ─────────────────────────────────────────
# zzz- prefix: the MOTD probes ~/.claude/.credentials.json etc., so it must
# run AFTER zz-styrman-credentials.sh has materialized them (audit finding —
# it otherwise nags "run styrman-setup" on a fully-onboarded project).
cp /styrman/configs/profile-motd.sh /etc/profile.d/zzz-styrman-motd.sh
rm -f /etc/profile.d/styrman-motd.sh
# Credential materialization (ADR-032): every login shell pulls the current
# env/file credentials from the broker on this project's bridge.
cp /styrman/configs/styrman-credentials.sh /etc/profile.d/zz-styrman-credentials.sh

# tmux defaults (hidden status bar, login shells) unless the user has their own.
if [ ! -f "${HOME_DIR}/.tmux.conf" ]; then
    runuser -u "${USERNAME}" -- cp /styrman/configs/tmux.conf "${HOME_DIR}/.tmux.conf" 2>/dev/null \
        || log "WARN: could not seed ~/.tmux.conf (home not writable?)"
fi

# ─── Agent harness from the base image (R5/ADR-030, ADR-074, ADR-078) ───────
# The image carries /styrman/bin/sh-agent, the styrman-harness RELEASE binary
# with the harness embedded, downloaded and verified at image build. Seeded
# write-once, keyed on the image version plus the binary's release, as the
# user: `sh-agent install` copies the context, skills and agents into
# ~/.claude and ~/.gemini/config, copies itself to ~/.local/bin/sh-agent
# (first on PATH, skel/bashrc) and writes its manifest to ~/.config/sh-agent/
# — NOT credentials, and no settings.json for either tool beyond one key:
# since harness 42ba2f5 it writes `"attribution": {"commit": "", "pr": ""}`
# into ~/.claude/settings.json when that key is absent (a present key is
# left alone), so no commit from a project carries a Co-Authored-By trailer
# (operator directive 2026-09-03, the exception approved 2026-09-16,
# ADR-072). A home seeded by the previous harness (≤ v1.19.x) is migrated
# first: configs/harness-migrate.sh moves its artifacts to
# ~/.styrman/harness-backup-<ts>/, never deletes. Between image bumps the
# project moves itself: its user runs `sh-agent update` inside it (gh via
# the broker's token, the signer compiled into the binary), which replaces
# ~/.local/bin/sh-agent and reinstalls; the never-downgrade guard below keeps
# an older image from undoing that. install is a file copier — no network,
# no agy/claude call, no git, no `go build` (a release binary installs what
# it carries; of its stdout only the settings.json lines are logged below —
# the "agent harness deployed" line is this boot's pass signal).
#
# The seed runs in TWO parts since ADR-078. install is no longer quick: the
# v2 harness is 1,788 files and the binary syncs each one — 0.06 s on tmpfs,
# 12.8 s on a disk-backed home in a microVM (measured 2026-10-02), where
# v1.6.0 took 0.02 s. In front of /run/styrman/ready that put a first boot,
# and the first boot after every release, at 16 s against the 15 s hard
# limit. So:
#   BEFORE ready (here): the migration, the never-downgrade decision and the
#     two MCP registrations — all quick, and the registrations write
#     ~/.claude.json, which the first login shell's credential snippet and
#     the user's own `claude` write too; before ready no terminal exists, so
#     nothing can race them.
#   AFTER ready (harness_finish, in the background): `sh-agent install`, then
#     the stamp and the pass or WARN lines. A `claude` or `agy` started in
#     those seconds sees no skills until it is restarted — the accepted cost
#     of an unchanged boot time.
# /run/styrman/harness-done marks the end of this boot's harness step,
# whatever its outcome; the integration suite waits on it.
# Non-fatal: a harness hiccup must never block the project from booting.
# harness_home_newer <home release> <image release> — true only when BOTH
# are canonical vX.Y.Z and the home's is strictly newer (sort -V). An
# unreadable image binary ("unknown") never wins the comparison, so it
# takes the failure path below instead of silently keeping an old home
# (security review 2026-09-21). Pure: tested by extraction in
# internal/config/entrypoint_guard_test.go.
harness_home_newer() {
    printf '%s\n' "$1" | grep -Eq '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$' || return 1
    printf '%s\n' "$2" | grep -Eq '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$' || return 1
    [ "$1" != "$2" ] && [ "$(printf '%s\n%s\n' "$2" "$1" | sort -V | tail -1)" = "$1" ]
}
# What harness_finish reads: set here so a boot with nothing to seed (stamp
# current, minimal mode, no binary) still has them defined under `set -u`.
HARNESS_PENDING=0   # 1: this boot has a seed to finish (stamp differs)
HARNESS_INSTALL=0   # 1: the finish step runs `sh-agent install`
if [ -z "${MINIMAL}" ] && [ -x /styrman/bin/sh-agent ]; then
    HARNESS_VER="$(cat /styrman/VERSION 2>/dev/null || echo dev)"
    # ADR-074: /styrman/bin/sh-agent is the styrman-harness RELEASE binary the
    # image downloaded at build, the harness embedded. The stamp keys on the
    # image version AND the binary's release, so an image bump re-seeds once,
    # as always. Between image bumps the project moves itself: its user runs
    # `sh-agent update` inside it, which replaces ~/.local/bin/sh-agent and
    # reinstalls; the guard below keeps an older image from undoing that.
    HARNESS_REL="$(/styrman/bin/sh-agent version 2>/dev/null | awk '{print $2}' | tr -cd '0-9a-z.')"
    [ -n "${HARNESS_REL}" ] || HARNESS_REL=unknown
    HARNESS_ID="${HARNESS_VER}+${HARNESS_REL}"
    HARNESS_STAMP="${HOME_DIR}/.styrman/.harness-version"
    if [ "$(cat "${HARNESS_STAMP}" 2>/dev/null || true)" != "${HARNESS_ID}" ]; then
        HARNESS_PENDING=1
        runuser -u "${USERNAME}" -- mkdir -p "${HOME_DIR}/.styrman" 2>/dev/null || true
        # Every step runs as the user, each under a timeout: the home is
        # user-controlled, so a stall here must never brick a boot. (The
        # 2026-09-03 security review feared a ~/.gitconfig include of a FIFO
        # hanging the installer's `git rev-parse`, and the install line
        # carried GIT_CONFIG_GLOBAL=/dev/null against it; a release binary
        # never runs git, so that guard is gone and the timeout is the whole
        # defence.) The stamp is written only when EVERY step succeeds, so a
        # failure is retried next boot (all are idempotent); install runs
        # even after a partial migration so a single unmovable old file
        # cannot withhold the harness — the WARN says what was left.
        # The three capped steps in FRONT of ready (migrate, two MCP
        # registrations, 20 s each) sum to 60 s worst case, inside the
        # server's 90 s ready wait; each finishes in well under a second
        # when nothing stalls. install's own cap (120 s, harness_finish) is
        # behind ready and bounds only the background job.
        HM_OK=1; HI_OK=1; HR_OK=1; HR_FAIL=""
        timeout 20 runuser -u "${USERNAME}" -- env HOME="${HOME_DIR}" bash /styrman/configs/harness-migrate.sh || HM_OK=0
        # Never downgrade: the home's manifest names the release its harness
        # came from ("repo": "embedded:vX.Y.Z" — the binary's own shape; a
        # path there is a pre-ADR-074 install and re-seeds). A home the user
        # moved past this image's release keeps its own sh-agent and harness.
        # The manifest is guest-writable: only a regular file is read, under
        # the same cap as every other step here (a FIFO must not stall the
        # boot before /run/styrman/ready — security review 2026-09-21).
        HOME_REL=""; HI_SKIPPED=0
        HS="${HOME_DIR}/.config/sh-agent/state.json"
        if [ -f "${HS}" ] && [ ! -L "${HS}" ]; then
            HOME_REL="$(timeout 5 sed -n 's/^ *"repo": *"embedded:\(v[0-9][0-9.]*\)".*/\1/p' "${HS}" 2>/dev/null | head -1)"
        fi
        if [ "${HARNESS_REL}" = unknown ]; then
            # No release to key on: nothing is installed, nothing stamped —
            # the WARN in harness_finish fires and the next boot retries.
            HI_OK=0
            log "WARN: /styrman/bin/sh-agent did not report a version — harness not deployed this boot"
        elif harness_home_newer "${HOME_REL}" "${HARNESS_REL}"; then
            HI_SKIPPED=1
            log "agent harness: home has sh-agent ${HOME_REL} (sh-agent update), image carries ${HARNESS_REL}; left alone"
        else
            HARNESS_INSTALL=1
        fi
        # Browser MCP (ADR-068 D4): register the project's browser as an MCP
        # server with each agent's OWN command — the tools' MCP registries
        # (~/.claude.json, ~/.gemini/config/mcp_config.json), still no
        # settings.json, hooks or scripts. `claude mcp add` refuses a duplicate
        # (exit 1), so the entry is removed first; `agy mcp add` is add-or-
        # update. Graded as ONE unit: either tool failing keeps the stamp
        # unwritten, so a half-registered home is retried next boot and never
        # reads as done (the BUG-035 rule: a gate names every condition it
        # guards). Neither command needs a login (verified 2026-09-11).
        # Output is KEPT (last line) so a home that makes the write fail is
        # diagnosable from the boot log, not a silent every-boot retry.
        # They run BEFORE install since ADR-078 (they ran after it): neither
        # registry is a file install manages, so the order between them is
        # free, and in front of ready is where nothing else writes them.
        HR_OUT="$(timeout 20 runuser -u "${USERNAME}" -- env HOME="${HOME_DIR}" sh -c \
            'claude mcp remove --scope user styrman-browser >/dev/null 2>&1 || true; claude mcp add --scope user styrman-browser -- /styrman/bin/styrman-browser mcp' 2>&1 >/dev/null)" \
            || { HR_OK=0; HR_FAIL="claude"; log "  claude mcp add: $(printf '%s' "${HR_OUT}" | tail -1 | tr -cd '[:print:]' | cut -c1-200)"; }
        HR_OUT="$(timeout 20 runuser -u "${USERNAME}" -- env HOME="${HOME_DIR}" \
            agy mcp add styrman-browser /styrman/bin/styrman-browser mcp 2>&1 >/dev/null)" \
            || { HR_OK=0; HR_FAIL="${HR_FAIL:+both}"; HR_FAIL="${HR_FAIL:-agy}"; log "  agy mcp add: $(printf '%s' "${HR_OUT}" | tail -1 | tr -cd '[:print:]' | cut -c1-200)"; }
    fi
fi

# harness_finish — the part of the seed that runs AFTER /run/styrman/ready,
# as a background job of this shell (it reads the results the block above
# left in HM_OK, HR_OK, HI_SKIPPED and the HARNESS_* variables). Writes
# /run/styrman/harness-done last, on every path: stamp already current, no
# binary in the image, deployed, kept, or failed.
harness_finish() {
    if [ "${HARNESS_PENDING}" = 1 ]; then
        if [ "${HARNESS_INSTALL}" = 1 ]; then
            # stdout is KEPT for one thing: what the installer says about
            # ~/.claude/settings.json — its one write outside the customization
            # directories (the attribution key, ADR-072). That is the `settings:`
            # line of the create, add and could-not-write paths, and the
            # `note: … attribution …` advisory it prints INSTEAD when the file
            # cannot be judged (not a regular file, unreadable, not a JSON
            # object): the key is not written then and only that line says so
            # (BUG-062 — the first filter kept `settings:` alone). The note is
            # matched on the hint's own wording, not on the word "attribution",
            # which a terminal username could carry into the PATH hint's path.
            # Its other note, that PATH hint printed on every seed, stays out,
            # as does everything else; stderr passes through as before (security
            # review of the 4aa19be refresh, finding 2). These lines are a
            # DIAGNOSTIC, not an attestation: the project user is root inside
            # its own VM and can write anything to this stdout, so a check that
            # needs the fact reads ~/.claude/settings.json itself (Tier 2 does).
            # 120 s: the copy takes about 13 s on a disk-backed home; the cap
            # only bounds a stall, and nothing waits on this job.
            HI_OUT="$(timeout 120 runuser -u "${USERNAME}" -- env HOME="${HOME_DIR}" \
                /styrman/bin/sh-agent install --home "${HOME_DIR}")" || HI_OK=0
            printf '%s\n' "${HI_OUT}" | grep -E '^settings:|^note: .*(attribution setting could not be checked|does not set "attribution")' \
                | tr -cd '[:print:]\n' | cut -c1-200 \
                | while IFS= read -r line; do log "  ${line}"; done
        fi
        if [ "${HM_OK}" = 1 ] && [ "${HI_OK}" = 1 ] && [ "${HR_OK}" = 1 ]; then
            # The version and the path travel as ARGUMENTS, never interpolated
            # into the sh -c string: a quote in a hand-built STYRMAN_VERSION
            # would otherwise run as the project user (audit 2026-09-13).
            runuser -u "${USERNAME}" -- sh -c 'printf "%s" "$1" > "$2"' _ "${HARNESS_ID}" "${HARNESS_STAMP}" 2>/dev/null || true
            if [ "${HI_SKIPPED}" = 1 ]; then
                log "agent harness kept (version ${HARNESS_VER}: home sh-agent ${HOME_REL} is newer than the image's ${HARNESS_REL}, install skipped; browser MCP registered for claude and agy)"
            else
                log "agent harness deployed (version ${HARNESS_VER}, sh-agent ${HARNESS_REL}; browser MCP registered for claude and agy)"
            fi
        else
            [ "${HM_OK}" = 1 ] || log "WARN: previous-harness migration reported errors or timed out (see the [harness-migrate] lines above)"
            [ "${HI_OK}" = 1 ] || log "WARN: sh-agent install reported errors or timed out — harness not deployed this boot (continuing)"
            [ "${HR_OK}" = 1 ] || log "WARN: browser MCP registration failed for ${HR_FAIL} — retried next boot (continuing)"
        fi
    fi
    touch /run/styrman/harness-done
}

# ─── Terminal transcripts (history) ─────────────────────────────────────────
# Every tmux session pipes its output to a log in the project home, so the
# record of what the agents and the operator did outlives the container.
# The path is project-specific, but ~/.tmux.conf is shared across projects —
# hence a per-container include file that the shared config sources.
LOG_DIR="${HOME_DIR}/.styrman/logs/${PROJECT}"
# If the log dir can't be made, the pipe-pane hooks below would fail per-pane
# and transcripts (R6) would silently stop — warn loudly instead.
runuser -u "${USERNAME}" -- mkdir -p "${LOG_DIR}" || log "WARN: could not create transcript dir ${LOG_DIR} — history will be incomplete"
mkdir -p /etc/styrman
cat > /etc/styrman/tmux-project.conf <<TEOF
set-hook -g after-new-session 'pipe-pane -o "cat >> ${LOG_DIR}/#{session_name}.log"'
set-hook -g after-new-window 'pipe-pane -o "cat >> ${LOG_DIR}/#{session_name}.log"'
# Track terminal bells (agents ring BEL when waiting for input) so the UI
# can badge unattended terminals; flags clear when the session is viewed.
set -g monitor-bell on
set -g bell-action any
set -g visual-bell off
# OSC 52: let tmux (and TUIs under it) write the system clipboard — the
# browser terminal's clipboard addon picks it up.
set -g set-clipboard on
set -as terminal-features ',xterm-256color:clipboard'
TEOF
# Ensure the shared tmux.conf sources it — only if the file exists (a failed
# cp above must NOT lead to `echo >>` creating a root-owned stub the guest
# can't edit, I10). Append AS THE USER so ownership never flips to root.
if [ -f "${HOME_DIR}/.tmux.conf" ]; then
    if ! grep -q 'styrman/tmux-project.conf' "${HOME_DIR}/.tmux.conf" 2>/dev/null; then
        runuser -u "${USERNAME}" -- sh -c \
            "printf '%s\n' 'source-file -q /etc/styrman/tmux-project.conf' >> '${HOME_DIR}/.tmux.conf'" 2>/dev/null || true
    fi
    # Migration: earlier defaults shipped 'mouse on', which steals mouse
    # selection from the browser terminal — only touch files still ours.
    if head -1 "${HOME_DIR}/.tmux.conf" 2>/dev/null | grep -q '^# styrman default tmux config'; then
        runuser -u "${USERNAME}" -- sed -i 's/^set -g mouse on$/set -g mouse off/' "${HOME_DIR}/.tmux.conf" 2>/dev/null || true
    fi
    # Self-heal: reclaim a ~/.tmux.conf that a previous (buggy) boot created
    # root-owned, so the guest can always read/edit its own config.
    chown "${UID_}:${GID_}" "${HOME_DIR}/.tmux.conf" 2>/dev/null || true
fi

# ─── Per-project Docker daemon (ADR-004 + ADR-008) ──────────────────────────
if [ -n "${MINIMAL}" ]; then
    touch /run/styrman/ready
    log "system container ${PROJECT} ready (minimal mode, user=${USERNAME})"
    trap 'exit 0' TERM INT
    sleep infinity &
    wait $!
    exit 0
fi

IMG="${HOME_DIR}/.dockerd/${PROJECT}.img"
if [ ! -f "${IMG}" ]; then
    # N30: fail fast if the home disk can't hold the image, instead of letting
    # dockerd hit ENOSPC mid-write inside a filesystem that thinks it has room.
    AVAIL_KB=$(df -Pk "${HOME_DIR}/.dockerd" 2>/dev/null | awk 'NR==2{print $4}')
    NEED_KB=$((DISK_GB * 1024 * 1024))
    if [ -n "${AVAIL_KB}" ] && [ "${AVAIL_KB}" -lt "${NEED_KB}" ]; then
        log "WARN: only ${AVAIL_KB}KiB free for a ${DISK_GB}G docker image — DinD may fail on ENOSPC"
    fi
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
# Check the ext4 image before mounting — a dirty journal from an ungraceful
# shutdown (now less likely with the SIGTERM trap) is repaired, not tolerated.
[ -f "${IMG}" ] && fsck.ext4 -p "${IMG}" >/dev/null 2>&1 || true
if [ -f "${IMG}" ] && mount -o loop "${IMG}" /var/lib/docker-project; then
    STORAGE_DRIVER="overlay2"
    DATA_ROOT="/var/lib/docker-project"
else
    # Fallback (ADR-008): vfs directly on the virtiofs home. Slow, and it uses
    # a DIFFERENT data-root — prior overlay2 images/containers won't be visible
    # (looks like data loss). Surface it loudly + via a marker the UI can read.
    log "WARN: loop mount of ${IMG} FAILED — falling back to slow 'vfs' storage."
    log "WARN: containers/images from the overlay2 image are not visible under vfs."
    STORAGE_DRIVER="vfs"
    DATA_ROOT="${HOME_DIR}/.dockerd/${PROJECT}"
    mkdir -p "${DATA_ROOT}"
    runuser -u "${USERNAME}" -- sh -c "printf 'vfs (loop mount failed)\n' > '${HOME_DIR}/.styrman/logs/${PROJECT}/storage-warning.txt'" 2>/dev/null || true
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
# /run is NOT a tmpfs in this image (it survives stop/start), so an unclean
# shutdown (SIGKILL) leaves stale docker/containerd sockets and pids that
# make the next dockerd wait on a dead containerd until timeout — observed
# as a "dockerd did not become ready" boot loop in prod. Clean slate.
rm -rf /var/run/docker.pid /var/run/docker /var/run/containerd 2>/dev/null || true
dockerd > /var/log/dockerd.log 2>&1 &
DOCKERD_PID=$!
DOCKER_UP=false
for i in $(seq 1 60); do
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

# ─── Agent harness, second part (ADR-078) ──────────────────────────────────
# The file install, behind ready and in the background: the boot does not
# wait on it. A shutdown does not wait on it either — the stamp is its last
# write, so an interrupted install is simply repeated on the next boot.
harness_finish &

# ─── apt persistence (operator Q7, ADR-028) ────────────────────────────────
# The rootfs survives stop/start but dies on container REMOVAL (delete,
# username change, image upgrade). Packages the user apt-installed are
# captured as a manifest (delta vs the image's base snapshot) at shutdown
# and replayed here IN THE BACKGROUND after ready — the boot SLA must not
# wait on apt. The manifest lives in the project home, wiped with the project.
APT_DIR="${HOME_DIR}/.styrman/apt"
APT_MANIFEST="${APT_DIR}/${PROJECT}.list"
APT_REPLAYING=/run/styrman/apt-replaying
if [ -z "${MINIMAL}" ] && [ -s "${APT_MANIFEST}" ]; then
    # Marker: while the replay runs (or after it fails), a shutdown must NOT
    # capture a fresh delta — mid-replay dpkg state would SHRINK the manifest
    # and permanently lose packages (review finding).
    touch "${APT_REPLAYING}"
    (
        log "replaying apt manifest ($(wc -l < "${APT_MANIFEST}") packages) — see .styrman/apt/${PROJECT}.log"
        export DEBIAN_FRONTEND=noninteractive
        if apt-get update -qq && xargs -a "${APT_MANIFEST}" apt-get install -y -qq --no-install-recommends --; then
            log "apt manifest replay complete"
            rm -f "${APT_REPLAYING}"
        else
            log "WARN: apt manifest replay failed — keeping existing manifest"
        fi
    ) > "${APT_DIR}/${PROJECT}.log" 2>&1 &
fi

capture_apt_manifest() {
    [ -n "${MINIMAL}" ] && return 0
    # Replay still running or failed: the existing manifest is more complete
    # than anything we could compute now — keep it.
    [ -e "${APT_REPLAYING}" ] && return 0
    runuser -u "${USERNAME}" -- mkdir -p "${APT_DIR}" 2>/dev/null || return 0
    # Delta vs the image's base package snapshot = what the user installed.
    # (apt REMOVALS of base packages are not recorded — ADR-028.)
    if dpkg-query -W -f='${Package}\n' 2>/dev/null | sort \
        | comm -13 /etc/styrman-base-packages - > "${APT_MANIFEST}.tmp" 2>/dev/null; then
        mv "${APT_MANIFEST}.tmp" "${APT_MANIFEST}" 2>/dev/null || true
        chown "${UID_}:${GID_}" "${APT_MANIFEST}" 2>/dev/null || true
    else
        rm -f "${APT_MANIFEST}.tmp" 2>/dev/null || true
    fi
}

# Graceful shutdown (I9): on `docker stop`, tini forwards SIGTERM here. Stop
# dockerd cleanly (so nested containers get their grace period and overlay2
# isn't torn down mid-write) and unmount the loop-mounted ext4 so its journal
# is clean on the next boot. Do NOT `exec` — the shell must stay alive to run
# the trap. tini (PID 1) still reaps exec'd session zombies.
shutdown() {
    log "shutting down project ${PROJECT}"
    capture_apt_manifest
    if [ -n "${DOCKERD_PID:-}" ]; then
        kill -TERM "${DOCKERD_PID}" 2>/dev/null || true
        wait "${DOCKERD_PID}" 2>/dev/null || true
    fi
    umount /var/lib/docker-project 2>/dev/null || true
    exit 0
}
trap shutdown TERM INT
sleep infinity &
wait $!
