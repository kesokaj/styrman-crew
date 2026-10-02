---
owner: main
date: 2026-10-02
---
# Stale keystrokes are dropped; a browser view whose connection went silent reconnects   (2026-10-02)

Phase 1 of `plans/2026-10-02-stale-keys-browser-liveness-updater-status.md`
(items 1 and 2 of the v1.40.0 handoff's decisions; the operator: "go with your
recommendations. Fix those then cut a new version").

## Keys typed while a pane reconnects (item 1)

v1.40.0 already dropped the queue when a pane that had GIVEN UP came back on a
wake. The gate's remaining finding was the other path: a pane still inside its
retry window when the machine slept keeps its queue, and the keys could be
typed into the shell hours later. The queue is now `web/src/lib/termOutbox.ts`:
the same 256-entry cap, and a 30 s age limit on its OLDEST entry, measured on
the wall clock so a sleep counts. Past the limit the whole queue is dropped,
never trimmed (half a command line is worse than none; D6).
`web/src/components/TerminalPane.tsx` uses it where it used a bare array.

## The browser view (item 2)

The browser view had the dead-socket case the terminal had in v1.40.0: a relay
socket that died without a close (sleep, a network change) still reads open,
and the screencast just stops. `CdpClient.alive()` (`web/src/lib/cdp.ts`) sends
`Browser.getVersion` and resolves true on ANY frame within 5 s (a reply, an
error reply, a screencast frame; D12), false on silence or a close.
`web/src/components/BrowserPane.tsx` probes when the tab is shown, when the
network returns and every 25 s while the document is visible (the Browser view
need not be the selected one; D15), and reconnects through its existing
`reconnect()` only when a probe timed out on a socket that still reads open. A
socket that closed (4002, 4003, 4004, any code) keeps its state and button.

## Tests
- `web/src/lib/termOutbox.test.ts` (new, 8 tests). With the age rule removed,
  4 fail.
- `web/src/lib/cdp.test.ts`: 6 `alive` tests. With the frame waiters not
  called, 3 fail.
- Web unit total: 203 tests in 17 files.
- Browser e2e: one new check, "a browser view whose socket went silent streams
  again once its tab is shown" (a throwaway page on terminal 2 — terminal 1 is
  the suite page's, and opening it would take its socket; D16). Against the
  page before the fix: `passed=97 failed=1 skipped=2`, that check alone. After:
  `passed=98 failed=0 skipped=2`.
- Verifier log: `testing/2026-10-02-verify-stale-keys-browser-liveness-updater-status-phase-1.md`.
- Not covered by any tier: a real sleep (the clock is injected); the 25 s
  interval and the `online` trigger in a browser.

## Docs
`specs/styrman/terminal-protocol.md` (the queue's age rule; the browser pane's
probe replaces the "Not covered: the browser pane" bullet),
`specs/styrman/guide.md` (the "Terminal stopped responding after sleep" row),
`testing/styrman/README.md` (Tier 1 and Tier 4 counts, the fixture's browser
sockets), the header comment of `BrowserPane.tsx`. `web/dist/client` rebuilt.

## Process
The plan's cold read ran three rounds (`reviews/2026-10-02-review-plan-stale-keys-browser-liveness-updater-status.md`,
overwritten each round). Round 3 ended `design needs revision`,
`implementation ready`, with four non-blocking gaps. At the cap the operator
said: "Fix all things before cut a new version or defer the things or skip."
All four were written into the plan (D18–D21). Code review and the docs lens run
once over the whole round, before the release.

## Learnings for the local host
None: nothing in this phase touches the host path.
