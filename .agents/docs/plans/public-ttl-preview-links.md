# Plan: public TTL preview links (1.1)

Ships alongside the email sharing in 1.0.0. Goal: the owner clicks "public link"
on a port and gets a **signed, expiring URL** that anyone can open with **no
Google login** — for demos / sharing with people who don't have (or shouldn't
need) an account.

## Model

A capability URL on a wildcard public host: `https://<token>.preview.styrman.xyz/…`.

- `<token>` is an opaque, unguessable id. Each link gets its OWN subdomain, so
  the preview runs on its own origin — which gives cross-origin isolation from
  the styrman SPA **for free** (supersedes the CSP-sandbox workaround for these
  links; the sandbox stays for the logged-in `/p/…` previews).
- The token maps to `{projectID, port, exp}`. Two options, pick at build:
  - **Stateless (HMAC):** token = base64url(payload) + HMAC-SHA256 with a server
    secret. No storage; revocation only by rotating the secret or short TTL.
  - **Stateful (store):** random token → a small `shares/links.json` row
    `{project, port, exp, createdBy}`. Allows explicit revoke + "list active
    links". Preferred — revocation matters for access control.
- TTL: owner picks (e.g. 1h / 24h / 7d / custom). Expired → 410 Gone.

## Ingress (the real work)

Per-host access mode means a public route needs its own host:

1. Wildcard DNS `*.preview.<BASE_DOMAIN>` (Cloudflare, managed by the ingress).
2. Wildcard TLS cert (ACME DNS-01 already used; add the SAN).
3. A `ModePublic` route for `*.preview.<BASE_DOMAIN>` → styrman-server:8080.
   The ingress currently maps one container → one host; add a label convention
   for a second (public) host, or a dedicated tiny route. This is the main
   ingress change.

## Server

- `GET /link/{token}/…` (public; authGuard exempts it like the health route,
  then validates the token itself). Resolves `{project, port, exp}`, checks
  expiry, then proxies exactly like `proxyPort` — same credential strip; the
  separate origin already isolates it.
- Token mint: `POST /api/projects/{project}/ports/{port}/links {ttl}` (owner
  only) → returns the full URL. `GET …/links` lists active, `DELETE …/links/{id}`
  revokes.
- Reuse `previewAccess` owner check for mint/list/revoke; the public GET path
  needs NO identity.

## UI

Extend the per-port share row: alongside the email list, a "Create public link"
control with a TTL picker → shows the URL + copy button + expiry, and a list of
active links each with revoke. Clear labeling that a public link needs no login.

## Security notes

- Token entropy ≥128 bits; constant-time compare (stateful) / HMAC verify.
- Short default TTL; always revocable (favors the stateful model).
- Audit-log mint + revoke (as email shares already do).
- Rate-limit the public host at the ingress (per-IP, as today).
- The wildcard host is PUBLIC — only `/link/{token}` resolves; everything else
  404s, and it never serves the SPA shell.

## Sequencing

1. Server: stateful token store + mint/list/revoke API + public `/link` handler
   (testable locally on the `/link` path before any DNS).
2. Ingress: wildcard DNS + cert + public host route.
3. UI: TTL picker + link management in the port share row.
4. e2e: mint a link, fetch it unauthenticated, let it expire / revoke → denied.
