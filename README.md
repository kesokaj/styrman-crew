# styrman-crew

Self-hosted, multi-user platform for running AI coding agents in isolated
projects. Sign in with Google, create a project, and get browser terminals
inside a dedicated **Kata Containers microVM** — own kernel, own Docker
daemon, own network, identified by a UUID with a friendly, renamable name.
Both [Claude Code](https://claude.ai/code) (`claude`) and
[Antigravity CLI](https://antigravity.google/product/antigravity-cli)
(`agy`) are preinstalled — run either, in as many terminals as you like.
Projects are truly individual — each its own home, so no tool state leaks
between them — while credentials are shared once and kept current by a broker;
containers disposable always.

*Styrman* (Swedish): helmsman / first mate — commanding a crew of agents.

## Why

- **The agent CLI is the interface.** No custom chat UI to maintain — the
  terminal is the surface, organized like a conversation app: projects →
  terminals in the sidebar, project context (files, ports, history, git)
  in the side panel. A thin status bar under every terminal shows live
  CPU/mem/disk/IP/uptime and how many credentials are synced into it.
- **Every project is a git repo, always.** Initialized (or cloned) at
  creation, no exceptions. The **Git tab is a real client**: status, stage/
  unstage, commit (author auto-injected), push, and per-file diffs, running
  inside the project's own VM so the right identity and credentials apply.
  Server-side **Format** reformats a file with its real CLI formatter
  (`gofmt`, `prettier`, `black`, `rustfmt`, `shfmt`, `terraform fmt`, …), also
  run inside the VM.
- **Per-project everything.** Each project owns its **home** (ADR-031) and
  its workspace storage (ADR-027), its Docker daemon and disk, its network —
  and its apt packages: what you `apt install` is captured at shutdown and
  replayed when the VM is ever recreated (ADR-028). Because the home is
  per-project, no tool state (`~/go`, `~/.npm`, caches, global installs,
  history) leaks between projects. Terminals get generated friendly names and
  an optional autostart command (start your dev server or a container the
  moment the session opens).
- **Hard isolation.** 1 project = 1 microVM (own kernel, own dockerd, own
  network, own home — projects can never reach each other, nor the host or
  the LAN). Enforced by a host firewall (dedicated iptables chains,
  insert-if-missing so there's no unprotected window) that a self-healing
  systemd timer re-applies every 60s (ADR-036; the host-reachability rule
  itself is ADR-029). Users never see each other's
  worlds.
- **Share previews, not access.** A project owner can grant specific Google
  emails access to ONE exposed port's preview — never the terminal, files,
  dashboard, or other ports. Sharing mints a stable **private link** on its
  own `*.private` subdomain (Google sign-in + share-list check, ADR-024/026);
  a public, expiring link (`1h`/`24h`/`7d`, no login, revocable) lives on
  `*.public` (ADR-025). The hostname tells you the auth model. All
  least-privilege by construction.
- **Credentials once, everywhere.** Sign in during first-run setup (git/ssh
  required; GitHub, Google Cloud, Claude and agy optional and skippable) and
  a **credential broker** (ADR-030/032/034) auto-seeds itself and keeps the
  single live copy current for every project — git (per-op helper, nothing
  stored in the VM), gh/npm/claude/git-identity tokens (exported into each
  login shell), gcloud ADC + ssh key + claude + agy's OAuth token (written
  into each home). A refresh anywhere propagates; new projects are current
  with no re-auth. Settings → Credentials shows a status-colored view
  (signed in & synced / not set up / signed in but not synced) with a
  click-to-read viewer for each stored value.

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
                                     ├── credentials pulled from the broker on the
                                     │     project bridge (git helper / env / files)
                                     ├── virtiofs: the project's own home
                                     │     (homes/<user>/p/<uuid>, ADR-031)
                                     └── virtiofs: the project's own workspace
                                         (StateDir/projects/<user>/<uuid>, ADR-027)
```

No database: state lives in Docker labels, the filesystem, and per-user JSON
stores — a restart or reboot recovers everything by adoption.

## Status

**v1.14.1**, deployed: https://crew.styrman.xyz (Google sign-in).

- Current release: the 2026-08-30 marathon — automated backups on BOTH
  layers (in-app cadence + retention + pre-wipe safety net, and daily
  whole-state restic→GCS), an ops row + microVM logs on the dashboard,
  sidebar git badges, project-scoped env vars, sticky stop with a
  busy-aware idle reaper, per-project backup/restore/clone (ADR-042…045) —
  followed by a same-day adversarial review with every finding fixed.
- Verified per release by four suites (unit+race, integration against real
  Kata, Playwright e2e, live post-deploy battery) — counts and how to run
  them: `.agents/docs/testing/styrman/README.md`.
- Self-updating (opt-in, ships disabled): a root systemd timer checks every
  5 min for the newest SIGNED `vX.Y.Z` release tag and redeploys it, rolling
  back automatically if it doesn't come up healthy — see `deploy.md`'s
  "Auto-update" section (ADR-050/051).
- Full release history: `.agents/docs/changes/` (one file per change) and
  every decision in `.agents/docs/architecture/DECISIONS.md`.

## Requirements

- Debian 13 host (baremetal, or a GCP VM with nested virtualization) — Docker,
  Kata, and everything else is installed by `deploy/host-setup.sh`
- Go 1.25+ to build/run the server
- Node 22+ / npm (only to rebuild the frontend; `web/dist` ships committed)

## Quick start

```bash
# 1. Host: Docker + Kata "mvm", firewall + backup + auto-update systemd
#    units, seeds deploy/.env with generated identity + secrets (idempotent).
#    Also builds the runtime image at the newest release tag —
#    styrman-runtime:<X.Y.Z>, the exact tag a same-version compose deploy
#    boots (no mutable tags, operator rule; see versioning.md below):
sudo bash deploy/host-setup.sh --user $USER
# 2. FRESH HOST ONLY: log out and back in (or `newgrp docker`) — the docker
#    group added in step 1 is not active in your current shell yet.

# Production (Google OAuth, HTTPS, reboot-safe) — no mutable tags: build the
# runtime image at the RELEASE version, then bring the stack up at that same
# version. Fill the OAuth/Cloudflare/domain values first:
vi deploy/.env
# --abbrev=0 + sed: the newest release tag, WITHOUT the v prefix — the same
# X.Y.Z string names all three images (a v-prefixed or SHA-suffixed value
# would land the build in a different tag namespace than the auto-updater's).
STYRMAN_VERSION=$(git describe --tags --abbrev=0 | sed 's/^v//')
docker build -t styrman-runtime:${STYRMAN_VERSION} \
  --build-arg STYRMAN_VERSION=${STYRMAN_VERSION} image/
STYRMAN_VERSION=${STYRMAN_VERSION} \
  docker compose -f deploy/docker-compose.yml up -d --build

# Dev (loopback, no auth; needs Go 1.25+) — builds its OWN image tag so
# experiments never touch the production one (~10 min first build):
docker build -t styrman-runtime:dev --build-arg STYRMAN_VERSION=dev-$(date +%s) image/
STYRMAN_IMAGE=styrman-runtime:dev go run ./cmd/styrman/   # → http://127.0.0.1:8080
```

Full walkthrough, operations, and troubleshooting:
`.agents/docs/specs/styrman/guide.md`. Image lineup, versioning scheme, the
manual/auto release procedures, and rollback:
`.agents/docs/specs/styrman/versioning.md`.

## Lineage

Combines and supersedes two earlier projects: `devbox` (multi-tenant Kata shell platform
with Caddy OAuth ingress) and `argolis-devbox` (single-VM cloud shell on GCP). Decision
history: `.agents/docs/architecture/DECISIONS.md`.
