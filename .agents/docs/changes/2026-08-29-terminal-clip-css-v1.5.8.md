# Terminal last-row clip — the real fix was CSS, not the font refit (v1.5.8)

Date: 2026-08-29. The v1.5.7 font-ready refit did NOT fix the clipped bottom
row (the fonts are self-hosted and load before/at first fit, so the refit was a
no-op). The actual cause was in the CSS.

## Cause

`index.css` forced `height: 100%` on xterm's own inner elements:

```css
.term-host, .term-host .xterm, .term-host .xterm .xterm-viewport {
  height: 100%; width: 100%;
}
```

The terminal is sized by the **fit addon**, which sets a row/column *count* and
lets xterm lay out `.xterm` and `.xterm-viewport` to that grid. Forcing those
inner elements to the full pane height fights that: the viewport is pinned to
the pane height while the row grid can come out a hair taller (sub-pixel cell
rounding), so the last row (the claude status line) ended up just past the
pinned viewport and was clipped instead of leaving a small background gap.

## Fix

Size only the container and let xterm manage its own inner heights (the
standard xterm setup):

```css
.term-host { height: 100%; width: 100%; overflow: hidden; }
.term-host .xterm { width: 100%; }
```

Any leftover is now an invisible background-colored gap below the last row
(bounded by one cell height), never a clipped row. `overflow: hidden` on the
host keeps a sub-pixel grid from poking into neighbouring UI. The v1.5.7 refit
is kept (harmless; helps if a font ever loads late).

Files: `web/src/index.css`.

## Release

Built + deployed live as styrman-server 1.5.8.
