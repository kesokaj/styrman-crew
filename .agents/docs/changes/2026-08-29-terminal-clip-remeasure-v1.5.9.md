# Terminal last-row clip — force a cell re-measure after font load (v1.5.9)

Date: 2026-08-29. Corrects v1.5.8: the CSS change there was NOT the fix — it
destabilized sizing and made the clip worse (operator: "it is worse, and it
depends on the window size"). Reverted the v1.5.8 CSS back to the original
(`.term-host, .xterm, .xterm-viewport { height/width: 100% }`).

## Real cause

xterm measures the character cell size once at first render. If the custom
terminal font (Google Sans Code) is still loading then, the cell height is
measured from the shorter FALLBACK font and cached. `fit()` reuses that cached
value — it does NOT re-measure — so once the real (taller) font lands, the row
count is one too many: the grid is slightly taller than the pane and the last
row (claude status line) clips. It's window-size-dependent because at some pane
heights the wrong count still happens to fit; at others it overflows by a row.

The v1.5.7 "refit on fonts.ready" didn't help precisely because `fit()` alone
kept the stale cell height.

## Fix

On `document.fonts.ready`, toggle the `fontFamily` option (to `monospace` and
back, same frame — no flash) to force xterm's char-size service to RE-MEASURE
with the loaded font, then `fit()`. A self-assignment is a no-op in xterm's
options service (it only reacts to a real change), so the value must actually
toggle. The server session is resized if the row count changed. The existing
ResizeObserver keeps later window resizes correct with the now-accurate cell
height.

Files: `web/src/components/TerminalPane.tsx`, `web/src/index.css` (revert).

## Status

Built + deployed live as styrman-server 1.5.9. Needs operator visual
confirmation (this session has no browser tools to verify the render). If a
clipped row survives a hard refresh, the next step is connecting Claude-in-Chrome
or a DevTools measurement of `.term-host` vs `.xterm` heights.
