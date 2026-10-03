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
containers disposable always. The agent harness (`styrman-harness`) reaches
every project as the `sh-agent` release binary baked into the runtime image
(ADR-074); a user can pull a newer release into their own project any time
with `sh-agent update`, no crew release required. The image currently
carries harness v2.3.2 (36 skills, 8 subagents); its `sh-agent install` runs
after the project reports ready, in the background (ADR-078). `agy update`
and `claude update` work inside a project (ADR-077).

*Styrman* (Swedish): helmsman / first mate — commanding a crew of agents.

## Why

- **The agent CLI is the interface.** No custom chat UI to maintain — the
  terminal is the surface, organized like a conversation app: projects →
  terminals in the sidebar, project context (files, ports, history, git)
  in the side panel. A thin status bar under every terminal shows live
  CPU/mem/disk/IP/uptime and how many credentials are synced into it. The
  file explorer has Back/Forward/Up navigation (`Alt+←`/`Alt+→` too) and a
  breadcrumb bar, with a collapsible, resizable Recent-files strip above
  it; Settings → Appearance sets terminal and editor font size and family.
  A **Browser** view shows each project's own headless Chromium, driven by
  the operator and by the agents alike through the `styrman-browser` MCP
  server (ADR-068). **PWA support** enables installing the web app directly
  to desktop or home screen (manifest, custom icons, theme color).
- **Every project is a git repo, always.** Initialized (or cloned) at
  creation, no exceptions. The **Git tab is a real client**: status, stage/
  unstage, commit (author auto-injected), a Fast commit button for a
  one-click stage-everything-and-commit, Fetch/Pull (`--ff-only`), push, a
  branch dropdown that switches or creates, a Commits list, and stash push/
  pop — all running inside the project's own VM so the right identity and
  credentials apply. Diffs open full-width in the main pane, a third view
  beside Terminal and Editor. Server-side **Format** reformats a file with
  its real CLI formatter (`gofmt`, `prettier`, `black`, `rustfmt`, `shfmt`,
  `terraform fmt`, …), also run inside the VM.
- **Per-project everything.** Each project owns its **home** (ADR-031) and
  its workspace storage (ADR-027), its Docker daemon and disk, its network —
  and its apt packages: what you `apt install` is captured at shutdown and
  replayed when the VM is ever recreated (ADR-028). Because the home is
  per-project, no tool state (`~/.go`, `~/.npm`, caches, global installs,
  history) leaks between projects. Terminals get generated friendly names and
  an optional autostart command (start your dev server or a container the
  moment the session opens).
- **Hard isolation.** 1 project = 1 microVM (own kernel, own dockerd, own
  network, own home — projects can never reach each other, Docker's own
  unconditional per-bridge DROP rule). A project's MODE is its Docker
  bridge interface name, chosen by the server at `docker network create` —
  nothing else changes and the server never touches iptables. Sandbox
  (default) is `sdb-<hash>`, matched by the root-owned host firewall's
  (ADR-036/067) drops of all private ranges plus the host itself, internet
  only, re-applied by a systemd timer. LAN is `lan-<hash>`, matched by no
  styrman rule at all, so the project reaches whatever the host reaches,
  still NAT-only outbound. An admin toggles a project's mode at any time,
  rebuilding that project's Docker network and container; there is no
  allowlist, no marker, no spool, and the credential broker withholds
  nothing from a LAN project. Users never see each other's worlds.
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
Browser ──HTTPS──► Caddy (Google OAuth, TLS via Cloudflare DNS or HTTP-01 ACME — vendored ingress)
                      │ identity header
                      ▼
              styrman server (Go; embedded TanStack Start SPA, PWA installable)
                ├── per-user spaces: homes/<user> (server-only parent of
                │     base/ and p/<uuid>, ADR-064), metadata JSON stores
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

