---
owner: explorer
date: 2026-09-11
---
# Recon — Browser view (Terminal/Editor/Browser tabs, in-guest CDP)   (2026-09-11)

## Question
Survey the code needed to plan a new "Browser" view next to Terminal/Editor
that (a) lets a user open sites (incl. localhost:port in the guest) and (b)
lets in-guest AI CLIs drive the same browser via CDP.

## Summary
The existing view-switch pattern (`web/src/routes/p.$project.$term.tsx`) mounts each
pane lazily then keeps it alive with CSS `hidden`, never unmount — a third
"Browser" tab following the same `editorUsed`/`diffUsed` ref-latch pattern is the
natural fit. There is **already a full port-preview reverse proxy** with WS-upgrade
passthrough, header stripping and CSP sandboxing (`internal/httpapi/proxy.go`,
`shares.go`) that could iframe an in-guest HTTP server today. There is **no
existing CDP/Chromium/screencast/noVNC machinery anywhere** in the repo, no
in-guest agent/HTTP daemon (ADR-002 deliberately forbids one — server talks to
guests only via `docker exec`), and the runtime image (`image/Dockerfile`) ships
no browser. R4/ADR-005 ("terminal-first, no embedded IDE... file viewer is the
ceiling") is a live constraint a CDP-driven browser UI must be reconciled with
before design.

## Relevant files

1. **web/ view switching**
   - `web/src/routes/p.$project.$term.tsx:28-30,40-58,109-171,179-219` — per-terminal
     pane state via `useEditorPane` (`web/src/lib/editorPane.ts`, not yet read in
     detail but referenced at line 11); toggle buttons render `["terminal","editor","diff"]`
     tuples (line 113-118); `pane.view` decides which pane is visible via CSS `hidden`,
     never unmount (`h-full ${pane.view === "x" ? "" : "hidden"}`, lines 184,194,204).
     Terminal is *always* mounted (never lazy) since its WS/tmux session must survive
     switches (comment lines 109-111, 179-181); Editor/Diff mount lazily on first use
     via `editorUsed`/`diffUsed` refs (lines 44-51) and then stay mounted — a fourth
     "Browser" pane should follow this exact ref-latch + `hidden` pattern.
   - `web/src/components/EditorPane.tsx`, `DiffPane.tsx`, `TerminalPane.tsx` — sibling
     pane components to model a `BrowserPane.tsx` on.
   - `web/src/components/SidePanelPorts.tsx` — existing Ports tab UI (listening ports,
     share emails) — likely the place a "open in Browser view" affordance would live.

2. **internal/httpapi/ — port-preview proxy**
   - `internal/httpapi/proxy.go:39-64` `proxyPort` — `/proxy/{project}/{port}/...`
     (also `/p/{project}/{term}/port/{port}/...`, see `proxyPrefix` line 121-128),
     resolves cross-owner access via `previewAccess`, then `serveProxy`.
   - `internal/httpapi/proxy.go:75-101` `serveProxy` — attaches server to the
     project's Docker network (`EnsureSelfAttached`), resolves guest IP
     (`s.orch.IPAddress`), reverse-proxies via `s.proxyFor`.
   - `internal/httpapi/proxy.go:132-228` `proxyFor` — builds a cached
     `httputil.ReverseProxy` per `(ip, port, sandbox, prefix)`. **WS upgrades pass
     through automatically** (comment line 37-38, `httputil.ReverseProxy` native
     behavior — no special-casing needed for a devtools/websocket-based CDP tunnel
     if the guest served CDP over this port). Header rewriting: strips
     `Cookie/Authorization/Proxy-Authorization/X-Styrman-Ingress` and any
     `X-Caddy-*` outbound (167-181); strips `Set-Cookie` Domain attrs
     (`stripCookieDomains`, 106-114); rewrites absolute `Location` redirects back
     under the prefix (183-200); sets `Referrer-Policy: no-referrer` and, when
     `sandbox=true` (same-origin preview), a strict CSP `sandbox` response header
     (206-213) — **this CSP would break most CDP/websocket-driven tooling if a
     future CDP endpoint were proxied through the sandboxed path**.
   - Auth: `proxyPort` (line 47-59) calls `previewAccess`/`previewAllowed` — owner
     always allowed, else a Google email must be in `SharedPorts[port]`
     (`internal/httpapi/shares.go:44-97`). `isPreviewPath` (shares.go:101-107) is the
     ONLY route class a non-authorized shared guest may reach (used by `authGuard`,
     not read in this pass — worth checking directly for exact bypass order).

3. **internal/links/** (not deep-read this pass, but per ADR-025/026 below):
   `internal/links/store.go` backs public TTL links (`/link/{token}/...`, own
   subdomain origin, exempt from `authGuard`) and OAuth-gated private links
   (own-origin per email share). Both differ from the authenticated same-origin
   path chiefly in NOT setting the CSP `sandbox` header (own-origin already
   isolates them) — see ADR-025/026 in Conventions below.

4. **internal/terminal/bridge.go** — the terminal WS bridge (full read). Protocol:
   binary = PTY bytes, JSON text = `{"type":"resize",...}` control (comment lines
   1-8; `controlMessage` struct line 94-98). Opens a TTY exec via
   `orch.StartInteractive` (`internal/orchestrator/exec.go:238-268`) against
   `tmux new-session -A -D -s <name> ...` (bridge.go:286-305) — i.e. **every
   terminal is a tmux pane attach, not a bare process**. Close codes 4000/4001/4002
   documented at bridge.go:1-8 and used throughout `serve()` (255-479). This is the
   reusable skeleton for any new "pipe an in-guest process's stream to a browser
   WebSocket" feature (e.g. relaying a CDP websocket or a screencast), but it is
   PTY-shaped (TTY exec, resize control messages) — a CDP relay would want the
   *non-interactive* `Exec`/raw-stream primitives instead, or a bare TCP proxy to
   the guest's CDP port reusing the `proxyFor` ReverseProxy machinery (network
   already joined, IP already resolved).
   - `internal/orchestrator/exec.go:148-228` `Exec` — non-interactive, capped
     output, exit code — not built for a long-lived CDP session.
   - `internal/orchestrator/exec.go:230-303` `StartInteractive`/`InteractiveExec` —
     TTY-only (`Tty: true`, `ConsoleSize`), Read/Write/Resize/ExitCodeWait/Close.
     **No non-TTY long-lived bidirectional stream primitive exists yet** — a raw
     CDP-over-WS relay would need either (a) a new non-TTY interactive exec
     variant, or (b) simplest: expose CDP as a plain TCP/HTTP port inside the
     guest and reuse the existing port-preview proxy (already handles WS
     upgrades) rather than routing it through `docker exec` at all.

5. **internal/conversation/conversation.go** — `Manager talks tmux inside project
   microVMs via exec` (comment line 42); `liveSessions`/`Live`/`KillSession` all via
   `m.orch.Exec` — same `docker exec` substrate, no separate in-guest agent.

6. **internal/orchestrator/** — `docker.go`/`orchestrator.go`/`ports.go` (not
   read line-by-line this pass) hold network/IP discovery (`IPAddress`,
   `EnsureSelfAttached`, `ListeningPorts`) used by proxy.go. **No in-guest
   HTTP sidecar/agent exists** — confirmed by ADR-002 (below) and by
   `image/Dockerfile`'s own comment: "Pure tooling: no server binary inside
   (ADR-002); the styrman server drives everything via docker exec."

7. **image/Dockerfile** (full read) — Debian trixie base, per-project DinD,
   no Chromium/Playwright/Xvfb/noVNC anywhere. Tools: git, tmux, build-essential,
   python3, Go, Node 22, Docker CLI, gh, gcloud, kubectl, OpenTofu, yq, hadolint,
   shellcheck, `claude` (npm) and `agy` (Go binary) — lines 1-91. A CDP-capable
   browser would need to be added here (headless Chromium/Chromium deb, or
   `npx playwright install`) — currently absent, and the image is AMD64-only,
   size-conscious (multi-stage, `--no-install-recommends` throughout), matching
   R11's "reproducible install" goal.
   - `image/entrypoint.sh:1-30` — services started at boot: guest tuning (cgroup
     nesting for DinD), user creation matching host UID/GID, skel dotfiles,
     tmux transcript pipe-pane hooks, harness seeding via `sh-agent install`
     (lines ~130-160), then per-project dockerd setup (ext4 loop image,
     `/etc/docker/daemon.json`, lines ~208-260) — dockerd itself is the only
     long-running daemon started; **no supervisor (systemd/s6) exists inside
     the guest** — tini is PID1 (Dockerfile last line) and the script ends by
     starting dockerd, presumably backgrounded then `wait`ed (not read past
     line 260 this pass — read the tail of entrypoint.sh before designing how a
     Chrome daemon would be started/kept alive; the pattern to reuse is likely
     "start it, write a ready marker, `wait`" like the minimal-mode branch at
     lines 200-206).
   - `image/harness/` seeds claude/agy config, MCP servers, skills via
     `sh-agent install --repo /styrman/harness --home <home>`, keyed by
     `STYRMAN_VERSION` (Dockerfile `ARG STYRMAN_VERSION`, `/styrman/VERSION`,
     stamp file `~/.styrman/.harness-version`, entrypoint.sh ~lines 128-160).
     A CDP endpoint's connection details (e.g. a fixed port/URL) could be
     seeded the same way — as an MCP server config or skill referencing
     `http://localhost:<port>` — keyed on the same version stamp so it
     re-seeds on image upgrade like everything else in the harness.

## Flows

- **Existing preview flow:** browser SPA → `GET /proxy/{project}/{port}/...` or
  `/p/{project}/{term}/port/{port}/...` → `authGuard` → `proxyPort` → `previewAccess`
  (cross-owner project lookup + share-list check) → `serveProxy` → `EnsureSelfAttached`
  (join project's Docker network) → `orch.IPAddress` → cached `ReverseProxy` to
  `http://<guestIP>:<port>` with header/CSP rewriting. This is the (a) path: an
  iframe pointed at a new "Browser" pane could literally reuse this proxy
  unchanged if the guest ran a browser exposing a plain HTTP/VNC-over-WS surface
  on a project port.
- **Existing terminal flow:** browser WS → `authGuard` → `Bridge.ServeHTTP` →
  resolve conversation record → `b.ensure` (boot VM if needed) →
  `b.orch.StartInteractive` (TTY `docker exec` of `tmux new-session -A ...`) →
  bidirectional pump (PTY bytes ↔ binary WS frames), heartbeat, re-auth watch,
  close-code disambiguation. This is the model for (b): a CDP relay WS endpoint
  would need its own `Bridge`-like struct, but over a *non-TTY* stream (CDP is a
  JSON-RPC-over-WS protocol, not PTY bytes) — closer to a raw TCP proxy than to
  this PTY bridge.
- **No flow exists today** for the server or guest CLI to reach a CDP endpoint;
  ADR-002 is an explicit architectural decision AGAINST any in-guest server
  process (to keep the image "pure tooling" and orchestration centralized) —
  introducing a long-lived in-guest Chrome-with-CDP daemon is a **deliberate
  reversal of that decision's spirit** (not literally the terminal server ADR-002
  targeted, but the same "no in-container server" principle) and should be
  called out explicitly to the operator/reviewer, not silently done.

## Conventions

- View toggle UI = pill buttons with `aria-pressed`, `aria-label` pinned
  (accessible-name stability, per code-review finding cited at
  `p.$project.$term.tsx:124-133`), badge state (dirty dot, filename) via
  `aria-describedby`. A Browser tab should follow the identical accessible-name
  pattern.
  "the terminal keeps running" comments (line 109-111, 141, 143-144) are load-bearing UX
  promises — any new pane must not interrupt the terminal's session either.
- Reverse-proxy header/cookie/CSP hardening in `proxyFor` (S1/S2/S3/N21/N22
  code comments) is dense and specific — any new proxied surface (e.g. a
  screencast WS or CDP JSON endpoint) reusing this path inherits ALL of it,
  including the CSP `sandbox` on same-origin previews, which will likely need
  a carve-out or a dedicated non-sandboxed path (own-origin, like the public
  link subdomain) if CDP tooling needs it.
- ADR entries in `.agents/docs/architecture/DECISIONS.md` are the source of
  truth for "why", cited by number in comments across the codebase — follow
  that convention (new ADR for any Browser-view decision).
- Image build: every tool pinned by version + sha256 checksum (Dockerfile
  comment ~line 46-52) — a Chromium/Playwright addition must follow the same
  discipline, not a `curl | bash` or unpinned apt install.

## Risks

- **R4/ADR-005 tension**: "no embedded IDE... file viewer is the ceiling" is a
  non-negotiable requirement (REQUIREMENTS.md immutable without approval). A
  full interactive browser view is arguably adjacent to "embedded IDE" scope
  creep the operator explicitly wanted to avoid — needs an explicit
  conversation/ADR before building, not just a UI addition.
- **ADR-002's "no in-container server" principle**: adding a long-lived Chrome+CDP
  daemon in the guest is the first in-guest network service since that decision.
  It changes the trust model (image now runs a server, not "pure tooling") and
  needs its own readiness/health/supervision story — none exists in entrypoint.sh
  today beyond dockerd's own lifecycle.
- **R12 network isolation**: the guest CDP endpoint, if bound to 0.0.0.0, would
  be reachable by anything the project's Docker network can reach — for sandbox
  mode that's fine (internet-only egress, cross-project DROP-per-bridge), but the
  CDP port itself must never be exposed outward; likely must bind to loopback
  inside the guest and only be reached via the existing proxy/exec substrate, not
  a published Docker port.
- **CSP sandbox on same-origin preview** (`proxyFor`, `sandbox=true` branch) will
  break most JS-heavy CDP/devtools/screencast client code if reused verbatim for
  a live browser view served same-origin; the public/private link paths (own
  origin) don't have this problem but require going through the link-mint flow.
- **No non-TTY interactive exec stream exists** in `internal/orchestrator` — a
  CDP relay through `docker exec` (rather than a guest TCP port) would need new
  code, unlike a simple TCP/HTTP CDP port which the existing preview-proxy
  machinery already supports today.
- The claim "WS upgrades pass through" rests on `httputil.ReverseProxy`'s
  documented behavior plus the comment at proxy.go:37-38 — not independently
  verified against a live CDP handshake in this recon pass.

## Open questions

- Does the operator want the Browser view to show an iframe of an in-guest
  app (reusing the existing proxy, zero new proxy code) or a screencast of a
  real browser window the AI CLIs also drive via CDP? These have very
  different architectures (a vs b in the dispatch) and very different risk
  profiles against R4/ADR-002.
- If (b): where does the CDP endpoint live — one shared headless Chromium
  per project VM (adds real DinD-adjacent resource cost, unclear how it
  interacts with the per-project DinD's own resource budget under R1's scale
  target of 10+ concurrent projects), started by entrypoint.sh or launched
  on demand by the CLI itself (`claude`/`agy` already know how to launch
  Playwright/Puppeteer processes on their own — does styrman need to run
  anything at all, or just expose a port when the agent starts one)?
- Read the tail of `image/entrypoint.sh` (past line 260) and `internal/orchestrator/docker.go`/`ports.go`
  before design — this pass did not confirm how dockerd is backgrounded/`wait`ed,
  which is the template for supervising any new guest daemon.
- Read `internal/httpapi/server.go`'s `previewHostRewrite` and `authGuard`
  ordering, and `internal/links/store.go` in full, before deciding whether a
  Browser view's CDP/screencast traffic should ride the authenticated
  same-origin path or a dedicated own-origin link path.
