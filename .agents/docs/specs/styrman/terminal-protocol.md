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
Before the upgrade, the server also joins its own container to the project's
bridge (`EnsureSelfAttached`, via `wsBoot` in `internal/httpapi/server.go`) so
the login shell the terminal spawns finds the credential broker reachable;
the join is memoized for 60s, so reconnects do not pay it. No supported
topology can see a fresh join disturb this handshake: loopback dev is
host-mode with no attach at all, and the compose deploy publishes no server
port (Caddy reaches it over the ingress bridge); the client's reconnect loop
would retry it anyway (BUG-060, ADR-071; the caveat ADR-071 states for
ADR-021's published-port dev topology describes no deployment this project
ships).

## Frames

| Direction | Frame | Meaning |
|---|---|---|
| server → client | binary | Raw PTY bytes. Write straight into xterm.js. |
| client → server | binary | Raw input bytes (keystrokes, paste). |
| client → server | text (JSON) | Control: `{"type":"resize","cols":N,"rows":N}`. |
| client → server | text (JSON) | Control: `{"type":"ping"}` (ADR-079). |
| server → client | text (JSON) | `{"type":"pong"}`, the answer to a ping, written under a 5 s bound. |

A ping does not count as activity for the idle reaper. Both `/ws/term/...`
and `/ws/setup` answer it. There are no other text messages; unknown control
types are ignored.

The client intercepts Ctrl/Cmd+V / Ctrl+Shift+V paste via the clipboard API,
and Cmd+C or Ctrl+Shift+C to copy a selection explicitly. Plain Ctrl+C is
handled specially — see Browser-side keyboard handling below.

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
  default can intercept it; any lingering selection is cleared. Plain Ctrl+C
  is reserved for SIGINT; Cmd+C (`metaKey`) or Ctrl+Shift+C still copy the
  selection explicitly (v1.8.3, hardened v1.8.6).
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

## Setup terminal (`/ws/setup`)

```
GET /ws/setup[?tool=git|ssh|gh|gcloud|claude|agy]
```

The user's hidden onboarding VM terminal — same wire protocol as above.
`tool` is optional and STRICTLY allowlisted (any other value is ignored,
falling back to the full `styrman-setup` flow), because it's interpolated
into `bash -lc 'styrman-setup <tool>'`; a targeted run gets its own tmux
session (`styrman-setup-<tool>`) so signing into one tool never replaces
another that's already running. This is what Settings → Credentials' "add a
credential" flow uses (guide.md) (v1.10.0).

