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
2. **1 user = 1 home disk** (ADR-019, R3). Each account owns
   `/var/lib/styrman/homes/<user>` — credentials, workspaces, transcripts —
   mounted into each of *their* microVMs. Sign in to claude/agy/gh/ssh once
   per user, forever; containers stay disposable. Users never see each
   other's worlds. Admins (env-provided) manage access in Settings.
3. **Terminals** (ADR-016): durable records backed by tmux sessions — they
   survive tab closes and disconnects, can be renamed, paused (session ends,
   record + transcript stay) and closed, singly or all at once. The shell
   exiting is presented the same way: a **Paused** state with a **Resume**
   action, not a dead terminal. Everything printed in them is transcribed to
   the home (ADR-017): the History tab. Copy is Ctrl/Cmd+C when a selection
   exists (otherwise it passes through as SIGINT); paste is Ctrl/Cmd+V or
   Ctrl+Shift+V via the clipboard API. A sidebar notification bell (badge,
   linking to the terminals waiting for input) surfaces terminals whose tmux
   window rang its bell while unattended — tmux clears the flag once the
   session is viewed. The terminal (`web/src/lib/terminal.ts`) copies on
   select: the clipboard is written the instant a selection is highlighted,
   not on an explicit Ctrl+C, because TUIs (claude, agy) redraw and wipe
   selections before a keypress can land — the runtime image's tmux
   `mouse off` default is what lets native browser selection work at all. A
   link-rescue bar above the terminal surfaces URLs as Open/Copy chips,
   reassembled from both the buffer (soft-wrapped lines rejoined) and the
   raw output stream (hard-wrapped lines collapsed) — auth flows print long
   URLs that terminals mangle either way. Ctrl+Shift+F opens a scrollback
   search overlay; OSC 52 lets tmux/TUIs write the system clipboard
   directly. It renders via xterm's DOM renderer, not WebGL, so terminal
   text stays real, selectable, assertable DOM. Anything that lands on the
   clipboard is recorded in the Clips side-panel tab — session-scoped
   (sessionStorage, 1h TTL, never persisted or synced) since copy-on-select
   could capture a secret typed during setup (ADR-023).
4. **The side panel is project context**: Files (workspace + home; view,
   edit via a lazy-loaded CodeMirror 6 editor with syntax highlighting by
   filename and Ctrl+S to save, upload by drag, download), Ports (what's
   listening, with process / inner-docker image, shareable
   `/p/<id>/<t>/port/<port>/` links), History (transcripts), Git
   (working-tree diff + branch, and checkpoints — see below), Clips
   (clipboard history, session-scoped — see above).
