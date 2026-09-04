# Set OAuth session token lifetime to 8 hours (date: 2026-09-04)

## Summary

The OAuth ingress (vendored caddy-security) never set an explicit `crypto default token lifetime` directive, so Caddy's 900-second default governed session expiry. BUG-001 (2026-09-03) made this expiry visible to users as a toast notification and page reload every 15 minutes of active use, prompting operator re-authentication. **Fix:** `generator.go` now exports `SessionLifetimeSeconds = 28800` and `writeSecurityBlock()` emits `crypto default token lifetime 28800` in the `authentication portal myportal` block, setting the session window to 8 hours zone-wide. Trade-off recorded in ADR-063: an 8-hour cookie-theft window replaces the 15-minute one, but the ingress cookie does not control session revocation — every request re-checks the userstore via `authGuard`, so changing a user's role or admin status takes effect on the next request regardless of token lifetime. Users sign in once when the ingress redeploys with this change (pre-existing behavior on any Caddy config change).

## Type

bugfix

## Files changed

- `deploy/caddy/ingress/generator.go` — New exported constant `SessionLifetimeSeconds = 28800` (8 hours in seconds). `writeSecurityBlock()` updated to emit `crypto default token lifetime 28800` in the `authentication portal myportal` block. This replaces the implicit Caddy default (900 s) with an explicit, operator-tunable value.
- `deploy/caddy/ingress/generator_test.go` — New test `TestGenerateCaddyfile_SessionLifetimeIsExplicit` validates that the generated Caddyfile contains the lifetime directive with the correct value (28800).
- `.agents/docs/specs/styrman/deploy.md` — Added documentation under the `STYRMAN_JWT_KEY` bullet explaining that session lifetime is now controlled by the ingress `crypto default token lifetime` directive and configurable via `SessionLifetimeSeconds` in `generator.go`.

## Agents dispatched

security (`.agents/docs/reviews/2026-09-04-security-session-lifetime.md`); verifier (`.agents/docs/testing/2026-09-04-verify-session-lifetime.md`). No reviewer: diff is one constant declaration, one `Fprintf` call, one test, and docs; author read it.

## Verification

- New test `TestGenerateCaddyfile_SessionLifetimeIsExplicit` confirms the generated Caddyfile includes `crypto default token lifetime 28800`.
- Verifier validates the generated Caddyfile syntax against the vendored Caddy binary and confirms the directive parses correctly.
- Security review confirms the trade-off (8-hour cookie window vs per-request authGuard re-check) meets the compliance and usability requirements.

## Requirements

R5 (updated via ADR-062 amendment, separate commit `24490c9`); R8 unchanged (authentication enforced at the ingress, authorization re-checked per request on every load). No further `REQUIREMENTS.md` edit.

## Related

- BUG-001 (`.agents/docs/bugs/BUG-001-fail-to-fetch-session-expiry.md`) — session expiry visibility fix that prompted this lifetime tuning.
- ADR-062 (Authorization and per-request re-checks; updated 2026-09-04 to reference the 8-hour lifetime).
- ADR-063 (Session lifetime decision: 28800 s zone-wide, authorized by operator 2026-09-04).
- Shipped in v1.20. Sessions survive a Caddy redeploy (the signing key is pinned by `STYRMAN_JWT_KEY`): tokens issued before the release keep the old 900 s expiry, later ones live eight hours; rotating the key is the only force-expire lever (security review, medium, fixed in ADR-063's text before commit).
