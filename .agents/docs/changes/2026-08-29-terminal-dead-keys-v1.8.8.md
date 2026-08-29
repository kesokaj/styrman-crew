# Terminal: Nordic/Swedish dead keys ~ ` ^ ´ ¨ (v1.8.8)

Date: 2026-08-29. Follow-up to the AltGr/Option research. Direct compose symbols
(`@ $ { } [ ] \ | € µ`) already work (v1.8.7). The remaining gap is **dead
keys** — `~ ` ^ ´ ¨` — which on Nordic/Swedish (ISO) layouts are typed as a
diacritic key followed by Space (bare char) or a vowel (accent). xterm routes
these through the browser IME, which is buggy for these layouts (xterm.js
[#2151](https://github.com/xtermjs/xterm.js/issues/2151),
[#2479](https://github.com/xtermjs/xterm.js/issues/2479) — both open), so `~ ` ^`
came out mangled. These matter in a shell (`~` home, `` ` `` command
substitution, `^` in `git HEAD^` / regex).

## Change (`web/src/lib/terminal.ts`)

A new first-in-chain key handler intercepts dead keys, bypassing xterm's IME:
- On a dead keydown (`ev.key === "Dead"`), map the physical key + modifiers to
  the pending spacing diacritic (`deadDiacritic`): `BracketRight` → `¨`/`^`/`~`
  (plain/Shift/AltGr), `Equal` → `´`/`` ` `` (plain/Shift). `preventDefault` +
  stash `pendingDead` so xterm never starts its (broken) composition.
- On the next keystroke: **Space** → emit the bare diacritic; a **vowel** in the
  `DEAD_COMPOSE` table → emit the precomposed letter (é, ã, ô, …); anything else
  → emit the bare diacritic then let the key proceed. Bare modifier presses are
  ignored so the sequence waits for the real base key.
- Shared `altGrOf()` now backs both the dead-key and the direct-compose handlers.

Keyed on `ev.key === "Dead"` + specific `ev.code`s, so it's inert on US layouts
(those keys aren't dead there) and falls through untouched for unmapped dead keys
(German/French/etc.) — no regression for other layouts.

## Temporary diagnostic

Mac dead-key `code`/modifier values can differ from PC-ISO and can't be tested
headless, so this ships a `console.log("[styrman kbd] Dead", …)` that fires
**only on dead-key presses**. If the mapping is wrong on the operator's Mac, the
log shows exactly what the browser sent so the mapping is corrected in one pass.
To be removed once Nordic dead keys are confirmed working.

## Release

Rebuilt bundle (tsc clean) → `styrman-server` 1.8.8, deployed live (healthy, crew
200). Needs a browser hard-refresh. **Operator to verify:** `~ ` ^ ´` (dead key
then Space) and an accent (e.g. dead `´` then `e` → `é`); if any misbehave, open
the browser console and paste the `[styrman kbd] Dead` lines.

## Verified + diagnostic removed (v1.8.9)

Operator confirmed all Nordic dead keys work on their Mac — the PC-ISO mapping
was correct as-is. The temporary `console.log("[styrman kbd] Dead", …)` was
removed and the server rebuilt + redeployed as **v1.8.9** (healthy, crew 200).

