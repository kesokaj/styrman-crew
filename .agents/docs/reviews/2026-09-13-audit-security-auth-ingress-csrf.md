---
owner: security
date: 2026-09-13
---
# Security audit — authentication and authorization end to end (2026-09-13)

Range reviewed: working tree at commit `e56e775708a9062cc6432f5c3278770bed63b685` (clean tree;
`git rev-parse HEAD` = e56e775708a9062cc6432f5c3278770bed63b685). Every citation is pinned to
that commit. This file REPLACES the in-progress stub left by an earlier attempt.

Surface, as dispatched: the vendored Caddy ingress (`deploy/caddy/ingress/`) — what it injects,
what it strips, whether a header can be spoofed, the compose network layout — then the server's
`authGuard` / `csrfGuard` / `sameOriginOrAbsent`, the admin checks, `previewAccess` vs
`getProjectFor`, the authGuard-exempt routes (`/link/`, `/shared/`, health, setup),
`internal/identity`, `internal/userstore`, the WebSocket origin checks and the 60 s revocation
re-check, session expiry, and the loopback dev listener.

## Verdict: FINDINGS

The worst thing here is not in styrman's own code: the generated ingress config trusts a post-login
redirect target by **naive string suffix** on the base domain
(`deploy/caddy/ingress/generator.go:264`, resolved in `go-authcrunch@v1.1.41`'s
`pkg/redirects/redirect_match.go:59-60` as a bare `strings.HasSuffix`). With `BASE_DOMAIN=styrman.xyz`,
a registrable domain such as `evilstyrman.xyz` is a string suffix of it and therefore a *trusted*
login redirect. An attacker-supplied `?redirect_url=` on the auth portal survives Google sign-in and
302s the freshly authenticated victim onto attacker-controlled ground. The session cookie itself does
not follow (browsers scope `Domain=` by DNS label, not by substring), so this is an open redirect and
phishing primitive on the OAuth portal, not a session handoff — medium, not high.

Everything the dispatch asked about *header spoofing* came back clean and, in several places,
better than clean: `config.Load` now refuses to start in header mode without an ingress token
(`internal/config/config.go:226-229`), every generated site block strips client-supplied
`X-Caddy-*` and `X-Styrman-Ingress` **before** injecting the authentic ones, the compose server
publishes no host port, and the port-preview proxy strips `Cookie`/`Authorization`/the ingress
token on the way to a guest. The userstore now refuses to start on a username-slug collision, which
closes the one identity-derivation hazard `DeriveUsername` still carries. No spoofing path found.

## Findings

- **[medium / confidence: high]** `deploy/caddy/ingress/generator.go:264` —
  `trust login redirect uri domain suffix <BASE_DOMAIN> path prefix /`. The library resolves
  `domain suffix` as `strings.HasSuffix(requestHost, cfg.Domain)`
  (`$GOMODCACHE/github.com/greenpau/go-authcrunch@v1.1.41/pkg/redirects/redirect_match.go:59-60`)
  with no label boundary, and `path prefix /` matches every path. Scenario: the attacker registers
  a domain whose name ENDS in the literal base-domain string (for `styrman.xyz`, e.g.
  `mystyrman.xyz`), then sends a victim `https://auth.<BASE_DOMAIN>/?redirect_url=https://mystyrman.xyz/x`.
  `injectRedirectURL` (`pkg/authn/inject_redirect_url.go:60`) accepts it and stores it in the referer
  cookie; after a genuine Google login `handle_http_login.go:377` re-checks with the same matcher and
  sets it as the `Location`. The victim goes through a real, correct-looking
  `auth.<BASE_DOMAIN>` sign-in and lands on the attacker's page. What does NOT leak:
  `AUTHP_ACCESS_TOKEN` (cookie `Domain=` is label-scoped, so `mystyrman.xyz` is not in scope) and the
  OAuth code (already exchanged before this hop). What does: the portal's credibility, and the
  `Referer` of the portal URL. Remediation, one line in the generator: emit a leading dot —
  `trust login redirect uri domain suffix .%s path prefix /` — every real service host
  (`crew.`, `auth.`, `*.public.crew.`, `*.private.crew.`) ends in `.<BASE_DOMAIN>`, the apex is not a
  service host, and `.` restores the label boundary. `domain regex` and `domain exact` are both
  accepted keywords if a tighter list is wanted
  (`caddy-security@v1.1.64/caddyfile_utils.go:27`). Add a generator test asserting the dot.

