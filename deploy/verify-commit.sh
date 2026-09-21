#!/usr/bin/env bash
# Verify a COMMIT, not the working tree.
#
# Why this exists: v1.18.0 was tagged from a working tree that built while the
# COMMIT did not — `bootFailed` was extracted into a new file that never got
# staged, so the pushed tree called a function it did not define. Every local
# gate passed, because every local gate ran against the working tree. The
# auto-updater built from the tag, failed, and rolled production back.
#
# Run this before signing a release tag. It checks out the commit into a
# detached worktree under /tmp and builds THAT — so anything unstaged,
# untracked, or forgotten is absent, exactly as it will be for the updater.
#
# Usage: deploy/verify-commit.sh [commit-ish]   (default: HEAD)
set -euo pipefail

REF="${1:-HEAD}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SHA="$(git -C "${ROOT}" rev-parse --verify "${REF}^{commit}")"
WT="$(mktemp -d "${TMPDIR:-/tmp}/styrman-verify-XXXXXX")"
# Build logs live OUTSIDE the worktree: a file inside it would sit inside the
# server image's build context and the gate would no longer build exactly the
# commit's tree (release review 2026-09-14).
LOGDIR="$(mktemp -d "${TMPDIR:-/tmp}/styrman-verify-logs-XXXXXX")"
IMG="styrman-verify-$$:tmp"
RIMG="styrman-verify-runtime-$$:tmp"

cleanup() {
    git -C "${ROOT}" worktree remove --force "${WT}" >/dev/null 2>&1 || true
    rm -rf "${WT}" "${LOGDIR}" 2>/dev/null || true
    docker rmi "${IMG}" >/dev/null 2>&1 || true
    docker rmi "${RIMG}" >/dev/null 2>&1 || true
}
trap cleanup EXIT

# Docker Hub's anonymous pull budget (100 per 6 h per IP) failed this gate
# once with every other check green (BUG-056). A build that dies on it is
# named as such — it is not a defect in the commit — instead of a generic
# failure. The whole build output (stderr included: BuildKit reports the
# 429 there) is captured, and its tail shown for any other failure.
build_image() {  # build_image <log> <docker build args…>
    local log="$1"; shift
    if ! docker build "$@" >"${log}" 2>&1; then
        if grep -qiE 'toomanyrequests|too many requests' "${log}"; then
            echo "FAIL: Docker Hub rate limit (anonymous pulls exhausted for this IP) — not a defect in the commit; wait for the window or docker login (.agents/docs/deploy/remote.md, Docker Hub)" >&2
            tail -n 20 "${log}" >&2
        else
            echo "FAIL: docker build failed — last lines of ${log}:" >&2
            tail -n 20 "${log}" >&2
        fi
        exit 1
    fi
}

# The runtime image build downloads the sh-agent release from the private
# styrman-harness repository: a GitHub token as a build secret, by the one
# rule every build site follows (GH_TOKEN, else the updater's token file,
# else stop before the build).
# The token reaches ONE process — the runtime image's docker build — through
# a temporary environment on that call, never an export the whole run (and
# every go test, npm step and image build in it) would inherit. A
# fine-grained token that reads only kesokaj/styrman-harness is the better
# GH_TOKEN; `gh auth token` is the broad login token (security review
# 2026-09-21).
BUILD_TOKEN=""
if [ -n "${GH_TOKEN:-}" ]; then
    BUILD_TOKEN="${GH_TOKEN}"; SECRET_ARGS=(--secret "id=gh_token,env=GH_TOKEN")
elif [ -r /etc/styrman/gh-token ]; then
    # The updater's own bar for this file (root-owned, no group/world access).
    [ "$(stat -c %u /etc/styrman/gh-token)" = 0 ] && [ -z "$(find /etc/styrman/gh-token -perm /077 2>/dev/null)" ] \
        || { echo "FATAL: /etc/styrman/gh-token is not root-owned or is readable beyond root (chmod 600 it)" >&2; exit 1; }
    SECRET_ARGS=(--secret "id=gh_token,src=/etc/styrman/gh-token")
elif BUILD_TOKEN="$(gh auth token 2>/dev/null)" && [ -n "${BUILD_TOKEN}" ]; then
    SECRET_ARGS=(--secret "id=gh_token,env=GH_TOKEN")
else
    echo "FATAL: no GitHub token for the styrman-harness download — GH_TOKEN=<a fine-grained token that reads kesokaj/styrman-harness> (or gh auth login), or install /etc/styrman/gh-token" >&2; exit 1
