# Terminal protocol spec

The wire protocol between the browser and `/ws/term/{project}/{conv}`, ported
verbatim from devbox so its hard-won semantics carry over.

## Endpoint

```
GET /ws/term/{project}/{conv}?cols=<n>&rows=<n>[&kind=shell]
```

- `{project}`: an existing project slug (404 otherwise).
- `{conv}`: conversation id, `[a-z0-9][a-z0-9-]{0,30}`. Maps to tmux session
  `conv-<id>` inside the project's microVM.
- `cols`/`rows`: the client's real terminal size at connect time. Passing them
  in the URL lets tmux create the session at the right size, avoiding the
  80x24-then-resize double-prompt redraw.
- `kind=shell`: attach a plain `bash -l` instead of the project's agent CLI.
  Used for auxiliary terminals (the right-rail "Terminals" in Phase 2).

Connecting boots the project's microVM if it is stopped (this is the one slow
path — up to a few seconds; the HTTP upgrade completes only after readiness).

## Frames

| Direction | Frame | Meaning |
|---|---|---|
| server → client | binary | Raw PTY bytes. Write straight into xterm.js. |
| client → server | binary | Raw input bytes (keystrokes, paste). |
| client → server | text (JSON) | Control: `{"type":"resize","cols":N,"rows":N}`. |

There are no other text messages; unknown control types are ignored.

## Close codes

| Code | Name | Client behavior |
|---|---|---|
| 4000 | shell exited | The tmux session genuinely ended (user typed `exit`, agent quit). Do NOT silently reconnect — show an "exited" state; reconnecting would create a fresh session. |
| 4001 | replaced | A newer WebSocket attached to this conversation (another tab). NEVER auto-reconnect — doing so death-spirals: each reconnect replaces the other tab, which reconnects, forever. |
| 1000/1001/1006 | normal/going away/abnormal | Transient. Silently reconnect; tmux reattaches with scrollback and running processes intact. |

## Session semantics

- One live WebSocket per conversation, enforced server-side (`activeConns`).
- The tmux session is created lazily by the first attach:
  `tmux new-session -A -s conv-<id> -x <cols> -y <rows> -c ~/projects/<project> <cmd>`
  where `<cmd>` is the project's agent CLI (or `bash -l` for kind=shell).
  `-A` attaches when the session exists (the command is then ignored).
- The pane command exiting ends the session → close 4000.
- A browser disconnect never signals the shell; the session stays alive until
  killed via the API, the pane command exits, or the container stops.
- Container stop/idle-reap ends all tmux sessions. Agent history survives in
  the shared home (`claude --resume`, agy's own history).
- Server keepalive: WebSocket ping every 30 s; a dead client is detected in
  ≤ 30 s instead of TCP-keepalive minutes.

## Transport internals (server side)

The server holds a Docker exec (TTY mode) into the project container —
`ContainerExecCreate` + `ContainerExecAttach` (hijacked raw stream, no
stdcopy demux in TTY mode) — and pumps bytes between it and the WebSocket.
Resize maps to `ContainerExecResize`. Exec stream EOF + a finished exec
inspect (`Running=false`) distinguishes "session exited" (→ 4000) from
attach teardown (→ normal close).
