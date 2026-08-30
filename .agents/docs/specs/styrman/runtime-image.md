# Runtime image spec (`image/`)

`styrman-runtime` is the per-project microVM image: **pure tooling, no server
binary** (ADR-002). Debian trixie, AMD64 only. Built locally:

```bash
docker build -t styrman-runtime:dev --build-arg STYRMAN_VERSION=<ver> image/
```

## Contents

- Base: git, gh, tmux, jq, ripgrep, fd, bat, build-essential, python3, vim/nano
- Docker CE + buildx + compose (per-project DinD, ADR-004)
- gcloud + gke-gcloud-auth-plugin + kubectl, helm, yq
- OpenTofu, via the official `get.opentofu.org` installer script
  (`--install-method standalone`), which verifies the release signature
  itself before installing (M32)
- Go (latest stable at build time), Node 22
- Agents: `claude` (npm `@anthropic-ai/claude-code`), `agy` (official installer → `/usr/local/bin/agy`)
- dbus, gnome-keyring, libsecret-tools — originally installed for agy's Secret
  Service (spike S4, ADR-032a); **removed from the image with ADR-040**:
  agy's real build never touched the keyring, and the broker's keyring tier
  that would have populated it was deleted outright, so `image/Dockerfile`
  no longer installs them (its own comment records the removal). No keyring
  daemon is started at login (no `profile-keyring.sh` step remains)
- e2fsprogs (project docker.img), tini (PID-1 reaper)
- `/etc/styrman-base-packages`: dpkg snapshot — the DIFF BASE for apt persistence (ADR-028: user-installed packages are captured vs this at shutdown and replayed on boot), and still useful for drift debugging

## Entrypoint contract (`entrypoint.sh`, runs as root)

Inputs (env, set by the orchestrator):