fi
unset GH_TOKEN

echo "==> verifying commit ${SHA:0:12} (${REF}) in a detached worktree"
git -C "${ROOT}" worktree add -q --detach "${WT}" "${SHA}"

# Anything the working tree has but the commit does not is invisible from here
# — which is the entire point.
echo "==> go build"; ( cd "${WT}" && go build ./... )
echo "==> go vet";   ( cd "${WT}" && go vet ./... )
echo "==> go test -race"; ( cd "${WT}" && go test -race -count=1 ./... )

# The vendored ingress is a SEPARATE Go module: the root `./...` never reaches
# it, so a break there ships silently (TODO 2026-09-01, closed 2026-09-02).
echo "==> vendored ingress module (separate go.mod)"
( cd "${WT}/deploy/caddy/ingress" && go vet ./... && go test ./... )

# The two modules share dependencies but pin them independently, and the
# ingress is the internet-facing one: it runs network_mode: host with the
# docker socket mounted, so it is the LAST place that should sit on an older
# version of anything. They had diverged on docker/docker (v27.5.1 in the
# ingress vs v28.5.2 in the root) until the 2026-09-08 audit, and nothing
# noticed because vulncheck scans the modules separately.
#
# BLOCKING for the dependencies where a version gap is a security question;
# a warning for the rest, because eight shared deps diverge today and turning
# all of them into a release blocker would either break this gate or force an
# untested eight-package bump. TODO.md carries the convergence work.
echo "==> shared dependencies agree across both modules"
( cd "${WT}" && python3 - <<'PY'
import re, sys
BLOCKING = {"github.com/docker/docker"}
def deps(path):
    out = {}
    for line in open(path):
        m = re.match(r"\s+(\S+)\s+(v\S+)", line)
        if m and not line.strip().startswith("//"):
            out[m.group(1)] = m.group(2)
    return out
a, b = deps("go.mod"), deps("deploy/caddy/ingress/go.mod")
bad = sorted((k, a[k], b[k]) for k in set(a) & set(b) if a[k] != b[k])
fatal = [x for x in bad if x[0] in BLOCKING]
for k, va, vb in bad:
    mark = "FATAL " if k in BLOCKING else "warn  "
    print(f"    {mark}{k}: root {va} vs ingress {vb}", file=sys.stderr)
if fatal:
    print(f"FATAL: {len(fatal)} security-relevant shared dependency version(s) disagree", file=sys.stderr)
    sys.exit(1)
print(f"    {len(bad)} non-blocking divergence(s); docker/docker agrees")
PY
) || exit 1


