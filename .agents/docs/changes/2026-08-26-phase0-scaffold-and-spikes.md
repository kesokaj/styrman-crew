# 2026-08-26 — Phase 0: scaffold, naming, de-risking spikes

## Summary

Project named **styrman**. Scaffolded the repo (AGENTS.md, REQUIREMENTS.md, TODO.md,
README.md, go.mod `github.com/kesokaj/styrman`, git init, ADR-001…007), then ran the
five Phase 0 de-risking spikes — all passed. One unplanned host fix (S0): Docker 29's
default time namespaces broke Kata; disabled via daemon.json feature flag. Spike S3
upgraded the DinD storage design from devbox's `vfs` to overlay2 on a loop-mounted
sparse ext4 image. New operator requirement R11: fully reproducible host install
(baremetal/GCP VM) — `deploy/host-setup.sh` promoted to a Phase 1 deliverable.

## Files changed

- `AGENTS.md`, `REQUIREMENTS.md` (R1–R11), `TODO.md`, `README.md`, `.gitignore`, `go.mod` — new
- `.agents/docs/architecture/DECISIONS.md` — ADR-001…010
- `.agents/docs/plans/2026-08-26-super-devbox-architecture.md` — renamed to styrman naming
- `.agents/docs/plans/2026-08-26-super-devbox-implementation-plan.md` — Phase 0 completion note
- `.agents/docs/testing/spikes/2026-08-26-phase0-results.md` — spike results
- Host (outside repo): `/etc/docker/daemon.json` + restart — added
  `"features": {"time-namespaces": false}` (backup: `daemon.json.bak-20260826`)

## Verification

- `docker run --runtime mvm --rm debian:trixie uname -r` → `6.18.28` (guest ≠ host 6.12.101)
- S1: TTY exec into Kata OK; no-op exec RTT ~31 ms incl. CLI overhead
- S2: two microVMs sharing one virtiofs dir — cross-VM visibility ≤ 41 ms, clean writes
- S3: in-guest dockerd `overlay2` on loop-mounted ext4 image → `hello-world` runs;
  overlay2 directly on virtiofs fails (expected); vfs works (fallback)
- S4: `agy 1.1.21` installed; headless gnome-keyring via `dbus-run-session` stores and
  reads secrets; keyring persists across container restart via mounted home
- S5: microVM cold start 1.03–1.10 s (target < 5 s)
- All spike containers/images/dirs removed afterwards (`docker ps -a` clean)
