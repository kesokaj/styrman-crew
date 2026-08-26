# styrman (né super-devbox) — Implementation Plan

Date: 2026-08-26
Status: awaiting approval to start
Architecture: see `2026-08-26-super-devbox-architecture.md` (rev 2 — all major decisions locked)

Host state verified 2026-08-26: NUC12WSHi7, Docker 29.7.2 with Kata `mvm` + gVisor registered, `/dev/kvm` present, no containers running, only sshd listening. Clean slate — no coexistence constraints with the old devbox stack.

## Success criteria (the end state)

1. Browse to `https://<domain>`, log in with Google, land in the styrman UI.
2. "New Project" → name + agent (`claude`/`agy`) + git-init toggle → < 5 s later a terminal is running that agent inside a dedicated Kata microVM, cwd `~/projects/<name>`.
3. Sidebar switches between projects and between conversations within a project; each conversation is an independent tmux session; browser reload reattaches seamlessly.
4. Every project has working `docker` inside its microVM, isolated from all other projects.
5. Destroy and recreate any project container: workspace, git history, and all credentials (`~/.ssh`, `~/.claude`, `~/.gemini`, gcloud, gh) survive. Sign in to claude/agy/gh/gcloud **once ever**.
6. Idle projects stop automatically; reopening restarts them transparently.
7. Server reboot: `docker compose up` (or systemd unit) restores everything; state recovered from labels + filesystem, no database.

## Repo layout

```
styrman/  (dir currently ~/workspace/super-devbox — rename pending, see TODO)
├── AGENTS.md  REQUIREMENTS.md  TODO.md  README.md
├── go.mod                        # github.com/kesokaj/styrman
├── cmd/styrman/main.go       # the single server binary (UI + API + orchestrator)
├── internal/
│   ├── config/                   # ports, label names, header names, env keys, paths
│   ├── orchestrator/             # Docker client: create/start/stop/adopt/idle microVMs
│   ├── terminal/                 # docker-exec ↔ WebSocket bridge (protocol ported from devbox)
│   ├── project/                  # projects.json store, name validation, git init
│   ├── conversation/             # tmux session list/create/kill via exec
│   ├── userstore/                # ported from devbox pkg/userstore
│   └── httpapi/                  # REST + WS routing, auth middleware, embedded UI
├── web/                          # TanStack Start + TS + Tailwind (SPA, static export)
├── image/                        # runtime image: Dockerfile, entrypoint.sh, configs/
├── deploy/                       # docker-compose.yml (server + caddy), host-setup.sh, .env.example
└── .agents/docs/{changes,architecture,specs,testing,plans}/
```

Two Go modules max: the server (above) and nothing else — the Caddy ingress module is consumed as the existing `devbox-caddy` image, not re-vendored, until/unless we need changes to it.

## Phases

### Phase 0 — Scaffold + de-risking spikes (do first, everything else depends on the answers)

Scaffold: project docs from `devshell_prompt/harness/shared/templates/`, go.mod, git init, first ADRs (ADR-001 microVM-per-project, ADR-002 exec transport, ADR-003 shared home, ADR-004 per-project DinD, ADR-005 no code-server, ADR-006 TanStack UI).

Spikes — each a throwaway script/program under `.agents/docs/testing/spikes/`, results recorded in the plan:

| # | Question | Method | Fallback if it fails |
|---|---|---|---|
| S1 | Does `docker exec -it` (TTY + resize) stream cleanly into a Kata container? | Run debian under `--runtime mvm`, small Go prog using Docker SDK exec with TTY, measure echo latency, test resize | In-container shell-server (devbox model) |
| S2 | N Kata microVMs virtiofs-mounting the same host dir: write visibility, locking behavior | Two `mvm` containers, same bind mount, cross-write test | Split mounts: shared `~` read-mostly + per-project workspace mount |
| S3 | dockerd inside Kata with data-root on the virtiofs home — does overlay2 work, or does devbox's setup rely on something else? Read devbox `entrypoint.sh` closely first | Boot runtime-image prototype, `docker run hello-world` inside | data-root on a per-project named volume instead of the home |
| S4 | Headless `gnome-keyring-daemon` + dbus in the container satisfies agy's Secret Service auth; keyring files persist in `~/.local/share/keyrings` | Prototype entrypoint, run `agy` sign-in via terminal, restart container, verify no re-auth | `GEMINI_API_KEY` in `~/.bash_secrets` |
| S5 | Kata microVM cold-start time with our image (target < 5 s to prompt) | Time `docker run` → tmux ready | Pre-warm: keep last-used projects running; adjust idle timeout |

Exit: all five spikes answered, decisions recorded as ADR amendments.

### Phase 1 — Runtime image + core backend (no UI, no auth)

1. **Runtime image** (`image/`): Debian trixie, tool set per architecture doc §5, `claude` + `agy` baked in, entrypoint adapted from devbox (user creation, cgroup nesting, per-project dockerd, keyring per S4, version-aware home-config deploy preserving `~/.bash_secrets`, tmux, PID-1 reaper). AMD64 only.
2. **Shared-home init**: first-run routine that creates `/var/lib/styrman/home`, deploys dotfiles + the `devshell_prompt/harness` (both vendors) into it, versioned marker file.
3. **Orchestrator** (`internal/orchestrator`): create/start/stop/destroy project microVMs with devbox's exact Kata config; per-project bridge network `sdb-<name>`; label schema `styrman.{project,agent,created}`; `RecoverState` adoption on boot; idle reaper (stop after N h, configurable).
4. **Project store** (`internal/project`): `projects.json` (atomic writes, userstore pattern) for agent choice/created/archived; slug validation; `git init` + initial commit using the shared-home gitconfig.
5. **Terminal bridge** (`internal/terminal`): WS endpoint → `ContainerExecCreate` TTY → `tmux new -A -s conv-<id> -c ~/projects/<name> <agent-cmd>`; port devbox's protocol exactly (binary = PTY bytes, JSON text = resize, close 4000 = exited, 4001 = replaced, single WS per conversation).
6. **REST API** (`internal/httpapi`), JSON per json-api-style skill:
   - `GET/POST /api/projects`, `GET/PATCH/DELETE /api/projects/{p}` (delete = destroy container; `?wipe=true` also removes workspace after confirmation)
   - `POST /api/projects/{p}/start|stop`
   - `GET/POST /api/projects/{p}/conversations`, `DELETE .../{c}` (kills tmux session)
   - `GET /ws/term/{p}/{c}`
   - `GET /api/health`, `GET /api/version`