Ctrl+C in this pane is a CLEAN exit (v1.10.0): `styrman-setup` traps SIGINT
and calls `exit 0` instead of dying by signal. Previously a signal-kill left
the tmux session in a state the browser terminal's reconnect (`tmux
new-session -A`) picked back up, which RESTARTED `styrman-setup` from the
top. Now the pane simply ends → close **4000**, exactly like the script
finishing on its own — the frontend finalizes or harvests (api.md's Setup
section) instead of silently reconnecting into a restarted script.

A setup pane additionally NEVER auto-reconnects once it has been open
(v1.10.1). The bridge only answers 4000 when dockerd registers the exec's
exit within its 1 s poll; a finished script can therefore still surface as
a TRANSIENT close, and a reconnect's `tmux new-session -A` would recreate
the ended session — restarting the script despite the trap. So the client
treats ANY post-open close of a setup pane as exited (→ `onExited`, which
is status-aware); only pre-open failures retry (the VM may still be coming
up). Ordinary project terminals keep the normal transient-reconnect
behavior — their panes idle at a shell, so reattaching is always safe.

## Close codes

| Code | Name | Client behavior |
|---|---|---|
| 4000 | shell exited | The tmux session genuinely ended (user typed `exit`, agent quit). Do NOT silently reconnect — the UI presents this state as **Paused** with a **Resume** action (the record and transcript persist; reconnecting starts a fresh shell). |
| 4001 | replaced | A newer WebSocket attached to this conversation (another tab). The pane never reconnects BY ITSELF — an automatic reconnect death-spirals: each reconnect replaces the other tab, which reconnects, forever. The pane shows "Opened in another tab" with a **Use here** button; the button, or the tab becoming visible again (hidden → visible), takes the terminal back in place, without boot intent. Being replaced triggers neither, so two pages cannot trade the terminal in a loop (see "Page liveness and wake" below). |
| 4002 | project stopped | The project VM was deliberately stopped (Stop button / idle reaper) and this connect carried no boot intent. NEVER auto-reconnect — a reconnect with `?boot=1` would boot the VM back up. The UI shows **Project stopped** with a **Start** action; that click (epoch bump → first connect → `?boot=1`) is the explicit intent that boots it (v1.11.0). |
| 1000/1001/1006 | normal/going away/abnormal | Transient. Silently reconnect; tmux reattaches with scrollback and running processes intact. |
| 4004 | browser unavailable | **Browser relay only** (`/ws/browser/{project}`, below): the project's Chromium could not be started or its DevTools endpoint could not be reached; the close reason carries the END of the launcher's log tail (cut to the 120-byte frame budget on a rune boundary, prefixed `"…"` — the "browser did not start: " head is dropped since the pane's own heading already says so). Do NOT auto-reconnect — a blind retry re-runs a launch that just failed; the pane shows the reason and a **Retry** button, which calls `POST /api/projects/{id}/browser/start` FIRST (its 502 body is the launcher's full 2 KiB log tail, shown in full) before reconnecting the relay — three guest execs total, intended: Retry only runs after an explicit click on a failed launch. |
| 4003 | access revoked | **Browser relay only**: the same allowlist re-check the bridge already runs, on the same 60 s interval, applied to the relay too — the caller was removed from the allowlist while the pane was open. Do NOT auto-reconnect; Reconnect fails the handshake (403) as today. |
| 1009 | frame too large | **Browser relay only**: a guest CDP frame (screencast or otherwise) over the 16 MiB read limit. The pane treats it exactly like a disconnect — phase `disconnected` with a **Reconnect** action. |

**Boot intent (`?boot=1`, v1.11.0):** the FIRST connect of a freshly opened
or explicitly resumed pane carries `?boot=1` — a user action that may boot a
stopped project. Automatic reconnect retries never carry it; on a
deliberately-stopped project the server answers them with close 4002 (before
any VM boot) instead of resurrecting the VM. This is what makes the sidebar
Stop stick while terminal tabs are open. Setup panes ignore `boot` (the
setup VM has no stop marker).

## Page liveness and wake (project terminals, ADR-079)

The page side of the `ping`/`pong` control frames above
(`web/src/components/TerminalPane.tsx`, `web/src/lib/termLiveness.ts`).
Project terminals only: a setup pane (`/ws/setup`) takes none of it and never
reconnects once it has been open.

- **Probe.** On socket open the page sends one ping; the pong marks this
  connection as one that answers pings. A probe is a ping with a 5 s
  deadline for ANY frame to arrive. It is sent when the tab becomes visible,
  when the browser reports the network back (`online`), and every 25 s while
  the tab is visible.
- **Unanswered probe.** The socket is dropped and reconnected at once, in
  place (same xterm, scrollback kept), with the retry counter reset and
  WITHOUT boot intent — a stopped project answers 4002 and stays stopped. A
  connection that never answered a ping is never declared dead, so a server
  without the pong reply behaves as before.
- **Wake events.** The same two events (tab shown, network back) also
  reconnect a pane that is "Disconnected" (its 8 retries used up, or a
  non-transient close) or waiting out a reconnect backoff. The 25 s interval
  never does.
- **Typed-while-down queue.** Keystrokes typed while a pane is reconnecting
  are queued (at most 256 entries) and sent when the reconnect lands. A pane
  that had GIVEN UP ("Disconnected") and is brought back by a wake event
  (tab shown, network back) drops that queue instead of sending it: the wake
  may come hours later, and replaying stale keys into a live shell is unsafe
  (security review 2026-10-02). A reconnect from a state that had not given
  up (a dead socket just noticed by the liveness probe, a backoff cut short
  by a wake) keeps the queue — unless its OLDEST entry is more than 30 s old
  (wall clock, so a machine that slept inside the retry window counts the
  sleep), in which case the whole queue is dropped, never trimmed: half a
  command line is worse than none (`web/src/lib/termOutbox.ts`).
- **4001.** Only the **Use here** button and a hidden → visible transition
  take the terminal back; `online` and the interval do not. With two pages
  both visible, the second holds the terminal and the first shows the button.
- 4000, 4002 and 4003 are unchanged.
- **The browser pane** (`/ws/browser/...`) has the same dead-socket case and
  the same three triggers (tab shown, network back, every 25 s while the
  document is visible, whether or not the Browser view is the one selected).
  Its probe is a CDP call through the relay (`Browser.getVersion`); ANY frame
  arriving within 5 s — the reply, an error, a screencast frame — counts as
  alive, and nothing at all reconnects the pane through its Reconnect path. A
  relay socket that CLOSED (4002, 4003, 4004, any code) never reconnects by
  itself: its state and button are unchanged.

## Browser relay (`/ws/browser/{project}`, ADR-068)

Not a terminal: the Browser pane speaks the Chrome DevTools Protocol (CDP)
to the project's in-guest Chromium, and this endpoint is a dumb relay
between the pane's WebSocket and the guest's browser-level DevTools socket
(`ws://<guest-ip>:9222/devtools/browser/<id>`, discovered from
`/json/version` and re-hosted to the guest IP we dial — the guest's own
answer is never trusted for WHERE to connect). Text frames both ways, no
protocol knowledge, no resize/control messages. Owner only: `authGuard`
resolves the identity and `getProjectFor` is the ownership check;
`previewAccess` is never consulted, so a share-list guest cannot reach it.

