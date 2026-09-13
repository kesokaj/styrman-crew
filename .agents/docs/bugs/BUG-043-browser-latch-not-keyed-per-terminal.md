---
owner: bugs
date: 2026-09-13
id: BUG-043
status: fixed
area: web/src/routes/p.$project.$term.tsx
---
# BUG-043: browser-used latch survived terminal switches, mounting a hidden BrowserPane per terminal visited

## Symptom
`browserUsed` (and the equivalent editor/diff latches) were stored in a ref
that outlived terminal switches. Every terminal visited after the first one
that used the browser mounted a hidden `BrowserPane` — one relay socket plus
one launcher exec each — climbing toward the per-user cap of 8.

## Repro
Open a terminal, trigger browser use, switch to a second terminal that never
used the browser: a hidden BrowserPane mounts for it too.

## Root cause
The "mounted once, keep mounted" latch was a single ref not keyed by which
terminal it belonged to, so it read as true for every subsequent terminal in
the same session.

## Fix
Per-`${project}/${term}` keyed latch (`useViewLatch`) replacing the single
shared ref.

## Prevention
A "mounted once, kept mounted" latch must be keyed by the identity of the
thing the mount belongs to (project+terminal here), never a bare ref shared
across everything the component renders for.

## Related
Same batch as BUG-042, BUG-044; plan .agents/docs/plans/2026-09-13-browser-review-fixes.md.
