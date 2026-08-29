# Sidebar project expand/collapse + terminal font-refit (v1.5.7)

Date: 2026-08-29. Two UI fixes reported while testing the live app.

## Sidebar: projects stay expanded, with explicit toggles

Before, a project's terminal list rendered only for the *active* project
(`params.project === p.id`), so opening another project collapsed the previous
one — you could never see two projects' terminals at once.

- Projects now stay expanded independently of which one is active. Expand state
  persists across navigation and reloads in `localStorage` (key
  `styrman-collapsed-projects`). We store the COLLAPSED set, so expanded is the
  default: a new project shows its terminals with no action, and collapse is
  explicit.
- Each project row has a **chevron toggle on the right** (rotates when
  expanded) to collapse/expand just that project without navigating.
- The "Projects" header has an **Expand all / Collapse all** button that flips
  based on current state.
- Opening a project (row click) also expands it. `TerminalList` now renders for
  every expanded project; `activeTerm` highlighting is scoped to the active one.

Files: `web/src/components/Sidebar.tsx`.

## Terminal: refit once fonts are ready (fixes clipped bottom row)

The terminal's first `fit()` runs before the custom terminal font
(Google Sans Code) has loaded, so cell height is measured from a fallback font.
When the real font loads and cell metrics change, the row count was never
recomputed — the last row clipped, showing as a "wrong bottom margin" (the
claude status line cut off at the bottom of the pane).

- `TerminalPane` now refits on `document.fonts.ready` (guarded against a
  swapped/torn-down terminal) and sends the corrected size to the server if the
  row count changed. The existing `ResizeObserver` already handled later size
  changes; this closes the initial font-load gap.

Files: `web/src/components/TerminalPane.tsx`.

## Release

Built + deployed live as styrman-server 1.5.7 (frontend rebuilt and embedded).
