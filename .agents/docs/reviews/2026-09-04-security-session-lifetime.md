---
owner: security
date: 2026-09-04
---
# Security review — ingress session lifetime (ADR-063)   (2026-09-04)

Range reviewed: 24490c9 (HEAD) .. working tree (uncommitted). Files: `deploy/caddy/ingress/generator.go`,
`deploy/caddy/ingress/generator_test.go`, `.agents/docs/architecture/DECISIONS.md` (ADR-063),
`.agents/docs/specs/styrman/deploy.md`, `TODO.md`, plus the two untracked artifacts
`.agents/docs/changes/2026-09-04-session-lifetime.md` and `.agents/docs/testing/2026-09-04-verify-session-lifetime.md`.
Blast radius followed outward: the whole caddy-security portal/authorizer token path
(`greenpau/caddy-security@v1.1.64` + `greenpau/go-authcrunch@v1.1.41`, both read from the module cache),
`internal/httpapi/auth.go`, `internal/userstore/`, `internal/httpapi/proxy.go`, `internal/httpapi/links_api.go`,
`internal/terminal/bridge.go`, `deploy/caddy/ingress/watcher.go`.

## Verdict: FINDINGS

Nothing in the diff is exploitable, and the directive itself is correct — I validated it against the
pinned binary, not from memory. The problem is what the record now claims. ADR-063 says
"Redeploying Caddy still rotates session state (all users sign in again once on the release)". That is
false: `STYRMAN_JWT_KEY` is set in production (verified present, 64 chars, in `styrman-caddy`), which is
documented in `deploy.md` two lines above the new text as making sessions *survive* a redeploy. Combined
with caddy-security's logout, which only deletes cookies and never invalidates the JWT, the eight-hour
`exp` is now the **only** bound on an exfiltrated token and it persists across restarts. An operator who
believes a redeploy flushes sessions has no working emergency revocation lever, and this change is the
moment that belief becomes load-bearing.

## Findings

- **[medium / confidence: high]** `.agents/docs/architecture/DECISIONS.md:2383` (ADR-063 Consequences), reinforcing
  `deploy/caddy/ingress/generator.go:271` — the ADR documents a session-revocation property that does not
  exist. With `crypto key sign-verify {env.STYRMAN_JWT_KEY}` (`generator.go:275`) the signing key is stable
  across restarts, and caddy-security keeps no session or deny list: logout only emits cookie deletions
  (`go-authcrunch@v1.1.41/pkg/authn/handle_http_logout.go:41-45`) and the authorizer validates signature +
  `exp` alone (`pkg/kms/crypto_keystore.go:236-268`). A token copied out of a browser is therefore valid for
  the full 28800 s regardless of logout, redeploy, or restart — where it was 900 s. Exploitable in principle
  by anyone who obtains a token once (malware on a signed-in laptop, a captured `Authorization: Bearer`
  response header from `grantAccess` at `pkg/authn/handle_http_login.go:351`, a debug proxy): the theft
  window widens 32×, and the operator's documented "just redeploy" response does nothing.
  Remediation: correct the ADR consequence line and add to `deploy.md` that rotating `STYRMAN_JWT_KEY` and
  restarting the ingress is the only way to force-expire every session — that is now the incident lever, and
  it should be written where the operator will find it under pressure.

- **[low / confidence: high]** `internal/terminal/bridge.go:226` (`websocket.Accept`), route mounted at
  `internal/httpapi/server.go:284` — a terminal WebSocket is authorized once at the handshake and never
  re-checked; `authGuard`'s per-request `IsAuthorized` (`internal/httpapi/auth.go:66`) never runs again for
  the life of the socket, and nothing closes it on token expiry. Since BUG-001's fix the SPA's expiry reload
  (`web/src/lib/api.ts:306`) forced a reconnect — and thus a fresh userstore check — roughly every 15
  minutes; at 28800 s that incidental re-check becomes ~8 hours. A user removed from the userstore keeps a
  live root-capable shell in their microVM until they reload. Pre-existing in kind (the socket was never
  bounded by the token), widened in practice by this change.
  Remediation: re-check `users.IsAuthorized` on the bridge's existing periodic tick and close with 4000 on
  failure; that makes revocation real for terminals instead of incidental.

