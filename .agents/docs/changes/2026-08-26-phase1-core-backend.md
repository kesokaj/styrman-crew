# 2026-08-26 — Phase 1: runtime image, core backend, terminal bridge, reproducible install

## Summary

Phase 1 delivered end-to-end on localhost: a project created through the API
boots a Kata microVM in ~2 s with git initialized, overlay2 DinD running, and
a browser-terminal WebSocket that attaches claude/agy (or a plain shell) via
tmux. Persistence proven: workspace files, docker.img, tmux state across
disconnects, and full credential survival across container destruction.
Integration suite: **18/18 checks pass**. Host provisioning (`host-setup.sh`)
and shared-home init (`home-init.sh`) validated on this machine — including
the agent harness deploy for both vendors.

## What was built

- `internal/config` — env-driven config; labels, close codes, per-container env contract
- `internal/project` — projects.json store (atomic writes), slug validation + unit tests
- `internal/orchestrator` — microVM lifecycle (devbox Kata config), label adoption, idle reaper, exec helpers (probe + interactive TTY)
- `internal/conversation` — tmux session list/kill via exec
- `internal/terminal` — WS↔exec bridge, devbox protocol (4000/4001, resize, heartbeat)
- `internal/httpapi` — REST API + embedded `/dev/` test console (xterm.js)
- `cmd/styrman`, `cmd/wsprobe` (WS integration probe)
- `image/` — Dockerfile (3.75 GB, both agents baked in), entrypoint (cgroup nesting, user@host-UID, ext4-loop overlay2 DinD, keyring plumbing), tmux.conf, profile-keyring.sh
- `deploy/host-setup.sh` (R11: fresh Debian → styrman host, idempotent, self-verifying), `deploy/home-init.sh`
- Specs: `api.md`, `terminal-protocol.md`, `runtime-image.md`, **`guide.md`** (the full project guide); testing guide + `integration.sh`

## What went wrong along the way (and fixes)

1. **Docker SDK module path**: `go get github.com/docker/docker@latest` now
   resolves to the renamed `moby/moby` modules and fails to build. Pinned
   `v28.5.2+incompatible` (same as devbox). Revisit when migrating to the
   moby SDK properly.
2. **Keyring never started for agent conversations** (caught in smoke test,
   after the integration suite passed — the suite only used `kind=shell`):
   the tmux pane command ran `claude`/`agy` directly, skipping login-shell
   profile.d, so agy would have had no Secret Service. Fixed by wrapping pane
   commands in `bash -lc 'exec <agent>'`. Lesson: test the agent path, not
   just the shell path — an integration check for DBUS presence in agent
   conversations should be added when the UI lands.
3. **wsprobe compile error** (`[]byte` vs `string` return) — trivial, caught
   by first build.
4. Carried from Phase 0 and now encoded in scripts: Docker 29 time-namespaces
   vs Kata (ADR-009, host-setup merges the flag), `/sys/fs/cgroup` read-only
   in guests (entrypoint remounts), `dbus-launch` vs `dbus-run-session`
   (image installs plain `dbus`; profile snippet uses dbus-daemon directly).

## Good / bad honestly

- **Good**: 2 s project creation (target was < 5); the exec transport worked
  first try under Kata including TTY resize; 18/18 integration on first full
  run; the devbox protocol port needed zero changes.
- **Bad / watchlist**: the runtime image is 3.75 GB (fine locally, slow to
  rebuild — consider layer-splitting agents from tools); keyring password is
  convenience-grade (documented in REQUIREMENTS R10 spirit, not a real
  at-rest defense); `createProject` boots the microVM synchronously (fine for
  curl/UI today, needs async + progress events in Phase 2); vfs fallback path
  in the entrypoint is untested in anger.

## Verification

- `go test ./...` green; `go vet` clean
- `bash .agents/docs/testing/styrman/integration.sh` → passed=18 failed=0
- `sudo bash deploy/host-setup.sh --user kesokaj` on this host → idempotent, microVM boot verified
- `bash deploy/home-init.sh` → harness deployed for claude + agy into /var/lib/styrman/home
- Live smoke: `demo` (claude) and `demo-agy` (agy) projects created; Claude
  Code v2.1.246 onboarding and agy's Google sign-in both render over the WS
  terminal; keyring store/lookup verified inside a guest login shell
- Left running for the operator: both demo projects + a dev server on
  http://127.0.0.1:8080 (restart anytime: `STYRMAN_IMAGE=styrman-runtime:dev go run ./cmd/styrman/`)
