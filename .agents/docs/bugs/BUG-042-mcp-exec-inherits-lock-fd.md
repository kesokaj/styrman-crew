---
owner: bugs
date: 2026-09-13
id: BUG-042
status: fixed
area: image/configs/styrman-browser (launcher do_mcp)
---
# BUG-042: chrome-devtools-mcp exec inherited the start-lock fd, blocking the next `start` for 20s

## Symptom
While an agent's chrome-devtools-mcp session was open, the next pane `start`
blocked ~20s and then failed — the flock on fd 9 was held for the MCP
server's entire lifetime, not just the launch.

## Repro
`do_mcp` exec's chrome-devtools-mcp without closing fd 9; run `start` again
while an MCP session from a prior `start` is still alive.

## Root cause
`do_mcp` opened the start-lock on fd 9 (flock) but the final `exec` into
chrome-devtools-mcp did not carry `9>&-`, so the child inherited the open fd
and held the flock for as long as the MCP process ran — far longer than the
lock was meant to cover.

## Fix
Added `9>&-` to the `do_mcp` exec line, matching the `launch`/`ensure_forwarder`
setsid lines which already closed it. `TestLauncherPinsPortsAndCaps` now
asserts the exec line closes fd 9.

## Prevention
Every exec/spawn from a function that opened a lock fd must list the fd
close on that exec line, and a test must assert the exec line's argv/redirects
directly — do not infer it from behavior alone.

## Related
Review batch .agents/docs/reviews/2026-09-13-coldread-browser-review-fixes*.md;
plan .agents/docs/plans/2026-09-13-browser-review-fixes.md.
