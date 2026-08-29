# Terminal: Ctrl+C interrupt + AltGr composed characters (v1.8.6)

Date: 2026-08-29. Two operator-reported terminal input bugs.

## 1. Ctrl+C still didn't interrupt (e.g. `ping`)

The v1.8.3 fix let Ctrl+C pass through to xterm's default (which sends `\x03`),
but that wasn't enough: **copy-on-select leaves a highlight, and the browser
copies on Ctrl+C and swallows the keydown** before xterm ever sends the byte, so
`^C` never reached the shell.

Verified the guest side is fine — in a project VM, `tmux send-keys C-c` killed a
foreground `sleep` and the pane exited, so the PTY delivers SIGINT correctly. The
loss was entirely browser-side.

**Fix (`web/src/lib/terminal.ts`):** Ctrl+C now sends ETX (`\x03`) *explicitly*
via `term.input()` and calls `ev.preventDefault()`, so neither the browser's copy
nor xterm's default can intercept it; any lingering selection is cleared. Only
Cmd+C (metaKey) copies.

## 2. AltGr+2 typed `(arg: 2)` instead of `@`

On Nordic/EU layouts `@` is **AltGr+2** (and `$ { } [ ] \ €` are other AltGr
combos). Browsers report AltGr as the `AltGraph` modifier state, or — Chrome on
Windows/Linux — as `ctrlKey+altKey` together. xterm mistook it for Meta and sent
`ESC 2`, which readline renders as the numeric-argument prompt `(arg: 2)`.

**Fix:** a new key handler detects AltGr (via `getModifierState("AltGraph")` or
`ctrlKey && altKey`) and, when the OS has already composed a printable
single-character `ev.key`, types exactly that character and stops xterm. Plain
left-Alt (true Meta, e.g. readline `Alt+b`) is deliberately left untouched, so
Meta bindings still work.

## Release

Rebuilt the web bundle (`npm run build`, tsc clean) → embedded in
`styrman-server`; built + deployed live as **v1.8.6**. Requires a browser
hard-refresh to pick up the new bundle. The server recreation left it multi-homed
(ingress + two project bridges) and Caddy correctly kept the ingress upstream
(172.18.0.2) — validating the v1.8.5 discovery fix under live conditions.
