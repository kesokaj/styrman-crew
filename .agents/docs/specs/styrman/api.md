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

## Terminals (durable, ADR-016)

A terminal is a persisted record (`conversations.json`) whose live view is a
tmux session `conv-<id>` in the project microVM. Every terminal is a login
shell — run `claude`, `agy`, or anything else in it; run several terminals
to work with several agents at once.

| Method & path | Purpose |
|---|---|
| `GET /api/projects/{p}/terminals` | Durable records (present even for stopped projects), newest-opened first, merged with live tmux state: `{id, title, createdAt, lastOpened, live, attached}`. |
| `POST /api/projects/{p}/terminals` | Create: `{"title"}` optional (defaults to "Terminal N"). The tmux session is created lazily on first WS attach. |
| `PATCH /api/projects/{p}/terminals/{t}` | Rename: `{"title"}`. |
| `DELETE /api/projects/{p}/terminals/{t}` | Kill the session and remove the record. The transcript stays in History. |

## Ports & preview

| Method & path | Purpose |
|---|---|
| `GET /api/projects/{p}/ports` | In-guest listening TCP ports ≥1024 (parsed from /proc/net/tcp{,6} via exec). |
| `/p/{p}/{term}/port/{port}/...` | Shareable link that proxies to the app inside the microVM (WebSockets pass through). Bare form 301s to the trailing slash. |
| `/proxy/{p}/{port}/...` | Same proxy, terminal-independent. |

## Files (ADR-018)

Two scopes: a project workspace, and the shared home. Served from the host
mount, so they work while the project is stopped. `path` is root-relative;
traversal and symlink escapes are refused.

| Method & path | Purpose |
|---|---|
| `GET /api/projects/{p}/files?path=` · `GET /api/home/files?path=` | Directory listing (dirs first). |
| `GET /api/projects/{p}/file?path=` · `GET /api/home/file?path=` | File contents (2 MiB cap, binary-flagged). |
| `PUT /api/projects/{p}/file?path=` · `PUT /api/home/file?path=` | Save: `{"content"}`. |
| `GET /api/projects/{p}/download?path=` · `GET /api/home/download?path=` | Raw download. |

## History & diff (ADR-017)

| Method & path | Purpose |
|---|---|
| `GET /api/projects/{p}/history` | Terminal transcripts: `{id, size, modified}`. |
| `GET /api/projects/{p}/history/{id}` | Transcript text, ANSI-stripped, tail-capped (`?bytes=`). |
| `GET /api/projects/{p}/diff` | `{repo, branch, status, diff}` from git in the workspace. |

## Settings

| Method & path | Purpose |
|---|---|
| `GET /api/settings` | Version, signed-in user, auth mode, allowed accounts, runtime and storage config. |
| `POST /api/settings/users` | Authorize a Google account: `{"email"}`. |
| `DELETE /api/settings/users/{email}` | Revoke (env-provided accounts cannot be revoked). |

## Terminal

`GET /ws/term/{p}/{c}` — WebSocket; see `terminal-protocol.md`.

## Ports & preview (Phase 4)

| Method & path | Purpose |
|---|---|
| `GET /api/projects/{p}/ports` | In-guest listening TCP ports ≥1024 (parsed from /proc/net/tcp{,6} via exec). |
| `/proxy/{p}/{port}/...` | Reverse proxy to the microVM's IP — open web apps running inside a project (WebSocket upgrades pass through). Behind authGuard like everything else. |

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
