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

[ -n "${STYRMAN_CRED_BROKER:-}" ] || return 0 2>/dev/null || exit 0
command -v curl >/dev/null 2>&1 || return 0 2>/dev/null || exit 0

# env-token tier: export each KEY=VALUE. jq is in the base image; fall back to
# doing nothing rather than mis-parsing if it is somehow absent.
if command -v jq >/dev/null 2>&1; then
    __sc_env="$(curl -fsS --max-time 5 "${STYRMAN_CRED_BROKER}/env" 2>/dev/null || true)"
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
    __sc_files="$(curl -fsS --max-time 5 "${STYRMAN_CRED_BROKER}/files" 2>/dev/null || true)"
    if [ -n "${__sc_files}" ]; then
        printf '%s' "${__sc_files}" \
            | jq -r '.[] | [.path, .mode, .data] | @tsv' 2>/dev/null \
            | while IFS="$(printf '\t')" read -r __p __m __d; do
                case "${__p}" in
                    "~/"*) __p="${HOME}/${__p#\~/}" ;;
                    /*)    continue ;;                 # never absolute
                    *)     __p="${HOME}/${__p}" ;;
                esac
                case "${__p}" in *..*) continue ;; esac
                mkdir -p "$(dirname "${__p}")" 2>/dev/null || continue
                if printf '%s' "${__d}" | base64 -d > "${__p}.styrman-tmp" 2>/dev/null; then
                    chmod "${__m:-0600}" "${__p}.styrman-tmp" 2>/dev/null || true
                    mv -f "${__p}.styrman-tmp" "${__p}" 2>/dev/null || rm -f "${__p}.styrman-tmp"
                else
                    rm -f "${__p}.styrman-tmp" 2>/dev/null || true
                fi
            done
    fi
    unset __sc_files

    # keyring tier (ADR-032, agy): store each Secret Service item into THIS
    # project's gnome-keyring (unlocked by styrman-keyring.sh, which sorts
    # before this snippet). go-keyring items key on {service, username} —
    # secret-tool store with identical attributes replaces/creates, so the
    # seeded item is indistinguishable from a native sign-in. agy itself is
    # already installed in the image — only the token is pulled here.
    if command -v secret-tool >/dev/null 2>&1; then
        __sc_kr="$(curl -fsS --max-time 5 "${STYRMAN_CRED_BROKER}/keyring" 2>/dev/null || true)"
        if [ -n "${__sc_kr}" ] && [ "${__sc_kr}" != "[]" ]; then
            printf '%s' "${__sc_kr}" \
                | jq -r '.[] | [.service, .username, .label, .secret] | @tsv' 2>/dev/null \
                | while IFS="$(printf '\t')" read -r __s __u __l __d; do
                    [ -n "${__s}" ] && [ -n "${__u}" ] || continue
                    printf '%s' "${__d}" | base64 -d 2>/dev/null \
                        | secret-tool store --label="${__l:-${__s}}" service "${__s}" username "${__u}" 2>/dev/null || true
                done
        fi
        unset __sc_kr
    fi
fi
