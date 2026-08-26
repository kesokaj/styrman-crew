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
Live at https://app.styrman.xyz.

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
   record + transcript stay) and closed, singly or all at once. Everything
   printed in them is transcribed to the home (ADR-017): the History tab.
4. **The side panel is project context**: Files (workspace + home; view,
   edit, upload by drag, download), Ports (what's listening, with process /
   inner-docker image, shareable `/p/<id>/<t>/port/<port>/` links), History
   (transcripts), Diff (git).

## 2. Architecture

```
Browser ──HTTPS──► styrman-caddy (host net): Google OAuth, TLS, Cloudflare DNS,
                   label discovery (vendored ingress, deploy/caddy)
                      │ X-Caddy-Email
                      ▼
   styrman-server (container; loopback dev door 127.0.0.1:8080, no auth)
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
`styrman-setup` (git identity → SSH key → gh → claude → agy). Everything
lands in that user's home — once, forever. The same script works in any
project terminal.

## 4. Operating it

- **New project**: name + optional git init → ~3 s to a live terminal; type
  `claude` or `agy`. Rename any time (⋯ → Rename); delete via the
  type-"delete" dialog (wipe checkbox controls files).
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
bash .agents/docs/testing/styrman/integration.sh     # 49 checks vs real Kata
node web/e2e/run.mjs                                 # 29 browser checks (Playwright)
cd web && npm run build                              # UI → dist/client (committed)
```

Layout: `cmd/styrman` (server) · `cmd/wsprobe` (WS test client) ·
`internal/{config, identity, userstore, project, conversation, orchestrator,
terminal, httpapi, files, history}` · `image/` (runtime microVM image) ·
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
