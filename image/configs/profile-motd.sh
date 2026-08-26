# styrman: nudge toward one-time onboarding while credentials are missing.
if [ "$(id -u)" != "0" ] && [ -z "${STYRMAN_MOTD_SHOWN:-}" ] && [ -t 1 ]; then
    export STYRMAN_MOTD_SHOWN=1
    if [ ! -f "$HOME/.claude/.credentials.json" ] || ! git config --global user.email >/dev/null 2>&1; then
        echo "· First time here? Run 'styrman-setup' to sign in to git, GitHub, claude and agy — once, for every project."
    fi
fi