| Env                                            | Meaning                                                                                                                             |
| ---------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------- |
| `STYRMAN_USER` / `STYRMAN_UID` / `STYRMAN_GID` | In-guest account (the user's configurable "terminal username"); uid/gid MUST match the host owner of the homes (spike S2)           |
| `STYRMAN_PROJECT`                              | Project UUID — keys the docker disk and transcript dir                                                                              |
| `STYRMAN_PROJECT_DIR`                          | Workspace dir name under ~/projects                                                                                                 |
| `STYRMAN_MINIMAL`                              | System VMs (onboarding): skip workspace + DinD                                                                                      |
| `STYRMAN_DOCKER_DISK_GB`                       | Size for a newly created docker.img                                                                                                 |
| `STYRMAN_CRED_BROKER`                          | Credential broker URL on this project's bridge (ADR-030/032); consumed by the git helper + login-shell snippet. Empty = broker off. |

Steps, in order:

1. Remount `/proc/sys` rw, restore `/etc/resolv.conf` (Kata has no Docker DNS).
2. cgroup-v2 nesting: remount `/sys/fs/cgroup` rw, create `init` group, enable
   subtree controllers (without this dockerd dies — spike S3).
3. mknod loop devices 0–7.
4. Create the user (matching uid/gid), passwordless sudo, docker group. A
   reserved name (root, daemon, …) that already exists at a different uid is
   rejected — the boot fails loudly instead of every `docker exec` silently
   running as the wrong uid on a home it doesn't own (I11).
5. `~/projects/<dir>`, `~/.dockerd`, `~/.config`, `~/.cache`, `~/.local/bin`
   (as the user; skipped in minimal mode). `~/.ssh` (mode 700) is created
   unconditionally, even in minimal mode. (The `~/.local/share/keyrings` mkdir
   and the dbus/gnome-keyring packages were dropped with the keyring tier,
   ADR-040.)
6. Seed the per-project home (ADR-031) write-once from the image: shell
   dotfiles from `image/skel/` (`~/.bashrc`, `~/.profile`, `~/.bash_*`, and
   `~/.npmrc` — the last referencing `${NPM_TOKEN}` from the broker) and the
   agent harness from `image/harness/`.
   6b. Login-shell `/etc/profile.d` snippets: `zzz-styrman-motd.sh` (zzz- so it
   runs LAST, after credentials are materialized — it probes
   `~/.claude/.credentials.json` etc. and would otherwise nag "run
   styrman-setup" on a fully-onboarded project) and
   `zz-styrman-credentials.sh` (ADR-032, credential materialization, below).
   **There is no keyring snippet anymore** — the former
   `styrman-keyring.sh`/`profile-keyring.sh` step (session dbus + gnome-keyring
   unlock) is deleted outright (ADR-040); no keyring daemon is started at
   login.
   6c. Credential materialization (`zz-styrman-credentials.sh`, ADR-032). Skipped
   entirely if `STYRMAN_SKIP_CRED_PULL` is set (the harvest/import exec sets
   this so it reads the guest's REAL current files instead of the broker's
   own copy — H6). Otherwise, on every login shell:
   - Derives the broker's address from the VM's OWN current IP (broadcast−1
     of its `/16`) rather than trusting `$STYRMAN_CRED_BROKER` baked into the
     container env at creation time — a network recreation can hand the
     bridge a new subnet, which would otherwise strand the project on a
     stale broker IP (M17). Falls back to the injected value if its own
     address can't be read.
   - `GET /env` → `export` each var. The broker merges any PROJECT-SCOPED
     variables (SidePanel → Env, ADR-043) over the user-wide set for the
     calling project — the guest snippet is unchanged; scoping happens
     entirely broker-side via the caller's IP→(user,project) resolution.
     `GET /files` → writes each file under
     `$HOME` at its mode, via an `umask 077` temp file + atomic rename (never
     a torn or briefly-world-readable credential file).
   - applies `STYRMAN_GIT_NAME`/`STYRMAN_GIT_EMAIL` (env creds) as
     `git config --global user.name/user.email` — per-project homes carry no
     git identity by default (only the auth token is a broker tier), so
     without this a fresh project's `git commit` fails with "who are you"
     even though push/pull already authenticate (v1.7.1). Set as global
     config, not `GIT_AUTHOR_*` env, so a per-repo `user.name` still wins.
   - when `~/.claude/.credentials.json` is present, jq-merges
     `hasCompletedOnboarding: true` into `~/.claude.json` (creating a
     minimal file if absent), writing atomically via a temp file + rename so
     a concurrent `claude` never reads a torn file — a fresh per-project home
     otherwise has the flag unset even with a valid token, so interactive
     `claude` re-runs first-run onboarding and prompts for login despite
     already being signed in (`claude -p` is unaffected — it skips
     onboarding) (v1.5.6).
   - There is no `GET /keyring` step — that broker tier and this snippet's
     former `secret-tool store` block are both deleted (ADR-040).
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
REQUIRED and OPTIONAL steps (v1.8.0), with two invocation modes (v1.10.0):

- `styrman-setup` — the full flow: required git identity + ssh key, then
  each optional tool in turn. Unchanged behavior.
- `styrman-setup <tool>...` — run ONLY the named step(s) (`git|ssh|gh|
gcloud|claude|agy`). When the target isn't git/ssh itself, the git
  identity is ensured quietly first (it's the prerequisite for the ssh key
  comment / a usable env). This is what the "add one credential later" flow
  in Settings → Credentials runs, via `GET /ws/setup?tool=<tool>` — see
  terminal-protocol.md and api.md's Setup section.

**Ctrl+C exits cleanly** (`trap 'printf "\n"; exit 0' INT`, v1.10.0).
Previously SIGINT killed the script by signal, and the browser terminal's
reconnect (`tmux new-session -A`) picked the ended session back up, which
RESTARTED `styrman-setup` from the top. The trap's `exit 0` now ends the
pane cleanly instead — and belt-and-braces, the client no longer
auto-reconnects a setup pane that has been open once (v1.10.1, see
terminal-protocol.md), so a restart is impossible even when the bridge's
1 s exit-code poll loses the race and reports a transient close.

**Keep / renew** (v1.10.1): every step is idempotent AND renewable. A
credential that already exists shows ✔ and offers `renew? [y/N]` — Enter
keeps it and moves on to the next step, `y` re-runs that sign-in (for the
ssh key, renewal generates a NEW key and keeps the old pair as
`id_ed25519.replaced` — the broker only harvests `id_ed25519` itself, and
the prompt warns the old key must be replaced wherever it's registered).
All status PROBES (`gh auth status`, `gcloud auth list`, `gcloud auth
application-default print-access-token`, `claude auth status`) run with
stdin `</dev/null` so none can swallow the user's typed-ahead answers to
later prompts (found by a pty test: `claude auth status` ate buffered
input, stranding the flow at its prompt).

- **Required** — git identity (`git config --global user.name/user.email`,
  prompted if unset) and an ssh key (generates `~/.ssh/id_ed25519` if
  absent; comments it with the user's git email, falling back to
  `user@styrman`, never the literal "styrman"). Neither can be skipped
  when absent — when present, the keep/renew prompt above applies.
- **Optional**, each gated behind a `[y/N]` prompt (`ask` — a
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

Finishing setup harvests whatever was just signed into straight into the
credential broker (ADR-034, v1.5.4), via one of two routes (v1.10.0):
`POST /api/setup/done` — the full first-run completion: harvest, write
`~/.styrman/setup-done`, and destroy the VM — or `POST /api/setup/harvest`
— the "add a credential" path: harvest only, no done-marker, VM left
running so the next tool can be signed into without rebooting. See
deploy.md's Credential broker section and api.md's Setup section.

## Credentials materialized per project (broker tiers)

| Tool                 | Broker tier                                                                                  | Source in the shared/setup home       | Notes                                                                                                                                                                                                                                                                                                                             |
| -------------------- | -------------------------------------------------------------------------------------------- | ------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| git (push/pull auth) | git helper (per-op; nothing stored in the project)                                           | `gh auth token --hostname github.com` | A system git credential helper (`image/configs/styrman-git-credential`) calls the broker per operation; parses the JSON response with `jq` (base image has it), falling back to a `sed` extraction only right for simple unescaped tokens                                                                                         |
| gh (`GH_TOKEN`)      | env                                                                                          | `gh auth token`                       | Same token as git, exported as a shell var                                                                                                                                                                                                                                                                                        |
| git identity         | env (`STYRMAN_GIT_NAME`/`STYRMAN_GIT_EMAIL`)                                                 | shared-home `.gitconfig`              | Applied as `git config --global user.name/user.email` on login (v1.7.1) — per-repo identity still overrides it                                                                                                                                                                                                                    |
| claude               | file (`~/.claude/.credentials.json`)                                                         | same path                             | Carries a refresh_token; login-shell snippet also jq-merges the onboarding flag (v1.5.6, see entrypoint step 6c)                                                                                                                                                                                                                  |
| gcloud ADC           | file (`~/.config/gcloud/application_default_credentials.json`)                               | same path                             | Application-default creds — terraform + client libraries                                                                                                                                                                                                                                                                          |
| gcloud CLI (account) | file (`~/.config/gcloud/credentials.db` + `active_config` + `configurations/config_default`) | same paths                            | `gcloud auth login` account + active config, so `gcloud <command>` is authenticated in every project (v1.9.3). Only the DEFAULT config is synced; `access_tokens.db` is a cache gcloud regenerates and is intentionally not synced                                                                                                |
| ssh                  | file — first of `id_ed25519` / `id_rsa` / `id_ecdsa`                                         | `~/.ssh/`                             | Kata blocks agent-socket forwarding, so the private key itself is materialized into each project                                                                                                                                                                                                                                  |
| npm                  | env (`NPM_TOKEN`)                                                                            | `~/.npmrc`'s `_authToken`             | `image/skel/.npmrc` references `${NPM_TOKEN}`                                                                                                                                                                                                                                                                                     |
| agy                  | file (`~/.gemini/antigravity-cli/antigravity-oauth-token`, 0600)                             | same path                             | **v1.5.5**: ADR-032a originally modelled this as a keyring (Secret Service) credential — wrong for this agy build, which persists a portable `{access_token, token_type, refresh_token, expiry}` JSON file, not a keyring item. Moved to the file tier, which also works from the minimal setup VM (no keyring daemon runs there) |

There is no keyring tier: the broker's keyring tier (`credstore.KeyringCred`/
`PutKeyring`, `GET /keyring`, the login snippet's `secret-tool store` block)
was left in place as dormant code after the v1.5.5 agy move, then **deleted
outright** in the 2026-08-30 security review (ADR-040) — no store kind,
broker endpoint, or shell step references it anymore.

## Invariants

- Everything user-precious lives in `/home/<user>` (the virtiofs mount);
  the container itself is disposable at all times.
- **Image upgrades apply on the next stop/start**: a stopped container whose
  rootfs came from an older build of the runtime image is recreated at boot
  (v1.5 review fix). Running containers keep their rootfs until stopped. The
  apt manifest (ADR-028) replays what the user installed.
- Agent conversations are exec'd as `bash -lc 'exec <agent>'` so
  `/etc/profile.d` (the credential-materialization snippet) always applies.
- The image never contains secrets; credentials arrive over the broker on
  the project's own bridge at each login (ADR-030/032) — never baked into
  the image, never passed to the guest via container env.
