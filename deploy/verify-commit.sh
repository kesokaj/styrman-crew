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
IMG="styrman-verify-$$:tmp"
RIMG="styrman-verify-runtime-$$:tmp"

cleanup() {
    git -C "${ROOT}" worktree remove --force "${WT}" >/dev/null 2>&1 || true
    rm -rf "${WT}" 2>/dev/null || true
    docker rmi "${IMG}" >/dev/null 2>&1 || true
    docker rmi "${RIMG}" >/dev/null 2>&1 || true
}
trap cleanup EXIT

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

# The harness mirror is a THIRD module and compiles into a binary every
# project runs (ADR-062). Root ./... never reaches it, and a partially staged
# refresh (151 files, refreshed by hand) is exactly the class this gate
# exists for — so check the mirror's own integrity manifest against the
# commit's tree, build the module, and pin the two Dockerfiles' Go toolchain
# to the same value (vulncheck.sh derives its scan image from
# Dockerfile.server alone).
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

echo "==> vendored harness mirror (image/harness: integrity, build, toolchain pin)"
# sha256sum -c proves every listed file is present and unchanged; the list
# diff proves nothing UNLISTED was added, and the symlink find that nothing
# in the mirror is a link (security review round 2).
( cd "${WT}/image/harness" && sha256sum -c --quiet SHA256SUMS && go vet ./... )
extra="$(cd "${WT}/image/harness" && diff <(find . -type f ! -name SHA256SUMS | sed 's#^\./##' | LC_ALL=C sort) <(cut -c67- SHA256SUMS) || true)"
links="$(cd "${WT}/image/harness" && find . -type l)"
if [ -n "${extra}" ] || [ -n "${links}" ]; then
    echo "FAIL: image/harness differs from its SHA256SUMS file list (unlisted or missing entries, or symlinks):" >&2
    printf '%s\n%s\n' "${extra}" "${links}" >&2
    exit 1
fi
srv_go="$(sed -n 's/^FROM \(golang:[^ ]*\) AS builder$/\1/p' "${WT}/deploy/Dockerfile.server")"
img_go="$(sed -n 's/^FROM \(golang:[^ ]*\) AS harness-builder$/\1/p' "${WT}/image/Dockerfile")"
if [ -z "${srv_go}" ] || [ "${srv_go}" != "${img_go}" ]; then
    echo "FAIL: Go toolchain pin differs: deploy/Dockerfile.server='${srv_go}' image/Dockerfile='${img_go}'" >&2
    exit 1
fi
echo "    mirror matches SHA256SUMS; builder pin ${img_go}"

# The Go binary EMBEDS web/dist (web/embed.go), and web/dist is committed. So a
# frontend change whose build output was never committed ships a server whose
# UI silently predates its API. Rebuild from the commit's own web/src and
# compare: vite's content hashes are derived from content, so a matching build
# produces identical filenames (TODO 2026-09-01; hit for real 2026-09-02).
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

# Host-side shell + units (ADR-065): the firewall script runs as root from
# the verified tag via the updater's host_converge, so a syntax error or a
# unit that systemd rejects must fail here, not on the host. The netns
# behavioural test of the script runs under go test (internal/netpolicy).
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
    unit_err="$(cd "${WT}/deploy/firewall" && systemd-analyze verify ./styrman-firewall.service ./styrman-firewall.timer ./styrman-firewall.path 2>&1 \
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
( cd "${WT}" && docker build -q -f deploy/Dockerfile.server \
    --build-arg STYRMAN_VERSION=verify -t "${IMG}" . >/dev/null )

# The runtime image is the updater's other build, and the one that ships the
# harness: build it from the commit too, then check that the harness the
# image lists is the harness the commit's tree holds (ADR-062).
echo "==> runtime image (image/) and the harness it seeds"
( cd "${WT}" && docker build -q --build-arg STYRMAN_VERSION=verify -t "${RIMG}" image/ >/dev/null )
want_skills="$(find "${WT}/image/harness/skills" -mindepth 2 -maxdepth 2 -name SKILL.md | wc -l | tr -d ' ')"
want_agents="$(find "${WT}/image/harness/agents" -maxdepth 1 -name '*.md' | wc -l | tr -d ' ')"
listed="$(docker run --rm --entrypoint /styrman/bin/sh-agent "${RIMG}" list --repo /styrman/harness 2>&1)"
if ! grep -q "Skills (${want_skills})" <<<"${listed}" || ! grep -q "Subagents (${want_agents})" <<<"${listed}"; then
    echo "FAIL: the runtime image lists a different harness than the commit's image/harness tree" >&2
    echo "      expected Skills (${want_skills}) and Subagents (${want_agents}); got:" >&2
    grep -E '^### ' <<<"${listed}" >&2 || true
    exit 1
fi
echo "    runtime image seeds ${want_skills} skills, ${want_agents} subagents (styrman-harness $(cut -c1-7 "${WT}/image/harness/COMMIT"))"

echo
echo "OK: commit ${SHA:0:12} builds and tests clean in isolation — safe to tag."