if [ -z "${SKIP_WEB_FRESHNESS:-}" ] && command -v npm >/dev/null 2>&1; then
    echo "==> web/dist freshness (rebuild and compare)"
    # ONLY dist/client: that is what web/embed.go embeds and what is
    # committed. dist/server is the unused Node handler Start also emits and is
    # gitignored, so including it compares a build artifact against nothing.
    committed="$(cd "${WT}/web/dist" && find client -type f | sort)"
    ( cd "${WT}/web" && npm ci --no-audit --no-fund >/dev/null 2>&1 && npm run build >/dev/null 2>&1 )
    rebuilt="$(cd "${WT}/web/dist" && find client -type f | sort)"
    if [ "${committed}" != "${rebuilt}" ]; then
        echo "FAIL: committed web/dist does not match a build of this commit's web/src." >&2
        echo "      Run 'cd web && npm run build' and commit web/dist." >&2
        diff <(printf '%s\n' "${committed}") <(printf '%s\n' "${rebuilt}") | head -20 >&2
        exit 1
    fi
    echo "    committed assets match a fresh build"

    # Fonts are BUNDLED, never fetched from a CDN. Until now that property was
    # asserted only in a comment in web/src/index.css, so an `@import
    # url(https://fonts.googleapis.com/...)` would have shipped green
    # (security review 2026-09-04). On a platform whose premise is isolation,
    # that is not cosmetic: it turns every page load into a third-party
    # request leaking the viewer's IP and referrer off the self-hosted box.
    # Data URIs are fine — vite inlines small font subsets as data:font/woff2.
    offenders="$(grep -roE "url\(['\"]?https?:" "${WT}/web/dist/client/assets/" --include='*.css' || true)"
    if [ -n "${offenders}" ]; then
        echo "FAIL: built CSS fetches from an absolute http(s) URL — fonts and" >&2
        echo "      other assets must be self-hosted, never a CDN." >&2
        printf '%s\n' "${offenders}" | head -10 >&2
        exit 1
    fi
    echo "    built CSS references no external origin"
fi

# Host-side shell + units (ADR-036/067): the firewall script runs as root from
# the verified tag via the updater's host_converge, so a syntax error or a
# unit that systemd rejects must fail here, not on the host. Its behavioural
# test is deploy/firewall/test.sh, which needs root and runs out of band.
echo "==> host scripts: bash -n, shellcheck, systemd-analyze verify"
( cd "${WT}" && bash -n deploy/host-setup.sh deploy/firewall/styrman-firewall.sh deploy/firewall/install.sh deploy/firewall/test.sh deploy/update/styrman-update )
if command -v shellcheck >/dev/null 2>&1; then
    ( cd "${WT}" && shellcheck -S warning deploy/firewall/*.sh deploy/update/styrman-update )
    echo "    shellcheck clean: deploy/firewall/*.sh deploy/update/styrman-update"
else
    echo "    shellcheck not installed — skipped"
fi
if command -v systemd-analyze >/dev/null 2>&1; then
    # Verifying files outside /etc/systemd/system: the referenced service and
    # the not-yet-installed ExecStart path are expected complaints; anything
    # else is a real unit error.
    unit_err="$(cd "${WT}/deploy/firewall" && systemd-analyze verify ./styrman-firewall.service ./styrman-firewall.timer 2>&1 \
        | grep -v -E 'Unit .* not found|is not executable|Failed to prepare filename' || true)"
    if [ -n "${unit_err}" ]; then
        echo "FAIL: systemd-analyze verify rejects a firewall unit:" >&2
        echo "${unit_err}" >&2
        exit 1
    fi
    echo "    firewall units verify"
else
    echo "    systemd-analyze not installed — skipped"
fi

echo "==> gofmt"
fmt="$(cd "${WT}" && gofmt -l internal/ cmd/)"
[ -z "${fmt}" ] || { echo "gofmt would reformat:"; echo "${fmt}"; exit 1; }

if [ -z "${SKIP_WEB_FRESHNESS:-}" ] && command -v npm >/dev/null 2>&1; then
    echo "==> frontend check / lint / unit tests"
    ( cd "${WT}/web" && npm run check >/dev/null && npm run lint >/dev/null && npm test >/dev/null )
fi

# The build the auto-updater actually runs. This is what failed for v1.18.0.
echo "==> server image (the build the updater runs)"
( cd "${WT}" && build_image "${LOGDIR}/server-build.log" -q -f deploy/Dockerfile.server \
    --build-arg STYRMAN_VERSION=verify -t "${IMG}" . )

# The runtime image is the updater's other build, and the one that carries
# the agent harness: the sh-agent release binary named by the Dockerfile's
# HARNESS_VERSION, downloaded from GitHub at build (ADR-074). The build needs
# a token that reads the private styrman-harness repository (one rule for
# every build site: GH_TOKEN, else /etc/styrman/gh-token, else stop).
echo "==> runtime image (image/) and the sh-agent release it carries"
( cd "${WT}" && GH_TOKEN="${BUILD_TOKEN}" build_image "${LOGDIR}/runtime-build.log" -q "${SECRET_ARGS[@]}" --build-arg STYRMAN_VERSION=verify -t "${RIMG}" image/ )
want_rel="$(sed -n -E 's/^ARG HARNESS_VERSION=(v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*))$/\1/p' "${WT}/image/Dockerfile")"
[ -n "${want_rel}" ] || { echo "FAIL: image/Dockerfile has no 'ARG HARNESS_VERSION=vX.Y.Z' line" >&2; exit 1; }
got_rel="$(docker run --rm --entrypoint /styrman/bin/sh-agent "${RIMG}" version 2>&1)"
if [ "${got_rel}" != "sh-agent ${want_rel}" ]; then
    echo "FAIL: the runtime image's sh-agent reports '${got_rel}', the Dockerfile pins ${want_rel}" >&2
    exit 1
fi
echo "    runtime image carries sh-agent ${want_rel}: $(docker run --rm --entrypoint /styrman/bin/sh-agent "${RIMG}" list 2>&1 | grep -E '^### ' | tr '\n' ' ')"

echo
echo "OK: commit ${SHA:0:12} builds and tests clean in isolation — safe to tag."