Order on connect (`internal/httpapi/browser.go`, `browserSocket`):
everything that can fail as plain HTTP fails BEFORE the upgrade — a browser
follows no redirect and reads no body on a WebSocket handshake, so a failed
handshake is what the pane retries. 404 (not the owner's project), 502
(guest IP lookup failed), **503 + `Retry-After: 1`** (this server has just
attached to the project bridge — the netns was reconfigured and this very
connection may be doomed; the pane reconnects after 1 s). Then, accepted
and closed at once: **4002** when the project is not running (no guest IP,
or the launcher's Status says so); **4004** when `styrman-browser start`
fails (reason = the END of the launcher's tail, cut on a rune boundary and
prefixed "…") or the DevTools endpoint cannot be discovered or dialed —
including when the server cannot learn its own bridge address to hand the
launcher as the forwarder's peer (`socat range=<peer>/32`): "no usable peer
address for :9222 (`STYRMAN_BROWSER_PEER` is set but not an IPv4 address)",
the same failure `POST .../browser/start` reports as 502 — fail closed
rather than starting a browser the pane can never reach. Once accepted, a
revocation watch (the same allowlist check the bridge already runs, every
60 s) closes the pane **4003 "access revoked"**. Then the pump: the pane
closing closes the guest side 1000; the guest side dropping (Chromium
crashed, project stopped) closes the pane **1011 "browser disconnected"**;
a guest frame over 16 MiB closes the pane **1009 "frame too large"** (the
pane treats it as a disconnect, Reconnect only — no partial frame is ever
delivered, since the relay copies one WebSocket message at a time and
abandons the pump on any read error). Read limits: 16 MiB from the guest (a
screencast frame is a base64 JPEG inside JSON, ~100–200 KiB at 1280×800),
1 MiB from the pane.
The upstream dial sends NO Origin header, which is what Chromium's DevTools
server admits — it refuses every Origin it sees, so a page inside the
browser can never open this socket (the launcher sets no
`--remote-allow-origins`).

Each pane connection gets its own upstream socket (Chromium multiplexes
clients); there is no 4001 replace semantics, but the relay DOES carry a
revocation watch (4003, above) — the same allowlist check the terminal
bridge already runs, on the same 60 s interval, so a removed user's open
Browser pane closes within the same window as their open terminals.

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
