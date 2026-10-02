---
owner: bugs
date: 2026-10-02
id: BUG-085
status: fixed
area: web/src/components/TerminalPane.tsx, internal/terminal/bridge.go
---
# BUG-085: a terminal stopped responding until the page was reloaded

## Symptom
Operator: "Somethings lets say If I close the tab for crew it freezed, i need to refresh the window pretty often, can we fix this?" No exact repro was given; asked, the operator chose "Infer both, fix both (Recommended)".

## Repro
Inferred, each scripted in the browser suite:
(a) open the same terminal in a second tab, close the second tab: the first shows "Opened in another tab" and nothing else.
(b) let the socket stop carrying data without a close event (sleep, network change): the pane reads connected and typing does nothing.

## Root cause
(a) On close code 4001 the pane set its closed-for-good state and rendered "Opened in another tab" with no control (buttons existed only for exited, stopped, disconnected), under the rule "NEVER reconnect on 4001". Any new attach closes the previous one with 4001 (`internal/terminal/bridge.go`), so a second tab, or the installed PWA window beside a tab, left the first pane dead until reload.
(b) The server pings every 30 s with a 10 s bound and closes ITS side, but that close cannot reach a page whose connection is gone, and a browser gives script no ping. The pane had no heartbeat and no handler for the tab being shown or the network returning; it reacted only to a close event. A pane that used its 8 retries (about 90 s of backoff) stayed "Disconnected" until clicked.
Independent diagnosis (`plans/2026-10-02-recon-ui-freeze-diagnosis.md`, an `explorer` given only the symptom, code at `1fe28a0`) ranked the same two causes first and second, and third the retry budget plus a WebSocket handshake that never triggers re-authentication.

## Fix
- `internal/terminal/bridge.go` `classifyControl`: a `{"type":"ping"}` text frame is answered `{"type":"pong"}` under a 5 s write bound and is not activity for the idle reaper (commit `11e4f9a`).
- `web/src/lib/termLiveness.ts` (new): per-socket liveness check.
- `TerminalPane.tsx`: project terminals probe when the tab is shown, when the network returns and every 25 s while visible; an unanswered probe (5 s) drops the socket and reconnects in place without boot intent; the same wake events reconnect a "Disconnected" pane; a replaced pane shows "Use here" and takes the terminal back by that button or when its tab is shown again. Decision: ADR-079. Fixed in the release after v1.39.0.
- Not fixed, by decision: the setup pane (a reconnect restarts a finished `styrman-setup`); the browser pane (`BrowserPane.tsx`) has cause (b) and is an open question for the operator.
- Expired-session half of the third ranked cause unchanged: page polls (`web/src/lib/projects.ts`) already detect it and reload to sign in (`reAuthOnce`, `web/src/lib/api.ts`, BUG-001); a terminal's own handshake still does not.

## Verification
Browser suite (`web/e2e/run.mjs`), three checks: "a terminal taken by another tab is taken back with one click", "a replaced terminal comes back by itself when its tab is shown again", "a terminal whose socket went silent works again once its tab is shown". Before the fix: `passed=93 failed=3 skipped=2` (missing "Use here": `locator.click: Timeout 15000ms exceeded`; recovery never came: `page.waitForFunction: Timeout 20000ms exceeded`, twice). With the fix: `passed=96 failed=0 skipped=2`. Unit: `web/src/lib/termLiveness.test.ts` (11 tests; four guards removed one at a time each failed exactly one test), `TestClassifyControl` (two mutations each failed it).
A first red run failed all three for the WRONG reason (`Please use browser.newContext()`: Playwright refuses `context.newPage()` on the implicit context of `browser.newPage()`); the suite now creates an explicit context and the red run was repeated.
Not covered: a real sleep, real network change, real hidden tab (emulated at the page's own APIs); the 25 s interval and the `online` trigger in a browser.

## Prevention
A state the page can enter must offer its way out on the same page, or say why there is none. A long-lived connection needs a liveness check owned by the side that has to react: a server-side ping protects the server, not the page.

## Related
BUG-001 (session expiry showed raw errors until F5: same symptom family, different cause), ADR-079.
