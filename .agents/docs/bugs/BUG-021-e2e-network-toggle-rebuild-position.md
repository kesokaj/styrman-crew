---
owner: bugs
date: 2026-09-09
id: BUG-021
status: fixed
area: web/e2e/run.mjs (ADR-067 network-mode checks)
---
# BUG-021: e2e network-mode toggle rebuilt the container mid-suite, killing 17 unrelated checks

## Symptom
The browser e2e suite went from 89 passing to 72 passed / 17 failed. Failures
spanned the port-preview proxy, per-port sharing, public/private links, the
whole git side panel (diff, fast commit, branches, log, stash, history) and
the client-error gate, plus three 500s on `/api/projects/<id>/...`. Nothing
connected the failures on their face; the git ones looked like a
guest-exec regression.

## Repro
`bash web/e2e/run.sh` with the Phase 5 rewrite of `run.mjs` in place.

## Root cause
Phase 5 replaced the old ADR-065/066 LAN checks with ADR-067 ones IN THE
SAME PLACE — `run.mjs` ~line 707, right after the Ports block starts the
`:4321` fixture app in a terminal (~line 699) and ahead of every later
check. The old checks were read-only against a stubbed root status file.
The new ones TOGGLE the mode, and under ADR-067 a toggle REBUILDS the
project container (deliberate operator decision — "restart is fine").
Confirmed by measurement: container id changed `3415cb5cfff2` ->
`30b481786c66` across one PUT, destroying the `:4321` server, the tmux
sessions and the live guest state that ports, links, history, git and
terminals all depend on. Secondary effect: the firewall probe (~line 767)
treats only a TimeoutError as the host firewall's DROP signature; with the
fixture killed it got connection-refused instead, logged "firewall probe:
fetch failed (NOT the firewall)", and ran proxy checks it should have
skipped — so a destroyed fixture presented as a firewall anomaly.

## Fix
Split the section. The three read-only checks stay at ~707 with a pointer
comment. The two toggling checks moved to their own "Network mode: the
toggle (ADR-067)" section right before Cleanup, after everything needing
guest state, prefixed with `page.goto(/p/<PID>/1)`. Follow-on defect found
by the move: the Network block lives inside the Ports tab, so the moved
block must click the Ports tab before waiting on anything in it.

## Prevention
A check that RESTARTS, REBUILDS or DELETES the thing under test is not
interchangeable with the read-only check it replaces, however similar its
subject — place it after every check that depends on live state, or give
it its own throwaway project. When replacing checks in a shared suite, ask
what the OLD ones did to the world before inheriting their position.

This rule graduated into `.agents/rules/e2e-checks-leave-the-suite-as-they-found-it.md` on 2026-09-22.

## Related
BUG-022, BUG-023, BUG-025, BUG-026 — all found in the same ADR-067 Phase 5
suite run, `.agents/docs/plans/2026-09-09-lan-mode-by-bridge-prefix.md`.
