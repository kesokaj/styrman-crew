# 2026-08-27 — Public TTL preview links: ingress wired (1.1 complete)

The public-link feature is now end-to-end. Enabled by STYRMAN_PREVIEW_DOMAIN.

- Ingress (`deploy/caddy/ingress`): `caddy_preview_target` label +
  `PREVIEW_DOMAIN` → a `*.PREVIEW_DOMAIN` public wildcard site block fronting
  the styrman server (Host preserved so the server reads the <token>
  subdomain; client X-Caddy-* stripped; ingress token injected), plus the
  wildcard DNS A record in `EnsureDNS`. Caddy issues the wildcard cert.
- Server: `previewHostRewrite` maps `<token>.STYRMAN_PREVIEW_BASE/…` →
  `/link/{token}/…`; each link gets its own origin so the app's root-relative
  paths work. `linkURL` returns the subdomain URL when the base is set.
- Compose: PREVIEW_DOMAIN (caddy) + STYRMAN_PREVIEW_BASE (server) from one
  STYRMAN_PREVIEW_DOMAIN var; caddy_preview_target label on the server.
- Fixed pre-existing vendored test drift (devbox- → styrman- prefix).

Verified LIVE: DNS record created, cert obtained, https://<token>.preview.
styrman.xyz served the app over TLS with no auth, bogus token 404'd, revoke →
404 immediately. Tests: go 8/8 pkgs, integration 62/62, e2e 36/36, ingress
unit tests green.
