---
owner: bugs
date: 2026-09-03
id: BUG-001
status: fixed
area: web/src/lib/api.ts (session/auth)
---
# BUG-001: sidebar and stats bar show raw "Failed to fetch" after ~5-15 minutes, F5 fixes it

## Symptom
Operator (2026-09-03, verbatim): "after some minutes the bottom stats bar and
the sidebar show 'fail to fetch'; a browser F5 fixes it until it recurs."

## Repro
Any view, tab active, 5-15 minutes after page load: every REST-backed surface
(stats bar, sidebar, file explorer) shows the raw "Failed to fetch" text; an
open terminal (WebSocket) keeps working; F5 recovers.

## Root cause
The caddy-security session token expires after its default `token lifetime`
of 900s (unset in `deploy/caddy/ingress/generator.go` `writeSecurityBlock`;
live ingress log showed the operator's IP failing with "keystore: parsed
token has expired" in bursts of ~80 req/min at 12:49-13:28, 20:06, and 21:30
on 2026-09-03). The authorizer answers an expired `/api/*` request with a 302
to `https://auth.<domain>/oauth2/google/?redirect_url=…`, which 302s on to
`accounts.google.com` — cross-origin. A browser `fetch()` that follows
redirects rejects with a bare `TypeError: Failed to fetch` before any
Response exists (confirmed in real Chromium and by curl against
crew.styrman.xyz), so the existing session-expiry handling in
`web/src/lib/api.ts` `request()` (`res.redirected && res.url.includes
("/oauth2/")` → `reAuthOnce()`, commit 19aabf3, 2026-08-30) never fired.
`usePolled`/the file explorer rendered the raw error on every poll until a
top-level navigation (F5) re-ran the OAuth flow.

Also went wrong during this fix (counted here per the "more than one
attempt" rule, not separate bugs):
1. First cut cleared the `reauth` sessionStorage counter on the first ok
   response, letting a page with one same-origin 3xx plus a healthy poll
   reload every 1.2s forever — caught by security review.
2. A test asserted a toast after advancing fake timers past the toast's 5s
   TTL — one extra attempt to fix the test.

## Fix
`web/src/lib/api.ts`: `request()` now passes `redirect: "manual"` and treats
`res.type === "opaqueredirect"` like a 401 (`reAuthOnce()`: one toast, one
reload after 1.2s); `uploadFiles` got the same handling. The `reauth`
sessionStorage counter is shape-validated and cleared only after the page
has stayed healthy for 10s with no re-auth fired. Tests:
`web/src/lib/api.session.test.ts` (5 tests). `web/dist/client` rebuilt.
Commit: the commit that carries this record.

## Prevention
A browser `fetch()` cannot observe the ingress's auth redirect by following
it — the chain is cross-origin and rejects with a bare TypeError — so every
API call must use `redirect: "manual"` and read `res.type ===
"opaqueredirect"` as an expired session. Never add a fetch path that
bypasses `request()` without the same two lines, and never reset the re-auth
counter on a response — only on a page that has stayed healthy.

Open follow-up (not this bug's fix): the 900s token lifetime is an operator
decision, tracked in TODO.md.

## Related
`.agents/docs/changes/2026-09-03-fail-to-fetch-session-expiry.md`;
`.agents/docs/plans/2026-09-03-diagnosis-fail-to-fetch.md` (explorer,
goldfish diagnosis); `.agents/docs/reviews/2026-09-03-security-fail-to-fetch.md`
(security, two rounds); `.agents/docs/testing/2026-09-03-verify-fail-to-fetch.md`
and `-round2.md` (verifier).
