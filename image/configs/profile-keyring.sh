# styrman: session dbus + unlocked gnome-keyring for the Secret Service API
# (agy stores its OAuth credentials there — spike S4). Keyring files live in
# ~/.local/share/keyrings on the per-project home (ADR-031 — which is why agy still needs per-project sign-in), so auth survives everything.
# Root and non-login contexts are skipped.
if [ "$(id -u)" != "0" ] && [ -z "${STYRMAN_KEYRING_READY:-}" ]; then
    export DBUS_SESSION_BUS_ADDRESS="unix:path=/tmp/styrman-bus-$(id -u)"
    if [ ! -S "/tmp/styrman-bus-$(id -u)" ]; then
        dbus-daemon --session --address="${DBUS_SESSION_BUS_ADDRESS}" --fork 2>/dev/null
    fi
    if [ -r /run/styrman/keyring-pw ] && ! pgrep -u "$(id -u)" -x gnome-keyring-d >/dev/null 2>&1; then
        eval "$(gnome-keyring-daemon --unlock --components=secrets < /run/styrman/keyring-pw 2>/dev/null)" >/dev/null 2>&1
        export GNOME_KEYRING_CONTROL 2>/dev/null
    fi
    export STYRMAN_KEYRING_READY=1
fi
