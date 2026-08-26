#!/usr/bin/env bash
# styrman shared-home initializer (ADR-003, R3).
#
# Seeds /var/lib/styrman/home with dotfiles and — if a devshell_prompt
# checkout is available — deploys the agent harness for both claude and agy
# into the shared home. Idempotent; existing user files are never overwritten
# (~/.bash_secrets especially, R10).
#
# Usage: ./home-init.sh [--harness <path-to-devshell_prompt>]
# Run as the operator (the user who owns the shared home), NOT root.
set -euo pipefail

HOME_DIR="${STYRMAN_HOME_DIR:-/var/lib/styrman/home}"
HARNESS="${HOME}/workspace/devshell_prompt"

while [[ $# -gt 0 ]]; do
    case "$1" in
        --harness) HARNESS="$2"; shift 2 ;;
        *) echo "unknown flag: $1"; exit 1 ;;
    esac
done
[[ -w "${HOME_DIR}" ]] || { echo "FATAL: ${HOME_DIR} not writable — run host-setup.sh first"; exit 1; }

log() { echo "==> $*"; }
put_once() { # $1=dest $2=content — write only if absent
    if [[ ! -e "${HOME_DIR}/$1" ]]; then
        printf '%s\n' "$2" > "${HOME_DIR}/$1"
        log "created ~/$1"
    fi
}

mkdir -p "${HOME_DIR}"/{projects,.ssh,.dockerd,.config,.cache,.local/bin,.local/share/keyrings}
chmod 700 "${HOME_DIR}" "${HOME_DIR}/.ssh"   # the home holds credentials (R3/R10)

put_once .bashrc '# styrman shared home
case $- in *i*) ;; *) return ;; esac
[ -f ~/.bash_aliases ] && . ~/.bash_aliases
[ -f ~/.bash_exports ] && . ~/.bash_exports
[ -f ~/.bash_secrets ] && . ~/.bash_secrets
export PATH="$HOME/.local/bin:$HOME/go/bin:$PATH"
PS1="\[\e[36m\]\u@\h\[\e[0m\]:\[\e[34m\]\w\[\e[0m\]\$ "'
put_once .profile '[ -n "$BASH_VERSION" ] && [ -f "$HOME/.bashrc" ] && . "$HOME/.bashrc"'
put_once .bash_aliases 'alias ll="ls -la"'
put_once .bash_exports 'export EDITOR=vim'
put_once .bash_secrets '# per-user secrets (never overwritten by provisioning; R10)
# export GEMINI_API_KEY=...   # agy fallback if keyring auth is unavailable'
chmod 600 "${HOME_DIR}/.bash_secrets"

# SSH key: generate once so R3 ("sign in once, ever") includes git-over-ssh.
if [[ ! -f "${HOME_DIR}/.ssh/id_ed25519" ]]; then
    ssh-keygen -t ed25519 -N "" -C "styrman" -f "${HOME_DIR}/.ssh/id_ed25519" >/dev/null
    log "generated ~/.ssh/id_ed25519 — add the public key to GitHub:"
    cat "${HOME_DIR}/.ssh/id_ed25519.pub"
fi

# Git identity: copy the operator's if the shared home has none.
if [[ ! -f "${HOME_DIR}/.gitconfig" && -f "${HOME}/.gitconfig" ]]; then
    cp "${HOME}/.gitconfig" "${HOME_DIR}/.gitconfig"
    log "copied ~/.gitconfig from operator"
fi

# Agent harness (both vendors) into the shared home.
if [[ -x "${HARNESS}/harness/setup.sh" ]]; then
    log "deploying agent harness from ${HARNESS}"
    HOME="${HOME_DIR}" bash "${HARNESS}/harness/setup.sh" || log "WARN: harness deploy reported errors (continuing)"
else
    log "no harness at ${HARNESS} — skipping (pass --harness <path>)"
fi

log "shared home ready: ${HOME_DIR}"