**v1.41.0**, tagged 2026-10-02. Both hosts take a signed tag by themselves: the GCP VM (https://crew.spgo.demo.altostrat.com, Google sign-in) on its next updater tick; the NUC (https://crew.styrman.xyz) runs v1.41.0 since 2026-10-03, and its updater works again with a replaced token (BUG-084, fixed: its first real tick wrote the Settings status file, `ok`). From this release on, Settings tells admins when a host's auto-updater is failing; a host shows it one updater tick after it takes the release. v1.39.0 reached the GCP VM only.

Since v1.22.0, one line per release; the full entry for each is
`.agents/docs/changes/<date>-vX.Y.Z-release.md`:

- **v1.41.0** (2026-10-02): Settings tells admins when the host's
  auto-updater is failing, has stopped running or cannot be read, and why
  (ADR-081: the updater records each tick in `/etc/styrman/status/updater.json`,
  mounted read-only into the server); a browser view whose connection went
  silent reconnects by itself; keys typed while a terminal reconnected are
  dropped once they are 30 s old instead of being replayed after a sleep; the
  Ports panel and the browser view no longer answer with an error while a
  project's network mode is switched or the project stops (BUG-088); ADR-082
  records the two release signers.
- **v1.40.0** (2026-10-02): `claude` and `agy` update themselves inside a
  project (agy in its own directory owned by the project user; the npm prefix
  Claude's updater needs is created at boot); a terminal recovers without a
  page reload — a terminal taken by another tab has "Use here" and comes back
  when its tab is shown again, and a connection that died during sleep is
  noticed by a ping and replaced in place; agent harness v2.3.2 (36 skills, 8
  subagents), its install moved behind ready so first boots stay near 3 s;
  runtime image pins refreshed (Node 22.23.3, Claude Code 2.1.287, OpenTofu
  1.13.1, yq 4.54.1); the ingress no longer takes a failed Caddy config load
  for a success (crew had been down six hours after a host reboot);
  `styrman-ctl install` writes a Cloudflare token the answers file carries.
- **v1.39.0** (2026-09-22): GCP target migrated to `daring-lioness-uwgw` (300 GB
  Hyperdisk Balanced, domain `spgo.demo.altostrat.com`, deployed to
  `https://crew.spgo.demo.altostrat.com` via signed tag + auto-updater);
  `CLOUDFLARE_API_TOKEN` is now optional across Caddy ingress, `styrman-ctl`,
  and `install.sh` (Caddy falls back to HTTP-01 ACME challenges for non-wildcard
  hostnames); `deploy/gcp/lib.sh` stat calls are portable across macOS and
  Linux (`stat -c` / `stat -f`); PWA support added (`manifest.webmanifest`,
  192/512/maskable PNG icons, apple-touch-icon, theme-color meta tag) enabling
  browser install prompts and home-screen app launch.
- **v1.38.0** (2026-09-22): a whole-project review, acted on in five phases.
  The file explorer's listing is bounded (5000 entries) and says when it was
  cut — it previously read an entire guest-owned directory and sorted it
  quadratically in the one process serving every tenant, 5.88 s at 20 000
  entries against 13.4 ms now; the managed-subtree guard refuses paths
  reaching `~/.dockerd` and friends through a symlink at any depth; a guest
  FIFO at `~/.ssh` can no longer wedge a server goroutine; the R12 boot
  assertion refuses an unreadable `ip_forward` instead of warning; the release
  gate compares the committed frontend by content rather than by file name;
  and the GCP scripts take their hardened secret-file helpers from one shared
  copy. Plus ADR-076 and a documentation pass that fixed five build commands
  which failed as written. Full entry:
  `.agents/docs/changes/2026-09-22-v1.38.0-release.md`.
- **v1.37.0** (2026-09-22): a GCP Compute Engine target running the
  unchanged host path (ADR-075) — `deploy/gcp/vm.sh` for the instance
  lifecycle over IAP and `deploy/gcp/install.sh` for bring-up on the VM —
  plus two fixes to the shared path it uncovered: `host-setup.sh` can
  install Kata on a fresh host again (BUG-070), and `styrman-ctl install
  --gh-token-file` exports the token in its own process rather than on a
  command line.
- **v1.36.0** (2026-09-21): the agent harness is delivered as the
  `sh-agent` release binary, downloaded and verified at image build time
  from the private `kesokaj/styrman-harness` repository (ADR-074). Every
  project seeds from it on first boot with `sh-agent install --home`; a
  user pulls a newer release into their own project with `sh-agent
  update`. This superseded ADR-062's `image/harness/` mirror and ADR-073's
  host harness channel (below).
- **v1.35.0** (2026-09-16): the agent harness reached every project
  through the host's HARNESS CHANNEL (ADR-073): a root timer walked the
  `styrman-harness` tags down to the newest SSH-signed one that verified
  against the host's pinned signers, exported it as a plain copy with an
  offline-built `sh-agent`, smoked it in the deployed runtime image, and
  swapped it in under `/var/lib/styrman/harness/current`; the server
  mounted it read-only into every project VM and a stopped VM picked a new
  export up on its next start, so a styrman-harness release no longer
  needed a crew release. The image's mirror stayed the fallback until the
  channel had delivered. **Superseded by ADR-074 (v1.36.0, above); the
  `styrman-ctl update --setup --harness-token-file` setup step it required
  is retired and must not be run.** Full entry:
  `.agents/docs/changes/2026-09-16-v1.35.0-release.md`.
- **v1.34.1** (2026-09-16): stopping a project (the Stop button or the idle
  reaper) now switches its container's restart policy to `no` before the
  guest's own shutdown, and the next Start or terminal attach switches it
  back to `unless-stopped`, so Docker no longer restarts the clean exit and
  then hard-kills the rebooted VM (BUG-063, seen on the live host right
  after the v1.34.0 deploy); the integration suite asserts it (106 checks).
  Full entry: `.agents/docs/changes/2026-09-16-v1.34.1-release.md`.
- **v1.34.0** (2026-09-16): every project home's agent harness refreshes to
  styrman-harness `4aa19be` (27 skills / 7 subagents; on its next seed
  `sh-agent install` writes the `attribution` key into
  `~/.claude/settings.json` when absent, so a commit made by Claude Code
  inside a project carries no Co-Authored-By trailer — R5 amended, ADR-072);
  the boot log carries the installer's `settings:` line for that file and
  its `note:` line when the file cannot be judged; the Tier 2 suite asserts
  the file and those lines (104 checks; BUG-061, BUG-062). Full entry:
  `.agents/docs/changes/2026-09-16-v1.34.0-release.md`.
- **v1.33.0** (2026-09-14): a containerized server refuses to start when
  `ip_forward` is on inside its container — R12's no-routing guarantee is
  asserted at boot, not assumed from the compose file (`STYRMAN_ALLOW_IP_FORWARD=1`
  is the deliberate override); every project home's agent harness refreshes
  to styrman-harness `cceee54` (27 skills / 7 subagents; the root-steps
  sentence each VM receives is conditioned on the project's recorded
  permission; skill/agent names validated, symlinks in skill dirs refused;
  the mirror's tests pass as a plain copy); the release gate names a Docker
  Hub rate limit for what it is (BUG-056). Ten carried items closed —
  `.agents/docs/plans/2026-09-14-v1.33.0-close-out.md`.
- **v1.32.0** (2026-09-14): a project left stopped across a server restart
  now has its credential broker reachable from its first terminal — the
  terminal-WebSocket boot path joins the server to the project's bridge
  before the socket is upgraded (BUG-060, ADR-071). Before, that first
  shell reported "credentials: the broker did not answer" and had no
  git/gh/ssh/gcloud/claude credentials until something else attached.
- **v1.31.0** (2026-09-14): the agent harness every project home receives
  is refreshed to styrman-harness `8046e4d`: 27 skills (`/review` replaces
  `/audit`; `gcloud-auth-verification` is folded into `gcloud-setup`), the
  rewritten global context and agents, `sh-agent install --vendor`, and an
  install that treats the image's mirror as the plain copy it is — never a
  rebuild, never a git call inside the VM (the harness's BUG-015 fix);
  `sh-agent status` in a project says so on its `repo` line. Every project
  re-seeds on its next start.
- **v1.30.1** (2026-09-13): the firewall reports a degraded host (missing
  `xt_physdev`) and `styrman-ctl status` shows it; the guest harness stamp
  no longer interpolates the version; `image/bump-pins.sh` refreshes the
  image pins.
- **v1.30.0** (2026-09-13): audit fixes — brokered credentials reach login
  shells again (BUG-054), long start/stop calls are no longer clipped to 20 s,
  Go/Node/Claude Code pinned by publisher hash (ADR-069), auth-portal and
  updater hardening.
- **v1.29.0** (2026-09-13): Browser view review fixes (pane, relay,
  launcher, Ports panel polling).
- **v1.28.0** (2026-09-11): the Browser view (one headless Chromium per
  project, ADR-068) and the operator's `styrman-ctl`.
- **v1.27.x** (2026-09-10): provisioning state, user revocation, editor
  polish, a Docker seam for tests; updater firewall idempotence.
- **v1.26.0** (2026-09-10): durable network records; the ssh public key is
  shown in Settings.
- **v1.25.0** (2026-09-09): the network switch — a project's mode is its
  Docker bridge prefix, matched by the root firewall (ADR-067).
- **v1.23.0–v1.24.1** (2026-09-08): per-project LAN access (ADR-065/066,
  since superseded by ADR-067) and the audit fixes behind it.
- **v1.22.0** (2026-09-04): the Git tab grew from a status/stage/commit/push
  box into a fuller client — Fast commit, Fetch/Pull (`--ff-only`), a branch
  dropdown, a Commits list, and stash push/pop — and diffs now open
  full-width in the main pane instead of a 64px box in the side panel.
  Settings → Appearance adds terminal/editor font size and family; the file
  explorer gained Back/Forward/Up navigation, a breadcrumb bar, and a
  collapsible, resizable Recent-files strip.
- **v1.21.0** (2026-09-04): the setup VM's home is now `homes/<user>/base`,
  not `homes/<user>` itself, so no guest mount contains the project homes
  (ADR-064). The server migrates an old base home into `base/` once at
  startup and recreates a stale setup VM by itself. This closes the
  check-to-mount window v1.20.0 had accepted (BUG-002/003).
- **v1.20.0** (2026-09-04): the agent harness every project home receives is
  now a verbatim mirror of the operator's `styrman-harness` repo, installed
  by `sh-agent` — the harness's skills, subagents and global context (the
  roster shipped at the time was 28 skills and 7 subagents; the current
  one is in `image/README.md`), no hooks, scripts,
  or settings files; homes seeded by the previous harness are migrated
  (never deleted) into `~/.styrman/harness-backup-<timestamp>/` (ADR-062),
  and Settings shows the installed roster. The "Failed to fetch" that hit
  active sessions every ~15 minutes is fixed — the SPA now re-authenticates
  once on session expiry instead of surfacing the error (BUG-001) — and the
  ingress session lifetime is now a deliberate eight hours, not Caddy's
  900-second default (ADR-063). Three symlink-plantable project-home path
  exposures found and fixed during the harness work (BUG-002/003):
  `internal/files.Trust` confines file access per tenant, the transcript
  janitor is confined the same way, and the orchestrator refuses a
  symlinked bind source before handing it to Docker. The integration suite
  now asserts the seeded harness and R1 boot latency (65 checks).
- v1.19.x (2026-09-02): host-writable-path containment asserted end to end,
  a cross-tenant project-deletion bug fixed, and the ingress admin loopback
  guard now verifies an IP instead of trusting a name.
- The 2026-08-30 marathon shipped an ops row + microVM logs on the
  dashboard, sidebar git badges, project-scoped env vars, and a sticky stop
  with a busy-aware idle reaper (ADR-042…045), followed by a same-day
  adversarial review with every finding fixed. It also shipped a two-layer
  backup feature that has since been **removed entirely** (operator
  decision 2026-09-01): there is no backup layer of any kind — durability
  is git, commit and push. A wipe-delete is irreversible.
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

- Debian 13 host (baremetal, or a GCP Compute Engine VM with nested
  virtualization — `deploy/gcp/`, `.agents/docs/deploy/gcp.md`) — Docker,
  Kata, and everything else is installed by `deploy/host-setup.sh`
- Go 1.25+ to build/run the server
- Node 22+ / npm (only to rebuild the frontend; `web/dist` ships committed)

## Quick start

```bash
# 1. Host: Docker + Kata "mvm", firewall + auto-update systemd units,
#    seeds deploy/.env with generated identity + secrets (idempotent).
#    Also builds the runtime image at the newest release tag —
#    styrman-runtime:<X.Y.Z>, the exact tag a same-version compose deploy
#    boots (no mutable tags, operator rule; see versioning.md below):
sudo bash deploy/host-setup.sh --user $USER
# 2. FRESH HOST ONLY: log out and back in (or `newgrp docker`) — the docker
#    group added in step 1 is not active in your current shell yet.

# Production (Google OAuth, HTTPS, reboot-safe) — no mutable tags. Fill the
# OAuth/domain values first (CLOUDFLARE_API_TOKEN is optional: without it Caddy
# uses HTTP-01, which covers the base app and auth portal only — the wildcard
# preview/private-link hostnames need DNS-01 and get no certificate):
vi deploy/.env
# The version derivation + build/deploy commands live in ONE place, not
# restated here — versioning.md's "Release procedure" (commit, verify the
# COMMIT not the working tree, sign + push a vX.Y.Z tag, let the auto-updater
# deploy it) is the real day-to-day path; its "Manual / break-glass fallback"
# has the exact commands for a from-scratch host where the updater isn't set
# up yet.

# Dev (loopback, no auth; needs Go 1.25+) — builds its OWN image tag so
# experiments never touch the production one (~10 min first build):
GH_TOKEN=$(gh auth token) docker build --secret id=gh_token,env=GH_TOKEN \
  -t styrman-runtime:dev --build-arg STYRMAN_VERSION=dev-$(date +%s) image/
STYRMAN_IMAGE=styrman-runtime:dev go run ./cmd/styrman/   # → http://127.0.0.1:8080
```

Full walkthrough, operations, and troubleshooting:
`.agents/docs/specs/styrman/guide.md`. Image lineup, versioning scheme, the
release procedure (tag + auto-updater, plus the manual break-glass
fallback), and rollback: `.agents/docs/specs/styrman/versioning.md`.

## Lineage

Combines and supersedes two earlier projects: `devbox` (multi-tenant Kata shell platform
with Caddy OAuth ingress) and `argolis-devbox` (single-VM cloud shell on GCP). Decision
history: `.agents/docs/architecture/DECISIONS.md`.
