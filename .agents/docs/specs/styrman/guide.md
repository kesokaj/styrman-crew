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
   copy-on-select highlight can no longer swallow it); Cmd+C or Ctrl+Shift+C
   copies explicitly; paste is Ctrl/Cmd+V or Ctrl+Shift+V via the clipboard API.
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
   Java/C/C++/Rust/PHP/SQL/XML/Terraform (v1.7.2) — the editor is a VIEW of
   the terminal's MAIN pane, not a separate modal (2026-09-01, replacing the
   earlier centered file-viewer modal): a `Terminal | Editor` control in the
   terminal header toggles the pane, state is kept per terminal (one
   terminal can sit on the editor while another in the same project shows a
   shell), and the terminal keeps running underneath either way (hidden with
   CSS, never unmounted, so its session survives the toggle). Clicking a
   file in the Files panel opens it in the pane and switches the view there;
   saving no longer closes it, just re-disables Save until the next edit
   (v1.7.2); a **Wrap** toggle controls soft-wrap (default on, persisted
   across sessions); markdown opens as
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
   TTL links on `https://<token>.public.…/`, ADR-025), **Env** —
   project-scoped env variables (v1.12.0, ADR-043): like Settings' custom
   variables but delivered ONLY to this project's new login shells; a
   project variable overrides a same-named global one, deleting it falls
   back to the global, and the variables die with the project. Click a name
   to reveal its value (audit-logged); stored via the broker's env tier so
   NO guest change was needed — the broker merges them per calling project
   — History (transcripts under their terminal's friendly name + FULL-TEXT
   SEARCH across all of them — everything you and the agents typed or saw),
   **Git** — a real git client (v1.7.0, see below), Clipboard
   (session-scoped — see above). The HOME surface is a dashboard: project
   cards with live transcript tails per running terminal (15s refresh,
   click to jump in), plus an OPS ROW per running card (v1.14.0):
   cpu/mem/disk/procs chips and a Logs dialog with the microVM console
   tail. The sidebar shows per-project GIT BADGES (branch, amber ±N when
   dirty — one batched /api/git/summary call). One shared listProjects
   poll feeds every surface; polling pauses app-wide in hidden tabs, and
   an expired session triggers one toast + reload into the OAuth flow.
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
   REMOVED in v1.7.0; the MANUAL checkpoint API that survived that change was
   then deleted outright in the 2026-08-30 security review (ADR-039) — it,
   and the standalone host-side `/diff` route, ran host-side `git` against a
   bind-mounted, guest-writable workspace, a guest→server RCE. All that
   remains host-side is a hardened `git init`/`git rev-parse`
   (`internal/gitrepo`), run with config overrides that neutralize a
   guest-controlled `.git/config`. Backups are now two layers, both live:
   in-app per-project archives (ADR-044, §2 below) plus a daily whole-state
   restic→GCS snapshot of `/var/lib/styrman` (v1.14.0) — see deploy.md's
   "Host-layer backups" section.

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
  metadata), and not the host itself (an INPUT drop) — plus a `physdev
  --physdev-is-bridged` exemption so a project can still reach the
  credential broker and port-preview proxy on its OWN bridge (same-bridge L2
  traffic otherwise also traverses the isolation chain). Installed by
  `host-setup.sh`'s `styrman-firewall.sh` into dedicated `STYRMAN-IN`/
  `STYRMAN-FWD` chains, kept applied by a self-healing `styrman-firewall.timer`
  (re-runs every 60s; INSERT-IF-MISSING, never delete-then-insert, so there is
  no window where isolation is briefly absent) (ADR-029, reworked 2026-08-30
  — see deploy.md).
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
  the home by the same snippet). That's all three tiers: the fourth
  `GET /keyring` tier ADR-032a added for agy (Secret Service) turned out to
  not match agy's real build (a plain file, ADR-038) and was left dormant,
  then deleted outright in the 2026-08-30 security review (ADR-040) — no
  broker code, store kind, or shell step references it anymore. Finishing
  first-run setup now seeds the broker automatically from whatever the user
  just signed into (ADR-034, v1.5.4) — no separate import step required;
  `POST /api/settings/credentials/import {project}` remains as a
  supplementary/refresh path. Manage from the Settings **Credentials**
  panel (consolidated status-colored view, v1.8.2 — see below).
