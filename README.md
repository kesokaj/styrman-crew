# styrman

Self-hosted, multi-user platform for running AI coding agents in isolated
projects. Sign in with Google, create a project, and get browser terminals
inside a dedicated **Kata Containers microVM** — own kernel, own Docker
daemon, own network, identified by a UUID with a friendly, renamable name.
Both [Claude Code](https://claude.ai/code) (`claude`) and
[Antigravity CLI](https://antigravity.google/product/antigravity-cli)
(`agy`) are preinstalled — run either, in as many terminals as you like.
Every user gets their own persistent home disk: credentials configured once,
forever; containers disposable always.

*Styrman* (Swedish): helmsman / first mate.

## Why

- **The agent CLI is the interface.** No custom chat UI to maintain — the
  terminal is the surface, organized like a conversation app: projects →
  terminals in the sidebar, project context (files, ports, history, diff)
  in the side panel.
- **Hard isolation.** 1 project = 1 microVM (own kernel, own dockerd, own
  network — projects can never reach each other). 1 user = 1 home disk —
  users never see each other's worlds.
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
                └── Docker SDK ──► per-project Kata microVM  styrman-<uuid>
                                     ├── tmux session per terminal → transcripts
                                     ├── own dockerd (overlay2 on a per-project disk)
                                     └── virtiofs: the user's home
```

No database: state lives in Docker labels, the filesystem, and per-user JSON
stores — a restart or reboot recovers everything by adoption.

## Status

Beta, deployed: https://app.styrman.xyz (Google sign-in, admin-managed
access). All four build phases landed — microVM runtime, UI, OAuth ingress,
multi-tenancy — verified by 3 suites (unit, 49 integration checks against
real Kata, 29 Playwright browser checks). The living documentation is
`.agents/docs/specs/styrman/guide.md`; the phase plans under
`.agents/docs/plans/` are historical.

## Requirements

- Debian 13 host (baremetal, or a GCP VM with nested virtualization) — Docker,
  Kata, and everything else is installed by `deploy/host-setup.sh`
- Go 1.24+ to build/run the server

## Quick start

```bash
sudo bash deploy/host-setup.sh --user $USER   # Docker + Kata "mvm" + dirs (idempotent)
docker build -t styrman-runtime:dev image/    # runtime image (~10 min first build)

# Dev (loopback, no auth):
STYRMAN_IMAGE=styrman-runtime:dev go run ./cmd/styrman/   # → http://127.0.0.1:8080

# Production (Google OAuth, HTTPS, reboot-safe):
vi deploy/.env && docker compose -f deploy/docker-compose.yml up -d --build
```

Full walkthrough, operations, and troubleshooting:
`.agents/docs/specs/styrman/guide.md`.

## Lineage

Combines and supersedes two earlier projects: `devbox` (multi-tenant Kata shell platform
with Caddy OAuth ingress) and `argolis-devbox` (single-VM cloud shell on GCP). Decision
history: `.agents/docs/architecture/DECISIONS.md`.
