---
owner: bugs
date: 2026-09-21
id: BUG-070
status: fixed
area: deploy/host-setup.sh (Kata install step)
---
# BUG-070: fresh-host Kata install 404s — pinned release asset URL/extension stale

## Symptom
2026-09-21, second real run of `deploy/gcp/install.sh --version 1.36.0`
against a fresh Debian 13 Compute Engine VM (plan
`.agents/docs/plans/2026-09-21-gcp-compute-engine-target.md`): after Docker
CE 29.7.2 installed, `deploy/host-setup.sh` (run by `styrman-ctl install
--yes` from the VM's checkout at tag v1.36.0) printed `==> installing Kata
3.31.0` then `curl: (22) The requested URL returned error: 404` and the
install exited 22 (the staged secrets were removed by install.sh's trap as
designed).

## Repro
Fresh Debian 13 Compute Engine VM, `deploy/gcp/install.sh --version
1.36.0`, first Kata install (no `/opt/kata/bin/kata-runtime` yet).

## Root cause
`deploy/host-setup.sh` step 2 downloaded
`https://github.com/kata-containers/kata-containers/releases/download/${KATA_VERSION}/kata-static-${KATA_VERSION}-amd64.tar.xz`
and, for the checksum, `…/kata-static-${KATA_VERSION}-amd64.tar.xz.sha256sum`.
The Kata 3.31.0 release (checked with `gh release view 3.31.0 --repo
kata-containers/kata-containers`) publishes
`kata-static-3.31.0-amd64.tar.zst` (1,533,746,221 bytes) and no
`.sha256sum` file; the release body mentions no sha256 either. The step is
guarded by `[[ ! -x /opt/kata/bin/kata-runtime ]]`, and the production NUC
has had Kata 3.31.0 installed since its first bring-up, so every later
`host-setup.sh` run there skipped the download and the stale URL was never
exercised — the bring-up of a fresh host is the only path that runs it, and
no test or gate covers that path (`deploy/verify-commit.sh` runs only
`bash -n` on `host-setup.sh`).

## Fix
Same day (the Phase 1 commit), `deploy/host-setup.sh`: the asset is
`kata-static-${KATA_VERSION}-amd64.tar.zst`, extracted with `tar --zstd -xf`
after `apt-get install zstd` when `zstd` is absent; the expected SHA256
comes, in order, from `KATA_SHA256` (env pin), then for the pinned 3.31.0
from a constant
`KATA_SHA256_3_31_0=68c2786a0b97023f62f3eca02dc868b78a794e5469d4ddd6cc9e0bd4a7212b0b`
(GitHub's release-asset `digest` field read via `gh api
repos/kata-containers/kata-containers/releases/tags/3.31.0` on 2026-09-21,
matched by `sha256sum` against a download of the asset made the same day
from the NUC; `tar --zstd -tf` of that download lists `./opt/kata/…` with
`opt/kata/bin/kata-runtime` present, the same layout as before), then for
any other version from the same release API parsed with python3 (asset
`name` match, `digest` minus its `sha256:` prefix); the fail-closed rule
(no digest → refuse to extract as root, M15) is unchanged. The parser was
checked with a positive control (the real asset name → the digest above)
and a negative control (a bogus asset name → empty output). `bash -n` and
`shellcheck -S warning` on `deploy/host-setup.sh` exit 0. Verification of
the fixed path on a fresh host is still pending: the VM's checkout runs the
released tag, so the fix reaches the GCP bring-up only with the next
release (`/preflight`), which is the operator's call.

## Prevention
A provisioning step that is skipped on every already-provisioned host is
only verified by provisioning a fresh host — a `[[ ! -x … ]]` guard hides a
dead URL from every re-run, so the fresh-host path needs a periodic or
gated exercise (the GCP bring-up is now that exercise). A download's
checksum must come from something the publisher still publishes (here
GitHub's asset digest), not from a sidecar file whose existence was
assumed.

Graduated to `.agents/rules/shared-path-learnings.md` on 2026-09-22.

## Related
BUG-056 (Docker Hub pulls: an external publisher's behaviour changing
under a green gate); BUG-035 (a mechanism must reach every caller — here
the fresh-host caller); BUG-069 (same day, same install run: a transport
failure misdiagnosed on the gcp path).
