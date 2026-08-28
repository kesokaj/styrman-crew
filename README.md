# styrman-crew

Self-hosted, multi-user platform for running AI coding agents in isolated
projects. Sign in with Google, create a project, and get browser terminals
inside a dedicated **Kata Containers microVM** — own kernel, own Docker
daemon, own network, identified by a UUID with a friendly, renamable name.
Both [Claude Code](https://claude.ai/code) (`claude`) and
[Antigravity CLI](https://antigravity.google/product/antigravity-cli)
(`agy`) are preinstalled — run either, in as many terminals as you like.
Every user gets their own persistent home disk: credentials configured once,
forever; containers disposable always.

*Styrman* (Swedish): helmsman / first mate — commanding a crew of agents.

## Why

- **The agent CLI is the interface.** No custom chat UI to maintain — the
  terminal is the surface, organized like a conversation app: projects →
  terminals in the sidebar, project context (files, ports, history, git)
  in the side panel.
- **Every project is a git repo, always.** Initialized (or cloned) at
  creation, no exceptions. Automatic checkpoints snapshot the whole working
  tree on key events AND every 10 minutes while you work, restorable with
  one click from the Git tab.
- **Per-project everything.** Each project owns its workspace storage
  (ADR-027), its Docker daemon and disk, its network — and its apt
  packages: what you `apt install` is captured at shutdown and replayed
  when the VM is ever recreated (ADR-028). Terminals get generated friendly
  names and an optional autostart command (start your dev server or a
  container the moment the session opens).
- **Hard isolation.** 1 project = 1 microVM (own kernel, own dockerd, own
  network — projects can never reach each other). 1 user = 1 home disk —
  users never see each other's worlds.
- **Share previews, not access.** A project owner can grant specific Google
  emails access to ONE exposed port's preview — never the terminal, files,
  dashboard, or other ports. Sharing mints a stable **private link** on its
  own `*.private` subdomain (Google sign-in + share-list check, ADR-024/026);
  a public, expiring link (`1h`/`24h`/`7d`, no login, revocable) lives on
  `*.public` (ADR-025). The hostname tells you the auth model. All
  least-privilege by construction.
- **Credentials once, forever.** Each user's home (`~/.ssh`, `~/.claude`,
  `~/.gemini`, gh) survives container rebuilds, project deletion, and host
  reboots. Terminal transcripts are captured there too.

## Architecture

```
Browser ──HTTPS──► Caddy (Google OAuth, TLS, Cloudflare DNS — vendored ingress)
                      │ identity header
                      ▼
              styrman server (Go; embedded TanStack Start SPA)
                ├── per-user spaces: homes/<user>, metadata JSON stores
                ├── REST + WS /ws/term/{projectId}/{terminal}
                ├── /p/{id}/{t}/port/{port}/ → apps inside the microVM
                │     (owner, or a Google email the owner shared that port with)
                ├── /link/{token}/ → public, expiring, no-login preview of one
                │     port, served at https://<token>.<public domain>/
                ├── /shared/{token}/ → email-gated share of one port, served at
                │     https://<token>.<private domain>/ (Google sign-in required)
                └── Docker SDK ──► per-project Kata microVM  styrman-<uuid>
                                     ├── tmux session per terminal → transcripts
                                     ├── own dockerd (overlay2 on a per-project disk)
                                     ├── virtiofs: the user's home (credentials)
                                     └── virtiofs: the project's own workspace
                                         (StateDir/projects/<user>/<uuid>, ADR-027)
```

No database: state lives in Docker labels, the filesystem, and per-user JSON
stores — a restart or reboot recovers everything by adoption.

## Status

**v1.3.0**, deployed: https://crew.styrman.xyz (Google sign-in). Verified by
3 suites (unit + race, 62 integration checks against real Kata, 35
self-hosted Playwright browser checks) plus a live post-deploy battery.
v1.3 brought the full UI redesign (Material 3 tokens, Google Sans Flex +
Google Sans Code self-hosted, light/dark/system themes, crew branding, zero
native browser popups), a dashboard with live terminal tails, full-text
history search, markdown preview, dynamic runtime settings, per-project
workspace storage (ADR-027), per-terminal autostart, and apt persistence
(ADR-028). Earlier: per-port preview sharing (ADR-024), public TTL links
(ADR-025), private email-gated links (ADR-026). The living documentation is
`.agents/docs/specs/styrman/guide.md`; per-release notes live in
`.agents/docs/changes/`.

## Requirements

- Debian 13 host (baremetal, or a GCP VM with nested virtualization) — Docker,
  Kata, and everything else is installed by `deploy/host-setup.sh`
- Go 1.25+ to build/run the server

## Quick start

```bash
sudo bash deploy/host-setup.sh --user $USER   # Docker + Kata "mvm" + dirs (idempotent)
docker build -t styrman-runtime:dev image/    # runtime image (~10 min first build)

# Dev (loopback, no auth):
STYRMAN_IMAGE=styrman-runtime:dev go run ./cmd/styrman/   # → http://127.0.0.1:8080

# Production (Google OAuth, HTTPS, reboot-safe; stamp the version you deploy):
vi deploy/.env && STYRMAN_VERSION=1.3.0 docker compose -f deploy/docker-compose.yml up -d --build
```

Full walkthrough, operations, and troubleshooting:
`.agents/docs/specs/styrman/guide.md`.

## Lineage

Combines and supersedes two earlier projects: `devbox` (multi-tenant Kata shell platform
with Caddy OAuth ingress) and `argolis-devbox` (single-VM cloud shell on GCP). Decision
history: `.agents/docs/architecture/DECISIONS.md`.
