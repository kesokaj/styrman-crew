---
owner: reviewer
date: 2026-09-11
---
# Cold-read review, round 3 (final) — Browser view plan (2026-09-11)

Document reviewed: `.agents/docs/plans/2026-09-11-browser-view.md` (revised
since round 2), plus the code surfaces it names:
`internal/orchestrator/ports.go`, `internal/httpapi/{proxy,shares,links_api,
explorer,server}.go`, `internal/config/config.go`, `internal/terminal/
bridge.go`, `image/entrypoint.sh`, `REQUIREMENTS.md` R4,
`.agents/docs/architecture/DECISIONS.md` ADR-062/ADR-068, `ROADMAP.md`,
`web/src/lib/editorPane.ts`, `web/src/routes/p.$project.$term.tsx`,
`web/src/components/DiffPane.tsx`. Round-2 report also read, to check its two
findings against this revision.

## Round-2 findings — resolution check

Both resolved:
1. The `PaneState` no-op guard for `browser` — plan line 97 now states
   explicitly that `set()`'s guard "gains `prev.browser === next.browser`"
   and that `openInBrowser` "always allocates a NEW `browser` object so a
   second port while already on the Browser view is never a no-op," with a
   named unit test. Matches the `diff`/`openDiffInPane` precedent
   (`editorPane.ts:150-152`) exactly.
2. `internal/httpapi/links_api.go` — the "Surfaces touched" table (plan line
   95) now names `links_api.go`, matching the real file
   (`createLink` at `links_api.go:51`).

## (1) Gaps — undecided before implementation

- **New, low severity: `openFileInPane` (`editorPane.ts:120-135`) is the one
  `set()` call site that does NOT spread `prev`/`get(...)` — it builds a
  literal object naming every field explicitly (`view`, `file`, `diff`,
  `dirty`, `draft`).** The plan's fix for round 2's gap (line 97: `set()`'s
  guard gains a `browser` comparison) implies `PaneState` gains a `browser`
  field, but the plan never says `openFileInPane`'s literal object also needs
  a `browser: get(project, term).browser` line, the same treatment it
  already gives `diff`/`dirty`/`draft` for exactly this reason (carry
  unrelated pane state through an object literal that doesn't spread). Left
  as-is, this is a straightforward `tsc` error ("Property 'browser' is
  missing"), not a silent bug — self-resolving the first time `npm run
  check` runs — but Phase 3's `editorPane.ts` task (plan line 427) lists only
  `npm test` (vitest) as its verify step; `npm run check` isn't run until the
  later `BrowserPane.tsx` task (line 431), so the error would surface several
  steps downstream of where it was introduced rather than immediately.
- **Minor, informational: Phase 1's first checkbox is already done.** Plan
  line 408 still lists "Operator approves the exact R4 sentence above; edit
  `REQUIREMENTS.md` R4 and write ADR-068 in `DECISIONS.md` in ONE commit with
  `ROADMAP.md` "Now"" as an unticked `- [ ]` task. It has already landed:
  commit `a9ae789` ("R4: a Browser view is a testing surface...") edited all
  three files together, `REQUIREMENTS.md` R4 already carries the exact
  sentence the plan proposes, ADR-068 is already in `DECISIONS.md`
  (verbatim, matching plan lines 122-136), and `ROADMAP.md` "Now" already
  points at this plan. Not a design problem — the decision was made and
  recorded exactly as specified — but the plan document itself is stale on
  this one box, and a first-pass implementer executing Phase 1 literally
  would either duplicate the ADR/R4 text or need to stop and check git log
  first.

## (2) Readiness — questions a first-pass implementer would still have to ask

1. Does `openFileInPane`'s literal `set()` call also need a
   `browser: get(project, term).browser` line, matching `diff`/`dirty`/
   `draft`? (See gap above — self-correcting via `tsc`, but the plan doesn't
   say it, so it costs a stop when `npm run check` finally runs at the
   `BrowserPane.tsx` step instead of at `editorPane.ts`.)
2. Is Phase 1's first checkbox already satisfied by commit `a9ae789`, or
   does the plan want it re-verified/re-committed? (See gap above — answered
   by one `git log` look, but the plan as written doesn't say so.)

No other open questions found: the WS relay's 503-on-fresh-attach shape, the
`Status`-before-`Exec` pattern (`ports.go:64-71`), the entrypoint's
`HR_OK`/`HM_OK`/`HI_OK` gating (`entrypoint.sh:143-153`), the
`previewPortAllowed` 400 refusals in `proxyPort`/`setShares`/`createLink`
(all three call the port-extraction/validation helper before other work, as
claimed), `GuestUser()`/`getProjectFor` auth citations, the `authGuard`
wrapping of `/ws/*` routes (`server.go:375-376,505`), and the
`editorUsed`/`diffUsed` hidden-not-unmounted latch pattern the plan says
`BrowserPane` will follow (`p.$project.$term.tsx:44-53`) all check out
exactly against the current code.

## Verdict

`design needs revision` — one narrow, low-severity gap: `openFileInPane`'s
literal `PaneState` construction needs the same `browser` carry-through the
plan already gives `set()`'s equality guard, one line the plan doesn't
state. Not `design ready` because that line is unstated, though the failure
mode here is a loud compile error rather than a silent bug (unlike round 2's
finding on the same file).

`implementation not ready` in the narrow sense — two small open items above,
both quickly resolved by inspection (the type checker for the first, `git
log` for the second) rather than by a real design choice. Every interface,
failure mode, and code citation checked this round is exact and actionable
as written.

## Not examined

- `image/Dockerfile`, `image/configs/*` (new files, nothing to cross-check
  against; Phase 1 surfaces, per round 1/2 scope).
- `web/src/lib/{cdp,browserInput,api}.ts`, `web/src/components/{BrowserPane,
  SidePanelPorts}.tsx`, `web/e2e/run.mjs`,
  `.agents/docs/testing/styrman/integration.sh` — Phase 3/4 surfaces,
  consistent with round 1/2's scope (new files or additive test counts, no
  existing code to cross-check).
- `coder/websocket` library semantics, Chromium/CDP protocol details,
  `chrome-devtools-mcp` behaviour — took the plan's claims at face value, as
  in rounds 1 and 2.
- `internal/orchestrator/orchestrator.go`'s `EnsureSelfAttached` internals
  (read only the call sites) and `network.go:241`.
