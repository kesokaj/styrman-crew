---
owner: bugs
date: 2026-10-05
id: BUG-089
status: fixed
area: deploy/host-setup.sh (Kata shim)
---
# BUG-089: dave runs Cloud Hypervisor only through a hand-made shim wrapper that no repo file writes

## Symptom
Found 2026-10-05 by an explorer recon (`.agents/docs/plans/2026-10-05-recon-project-resources.md`, § 6) while planning per-project resources (`.agents/docs/plans/2026-10-05-project-resources.md`). On dave, `/usr/local/bin/containerd-shim-mvm-v2` is a hand-made 3-line bash wrapper (verified with `cat` and `file`, 2026-10-05):
`KATA_CONF_FILE=/opt/kata/share/defaults/kata-containers/configuration-clh.toml exec /opt/kata/bin/containerd-shim-kata-v2 "$@"`
Grep over the repo for `KATA_CONF_FILE` / `configuration-clh`: no hits.

## Repro
On dave: `cat` and `file` on `/usr/local/bin/containerd-shim-mvm-v2`. In the repo: read `deploy/host-setup.sh:104-107`.

## Root cause
A host change made by hand on dave was never captured in the shared install path. `deploy/host-setup.sh:104-107` creates the shim as a plain symlink to `containerd-shim-kata-v2`, and only when the path is absent (`[[ ! -x ... ]]`), so re-running host-setup on dave never shows the difference. Without `KATA_CONF_FILE` the shim loads `/opt/kata/share/defaults/kata-containers/configuration.toml`, a symlink to `configuration-qemu.toml`.
Consequence: a fresh host brought up by host-setup.sh (the GCP Compute Engine target, ADR-075) most likely runs QEMU, not Cloud Hypervisor, a different hypervisor than the one every test and measurement on dave exercised. NOT verified on the GCP VM (dave has no gcloud).

## Fix
Fixed in code 2026-10-05. New `deploy/lib/kata-config.sh`:
- `kata_shim_wrapper <shim-path> <kata-prefix>` writes the clh wrapper when the shim path is missing, a symlink, mode other than 755, or different content; atomic temp file + rename; refuses when the prefix has no `containerd-shim-kata-v2`.
- `kata_clh_reclaim <toml-path>` sets `reclaim_guest_freed_memory = true` with an anchored sed; returns non-zero with "missing: ..." when the file or the key is absent.
`deploy/host-setup.sh` sources it in place of the old `ln -sf` step and treats either failure as FATAL.
Hosts take the fix only when an operator re-runs host-setup.sh as root after the release reaches them; the auto-updater does not run host-setup.sh.
Test: `deploy/lib/kata-config-test.sh` (39 checks, scratch prefix, no root), run under `go test ./internal/config/` by `TestKataConfigScript`. Must-fail proof: reverting the wrapper check to existence-only failed 8 checks; an unanchored sed (which also edited a commented line) failed 1.
On dave, `kata_shim_wrapper /usr/local/bin/containerd-shim-mvm-v2 /opt/kata` reads `unchanged` (the hand-made wrapper's bytes match exactly), checked read-only 2026-10-05.
Verifier log: `.agents/docs/testing/2026-10-05-1312-verify-project-resources-phase-1.md` (PASS).
Not exercised: host-setup.sh's own root run on a real host, and the GCP VM (no gcloud on dave).

## Prevention
- A hand-made change on a live host is captured in the shared install path (host-setup.sh or the updater) in the same session, or it is drift.
- host-setup verifies the CONTENT of what it installs, not only that a path exists.
- The existing rule `.agents/rules/shared-path-learnings.md` (a step guarded by "already installed" is verified only by a host that has not installed it) already covers the guard half; it was not followed for the shim.

## Related
BUG-070 (same family: the Kata install step guarded by "already installed", graduated to `.agents/rules/shared-path-learnings.md`); BUG-073 (status probed a path nothing creates). Plans and recon named above.
