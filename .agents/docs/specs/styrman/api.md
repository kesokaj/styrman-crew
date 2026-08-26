# REST API spec

Base: the styrman server (default `127.0.0.1:8080`). All bodies are JSON,
camelCase. Errors: `{"error":{"message":"..."}}` with an appropriate 4xx/5xx.
Phase 1 has no auth; Phase 3 puts Caddy + Google OAuth in front and the server
re-checks `X-Caddy-Email` against the userstore.

## Projects

| Method & path | Purpose |
|---|---|
| `GET /api/projects` | List projects, newest first, each with live `state` (`running` / `stopped` / `absent`). |
| `POST /api/projects` | Create: `{"name","agent","gitInit"}`. Validates slug (`^[a-z][a-z0-9-]{0,30}$`); `agent` optional/deprecated (ADR-014), makes `~/projects/<name>`, records metadata, **boots the microVM synchronously** (~2 s; up to 2 min timeout), optionally `git init -b main` inside the guest as the user. Returns 201 + the project. 409 on duplicate. |
| `GET /api/projects/{p}` | One project + state. |
| `DELETE /api/projects/{p}[?wipe=true]` | Destroy container + network and remove metadata. `wipe=true` additionally deletes the workspace dir and the project's `~/.dockerd/<p>.img` — irreversible, so opt-in per request. |
| `POST /api/projects/{p}/start` | Ensure the microVM is running (used after idle-stop). |
| `POST /api/projects/{p}/stop` | Stop the container. Home/workspace persist; tmux sessions end. |

## Conversations (durable, ADR-011)

A conversation is a persisted record (`conversations.json`) whose live view
is a tmux session `conv-<id>` in the project microVM. For claude, the
record's `agentSession` UUID pins the Claude Code session: the first attach
runs `claude --session-id <uuid>`, later attaches (after container restarts,
idle stops, reboots) run `claude --resume <uuid>` — history survives
everything. agy conversations relaunch `agy` (native-resume wiring TODO).

| Method & path | Purpose |
|---|---|
| `GET /api/projects/{p}/conversations` | Durable records (present even for stopped projects), newest-opened first, merged with live tmux state: `{id, title, agentSession, createdAt, lastOpened, live, attached}`. |
| `POST /api/projects/{p}/conversations` | Create a record: `{"title"}` (optional; defaults to "Conversation N"). tmux session created lazily on first WS attach. |
| `PATCH /api/projects/{p}/conversations/{c}` | Rename: `{"title"}`. |
| `DELETE /api/projects/{p}/conversations/{c}` | Kill the tmux session and remove the record. The agent's transcript files in the shared home are kept. |

## Terminals (ephemeral)

Plain shells (`term-<id>` tmux sessions) — the right rail in the UI. Listed
straight from tmux, no records, gone when killed or the container stops.

| Method & path | Purpose |
|---|---|
| `GET /api/projects/{p}/terminals` | Live shells: `{"id","attached"}`. |
| `POST /api/projects/{p}/terminals` | Next free numeric id (session created on WS attach with `kind=shell`). |
| `DELETE /api/projects/{p}/terminals/{t}` | Kill the shell. |

## Terminal

`GET /ws/term/{p}/{c}` — WebSocket; see `terminal-protocol.md`.

## Setup (first-time onboarding)

A hidden system microVM (`styrman-sys-setup`, label `styrman.system=setup`,
never listed as a project) runs the guided `styrman-setup` script; every
credential it produces lands in the shared home, so onboarding happens once.

| Method & path | Purpose |
|---|---|
| `GET /api/setup/status` | Host-side probe of the shared home: `{gitIdentity, sshKey, github, claude, keyring, done, vmRunning}`. `done` = the operator marked setup finished (marker `~/.styrman/setup-done`). |
| `POST /api/setup/start` | Boot the hidden VM (minimal mode: no workspace/DinD, ~1 s). |
| `GET /ws/setup` | Terminal into it, running `styrman-setup` then a shell (same protocol as `/ws/term`). |
| `POST /api/setup/done` | Write the done marker into the home and destroy the VM. |
| `DELETE /api/setup` | Destroy the VM without marking done. |

## Misc

| Method & path | Purpose |
|---|---|
| `GET /api/health` | `{"status":"ok"}` iff Docker is reachable. |
| `GET /api/version` | `{"version":"<ldflags-stamped>"}`. |
| `GET /dev/` | Phase 1 test console (removed in Phase 2). |

## Environment variables (server)

| Var | Default | Meaning |
|---|---|---|
| `STYRMAN_ADDR` | `127.0.0.1:8080` | Listen address |
| `STYRMAN_STATE_DIR` | `/var/lib/styrman` | State root (`home/`, `projects.json`) |
| `STYRMAN_HOME_DIR` | `<state>/home` | Shared home mounted into every microVM |
| `STYRMAN_IMAGE` | `styrman-runtime:latest` | Runtime image |
| `STYRMAN_RUNTIME` | `mvm` | Docker runtime (Kata) |
| `STYRMAN_USER` | current user | In-guest username; must own the home |
| `STYRMAN_DOCKER_DISK_GB` | `20` | Per-project docker.img size |
| `STYRMAN_KEYRING_PW` | `styrman` | In-guest keyring password (convenience-grade) |
| `STYRMAN_IDLE_TIMEOUT` | `24h` | Idle stop; Go duration, `0` disables |