5. **Git is the substrate** (R13, ADR-022): every project workspace IS a git
   repository, no exceptions — `git init -b main` runs synchronously at
   creation, or a clone runs in-guest instead (new project dialog's "Clone a
   repo" option, so the user's ssh key/gh credentials apply; falls back to
   an empty repo on failure), and a boot-time sweep initializes any
   repo-less workspace it finds (adopted pre-R13 projects). Checkpoints are
   automatic whole-tree snapshots (tracked + untracked; ignored files
   excluded) taken via a private index and stored under
   `refs/styrman/checkpoints/*` — never on the user's branches, never
   pushed, invisible to `git log`, capped at 40. Triggers: project stop,
   terminal pause/close/close-all, and before every restore (so restores
   are undoable); manual via the Git tab's "Checkpoint now" button. Restore
   rewrites the working tree to that exact snapshot (later files deleted;
   ignored files survive; the user's branches/commits untouched).

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
                          ├── tmux conv-<n> per terminal (bash -l), piped to
                          │   ~/.styrman/logs/<id>/  (transcripts)
                          └── virtiofs: /home/<guestname> = homes/<user>
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
- **Isolation** (R12): per-project bridges shared only with the server,
  which runs `ip_forward=0`; the server joins project networks in the
  background (a live join can drop in-flight connections — the preview
  proxy 307s once instead when it must join on demand).
- **Idle lifecycle**: terminal traffic timestamps the project; idle ones
  stop after `STYRMAN_IDLE_TIMEOUT` (24h); reopening boots on connect.
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
# DNS records + certs are automatic → https://app.<domain>
```

First use per user: the sidebar shows a **First-time setup** banner →
"Open setup" boots a hidden minimal microVM running the guided
`styrman-setup` (git identity → SSH key → gh → claude → agy). The generated
SSH key's comment is the user's git email (fallback `user@styrman` if none
is set), not "styrman". Everything lands in that user's home — once,
forever. The same script works in any project terminal.

## 4. Operating it

- **New project**: name + repo choice (empty git repo, clone an existing
  one, or create+push a fresh GitHub repo via `gh`) → ~3 s to a live
  terminal; type `claude` or `agy`. Every workspace is a git repo from
  creation, no exceptions (R13). Rename any time (⋯ → Rename); delete via
  the type-"delete" dialog (wipe checkbox controls files).
- **Checkpoints**: the Git tab lists automatic snapshots (taken on stop,
  terminal pause/close, and before restores) plus a "Checkpoint now"
  button; Restore (confirmation required) rewrites the working tree to that
  point and always checkpoints the current state first, so it's undoable.
- **Terminals**: sidebar, per project; ⏸ pause / ✕ delete / close all;
  transcripts land in History regardless.
- **Settings**: account + terminal username, per-tool credential cards
  (incl. claude/agy skills & subagents), theme, runtime/storage facts, and —
  admins only — user access management.
- **Where things live** on the host:

| Path | What |
|---|---|
| `/var/lib/styrman/homes/<user>` | A user's world — THE thing to back up |
| `…/homes/<user>/projects/<dir>` | Workspaces (dir = name at creation) |
| `…/homes/<user>/.dockerd/<uuid>.img` | Per-project Docker disk (sparse) |
| `…/homes/<user>/.styrman/logs/<uuid>/` | Terminal transcripts |
| `/var/lib/styrman/users/<user>/*.json` | Project/terminal/profile metadata |
| `/var/lib/styrman/users.json` | Runtime-added accounts (admins are env-only) |
| containers `styrman-<uuid>`, networks `sdb-<uuid>` | Disposable runtime state |

- **Reboots**: containers restart themselves (`unless-stopped`); the server
  re-adopts from labels. **Disaster recovery**: fresh host → host-setup →
  restore `/var/lib/styrman` → rebuild images.

## 5. Troubleshooting

| Symptom | Cause / fix |
|---|---|
| `invalid namespace type` starting any microVM | Docker ≥29.5 time-namespaces vs Kata ≤3.31 — host-setup sets the daemon.json feature flag (ADR-009). |
| Site unreachable but server fine | Check DNS first — a "newly registered domain" blocklist (NextDNS et al.) sinkholed styrman.xyz once (`dig` for 0.0.0.0 answers). |
| dockerd inside a project uses `vfs` | Loop mount of `~/.dockerd/<uuid>.img` failed; see container logs (ADR-008 fallback). |
| agy re-asks for login | Keyring didn't start (login shells source `/etc/profile.d/styrman-keyring.sh`); fallback `GEMINI_API_KEY` in `~/.bash_secrets`. |
| Terminal shows "replaced" | Same terminal opened in another tab (close 4001) — one attach each, by design. |
| First port-preview 307s or drops once | The server just joined that project's network; the redirect/retry lands. |
| Project won't start | `docker logs styrman-<uuid>` — the entrypoint logs every step. |

## 6. Development

```bash
go test ./...                                        # unit (stores, guards, routing)
bash .agents/docs/testing/styrman/integration.sh     # 56 checks vs real Kata
bash web/e2e/run.sh                                  # 33 browser checks (Playwright, self-hosted)
cd web && npm run build                              # UI → dist/client (committed)
```

Layout: `cmd/styrman` (server) · `cmd/wsprobe` (WS test client) ·
`internal/{config, identity, userstore, project, conversation, orchestrator,
terminal, httpapi, files, history, gitrepo}` · `image/` (runtime microVM
image) ·
`deploy/` (host-setup, compose, vendored caddy ingress, server Dockerfile) ·
`web/` (TanStack Start SPA). Conventions: `AGENTS.md`; non-negotiables:
`REQUIREMENTS.md` (R1–R12).

## 7. Roadmap

Remaining candy: transcript rotation, ops panel (disk/logs per project),
git badges in the sidebar, agy native session resume, moby SDK migration,
backup automation for `/var/lib/styrman`. See `TODO.md`.

Lineage: combines `devbox` (Kata multi-tenant shell platform; ingress,
terminal protocol) and `argolis-devbox` (single-VM cloud shell). Full
history: `.agents/docs/plans/` and `.agents/docs/changes/`.
