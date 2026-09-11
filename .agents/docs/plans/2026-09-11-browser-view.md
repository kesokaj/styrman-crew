---
owner: main
date: 2026-09-11
---
# Plan — Browser view: an in-guest Chromium shared by the user and the agents   (2026-09-11)

## Why

Operator request, 2026-09-11: "add a browser in this app, where I can open
websites in the app … in the top bar where I can choose terminal/editor/browser.
The plan here is when I use claude or agy it can use this browser for testing
and I can try local sites directly. I know I already can expose sites with
share links etc. This is not a replacement for that."

Today a site running inside a project (a dev server on `localhost:3000`, a
container the project's own dockerd published) is reachable from the SPA only
through the port-preview proxy or a share link, and an agent inside the VM has
no browser at all: it cannot look at what it built. The feature gives every
project ONE real browser, running inside its microVM, that the operator drives
from a new "Browser" view and that `claude` / `agy` drive through an MCP
server pointed at the same browser. The operator watches what the agent does
and can take over at any time; `localhost` inside that browser IS the
project's localhost, so no proxy, share list or link is involved.

Decisions taken with the operator on 2026-09-11 (scope confirmation):
- Architecture: a real headless Chromium inside the guest, its DevTools
  protocol (CDP) screencast relayed to the SPA (chosen over an iframe through
  the preview proxy, which the agents could never use and which most public
  sites refuse, and over Xvfb+VNC, which is heavier).
- Agent access: an MCP server registered for BOTH agents at harness seeding.
- Cost accepted: ~400 MB more runtime image; a few hundred MB of RAM per
  project while its browser runs.
- R4 gets a one-line clarification and a new ADR records both that and the
  first long-running in-guest daemon since ADR-002 (see "Requirement edit"
  and "ADR" below). REQUIREMENTS.md is edited only after the operator
  approves the exact sentence in this plan.

## Scope

**In**
- Runtime image: Debian `chromium` (headless), a launcher script
  `/styrman/bin/styrman-browser` (`start|stop|status|mcp`), and the pinned
  npm package `chrome-devtools-mcp` as the MCP server the agents use.
- Harness seeding: register the MCP server named `styrman-browser` for
  `claude` (`claude mcp add --scope user`) and `agy` (`agy mcp add`) inside
  the existing version-stamped seeding block of `image/entrypoint.sh`.
- Server: three owner-only REST routes (`status`, `start`, `stop`) and one
  owner-only WebSocket relay `/ws/browser/{project}` between the SPA and the
  guest's CDP browser endpoint. Port 9222 is excluded from the port list and
  refused by the preview proxy and the link/share paths.
- SPA: a fourth main-pane view "Browser" (toggle pill next to Terminal /
  Editor / Diff) with URL bar, back / forward / reload, a tab strip, a canvas
  showing the screencast, mouse + keyboard + wheel input, and "Open in
  Browser" on every row of the Ports side panel.
- Lifecycle: the browser starts on demand (first Browser-pane connection, or
  the agent's MCP server starting) and lives until the project stops. Its
  profile persists in the project home (`~/.styrman/browser/profile`), so a
  login done in one session survives, per project (R3: nothing shared).
- Tests at every tier (see Verification) and the docs listed in Phase 4.

**Out**
- Any change to public/private share links and the preview proxy beyond
  refusing port 9222.
- A headed browser UI (Chrome's own chrome, extensions, DevTools panel,
  print, downloads, file-chooser dialogs, PDF viewer). JavaScript dialogs
  (`alert`/`confirm`/`prompt`) are ACCEPTED automatically with an empty
  prompt answer and shown as a one-line notice in the pane; nothing more.
- Clipboard integration between the page and the operator's machine, IME /
  dead-key composition, touch events, audio, WebRTC camera / microphone.
- Idle reaping of a running browser (it stops with the project); a
  per-project on/off switch; sharing the browser view with a shared guest.
- Multiple browsers per project, or one browser serving several projects.
- Making the browser reachable from a share link, a public link, or the
  preview proxy.

**Follow-ups noted, not built**
- A pipe transport (`--remote-debugging-pipe`) so that even a container run
  with `--network=host` inside the project cannot reach Chromium's loopback
  DevTools listener (security review round 3, accepted boundary for v1:
  host networking is the project user's own choice inside their own VM).
- A separate profile (or a target allowlist in the relay) so the agents and
  the operator do not share cookies (security review round 1, accepted per
  ADR-068 D1; the guide carries the warning).
- A sweep of stale link/share records for the browser ports (they 404 but
  still show in the owner's UI).
- A lockfile-based install of `chrome-devtools-mcp` (the resolved tree is
  recorded for diffing).
- A cap on consecutive harness re-seed attempts when a home keeps breaking
  its own registration (the WARN now says why).
- Idle stop of the browser after N minutes without a SPA or MCP client.
- A "follow the agent" mode that switches the visible tab to whatever tab
  the agent last activated (v1 shows the tab strip and lets the operator
  pick; new tabs appear in the strip as they are created).
- Screencast quality / fps knob in Settings.

## Surfaces touched

| Layer | Path | Change |
|---|---|---|
| image | `image/Dockerfile` | one RUN after the agents step and BEFORE the base-package snapshot: `apt-get install chromium fonts-liberation socat` (pinned to the Debian archive's current `152.0.7977.82-1~deb13u1`; Debian security updates move it, like `docker-ce`) + `npm install -g chrome-devtools-mcp@${CHROME_DEVTOOLS_MCP_VERSION}` (ARG, 1.9.0); `install` `configs/styrman-browser` to `/styrman/bin/styrman-browser` (0755) in the final RUN |
| image | `image/configs/styrman-browser` (new) | launcher: `start`, `stop`, `status`, `mcp` |
| image | `image/entrypoint.sh` | MCP registration for both agents inside the stamped harness block (third step, same timeout and success gating) |
| image | `image/README.md`, `.agents/docs/specs/styrman/runtime-image.md` | document the package, the launcher, the port, the MCP registration |
| server | `internal/config/config.go` | `BrowserPort = 9222`, `CloseBrowserUnavailable = 4004` (with the reconnect-behaviour comment the other codes carry: the pane does NOT auto-reconnect on 4004, it shows the reason and a Retry button) |
| server | `internal/orchestrator/browser.go` (new) + `browser_test.go` | `BrowserStart`, `BrowserStop`, `BrowserStatus` over `Exec` |
| server | `internal/orchestrator/ports.go` | drop `config.BrowserPort` from `ListeningPorts` output |
| server | `internal/httpapi/browser.go` (new) + `browser_test.go` | REST handlers, WS relay, `previewPortAllowed` guard |
| server | `internal/httpapi/proxy.go`, `shares.go`, `links_api.go` | refuse `config.BrowserPort` (400) in `proxyPort`, `setShares`, `createLink` |
| server | `internal/httpapi/server.go` | route registration |
| web | `web/src/lib/editorPane.ts` (+test) | `view` union gains `"browser"`; `browser: { url: string \| null }` field + `openInBrowser(project, term, url)`; the no-op guard in `set()` (`editorPane.ts:96-107`) gains `prev.browser === next.browser` — exactly the omission its own comment records for `diff`; `openInBrowser` always allocates a NEW `browser` object so a second port while already on the Browser view is never a no-op; a unit test opens two ports in a row and asserts two emits |
| web | `web/src/lib/cdp.ts` (new, +test) | minimal CDP client over a WebSocket |
| web | `web/src/lib/browserInput.ts` (new, +test) | pure key / mouse / wheel → CDP `Input.*` param mapping and coordinate scaling |
| web | `web/src/lib/api.ts` (+ `api.encoding.test.ts`) | `browserStatus`, `browserStart`, `browserStop`, `browserSocketUrl` |
| web | `web/src/components/BrowserPane.tsx` (new) | the view |
| web | `web/src/routes/p.$project.$term.tsx` | fourth pill, `browserUsed` latch, hidden-not-unmounted mount |
| web | `web/src/components/SidePanelPorts.tsx` | "Open in Browser" button per port row |
| tests | `.agents/docs/testing/styrman/integration.sh`, `web/e2e/run.mjs`, `.agents/docs/testing/styrman/README.md` | new checks and their counts |
| docs | `.agents/docs/specs/styrman/{guide,api,terminal-protocol}.md`, `REQUIREMENTS.md` (R4, one sentence, after approval), `.agents/docs/architecture/DECISIONS.md` (ADR-068), `ROADMAP.md` | |

Nothing in `deploy/`, the ingress, the firewall or the credential broker
changes. The runtime image must be rebuilt at the release version like any
image change (`versioning.md`, the three-image lineup).

## Requirement edit (needs the operator's explicit approval before Phase 1)

R4 today: "**R4 — Terminal-first, no embedded IDE**: the agent CLI is the
primary interface. No code-server / VS Code embed. A lightweight file viewer
is the ceiling."

Proposed R4, one sentence appended: "A Browser view showing the project's own
in-guest Chromium, shared with the agents for testing what they build, is a
testing surface and not an IDE (ADR-068, operator approval 2026-09-11)."

## ADR-068 (written in Phase 1, before code)

Title: "Browser view — one headless Chromium per project microVM, driven by
the operator through a CDP screencast and by the agents through an MCP
server". Records: (a) the R4 clarification above; (b) the first long-running
in-guest daemon since ADR-002 — Chromium is a TOOL the project user runs (as
that user, in that home), not a server the styrman server depends on: the
server still reaches the guest only through `docker exec` (to start / stop /
probe it) and through the project bridge (to relay CDP), exactly as the
port-preview proxy reaches a dev server; (c) CDP listens on the guest's
bridge address, port 9222, reachable by the styrman server and by the
project's own processes and inner containers (one trust domain), never
published, never proxied, never shareable; (d) the narrow ADR-062 exception:
the entrypoint registers ONE MCP server with each agent's own `mcp add`
command, writing the tools' own MCP registries — still no `settings.json`,
no hooks, no scripts for either tool; (e) accepted costs (image size, RAM).

## Interfaces

### Guest launcher — `/styrman/bin/styrman-browser` (bash, runs as the project user)

```
styrman-browser start    # idempotent; exit 0 when /json/version answers (≤ 15 s), else 1 with the last 20 log lines on stdout
styrman-browser stop     # kills the process group from ~/.styrman/browser/pid; exit 0 even if not running
styrman-browser status   # prints JSON {"running":true|false,"version":"Chrome/152…"|""} ; exit 0
styrman-browser mcp      # `start` (quiet) then `exec chrome-devtools-mcp --browserUrl http://127.0.0.1:9229`
```

- State dir `${HOME}/.styrman/browser/` (mode 0700): `profile/` (the
  Chromium user data dir), `pid`, `chromium.log`.
- Launch (one process GROUP under `setsid -f`, stdio to the log; the lock
  fd is closed in the child with `9>&-`, or Chromium would hold the start
  lock for life): `socat TCP-LISTEN:9222,fork,reuseaddr,bind=0.0.0.0
  TCP:127.0.0.1:9229 &` then `exec chromium --headless=new
  --remote-debugging-port=9229
  --user-data-dir=${HOME}/.styrman/browser/profile --window-size=1280,800
  --no-first-run --no-default-browser-check --disable-gpu
  --disable-dev-shm-usage about:blank`. Why the forwarder: new-headless
  Chromium IGNORES `--remote-debugging-address` and binds loopback only
  (verified 2026-09-11 with `ss` in the Kata guest), and the server reaches
  the guest over the bridge. `stop` signals the group, so both die together.
  Security review (2026-09-11, two rounds) narrowed it: socat runs in its
  own process group with `range=<peer>/32,max-children=4` where the peer is
  the server's own bridge address, passed EXPLICITLY as
  `STYRMAN_BROWSER_PEER` on every server-run `start`
  (`Orchestrator.BrowserPeer`: the server container's live address on that
  network, or the bridge gateway for a host-run dev server), else the host
  of `STYRMAN_CRED_BROKER`; IPv4 literal only; no peer → no forwarder
  (fail closed, the MCP verb still works on loopback 9229, and the pane's
  later `start` adds the forwarder to the running browser). The relay
  follows no redirect from the guest, checks Origin before any side effect
  and caps concurrent relays; stale link/share records for the browser
  ports 404.
  No `--no-sandbox`: probed 2026-09-11 inside a real Kata `mvm` container
  of `styrman-runtime:1.27.1` (guest kernel 6.18.28, user namespaces
  enabled) — the launch above, as an unprivileged user, answered
  `/json/version` within 15 s with Chromium's own sandbox intact. The flag
  is not added; if a future image or kernel breaks the sandbox the launcher
  fails loudly (the log tail reaches the pane) rather than silently
  weakening it.
- `start` is safe to call concurrently (the SPA relay and the MCP server may
  race): it takes `flock -w 20` on `${HOME}/.styrman/browser/lock` around
  the probe-then-launch (bounded, so a stuck holder is a failure, not a hang
  under the server's 30 s exec cap). A failed launch calls `stop` on its own
  group before returning 1.
- Readiness probe: `curl -fsS http://127.0.0.1:9222/json/version`.
- Chromium's DevTools HTTP server rejects a `Host` header that is neither
  `localhost` nor an IP literal (verified: `Host: example.test` → 500;
  `Host: 127.0.0.1:9222` → 200; through the forwarder from the host,
  `Host: <guest-ip>:9222` → 200 and the reported `webSocketDebuggerUrl`
  carries that guest IP; a raw WebSocket upgrade to it answered 101 —
  2026-09-11 Kata probe). Every caller uses an IP or `localhost`.

### MCP registration — `image/entrypoint.sh`, inside the stamped harness block

Third step after `harness-migrate.sh` and `sh-agent install`, as the user,
`timeout 20` each, both required for the stamp to be written:

```
claude mcp remove --scope user styrman-browser >/dev/null 2>&1 || true
claude mcp add --scope user styrman-browser -- /styrman/bin/styrman-browser mcp
agy mcp add styrman-browser /styrman/bin/styrman-browser mcp       # "add or update"
```

Gate: a third flag `HR_OK` beside `HM_OK` / `HI_OK`, graded as ONE unit
— `HR_OK=0` if EITHER `claude mcp add` or `agy mcp add` exits non-zero or
times out; the swallowed `claude mcp remove … || true` never counts (a
missing entry is the normal first-boot case). The stamp is written only when
`HM_OK && HI_OK && HR_OK`; a failure logs `WARN: MCP registration failed for
<claude|agy|both> — retried next boot`, so a half-registered home is never
stamped as done (the BUG-035 rule: the gate must name every condition it
guards). Both commands run with `HOME=${HOME_DIR}` under `timeout 20`.

Verified 2026-09-11 in a throwaway container of `styrman-runtime:1.27.1`
with a fresh, unauthenticated home: `claude mcp add --scope user` needs no
login and writes `~/.claude.json` (`mcpServers.styrman-browser`, type
stdio) and exits 1 on a duplicate — hence the `remove` first; `agy mcp add`
is "add or update" (exit 0 twice) and writes `~/.gemini/config/mcp_config.json`.
Neither path is in `configs/harness-migrate.sh`'s move list (it names
`.gemini/settings.json` and `.gemini/config/{skills,plugins/harness,global_workflows}`,
never `.gemini/config/mcp_config.json` or `.claude.json`) and neither is a
path `sh-agent install` manages (`image/harness/internal/harness/plan.go`:
rules, skills, agents, workflows under `.gemini/config`). Both facts are
re-asserted by the Phase 1 boot check.

### Server — `internal/orchestrator/browser.go`

```go
// BrowserStatus is what the guest launcher reports.
type BrowserStatus struct {
    Running bool   `json:"running"`
    Version string `json:"version,omitempty"`
}
// Each verb first calls o.Status(ctx, user, project) exactly as
// ListeningPorts does (ports.go:64-71): State != "running" → ErrNotRunning
// BEFORE any Exec. Exec errors and non-zero exits are NEVER classified by
// message — they surface as real errors (REST 502 / WS 1011), and, as in
// ListeningPorts, a container that vanished between Status and Exec is
// re-asked through a second Status and reported as ErrNotRunning only when
// that second look says so. Then /styrman/bin/styrman-browser <verb> runs as
// guestUser through Exec (30 s cap inherited).
func (o *Orchestrator) BrowserStart(ctx context.Context, user, project, guestUser string) (BrowserStatus, error)
func (o *Orchestrator) BrowserStop(ctx context.Context, user, project, guestUser string) error
func (o *Orchestrator) BrowserStatus(ctx context.Context, user, project, guestUser string) (BrowserStatus, error)
var ErrNotRunning = errors.New("project is not running")
```

`BrowserStart` returns an error whose message is the launcher's stdout (the
log tail) when the exit code is non-zero, wrapped as
`fmt.Errorf("browser did not start: %s", tail)` with the tail truncated to
2 KiB. `ListeningPorts` drops `config.BrowserPort` before returning.

### Server — `internal/httpapi/browser.go`

| Route | Auth | Response |
|---|---|---|
| `GET /api/projects/{project}/browser` | owner (`getProjectFor`) | 200 `{"running":bool,"version":string}`; 200 `{"running":false}` when the project is not running |
| `POST /api/projects/{project}/browser/start` | owner | 200 `{"running":true,"version":…}`; 409 `{"error":"project is not running"}`; 502 `{"error":"browser did not start: <tail>"}` |
| `POST /api/projects/{project}/browser/stop` | owner | 204; 409 when the project is not running |
| `GET /ws/browser/{project}` | owner (authGuard + `getProjectFor`, never `previewAccess`) | WebSocket relay, below |

Guest user: `sp.profile.GuestUser()` as in `explorer.go:629`.

WS relay (`coder/websocket`, the approved library):
1. BEFORE the upgrade, still as plain HTTP: `orch.EnsureSelfAttached`
   (as `serveProxy`). A browser never follows a redirect on a WebSocket
   handshake and the attach may doom this very connection, so a FRESH attach
   answers `503 Service Unavailable` with `Retry-After: 1` and no upgrade;
   the pane treats a failed handshake like any close and reconnects after
   1 s. The REST `start` handler answers the same 503 on a fresh attach (a
   `fetch` would follow a 307, but the response may never arrive, so the
   pane's one retry is the contract for both). There is NO 1012 or any
   other unnamed close code in this feature: every close the relay sends is
   a `config.Close*` constant or a standard 1000 / 1009 / 1011.
2. `orch.IPAddress`; "" (not running) → `websocket.Accept` then close
   `config.CloseProjectStopped` (4002) "project stopped" (the accept-then-
   close shape `bridge.go` uses for the stopped gate).
3. `BrowserStart` (idempotent) → `ErrNotRunning` → 4002 as above; any other
   error → accept then close `CloseBrowserUnavailable` (4004) with the error
   text as the reason (truncated to 120 bytes, the close-frame limit).
4. `GET http://<ip>:9222/json/version` (Host = `<ip>:9222`, an IP literal,
   accepted by Chromium), read `webSocketDebuggerUrl`, replace its host with
   `<ip>:9222`, dial it with `websocket.Dial` (no `Origin` header is sent —
   which is what admits it: there is NO `--remote-allow-origins`, so a page
   rendered inside the browser, which always sends an Origin, is refused a
   DevTools socket with 403 — verified 2026-09-11 under Kata; Phase 1
   review finding).
5. Pump text frames both ways until either side closes; close the other
   side with 1000. Read limit on the SPA side 1 MiB, on the guest side
   16 MiB (screencast frames arrive base64 inside JSON; a JPEG at
   1280×800 q70 is ~100–200 KiB).
6. No single-connection semantics: each SPA connection gets its own upstream
   CDP connection (Chromium multiplexes clients). No re-auth watch beyond
   what `authGuard` does at accept time; the terminal bridge's revocation
   watch is deliberately not duplicated (terminal-only by design, handoff
   2026-09-10).

Port 9222 refusals: `proxyPort`, `setShares`, `createLink` return 400
`{"error":"port 9222 is the project's browser and cannot be previewed or shared"}`
before any other work, via one helper `previewPortAllowed(port int) error`.

### Web — `web/src/lib/cdp.ts`

```ts
export type CdpEvent = { method: string; params: unknown; sessionId?: string };
export class CdpClient {
  constructor(ws: WebSocket);
  send<T = unknown>(method: string, params?: object, sessionId?: string): Promise<T>; // rejects with CdpError {code, message} on a protocol error
  on(method: string, fn: (params: any, sessionId?: string) => void): () => void;      // returns unsubscribe
  close(): void;
}
```
Ids are a monotonic counter; a reply with an unknown id is ignored; the
`close` rejects every pending call with `CdpError("closed")`.

### Web — `web/src/lib/browserInput.ts` (pure, tested)

```ts
export function mouseParams(e: {type:"mousedown"|"mouseup"|"mousemove"; button:number; clientX:number; clientY:number; detail:number; modifiers}, rect: DOMRectLike, frame: {deviceWidth:number; deviceHeight:number}): InputDispatchMouseEvent
export function wheelParams(e: {deltaX:number; deltaY:number; clientX; clientY}, rect, frame): InputDispatchMouseEvent // type "mouseWheel"
export function keyParams(e: {type:"keydown"|"keyup"; key:string; code:string; keyCode:number; modifiers}): InputDispatchKeyEvent // text set for printable keys on keydown; "rawKeyDown" is not used
export function modifiersOf(e: {altKey; ctrlKey; metaKey; shiftKey}): number // CDP bitmask 1 alt, 2 ctrl, 4 meta, 8 shift
```
Coordinates: `x = (clientX - rect.left) * frame.deviceWidth / rect.width`,
same for y, rounded.

### Web — `BrowserPane` behaviour

- Props `{ project, term, onShowTerminal }` like `DiffPane`. Keyed
  `${project}/${term}` like the others.
- Connects on first mount to `browserSocketUrl(project)`; shows "Starting
  the browser…" until the first `Target.getTargets` reply; on close 4002
  shows "Project is stopped" with a "Show terminal" link (the terminal boots
  it); on 4004 shows the reason text and a "Retry" button; on any other
  close shows "Browser disconnected" and "Reconnect".
- After connect: `Target.setDiscoverTargets {discover:true}`; tab strip =
  targets with `type === "page"`, ordered by creation; the active tab is the
  one the pane last attached to (initially the first page, or a new one via
  `Target.createTarget {url:"about:blank"}` if none). Attach with
  `Target.attachToTarget {targetId, flatten:true}` → `sessionId`; then
  `Page.enable`, `Runtime.enable` are NOT needed; `Emulation.setDeviceMetricsOverride
  {width, height, deviceScaleFactor:1, mobile:false}` sized to the canvas
  box (re-sent on ResizeObserver, debounced 150 ms), `Page.startScreencast
  {format:"jpeg", quality:70, maxWidth:width, maxHeight:height, everyNthFrame:1}`.
- `Page.screencastFrame` → decode `data:image/jpeg;base64,…` into an
  `Image`, draw on the canvas (canvas backing size = frame `deviceWidth ×
  deviceHeight`), then `Page.screencastFrameAck {sessionId}`. Frames that
  arrive while one is still decoding are dropped, keeping only the newest
  (latency over completeness).
- URL bar: shows `Target.targetInfoChanged`'s `url` for the active tab;
  Enter → `Page.navigate {url}` where a bare `host[:port]/path` gets `http://`
  and a `localhost:<n>` typed as `:<n>` becomes `http://localhost:<n>/`.
  Back / forward: `Page.getNavigationHistory` → `Page.navigateToHistoryEntry`.
  Reload: `Page.reload`. New tab: `Target.createTarget`. Close tab:
  `Target.closeTarget`. Switching tabs: detach (`Page.stopScreencast`,
  `Target.detachFromTarget`) then attach the other.
- Input: the canvas is focusable (`tabIndex=0`); mouse / wheel / key events
  are mapped by `browserInput.ts` and sent to the active session. Browser
  shortcuts the SPA must keep (Ctrl/Cmd+L focuses the URL bar; Ctrl/Cmd+R
  reloads) are handled in the pane and not forwarded.
- `Page.javascriptDialogOpening` → `Page.handleJavaScriptDialog {accept:true}`
  and a 5-second notice line with the message.
- "Open in Browser" (Ports panel) → `openInBrowser(project, term,
  "http://localhost:<port>/")`: sets `view: "browser"` and `browser.url`; the
  pane navigates the active tab to it once connected and clears the field.
- Hidden-not-unmounted like the editor; the WebSocket stays open while the
  view is hidden (a screencast keeps flowing — acceptable for v1, noted as a
  follow-up: `Page.stopScreencast` while hidden).

### Web — `api.ts`

```ts
browserStatus: (project) => request<{running:boolean; version?:string}>(u`/api/projects/${project}/browser`)
browserStart:  (project) => request<{running:boolean; version?:string}>(u`/api/projects/${project}/browser/start`, {method:"POST"})
browserStop:   (project) => request<void>(u`/api/projects/${project}/browser/stop`, {method:"POST"})
browserSocketUrl(project): string  // `${wss|ws}://${location.host}` + u`/ws/browser/${project}`
```

## Failure modes

| Failure | What the operator sees | What the API does |
|---|---|---|
| Project not running | pane: "Project is stopped — Show terminal" | REST 409 / WS close 4002 |
| Chromium fails to launch (missing lib, sandbox refusal, profile lock) | pane: the launcher's log tail + Retry | REST 502 with the tail; WS close 4004 with the tail's first 120 bytes; server log WARN with the full tail |
| Server not yet attached to the project bridge (first browser use after a boot) | nothing visible: the pane's handshake fails once and it reconnects after 1 s | `EnsureSelfAttached` fresh → plain-HTTP 503 + `Retry-After: 1` before any upgrade (REST `start`: the same 503); the pane retries once after 1 s |
| Upstream CDP socket drops (Chromium crashed, project stopped) | "Browser disconnected — Reconnect" | relay closes the SPA side 1011 with the upstream's reason |
| A shared guest (share-list email) calls any browser route | 403 as for any non-owner project route | `getProjectFor` fails |
| Someone shares / links / previews port 9222 | Ports panel never lists it; a hand-typed URL gets a 400 with the message above | `previewPortAllowed` |
| MCP registration fails at boot | boot continues; `styrman-browser` absent from `claude mcp list` / `agy mcp list`; entrypoint WARN line; the stamp is NOT written so the next boot retries | n/a |
| Agent starts the MCP server while the SPA is connected (or vice versa) | both see the same browser; a tab the agent opens appears in the strip | `flock` in `start` |
| Screencast frame larger than the read limit | never at 1280×800 q70; a frame over 16 MiB closes the relay 1009 and the pane reconnects | |

## Verification

Tiers: unit (Go `-race`, vitest) · integration (Tier 2, real Kata) · e2e
(Tier 4, Playwright against the self-hosted server) · manual (the agent
path, operator).

What "works" means per tier:
- **Unit**: `browser_test.go` (orchestrator) proves the three verbs run the
  launcher as the guest user and map exit codes through the fake Docker
  client; `browser_test.go` (httpapi) proves the relay pumps frames both ways
  against an `httptest` fake CDP server, and the four close codes; `cdp.test.ts`
  proves id correlation, event fan-out, error rejection and close-rejects-all;
  `browserInput.test.ts` proves coordinate scaling and the key/mouse/wheel
  mappings; `editorPane.test.ts` proves `openInBrowser` sets the view and url;
  `api.encoding.test.ts` covers the four new call sites.
- **Integration** (`integration.sh`, +5 checks — three in Phase 1, two in
  Phase 2): in-guest `styrman-browser start` answers `/json/version` with
  no `--no-sandbox` in the live cmdline; the MCP entry is present in both
  tools' registries (read from the files, not `mcp list`, whose health
  probe would spawn the server); `stop` leaves `status` not running; then,
  once the server knows the port (Phase 2): `GET /api/projects/{id}/ports`
  omits 9222 while it listens and `GET /proxy/{id}/9222/` is a 400.
- **E2E** (`run.mjs`, +4 checks): "Browser pill opens the browser view and a
  frame arrives" (`[data-frames]` on the canvas becomes > 0 within 20 s);
  "typing a localhost URL renders the fixture app" (the run already serves a
  fixture app on a port inside the project — navigate to it, the URL bar
  reads it, the tab title is the fixture's); "Open in Browser from the Ports
  panel switches the view and navigates"; "the terminal session survived the
  browser round-trip" (the existing survival check, repeated for this view).
- **Manual (operator)**: in a project terminal run `claude`, ask it to open
  `http://localhost:<port>` and describe the page; watch the tab appear in
  the Browser view. Same with `agy`. Logged in `testing/`.

Host-firewall caveat (learned from the Phase 1 integration run, 2026-09-11):
on this hardened host the three preview checks auto-SKIP because a server
running ON THE HOST cannot reach a sandbox project's `sdb-*` bridge — the
firewall drops guest→host replies. The browser relay has the same shape, so
the Phase 4 e2e browser checks run INSIDE the existing end-of-run LAN window
(after "enabling flips the record and shows the LAN badge", before
"disabling returns it to sandbox"), where the fixture project's `lan-*`
bridge is matched by no rule; Phase 2's manual curl/WS test uses a LAN
project or the containerized deploy the same way. Production is unaffected:
the compose server sits on the bridge itself.

E2E criteria — golden path: open a project, click Browser, see the page,
type a URL of a site served inside the project, see it render, click a link
in it, see the URL bar follow. Edge case: stop the project from the sidebar
while the Browser view is open — the pane shows "Browser disconnected" the
moment the guest's Chromium goes (the relay closes 1011); Reconnect on the
stopped project shows "Project is stopped" with Show terminal and
Reconnect; after the project is started again, Reconnect brings the browser
back with a fresh tab. (Observed 2026-09-11, twice; the stop itself takes
≈48 s on this host with or without a browser running — pre-existing, not
this feature's.)

## Phases

### Phase 1 — Guest image: Chromium, launcher, MCP registration, ADR
- [x] Operator approves the exact R4 sentence above; edit `REQUIREMENTS.md` R4 and write ADR-068 in `DECISIONS.md` in ONE commit with `ROADMAP.md` "Now" — verify: the diff touches those three files only; who: user (approval) + AI — DONE 2026-09-11, commit `a9ae789` (approval: "approve the R4 sentence, go ahead")
- [x] `image/Dockerfile`: `chromium fonts-liberation socat` + `chrome-devtools-mcp@1.9.0` in one RUN before the base snapshot, launcher installed — verified 2026-09-11: build green; 3.56 GB → 4.32 GB (+760 MB, more than the ≈400 MB estimate: chromium's own dependency set); `chromium --version` = 152.0.7977.82; `chrome-devtools-mcp --help` lists `--browserUrl`; who: AI
- [x] `image/configs/styrman-browser` with `start|stop|status|mcp`, `flock -w 20`, pid, log, profile dir 0700, socat forwarder — verified 2026-09-11 (shellcheck clean; under Kata as an unprivileged user: `start` 0, `status` running with version, second `start` returns at once, `stop` closes 9222 and 9229 with no live chromium/socat left; under plain runc the launch fails LOUDLY with "No usable sandbox" and cleans its group — expected, runc is not the target). Two defects found and fixed on the way: the lock fd was inherited by chromium (second start hung forever → `9>&-` + bounded flock), and `stop` skipped the group kill when the leader had died; who: AI
- [x] Re-run the sandbox probe with the BUILT image and the real launcher under `docker run --runtime mvm` — verified 2026-09-11: running, `--no-sandbox` absent from the live cmdline; from the host over the bridge: `/json/version` 200 with `Host: <ip>:9222`, 500 with `Host: example.test`, a raw WebSocket upgrade to the reported `webSocketDebuggerUrl` answered 101; who: AI
- [x] `image/entrypoint.sh`: the third seeding step (`HR_OK`, graded as one unit) — verified 2026-09-11 by the integration run: both registries carry `styrman-browser` in a fresh project home; boot→ready 2.75 s with the registration inside (soft target 5 s); `harness-migrate.sh` and `sh-agent install` touch neither file (read); who: AI
- [x] `integration.sh`: the three guest-side checks; README counts updated — verified 2026-09-11: `passed=87 failed=0 skipped=3` (was 84/0/3), log `testing/2026-09-11-verify-browser-phase1.md`; who: AI
- [x] `runtime-image.md` ("Browser" section), `image/README.md`: package, launcher, ports and forwarder, registration paths, sandbox outcome — written by main 2026-09-11 alongside the code (the `documentation` agent is dispatched over the whole feature in Phase 4); who: AI
- [ ] Phase checkpoint: `changes/` entry, commit, push — who: AI

### Phase 2 — Server: verbs over the guest seam, REST, WS relay, port refusals
- [x] `internal/config`: `BrowserPort`, `BrowserChromePort` (9229, Chromium's loopback listener), `CloseBrowserUnavailable = 4004`, documented beside the other codes and in `terminal-protocol.md` — `go vet` clean 2026-09-11; who: AI
- [x] The three verbs — placed in `internal/httpapi/browser.go` over the existing `guestRuntime` seam (Status first, then Exec of the launcher as the guest user) rather than in the orchestrator: the seam is what makes them testable without Docker (`fakeGuest` scripts the launcher's replies; the orchestrator's fake Docker client cannot produce exec output). `errProjectNotRunning`, the log-tail error, and the vanished-container re-ask all covered — `go test -race ./internal/httpapi/` green 2026-09-11; who: AI
- [x] `ListeningPorts` drops 9222 and 9229: parsing extracted into the pure `parseListeningPorts`, unit-tested with a scripted scan (`ports_browser_test.go`); three server-side integration checks (ports list omits both while the browser listens; `/proxy/{id}/9222/` and the share PUT are 400; status/start/stop round-trip for the owner) — `passed=90 failed=0 skipped=3` 2026-09-11, README count 93; who: AI
- [x] `internal/httpapi/browser.go`: three REST handlers + the relay; `previewPortAllowed` in `proxyPort` and in `sharePort` (which `getShares`, `setShares` and `createLink` all parse through); routes registered — `browser_test.go` against an `httptest` fake DevTools server: frames both ways, the unsolicited event, 503 + Retry-After on a fresh attach for REST and WS with no exec attempted, 4002 / 4004 (reason carries the launcher's tail) / 1011 closes, 404 for a project the caller does not own (the same answer every owner-only route gives — the plan's earlier "403" was the wrong word), 400 for 9222 AND 9229 on all five refusing routes, and the fake refuses any Origin exactly as Chromium does. `go test -race` green 2026-09-11; who: AI
- [x] `api.md` ("Browser (ADR-068)" section) and `terminal-protocol.md` ("Browser relay" section, the 4004 row) written by main 2026-09-11; the `documentation` agent runs over the whole feature in Phase 4; who: AI
- [x] Live REST path: covered by the integration check "browser status/start/stop routes answer for the owner" against the self-hosted server and a real Kata project (start → `running:true`, status → `Chrome/…`, stop → 204, status → `running:false`). The live RELAY path cannot be driven from a host-run server on this firewall-hardened host (it cannot reach a sandbox bridge — the same reason the three preview checks auto-SKIP); its pieces are proven separately: the Go relay against the fake upstream, and a raw WebSocket upgrade to the guest's DevTools socket over the bridge (101) in the Phase 1/2 Kata probes. The end-to-end relay is exercised in Phase 4's e2e inside the LAN window; who: AI
- [x] Phase checkpoint — committed 2026-09-11 (change entry `changes/2026-09-11-browser-phase2-server-and-review-fixes.md`, ADR-068 amendment); who: AI

### Phase 3 — SPA: CDP client, BrowserPane, toggle, Ports button
- [x] `editorPane.ts`: `"browser"` view, `browser: BrowserRequest | null`, `openInBrowser` / `clearBrowserRequest`, the guard's `browser` comparison, `openFileInPane` carrying it through; tests: same-URL twice yields a new request object, file/draft kept, clear is idempotent — `npm run check && npm test` green 2026-09-11; who: AI
- [x] `cdp.ts` (`CdpClient` over a `CdpTransport` interface, `CdpError`, `CDP_CLOSED`) + 6 tests: id correlation out of order, protocol error, event fan-out with sessionId and unsubscribe, malformed/binary/unknown frames dropped, close rejects every pending call and refuses new ones, close reports once — green 2026-09-11; who: AI
- [x] `browserInput.ts` (`pointOf`, `modifiersOf`, `mouseParams`, `wheelParams`, `keyParams`, `normalizeUrl`) + 11 tests — green 2026-09-11 (one defect caught by its own test: `localhost:8080/x` read as a scheme; the scheme test now needs `://` or a known non-hierarchical scheme); who: AI
- [x] `api.ts`: `browserStatus`/`browserStart`/`browserStop`/`browserSocketUrl` + encoding tests (three call sites in the hostile-value table, a `browserSocketUrl` describe) — green 2026-09-11; who: AI
- [x] `BrowserPane.tsx` — verified 2026-09-11 by a Playwright walk of the built SPA against a local server and a real Kata project in LAN mode (`testing/2026-09-11-verify-browser-phase3.md`): first frame within seconds; typed `localhost:4100` navigates and the URL bar follows; the tab strip shows the page's title (after a `Target.getTargets` refresh on load — Chromium's `targetInfoChanged` carries the URL as an interim title); a click inside the page navigates; Back returns; Ctrl+L focuses the address bar; New tab works; the terminal survives; Ports → browser switches the view and navigates. Three defects found by the walk and fixed: the pane's automatic handshake retry was unbounded (now one per chain); the init failure handler overwrote a 4002 "stopped" with "disconnected" a microtask later (now defers to the close handler); the stopped state offered no way back in once the project ran again (now Show terminal + Reconnect); who: AI
- [x] `p.$project.$term.tsx`: the fourth pill, `browserUsed` latch, hidden-not-unmounted mount — the walk's "terminal still mounted" step passed; the existing e2e toggle checks run in Phase 4; who: AI
- [x] `SidePanelPorts.tsx`: a "browser" button per port row — the walk's step 9 passed; who: AI
- [x] Phase checkpoint — committed 2026-09-11 (change entry `changes/2026-09-11-browser-phase3-spa-and-security-round2.md`; integration `passed=90 failed=0 skipped=3` on the corrected checks); who: AI

### Phase 4 — E2E, docs, done
- [x] `run.mjs`: the four checks inside the LAN window; README Tier 4 count 87→91 — `bash web/e2e/run.sh` solo: `passed=91 failed=0 skipped=2`, client-error gate green (2026-09-11; two earlier runs failed for a load blip, a `!` history expansion in the typed fixture, and BUG-039's 500 — all recorded in `testing/2026-09-11-verify-browser-phase4.md`); who: AI
- [x] `guide.md` (§1, §2 "Browser view", §4 "Using the browser" with the shared-profile warning verbatim), `README.md`, `ROADMAP.md` — `documentation` agent 2026-09-11, text read back against the plan (no unmeasured claims); `terminal-protocol.md` was written in Phase 2; who: AI
- [x] Definition of Done: `verifier` (Phases 1–2 and 1–3 logs, PASS; over the Phase 4 commit right after it), `reviewer` and `security` per phase (three rounds, every finding fixed or rebutted in the change entries; the Phase 4 commit gets its own pass), `changelog` per phase, `bugs` BUG-039/040 — who: AI
- [ ] Manual agent walk by the operator (above), logged in `testing/` — who: user
- [x] Release: v1.28.0, MINOR, together with styrman-ctl — the operator said "ship it" 2026-09-11; gate `reviews/2026-09-11-preflight-v1.28.0.md`; who: user + AI

## Risks

- **Chromium under Kata**: probed green with the sandbox on (above); the
  residual is a future kernel or package change, caught by the Phase 1 boot
  check and the launcher's loud failure.
- **RAM and CPU**: measured 2026-09-11 in the Kata probe — a fresh headless
  Chromium on `about:blank` raised the guest's used memory by ≈250 MB; a
  heavy page costs more. Project VMs are not held to Kata's 2048 MB base:
  `default_maxmemory = 0` in `/opt/kata/share/defaults/kata-containers/configuration-qemu.toml`
  lets the VM grow, and a live production project reports the host's full
  memory. Ten projects with browsers open fit the NUC's 62 GB. No idle stop
  in v1 (follow-up).
- **Image size and build time**: +≈400 MB (3.56 GB → ≈4 GB), one more apt
  step; the auto-updater builds it at every release.
- **The 9222 listener and the project's inner containers** — CLOSED by the
  Phase 1 security review (2026-09-11): a container the user merely runs
  would have read the home through `file://`. The forwarder now admits one
  peer, the server's bridge address (`range=`), with `max-children=4`;
  in-guest clients use Chromium's loopback port; `--remote-allow-origins`
  is gone so no page inside the browser can open the socket. ADR-068 is
  amended accordingly.
- **Keyboard fidelity**: layouts, dead keys and IME are out of scope; the
  mapping is what CDP's `Input.dispatchKeyEvent` supports from a DOM
  `KeyboardEvent`. Good enough for testing apps, not for writing prose.
- **Screencast bandwidth**: ~1–2 MB/s at 1280×800 while the page animates;
  fine on the LAN, and the pane drops stale frames rather than queueing.
- **R4 wording**: the sentence is short and names the ADR; the operator's
  approval is a task, not assumed.
- **A stale `dist`**: the e2e runner rebuilds the SPA first (documented in
  AGENTS.md); Phase 4 relies on that.
