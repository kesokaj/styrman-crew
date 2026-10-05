---
owner: main
date: 2026-10-05
---
# Per-project resources, Phase 1: Kata host settings in host-setup.sh   (2026-10-05)

Plan: `.agents/docs/plans/2026-10-05-project-resources.md` (Phase 1). Fixes
BUG-089: dave ran Cloud Hypervisor only through a hand-made shim wrapper,
and `host-setup.sh` made a plain symlink, so a fresh host ran QEMU.

- **New `deploy/lib/kata-config.sh`**, sourced by `deploy/host-setup.sh` in
  place of the old `ln -sf` step:
  - `kata_shim_wrapper <shim-path> <kata-prefix>` writes the wrapper that sets
    `KATA_CONF_FILE=…/configuration-clh.toml` when the shim path is missing, a
    symlink, not mode 755, or different. It writes a temp file in the same
    directory and renames it into place. It refuses when the prefix has no
    `containerd-shim-kata-v2`.
  - `kata_clh_reclaim <toml-path>` sets `reclaim_guest_freed_memory = true`
    with an anchored `sed`. A missing file or key is an error, not a no-op.
  - Either failure is FATAL in `host-setup.sh`. Each prints
    `changed`/`unchanged`.
- **Test:** `deploy/lib/kata-config-test.sh` has 39 checks against its own
  scratch prefix, with no root. The wrapper is covered from four starting
  states (missing, symlink, other content, correct) plus a wrong-mode case;
  the symlink target stays untouched; a missing prefix is refused; reclaim
  covers false, true, missing key and missing file; no temp files are left.
  `TestKataConfigScript` (`internal/config/kata_config_script_test.go`) runs it
  under `go test ./...`.
  - Must-fail proof: an existence-only wrapper check failed 8 checks; an
    unanchored `sed`, which also edited a commented line, failed 1.
- `deploy/verify-commit.sh` adds `deploy/lib/*.sh` to its `bash -n` and
  shellcheck lists.
- Docs: `deploy/README.md` (what host-setup now sets and how it is tested),
  `.agents/docs/deploy/remote.md` (the post-release root re-run, since the
  updater never runs `host-setup.sh`).
- dave: `kata_shim_wrapper /usr/local/bin/containerd-shim-mvm-v2 /opt/kata`
  reads `unchanged` (checked read-only). Reclaim on dave stays `false` until
  the operator re-runs `host-setup.sh` after the release.

Verification: `verifier`, `.agents/docs/testing/2026-10-05-1312-verify-project-resources-phase-1.md`
(PASS; `go test -race ./...` 425 pass / 0 fail; shell test 39/0; `bash -n`
and shellcheck exit 0). Not exercised: `host-setup.sh`'s own root run on a
real host, and the GCP VM.

Agents: `explorer` (recon), `reviewer` (cold read, three rounds), `bugs`
(BUG-089), `verifier` (phase 1). Diff read by me (under 50 source lines per
file; the phase's code review goes with the whole change at Phase 4).

## Learnings for the local host
dave's wrapper was the one hand-made piece; it now has a source of truth.
Reclaim is the only setting that changes there, on the post-release re-run.
