# Phase 0 spike results

Date: 2026-08-26 · Host: NUC12WSHi7, Debian 13 (6.12.101), Docker 29.7.2, Kata 3.31.0 (`mvm`, QEMU/virtio-fs)

All five spikes **passed** (S0 was an unplanned prerequisite fix). Repro commands are inline; no spike code was kept — everything below is reproducible with stock images.

## S0 (unplanned) — Docker 29 broke Kata: `invalid namespace type`

Any `--runtime mvm` run failed with `failed to create shim task: invalid namespace type`.
Root cause: Docker 29.5.0+ enables private **time namespaces** by default
(moby/moby#52326); Kata ≤ 3.31 guest agent rejects the `time` namespace type
(kata-containers/kata-containers#13080). devbox ran Docker 28.5.2 — the host's Docker
was upgraded since.

**Fix applied** to `/etc/docker/daemon.json` (backup at `daemon.json.bak-20260826`):

```json
"features": { "time-namespaces": false }
```

**Consequence:** this flag is mandatory host config → belongs in `deploy/host-setup.sh`
and the docs. Revisit when Kata ships time-ns support.

## S1 — exec/TTY streaming into Kata: PASS

`docker exec` into an `mvm` container works, including TTY mode (verified via
`script -qec 'docker exec -it …'`). Round-trip for a no-op exec: **~31 ms** including
docker CLI startup — the SDK-attached stream used by the terminal bridge will be lower.
Guest kernel 6.18.28 vs host 6.12.101 confirms real microVM isolation.
→ ADR-002 (exec transport, no in-container server) stands.

## S2 — concurrent shared virtiofs home: PASS

Two `mvm` microVMs bind-mounting the same host dir: cross-VM write visibility ≤ 41 ms
(measurement floor = exec overhead), bidirectional writes clean, files intact on host.
→ ADR-003 (single shared home) stands, with the write-discipline rule.
**Note:** container-root writes land as `root` on the host — the runtime image MUST run
the user at the host owner's UID/GID (devbox entrypoint pattern).

## S3 — DinD storage driver on virtiofs: PASS (with a design upgrade)

| Variant | Result |
|---|---|
| `overlay2` data-root directly on virtiofs | **FAIL** — `failed to mount overlay: invalid argument` (expected; why devbox chose vfs) |
| `vfs` data-root on virtiofs (devbox approach) | works, but vfs = full file copies per layer: slow, disk-hungry |
| **sparse ext4 image file on virtiofs, loop-mounted in-guest, `overlay2` on top** | **WORKS** — `docker run hello-world` clean; `docker.img` persists on host |

Decision: per-project `~/.dockerd/<project>.img` (sparse, default 20 G, configurable),
loop-mounted by the entrypoint, overlay2 inside. Prereqs verified: remount
`/sys/fs/cgroup` rw + cgroup-v2 nesting (devbox entrypoint lines 75–87) — without it
dockerd dies at `mkdir /sys/fs/cgroup/init: Read-only file system`; loop device mknod.
vfs remains a config fallback. → new ADR-008.

## S4 — agy + headless keyring: PASS

- `agy 1.1.21` installs in a Debian trixie image via the official install script and runs.
- Headless Secret Service works: `dbus-run-session` +
  `eval "$(printf '<pw>' | gnome-keyring-daemon --unlock --replace --components=secrets)"`
  creates/unlocks `login.keyring`; `secret-tool` store/lookup OK; keyring files live in
  `~/.local/share/keyrings/` → persisted via the shared home → **survives container
  restart** with no re-auth.
- Gotchas: password must be non-empty (empty → GUI prompter → crash, no X); use
  `dbus-run-session` (plain `dbus` package), not `dbus-launch` (needs dbus-x11).
- Keyring password: fixed value from host `.env`, injected per-container. It's
  convenience-grade at-rest protection (password and data share a disk) — documented.
- Still deferred to Phase 1 with a human: the actual `agy` Google sign-in (needs the
  operator's browser). Infrastructure proven; `GEMINI_API_KEY` remains the escape hatch.

## S5 — Kata cold-start: PASS

`docker run --runtime mvm --rm debian:trixie true`: **1.03–1.10 s** over 3 runs.
Budget to agent prompt: + image size + entrypoint (user setup, loop mount, dockerd,
keyring, tmux). The < 5 s target is comfortably plausible; re-measure with the real
runtime image in Phase 1.
