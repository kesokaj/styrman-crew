# The styrman guide

Everything about the project in one place: what it is, how it works, how to
install it from nothing, how to operate it, and how to develop it. Detail
specs: `api.md`, `terminal-protocol.md`, `runtime-image.md`, `deploy.md`;
decisions: `../../architecture/DECISIONS.md`; history: `../../changes/`.

## 1. What styrman is

A self-hosted, multi-user web platform for running AI coding agents in
isolated projects. Sign in with Google, create a project, and get browser
terminals inside a dedicated **Kata Containers microVM** — its own kernel,
its own Docker daemon, its own network. Both agents (`claude`, `agy`) are
preinstalled; run whichever you want, in as many terminals as you want.
Live at https://crew.styrman.xyz.

Load-bearing ideas:

1. **1 project = 1 microVM** (ADR-001), identified by a **UUID** (ADR-020) —
   containers are `styrman-<uuid>`, so friendly names are freely renamable
   and reusable. Projects cannot reach each other over the network, ever
   (R12, verified adversarially).
2. **1 project = 1 home** (ADR-031, revising ADR-019; R3). Each project
   mounts its OWN home at `/var/lib/styrman/homes/<user>/p/<uuid>`, seeded
   identically from the base image (dotfiles + agent harness) — no tool
   state leaks between projects. The workspace additionally lives in its own
   storage dir (`/var/lib/styrman/projects/<user>/<uuid>`, ADR-027),
   bind-mounted at `~/projects/<dir>`. The ONLY thing shared across a
   user's projects is credentials, via the broker (ADR-030/032): sign in
   once during first-run setup (or in any project) and every project stays
   current automatically, no separate import step (ADR-034). Containers
   stay disposable; users never see each other's worlds. Access is managed
   via `STYRMAN_ALLOWED_EMAILS` (+ admin-only API endpoints — the runtime
   user-management UI was removed in v1.3, operator Q4).
3. **Terminals** (ADR-016): durable records backed by tmux sessions — they
   survive tab closes and disconnects, get a GENERATED friendly name at
   creation (rename any time: header click or the sidebar pencil), carry an
   optional AUTOSTART command (runs when the session is created — first
   open and re-opens after pause/VM restart, never on reconnect — then
   drops to the shell; bolt badge in the sidebar), can be paused (session
   ends, record + transcript stay) and closed, singly or all at once. The shell
   exiting is presented the same way: a **Paused** state with a **Resume**
   action, not a dead terminal. Everything printed in them is transcribed to
   the home (ADR-017): the History tab. A thin **status bar** sits under every
   terminal (`GET /api/projects/{id}/stats`, polled every 4s): CPU %, mem,
   disk, process count, IP, uptime, and a `creds N synced` count with a
   tooltip of what the broker has provisioned into this project (v1.6.0/
   v1.8.1). Plain **Ctrl+C always sends SIGINT** (explicit `\x03`, bypassing
   both the browser's own copy-on-Ctrl+C and xterm's default so a lingering
   copy-on-select highlight can no longer swallow it); Cmd+C copies
   explicitly; paste is Ctrl/Cmd+V or Ctrl+Shift+V via the clipboard API.
   AltGr (Windows/Linux) and Option (macOS) compose non-US symbols (`@ $ {
   } [ ] € …`) instead of acting as Meta, and Nordic/Swedish dead keys (`~
   \` ^ ´ ¨`) are resolved directly rather than through xterm's buggy
   browser-IME path — see terminal-protocol.md's keyboard-handling section.
   A sidebar notification bell (badge,
   linking to the terminals waiting for input) surfaces terminals whose tmux
   window rang its bell while unattended — tmux clears the flag once the
   session is viewed. The terminal (`web/src/lib/terminal.ts`) copies on
   select: the clipboard is written the instant a selection is highlighted,
   not on an explicit Ctrl+C, because TUIs (claude, agy) redraw and wipe
   selections before a keypress can land — the runtime image's tmux
   `mouse off` default is what lets native browser selection work at all. (The v1.2 link-rescue bar was removed in v1.3 — copy-on-select
   superseded it.) Ctrl+Shift+F opens a scrollback
   search overlay; OSC 52 lets tmux/TUIs write the system clipboard
   directly. It renders via xterm's DOM renderer, not WebGL, so terminal
   text stays real, selectable, assertable DOM. Anything that lands on the
   clipboard is recorded in the Clipboard side-panel tab — session-scoped
   (sessionStorage, 30 entries, 1h TTL, never persisted or synced) since
   copy-on-select could capture a secret typed during setup (ADR-023).
