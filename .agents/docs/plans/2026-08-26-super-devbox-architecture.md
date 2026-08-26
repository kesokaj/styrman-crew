# styrman (né super-devbox) — Architecture Proposal

Date: 2026-08-26 (rev 2 — user decisions incorporated)
Status: proposal
Inputs: exploration of `~/workspace/devbox` (v0.5.11), `~/workspace/argolis-devbox`, `~/workspace/devshell_prompt/harness`, Antigravity 2.0 UI screenshots, host inspection.

Decisions locked by the user (2026-08-26):
- Deploy target: **this machine** (Intel NUC12WSHi7, 16 threads, 62 GB RAM, 915 GB NVMe). Everything local. Host already has Docker 29.7.2 with Kata registered as runtime `mvm` (cloud-hypervisor/firecracker/QEMU under `/opt/kata`), gVisor `runsc`/`gvisor`, and `/dev/kvm`.
- Isolation: **Kata microVMs — 1 project = 1 container = 1 microVM.**
- Docker: **per-project Docker-in-Docker inside each microVM.** No shared host docker.sock.
- Agent CLIs: `claude` + `agy` (Antigravity CLI). Harness deployed from `~/workspace/devshell_prompt/harness` (vendor-neutral `shared/` + per-vendor setup, Claude as source of truth, agy config derived).

## 1. What we're building

A self-hosted web platform where you:

1. Log in with Google (Caddy + caddy-security, reused from `devbox`).
2. See a sidebar of **projects**; each project is a directory + its own Docker container.
3. Create a project by choosing a name, an agent (`claude` or `agy`), and whether to `git init`.
4. Open a project → the center pane is a browser terminal attached to a tmux session running the chosen agent CLI. The agent CLI **is** the chat window — no custom chat UI.
5. Jump between projects freely; sessions persist via tmux while the container runs, and via `claude --resume` / agent-native history across container restarts.
6. Never redo auth: one persistent host directory is mounted as `$HOME` into every project container, so `~/.ssh`, `~/.claude`, `~/.gemini`, `~/.config/gcloud`, `~/.gitconfig` etc. survive everything.

Explicitly **out**: code-server. No embedded VS Code. A lightweight read/edit file panel can come later (Phase 4); "Open IDE" can be a link that opens VS Code Remote-SSH / `code tunnel` if ever needed.

## 2. Why a rewrite (and what carries over)

`devbox` already solved the hard problems: OAuth ingress, programmatic container orchestration, WS↔PTY↔tmux terminal transport, persistent bind-mounted homes, state recovery from Docker labels, idle lifecycle. `argolis-devbox` contributed the polished single-tenant terminal UX and the Claude harness bundle, but is VM-based and not containerized.

The delta for styrman is: **project-shaped instead of user-shaped**. In `devbox`, the unit is "one container per user". Here it's "one container per project, N conversations (tmux sessions) per project, one shared home per user".

Note: `devbox/.agents/docs/plans/2026-06-12-argolis-devbox-transfer-analysis.md` already documents the merge boundary between the two codebases — read it before porting anything.

### Reuse map (from `devbox` unless noted)

| Component | Source | Verdict |
|---|---|---|
| Caddy ingress module (Docker-label discovery, Google OAuth, Cloudflare DNS, identity headers) | `caddy/ingress/` | Reuse as-is |
| Terminal WS↔PTY↔tmux bridge, close-code protocol (4000 exit / 4001 replaced) | `shell-launcher/server/internal/terminal/` | Reuse, re-keyed by project+conversation instead of username |
| Orchestrator: container create/adopt/idle-stop, state from labels | `internal/orchestrator/` | Adapt: per-project containers |
| userstore (JSON, atomic writes, env superadmins) | `pkg/userstore/` | Reuse as-is |
| Config versioning of home files, `~/.bash_secrets` preservation | `entrypoint.sh` pattern | Reuse pattern |
| Port scanner + `/proxy/<port>/` preview | both repos | Port later (Phase 4), exec-based |
| Claude harness bundle (skills, settings, templates) | both repos' `configs/claude/` | Reuse, deploy into shared home |
| devbox-cli, systemd/VM provisioning, IAP | argolis | Drop |
| code-server, `cmd/editor` supervisor | both | Drop |
| Vanilla-JS monolith UI | both | Drop — new frontend |

## 3. Architecture

