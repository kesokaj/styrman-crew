---
owner: bugs
date: 2026-09-13
id: BUG-044
status: fixed
area: web/src/components/BrowserPane.tsx
---
# BUG-044: hidden BrowserPane pushed a collapsed 320x200 viewport and kept streaming to the shared browser

## Symptom
Switching away to another terminal still pushed a viewport resize (320×200,
`Math.max` floor over a 0×0 hidden box) to the Chrome instance the AGENTS
share, and kept the screencast running while the pane was invisible.

## Repro
Mount BrowserPane, switch to a terminal that hides it: a resize command and
screencast frames still go out.

## Root cause
The component derived what to send to the shared browser from its own
(collapsed) DOM geometry, with no notion of visibility — a 0×0 hidden box
still produced a "real" clamped size, and there was no gate on the
screencast writer either.

## Fix
Added a `visible` prop, a 0×0 guard on the resize path, and a guarded
screencast writer that only writes frames while visible.

## Prevention
A hidden pane must never write to a shared resource on the strength of its
own collapsed geometry — visibility must be an explicit input to any code
path that mutates something other components/agents depend on.

## Related
BUG-043 (same component tree, same root symptom of latched/hidden mounts
acting as if live).