4. **The side panel is project context**: Files (workspace + home; create/
   rename/delete files and folders; view + edit via a lazy-loaded
   CodeMirror 6 editor with syntax highlighting for ~20 languages including
   Java/C/C++/Rust/PHP/SQL/XML/Terraform (v1.7.2) — the editor window is
   resizable (size remembered in localStorage) and saving no longer closes
   it, just re-disables Save until the next edit (v1.7.2); markdown opens as
   a RENDERED preview with an Edit toggle; a **Format** button reformats with
   the file's real CLI formatter running INSIDE the project VM — `gofmt`,
   `prettier`, `black`/`ruff`, `rustfmt`, `shfmt`, `terraform fmt`/`tofu fmt`
   — except JSON, which still formats locally/offline (v1.7.3); upload by
   drag, download; destructive operations on platform-managed home paths are
   refused), Ports (what's
   listening, with process / inner-docker image, shareable
   `/p/<id>/<t>/port/<port>/` links; per-port sharing — adding a Google
   email mints a stable **private link** `https://<token>.private.…/`
   requiring sign-in + membership in the share list, ADR-026 — and public
   TTL links on `https://<token>.public.…/`, ADR-025), History
   (transcripts under their terminal's friendly name + FULL-TEXT SEARCH
   across all of them — everything you and the agents typed or saw), **Git**
   — a real git client (v1.7.0, see below), Clipboard
   (session-scoped — see above). The HOME surface is a dashboard: project
   cards with live transcript tails per running terminal (15s refresh,
   click to jump in); polling pauses app-wide in hidden browser tabs.
5. **Git is the substrate** (R13, ADR-022): every project workspace IS a git
   repository, no exceptions — `git init -b main` runs synchronously at
   creation, or a clone runs in-guest instead (new project dialog's "Clone a
   repo" option, so the user's ssh key/gh credentials apply; falls back to
   an empty repo on failure), and a boot-time sweep initializes any
   repo-less workspace it finds (adopted pre-R13 projects). The Git side
   panel tab is a real git client (v1.7.0): branch + ahead/behind, Push, a
   commit-message box, "Staged"/"Changes" file groups with per-file stage/
   unstage/discard, and a colored per-file diff viewer — every command runs
   inside the guest VM so the user's identity and the broker's credential
   helper apply, and commits are authored correctly (`git commit` in a
   terminal works too as of v1.7.1 — the broker syncs the git identity, not
   just the auth token). The AUTOMATIC checkpoint mechanism this section
   used to describe (a 10-min loop + snapshots on stop/pause/close) was
   REMOVED in v1.7.0 — backups are now expected to come from microVM volume
   snapshots of `/var/lib/styrman` (host infra: LVM/ZFS/restic — tracked in
   `TODO.md`, not yet built). The manual checkpoint API still exists
   backend-side but nothing in the UI calls it (api.md).

## 2. Architecture

```
Browser ──HTTPS──► styrman-caddy (host net): Google OAuth, TLS, Cloudflare DNS,
                   label discovery (vendored ingress, deploy/caddy)
                      │ X-Caddy-Email + X-Styrman-Ingress
                      ▼
   styrman-server (container; header-auth; no auth-bypass listener in prod)
     ├── embedded TanStack Start SPA (ADR-013)
     ├── identity → per-user space: homes/<user>, users/<user>/{projects,
     │   conversations,profile}.json  (ADR-019; no database, ADR-007)
     ├── REST (api.md) · WS /ws/term/{id}/{t} (terminal-protocol.md)
     ├── /p/{id}/{t}/port/{port}/ → proxy to apps inside the microVM
     └── Docker SDK ──► per-project Kata microVM  styrman-<uuid>  (mvm)
                          ├── tini → entrypoint → sleep; exec-based terminals
                          ├── dockerd: overlay2 on loop-mounted ~/.dockerd/<id>.img
                          ├── virtiofs: projects/<user>/<uuid> → ~/projects/<dir> (ADR-027)
                          ├── tmux conv-<n> per terminal (bash -l), piped to
                          │   ~/.styrman/logs/<id>/  (transcripts)
                          └── virtiofs: /home/<guestname> = homes/<user>/p/<uuid> (ADR-031)
```

