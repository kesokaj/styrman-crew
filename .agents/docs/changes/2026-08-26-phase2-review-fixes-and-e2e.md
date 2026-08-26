# 2026-08-26 — Phase 2 review fixes + Playwright e2e suite

## Summary

The Phase 2 reviewer pass returned 4 blocking + ~14 should-fix findings; all
blockers and the high-value items are fixed. Added a full browser e2e suite
(Playwright, 18 checks with screenshots) since the operator couldn't test
interactively — it immediately caught two real bugs the API-level suite
couldn't see. Final state: unit green, integration 23/23, e2e 18/18.

## Repo-critical review findings

- **`cmd/styrman/` was never committed**: the `.gitignore` pattern `styrman`
  (for the build artifact) was unanchored and swallowed the server main
  package AND `.agents/docs/specs|testing/styrman/`. A fresh clone didn't
  build. Fixed to `/styrman`; everything added this commit.
- **`web/dist` still ignored by the Vite template's `web/.gitignore`**,
  contradicting ADR-012 ("dist committed so go build needs no Node"). Fixed.

## Bugs fixed (review + e2e)

- Deleted conversations resurrected: WS attach auto-created missing records,
  so a stale reconnect recreated them with a fresh session. WS attach is now
  strict 404; `EnsureRecord` removed. (Caught live by the e2e suite.)
- Client abort stranded half-deleted projects: DELETE/stop handlers used the
  request context, so a closed tab cancelled destroy mid-flight (container
  gone, record left). Destructive handlers now run on detached contexts.
  (Caught by the e2e suite; diagnosed via server log "context canceled".)
- Unbounded terminal reconnect on any close code — now only 1000/1001/1006,
  exponential backoff capped at 8 tries, and a Disconnected/Reconnect and
  Session-ended/Start-again UI state (epoch remount).
- claude resume no longer depends on transcript-path guessing: resume-first
  (`--resume || --session-id`), so a wrong path costs one retry, not a
  bricked conversation. `agentSession` validated on load AND at use (UUID
  regex) — closes the hand-edited-store shell-injection hole; agentCmd used
  in both branches.
- SPA catch-all returned 200 HTML for unknown /api and /ws paths (now JSON
  404) and could list embedded directories (now index fallback only).
- Terminal-id TOCTOU: reservations with 60s expiry.
- Missed resize while disconnected: explicit resize frame on WS open.
- Unhandled promise rejections in all async click handlers → `tryUI` helper
  surfaces API errors; rename Enter+blur double-PATCH guarded; hooks ref
  assignment moved to effect.
- New tests: `paneCommand` table test (incl. injection attempts), httpapi
  routing test (JSON 404s, SPA fallback, no dir listing, CSRF 403).

## Deferred (TODO'd)

Poll amplification (3 pollers per open project), two-stage delete confirm UX
→ proper dialog, log-level tuning, REQUIREMENTS/AGENTS frontend-stack line
alignment (needs operator approval — see below).

## Open question for the operator

REQUIREMENTS.md constraint says "TanStack Start"; ADR-012 documents why the
embedded static SPA uses TanStack Router + Vite instead. Reviewer correctly
notes REQUIREMENTS.md must not change without explicit approval — awaiting
operator sign-off to amend the constraint line (or a decision to migrate).

## Verification

go test ./... green (4 packages) · integration 23/23 · Playwright e2e 18/18
(screenshots in /tmp/styrman-shots) · live on http://192.168.86.58:8080