- **Idle lifecycle** (v1.11.0): terminal traffic (keystrokes + output on
  ATTACHED terminals) timestamps the project; candidates older than
  `STYRMAN_IDLE_TIMEOUT` (default 8h — a stop only powers the VM off,
  everything persists and the next attach resumes exactly where it left
  off) get a final in-guest busy probe before the plug is pulled: any
  RUNNING DinD container (`docker ps`), or any tmux pane whose foreground
  process isn't a plain shell (a detached build, claude still working),
  blocks the stop and resets the idle clock. Reopening a terminal boots on
  connect.
- **Per-project backups** (v1.13.0, ADR-044): the sidebar ⋯ menu's
  **Backup** snapshots the workspace + project home (docker disk excluded —
  reproducible cache) into a server-side tar.gz; works while running
  (crash-consistent). **Settings → Backups** lists them all with
  restore/delete: restore replaces the STOPPED project's files
  transactionally (current state is quarantined and renamed back if the
  extraction fails — root-assisted via the server's own container where
  dockerd stubs or guest-`sudo` files are root-owned). A deleted project's
  backups survive and can seed a new project: **"From backup"** in the
  create dialog clones workspace + home + the source's project env vars.
  The archive itself does NOT contain the project-scoped env variables —
  "From backup" copies them live from the source project's broker-side
  store at creation time, so they only transfer while the source project
  still exists (deleting a project wipes its scoped env set too); a
  deleted project's backup restores workspace + home with zero scoped
  vars. Since v1.14.0 backups are also AUTOMATIC: a cadence pass backs up
  every project each `STYRMAN_BACKUP_CADENCE` (default 24h) keeping the
  newest `STYRMAN_BACKUP_KEEP` (5) per project, and a wipe-delete takes
  a pre-wipe safety backup FIRST, aborting the wipe if it fails. The
  disaster-recovery layer is live too: daily whole-state restic
  snapshots to GCS Stockholm (see deploy.md's Host-layer backups).
- **Stop is sticky** (v1.11.0): the sidebar Stop button (and the idle
  reaper) marks the project deliberately stopped; open terminal tabs show
  "Project stopped" instead of auto-reconnecting — an automatic reconnect
  used to boot the VM right back up, making Stop un-stick whenever a tab
  was open. Only explicit user intent boots it again: the Start button,
  opening/resuming a terminal (its first connect carries `?boot=1`), or
  any API start. See terminal-protocol.md's close 4002.
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
  review). Every auth misconfiguration fails CLOSED at startup, not just at
  request time: `config.Load` refuses to start with `STYRMAN_AUTH=header` and
  an empty `STYRMAN_INGRESS_TOKEN`, and refuses any `STYRMAN_AUTH` value
  other than `none`/`header` outright (2026-08-30 security review, H2/M16).
- **Host-side git is bootstrap-only** (ADR-039): the server process itself
  only ever runs `git init`/`git rev-parse` (`internal/gitrepo`), both
  hardened against a guest-controlled `.git/config` (fsmonitor/hooksPath/ext
  transport all neutralized). Everything else git — status, diff, stage,
  commit, push — runs INSIDE the project's own guest VM (ADR-035); the
  former host-side checkpoint machinery, which ran unhardened host-side git
  against the same bind-mounted, guest-writable trees, is deleted outright,
  not merely unused.
- **Guest exec output is bounded** (H4/H7): `Orchestrator.Exec` (readiness
  probes, tmux bookkeeping, git status/diff, stats, format) caps a guest
  command's combined stdout/stderr at 2 MiB in memory — the guest is
  untrusted root in its own VM and otherwise controls how much the shared
  server holds — and guarantees a 30s deadline when the caller passes none,
  so a wedged guest command can't pin a goroutine forever.
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
sudo bash deploy/host-setup.sh --user $USER   # Docker, Kata "mvm", daemon cfg, dirs, the
                                              # isolation firewall + backup + auto-update
                                              # systemd units (auto-update installed but
                                              # disabled until its clone+deploy-key exist,
                                              # see deploy.md), seeds deploy/.env, and does
                                              # a one-time SMOKE-TEST build of
                                              # styrman-runtime:latest to confirm Kata can
                                              # boot it — production never runs this tag
                                              # (no mutable tags; versioning.md)
docker build -t styrman-runtime:dev --build-arg STYRMAN_VERSION=dev-$(date +%s) image/  # separate tag for the loopback dev run; the version keys harness re-seeding

