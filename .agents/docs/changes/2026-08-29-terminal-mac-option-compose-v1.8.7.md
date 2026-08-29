# Terminal: mac Option composes @ (cross-platform) (v1.8.7)

Date: 2026-08-29. Follow-up to v1.8.6. Ctrl+C now works, but the AltGr fix only
covered Windows/Linux — the operator is on a **Mac** and types `@` with **left
Option+2**, which still produced `(arg: 2)`.

## Cause

v1.8.6 detected "compose" only via `getModifierState("AltGraph")` or
`ctrlKey+altKey` (the Windows/Linux AltGr signals). On macOS, left/right Option
reports plain `altKey` with **no** AltGraph and **no** ctrlKey, so the branch
missed it and xterm fell back to treating Option as Meta → `ESC 2`.

We deliberately keep `macOptionIsMeta` **off** so Option types symbols (that's
how a Nordic Mac layout makes `@ { } [ ] $ €`), which is exactly what the
operator wants — so the fix is to recognise Option-as-compose, not flip that
option.

## Fix (`web/src/lib/terminal.ts`)

`compose` now also matches `isMac && ev.altKey && !ev.ctrlKey`, where `isMac` is
a one-time `navigator.platform`/userAgent check. When the OS has composed a
printable single-character `ev.key`, we type it directly and stop xterm. The
existing length-1 guard still skips Option+Arrow (word-nav), F-keys and dead
keys, so those keep their Meta/escape behavior.

Platform matrix now covered:
- **Windows/Linux** — AltGr (Right Alt) via AltGraph or ctrl+alt; left-Alt stays
  real Meta (readline `Alt+b`).
- **macOS** — Option (either side) composes; `@` = Option+2.

## Release

Rebuilt bundle (tsc clean) → embedded in `styrman-server`; deployed live as
**v1.8.7**. Server healthy, crew 200, Caddy upstream 172.18.0.2. Needs a browser
hard-refresh.