7. **Test harness**: a plain static test page (xterm.js, no framework) served at `/dev/` for manual verification, plus Go tests for store/slug/label logic and an integration test script (`.agents/docs/testing/styrman/`) that exercises the full lifecycle with curl + a WS client.

Exit criteria: success-criteria items 2 (via test page), 3 (tmux reattach), 4, 5, 6, 7 — all demonstrated locally over plain HTTP on localhost.

### Phase 2 — The real UI

TanStack Start SPA in `web/`, static build embedded via `go:embed`. Antigravity-2.0-inspired layout:

- **Left sidebar**: project list (running/stopped indicator) → conversations nested under each, "New Conversation" per project, "New Project" button at top, settings at bottom.
- **New Project modal**: name, agent picker (claude/agy), git-init toggle.
- **Center**: xterm.js terminal for the active conversation (the agent CLI is the chat). Tab bar or conversation title header.
- **Right panel** (collapsible): additional plain-shell terminals into the same project ("Terminals" like Antigravity's right rail). Artifacts/files-changed panes are Phase 4.
- Keyboard: quick-switcher (Ctrl+K) across projects/conversations.
- Theme: dark default, light supported.

Frontend talks only to the API from Phase 1 (one addition: server-sent events or polling for project/conversation state so the sidebar stays live). Follow `frontend` + `typescript` skills.

Exit: daily-drivable UI on localhost; the `/dev/` test page is deleted.

### Phase 3 — Auth + ingress + service-ification

1. Reuse the `devbox-caddy` image (Caddy + caddy-security Google OAuth + Cloudflare DNS + label ingress) unchanged; run it and the styrman server via `deploy/docker-compose.yml`; server labeled `caddy_hostname: <name>` so ingress picks it up.
2. Auth middleware: require `X-Caddy-Email` + `userstore.IsAuthorized` (devbox's two-layer model); superadmins from `SUPERADMINS` env. Server binds to an internal interface only.
3. `deploy/host-setup.sh`: idempotent — dirs under `/var/lib/styrman`, compose up, systemd unit for boot.
4. `.env.example` documenting: `GOOGLE_CLIENT_ID/SECRET`, `CLOUDFLARE_API_TOKEN`, `BASE_DOMAIN`, `SUPERADMINS`, `IDLE_TIMEOUT`.
5. **Needs from user at this point**: the Google OAuth client credentials, Cloudflare token, and domain (the old devbox `.env` isn't on this machine).

Exit: success-criterion 1; full flow from a phone/laptop on the network.

### Phase 4 — Comfort features (ordered, each independently shippable)

1. Conversation history: parse `~/.claude/projects/*/` (+ agy equivalent) to list past sessions with titles; "resume" starts a conversation with `claude --resume <id>`.
2. Port preview: exec-based `/proc/net/tcp` scan + `/proxy/{p}/{port}/` reverse proxy to the project microVM's IP.
3. File upload/download into `~/projects/<name>` (port devbox `transfer`).
4. Lightweight file viewer/editor panel (CodeMirror; read + simple edits — not an IDE).
5. Admin/ops view: per-project disk & memory usage, logs, force-destroy, home-dir size.
6. Git status in sidebar (dirty/branch badge per project).

## Testing strategy (local-first, per global standards)

- Go unit tests for every `internal/` package with logic (store, slugs, labels, protocol framing).
- Integration script per phase in `.agents/docs/testing/styrman/` — real Docker, real Kata, no mocks for the orchestrator path.
- Manual test checklist for terminal UX (reattach, resize, close codes) — the part unit tests can't see.
- `reviewer` subagent after each phase's significant changes; `testing` subagent when code lands without tests; `changelog` + `documentation` after each phase.

## Documentation deliverables

- `README.md` (quick start, architecture sketch, deploy) — updated every phase.
- `.agents/docs/specs/styrman/` — API spec + terminal protocol spec (Phase 1), UI spec (Phase 2), deploy spec (Phase 3).
- ADRs appended as decisions happen; changelog entry per completed task.

## Risks

| Risk | Likelihood | Mitigation |
|---|---|---|
| overlay2 broken on virtiofs → DinD storage fails | Medium | S3 first; named-volume fallback |
| agy keyring in headless container | Medium | S4; API-key fallback documented |
| Kata cold start > 5 s feels sluggish | Medium | S5; keep recent projects warm, honest idle timeout |
| Exec streaming flaky under Kata | Low | S1; shell-server fallback is proven code |
| Concurrent shared-home writes corrupt state | Low | S2; write-discipline rule + split-mount fallback |
| Scope creep toward rebuilding an IDE | High | REQUIREMENTS.md will state "terminal-first, no embedded IDE" as non-negotiable |

## Order of work once approved

Phase 0 scaffold → S1–S5 spikes → report results → Phase 1. Phases are strictly sequential; within Phase 1, image and orchestrator proceed in parallel with the terminal bridge.
