# shellcheck shell=sh
# styrman: nudge toward one-time onboarding while credentials are missing.
# Sourced from /etc/profile.d by every login shell (no shebang on purpose).
if [ "$(id -u)" != "0" ] && [ -z "${STYRMAN_MOTD_SHOWN:-}" ] && [ -t 1 ]; then
    export STYRMAN_MOTD_SHOWN=1
    if [ ! -f "$HOME/.claude/.credentials.json" ] || ! git config --global user.email >/dev/null 2>&1; then
        echo "· First time here? Run 'styrman-setup' to sign in to git, GitHub, claude and agy — once, for every project."
    fi
    # Written by zz-styrman-credentials.sh (which runs first) when the broker
    # did not answer: brokered tokens, gcloud ADC and the ssh key are then
    # missing from THIS shell — say so instead of letting git fail later
    # (BUG-054: this outage was silent for three days).
    if [ -f "$HOME/.styrman/broker-unreachable" ]; then
        echo "· credentials: the broker did not answer ($(cat "$HOME/.styrman/broker-unreachable" 2>/dev/null)) — brokered tokens, gcloud ADC and the ssh key are not in this shell; open a new terminal to retry."
    fi
fi