- **[medium / confidence: high]** `internal/terminal/bridge.go:192-226` (`ServeSetup`) —
  the WebSocket Origin check is the default one inside `websocket.Accept`, which does not run until
  `bridge.go:263`, i.e. AFTER `b.orch.EnsureSetup(...)` at `bridge.go:196` has created and started the
  user's setup microVM (`internal/orchestrator/orchestrator.go:675-...`). `csrfGuard` exempts GET
  (`internal/httpapi/server.go:609`), so nothing upstream checks the origin either. `/ws/setup` takes
  no unguessable parameter. Scenario: a signed-in operator visits any page; that page runs
  `new WebSocket("wss://crew.<BASE_DOMAIN>/ws/setup")`; the browser attaches the session cookie, the
  ingress admits it, authGuard resolves the identity, a Kata microVM boots, and only then does the
  handshake fail 403. No data is read — the damage is a forced state change and a forced resource
  spend, repeatable. The same ordering applies to `bridge.go:166` (`b.ensure` boots a project VM
  before the upgrade), bounded there by needing the project and terminal UUIDs.
  `internal/httpapi/browser.go:344-348` is the correct model: `sameOriginOrAbsent(r)` is the first
  statement in the handler, explicitly "BEFORE any side effect (bridge attach, docker exec, guest
  dial)". Remediation: call the same `sameOriginOrAbsent` at the top of `ServeHTTP` and `ServeSetup`;
  it is already exported-in-spirit and unit-testable.

- **[low / confidence: high]** `internal/terminal/bridge.go:369-387` and
  `internal/httpapi/browser.go:428-437` — the 60 s re-check (`RevocationWatch`, `bridge.go:76-93`)
  asks only `users.IsAuthorized(email)` (`internal/httpapi/server.go:240`). It never asks whether the
  caddy-security session that authenticated the handshake is still valid. The JWT lifetime is
  deliberately 8 h (`deploy/caddy/ingress/generator.go:243,271`; ADR-063), but an UPGRADED socket
  never revisits it: a terminal or CDP relay opened at t=0 keeps streaming PTY bytes and DevTools
  frames indefinitely, for as long as the email stays in the allowlist. Scenario: a browser tab left
  open on an unattended or later-compromised machine retains full shell access long past the point
  the operator believes the session expired. ADR-063 states plainly that the lifetime is not the
  revocation control and that the trade is "how long a stolen cookie stays valid" — this finding is
  that for an open socket the answer is "forever", which is not what 8 h implies. Remediation: carry
  the token's `exp` from the handshake into the session and have `RevocationWatch`'s check also fail
  once it passes; or cap socket lifetime at `SessionLifetimeSeconds` and let the client reconnect
  through a fresh authGuard.