# Dev (no auth, loopback; needs a Go 1.25+ toolchain — host-setup.sh does NOT install one):
STYRMAN_IMAGE=styrman-runtime:dev go run ./cmd/styrman/   # → http://127.0.0.1:8080

# Production (Google OAuth + HTTPS + reboot-safe; needs neither Go nor Node —
# build the runtime image at the RELEASE version, then bring the stack up at
# that same version; no mutable tags — full procedure: versioning.md):
vi deploy/.env    # BASE_DOMAIN, GOOGLE_CLIENT_ID/SECRET, CLOUDFLARE_API_TOKEN,
                  # ADMIN_EMAIL, STYRMAN_ALLOWED_EMAILS, STYRMAN_HOSTNAME (default
                  # styrman; the live deploy uses crew) (redirect URI:
                  # https://auth.<domain>/oauth2/google/authorization-code-callback)
STYRMAN_VERSION=$(git describe --tags --always)
docker build -t styrman-runtime:${STYRMAN_VERSION} \
  --build-arg STYRMAN_VERSION=${STYRMAN_VERSION} image/
STYRMAN_VERSION=${STYRMAN_VERSION} \
  docker compose -f deploy/docker-compose.yml up -d --build
# DNS records + certs are automatic → https://<STYRMAN_HOSTNAME>.<domain>
```

`host-setup.sh` installs a pinned Docker CE version and Kata release with a
fail-closed SHA256 check before extracting it as root — see deploy.md for
the exact pins and the `daemon.json` keys it sets. Node 22+ is needed only
to rebuild the frontend (`cd web && npm run build`; `web/dist` ships
committed) — neither path above requires it to just run.

First use per user: the sidebar shows a **First-time setup** banner →
"Open setup" boots a hidden minimal microVM running the guided
`styrman-setup`. Only git identity and an ssh key are REQUIRED; GitHub,
Google Cloud (account login + separately Application Default Credentials),
Claude, and agy are each OPTIONAL — `[y/N]` prompts, skip any and rerun
`styrman-setup` anytime, in the setup VM or any project terminal, to pick up
where you left off (v1.8.0). Already-set credentials just show ✔ with a
`renew? [y/N]` prompt — Enter keeps and moves to the next step, `y` re-runs
that sign-in (renew/switch account; a renewed ssh key keeps the old pair as
`id_ed25519.replaced`) (v1.10.1). The generated SSH key's comment is the user's
git email (fallback `user@styrman` if none is set), not "styrman". Closing
the setup terminal (typing `exit`, letting the script finish, or **Ctrl+C** —
a clean exit as of v1.10.0, no longer a restart) returns to the dashboard and
seeds the credential broker from whatever was just signed into — every
project, current and future, is current with zero re-auth, no manual import
step (ADR-034, v1.5.4). The exit is **status-aware** (v1.10.0): setup is only
marked done (`POST /api/setup/done`, VM destroyed) once the required creds
(git identity + ssh key) are present; a Ctrl+C before that just discards the
VM (`DELETE /api/setup`) and leaves the banner up so you can resume — an
incomplete setup is never marked done. Settings → Credentials →
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
  the sidebar ⋯ menu's **Backup** / **Settings → Backups** instead, plus the
  host-layer disaster-recovery snapshot described in deploy.md.)
- **Terminals**: sidebar, per project; generated friendly names; ⏸ pause /
  ✕ delete / close all; per-terminal autostart command (pencil → dialog;
  set the autostart to `claude --continue` / `agy --continue` and the
  terminal RESUMES its conversation after a pause/VM restart (both CLIs
  start fresh when there is nothing to resume). Deliberately OPT-IN per
  terminal, not automatic: all terminals of a project share one cwd, so an
  automatic rewrite made several terminals resume the SAME conversation
  (2026-08-30 review);
  runs at session creation, then drops to the shell); transcripts land in
  History regardless; a status bar under the terminal shows live CPU/mem/
  disk/IP/uptime and synced-credential count (v1.6.0/v1.8.1).
- **Settings**: account + terminal username, a single status-colored
  **Credentials** view (v1.8.2, replacing the old separate "Tools &
  credentials" cards) — GREEN = signed in and synced to the broker (click a
  green row to read its stored value, v1.8.1), GREY = not set up, RED =
  signed in locally but not yet synced. Grey and red rows — or their "sign
  in"/"re-sync" button — are clickable too (v1.10.0): clicking opens a modal
  that boots the hidden setup VM once and runs `styrman-setup <tool>` in an
  embedded terminal, harvesting that tool into the broker as soon as the
  step finishes; the VM stays up so a picker lets you add several
  credentials in one sitting without rebooting between them, and Ctrl+C in
  that terminal cancels just the current step and returns to the picker
  (the script's SIGINT trap makes this a clean exit, not a restart — see
  terminal-protocol.md). The picker's green rows are clickable too: an
  already-signed-in tool re-runs its step with a `renew? [y/N]` prompt —
  Enter keeps it, `y` re-authenticates (renew / switch account) (v1.10.1).
  "Done" (or clicking outside the modal) tears the VM down. An "Add a variable" row stores any custom env var (NPM_TOKEN,
  DATABASE_URL, …); "Import from a project" is marked optional (setup
  already auto-syncs, ADR-034) — theme (Light / Dark / System), and —
  admins only — LIVE runtime tunables: idle-stop timeout (applies to the
  reaper immediately) and docker disk size for new project VMs, persisted
  in `StateDir/settings.json` over the env defaults. (The user-access card
  was removed in v1.3 — access = env + admin API.)
- **Where things live** on the host:

| Path | What |
|---|---|
| `/var/lib/styrman/homes/<user>/p/<uuid>` | A project's OWN home (ADR-031): dotfiles, tool state, docker disk, transcripts — back up |
| `/var/lib/styrman/projects/<user>/<uuid>` | Workspaces (ADR-027, per-project storage) — back up TOO: source code no longer lives in the home |
| `…/homes/<user>/p/<uuid>/.dockerd/<uuid>.img` | Per-project Docker disk (sparse) |
| `…/homes/<user>/p/<uuid>/.styrman/logs/<uuid>/` | Terminal transcripts |
| `/var/lib/styrman/credentials/<user>/` | The broker's credential store (ADR-032, 0600) — back up |
| `/var/lib/styrman/backups/<user>/<uuid>/` | In-app per-project backup archives (ADR-044) — a DELETED project's only surviving copy; in the host-layer restic snapshot too — back up |
| `/var/lib/styrman/users/<user>/*.json` | Project/terminal/profile metadata |
| `/var/lib/styrman/users.json` | Runtime-added accounts (admins are env-only) |
| `/var/lib/styrman/links.json` | Public/private preview share-link tokens (ADR-025/026) |
| `/var/lib/styrman/settings.json` | Admin-set runtime tunables (idle timeout, docker disk size) overriding env defaults |
| containers `styrman-<uuid>`, networks `sdb-<uuid>` | Disposable runtime state |

Deleting a project WITHOUT "wipe files" keeps its workspace dir under
`/var/lib/styrman/projects/<user>/` (that's the point), but nothing in the
UI lists it anymore — reclaim disk by removing it manually if you never
re-import it.

- **Reboots**: containers restart themselves (`unless-stopped`); the server
  re-adopts from labels. **Disaster recovery**: see deploy.md's "Restore on
  a new host" runbook — `host-setup.sh` seeds a FRESH `deploy/.env`, so the
  OAuth/Cloudflare/domain secrets must be reconstructed after restoring
  `/var/lib/styrman` from the daily restic→GCS snapshot, before bringing
  the stack back up.

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
bash .agents/docs/testing/styrman/integration.sh     # 60 checks vs real Kata (3 auto-skip on firewall-hardened hosts)
bash web/e2e/run.sh                                  # browser e2e (Playwright, self-hosted; check counts: .agents/docs/testing/styrman/README.md)
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

Deliberately EMPTY as of v1.14.0 (operator directive, 2026-08-30: "I want
nothing in todo and roadmap — start fresh with new things"). Everything the
roadmap used to list here — transcript rotation, an ops panel, sidebar git
badges, agy native session resume, backup automation — SHIPPED that release;
a few older ideas (a warm VM pool, a platform MCP server, auto-titles from
transcripts, a moby SDK migration) were explicitly RETIRED with reasons
instead. Full shipped/retired history: **ADR-045**
(`.agents/docs/architecture/DECISIONS.md`) and `.agents/docs/changes/`.
`TODO.md` is the inbox for new items going forward — add there, not here.

Lineage: combines `devbox` (Kata multi-tenant shell platform; ingress,
terminal protocol) and `argolis-devbox` (single-VM cloud shell). Full
history: `.agents/docs/plans/` and `.agents/docs/changes/`.