```
Browser
  │ HTTPS
  ▼
Caddy (devbox-caddy image, reused)          — Google OAuth, TLS, DNS
  │ X-Caddy-Email / X-Caddy-User headers
  ▼
styrman server (one Go binary, container w/ docker.sock + state mount)
  ├── serves the UI (TanStack Start build, embedded via go:embed)
  ├── REST  /api/projects            create/list/archive/start/stop
  │         /api/projects/{p}/conversations   create/list/kill
  │         /api/history             parse ~/.claude/projects (later)
  ├── WS    /ws/term/{project}/{conversation}
  │            └── Docker exec API → PTY → tmux new -A -s conv-<id> <agent-cmd>
  └── Docker SDK: creates one container per project
        ▼
per-project Kata microVM (image: styrman-runtime, runtime: mvm)
  ├── tools: git, gh, gcloud, go, node, tmux, claude, agy, …
  ├── own dockerd (data-root ~/.dockerd/<project>)
  ├── tmux server: one session per conversation
  └── virtiofs mount:
       /home/<user>            ← shared persistent (auth, dotfiles, harness)
       /home/<user>/projects/<name>  (lives inside the home mount — nothing extra)
```

### Key decisions

**D1 — Terminal transport: Docker exec, no in-container server (v1).**
Unlike `devbox`, project containers run no HTTP server. The orchestrator uses the Docker API (`ContainerExecCreate` with TTY) to run `tmux new-session -A -s conv-<id> -c ~/projects/<name> <agent>` and bridges it to the browser WebSocket, reusing the existing resize/binary-frame/close-code protocol. Exec works with Kata (the kata-agent in the guest handles it), including TTY resize. Benefits: server logic updates without rebuilding the runtime image; the runtime image is just tools; fewer moving parts. The `devbox`-style in-container shell-server remains the fallback if exec streaming proves limiting (upload/download and port-proxy then get easier). Verify exec-stream latency/robustness under Kata as the first Phase 1 spike; if it disappoints, fall back to the shell-server model without changing anything user-visible.

