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
- `/etc/styrman-base-packages`: dpkg snapshot — the DIFF BASE for apt persistence (ADR-028: user-installed packages are captured vs this at shutdown and replayed on boot), and still useful for drift debugging

## Entrypoint contract (`entrypoint.sh`, runs as root)

Inputs (env, set by the orchestrator):

| Env | Meaning |
|---|---|
| `STYRMAN_USER` / `STYRMAN_UID` / `STYRMAN_GID` | In-guest account (the user's configurable "terminal username"); uid/gid MUST match the host owner of the homes (spike S2) |
| `STYRMAN_PROJECT` | Project UUID — keys the docker disk and transcript dir |
| `STYRMAN_PROJECT_DIR` | Workspace dir name under ~/projects |
| `STYRMAN_MINIMAL` | System VMs (onboarding): skip workspace + DinD |
| `STYRMAN_DOCKER_DISK_GB` | Size for a newly created docker.img |
| `STYRMAN_CRED_BROKER` | Credential broker URL on this project's bridge (ADR-030/032); consumed by the git helper + login-shell snippet. Empty = broker off. |
| `STYRMAN_KEYRING_PW` | Written to `/run/styrman/keyring-pw` (0400, user-owned), then unset |

Steps, in order:

1. Remount `/proc/sys` rw, restore `/etc/resolv.conf` (Kata has no Docker DNS).
2. cgroup-v2 nesting: remount `/sys/fs/cgroup` rw, create `init` group, enable
   subtree controllers (without this dockerd dies — spike S3).
3. mknod loop devices 0–7.
4. Create the user (matching uid/gid), passwordless sudo, docker group. A
   reserved name (root, daemon, …) that already exists at a different uid is
   rejected — the boot fails loudly instead of every `docker exec` silently
   running as the wrong uid on a home it doesn't own (I11).
5. `~/projects/<dir>` and `~/.dockerd` (as the user; skipped in minimal mode).
6. Keyring plumbing: password file + `/etc/profile.d/styrman-keyring.sh`
   (starts session dbus + unlocked gnome-keyring on first login shell; keyring
   files persist at `~/.local/share/keyrings` in the PER-PROJECT home). Still
   installed, but the broker's keyring tier is dormant (see 6c) — agy does
   not use the guest keyring for its OAuth token (see Credentials below).
6b. Seed the per-project home (ADR-031) write-once from the image: shell
    dotfiles from `image/skel/` (`~/.bashrc`, `~/.profile`, `~/.bash_*`, and
    `~/.npmrc` — the last referencing `${NPM_TOKEN}` from the broker) and the
    agent harness from `image/harness/`.
6c. Credential materialization (ADR-032): install
    `/etc/profile.d/zz-styrman-credentials.sh` (sorts after the keyring
    snippet). On every login shell it pulls the CURRENT credentials for this
    project's user from the broker on the project bridge (`$STYRMAN_CRED_BROKER`)
    — `GET /env` → `export` each var, `GET /files` → write each file at its
    mode under `$HOME`. The same snippet also, after materializing:
    - applies `STYRMAN_GIT_NAME`/`STYRMAN_GIT_EMAIL` (env creds) as
      `git config --global user.name/user.email` — per-project homes carry no
      git identity by default (only the auth token is a broker tier), so
      without this a fresh project's `git commit` fails with "who are you"
      even though push/pull already authenticate (v1.7.1). Set as global
      config, not `GIT_AUTHOR_*` env, so a per-repo `user.name` still wins.
    - when `~/.claude/.credentials.json` is present, jq-merges
      `hasCompletedOnboarding: true` into `~/.claude.json` (creating a
      minimal file if absent) — a fresh per-project home otherwise has the
      flag unset even with a valid token, so interactive `claude` re-runs
      first-run onboarding and prompts for login despite already being
      signed in (`claude -p` is unaffected — it skips onboarding) (v1.5.6).
    - `GET /keyring` → `secret-tool store` each Secret Service item is still
      present but is now **dormant**: nothing currently harvests into the
      keyring tier (see agy below). Left in place, harmless, rather than
      ripped out; a future cleanup may remove it.
7. Default `~/.tmux.conf` if the user has none (status off, login shells,
   `mouse off` — mouse mode steals selection from the browser terminal, so
   it's off so xterm.js can select natively and copy-on-select works).
7b. tmux transcript hook: pipe-pane every session to `~/.styrman/logs/<uuid>/`
    (ADR-017). The generated `/etc/styrman/tmux-project.conf` also sets
    `monitor-bell on`, `bell-action any`, `visual-bell off` (unattended-bell
    notifications) and `set-clipboard on` (OSC 52, so tmux/TUIs can write the
    browser's clipboard through the terminal's OSC 52 handler).
8. DinD (ADR-008): create sparse ext4 `~/.dockerd/<uuid>.img` if absent,
   loop-mount at `/var/lib/docker-project`, start dockerd with **overlay2**;
   fall back to **vfs** directly on the home if the loop mount fails.
9. `touch /run/styrman/ready` — the orchestrator's readiness probe target.
10. Install a SIGTERM trap, then `sleep infinity &` and `wait` (not `exec`,
    so the shell stays alive to run the trap; tini stays PID 1 and still
    reaps exec'd session zombies). The trap stops dockerd cleanly and
    unmounts the loop-mounted ext4, so its journal is clean on the next boot
    (I9). Kata's `docker stop` powers the guest off without delivering
    SIGTERM to init, so the orchestrator's `Stop` triggers this itself first
    — `docker exec kill -TERM 1` — and waits briefly before falling through
    to `ContainerStop`.

## First-run onboarding (`image/configs/styrman-setup`)

Idempotent, interactive script run in the hidden setup VM (also re-runnable
in any project to check status or sign in to more tools). Split into
REQUIRED and OPTIONAL steps (v1.8.0):

- **Required** — git identity (`git config --global user.name/user.email`,
  prompted if unset) and an ssh key (generates `~/.ssh/id_ed25519` if
  absent; comments it with the user's git email, falling back to
  `user@styrman`, never the literal "styrman"). Neither can be skipped.
- **Optional**, each gated behind a `[y/N]` prompt (`ask_optional` — a
  non-interactive run, e.g. exec'd from a harvest, always answers no, so it
  never blocks): GitHub (`gh auth login --hostname github.com --git-protocol
  ssh --web`); Google Cloud, guarded by `command -v gcloud` (`gcloud auth
  login` for the account, then SEPARATELY `gcloud auth
  application-default login` for ADC — the ADC step is what terraform and
  the client libraries use, and the one the broker syncs); Claude
  (`claude`); Antigravity (`agy`). Skipping any of them is fine — rerun
  `styrman-setup` anytime to pick up where it left off; the completion
  message says credentials are shared with every project automatically
  (rewritten v1.5.5 — used to send the operator to a now-obsolete manual
  Settings → Credentials → Import step).

Finishing setup (`POST /api/setup/done`) harvests whatever was just signed
into straight into the credential broker (ADR-034, v1.5.4) before tearing
the VM down — see deploy.md's Credential broker section and api.md's Setup
section.

## Credentials materialized per project (broker tiers)

| Tool | Broker tier | Source in the shared/setup home | Notes |
|---|---|---|---|
| git (push/pull auth) | git helper (per-op; nothing stored in the project) | `gh auth token --hostname github.com` | A system git credential helper calls the broker per operation |
| gh (`GH_TOKEN`) | env | `gh auth token` | Same token as git, exported as a shell var |
| git identity | env (`STYRMAN_GIT_NAME`/`STYRMAN_GIT_EMAIL`) | shared-home `.gitconfig` | Applied as `git config --global user.name/user.email` on login (v1.7.1) — per-repo identity still overrides it |
| claude | file (`~/.claude/.credentials.json`) | same path | Carries a refresh_token; login-shell snippet also jq-merges the onboarding flag (v1.5.6, see entrypoint step 6c) |
| gcloud ADC | file (`~/.config/gcloud/application_default_credentials.json`) | same path | The `gcloud auth login` account login is NOT yet broker-synced |
| ssh | file — first of `id_ed25519` / `id_rsa` / `id_ecdsa` | `~/.ssh/` | Kata blocks agent-socket forwarding, so the private key itself is materialized into each project |
| npm | env (`NPM_TOKEN`) | `~/.npmrc`'s `_authToken` | `image/skel/.npmrc` references `${NPM_TOKEN}` |
| agy | file (`~/.gemini/antigravity-cli/antigravity-oauth-token`, 0600) | same path | **v1.5.5**: ADR-032a originally modelled this as a keyring (Secret Service) credential — wrong for this agy build, which persists a portable `{access_token, token_type, refresh_token, expiry}` JSON file, not a keyring item. Moved to the file tier, which also works from the minimal setup VM (no keyring daemon runs there) |
| — | keyring — **dormant** | — | The broker's keyring tier (`credstore.KeyringCred`, `GET /keyring`, the login snippet's `secret-tool store` block) is unused by any tool as of v1.5.5. Left in place, harmless, rather than ripped out; removal is a separate future cleanup |

## Invariants

- Everything user-precious lives in `/home/<user>` (the virtiofs mount);
  the container itself is disposable at all times.
- **Image upgrades apply on the next stop/start**: a stopped container whose
  rootfs came from an older build of the runtime image is recreated at boot
  (v1.5 review fix). Running containers keep their rootfs until stopped. The
  apt manifest (ADR-028) replays what the user installed.
- Agent conversations are exec'd as `bash -lc 'exec <agent>'` so
  `/etc/profile.d` (keyring env) always applies.
- The image never contains secrets; `STYRMAN_KEYRING_PW` arrives via env and
  is unset after being written to the tmpfs file.
