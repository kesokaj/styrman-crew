---
owner: bugs
date: 2026-09-13
id: BUG-045
status: fixed
area: internal/httpapi/browser.go (relayWebSockets)
---
# BUG-045: `websocket.CloseStatus(err) == StatusMessageTooBig` branch was unreachable

## Symptom
An oversized upstream frame closed the browser pane with 1011 instead of the
intended 1009; found while planning, not in production incident review.

## Repro
Send a 16 MiB+1 frame through the relay; observe close code.

## Root cause
`coder/websocket` v1.8.15's read path (read.go:527) returns a wrapped
`ErrMessageTooBig`, not a bare value `websocket.CloseStatus` can extract, so
the `==` comparison against `StatusMessageTooBig` never matched and the 1009
branch was dead code.

## Fix
Switched to `errors.Is`, pinned by `TestBrowserRelayOversizeUpstreamFrameIs1009`
which sends a real 16 MiB+1 frame and asserts the resulting close code.

## Prevention
An error-classification branch needs a test that actually reaches it through
the real dependency's error path — a branch no test hits is a branch that
may not exist. When classifying a third-party library's errors, check
whether it wraps rather than assuming a bare sentinel.

## Related
Batch .agents/docs/plans/2026-09-13-browser-review-fixes.md.