- **Identity**: email → username slug (`ostling.simon@gmail.com` →
  `ostling-simon`); the *terminal username* inside guests defaults to the
  slug but is user-configurable (Settings → Account; applies to VMs started
  after the change).
- **State recovery** (ADR-007): containers carry `styrman.*` labels
  (project UUID, user, name) and are re-adopted at boot; legacy layouts are
  migrated automatically (single home → per-user; name-keyed → UUID).
- **Project creation is async**: the API returns instantly
  (`state:"creating"`); the microVM boots in the background (~3 s) and the
  first terminal attach waits on readiness.
- **Isolation** (R12): per-project bridges (`sdb-<uuid>`, named
  `sdb-<11hex>` at the Linux interface level via `config.BridgeNameFor` —
  Docker's default `br-<hash>` interface name would slip past the firewall
  match, which is exactly what happened until v1.8.3, see deploy.md) shared
  only with the server, which runs `ip_forward=0`; the server joins project
  networks in the background (a live join can drop in-flight connections —
  the preview proxy 307s once instead when it must join on demand). A
  project reaches the **internet and nothing else**: not another project
  (separate bridges + Docker's inter-bridge DROP + a DOCKER-USER rule
  dropping private-range FORWARD traffic — LAN, other bridges, cloud
  metadata), and not the host itself (an INPUT drop). Installed by
  `host-setup.sh`'s `styrman-firewall.sh` and kept applied by a self-healing
  `styrman-firewall.timer` (re-runs every 60s; delete-then-insert so a
  Docker restart or manual flush is undone within a minute) (ADR-029,
  hardened v1.8.3/v1.8.4).
- **Credential broker** (ADR-030/032): a credential-only listener (`:8091`,
  bridge-only, never published to the host) hands each project the CURRENT
  credential for a tool, identified by its bridge source IP — so a refresh in
  the one broker store propagates to every project. Tiers: `GET /git`
  (a system git helper pulls per-op, nothing stored in the project);
  `GET /env` (shell vars — `GH_TOKEN`, `NPM_TOKEN`, `STYRMAN_GIT_NAME`/
  `STYRMAN_GIT_EMAIL` — exported/applied by a login-shell snippet,
  `/etc/profile.d/zz-styrman-credentials.sh`, which also jq-merges claude's
  `hasCompletedOnboarding` flag so a materialized token doesn't re-trigger
  first-run login, v1.5.6); and `GET /files` (files — gcloud ADC, ssh key,
  claude's `.credentials.json`, and agy's OAuth token file — written into
  the home by the same snippet). The `GET /keyring` tier (agy's OAuth via
  Secret Service, ADR-032a) is now **dormant**: agy's real build stores its
  token in a plain file, not the keyring — file tier since v1.5.5. Finishing
  first-run setup now seeds the broker automatically from whatever the user
  just signed into (ADR-034, v1.5.4) — no separate import step required;
  `POST /api/settings/credentials/import {project}` remains as a
  supplementary/refresh path. Manage from the Settings **Credentials**
  panel (consolidated status-colored view, v1.8.2 — see below).
- **Idle lifecycle**: terminal traffic timestamps the project; idle ones
  stop after `STYRMAN_IDLE_TIMEOUT` (24h); reopening boots on connect.
- **Caddy's ingress upstream is chosen deterministically**: the server
  container is multi-homed (the ingress network plus each project's `sdb-*`
  bridge, for the broker + preview proxy), so a naive "first network" pick
  is random-order and can resolve to a dead project-bridge IP — this caused
  a live 502 (v1.8.5). `extractContainerIP` now prefers `styrman-ingress`,
  then any non-`sdb-*` network, falling back to whatever's there (correct
  for a preview VM, which has only its own `sdb-*` network) — see deploy.md.
- **Auth is two-layer** (R8): the ingress proves identity via Google OAuth
  (`X-Caddy-Email`) and a shared, constant-time-compared `X-Styrman-Ingress`
  token proves the request actually came through Caddy — a project VM on
  the server's shared bridge can't reach `:8080` and spoof either header;
  the server then re-checks the email against its own userstore. Production
  runs no auth-bypass dev listener (`STYRMAN_DEV_ADDR` is opt-in, must bind
  loopback, and is unset in the compose deploy — 2026-08-27 security
  review).
