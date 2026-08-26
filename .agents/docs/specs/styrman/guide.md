# The styrman guide

Everything about the project in one place: what it is, how it works, how to
install it from nothing, how to operate it, and how to develop it. Detail
specs: `api.md`, `terminal-protocol.md`, `runtime-image.md`; decisions:
`../../architecture/DECISIONS.md`; history: `../../changes/`.

## 1. What styrman is

A self-hosted web platform for running AI coding agents in isolated projects.
You create a project and get browser terminals inside a dedicated **Kata
microVM** — its own kernel, its own Docker daemon, its own network. Both
agents (`claude` and `agy`) are preinstalled; run whichever you want, in as
many terminals as you want. styrman organizes it Antigravity-style: projects
→ terminals in the sidebar, project context (files, ports, history, diff) in
the side panel, everything persistent.

Three load-bearing ideas:

1. **1 project = 1 microVM** (ADR-001). Hard isolation; an agent gone wild
   stays inside its project. `docker` works inside each project (real DinD,
   overlay2 on a per-project disk image — ADR-004/008).
2. **One shared home** (ADR-003). `/var/lib/styrman/home` is mounted as
   `$HOME` in every microVM. Sign in to claude, agy, gh, gcloud, and generate
   your SSH key **once, ever**. Containers are disposable at all times.
3. **Terminals** (R6, ADR-016). A project has as many as you want — each a
   durable record backed by a tmux session, surviving browser disconnects and
   tab switches. Run `claude` in one and `agy` in another to work with several
   agents at once. Everything printed in them is transcribed to the shared
   home (ADR-017), so the record outlives the container.

## 2. Architecture

```
Browser ──HTTPS──► Caddy + Google OAuth (Phase 3; Phase 1 is localhost HTTP)
                      │
                      ▼
   styrman server (one Go binary, runs as the operator user)
     ├── UI (embedded TanStack Start SPA, ADR-013)
     ├── REST /api/projects, .../terminals, .../files, .../history  (api.md)
     ├── WS   /ws/term/{p}/{term}                 (terminal-protocol.md)
     ├── proxy /p/{p}/{term}/port/{port}/ → the app inside the microVM
     └── Docker SDK ──► per-project container, runtime "mvm" (Kata)
                          ├── tini (PID 1) → entrypoint → sleep
                          ├── dockerd: overlay2 on loop-mounted ~/.dockerd/<p>.img
                          ├── tmux: session conv-<id> per terminal (bash -l),
                          │     piped to ~/.styrman/logs/<p>/ (ADR-017)
                          └── virtiofs: /home/<user> = /var/lib/styrman/home
```

- **State** (ADR-007): no database. Containers carry `styrman.*` labels and
  are re-adopted on server start; live sessions are discovered from tmux;
  `projects.json` + `conversations.json` hold only what must outlive
  containers (project metadata, terminal titles).
- **Terminal transport** (ADR-002): the server exec's
  `tmux new-session -A -D -s conv-<id> -c ~/projects/<p> bash -l` into the
  container (TTY exec, hijacked stream) and pumps bytes to the browser
  WebSocket. Project containers run no server, so orchestration logic updates
  without image rebuilds.
- **Idle lifecycle**: terminal traffic touches a per-project timestamp; a
  reaper stops containers idle past `STYRMAN_IDLE_TIMEOUT` (default 24h).
  Reopening a stopped project boots it again (~2 s) on connect.

## 3. Install from scratch (R11)

Works on baremetal Debian 13 or a GCP VM (needs nested virtualization:
`gcloud compute instances create ... --enable-nested-virtualization`).

```bash
git clone <repo> styrman && cd styrman

# 1. Host: Docker CE, Kata (runtime "mvm"), daemon.json (incl. the
#    time-namespaces fix, ADR-009), KSM tuning, /var/lib/styrman dirs.
#    Idempotent; ends by boot-testing a microVM.
sudo bash deploy/host-setup.sh --user $USER
newgrp docker   # or log out/in, if you weren't in the docker group

# 2. Shared home: dotfiles, SSH key, gitconfig, agent harness (if you have a
#    devshell_prompt checkout — pass --harness <path> otherwise).
bash deploy/home-init.sh

# 3. Runtime image (~10 min first time, 3.75 GB)
docker build -t styrman-runtime:dev image/

# 4. Run (dev: no auth, localhost)
STYRMAN_IMAGE=styrman-runtime:dev go run ./cmd/styrman/
# → http://127.0.0.1:8080

# …or the production stack (Google OAuth + HTTPS): fill deploy/.env, then
docker compose -f deploy/docker-compose.yml up -d --build
```

