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
  systemd timer re-applies every 60s (ADR-029). Users never see each other's
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

**v1.9.1**, deployed: https://crew.styrman.xyz (Google sign-in). Verified by
3 suites (unit + race, 56 integration checks against real Kata, self-hosted
Playwright browser checks) plus a live post-deploy battery on every release.
v1.9.0 shipped a full security-review remediation (62 findings, 1 critical):
closed a host-side git RCE by deleting the checkpoint machinery outright and
hardening the two host-side git calls that remain (`git init`/`rev-parse`)
against a guest-controlled `.git/config` — the Git tab's own client already
ran everything else inside the guest VM (ADR-039); deleted the dormant broker
keyring tier entirely, so agy's OAuth token is unambiguously a `file`
credential now, not a fourth "dormant" tier (ADR-040); made every auth
misconfiguration fail closed at startup instead of at request time; capped
guest exec output in memory (a project VM is untrusted root in its own
microVM); and reworked the project-isolation firewall onto dedicated
insert-if-missing iptables chains so there's no self-heal window where
isolation is briefly absent. v1.5 completed the **credential broker**
(ADR-030/032/034): finishing first-run setup auto-seeds it — sign in once
(git/ssh required, the rest optional and skippable), no manual import step.
v1.7 replaced automatic git checkpoints with a **real git
client** in the side panel (status/stage/commit/push/diff, running inside
the project VM so identity and auth apply) and added server-side code
**Format**ting via each language's real CLI formatter. v1.8 hardened project
network isolation with a self-healing firewall (re-applies every 60s, so a
Docker restart can't leave a project reachable from the host/LAN, ADR-029),
fixed a Caddy multi-homing bug that could 502 the whole app, consolidated
Settings into one status-colored **Credentials** view, added a live terminal
status bar, and improved terminal keyboard handling (Ctrl+C, AltGr/Option
symbol compose, Nordic dead keys) for non-US layouts. It builds on v1.4's
**per-project homes** (ADR-031) — each project fully individual, credentials
the only shared thing — and v1.3's UI redesign (Material 3 tokens, Google
Sans Flex + Google Sans Code self-hosted, light/dark/system themes, crew
branding), a dashboard with live terminal tails, full-text history search,
markdown preview, dynamic runtime settings, per-project workspace storage
(ADR-027), per-terminal autostart, and apt persistence (ADR-028). Earlier:
per-port preview sharing (ADR-024), public TTL links (ADR-025), private
email-gated links (ADR-026). The living documentation is
`.agents/docs/specs/styrman/guide.md`; per-release notes live in
`.agents/docs/changes/`.

## Requirements

- Debian 13 host (baremetal, or a GCP VM with nested virtualization) — Docker,
  Kata, and everything else is installed by `deploy/host-setup.sh`
- Go 1.25+ to build/run the server
- Node 22+ / npm (only to rebuild the frontend; `web/dist` ships committed)

## Quick start

```bash
sudo bash deploy/host-setup.sh --user $USER   # Docker + Kata "mvm" + dirs (idempotent)
docker build -t styrman-runtime:dev --build-arg STYRMAN_VERSION=dev-$(date +%s) image/  # runtime image (~10 min first build; the version keys harness re-seeding)

# Dev (loopback, no auth):
STYRMAN_IMAGE=styrman-runtime:dev go run ./cmd/styrman/   # → http://127.0.0.1:8080

# Production (Google OAuth, HTTPS, reboot-safe; stamp the version you deploy):
vi deploy/.env && STYRMAN_VERSION=1.9.1 docker compose -f deploy/docker-compose.yml up -d --build
```

Full walkthrough, operations, and troubleshooting:
`.agents/docs/specs/styrman/guide.md`.

## Lineage

Combines and supersedes two earlier projects: `devbox` (multi-tenant Kata shell platform
with Caddy OAuth ingress) and `argolis-devbox` (single-VM cloud shell on GCP). Decision
history: `.agents/docs/architecture/DECISIONS.md`.