**D2 — One shared `$HOME`, projects inside it.**
`/var/lib/styrman/home` on the host is bind-mounted as `/home/<user>` in **every** project microVM (Kata shares host dirs into the guest via virtiofs — same mechanism `devbox` already uses for its per-user homes). Project workspaces live at `~/projects/<name>`, so a single mount covers both credentials and code. Per-daemon state must be project-scoped:
- Docker-in-Docker: each microVM runs its own `dockerd` with `data-root ~/.dockerd/<project>/` (extends devbox's `~/.dockerd` convention with a per-project suffix so concurrent daemons never share storage).
- tmux: each microVM has its own tmux server (own `/tmp`), no conflict.
- Caveat to verify in Phase 1: this is N concurrent microVMs virtiofs-mounting the *same* host directory — unlike devbox, where each home had exactly one container. Fine for credentials/dotfiles (rarely written, single writer at a time), but virtiofs caching means cross-VM visibility of writes isn't instant, and two agents writing the same file from two projects could conflict. Rule: a project only writes inside `~/projects/<its-name>` plus its own `~/.dockerd/<name>`; shared-home writes (auth, config) are human-driven and serialized in practice. If this ever bites, fallback is splitting the mount: `~` shared read-mostly + per-project workspace mounts.

**D3 — Kata microVMs, exactly as devbox does it.**
1 project = 1 container = 1 microVM via runtime `mvm`. This is what gives hard isolation between projects (own kernel, own dockerd, agent YOLO-mode blast radius contained) and it's already proven on this host. Reuse devbox's container config verbatim from `internal/orchestrator/manager.go`: `Runtime: "mvm"`, `CapAdd: SYS_ADMIN, NET_ADMIN, NET_RAW` (not `--privileged` — breaks Kata device hotplug), `apparmor=unconfined`, explicit DNS 8.8.8.8/1.1.1.1 (Kata lacks Docker's embedded DNS), `net.ipv4.ip_forward=1`, and a dedicated bridge network per project (`sdb-<project>`). Also reuse the `system/` host tuning (nested-KVM modprobe, KSM/THP tmpfiles, sysctls) — with 62 GB RAM, KSM matters for running many idle microVMs; default Kata guest sizing (e.g. 2 vCPU / 4 GB, memory hotplug up) keeps ~10+ concurrent projects comfortable. Keep the runtime field configurable per-project anyway (runc escape hatch for a quick throwaway).

**D4 — State = Docker labels + filesystem + one small JSON file.**
No database. Containers carry labels (`styrman.project`, `styrman.agent`, `styrman.user`); `RecoverState` re-adopts them on restart (proven in `devbox`). Project metadata that must survive container deletion (agent choice, created-at, archived flag) goes in `/var/lib/styrman/projects.json` via the userstore pattern.

**D5 — Conversation = tmux session; history via agent-native resume.**
Sidebar "conversations" list = `tmux list-sessions` (exec'd) per running project. A conversation survives browser disconnects (tmux) but not container stop. That's fine: `claude --resume` (and gemini's equivalent) restore history from `~/.claude` / `~/.gemini` in the shared home. Later, parsing `~/.claude/projects/*/` gives the Antigravity-style "Conversation History" list with titles.

**D6 — Frontend: TanStack Start + TypeScript + Tailwind in `web/`** (per global standards), xterm.js for terminals, static-exported and embedded in the Go binary. Layout mirrors Antigravity 2.0: left sidebar (projects → conversations, New Project button), center terminal pane, collapsible right panel (extra terminals now; artifacts/files-changed later). No SSR needed — it's an SPA behind auth.

**D7 — Auth: Caddy identity headers + userstore re-check** (devbox model, host-agnostic — works on the home box and on a GCP VM identically). The server trusts `X-Caddy-Email` only because it binds to the internal Docker network / localhost; keep `devbox`'s second-layer `userstore.IsAuthorized` check. Drop IAP.

## 4. New project flow (the core feature)

`POST /api/projects {name, agent: "claude"|"agy", gitInit: bool}`:
1. Validate/slugify name; reject duplicates.
2. `mkdir ~/projects/<name>` (via host path); optionally `git init` + first commit with your gitconfig from the shared home.
3. Record in `projects.json`.
4. Create + start container `styrman-<name>` from `styrman-runtime` with labels, home mount, resource limits.
5. Create conversation #1: exec `tmux new -A -s conv-1 -c ~/projects/<name> <agent>`.
6. UI navigates to the new project's terminal. Total target time: < 3 s (image is pre-pulled).

Stopping/idle: reuse `devbox`'s idle reaper — stop project containers after N hours idle; starting is instant since the workspace and history persist.

## 5. Runtime image

One `styrman-runtime` image (Debian trixie), derived from `devbox`'s worker Dockerfile minus code-server/Playwright, plus both agent CLIs:
- Base tools: git, gh, tmux, jq, ripgrep, fd, curl, build-essential
- Clouds/infra: gcloud + gke-auth-plugin, kubectl, helm, opentofu, docker-ce + compose + buildx (for the in-VM dockerd)
- Languages: Go (latest), Node 22, Python 3
- Agents: `claude` (npm `@anthropic-ai/claude-code`), `agy` — Antigravity CLI, single Go binary: `curl -fsSL https://antigravity.google/cli/install.sh | bash` in the Dockerfile, binary moved to `/usr/local/bin/agy`
- Entrypoint (adapted from devbox's `entrypoint.sh`): create user matching host UID/GID, cgroup-v2 nesting + loop devices, start per-project `dockerd` (data-root `~/.dockerd/<project>`), version-aware home-config deploy (preserve `~/.bash_secrets`), start tmux server, PID-1 zombie reaper.

**Harness**: deployed from `~/workspace/devshell_prompt/harness` into the **shared home** (`~/.claude/*` + `~/.gemini/config/*` via its `setup.sh` / derive scripts) — because the home is shared and persistent, the harness is deployed once at home-init and on version bumps, not per container. The harness repo gets vendored (or git-pulled) into the shared home rather than baked into the image, so harness updates don't require image rebuilds.

**agy auth in a microVM**: `agy` stores OAuth credentials in the Linux Secret Service keyring, which doesn't exist in a bare container. Two options, in preference order: (1) entrypoint starts `dbus-run-session` + headless `gnome-keyring-daemon` with keyrings at `~/.local/share/keyrings` — persistent via the shared home, so the SSH-style copy-URL/paste-code sign-in flow happens once ever; (2) fallback: `GEMINI_API_KEY` in `~/.bash_secrets` (agy skips sign-in entirely). Verify (1) during Phase 1; ship (2) as the documented escape hatch. `claude` needs no such dance — its credentials are plain files under `~/.claude`.

AMD64 only (single NUC target, matching argolis's ADR-007); built and tested locally, no registry push needed while single-host — keep a Makefile target for Artifact Registry if that changes.

## 6. Phases

1. **Core loop (no auth, localhost):** runtime image + orchestrator + exec-terminal bridge + minimal UI. Success: create project → claude runs in browser → reload browser → session intact → recreate container → `claude --resume` works, ssh/gcloud creds intact.
2. **Real UI:** Antigravity-style layout, project/conversation sidebar, multiple terminals per project, agent picker, git-init toggle.
3. **Auth + ingress:** wire in reused Caddy ingress, userstore, identity headers; deploy on the target host.
4. **Comfort:** port preview proxy, upload/download, conversation-history parsing, lightweight file viewer/editor panel, idle reaper tuning.

## 7. Remaining open questions

- ~~Deployment target~~ → this NUC, everything local.
- ~~docker.sock vs DinD~~ → per-project DinD inside each microVM.
- ~~agy install method~~ → official install script, single Go binary; keyring handling per §5.
- Name: `super-devbox` placeholder — alternatives: `agentbox`, `foundry`, `hangar`.
- Phase 1 verification spikes (technical, no user input needed): docker-exec TTY streaming under Kata; concurrent virtiofs shared-home behavior; headless keyring for agy.
