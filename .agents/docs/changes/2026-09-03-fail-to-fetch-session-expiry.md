# Session expires: handle opaque redirects to prevent "fail to fetch" in UI     (date: 2026-09-03)

## Summary

After 900 seconds (the caddy-security default token lifetime), the Caddy ingress expires the session. When a user's SPA subsequently fetches `/api/*`, the authorizer returns a 302 cross-origin redirect to `https://auth.<domain>/oauth2/google/` → `accounts.google.com`. The SPA's `request()` function in `web/src/lib/api.ts` followed redirects by default, so `fetch()` rejected the cross-origin chain with `TypeError: Failed to fetch` *before* any Response object was constructed. The 2026-08-30 expiry-handling logic (`res.redirected && res.url.includes("/oauth2/")` → `reAuthOnce`) never fired, leaving the user seeing "fail to fetch" in the stats bar and sidebar until F5. The WebSocket terminal was unaffected.

**Fix:** `request()` now passes `redirect: "manual"` (pinned after the init spread) so cross-origin redirects become opaque responses (`res.type === "opaqueredirect"`), caught and handled as an expired session. `reAuthOnce()` fires once per session expiry (toast + one reload after 1.2 s, capped at 2 per minute via a sessionStorage counter cleared only by a 10 s timer that checks no re-auth happened). The counter is shaped-validated and overwrites corrupted state rather than disabling the cap. `uploadFiles()` (which bypassed `request()`) now gets the same redirect handling. Security review round 1 found a reload-loop bug where the counter could be defeated via partial failure (same-origin 3xx + healthy poll reset the counter) — fixed by only clearing on a no-reauth timer, not on reload. Round 2 confirmed the fix.

## Type

bugfix

## Files changed

- `web/src/lib/api.ts` — `request()` passes `redirect: "manual"` (pinned after init spread so callers cannot bypass it) and treats `res.type === "opaqueredirect"` as an expired session → `reAuthOnce()`. `uploadFiles()` updated to handle the same redirect case. New sessionStorage counter `REAUTH_STATE` and `REAUTH_CLEAR_AFTER_MS` (10 s) timer mechanism prevent reload loops under partial failure (session expires on one endpoint, other endpoints healthy). Counter is shape-validated (type is string `"<number>"`); corrupt or missing records are overwritten instead of disabling the cap.
- `web/src/lib/api.session.test.ts` — New file, 6 tests: re-authenticates once instead of surfacing "Failed to fetch" (failed before fix); bounds reload loop when one endpoint redirects and others are healthy; clears counter only after page stays healthy for 10 s; ignores malformed counter; overwrites corrupt counter; `uploadFiles` re-authenticates too.
- `web/dist/client` — Rebuilt with `npm run build` and committed as a whole tree so the `go:embed` bundle carries the fix.

## Agents dispatched

explorer (goldfish diagnosis: `.agents/docs/plans/2026-09-03-diagnosis-fail-to-fetch.md`); verifier rounds 1 and 2 (`.agents/docs/testing/2026-09-03-verify-fail-to-fetch.md`, `…-round2.md`, both PASS, every changed hunk covered); security rounds 1 and 2 (`.agents/docs/reviews/2026-09-03-security-fail-to-fetch.md`: round 1 = 1 medium / 5 low; round 2 = medium closed, 2 new lows, one fixed with test and one recorded in TODO); bugs (BUG-001); changelog (this). No reviewer (diff under 50 lines; author read it). No third security round (round-2 low fix is the reviewer's own stated resolution, three lines, covered by test).

## Verification

- 6 new tests in `web/src/lib/api.session.test.ts` cover re-auth behavior: expired-session detection, reload loop bound, counter lifecycle, malformed/corrupt handling, uploadFiles re-auth.
- verifier rounds 1 and 2: all changed hunks covered; counts in `.agents/docs/testing/2026-09-03-verify-fail-to-fetch.md` and `…-round2.md`.
- Frontend test suite: 17 tests / 3 files, all passing.
- Evidence of root cause: ingress log shows "keystore: parsed token has expired" at ~80/min bursts (2026-09-03 12:49–13:28, 20:06, 21:30); curl confirms redirect chain; real Chromium confirms `fetch()` with default redirect follows cross-origin chain (TypeError) but `redirect: "manual"` returns opaqueredirect.

## Requirements

R8 unchanged (auth at the ingress; authorization re-checked per request on every load). No `REQUIREMENTS.md` edit.

## Related

- BUG-001 (`.agents/docs/bugs/BUG-001-fail-to-fetch-session-expiry.md`)
- ADR-030 / ADR-032 (credential broker architecture; session tokens managed by the ingress)
- Token lifetime tuning is deferred: the operator's decision on token TTL (suggested 8 h vs current 900 s / 15 min), recorded in TODO.md with security review context. The reload currently recurs every 15 minutes of use.
- Ingress's zone-wide trusted login-redirect suffix (pre-existing, low/low) recorded in TODO.md as out of this fix's scope.
