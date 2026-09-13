---
owner: reviewer
date: 2026-09-13
---
# Review — audit of web/src/lib/   (2026-09-13)

Scope: `/audit` scan against recorded intent. Every `.ts` file in
`web/src/lib/` except `*.test.ts`, at HEAD `e56e775708a9062cc6432f5c3278770bed63b685`:
`api.ts`, `browserInput.ts`, `cdp.ts`, `clips.ts`, `editorPane.ts`,
`explorerHistory.ts`, `fonts.ts`, `hooks.ts`, `lan.ts`, `projectBadge.ts`,
`projects.ts`, `recentsPanel.ts`, `sshKey.ts`, `terminal.ts`, `toast.ts`.
Judged against `AGENTS.md`, `REQUIREMENTS.md` (R1–R13), the three newest
`.agents/docs/changes/` entries, `.agents/docs/specs/styrman/*.md`, and
`.agents/docs/bugs/INDEX.md`.

## Verdict: REQUEST CHANGES

The directory is otherwise unusually well hardened (extensive comments citing
prior review rounds and bug numbers), but `request()`'s generic 20 s abort
silently clips the three call sites that deliberately override it to 150 s /
210 s (`startProject`, `stopProject`, `setProjectLan`) back down to ~20 s.
This is the exact mechanism of **BUG-022** ("setProjectLan kept request()'s
20s abort though the PUT rebuilds the project"), reintroduced one commit
after BUG-022 was fixed, and now affects two more call sites than the
original bug did.

## Findings

- **[high / confidence: high]** `web/src/lib/api.ts:361-372` (`request()`) —
  `signal: init?.signal ? AbortSignal.any([init.signal, AbortSignal.timeout(20000)]) : AbortSignal.timeout(20000)`
  combines **any** caller-supplied signal with a *fresh* 20 s timeout via
  `AbortSignal.any`, which aborts on whichever fires first. `startProject`
  (api.ts:515-519) and `stopProject` (api.ts:520-524) pass
  `AbortSignal.timeout(150000)`, and `setProjectLan` (api.ts:502-507) passes
  `AbortSignal.timeout(210000)`, each with a comment explaining exactly why
  the longer bound is required (Kata boot/teardown, network-mode rebuild).
  Because `AbortSignal.any` always races the fresh 20 s timer in on every one
  of these calls, the effective client timeout for all three is ~20 s, not
  150 s/210 s: on a slow (but successful, server-side-bounded) boot,
  teardown, or LAN rebuild, the fetch aborts at 20 s and the UI reports a
  failure for an operation that is still running and will complete —
  reproducing BUG-022's exact symptom, now for `startProject`/`stopProject`
  too, one commit (`3ce8a37`) after BUG-022's own fix (`8db7ac6`/`5697e14`)
  landed. Confirmed by diff: before `3ce8a37`, `request()` used
  `signal: init?.signal ?? AbortSignal.timeout(20000)` (caller signal REPLACES
  the default); `3ce8a37` changed this to always AND-combine, to fix
  `checkFile`'s cancellation signal (which is a genuine cancel-on-unmount
  signal, correctly meant to keep the 20 s floor) — but the same code path
  now silently breaks every caller that passes a signal meaning "give me
  MORE time," not "let me cancel early." `api.encoding.test.ts` exercises
  these three call sites only for URL-encoding, never for the abort budget,
  so no test catches this. **Fix:** distinguish "this signal extends the
  budget" from "this signal is a cancellation trigger" — e.g. give `request`
  an explicit `timeoutMs` option (default 20000) used alone via
  `AbortSignal.timeout(timeoutMs)`, and combine only a caller's *cancellation*
  signal (`init.signal`) with it via `AbortSignal.any`, never overwrite a
  caller's own `AbortSignal.timeout(N)` with the 20 s default's race.

