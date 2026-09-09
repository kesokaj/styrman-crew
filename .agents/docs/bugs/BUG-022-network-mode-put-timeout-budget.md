---
owner: bugs
date: 2026-09-09
id: BUG-022
status: fixed
area: web/src/lib/api.ts (ADR-067 network mode)
---
# BUG-022: setProjectLan inherited request()'s 20s default instead of the stop/start timeout budget

## Symptom
The two e2e network-mode toggle checks failed with
`apiRequestContext.put: Timeout 30000ms exceeded` while the rebuild
completed successfully server-side.

## Repro
PUT `/api/projects/<id>/network` on a RUNNING project; the synchronous
rebuild (stop, swap bridge, boot the Kata microVM) outlasts the client's
abort.

## Root cause
`setProjectLan` went through `request()` (`web/src/lib/api.ts:363`) and so
inherited its default `AbortSignal.timeout(20000)`. That default exists to
stop a wedged server hanging the UI forever. But a mode change is a
stop+start, and `startProject`/`stopProject` already carry
`AbortSignal.timeout(150000)` for exactly that reason ("start/stop
legitimately take minutes on a slow Kata boot"). The new endpoint was
added without that budget, so a user flipping the switch in the browser
would see a failure for a change that had actually worked. The e2e did not
catch it earlier because its checks called `page.request.put` directly,
bypassing the SPA's api client and hitting Playwright's own 30s default
instead.

## Fix
**Second attempt, and the first one was wrong in the same way as the bug.**

The first fix gave `setProjectLan` `AbortSignal.timeout(150000)` by copying
`startProject`/`stopProject`. The code review caught that 150 s is LESS than
the server's own bound for this request — `lanRebuildTimeout = 3 * time.Minute`
(`internal/httpapi/network.go:36`) — so a rebuild taking 150-180 s would abort
in the client while succeeding on the server: BUG-022's exact failure, merely
in a narrower window.

150 s is correct for start/stop because the SERVER bounds a plain start at 2
minutes; it was never a general "slow operation" constant. Copying a sibling's
number instead of reading the server budget for THIS request is what went
wrong twice.

Now `AbortSignal.timeout(210000)` — the server's 3 minutes plus the same 30 s
slack start/stop carries over its own 2 — with a comment pairing it to the Go
constant by name. The two e2e checks were raised to 210 s for the same reason.

## Prevention
A client-side timeout must be derived from the SERVER's bound for the same
request, and must exceed it. Read that constant; do not copy the number a
neighbouring endpoint uses, because its value encodes ITS server's budget, not
a general notion of "slow". Where the two live in different languages, name the
server constant in a comment beside the client value so the pairing is
greppable — nothing else links them.

Original rule, still true:
When a new endpoint performs an operation an existing endpoint already
performs (here: a container stop+start), it inherits that operation's
TIMEOUT BUDGET too — grep for how the existing caller of the same
operation is configured before adding the new one. A test that bypasses
the client under test cannot vouch for the client's own settings.

## Related
BUG-021 (same ADR-067 toggle), BUG-023.