- **[low / confidence: medium]** `deploy/caddy/ingress/generator_test.go:387` — the only gate on this directive is
  a substring assertion. Nothing in `deploy/verify-commit.sh` or `.agents/docs/testing/styrman/integration.sh`
  runs `caddy validate`/`caddy adapt` over generated output (grepped; no match). A malformed `crypto` line is
  rejected by the plugin at load, and `noteReloadFailure` (`deploy/caddy/ingress/watcher.go:234-255`) retries a
  rejected load forever — safe on a running Caddy, but on a cold boot the ingress would serve nothing at all.
  This specific directive is fine (I validated it, see Q1), so the risk is the next hand-written directive.
  Remediation: add a `caddy adapt --adapter caddyfile` of the generator's output, inside the pinned image, to
  `verify-commit.sh`.

- **[low / confidence: high]** `.agents/docs/changes/2026-09-04-session-lifetime.md:13` and `:15` — describes the
  value as "an explicit, operator-tunable value" / "configurable via `SessionLifetimeSeconds` in
  `generator.go`". It is a compile-time `const` (`generator.go:243`) with no env override anywhere (grepped
  for `TOKEN_LIFETIME`/`SESSION_LIFETIME`: no hits). Changing it requires rebuilding and redeploying the
  `styrman-caddy` image at a new version. Remediation: say "compile-time constant; requires a caddy image
  rebuild", so nobody plans an incident response around an env var that does not exist.

## Answers

1. **Syntax and OAuth coverage — correct, verified two ways.** Grammar: `caddyfile_authn.go:97` routes `crypto`
   inside `authentication portal` to `parseCaddyfileAuthPortalCrypto` (`caddyfile_authn_crypto.go:29-37`), which
   accepts `args[0] == "default"` and stores the line via `AddRawCryptoKeyStoreConfig`;
   `go-authcrunch/pkg/kms/crypto_keystore_config.go:56-71` parses `crypto default token lifetime <int>` into
   `cfg.TokenLifetime`. Upstream ships the identical line in its own tests and examples
   (`caddy-security@v1.1.64/caddyfile_authn_test.go:41`, `assets/config/Caddyfile:58`). Empirically:
   `caddy validate` and `caddy adapt` inside the pinned `styrman-caddy:1.19.1` (Caddy 2.11.4 + caddy-security
   v1.1.64) returned `Valid configuration` and
   `crypto_key_store_config: {"token_lifetime": 28800, "raw_key_configs": ["crypto key sign-verify {env.STYRMAN_JWT_KEY}"]}`.
   The live production config today carries no `token_lifetime`, confirming the 900 s default ADR-063 describes.
   It applies to Google OAuth tokens, not only local users: the portal mints its own JWT for every backend and
   takes `exp` from `keystore.GetTokenLifetime` at `pkg/authn/handle_http_login.go:268` and `:331`
   (`grantAccess`, the common tail of the OAuth callback via `respond_http.go:62`).

2. **The trade, and the per-request re-check.** `authGuard` calls `s.users.IsAuthorized(email)` on every request
   (`internal/httpapi/auth.go:66`) against live mutex-guarded maps (`internal/userstore/userstore.go:117-129`),
   so a removed user is refused on their next HTTP request — the ADR's central claim holds. Nothing else
   depended on short-lived tokens: the port-preview proxy deletes `Cookie`, `Authorization`,
   `Proxy-Authorization`, `X-Styrman-Ingress` and every `X-Caddy-*` before forwarding
   (`internal/httpapi/proxy.go:171-181`), so the zone-wide cookie never reaches a tenant app; public links
   (`/link/<token>`) are identity-free by design (`auth.go:38-47`); private/shared links re-evaluate the owner's
   share list per request (`links_api.go:226`). One mitigating detail worth recording: `cookie lifetime` is not
   emitted, so `cookie_get.go:42-46` omits `Max-Age` — the access cookie is a browser-session cookie
   (`Domain=styrman.xyz; Path=/; Secure; HttpOnly`, SameSite unset → browser default Lax) and dies when the
   browser closes. The 8 h window is on the JWT itself, not on a cookie left on disk. The WebSocket is the one
   real widening (finding 2).

