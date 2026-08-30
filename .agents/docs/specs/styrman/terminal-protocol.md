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

The client intercepts Ctrl/Cmd+V / Ctrl+Shift+V paste via the clipboard API,
and Cmd+C to copy a selection explicitly. Plain Ctrl+C is handled specially —
see Browser-side keyboard handling below.

OSC 52 (an escape sequence the guest PTY can emit to read/write the system
clipboard) is **write-only**: tmux/TUI copy (yank in vim, tmux copy-mode)
still lands on the system clipboard, but a guest-originated OSC 52 **read**
query (`\x1b]52;c;?`) is hard-denied — the xterm clipboard addon's `readText`
always returns empty. The (untrusted) guest is root in its own VM; honoring
an OSC 52 read would have handed it the user's real browser clipboard,
including anything just copied from the credential viewer (H8,
`web/src/lib/terminal.ts`).

## Browser-side keyboard handling

The wire protocol above is unchanged; these are `web/src/lib/terminal.ts`
key-event interceptors that decide what bytes get sent, ahead of xterm's own
handling (xterm supports exactly one custom key handler; this module chains
several via an internal `keyHandlers` array, first `false` wins).

- **Ctrl+C always interrupts.** Copy-on-select (the terminal copies the
  moment a selection is highlighted) leaves a lingering highlight, and the
  BROWSER's own copy-on-Ctrl+C swallowed the keydown before xterm ever saw
  it — so `^C` never reached the shell (couldn't kill e.g. `ping`), even
  though the guest PTY delivers SIGINT correctly once the byte arrives.
  Fixed: plain Ctrl+C sends ETX (`\x03`) explicitly via `term.input()` and
  calls `preventDefault()`, so neither the browser's copy nor xterm's
  default can intercept it; any lingering selection is cleared. Only Cmd+C
  (`metaKey`) still copies (v1.8.3, hardened v1.8.6).
- **AltGr / Option compose** non-US symbols (`@ $ { } [ ] \ € …`) instead of
  acting as Meta. xterm otherwise mistakes the modifier for Meta and sends
  `ESC <key>`, which readline renders as `(arg: N)` instead of typing the
  character. Detected per platform: Windows/Linux via the `AltGraph`
  modifier state or (Chrome) `ctrlKey && altKey` together; macOS via `altKey`
  alone (`macOptionIsMeta` is deliberately kept off so Option types symbols —
  Nordic Mac `@` is Option+2). When the OS has already composed a printable
  single character into `ev.key`, that exact character is typed and xterm is
  stopped; left-Alt-only (real Meta, e.g. readline `Alt+b`) and Option+Arrow/
  F-keys/dead keys are left untouched (v1.8.6 Windows/Linux, v1.8.7 macOS).
- **Nordic/Swedish dead keys** (`~ \` ^ ´ ¨`) are intercepted ahead of
  xterm's browser-IME-based composition, which is buggy for these layouts
  (xterm.js #2151/#2479). On a dead keydown (`ev.key === "Dead"`), the
  physical key (`ev.code`, layout-independent) + modifiers resolve to a
  pending spacing diacritic; the next keystroke resolves it — Space emits
  the bare diacritic, a vowel in the precompose table emits the accented
  letter (é, ã, ô, …), anything else emits the bare diacritic and falls
  through. Inert on layouts where those keys aren't dead (e.g. US), and
  falls through untouched for dead keys it doesn't map (v1.8.8/v1.8.9).

## Close codes

| Code | Name | Client behavior |
|---|---|---|
| 4000 | shell exited | The tmux session genuinely ended (user typed `exit`, agent quit). Do NOT silently reconnect — the UI presents this state as **Paused** with a **Resume** action (the record and transcript persist; reconnecting starts a fresh shell). |
| 4001 | replaced | A newer WebSocket attached to this conversation (another tab). NEVER auto-reconnect — doing so death-spirals: each reconnect replaces the other tab, which reconnects, forever. |
| 1000/1001/1006 | normal/going away/abnormal | Transient. Silently reconnect; tmux reattaches with scrollback and running processes intact. |

## Session semantics

- One live WebSocket per conversation, enforced server-side (`activeConns`).
- The tmux session is created lazily by the first attach:
  `tmux new-session -A -D -s conv-<id> -x <cols> -y <rows> -c ~/projects/<dir> <pane-cmd>` — `<pane-cmd>` is `bash -l`, or with an autostart set: `bash -lc '<command>\nexec bash -l'` (runs only when the session is CREATED; `-A` reattach never re-runs it)
  — every terminal is a login shell (ADR-014/016); `-A` attaches when the
  session exists, `-D` detaches stale clients. Panes are transcribed via
  pipe-pane to `~/.styrman/logs/<projectId>/` (ADR-017).
- The pane command exiting ends the session → close 4000.
- A browser disconnect never signals the shell; the session stays alive until
  killed via the API, the pane command exits, or the container stops.
- Container stop/idle-reap ends all tmux sessions. Agent history survives in
  the project's own home (`claude --resume`, agy's own history).
- Server keepalive: WebSocket ping every 30 s; a dead client is detected in
  ≤ 30 s instead of TCP-keepalive minutes.

## Transport internals (server side)

The server holds a Docker exec (TTY mode) into the project container —
`ContainerExecCreate` + `ContainerExecAttach` (hijacked raw stream, no
stdcopy demux in TTY mode) — and pumps bytes between it and the WebSocket.
Resize maps to `ContainerExecResize`. Exec stream EOF + a finished exec
inspect (`Running=false`) distinguishes "session exited" (→ 4000) from
attach teardown (→ normal close).
