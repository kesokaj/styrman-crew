---
owner: explorer
date: 2026-10-02
---
# Recon — ui-freeze-diagnosis   (2026-10-02)

## Status: COMPLETE

## Question
Why does crew (web UI) freeze after closing a tab / sleep / network change, needing a refresh? Code as of 1fe28a0 only.

## Summary
Most likely the terminal pane has no way back from a dead-but-not-closed or "final" WebSocket state: 4001 "replaced" is terminal with no button, and a half-open socket after sleep/network change is never detected client-side and has no wake/online/visibility trigger. Polling hooks look safe (20 s fetch timeout, visibility refresh).

## Ranked causes
1. **4001 replaced is permanent and has no Resume button.** TerminalPane.tsx:223-227 sets closedForGood on 4001; button shown only for exited/stopped/disconnected (:427-429). Server: bridge.go:282-291 closes the OLD conn with 4001 whenever any new attach to the same session key arrives (a reopened/restored tab, the PWA window plus a tab, a second device). Operator closes/reopens crew in a different tab or window; the other window shows "Opened in another tab" with only refresh as escape. Falsify: reproduce freeze with a single window only, or state shown is not "Opened in another tab".
2. **Half-open socket after sleep/network change: no client heartbeat, no wake/online/visibilitychange handler in the pane.** Client only reacts to onclose (:221); state stays "connected" and keystrokes go to a dead socket (:277-279 sends if readyState OPEN; no failure signal). Server pings only detect it server-side (bridge.go:354-376, 30 s + 10 s). Reconnect relies on timers that browsers throttle/suspend in sleep or hidden tabs. Falsify: after wake, DevTools shows ws already closed with a code and "Reconnecting…" pill; or the pill reads "connected" and a ping from server side shows nothing wrong.
3. **Retry budget exhausts then Disconnected needs a click; WS handshake failures never trigger re-auth.** TerminalPane.tsx:32-33, 252-261: codes 1000/1001/1006 retry up to 8 times with backoff 2,4,8,15... s (about 90 s total), then "disconnected" (button Reconnect exists but easy to miss, and nothing retries on wake). An expired ingress session (8 h token, ADR-063) or network-down laptop makes the upgrade fail as 1006; WS path does not go through request()'s reAuthOnce (api.ts:383-386), so only polling (visible tab) can reload the page. Falsify: pill reads "Disconnected" with Reconnect working, or a terminal-only page with no polls recovers by itself.

## Checked, less likely
- hooks.ts:60-95 polling: inFlight guard could wedge, but request() has AbortSignal.timeout(20000) (api.ts:372) so it settles; hidden tab skips polls, visibilitychange refreshes.
- Session expiry on fetch: handled via redirect:"manual" + reAuthOnce reload (api.ts:362-404), capped at 2 per minute.
- BrowserPane.tsx has its own WebSocket with one auto-retry (:392-407); only skimmed, not a cause established.

## Not examined
TerminalPane unmount cleanup beyond :348-361 (looked fine); server RevocationWatch internals; BrowserPane full flow; route files (p.$project.$term.tsx), service worker/PWA manifest, ingress config; lib/terminal.ts; no runtime reproduction.

## Open questions
What exact state does the frozen pane show (pill text)? Single window or multiple (tab plus PWA)? Does Reconnect/F5 differ?

diagnosis complete
