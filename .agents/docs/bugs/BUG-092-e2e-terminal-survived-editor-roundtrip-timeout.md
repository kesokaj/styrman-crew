---
owner: bugs
date: 2026-10-05
id: BUG-092
status: open
area: web/e2e/run.mjs (editor/terminal round-trip)
---
# BUG-092: intermittent e2e "terminal session survived the editor round-trip" times out after 15 s

## Symptom
`terminal session survived the editor round-trip — page.waitForSelector: Timeout 15000ms exceeded.`
The next check, "terminal keeps its width across the round-trip", passed both times; nothing cascaded.

## Repro
Not reproduced on demand. Seen twice on 2026-10-05, each as `passed=100 failed=1`:
- at b1eabeb (`.agents/docs/testing/2026-10-05-verify-v1.42.0-tiers-r2.md`) while a server image build and a security review's measurements ran on the host. A rerun on a quiet host (load 1.36) passed (`.agents/docs/testing/2026-10-05-verify-v1.42.0-e2e-r3.md`).
- at f3a5f53 (`.agents/docs/testing/2026-10-05-verify-v1.42.2-full-r4.md`) with host load average about 2.2 and only a read-only review running.
Other e2e runs that day passed this check (at least 8 runs).

## Root cause
Unknown. The check waits 15 s for a selector after the editor pane toggles back to the terminal. Unverified hypotheses: the terminal's WebSocket reconnect after the round-trip sometimes takes longer than 15 s; the 15 s timeout is tight for a full-size VM.

## Fix
Not fixed. Next step: print what the main pane shows when the wait times out (the e2e `check()` already prints role=alert/status text since BUG-091), and measure the reconnect time.

## Prevention
An intermittent e2e failure gets a numbered record on its second occurrence, with every run that showed it, rather than a rerun alone.

## Related
BUG-091 (another intermittent e2e failure the same day, same file).
