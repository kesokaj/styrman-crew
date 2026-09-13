# shellcheck shell=sh
# styrman credential materialization (ADR-032). Sourced by every login shell.
# Pulls the CURRENT credentials for this project's user from the broker on the
# project's own bridge and makes them usable:
#   - /env   → exports shell variables (GH_TOKEN, NPM_TOKEN, CLAUDE_* …)
#   - /files → writes files into the home (gcloud ADC, ssh key …)
# The broker owns the single live copy, so a refresh anywhere shows up on the
# next login shell — sign in once, every project stays current, no re-auth.
#
# Quiet and fast: no broker (host-mode dev) or any hiccup → do nothing. The
# broker URL is the fixed per-bridge address in $STYRMAN_CRED_BROKER.

# A styrman credential harvest/import ("Import from project") reads the guest's
# REAL current files; it sets STYRMAN_SKIP_CRED_PULL so this shell does NOT
# overwrite them with the broker's copy first, which would both make the import
# read back a stale value and clobber a token the user just re-authed (H6).
[ -z "${STYRMAN_SKIP_CRED_PULL:-}" ] || return 0 2>/dev/null || exit 0

# STYRMAN_CRED_BROKER is injected by the server when it creates this
# container: broadcast-1 of the bridge's REAL subnet (ADR-030,
# orchestrator.brokerIPForSubnet). It is authoritative. This snippet used to
# re-derive it from the VM's own IP as "<o1>.<o2>.255.254" — a /16
# assumption — so that a recreated network could not strand the project on
# a stale address; the day the host pinned bridges to /24 (v1.27.1,
# 2026-09-10) that arithmetic pointed every login shell at an address nobody
# answers, silently (BUG-054). The stale-network case is the server's job
# now: a container whose network vanished is recreated with a fresh env,
# never reconnected (ADR-069).
__sc_marker="${HOME}/.styrman/broker-unreachable"
if [ -z "${STYRMAN_CRED_BROKER:-}" ]; then
    # No broker by design (host-mode dev): nothing to reach, nothing stale.
    rm -f "${__sc_marker}" 2>/dev/null || true
    unset __sc_marker
    return 0 2>/dev/null || exit 0
fi
command -v curl >/dev/null 2>&1 || return 0 2>/dev/null || exit 0
# Either probe failing sets the marker the MOTD reports; it is cleared only
# when BOTH answered (BUG-054's lesson: a silent `|| true` hid the outage).
__sc_fail=0

# env-token tier: export each KEY=VALUE. jq is in the base image; fall back to
# doing nothing rather than mis-parsing if it is somehow absent.
if command -v jq >/dev/null 2>&1; then
    if ! __sc_env="$(curl -fsS --connect-timeout 1 --max-time 5 "${STYRMAN_CRED_BROKER}/env" 2>/dev/null)"; then
        __sc_env=""
        __sc_fail=1
    fi
    if [ -n "${__sc_env}" ]; then
        while IFS="$(printf '\t')" read -r __k __v; do
            case "${__k}" in
                ''|*[!A-Za-z0-9_]*) continue ;;   # defensive: only plain identifiers
            esac
            export "${__k}=${__v}"
        done <<EOF
$(printf '%s' "${__sc_env}" | jq -r 'to_entries[] | "\(.key)\t\(.value)"' 2>/dev/null)
EOF
    fi
    unset __sc_env __k __v

    # file-injection tier: write each file at its mode under $HOME.
    if ! __sc_files="$(curl -fsS --connect-timeout 1 --max-time 5 "${STYRMAN_CRED_BROKER}/files" 2>/dev/null)"; then
        __sc_files=""
        __sc_fail=1
    fi
    if [ -n "${__sc_files}" ]; then
        printf '%s' "${__sc_files}" \
            | jq -r '.[] | [.path, .mode, .data] | @tsv' 2>/dev/null \
            | while IFS="$(printf '\t')" read -r __p __m __d; do
                # shellcheck disable=SC2088  # "~/"* is a case PATTERN, not an expansion
                case "${__p}" in
                    "~/"*) __p="${HOME}/${__p#\~/}" ;;
                    /*)    continue ;;                 # never absolute
                    *)     __p="${HOME}/${__p}" ;;
                esac
                case "${__p}" in *..*) continue ;; esac
                mkdir -p "$(dirname "${__p}")" 2>/dev/null || continue
                if (umask 077; printf '%s' "${__d}" | base64 -d > "${__p}.styrman-tmp") 2>/dev/null; then
                    chmod "${__m:-0600}" "${__p}.styrman-tmp" 2>/dev/null || true
                    mv -f "${__p}.styrman-tmp" "${__p}" 2>/dev/null || rm -f "${__p}.styrman-tmp"
                else
                    rm -f "${__p}.styrman-tmp" 2>/dev/null || true
                fi
            done
    fi
    unset __sc_files

    # The marker (read by profile-motd.sh): never fatal, the login must go on.
    if [ "${__sc_fail}" = 1 ]; then
        mkdir -p "${HOME}/.styrman" 2>/dev/null || true
        printf '%s %s\n' "$(date -u +%FT%TZ 2>/dev/null || echo unknown)" "${STYRMAN_CRED_BROKER}" > "${__sc_marker}" 2>/dev/null || true
    else
        rm -f "${__sc_marker}" 2>/dev/null || true
    fi

    # git identity: the broker serves the author name/email as env creds (the
    # token flows via the git credential helper, but the identity doesn't).
    # Apply it as global git config so `git commit` works in a terminal — as
    # config, not GIT_AUTHOR_* env, so a per-repo user.name still overrides it.
    if [ -n "${STYRMAN_GIT_NAME:-}" ] && command -v git >/dev/null 2>&1; then
        git config --global user.name "${STYRMAN_GIT_NAME}" 2>/dev/null || true
        [ -n "${STYRMAN_GIT_EMAIL:-}" ] &&
            git config --global user.email "${STYRMAN_GIT_EMAIL}" 2>/dev/null || true
    fi

    # claude onboarding flag: a materialized ~/.claude/.credentials.json means
    # the user already signed in and onboarded (in the setup VM / another
    # project), but a fresh per-project ~/.claude.json has no
    # hasCompletedOnboarding, so the interactive CLI re-runs onboarding and
    # prompts for login DESPITE valid creds (`claude -p` works, `claude` asks to
    # sign in). Set the flag whenever the token is present — jq-merge to keep the
    # file's other per-project state; create a minimal file if it's absent.
    if [ -f "${HOME}/.claude/.credentials.json" ]; then
        __cj="${HOME}/.claude.json"
        if [ -f "${__cj}" ]; then
            __ct="$(jq '.hasCompletedOnboarding = true' "${__cj}" 2>/dev/null)"
            # Atomic write so a concurrent `claude` never reads a torn file.
            [ -n "${__ct}" ] && printf '%s' "${__ct}" > "${__cj}.tmp" && mv -f "${__cj}.tmp" "${__cj}"
            unset __ct
        else
            printf '{"hasCompletedOnboarding":true}' > "${__cj}.tmp" && mv -f "${__cj}.tmp" "${__cj}"
        fi
        unset __cj
    fi
fi
