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
`setProjectLan` now passes `signal: AbortSignal.timeout(150000)`
(`web/src/lib/api.ts:490`), matching start/stop, with a comment saying why.
The two e2e checks pass `timeout: 150000`.

## Prevention
When a new endpoint performs an operation an existing endpoint already
performs (here: a container stop+start), it inherits that operation's
TIMEOUT BUDGET too — grep for how the existing caller of the same
operation is configured before adding the new one. A test that bypasses
the client under test cannot vouch for the client's own settings.

## Related
BUG-021 (same ADR-067 toggle), BUG-023.
