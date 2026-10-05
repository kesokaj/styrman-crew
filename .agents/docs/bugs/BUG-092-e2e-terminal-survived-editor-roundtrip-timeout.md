---
owner: bugs
date: 2026-10-05
id: BUG-092
status: fixed
area: web/e2e/run.mjs (editor/terminal round-trip)
---
# BUG-092: intermittent e2e "terminal session survived the editor round-trip" times out after 15 s

## Symptom
`terminal session survived the editor round-trip — page.waitForSelector: Timeout 15000ms exceeded.`
The next check, "terminal keeps its width across the round-trip", passed both times; nothing cascaded.

## Repro
Not reproduced on demand. Seen on 2026-10-05, each as `passed=100 failed=1`:
- at b1eabeb (`.agents/docs/testing/2026-10-05-verify-v1.42.0-tiers-r2.md`) under host load; a quiet rerun passed (`...-verify-v1.42.0-e2e-r3.md`).
- at f3a5f53 (`.agents/docs/testing/2026-10-05-verify-v1.42.2-full-r4.md`), load average about 2.2.
- again at f3a5f53 (`.agents/docs/testing/2026-10-05-verify-v1.42.2-e2e-r5.md`) and at ef04b30 in an A/B worktree run (`.agents/scratch/e2e-ab-ef04b30.log`), both `passed=100 failed=1`: not caused by f3a5f53.
Other runs that day passed this check.

## Root cause
Well supported, not proven by a failing trace: the check types the instant the view toggles back to the terminal. Focus returns to the terminal's textarea by itself, but not in the same tick, so keystrokes typed before it lands are lost and the echo never appears.
A temporary diagnostic (not committed) logged the focused element right before typing: `TEXTAREA.xterm-helper-textarea`, echo after 48 ms; that run passed 101/0/2 (its own evaluate added a few ms before typing).
Ruled out: a write deadline set through http.ResponseController on one request does not persist on a reused keep-alive connection (scratch Go test, request B answered 200). The earlier hypotheses (slow WebSocket reconnect, tight 15 s timeout) were not the cause.

## Fix
Commit 12c573e, `web/e2e/run.mjs`: the check waits up to 5 s for `document.activeElement` to be the xterm helper textarea before typing. It still fails if focus never returns by itself, which is the check's purpose.

## Prevention
An e2e step that types into the terminal waits for the terminal's input to hold focus first; typing the instant a pane toggles races the focus restore.
An intermittent e2e failure gets a numbered record on its second occurrence, with every run that showed it, rather than a rerun alone.

## Related
BUG-091 (lost head of a typed line before a git commit, same file, same day): consistent with the same race; its fixture already self-checks and retries. Possible link only; BUG-091's status is unchanged.