- **File ops are confined** (`internal/files`, `os.Root`): every
  home-relative read/write is rooted at the user's home and refuses any
  symlink that would leave it, atomically at every path component — the
  tenancy boundary for guests that can otherwise symlink freely inside
  their own home.
- **Sessions survive redeploys**: caddy-security's JWT signing key is
  pinned via `STYRMAN_JWT_KEY` (`host-setup.sh` generates it) instead of
  rotating per boot, so restarting Caddy doesn't log everyone out.

## 3. Install from scratch (R11)

Baremetal Debian 13 or a GCP VM with nested virtualization.

```bash
git clone <repo> styrman && cd styrman
sudo bash deploy/host-setup.sh --user $USER   # Docker, Kata "mvm", daemon cfg, dirs, seeds deploy/.env
docker build -t styrman-runtime:dev image/    # runtime image (~10 min first time)

# Dev (no auth, loopback):
STYRMAN_IMAGE=styrman-runtime:dev go run ./cmd/styrman/   # → http://127.0.0.1:8080

# Production (Google OAuth + HTTPS + reboot-safe):
vi deploy/.env    # BASE_DOMAIN, GOOGLE_CLIENT_ID/SECRET, CLOUDFLARE_API_TOKEN,
                  # ADMIN_EMAIL, STYRMAN_ALLOWED_EMAILS (redirect URI:
                  # https://auth.<domain>/oauth2/google/authorization-code-callback)
docker compose -f deploy/docker-compose.yml up -d --build
# DNS records + certs are automatic → https://<STYRMAN_HOSTNAME>.<domain>
```

First use per user: the sidebar shows a **First-time setup** banner →
"Open setup" boots a hidden minimal microVM running the guided
`styrman-setup`. Only git identity and an ssh key are REQUIRED; GitHub,
Google Cloud (account login + separately Application Default Credentials),
Claude, and agy are each OPTIONAL — `[y/N]` prompts, skip any and rerun
`styrman-setup` anytime, in the setup VM or any project terminal, to pick up
where you left off (v1.8.0). The generated SSH key's comment is the user's
git email (fallback `user@styrman` if none is set), not "styrman". Closing
the setup terminal (typing `exit`, or letting the script finish)
auto-closes the VM and seeds the credential broker from whatever was just
signed into — every project, current and future, is current with zero
re-auth, no manual import step (ADR-034, v1.5.4). Settings → Credentials →
**Import from a project** still exists as an optional refresh path (a
rotated token, or a tool signed into inside a project instead of at setup).

## 4. Operating it

- **New project**: name + repo choice (empty git repo, clone an existing
  one, or create+push a fresh GitHub repo via `gh`) → ~3 s to a live
  terminal; type `claude` or `agy`. Every workspace is a git repo from
  creation, no exceptions (R13). Rename any time (⋯ → Rename); delete via
  the type-"delete" dialog (wipe checkbox controls files).
- **Git**: the Git tab is a real client (v1.7.0) — branch bar with Push, a
  commit-message box, "Staged"/"Changes" file lists with per-file stage/
  unstage/discard, and a colored per-file diff viewer. (Automatic
  checkpoint snapshots were REMOVED in v1.7.0 — see §1 idea 5. Back up via
  microVM volume snapshots instead, once that host tooling exists.)
- **Terminals**: sidebar, per project; generated friendly names; ⏸ pause /
  ✕ delete / close all; per-terminal autostart command (pencil → dialog;
  runs at session creation, then drops to the shell); transcripts land in
  History regardless; a status bar under the terminal shows live CPU/mem/
  disk/IP/uptime and synced-credential count (v1.6.0/v1.8.1).
