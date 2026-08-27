# Terminal protocol spec

The wire protocol between the browser and `/ws/term/{project}/{conv}`, ported
verbatim from devbox so its hard-won semantics carry over.

## Endpoint

```
GET /ws/term/{projectId}/{terminal}?cols=<n>&rows=<n>
```

- `{projectId}`: the project's UUID (404 otherwise).
- `{terminal}`: terminal id, `[a-z0-9][a-z0-9-]{0,30}`. Maps to tmux session
  `conv-<id>` inside the project's microVM. The record must exist (created
  via POST) — unknown terminals are refused, never auto-created.
- `cols`/`rows`: the client's real terminal size at connect time. Passing them
  in the URL lets tmux create the session at the right size, avoiding the
  80x24-then-resize double-prompt redraw.

Connecting boots the project's microVM if it is stopped (this is the one slow
path — up to a few seconds; the HTTP upgrade completes only after readiness).

## Frames

| Direction | Frame | Meaning |
|---|---|---|
| server → client | binary | Raw PTY bytes. Write straight into xterm.js. |
| client → server | binary | Raw input bytes (keystrokes, paste). |
| client → server | text (JSON) | Control: `{"type":"resize","cols":N,"rows":N}`. |

There are no other text messages; unknown control types are ignored.

The client intercepts Ctrl/Cmd+C only when a selection exists (copies it;
otherwise the byte passes through as SIGINT), and Ctrl/Cmd+V / Ctrl+Shift+V
paste via the clipboard API.

## Close codes

| Code | Name | Client behavior |
|---|---|---|
| 4000 | shell exited | The tmux session genuinely ended (user typed `exit`, agent quit). Do NOT silently reconnect — the UI presents this state as **Paused** with a **Resume** action (the record and transcript persist; reconnecting starts a fresh shell). |
| 4001 | replaced | A newer WebSocket attached to this conversation (another tab). NEVER auto-reconnect — doing so death-spirals: each reconnect replaces the other tab, which reconnects, forever. |
| 1000/1001/1006 | normal/going away/abnormal | Transient. Silently reconnect; tmux reattaches with scrollback and running processes intact. |

## Session semantics

- One live WebSocket per conversation, enforced server-side (`activeConns`).
- The tmux session is created lazily by the first attach:
  `tmux new-session -A -D -s conv-<id> -x <cols> -y <rows> -c ~/projects/<dir> bash -l`
  — every terminal is a login shell (ADR-014/016); `-A` attaches when the
  session exists, `-D` detaches stale clients. Panes are transcribed via
  pipe-pane to `~/.styrman/logs/<projectId>/` (ADR-017).
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
