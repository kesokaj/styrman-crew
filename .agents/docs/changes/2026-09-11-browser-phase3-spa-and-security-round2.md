---
owner: main
date: 2026-09-11
---

# Browser view — Phase 3: SPA pane and Phase 2 security findings (2026-09-11)

Phase 3 of 4: frontend SPA pane with CDP client, user input forwarding, browser status management, and all fixes for review and security findings from Phase 2 commit 0e543e3.

## Summary

**Verification at completion:** `npm run check && npm run lint && npm test` green (147 → 171 tests); `go vet + go test -race ./...` green; ingress module green; shellcheck clean; image rebuilt; Kata launcher probe (4 cases) passed; Playwright hand-walk of built SPA against real LAN-mode project passed (golden path, stop edge case, twice); integration test clean run (line being appended to Phase 3 testing log).

## The code

**web/src/lib/editorPane.ts:** `view` store gains `"browser"` option. `browser: BrowserRequest | null` field (new object per request for identity guard). New actions `openInBrowser(port)`, `clearBrowserRequest()`. Guard compares `browser` identity. `openFileInPane()` carries browser request through. Tests +3.

**web/src/lib/cdp.ts (new, 140 lines, 6 tests):** `CdpClient` over a `CdpTransport` interface for structured CDP communication. Implements id correlation (request→response matching), event fan-out by sessionId, close rejects pending calls with `CDP_CLOSED`. Testable via fake transport.

**web/src/lib/browserInput.ts (new, 110 lines, 11 tests):** Input param builders: `pointOf` (DOMRect → screencast coords), `modifiersOf` (KeyboardEvent flags), `mouseParams`, `wheelParams`, `keyParams` (for Input.dispatchMouseEvent, etc.). `normalizeUrl`: bare host → `http://`, `:3000` → `http://localhost:3000`, `localhost:8080/x` is NOT `http://` (not a scheme).

**web/src/lib/api.ts:** Router adds `browserStatus()`, `browserStart()`, `browserStop()`, `browserSocketUrl()`. Encoding tests +4.

**web/src/components/BrowserPane.tsx (new, 380 lines):** Full pane component. Toolbar: Page back/forward, Reload, Address bar, New tab, close-to-terminal button. Tab strip: all pages including agent's tab (refreshed via Target.getTargets on Page.loadEventFired/frameNavigated; Chromium's targetInfoChanged carries URL as interim title). Canvas screencast: Page.startScreencast (jpeg, q70), decode on newest-frame-wins basis, ack immediately. Emulation.setDeviceMetricsOverride syncs to ResizeObserver changes (150 ms debounce). Input.* forwards mouse/wheel/keyboard; Ctrl/Cmd+L and Ctrl/Cmd+R intercepted. JS dialogs auto-accepted with notice. States: `connecting` / `ready` / `stopped` (4002, show terminal + Reconnect) / `unavailable` (4004, reason + Retry) / `disconnected` (Reconnect). Automatic handshake retry per chain.

**web/src/routes/p.$project.$term.tsx:** Fourth pill "Browser". `browserUsed` latch tracks if pane opened during session. Hidden-not-unmounted mount.

**web/src/components/SidePanelPorts.tsx:** "browser" button per row → `openInBrowser(http://localhost:<port>/)`.

**web/dist:** Rebuilt and committed.

## Defects found during Phase 2 reviews and fixed in Phase 3

All findings from `.agents/docs/reviews/2026-09-11-review-browser-phase2.md` (APPROVE WITH NITS: 1 low) and `.agents/docs/reviews/2026-09-11-security-browser-phase2.md` (FINDINGS: 1 high, 2 medium, 5 low) addressed:

**HIGH / security#1:** Stale link/share records for ports 9222/9229 served through `serveLink`/`serveShared` (no dynamic filtering). **Fixed:** both handlers now call `previewPortAllowed` and 404 for browser ports. Tests plant records for both ports through the link store and SetPortShares.

**MEDIUM#1 / security#2:** Relay followed redirects from guest's DevTools server → cross-project steering (R12 violation). **Fixed:** dedicated `http.Client` with `ErrUseLastResponse` for `/json/version`. Passed as WebSocket dial's HTTPClient (coder/websocket honours caller's CheckRedirect). Test with redirecting fake for both discovery and dial.

**MEDIUM#2 / security#3:** Gateway fallback failed open toward host (socat range unset = any peer). **Fixed:** Launcher no longer guesses. Server passes `STYRMAN_BROWSER_PEER` on every start (bridge's live address on that network, else bridge gateway for dev server, else none). Launcher accepts IPv4 literal only, no peer → no forwarder (loopback-only). Forwarder in separate process group so agent-started browser gains the door on pane's first start (`ensure_forwarder`). Kata-verified.

