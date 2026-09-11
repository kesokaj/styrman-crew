---
owner: reviewer
date: 2026-09-11
---
# Cold-read review, round 2 — Browser view plan (2026-09-11)

Document reviewed: `.agents/docs/plans/2026-09-11-browser-view.md` (revised),
plus the code surfaces it names: `internal/orchestrator/{ports,exec,orchestrator}.go`,
`internal/httpapi/{proxy,shares,links_api,explorer,server}.go`,
`internal/config/config.go`, `internal/terminal/bridge.go`,
`image/entrypoint.sh`, `REQUIREMENTS.md` R4, `.agents/docs/architecture/DECISIONS.md`
ADR-062, `web/src/lib/editorPane.ts`, `web/src/routes/p.$project.$term.tsx`,
`web/src/components/DiffPane.tsx`. Round-1 report also read, to check its
three gaps and three questions against this revision.

## Round-1 gaps — resolution check

All three are resolved in this revision:
1. `ErrNotRunning` detection — plan lines 212-219 now say explicitly "Each
   verb first calls `o.Status(ctx, user, project)` exactly as
   `ListeningPorts` does (`ports.go:64-71`)" — matches the real code exactly.
2. The `1012` code — removed. The revised WS relay uses a pre-upgrade 503
   for a fresh attach instead (no unnamed close code); the plan says so in
   words: "There is NO 1012 or any other unnamed close code in this
   feature." Verified against `proxy.go:76-88`'s existing 307-on-fresh-attach
   pattern, which the plan explicitly diverges from with a stated reason (a
   WS handshake can't follow a redirect) — sound.
3. Entrypoint stamp gating — plan lines 183-190 now spell out `HR_OK` as one
   unit, ANDed with `HM_OK && HI_OK`, the swallowed `mcp remove` never
   counting, and a named WARN line — matches the existing `HM_OK`/`HI_OK`
   pattern at `image/entrypoint.sh:143-153` exactly, including the
   write-once-per-version stamp shape (not gated on "did something change
   this tick," so BUG-035's failure mode does not apply here).

## (1) Gaps — undecided before implementation

- **New: the `PaneState` no-op equality guard is not extended to the new
  `browser` field — a real, demonstrated bug class in this exact file.**
  `web/src/lib/editorPane.ts:96-107`'s `set()` bails as a no-op unless
  `view`, `file`, `diff`, or `dirty` changed; the code comment at lines
  101-104 exists *specifically* because leaving `diff` out of that list once
  caused "opening a second diff while already on the diff view" to be a
  silent no-op. The plan's own "Open in Browser" behaviour (line 338-340:
  "sets `view: "browser"` and `browser.url`... the pane navigates the active
  tab to it once connected") has the identical shape: clicking "Open in
  Browser" for port B while already on the Browser view showing port A keeps
  `view` unchanged ("browser" → "browser") and touches none of `file`/`diff`/
  `dirty` — under the current guard this is a silent no-op, exactly the bug
  the `diff` comment warns about, unless `browser` is added to the compared
  fields. The "Surfaces touched" table (plan line 97) and the `cdp.ts`
  Interfaces section describe adding the `browser: { url: string | null }`
  field and `openInBrowser`, but never mention updating the equality guard.
  This needs one explicit line before Phase 3 (`set()`'s no-op comparison
  must include `browser`, or `openInBrowser` must bypass `set()`'s dedup
  entirely by constructing a state object whose `view` is guaranteed to
  differ — e.g. by not being a no-op when already on that view, matching how
  `openDiffInPane` relies on `diff` being a freshly-constructed object each
  call).
- **Minor: `internal/httpapi/links.go` does not exist.** The "Surfaces
  touched" table (plan line 95) names `internal/httpapi/proxy.go, shares.go,
  links.go` for the port-9222 refusal in `createLink`; the actual file is
  `internal/httpapi/links_api.go` (`createLink` is defined there, confirmed
  at line 51). Low-stakes — grep finds it in seconds — but worth a one-word
  fix so an implementer doesn't create a stray `links.go` alongside the real
  file.

## (2) Readiness — questions a first-pass implementer would still have to ask

1. Does `openInBrowser` need to add `browser` to `editorPane.ts`'s `set()`
   no-op guard (line 98-107), or restructure to avoid the guard, so that
   navigating to a second URL while already on the Browser view actually
   takes effect? (See gap above — this is the one question with a concrete
   failure mode if guessed wrong: "Open in Browser" silently does nothing
   the second time it's clicked from the Ports panel while the pane is
   already showing the Browser view.)

No other open questions found in this pass — the WS relay's 503 vs. 307
divergence, the `ErrNotRunning`/`Status`-first pattern, the entrypoint gate,
the `previewPortAllowed` 400 refusals, and the `GuestUser()`/`getProjectFor`
auth citations all check out exactly against the current code.

## Verdict

`design needs revision` — one gap, narrow and locally resolvable: the
`PaneState` no-op guard must be extended (or worked around) for the new
`browser` field, or "Open in Browser" from the Ports panel silently fails to
re-navigate whenever the pane is already on the Browser view — the same bug
class the file's own `diff` comment documents as having happened once
already. Not `design ready` because that line is currently unstated.

`implementation not ready` in the narrow sense — one open question (above)
that a first-pass implementer would have to resolve by either asking or
independently noticing the `diff`-guard precedent; every other interface,
failure mode, and citation in the plan is exact and actionable as-is.

## Not examined

- `image/Dockerfile`, `image/configs/*` (new files, nothing to cross-check;
  Phase 1 surfaces).
- `web/src/lib/api.ts`, `web/src/components/{BrowserPane,SidePanelPorts}.tsx`,
  `web/e2e/run.mjs`, `.agents/docs/testing/styrman/integration.sh` — Phase
  3/4 surfaces, consistent with round 1's scope.
- `coder/websocket` library semantics, Chromium/CDP protocol details,
  `chrome-devtools-mcp` behaviour — took the plan's claims at face value, as
  in round 1.
- `internal/orchestrator/orchestrator.go`'s `EnsureSelfAttached` internals
  (read only the call sites, not the implementation) and `network.go:241`.
