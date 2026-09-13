---
owner: bugs
date: 2026-09-13
id: BUG-055
status: fixed
area: web/src/lib/api.ts (request helper)
---
# BUG-055: request() re-clips long-running calls to 20s, reintroducing BUG-022

## Symptom
`startProject`/`stopProject` (150s budget) and `setProjectLan` (210s
budget) time out at 20s even though the server completes the operation —
a LAN toggle or slow start surfaces a spurious timeout error.

## Repro
Not independently reproduced this session; found by
`.agents/docs/reviews/2026-09-13-audit-project.md` reading
`web/src/lib/api.ts:370-372`.

## Root cause
`request()` combines every caller-supplied `AbortSignal` with a fixed
`AbortSignal.timeout(20000)` via `AbortSignal.any`, unconditionally. This
is BUG-022's exact mechanism (a 20s bound silently overriding a longer
caller-set budget), reintroduced one commit after BUG-022 was closed: the
2026-09-10 fix for "the first abortable check request silently lost its
20s bound" combined signals for the general case but did not special-case
callers that already supply their own bound.

## Fix
`web/src/lib/api.ts:370-372` — `signal: init?.signal ?? AbortSignal.timeout(20000)`,
so a caller-supplied signal (already itself combined with its own bound,
as `checkFile` does) is used as-is instead of being intersected with the
default. Vitest asserts a 30s caller signal is not aborted at 20s.

## Prevention
A helper that applies a default bound must not silently override an
explicit one supplied by the caller. Any change to a shared "combine with
default" helper must grep every caller that already sets its own
bound/signal before assuming the default is additive.

## Related
BUG-022 (original mechanism). `.agents/docs/reviews/2026-09-13-audit-project.md`.