3. **`STYRMAN_JWT_KEY` and `trust login redirect uri`.** No conflict with the key pinning: the two directives
   land in the same keystore and the ordering is irrelevant — `NewCryptoKeyStore` prepends the default ahead of
   the raw key configs (`crypto_keystore.go:54-57`), and with `JWTKey` empty (dev) `AutoGenerate` applies the
   same lifetime (`:105-106`). The authorization policies get their own keystore with no `token_lifetime`
   (adapt output confirms), but `MaxLifetime` there is only a "key is usable" filter
   (`pkg/authz/validator/validator.go:348`); expiry comes from the token's own `exp` via `jwtlib.Parse`
   (`crypto_keystore.go:300`), so the authorizers do not cap the 8 h token and there is no portal/authorizer
   mismatch. The interaction that does matter is the one in finding 1: pinned key + 8 h `exp` + no revocation
   list. `trust login redirect uri domain suffix <BaseDomain>` is unchanged — the token rides the zone-wide
   cookie, not the redirect URL, so a redirect to a tenant-controlled preview host was already cookie-bearing
   and the proxy strips it; the existing TODO item stands, with an 8 h blast radius instead of 15 min.

4. **Release notes.** (a) The change ships in the **styrman-caddy** image, not the server image — a partial
   deploy silently leaves 900 s in force. (b) Existing sessions are *not* flushed by the redeploy (pinned key);
   everyone does re-authenticate within 15 minutes because their current tokens still carry the old 900 s `exp`,
   so the observable effect matches the ADR while the stated mechanism does not — fix the wording.
   (c) Record `STYRMAN_JWT_KEY` rotation as the only force-expire-all-sessions lever.

## Checked

- **Secrets** — none added. The generator emits `{env.STYRMAN_JWT_KEY}` as a placeholder, never a value;
  `IngressToken` is written literally into the Caddyfile (`generator.go:141,188,458`) but the Caddyfile is
  never logged — `reloadCaddy` logs only the status code (`watcher.go:352`) and the body goes straight to the
  loopback admin API. No secret in the diff, tests, ADR, or changelog. I read env variable *names* only.
- **Injection** — the new line is `fmt.Fprintf(b, "…%d\n", SessionLifetimeSeconds)` on an untyped int constant
  (`generator.go:271`, `:243`). No external input, no parsing boundary, no injection surface. The neighbouring
  `%s` interpolations of `cfg.BaseDomain` are pre-existing and untouched.
- **Authz/authn** — see Q2 and findings 1–2. `authGuard` ordering unchanged; no endpoint bypasses middleware in
  this diff; no client-supplied ID is newly trusted.
- **Crypto** — nothing hand-rolled. The change is a configuration integer handed to the vetted
  caddy-security / go-authcrunch primitives; signing stays HMAC over the pinned env key.
- **Dependencies** — no new dependency. `bash deploy/vulncheck.sh` exit 3: GO-2026-4887 and GO-2026-4883
  (docker/docker) called in both the root module and `deploy/caddy/ingress`, `image/harness` clean. These are the
  pre-existing, no-fix daemon-side findings the script itself documents as accepted per release; unchanged by
  this diff. `npm audit` in `web/`: 0 vulnerabilities (info/low/moderate/high/critical all 0).
  `(cd deploy/caddy/ingress && go test ./...)` passes (`ok … 2.035s`).
- **GCP surface** — none. Self-hosted; `.agents/docs/deploy/gcp.md` is marked not applicable. No IAM binding,
  service account, `--allow-unauthenticated`, Secret Manager, or key file appears in the diff or its reach.

## What was not checked

The root `go test ./...`, `.agents/docs/testing/styrman/integration.sh`, and `web/e2e/run.sh` (verifier's tiers);
a live Google OAuth login against a rebuilt caddy image to observe a real 28800 s `exp` end to end — the
directive was proved by `caddy validate`/`caddy adapt` in the pinned image, not by a signed-in session; the
pre-existing `trust login redirect uri` suffix-matcher label boundary and the `web/src/lib/api.ts`
`encodeURIComponent` item, both already open in `TODO.md` from the 2026-09-03 review.
