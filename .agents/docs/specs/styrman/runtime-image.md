# Runtime image spec (`image/`)

`styrman-runtime` is the per-project microVM image: **pure tooling, no server
binary** (ADR-002). Debian trixie, AMD64 only. Built locally:

```bash
docker build -t styrman-runtime:dev --build-arg STYRMAN_VERSION=<ver> image/
```

## Contents

- Base: git, gh, tmux, jq, ripgrep, fd, bat, build-essential, python3, vim/nano
- Docker CE + buildx + compose (per-project DinD, ADR-004)
- gcloud + gke-gcloud-auth-plugin + kubectl, helm, opentofu, yq
- Go (latest stable at build time), Node 22
- Agents: `claude` (npm `@anthropic-ai/claude-code`), `agy` (official installer → `/usr/local/bin/agy`)
- Keyring stack for agy: dbus, gnome-keyring, libsecret-tools (spike S4)
- e2fsprogs (project docker.img), tini (PID-1 reaper)
- `/etc/styrman-base-packages`: dpkg snapshot for drift debugging

## Entrypoint contract (`entrypoint.sh`, runs as root)

Inputs (env, set by the orchestrator):

| Env | Meaning |
|---|---|
| `STYRMAN_USER` / `STYRMAN_UID` / `STYRMAN_GID` | In-guest account (the user's configurable "terminal username"); uid/gid MUST match the host owner of the homes (spike S2) |
| `STYRMAN_PROJECT` | Project UUID — keys the docker disk and transcript dir |
| `STYRMAN_PROJECT_DIR` | Workspace dir name under ~/projects |
| `STYRMAN_MINIMAL` | System VMs (onboarding): skip workspace + DinD |
| `STYRMAN_DOCKER_DISK_GB` | Size for a newly created docker.img |
| `STYRMAN_KEYRING_PW` | Written to `/run/styrman/keyring-pw` (0400, user-owned), then unset |

Steps, in order:

1. Remount `/proc/sys` rw, restore `/etc/resolv.conf` (Kata has no Docker DNS).
2. cgroup-v2 nesting: remount `/sys/fs/cgroup` rw, create `init` group, enable
   subtree controllers (without this dockerd dies — spike S3).
3. mknod loop devices 0–7.
4. Create the user (matching uid/gid), passwordless sudo, docker group.
5. `~/projects/<dir>` and `~/.dockerd` (as the user; skipped in minimal mode).
6. Keyring plumbing: password file + `/etc/profile.d/styrman-keyring.sh`
   (starts session dbus + unlocked gnome-keyring on first login shell; keyring
   files persist at `~/.local/share/keyrings` on the shared home).
7. Default `~/.tmux.conf` if the user has none (status off, login shells).
7b. tmux transcript hook: pipe-pane every session to `~/.styrman/logs/<uuid>/` (ADR-017).
8. DinD (ADR-008): create sparse ext4 `~/.dockerd/<uuid>.img` if absent,
   loop-mount at `/var/lib/docker-project`, start dockerd with **overlay2**;
   fall back to **vfs** directly on the home if the loop mount fails.
9. `touch /run/styrman/ready` — the orchestrator's readiness probe target.
10. `exec sleep infinity` under tini (PID 1 reaps exec'd session zombies).

## Invariants

- Everything user-precious lives in `/home/<user>` (the virtiofs mount);
  the container itself is disposable at all times.
- Agent conversations are exec'd as `bash -lc 'exec <agent>'` so
  `/etc/profile.d` (keyring env) always applies.
- The image never contains secrets; `STYRMAN_KEYRING_PW` arrives via env and
  is unset after being written to the tmpfs file.