- **Settings**: account + terminal username, a single status-colored
  **Credentials** view (v1.8.2, replacing the old separate "Tools &
  credentials" cards) — GREEN = signed in and synced to the broker, GREY =
  not set up, RED = signed in locally but not yet synced; each row is
  clickable to read its stored value (v1.8.1); an "Add a variable" row
  stores any custom env var (NPM_TOKEN, DATABASE_URL, …); "Import from a
  project" is marked optional (setup already auto-syncs, ADR-034) — theme
  (Light / Dark / System), and — admins only — LIVE runtime tunables:
  idle-stop timeout (applies to the reaper immediately) and docker disk
  size for new project VMs, persisted in `StateDir/settings.json` over the
  env defaults. (The user-access card was removed in v1.3 — access = env +
  admin API.)
- **Where things live** on the host:

| Path | What |
|---|---|
| `/var/lib/styrman/homes/<user>/p/<uuid>` | A project's OWN home (ADR-031): dotfiles, tool state, docker disk, transcripts — back up |
| `/var/lib/styrman/projects/<user>/<uuid>` | Workspaces (ADR-027, per-project storage) — back up TOO: source code no longer lives in the home |
| `…/homes/<user>/p/<uuid>/.dockerd/<uuid>.img` | Per-project Docker disk (sparse) |
| `…/homes/<user>/p/<uuid>/.styrman/logs/<uuid>/` | Terminal transcripts |
| `/var/lib/styrman/credentials/<user>/` | The broker's credential store (ADR-032, 0600) — back up |
| `/var/lib/styrman/users/<user>/*.json` | Project/terminal/profile metadata |
| `/var/lib/styrman/users.json` | Runtime-added accounts (admins are env-only) |
| containers `styrman-<uuid>`, networks `sdb-<uuid>` | Disposable runtime state |

Deleting a project WITHOUT "wipe files" keeps its workspace dir under
`/var/lib/styrman/projects/<user>/` (that's the point), but nothing in the
UI lists it anymore — reclaim disk by removing it manually if you never
re-import it.

- **Reboots**: containers restart themselves (`unless-stopped`); the server
  re-adopts from labels. **Disaster recovery**: fresh host → host-setup →
  restore `/var/lib/styrman` → rebuild images.

## 5. Troubleshooting

| Symptom | Cause / fix |
|---|---|
| `invalid namespace type` starting any microVM | Docker ≥29.5 time-namespaces vs Kata ≤3.31 — host-setup sets the daemon.json feature flag (ADR-009). |
| Site unreachable but server fine | Check DNS first — a "newly registered domain" blocklist (NextDNS et al.) sinkholed styrman.xyz once (`dig` for 0.0.0.0 answers). |
| dockerd inside a project uses `vfs` | Loop mount of `~/.dockerd/<uuid>.img` failed; see container logs (ADR-008 fallback). |
| agy/claude ask to sign in despite a synced credential | A tool's TOKEN can sync while its per-project CLIENT STATE doesn't (e.g. claude's `hasCompletedOnboarding` — fixed for new logins since v1.5.6). If it's an OLDER project VM, recreate it to pick up a runtime-image fix; a running container keeps the rootfs it booted with until stopped. |
| `git commit` in a terminal fails "who are you" | The user's git identity hasn't reached the broker yet — rerun `styrman-setup` (or `Settings → Credentials → Import`) so `STYRMAN_GIT_NAME`/`STYRMAN_GIT_EMAIL` get harvested, then open a fresh login shell (v1.7.1). |
| Terminal shows "replaced" | Same terminal opened in another tab (close 4001) — one attach each, by design. |
| First port-preview 307s or drops once | The server just joined that project's network; the redirect/retry lands. |
| Project won't start | `docker logs styrman-<uuid>` — the entrypoint logs every step. |

## 6. Development

```bash
go test ./...                                        # unit (stores, guards, routing)
bash .agents/docs/testing/styrman/integration.sh     # 62 checks vs real Kata
bash web/e2e/run.sh                                  # 35 browser checks (Playwright, self-hosted)
cd web && npm run build                              # UI → dist/client (committed)
```

Layout: `cmd/styrman` (server) · `cmd/wsprobe` (WS test client) ·
`internal/{config, identity, userstore, project, conversation, orchestrator,
terminal, httpapi, files, history, gitrepo, links, credstore, credbroker}` · `image/` (runtime microVM
image) ·
`deploy/` (host-setup, compose, vendored caddy ingress, server Dockerfile) ·
`web/` (TanStack Start SPA). Conventions: `AGENTS.md`; non-negotiables:
`REQUIREMENTS.md` (R1–R13).

## 7. Roadmap

Remaining candy: transcript rotation, ops panel (disk/logs per project),
git badges in the sidebar, agy native session resume, moby SDK migration,
backup automation for `/var/lib/styrman`. See `TODO.md`.

Lineage: combines `devbox` (Kata multi-tenant shell platform; ingress,
terminal protocol) and `argolis-devbox` (single-VM cloud shell). Full
history: `.agents/docs/plans/` and `.agents/docs/changes/`.
