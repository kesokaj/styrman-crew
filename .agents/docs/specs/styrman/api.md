# REST API spec

Base: the styrman server. All bodies JSON, camelCase; errors
`{"error":{"message":"..."}}`. Every request carries an identity: the
Google email injected by the ingress (`X-Caddy-Email`, auth mode `header`),
or the configured default user on the loopback dev listener (mode `none`).
All project/terminal/file/history resources are scoped to that user
(ADR-019). `{id}` below is always the project **UUID** (ADR-020); friendly
names are display-only.

## Projects

| Method & path | Purpose |
|---|---|
| `GET /api/projects` | The user's projects, newest first: `{id, name, dir, createdAt, gitInit, archived, state}`. |
| `POST /api/projects` | Create: `{"name","cloneUrl","github","gitInit"}`. Every project is a git repo, no exceptions (R13, ADR-022): by default a synchronous `git init -b main` runs before the `201`; `cloneUrl` (mutually exclusive with `github`) clones that URL in-guest instead, using the user's ssh key/gh credentials, falling back to an empty repo on failure; `github` (`"private"` or `"public"`) also creates and pushes a GitHub repo via the user's `gh` login. `gitInit` is accepted for compatibility and ignored. Returns **201 immediately** with `state:"creating"`; the microVM boots in the background (terminal attach waits for readiness). Name must not collide with a *current* project; reuse after deletion is fine. |
| `GET /api/projects/{id}` | One project + live state. |
| `PATCH /api/projects/{id}` | Rename (friendly name only): `{"name"}`. The workspace dir (`dir`) never moves. |
| `DELETE /api/projects/{id}[?wipe=true]` | Destroy the microVM + records. `wipe` also deletes workspace, `~/.dockerd/<id>.img`, and transcripts — the UI requires typing "delete". |
| `POST /api/projects/{id}/start` · `/stop` | Boot / stop the microVM. |

## Terminals (durable, ADR-016)

A terminal is a persisted record whose live view is a tmux session
(`conv-<n>`) in the project microVM. Every terminal is a login shell — run
`claude`, `agy`, or anything; several at once to drive several agents.

| Method & path | Purpose |
|---|---|
| `GET /api/projects/{id}/terminals` | Records merged with live tmux state: `{id, title, createdAt, lastOpened, live, attached, bell}`. `bell` (bool): rang the terminal bell while unattended — agent likely waiting for input; cleared by tmux when the session is viewed. Listed even when the project is stopped. |
| `POST /api/projects/{id}/terminals` | Create: `{"title"}` optional (default "Terminal N"). Session created lazily on WS attach. |
| `PATCH .../terminals/{t}` | Rename: `{"title"}`. |
| `POST .../terminals/{t}/pause` | Kill the session, keep the record + transcript ("paused"; reopening starts a fresh shell). |
| `DELETE .../terminals/{t}` | Kill session + remove record (transcript stays in History). |
| `DELETE .../terminals` | Close all terminals in the project. |

`GET /ws/term/{id}/{t}` — the terminal WebSocket (see terminal-protocol.md).
`GET /ws/setup` — the user's hidden onboarding VM terminal.

### Notifications

| Method & path | Purpose |
|---|---|
| `GET /api/notifications` | `{"notifications":[{"projectId","projectName","terminalId","title"}]}` — terminals across the user's *running* projects whose tmux window bell flag is set. One docker exec per running project per call; the UI polls this every 8 s. |

## Ports & preview

| Method & path | Purpose |
|---|---|
| `GET /api/projects/{id}/ports` | Listening TCP ports ≥1024 inside the microVM: `{port, process?, image?}` — process from /proc, image when the port belongs to the project's inner docker. |
| `/p/{id}/{t}/port/{port}/...` | Shareable link proxying to the app inside the microVM (WS pass-through). Bare form 301s to the trailing slash; a first-ever preview may 307 once while the server joins the project network. |
| `/proxy/{id}/{port}/...` | Terminal-independent form. |

## Files (ADR-018)

Scopes: the project workspace (`/api/projects/{id}/...`) or the user's home
(`/api/home/...`). Served from the host mount — works while stopped.
Traversal and symlink escapes refused.

| Method & path | Purpose |
|---|---|
| `GET .../files?path=` | Directory listing (dirs first, dotfiles included). |
| `GET .../file?path=` | Contents (2 MiB cap, binary-flagged). |
| `PUT .../file?path=` | Save/create: `{"content"}`. |
| `POST .../upload?path=` | Multipart upload into a directory (drag & drop; ≤512 MB). |
| `GET .../download?path=` | Raw download. |