First use: the UI shows a **First-time setup** banner (until marked done).
"Open setup" boots a hidden system microVM (~1 s, no workspace/DinD) running
the guided `styrman-setup` script: git identity → SSH key → gh → claude → agy,
each idempotent. Everything lands in the shared home — claude's wizard writes
`~/.claude`, agy's Google flow stores its token in the keyring at
`~/.local/share/keyrings` (spike S4) — so onboarding happens **once, ever**;
"Mark setup done" records a marker in the home and removes the VM. The same
script is available as `styrman-setup` in any project terminal, and new
shells hint at it while credentials are missing. (agy fallback:
`GEMINI_API_KEY` in `~/.bash_secrets`.)

## 4. Operating it

- **New project**: UI form (name, git-init) or
  `curl -X POST localhost:8080/api/projects -d '{"name":"x","gitInit":true}'`.
  ~2 s to a live terminal; type `claude` or `agy` in it.
- **Jump projects/terminals**: click in the sidebar; each terminal is an
  independent tmux session.
- **Side panel** (right): **Files** browses the project workspace and the
  shared home (view, edit, download — works while the project is stopped);
  **Ports** lists what is listening inside the microVM with shareable links
  (`/p/<project>/<terminal>/port/<port>/`); **History** shows the terminal
  transcripts; **Diff** shows the workspace's git status and diff.
- **Settings** (sidebar footer): who is signed in, allowed accounts (add or
  revoke), setup status, theme, runtime and storage facts.
- **Stop / start / delete**: API or UI. Delete removes the container; the
  workspace stays unless you pass `wipe=true`.
- **Where things live** on the host:

| Path | What |
|---|---|
| `/var/lib/styrman/home` | THE shared home — the only thing you must back up |
| `/var/lib/styrman/home/projects/<p>` | Project workspaces (code, git) |
| `/var/lib/styrman/home/.dockerd/<p>.img` | Per-project Docker disk (sparse; delete to reclaim) |
| `/var/lib/styrman/home/.styrman/logs/<p>/` | Terminal transcripts (History tab) |
| `/var/lib/styrman/projects.json` | Project metadata |
| containers `styrman-<p>`, networks `sdb-<p>` | Disposable runtime state |

- **Server restart / host reboot**: containers restart themselves
  (`unless-stopped`); the server re-adopts them from labels on boot.
- **Disaster recovery**: fresh host → `host-setup.sh` → restore
  `/var/lib/styrman` → rebuild image → done.

## 5. Troubleshooting

| Symptom | Cause / fix |
|---|---|
| `invalid namespace type` starting any microVM | Docker ≥ 29.5 time namespaces vs Kata ≤ 3.31. `host-setup.sh` sets `"features":{"time-namespaces":false}` in daemon.json (ADR-009). |
| dockerd inside a project uses `vfs`, feels slow | The loop mount of `~/.dockerd/<p>.img` failed (see container logs); entrypoint fell back. Usually a loop-device or e2fsprogs issue. |
| agy can't keep credentials / re-asks every time | Keyring didn't start: check `/etc/profile.d/styrman-keyring.sh` ran (login shell), `/run/styrman/keyring-pw` exists, and `~/.local/share/keyrings` is writable. Fallback: `GEMINI_API_KEY` in `~/.bash_secrets`. |
| Files created by a project owned by root on the host | The in-guest user's UID must match the home's host owner. The server passes `STYRMAN_UID/GID` from the home dir's owner — check `ls -ld /var/lib/styrman/home`. |
| Terminal shows "replaced" | That terminal was opened in another tab (close code 4001). One attach per terminal, by design. |
| History tab is empty | Transcripts start when a tmux session is created; sessions predating ADR-017 have none. Check `~/.styrman/logs/<project>/` and that `/etc/styrman/tmux-project.conf` exists in the container. |
| Project won't start, `waitReady` timeout | `docker logs styrman-<p>` — the entrypoint logs each step (`[styrman] ...`). |

## 6. Development

```bash
go test ./...                                        # unit
bash .agents/docs/testing/styrman/integration.sh     # full lifecycle vs real Kata (18 checks)
go build ./... && go vet ./...
```

Layout: `cmd/styrman` (server), `cmd/wsprobe` (WS test client),
`internal/{config,project,orchestrator,conversation,terminal,httpapi}`,
`image/` (runtime image), `deploy/` (host-setup, home-init), `web/` (Phase 2
UI). Conventions: AGENTS.md; non-negotiables: REQUIREMENTS.md (R1–R11).

## 7. Roadmap

- **Phase 2** — real UI: TanStack Start SPA, Antigravity-style layout.
- **Phase 3** — Caddy + Google OAuth ingress (reused devbox-caddy), userstore,
  systemd + compose. Needs: OAuth client, Cloudflare token, domain.
- **Phase 4** — conversation history from `~/.claude/projects`, port preview,
  upload/download, file viewer, ops panel, git badges.

Lineage: combines `devbox` (Kata multi-tenant shell platform; orchestrator,
terminal protocol, Caddy ingress) and `argolis-devbox` (single-VM cloud
shell; terminal UX patterns). See `.agents/docs/plans/` for the full analysis.
