# 2026-08-28 — Private share links: email shares get their own subdomains (1.2)

## Summary

Both sharing modes now live on self-describing wildcard hosts under the app
domain (operator's naming): `*.public.crew.styrman.xyz` for anyone-with-the-
URL TTL links (renamed from `preview.`), and NEW `*.private.crew.styrman.xyz`
for email shares. Sharing a port mints a stable, non-expiring private token;
its subdomain is OAuth-gated at the ingress and `serveShared` re-checks the
verified email against the port's current share list on every request. Own
origin per share = no CSP sandbox = stateful apps finally work for email
guests (the `/p/` sandboxed preview remains for the owner's in-SPA view).
ADR-026.

Key security property (tested both directions): kinds are locked to routes —
a private token 404s on no-login `/link`, a public token 404s on `/shared`.

## Files changed

- `internal/links/store.go` — `Kind` (public/private), zero-expiry semantics,
  `EnsurePrivate` (idempotent) / `GetPrivate` / `DeletePrivate`, per-kind cap.
- `internal/httpapi/links_api.go` — `serveShared`, kind locks in `serveLink` /
  `revokeLink` / `listLinks`, `linkURL` private form.
- `internal/httpapi/shares.go` — mint/delete lifecycle in `setShares`; `url`
  in both shares responses.
- `internal/httpapi/server.go` — `previewHostRewrite` handles both bases →
  `/link` | `/shared`; routes; csrf exemption. `auth.go` — guests may reach
  `/shared`.
- `internal/config/config.go` — `STYRMAN_PRIVATE_BASE`.
- `deploy/caddy/ingress/` — `PRIVATE_DOMAIN` env, `writePrivateWildcard`
  (OAuth wildcard reusing the app's policy), DNS record in `EnsureDNS`.
- `deploy/docker-compose.yml`, `deploy/.env.example` — operator vars renamed
  to `STYRMAN_PUBLIC_DOMAIN` / new `STYRMAN_PRIVATE_DOMAIN`.
- `web/` — shares API returns `url`; share panel shows "copy private link";
  dist rebuilt.
- Tests: `internal/links` private lifecycle; `internal/httpapi`
  `shared_link_test.go` (access matrix + kind locks), private host rewrite;
  ingress `TestPrivateWildcardBlock`; e2e share check extended to serve +
  revoke through the private URL (mutating-path lesson from 1.1 applied).
- Docs: api.md, deploy.md, guide.md, README, ADR-026.

## Verification

- `go test -race ./...` all green; ingress module tests green.
- Integration 62/62; Playwright e2e 36/36 (share check now exercises the
  private link end-to-end).
- Live: both wildcard certs issued (`*.public.`, `*.private.`); anonymous on
  a private-host token → 302 to Google sign-in with correct redirect back;
  bogus token on public host → 404. Browser OAuth handshake left for the
  operator to confirm, as usual.

## Review fixes (adversarial review, same day)

- **Blocker:** the `/shared/…` (and `/link/…`) PATH FORMS are served on the
  main SPA origin, where `sandbox=false` would let a project's app script
  `/api/...` with the viewer's session (N22). Fixed: sandbox now derives from
  how the request arrived — token host (own origin, `previewHostRewrite`
  stashed the original path) → unsandboxed; main-origin path form →
  sandboxed. Tested (`TestSandboxDiscriminator`, `TestProxySandboxHeader`).
- `setShares` now surfaces mint/revoke failures (409 for the link cap, 500
  otherwise) instead of silently returning 200; the write+mint pair is
  serialized (`sharesMu`).
- `links.Expired` keys on Kind, fail-closed: a zero-expiry PUBLIC record is
  expired, never immortal. `DeletePrivate` rolls back on save failure (B3
  parity). `ListPublicFor` keeps the private-filter in the store.
- Ingress: both wildcards now pick the SAME target app, with `slog.Warn`
  when a domain is set but no eligible target exists; `X-Styrman-Ingress` is
  stripped unconditionally (before the conditional inject) in all three
  block types.
- `previewHostRewrite` matches the longest base first (an overlapping
  public/private pair can no longer shadow the private links).
- **Operator migration note:** `STYRMAN_PREVIEW_DOMAIN` was renamed
  `STYRMAN_PUBLIC_DOMAIN`; compose falls back to the old var so an
  un-migrated `.env` keeps public links alive.
- Cookie-exposure check (reviewer flag): verified in go-authcrunch v1.1.41
  source — `AUTHP_*` cookies render `Secure; HttpOnly` unless `insecure` is
  configured (we never do), so a shared app's JS cannot read the visitor's
  session; the proxy already strips `Cookie` upstream.
- Kept deliberately: `serveShared`'s 403 (vs no-oracle 404) for an
  authenticated-but-unshared visitor — usability over an existence oracle
  that only an authenticated token-holder can consult (commented in code).
