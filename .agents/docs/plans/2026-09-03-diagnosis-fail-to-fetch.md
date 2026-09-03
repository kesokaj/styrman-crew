---
owner: explorer
date: 2026-09-03
---
# Recon — "fail to fetch" after 5-15 minutes (2026-09-03)

## Question
Rank the top 1-3 candidate root causes for every REST-backed view (stats bar,
sidebar, file explorer) showing literal "fail to fetch" text after 5-15
minutes of active use, while the terminal WebSocket keeps working, and F5
fixes it — cover the SPA fetch layer, client-side persisted state, the Go
server, and the vendored Caddy ingress (OAuth session, ingress token, rate
limits).

## Summary
The strongest candidate is an expired caddy-security OAuth session/JWT: once
it lapses, every subsequent `/api/*` fetch is met with a redirect to the auth
portal, and the SPA's `request()` in `web/src/lib/api.ts` has **no try/catch
around the bare `fetch()` call itself** (api.ts:293) — only around JSON body
parsing. A cross-origin/preflight-blocked redirect (every request sets
`Content-Type: application/json`, forcing a CORS preflight — api.ts:294)
makes `fetch()` reject with a raw `TypeError: Failed to fetch` *before* the
existing 401/`res.redirected` handling (api.ts:300-303) ever runs, so the
already-built session-expiry auto-reload (`reAuthOnce`, added 2026-08-30,
commit `19aabf3`) never fires and the raw error message propagates straight
into component state (`FileExplorer.tsx:121`, `hooks.ts:63`) and renders in
the page body. This explains every symptom: simultaneous failure of all
REST-polled views (all hit the same expired-session wall on their next
poll), continued WS activity (WS auth is checked once at handshake, not on
each frame), and F5 fixing it (a top-level navigation follows the redirect
to Google/portal without CORS restrictions and re-establishes the session).
The commit message for `19aabf3` itself documents the *exact* mechanism this
brief flags as still unhandled: "the OAuth portal redirect an XHR can't
complete." The 5-15 minute window is consistent with a short OAuth
access-token/session lifetime (greenpau/caddy-security v1.1.64, vendored
Caddy build — `deploy/caddy/Dockerfile`), but the exact configured lifetime
could not be verified from this repo (no `token lifetime` / `expires_in`
setting is emitted by `generator.go`'s `writeSecurityBlock`, meaning the
plugin's built-in default applies, and that default is not vendored here).

## Relevant files
- `web/src/lib/api.ts:292-300` — `request()`: bare `fetch()` with no
  try/catch around the call itself; the 401/redirect-handling and
  `reAuthOnce()` (line 258-289) only run if `fetch()` resolves to a `Response`.
- `web/src/lib/hooks.ts:47-72` — `usePolled`: catches only rejections from
  the fetcher promise (`e.message` -> `setError`), no distinction between a
  clean HTTP error and a raw network `TypeError`; retries every interval
  regardless, so a persistent server/ingress-side condition reproduces the
  same failure on every poll (explains "stays broken until F5", not merely
  "recovers after one)").
- `web/src/components/FileExplorer.tsx:113-124` — same pattern, hand-rolled
  10s poll (not `usePolled`), renders `error` text directly in the DOM
  (line 317) — this is one of the three visibly affected surfaces.
- `web/src/components/Sidebar.tsx`, `web/src/components/TerminalStatusBar.tsx`
  — other `usePolled` consumers (sidebar project list, stats bar); not fully
  read but same `usePolled` failure path applies.
- `deploy/caddy/ingress/generator.go:236-300` (`writeSecurityBlock`) — OAuth
  identity provider + authentication portal `myportal`; no
  `token lifetime`/`expires_in` override is written, so the caddy-security
  plugin's built-in default session/JWT lifetime governs expiry.
- `deploy/caddy/Dockerfile:1-15` — pins `github.com/greenpau/caddy-security@v1.1.64`;
  its default lifetime is not vendored in this repo (cannot be confirmed here).
- `deploy/caddy/ingress/generator.go:379-445` — per-route `rate_limit` zone
  (`events {RPS} / window 1s`, keyed by `remote_host`); `DefaultRPS = 100`
  (`config.go:63`), overridable via `DEFAULT_RPS` env
  (`docker-compose.yml:48`, unset by default in the template).
- `cmd/styrman/main.go:110-140` — server `http.Server{ReadHeaderTimeout: 10s}`
  only; no `WriteTimeout`/`IdleTimeout` set (default 0, i.e. none) — rules
  out a Go-side write/idle timeout as the cause.
- `.agents/docs/changes/2026-08-27-security-batch-a.md:93-94` — prior note
  that redeploying Caddy rotates caddy-security session state, forcing
  re-login; establishes the session IS finite/rotatable by design.

## Flows
1. SPA mounts a view (sidebar/stats bar/file explorer) → `usePolled` or a
   hand-rolled `setInterval` loop calls `api.<endpoint>()` → `request()` in
   `api.ts` → `fetch(path, { headers: {"Content-Type": "application/json"},
   signal: AbortSignal.timeout(20000) })`.
2. Request crosses the vendored Caddy ingress: `rate_limit` zone check →
   (authenticated apps) `authorize with <policy>_policy` against the
   caddy-security JWT/session cookie → on success, ingress token + identity
   headers (`X-Caddy-Email` etc.) are injected → `reverse_proxy` to the
   styrman container.
3. If the session/JWT has expired, caddy-security's `authorize` directive
   answers with a redirect to `myportal` instead of proxying upstream.
4. Back in the SPA: if `fetch()` *resolves* with that redirect response,
   `request()`'s `res.redirected && res.url.includes("/oauth2/")` check
   (api.ts:300) catches it, calls `reAuthOnce()` (toast + `location.reload()`)
   — the intended, already-built path. If instead the redirect trips CORS
   (cross-origin preflight is forbidden from itself redirecting per the Fetch
   spec, or the final response lacks CORS headers for this now-non-simple,
   `Content-Type: application/json` request) `fetch()` *rejects* with
   `TypeError: Failed to fetch` before a `Response` object ever exists —
   bypassing `reAuthOnce()` entirely. The rejection reaches the calling
   component's `.catch` and is rendered as literal error text.
5. Every other in-flight/next-interval poller across every mounted component
   hits the same expired session, so all REST-backed views degrade at once,
   continuously, without self-healing on schedule.
6. The terminal's WebSocket, already connected before expiry, keeps working
   because the WS transport is authenticated once at handshake — the ongoing
   PTY byte stream (`internal/terminal/`) does not re-validate the HTTP
   session cookie per frame.
7. F5 forces a top-level browser navigation, which — unlike `fetch()` — is
   permitted to follow a cross-origin redirect to Google/the auth portal
   without CORS restriction, silently completes sign-in (already logged into
   Google) and returns with a fresh session cookie, restoring all views.

## Conventions
- `usePolled` (hooks.ts) is the house pattern for REST polling: single
  shared fetcher ref, in-flight de-dup, generation guard against stale
  responses, tab-visibility pause. `FileExplorer` deliberately does NOT use
  it (hand-rolled `setInterval`) — a documented exception, not an oversight
  (comment at FileExplorer.tsx ~123-125), which is worth knowing since it's
  one of the three broken surfaces and doesn't share `usePolled`'s bookkeeping.
- Session-expiry handling is meant to be centralized in `request()`
  (`reAuthOnce`), per the 2026-08-30 audit item — the convention is "one
  place decides what an expired session means," which this bug appears to
  circumvent via a code path (raw `fetch()` throw) that predates or was
  missed by that centralization.

## Risks
- `reAuthOnce`'s cross-reload cap (2 attempts/60s, api.ts:258-289) never
  engages for this failure mode, because `reAuthOnce()` is never called —
  so there is no visible "sign-in keeps failing" escalation, just silent,
  permanent "fail to fetch" until a manual F5.
- `usePolled`'s error state (hooks.ts:61-64) doesn't distinguish a raw
  network exception from an HTTP error, so any fix that special-cases
  "session expired" must still decide what a bare `TypeError` from `fetch()`
  should mean generically (it can also mean an offline network, unrelated to
  auth).
- The rate-limit and Go-timeout candidates were checked and look weak/
  inconsistent with the reported persistence pattern (see Open questions),
  but were not fully ruled out on the production host itself (log access
  wasn't available for this recon).

## Open questions
- The actual configured session/JWT lifetime for `authentication portal
  myportal` (caddy-security v1.1.64 default) is not visible in this repo —
  `writeSecurityBlock` (generator.go:236-300) sets no explicit
  `token lifetime`, only `cookie domain` and the signing key. Confirming the
  plugin's default (commonly short, e.g. ~15 min class of value) against the
  observed 5-15 minute repro window would strengthen or weaken candidate #1;
  this needs either the vendored plugin source (not present) or a live check
  against the running ingress/portal.
- Whether the specific redirect from caddy-security is same-origin (same
  `crew.styrman.xyz` host, different path) or a genuinely cross-origin hop
  (e.g. to `accounts.google.com` directly) determines whether the failure is
  a CORS-preflight-redirect rejection specifically, or some other reason
  `fetch()` fails to complete the redirect — this determines exactly which
  DevTools network signature to expect and wasn't confirmed here (no
  DevTools output was available per the dispatch).
- Whether `DEFAULT_RPS` is overridden to something low in the actual
  production `.env` (not committed) is unknown; if so, ranked candidate
  "rate limiting" rises in likelihood. Falsify/confirm via the deployed
  `.env` or Caddy access logs (429s in the failure window) on the production
  host, which was not accessible from this recon.
- No DevTools console/network capture was available; the literal error
  string ("Failed to fetch" vs. Firefox's "NetworkError when attempting to
  fetch resource" vs. a caught HTTP error rendering `res.statusText`) would
  distinguish the ranked candidates definitively and should be captured live
  before committing to a fix.

## Candidates, ranked

### 1. (Most likely) Expired OAuth/JWT session redirect throws a raw `fetch()` TypeError that bypasses the existing `reAuthOnce` handling
- Evidence: `web/src/lib/api.ts:292-303` has no try/catch around the `fetch()`
  call; all requests set `Content-Type: application/json`
  (api.ts:294), forcing CORS preflight on any cross-origin hop; the commit
  that added the *intended* fix (`19aabf3`, 2026-08-30) literally documents
  "the OAuth portal redirect an XHR can't complete" as the problem it was
  built to catch — implying that failure mode is real and only partially
  covered (the `res.redirected` branch, not a `fetch()`-throws branch);
  matches all four symptom facets (simultaneous multi-view failure,
  WS survives, F5 fixes it, persists until reload not just one interval).
- Falsify: reproduce with DevTools open; if the network tab shows a *completed*
  redirect response (not a failed/red request) at the moment of failure, or
  the console shows an HTTP error rather than `TypeError: Failed to fetch`,
  this candidate is wrong — look instead at candidate 2.

### 2. Session/cookie lifetime is short enough that this is simply routine, frequent expiry hitting a known-incomplete code path
- Evidence: same code paths as #1; `.agents/docs/changes/2026-08-27-security-batch-a.md:93-94`
  confirms sessions are finite/rotatable by design; the 5-15 minute window is
  the class of value typical for OAuth access-token lifetimes, though the
  exact number isn't visible in this repo (open question above).
- Falsify: if the failure recurs on a fixed, much-longer cadence (hours) or
  a fixed short cadence unrelated to session issuance time (e.g. exactly
  every N polls regardless of login time), this points away from session
  expiry and toward a client-side accumulation bug (leaked timers, unclosed
  connections) instead.

### 3. (Least likely) Per-IP Caddy rate limiting from many concurrent pollers
- Evidence: `deploy/caddy/ingress/generator.go:379-445` rate-limits each
  authenticated route at `DefaultRPS` (100/s) per `remote_host`; several
  components poll independently (Sidebar, TerminalStatusBar, FileExplorer,
  SidePanelGit, NotificationBell, settings) which could burst together.
- Falsify (and why it's ranked low): the window resets every 1s, so a burst
  should produce transient, self-healing 429s within seconds, not a
  continuous failure that only clears on full page reload — inconsistent
  with the reported "stays broken for the rest of the session until F5"
  behavior, unless `DEFAULT_RPS` is set unusually low in the (unseen)
  production `.env`. Check Caddy access logs for 429 responses in the
  failure window to confirm or rule out.