- **[low / confidence: high]** `web/src/lib/diffReveal.test.ts` — this test
  file lives in `web/src/lib/` but imports and tests `reveal` from
  `../components/DiffView` (`web/src/components/DiffView.tsx`), not anything
  in `lib/`. Per review-standards criterion 4 (directory layout) and the
  `typescript` skill, "tests live beside the code they cover... not in a
  parallel tree." An engineer looking for `DiffView`'s tests in
  `web/src/components/` would not find them. **Fix:** move the file to
  `web/src/components/DiffView.test.ts` (or `.tsx`).

- **[low / confidence: medium]** `web/src/lib/clips.ts` — no `clips.test.ts`
  exists, and no test imports the module directly (only `SidePanelClips.tsx`
  uses it). The module has non-trivial invariants documented in its own
  comments (TTL filtering, first-wins dedup, and — explicitly called out at
  `clips.ts:71-74` — `getClips()` must return a *stable reference* when
  nothing expired, or `useSyncExternalStore` infinite-loops). None of that is
  exercised by vitest, unlike every sibling module with comparable logic
  (`lan.ts`, `editorPane.ts`, `recentsPanel.ts`, `explorerHistory.ts` all have
  dedicated test files). **Fix:** add `clips.test.ts` covering TTL expiry,
  dedup-on-add, and the stable-reference contract of `getClips()`.

## Criteria

1. **Dead code** — pass. No unused exports, no commented-out blocks, no
   orphaned files found in this directory.
2. **Flow tracing** — fail on the one high finding above: `request()`'s
   signal-combining change (commit `3ce8a37`) was not traced to its other
   two long-timeout callers (`startProject`, `stopProject`) when it was made
   to fix `checkFile`'s cancellation case; everything else traces cleanly
   (error paths wrapped in `ApiError`/toast, `AbortSignal` threaded through
   `checkFile`, cleanup functions returned from every `useSyncExternalStore`
   subscribe and every `useEffect`).
3. **Standards conformance** — otherwise strong: no `any`, no bare
   `// @ts-ignore`, `catch` blocks narrow via `instanceof`/type guards, no
   floating promises found (async chains are `.then/.catch`-terminated or
   awaited), boundaries (`localStorage`/`sessionStorage`/JSON parses) are
   consistently guarded and sanitized before use.
4. **Directory layout** — one violation: `diffReveal.test.ts` misplaced (see
   finding above). Everything else sits where expected.
5. **Simplicity (DIY-except-security)** — pass. No new dependencies; the CDP
   client (`cdp.ts`) and the terminal wrapper (`terminal.ts`) are hand-rolled
   but neither touches auth/crypto/session material (the OSC-52 clipboard
   read is explicitly denied for that reason), so the security exemption
   does not apply and stdlib-first is honored throughout.
6. **Decoupling** — pass. Small, well-scoped modules (`fonts.ts`, `lan.ts`,
   `projectBadge.ts` are pure and framework-free by design so logic-heavy
   surfaces stay testable); `api.ts` is a single large file but is a flat
   list of thin per-endpoint wrappers around one `request()`, not a grab-bag
   of unrelated logic — acceptable for a typed API client.

## Not examined

- `web/src/lib/*.test.ts` files themselves beyond checking what they cover
  (out of the stated scope, and per instructions not a source file to audit).
- Runtime behavior was not executed (no `npm test`/`npm run check` run in
  this session); the `AbortSignal.any` finding is established by static
  reading of `api.ts` and the git history of commit `3ce8a37`, not by an
  observed failing test — no test currently exercises the abort budget, so
  none can be pointed to as red.
- `web/src/components/`, `web/src/routes/`, server-side (`internal/`, `cmd/`)
  code, and the vendored Caddy ingress — out of the stated scope
  (`web/src/lib/` only).
- Whether the same signal-combining defect also affects any WebSocket-based
  call path (`termSocketUrl`, `browserSocketUrl`) — those do not go through
  `request()` and were not separately audited for an analogous timeout bug.