## History & diff (ADR-017)

| Method & path | Purpose |
|---|---|
| `GET /api/projects/{id}/history` | Terminal transcripts (`~/.styrman/logs/<id>/`): `{id, size, modified}`. |
| `GET .../history/{session}` | Transcript text, ANSI-stripped, tail-capped (`?bytes=`). |
| `GET /api/projects/{id}/diff` | `{repo, branch, status, diff}` from git in the workspace. Every workspace is a git repo (R13), so this never 404s for a missing repo. |

## Checkpoints (ADR-022)

Automatic whole-tree snapshots of the workspace (tracked + untracked files;
`.gitignore`d files excluded), stored under `refs/styrman/checkpoints/*` via
a private index — never on the user's branches, never pushed, invisible to
`git log`. Capped at 40 (oldest pruned). Auto-triggers: project stop,
terminal pause/close/close-all, and before every restore; manual via the
Git tab's "Checkpoint now" button or the POST below.

| Method & path | Purpose |
|---|---|
| `GET /api/projects/{id}/checkpoints` | Newest first: `{"checkpoints":[{id,time,subject}]}`. |
| `POST /api/projects/{id}/checkpoints` | Create: `{"label"}` optional (default `"manual"`). **201** `{"id","created":true}`; **200** `{"created":false}` when the tree is unchanged since the newest checkpoint (dedup). |
| `POST .../checkpoints/{id}/restore` | Rewrite the working tree to that checkpoint exactly (later files deleted; ignored files survive; the user's branches/commits untouched). Snapshots the current state first, so restores are themselves undoable: `{"restored","safetyId"}`. |

## Settings, profile & admin

| Method & path | Purpose |
|---|---|
| `GET /api/settings` | Version, identity (`user`, `username`, `isAdmin`), auth mode, allowed accounts, per-user home, runtime/storage facts, `terminalUsername`, setup status. |
| `POST /api/settings/profile` | Set `{"terminalUsername"}` — the in-guest username; applies to microVMs started afterwards. Empty resets to the identity slug. |
| `GET /api/settings/tools` | Per-tool cards from the user's home: git/ssh/github/claude/agy — ready state, details, and claude/agy skills, subagents, commands. |
| `POST /api/settings/users` · `DELETE /api/settings/users/{email}` | **Admin only** (env-provided accounts). Manage who may sign in; each user gets their own isolated space. |

## Setup (first-time onboarding, per user)

| Method & path | Purpose |
|---|---|
| `GET /api/setup/status` | Probe of the user's home: git/ssh/github/claude/keyring + done + vmRunning. |
| `POST /api/setup/start` | Boot the user's hidden minimal VM (~1 s). |
| `POST /api/setup/done` | Write `~/.styrman/setup-done`, destroy the VM. |
| `DELETE /api/setup` | Destroy the VM without marking done. |

## Misc

`GET /api/health` (open, for probes) · `GET /api/version`.

## Server environment

| Var | Default | Meaning |
|---|---|---|
| `STYRMAN_ADDR` | `127.0.0.1:8080` | Main listener |
| `STYRMAN_DEV_ADDR` | unset | Auth-bypassed second listener — publish on host loopback ONLY |
| `STYRMAN_AUTH` | `none` | `header` behind the ingress |
| `STYRMAN_INGRESS_TOKEN` | unset (disabled) | Shared secret the ingress injects as `X-Styrman-Ingress`; header mode rejects requests without it when set — blocks a project VM from spoofing `X-Caddy-Email` directly against the server |
| `STYRMAN_ALLOWED_EMAILS` | — | Admin accounts (comma-sep); first one is the dev-listener identity |
| `STYRMAN_STATE_DIR` | `/var/lib/styrman` | Root: `homes/<user>`, `users/<user>/*.json`, `users.json` |
| `STYRMAN_IMAGE` / `STYRMAN_RUNTIME` | `styrman-runtime:latest` / `mvm` | Project microVM image + runtime |
| `STYRMAN_SELF_CONTAINER` | unset | The server's own container name (containerized deploys) |
| `STYRMAN_USER` | OS user | Dev-mode default identity when no allowed emails |
| `STYRMAN_DOCKER_DISK_GB` / `STYRMAN_KEYRING_PW` / `STYRMAN_IDLE_TIMEOUT` | 20 / styrman / 24h | Per-project disk, guest keyring, idle stop |