- **[low / confidence: medium]** `deploy/caddy/ingress/generator.go:402-411` — the `caddy_public`
  branch strips `X-Caddy-Email/User/Role/Subject` and `X-Styrman-Ingress` but NOT `Cookie`, while
  `generator.go:263` scopes the session with `cookie domain <BASE_DOMAIN>`, so `AUTHP_ACCESS_TOKEN`
  is sent to every host under the base domain. Scenario: the day any container is labelled
  `caddy_public: 'true'`, it receives every signed-in user's session JWT verbatim and can replay it
  against `crew.<BASE_DOMAIN>`. Latent today — the only labelled container in
  `deploy/docker-compose.yml:122-136` is the ModeAuthenticated styrman server — which is exactly why
  it is worth closing before it is load-bearing. Note the asymmetry with styrman's own preview proxy,
  which DOES strip it (`internal/httpapi/proxy.go:177-182`, "never hand the user's styrman
  session/identity to a project's app"). Remediation: add `Cookie` and `Authorization` to the public
  block's strip list.

- **[low / confidence: high]** `internal/httpapi/auth.go:38`, `internal/httpapi/links_api.go:274-279`,
  `internal/httpapi/shares.go:101-107`, `internal/httpapi/server.go:605` — the authGuard and csrfGuard
  exemptions are raw `strings.HasPrefix` tests on `r.URL.Path`, computed on a path the router may not
  route. I probed this rather than assume it (scratch program against a `ServeMux` shaped like
  `RoutesWithAuth`): a literal `POST /proxy/../api/settings/users` is seen by the guard as exempt
  (`exempt=true`), and `ServeMux` answers **301 to `/api/settings/users`** — a redirect, which the
  client re-issues through the full guard chain, not a route. `%2e%2e` does not decode into a
  segment, so `POST /proxy/%2e%2e/api/settings/users` routes to the proxy handler with
  `project="%2e%2e"`, which `previewAccess` then fails to resolve. So there is **no current bypass**,
  and this is recorded as the hardening note it is: the exemption and the routing decision are taken
  from two different strings, and the only thing between them is `ServeMux`'s redirect-instead-of-route
  behaviour. Additionally, the `/link/` branch calls `next.ServeHTTP(w, r)` with **no identity on the
  context** (`auth.go:44`), so any future exempt handler that reads `identity.FromContext` silently
  gets the zero `User{}` — safe today only because the SPA catch-all refuses it
  (`server.go:498`). Remediation: compare against the cleaned path, and set an explicit anonymous
  `identity.User{}` rather than leaving the context unset.

- **[low / confidence: medium]** `internal/identity/identity.go:14-34` — `DeriveUsername` discards the
  email DOMAIN entirely, maps `.`→`-`, drops every other rune, and truncates at 32 chars. Many distinct
  emails therefore share one slug (`a@gmail.com` and `a@anywhere.else`; `user+tag@` and `usertag@`;
  any two local parts agreeing on 32 characters). The slug keys per-user homes, state dirs and broker
  isolation. This is **already defended**: `userstore.New` refuses to start on a collision among
  authorized emails (`internal/userstore/userstore.go:67-74`), `Store.Add` refuses to add one
  (`userstore.go:141-143`), and authGuard leaves `Username` empty for a non-authorized preview guest
  so a colliding slug cannot satisfy an ownership check (`auth.go:75`, and `previewAllowed`'s
  `u.Authorized &&` in `shares.go:80`). The residual is that the guard lives only in the userstore:
  `STYRMAN_ALLOWED_EMAILS` collisions are caught at boot (fail-closed, good), but any FUTURE path that
  mints a username from an email without going through the store inherits the hazard silently. Worth a
  comment on `DeriveUsername` naming the store as its only collision guard.

## Cleared on inspection

- **Ingress header injection / spoofing.** Every non-public site block strips the client's copy of
  `X-Token-User-{Email,Name,Roles}`/`X-Token-Subject` BEFORE `authorize` runs, then re-derives
  `X-Caddy-*` from the authorizer's output and strips the upstream headers again
  (`generator.go:421-449`). `X-Styrman-Ingress` is stripped unconditionally and re-injected only from
  `cfg.IngressToken` (`generator.go:456-459`, `:184-189`, `:137-142`) — the strip is outside the
  `if token != ""`, so a token-less deploy still cannot relay a forged one. The public preview
  wildcard strips all five identity headers and injects no identity (`generator.go:136-142`).
- **A second path to the server container.** `deploy/docker-compose.yml:116-121` publishes no host
  port for `styrman`; it sits on `styrman-ingress` and is reached by the `network_mode: host` Caddy
  over the bridge. The server DOES join every project bridge for the preview proxy, which is the real
  exposure — and `internal/config/config.go:226-229` now refuses to start in header mode without
  `STYRMAN_INGRESS_TOKEN`, so a project VM hitting `:8080` directly lacks the header and gets 403 at
  `auth.go:54-58`. Compared constant-time (`crypto/subtle`). `/link/` paths get the same check
  (`auth.go:38-43`), which is what keeps a project VM from proxying itself into another project's port.
- **Loopback dev listener, production.** Not set anywhere in the deploy: `grep -rn STYRMAN_DEV_ADDR
  deploy/` finds only the ingress's comparison comment (`deploy/caddy/ingress/cmd.go:148`) and the
  compose's SECURITY note (`deploy/docker-compose.yml:111-115`). It is not in
  `deploy/.env.example`. When it IS set, `config.Load` refuses a non-loopback bind
  (`internal/config/config.go:208-210`), and `AuthMode=none` refuses a non-loopback main listener
  (`:214-216`). Unknown auth modes fail closed twice — at load (`:220-222`, M16) and again in
  `authGuard`'s `else` (`auth.go:88-93`).
- **Caddy admin API.** Pinned to a loopback IP literal, validated at load, refusing a NAME
  (`deploy/caddy/ingress/cmd.go:149-153`, `config.go:65-70`), and re-emitted into every generated
  config so a `/load` cannot move the listener off loopback (`generator.go:205-219`). The comment
  correctly states that `origins` is browser-only and NOT a caller bind.
- **Admin authority.** Admin comes ONLY from `STYRMAN_ALLOWED_EMAILS` via
  `userstore.IsAdmin` (`userstore.go:110-114`) — never from an ingress role header; the server reads
  no `X-Caddy-Role` at all (grep: zero non-test readers). So a guest admitted through the
  ModeAuthenticated ingress block cannot become an admin. `requireAdmin` (`auth.go:100-106`) gates
  `updateRuntime`, `addUser`, `removeUser` (`explorer.go:462,577,599`) and LAN ENABLE
  (`network.go:109`) — disable is deliberately owner-allowed ("tightening must never be refused"),
  which is the right asymmetry.
- **`previewAccess` vs `getProjectFor`.** Every owner-scoped route resolves in the requester's OWN
  store (`server.go:648-655`); only the preview/link paths resolve cross-owner
  (`shares.go:44-72`), and every one of them re-reads the LIVE record before deciding — the
  `previewOwners` memo skips the scan, never the check (`shares.go:50-58`).
  `previewAllowed` (`shares.go:79-84`) requires `u.Authorized` for the owner branch, so an empty
  guest slug can never match. `serveShared` re-checks the CURRENT share list on every request
  (`links_api.go:232`), so revoking an email cuts access immediately and the token never does.
- **Public vs private link kinds.** `serveLink` 404s a private token and `serveShared` 404s a public
  one (`links_api.go:164`, `:217`) — kinds cannot cross, so a token cannot change its auth model by
  being pasted on the other host. `previewHostRewrite` 404s any preview host whose subdomain is not a
  shape-valid token before auth even looks at it (`server.go:562-567`), and orders the two bases
  longest-first so a base that is a suffix of the other cannot shadow it (`server.go:550-552`).
  Tokens are 128-bit `crypto/rand` hex (`internal/links/store.go:96-102`), shape-validated on load
  and on use (`store.go:356-366`).
- **SPA catch-all.** A preview-only guest that falls through to `GET /` gets a JSON 404, never the app
  shell (`server.go:498-501`), and `/api/`/`/ws/` prefixes 404 as JSON rather than serving the shell.
- **Health exemption.** `/api/health` is exact-matched (`auth.go:28`) and only a loopback caller
  triggers the Docker ping (`server.go:1293-1302`) — a project VM on a shared bridge gets a static OK
  and cannot make the server hammer dockerd.
- **CSRF on the JSON API.** `csrfGuard` (`server.go:599-638`) rejects any mutating request whose
  `Origin` does not match `Host`; Caddy preserves `Host` to the upstream, so this holds behind the
  ingress. Absent-Origin passes by design (non-browser clients) — acceptable because every modern
  browser sends `Origin` on cross-site POST, including form submissions and `sendBeacon`. Body caps
  are applied per route.
- **Browser CDP relay.** Owner-only via `getProjectFor`, never `previewAccess`
  (`browser.go:336-349`); origin checked first; the upstream client follows NO redirect, so one
  project cannot steer the relay into another (`browser.go:316-321`, R12); the upstream dial carries
  no Origin, which is what Chromium's DevTools accepts, so a page inside the guest browser cannot open
  the socket itself (`browser.go:409-412`); revocation closes 4003 and wins a racing close
  (`browser.go:495-500`).
- **Preview proxy header hygiene.** `Cookie`, `Authorization`, `Proxy-Authorization`,
  `X-Styrman-Ingress` and every `X-Caddy-*` are deleted before the guest sees the request
  (`proxy.go:177-187`); `Set-Cookie` `Domain=` is stripped so a guest app cannot toss a cookie onto a
  sibling styrman host (`proxy.go:109-120`); `Referrer-Policy: no-referrer` keeps the capability URL
  out of the guest's outbound links; the same-origin path form is CSP-sandboxed
  (`proxy.go:212-220`).
- **Setup flow authz.** `getSetupStatus`/`startSetup`/`harvestSetup`/`finishSetup`/`discardSetup`
  (`internal/httpapi/setup.go:54-140`) all scope to `s.who(r)` — no path parameter, so no cross-user
  reach — and the done-marker write is confined (`setup.go:127`). `?tool=` is strictly allowlisted
  before interpolation into a login shell (`bridge.go:228-239`).
- **`space()` path safety.** `userSlugRe` rejects any non-slug before the value is joined into a
  state path (`server.go:292,306-308`), which also covers a hand-edited `Link.Owner`.
- **Ingress label parsing.** Emails reaching `transform user`/`match email` verbatim are validated
  against a regex that excludes whitespace, braces, quotes and `#` — the bytes that would end a block
  early (`labels.go:132,146-162`); hostnames are reduced to DNS-label-safe runes
  (`labels.go:172-185`); `auth`/`shell` are reserved from non-`styrman-*` containers
  (`labels.go:72-74`).
- **Secrets.** No credential, token or key in tracked source or config. `git ls-files | grep .env`
  returns only `deploy/.env.example`, whose `STYRMAN_INGRESS_TOKEN=` and `STYRMAN_JWT_KEY=` are empty
  with a pointer to `host-setup.sh` for generation. `.gitignore:1-18` covers `.env`, `*.env`, `.env.*`
  with `!.env.example`. Neither the ingress token nor the JWT key is logged: `loadConfig`'s
  `log.Info("config loaded", ...)` (`cmd.go:167-175`) lists domains, admin email, superadmins and RPS
  only. The server logs identity from the context, not the raw header (`links_api.go:140-143`).
- **Crypto.** Nothing hand-rolled on this surface. OAuth/OIDC and session JWTs are entirely
  caddy-security's; the server's only primitive is `crypto/subtle.ConstantTimeCompare` on the ingress
  token (`auth.go:40,55`) and `crypto/rand` for link tokens. `STYRMAN_JWT_KEY` is pinned so tokens
  survive a restart instead of being silently re-minted (`generator.go:272-276`, `:341-343`).

## Checked

- **Secrets** — clean. Only `deploy/.env.example` is tracked, with empty placeholder values; no token
  or key reaches a log line (see "Cleared on inspection").
- **Injection** — clean on this surface. Caddyfile-significant bytes are stripped from every
  label-derived value before it is written into the generated config (`labels.go:132,172-185`); the
  ingress token goes in `%q`-quoted; the only shell interpolations on the auth surface are the
  allowlisted `?tool=` (`bridge.go:228-239`) and the regex-bounded credential host
  (`explorer.go:505,537`).
- **Authz/authn** — three findings above (open redirect, WS origin-after-side-effect, socket outliving
  session expiry) plus two hardening notes (path-prefix exemptions, slug derivation). No spoofing or
  cross-user path found; the ingress-token requirement is now fail-closed at config load.
- **Crypto** — nothing hand-rolled; vetted libraries only. See above.
- **Dependencies** — `bash deploy/vulncheck.sh` (govulncheck inside the pinned
  `golang:1.25.13-trixie` builder, which is what actually ships): root module **2 affected**
  (GO-2026-4887, GO-2026-4883, both `github.com/docker/docker@v28.5.2+incompatible`, both
  `Fixed in: N/A`); `deploy/caddy/ingress` the same 2; `image/harness` clean. A bare local
  `govulncheck ./...` reports 31 instead — that run uses this machine's `go1.25.0` toolchain, NOT the
  pinned 1.25.13 builder, so its 29 extra stdlib hits are an artifact of the local toolchain and are
  NOT in the shipped binary. `npm audit --omit=dev` in `web/`: 1 high, `js-yaml 4.0.0-4.3.1`
  (GHSA-2883-xcg3-v3hh), `npm audit fix` available. All three are outside this surface and belong to
  `2026-09-13-audit-security-web-and-deps.md`; recorded here only as tool output.
- **GCP surface** — not applicable. Self-hosted on the operator's Debian host;
  `.agents/docs/deploy/gcp.md` records the same. No IAM binding, service account, Cloud Run flag or
  service-account key file anywhere on this surface.

## Judged against

- **R8** (`REQUIREMENTS.md:60-61`) — satisfied. The ingress proves identity, the userstore proves
  permission, and the permission check is re-run on every request plus every 60 s on an open socket.
  The one gap against "no unauthenticated exposure" is the open redirect above, which is on the
  portal rather than on styrman's own surface.
- **R12** (`REQUIREMENTS.md:73-...`) — nothing on this surface weakens it. The ingress token is what
  stops a project VM from spoofing `X-Caddy-Email` or riding `/link/` into another project's port
  (`auth.go:32-46`), and the browser relay's no-redirect client closes the other steering path
  (`browser.go:316-321`).
- **ADR-045** — roadmap reset; no auth claim to check against.
- **ADR-052** — privileged tests must not alias production paths. No privileged test was added or
  touched on this surface; not exercised.
- **ADR-064** — the tenant dir is server-only and no guest mounts it. Consistent with what the setup
  path does here: `EnsureSetup` binds `homes/<u>/base` and asserts no symlink component below the
  tenant dir first (`orchestrator.go:684-686`).
- **ADR-067** — the bridge interface prefix IS the network mode. The authz half is correct: enabling
  LAN is admin-only and disabling is owner-allowed (`network.go:106-111`), the decision is taken
  under the project lock against the LIVE bridge, and the reply is initialised fail-closed
  (`network.go:140`). BUG-028/029/031/032's fail-open family does not recur in the code as it stands.
- **`.agents/docs/bugs/INDEX.md`** — read. BUG-001 (session expiry visible to the SPA) is the nearest
  neighbour of the session-expiry finding above; BUG-015 (a request path that makes root work,
  rate-limited by who can REACH it, not who is expected to) is the nearest neighbour of the
  `/ws/setup` finding — same shape, one layer up.

## Not examined

- **Runtime behaviour.** Nothing was executed against a live ingress, a live Caddy, or the production
  host. The open-redirect finding is read from the vendored library's source and the generator's
  emitted line; it was NOT confirmed against a running portal, and no exploit was built. Confirming it
  needs one `curl -I` against `auth.<BASE_DOMAIN>/?redirect_url=...` on a host the operator controls.
- **The operator's actual `BASE_DOMAIN`.** `deploy/.env` is gitignored and was not read. The finding
  is stated as conditional on the base domain being string-suffix-registrable; for `styrman.xyz`
  (the domain `AGENTS.md` names as live) it is.
- **caddy-security / go-authcrunch beyond the redirect matcher.** The OAuth code exchange, JWT minting
  and verification, cookie flags (`Secure`/`HttpOnly`/`SameSite`), and the sandbox/MFA paths were not
  reviewed. They are a third-party auth library and out of the dispatched diff; the project's rule is
  "vetted libraries only", which this satisfies.
- **`deploy/caddy/ingress/cloudflare.go` and `watcher.go`** beyond the `/load` path — DNS record
  management and the Docker event loop were not audited.
- **`internal/credbroker` / `internal/credstore`** — the broker's bridge-position identity model is a
  separate surface with its own file (`2026-09-13-audit-security-credentials-broker.md`).
- **`internal/files` / guest-writable path confinement** — separate surface
  (`2026-09-13-audit-security-guest-paths-files.md`).
- **The web SPA's own session handling** (`web/src/lib/api.ts`, BUG-001's `opaqueredirect` path) and
  the `js-yaml` advisory — `2026-09-13-audit-security-web-and-deps.md`.
- **The root firewall scripts and the runtime image** — `2026-09-13-audit-security-root-scripts-image.md`.
- **Rate limiting as a control.** `rate_limit ... key {remote_host}` is emitted on every block
  (`generator.go:129-135,161-166,394-400`) but its effectiveness (zone sharing, burst behaviour,
  whether `{remote_host}` is the true client under `network_mode: host`) was not tested.
- **Test suites.** `go test ./...`, the ingress module's tests, `web` vitest, `integration.sh` and the
  browser e2e were NOT run — this was a read-only review, and the `verifier` agent owns that tier.
