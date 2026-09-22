---
owner: bugs
date: 2026-09-22
id: BUG-074
status: open
area: deploy/caddy/ingress (config.go, generator.go)
---
# BUG-074: production browser session bounces forever on the second styrman host

## Symptom
2026-09-22, the operator's first browser sign-in to the new GCP host:
`https://crew.gcp.styrman.xyz/` failed with `ERR_TOO_MANY_REDIRECTS` in the
operator's normal browser window (no https padlock shown). The same URL in
an incognito window worked: sign-in completed and creating a project
worked.

## Repro
Not reproduced by an anonymous request — an anonymous `curl` over http and
https produced the identical (correct) redirect chain to Google's sign-in
page on both the GCP host and the NUC, ending at
`redirect_uri=https://auth.gcp.styrman.xyz/oauth2/google/authorization-code-callback`.
A request carrying a single invalid `AUTHP_ACCESS_TOKEN` cookie did not
loop either. The loop only reproduces from a browser that already holds a
valid NUC production session, then visits the GCP host. The GCP host's
ingress log showed it directly: repeated `Successful login` entries from
the `security` logger with the same `session_id`, each followed by
`http.handlers.authentication ... user authorization failed: ... reason:
no token found` from the authorizer.

## Root cause
Both hosts' portals use the same session cookie name, `AUTHP_ACCESS_TOKEN`
(the `AuthCookieName` constant in `deploy/caddy/ingress/config.go` and the
`cookie_name_prefix` `AUTHP`), and the generator scopes the cookie to the
host's own `BASE_DOMAIN` (`deploy/caddy/ingress/generator.go`, the `cookie
domain %s` line). The NUC's cookie domain is `styrman.xyz`; the GCP host's
is `gcp.styrman.xyz`. `styrman.xyz` is a parent of `crew.gcp.styrman.xyz`,
so a browser holding a NUC production session sends the NUC's
`AUTHP_ACCESS_TOKEN` to the GCP host too. The app then sees two cookies of
the same name, gets no token it can validate, and redirects to the portal;
the portal already has its own valid session and redirects straight back —
an endless bounce. An incognito window carries no NUC cookie, which is why
it worked. Nothing is wrong with the GCP host's DNS, certificate, OAuth
client, or configuration — confirmed by reading both hosts' live Caddy
configs through the admin API and by DNS lookups (1.1.1.1, 8.8.8.8, 9.9.9.9
all resolving both hostnames to the VM's own 34.51.142.50) and the
certificate (valid Let's Encrypt, `CN=crew.gcp.styrman.xyz`, issued
2026-09-22 04:22 UTC).

## Fix
Not fixed. Proposed: make the session cookie name unique per deployment
rather than a constant — caddy-security already exposes
`cookie_name_prefix`, currently the fixed string `AUTHP`; deriving the
prefix from the host's `BASE_DOMAIN` would stop the collision. Both the
constant in `deploy/caddy/ingress/config.go` and the public-preview
block's cookie-stripping regex in `deploy/caddy/ingress/generator.go`
(which names the same cookie) must change together. This reaches a live
host only through a release, so a second styrman deployment sharing a
parent domain with an existing one stays affected until then; until it
ships, a browser should hold a session for one styrman host at a time — a
separate browser profile or a private window for the second host. No
host-side workaround exists or is recommended.

## Prevention
A cookie, header, or storage key that identifies a DEPLOYMENT must carry
something unique to that deployment in its NAME, not only in its scope —
scoping a shared name to a domain fails the moment two deployments share a
parent domain. The symptom (an endless redirect between app and portal)
points at neither side, so when two installations of the same platform can
see each other's browser state, test the second one from a browser session
that already holds the first one's, never from a clean one — an anonymous
`curl` cannot reproduce a defect that only exists with an existing session;
the reproduction here needed the operator's own browser.

## Related
BUG-069 and BUG-072 — same bring-up, each a check or a claim that pointed
at the wrong thing before the real cause was found.

## Operator decision, 2026-09-22

"it worked in incognito. That is enough for me. I will not use the local and gcp with the same gmail account."

The operator accepts the constraint and will not run both hosts from one
signed-in browser session. This record stays OPEN but is NOT to be fixed
unasked: the code change it proposes rewrites how the ingress loads config
(it pushes a Caddyfile to the admin API; the portal's cookie name is not
expressible there, so a fix must adapt to JSON and patch it) and touches
authentication on the live production host. Reopen it only if the operator
asks for several hosts under one parent domain in one browser profile.

One precision for whoever reads this next: the collision is a property of the
BROWSER PROFILE, not of the Google account. Two different Google accounts in
the SAME profile still send the same `AUTHP_ACCESS_TOKEN` cookie to both
hosts. What avoids it is a separate profile (or a private window) per host,
which is how a second Google account is normally used anyway.