**LOW#1:** Peer string unvalidated inside socat's option list (hostile `1.2.3.4,range=0.0.0.0/0` accepted). **Fixed:** IPv4 regex; hostile strings refused in probe.

**LOW#2:** Relay side effects before Origin check. **Fixed:** `sameOriginOrAbsent` first. Test sends hostile Origin → 403, no exec.

**LOW#3:** No cap on concurrent relays. **Fixed:** `browserRelayMax = 32`; 503 + Retry-After when exceeded. Test included.

**LOW#4:** `refuse_links` ran after `mkdir -p` (symlink race). **Fixed:** checked first. `~/.styrman` included.

**LOW#5:** Guest-controlled text in boot log. **Fixed:** `tr -cd '[:print:]' | cut -c1-200` on all guest logs.

**Reviewer LOW:** Message name hardcoded 9222 when refusing 9229. **Fixed:** message names the refused port; test asserts it.

**Accepted risk:** Phase 1 MEDIUM#4 (shared Chromium profile/cookies) remains accepted design per ADR-068.

## Defects found during Phase 3 hand-walk and fixed before commit

Three issues found during the hand-walk and corrected:

1. Unbounded automatic handshake retry → capped per chain.
2. Init failure handler overwrote 4002 "stopped" with "disconnected" a microtask later → fixed sequencing.
3. Stopped state had no recovery path once project restarted → reconnect button now visible in stopped state.

## Verification

**Static:** `npm run check` (tsc -b) clean; `npm run lint` (oxlint) clean; `go vet ./...` clean; `shellcheck -S warning` clean.

**Unit tests:** `npm run test` (vitest) green (147 → 171 tests). `go test -race ./...` green. Ingress module `(cd deploy/caddy/ingress && go test ./...)` green.

**Docker build:** `docker build -t styrman-runtime:browser-p3 --build-arg STYRMAN_VERSION=dev-<ts> image/` green.

**Kata integration (Tier 2):** `.agents/docs/testing/2026-09-11-verify-browser-phase3.md` recorded. `bash .agents/docs/testing/styrman/integration.sh styrman-runtime:test`: the clean run's line is recorded in the Phase 3 testing log; two server-side checks were corrected first, because a bare-exec'd `start` (no `STYRMAN_BROWSER_PEER`) is loopback-only by design since round 2 — the ports-list check now watches 9229, and the routes check asserts the 9222 forwarder is up and hidden after the SERVER's start. (Earlier run was invalid due to port 18080 collision with hand-walk's local server; follow-up: make integration.sh port overridable.)

**Playwright hand-walk:** Built SPA against real LAN-mode project: golden path (address bar navigation, tab creation, stop state transition, reconnect) and stop edge case (project stop mid-screencast, reconnect after restart), each run twice.

**Observations (pre-existing, not fixed):** Project stop takes ~48 s on this host with or without browser. `GET /api/projects/{id}/terminals` answered 500 once during stop window.

## Files changed

New:
- `web/src/lib/cdp.ts` (140 lines, 6 tests)
- `web/src/lib/browserInput.ts` (110 lines, 11 tests)
- `web/src/components/BrowserPane.tsx` (380 lines)

Modified:
- `web/src/lib/editorPane.ts` (+3 tests)
- `web/src/lib/api.ts` (+4 tests)
- `web/src/routes/p.$project.$term.tsx` (fourth pill, browserUsed latch)
- `web/src/components/SidePanelPorts.tsx` (browser button)
- `web/dist/` (rebuilt)
- `internal/httpapi/browser.go` (previewPortAllowed check in serveLink/serveShared)
- `internal/httpapi/browser_test.go` (tests for port filtering on link/share)
- `image/configs/styrman-browser` (STYRMAN_BROWSER_PEER acceptance, range/max-children)
- `.agents/docs/testing/styrman/README.md` (the two corrected server-side browser checks described; the count stays 93)
- `.agents/docs/plans/2026-09-11-browser-view.md` (Phase 3 ticked)

## Agents dispatched

- **reviewer:** Phase 2 diff at commit 0e543e3 (`.agents/docs/reviews/2026-09-11-review-browser-phase2.md`).
- **security:** Phase 2 diff at commit 0e543e3 (`.agents/docs/reviews/2026-09-11-security-browser-phase2.md`).
- **verifier:** Phase 1+2 integration (`.agents/docs/testing/2026-09-11-verifier-browser-phase1-2.md` PASS).
- **reviewer, security, verifier:** Phase 3 diff (after commit).
- **changelog:** this entry.

## Requirements

R4 (Browser testing surface): met by SPA pane with agent-shared session in per-project Chromium profile, full user interaction capability.

R3 (per-project isolation): enforced by separate profile per project home (`~/.styrman/browser/profile`).

R12 (project isolation): relay honors no redirects (ErrUseLastResponse); no cross-project steering.

ADR-068: Browser view architecture (peer restriction, profile, DevTools port gating, accepted shared-session risk).
