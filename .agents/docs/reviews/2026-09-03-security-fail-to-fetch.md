---
owner: security
date: 2026-09-03
---
# Security review — fail-to-fetch / redirect:"manual" session-expiry handling   (2026-09-03)

Range reviewed: b3dad3c..working tree — `web/src/lib/api.ts` (tracked, modified), the new
untracked `web/src/lib/api.session.test.ts`, and the rebuilt `web/dist/client` bundle that
ships with them. Blast radius followed outward: every `api.*` caller that goes through
`request()`, `web/src/lib/hooks.ts` (`usePolled`), the two server-side redirect sites
(`internal/httpapi/server.go:472`, `internal/httpapi/proxy.go:86`) and their route
registrations, the SPA catch-all (`internal/httpapi/server.go:369-400`), the auth guard
(`internal/httpapi/auth.go`), the preview proxy's header/cookie/CSP handling
(`internal/httpapi/proxy.go:132-225`), and the ingress redirect chain
(`deploy/caddy/ingress/generator.go:118-361`). R8 (`REQUIREMENTS.md:55`).

## Verdict: FINDINGS

The change is directionally right — it closes a fail-open path (a followed redirect's HTML
login page could previously be read as a successful JSON response) and it does not weaken
cookie handling — but it widens the "session expired, reload the page" trigger from a
condition that never fired to **any same-origin 3xx on an `/api/*` path**, and the existing
two-reloads-per-minute cap does not survive that widening. `request()` clears the
`sessionStorage` counter on the first `res.ok` response of every page load
(`web/src/lib/api.ts:314-321`), so when only *some* requests redirect while others succeed,
the counter is reset to 1 on every load and the cap never engages: the tab reloads roughly
every 1.2 s, indefinitely. That state is reachable from a plain same-origin link because
`scopeBase()` and the `/api/projects/${id}` templates interpolate the route-supplied project
id into the path without `encodeURIComponent`, and Go's `ServeMux` answers a path needing
cleaning with a 301 (verified empirically, see finding 1). No data exposure, no privilege
change, no attacker-chosen navigation target — `window.location.reload()` is same-origin —
so this is availability, not compromise.

Answers to the four questions, with evidence, are in **Notes** below.

## Findings

- **[medium / confidence: high]** `web/src/lib/api.ts:308` with `web/src/lib/api.ts:314-321`
  — **the reload cap is defeated by partial failure, giving an unbounded reload loop.**
  `res.type === "opaqueredirect"` now classifies *any* same-origin 3xx as a session expiry
  and calls `reAuthOnce()`, which schedules `window.location.reload()` after 1.2 s. The
  cross-reload cap depends on the `reauth` `sessionStorage` record, but line 314-321 removes
  that record on the first `res.ok` response of the page load. `reAuthTriggered` is
  module-scope, so each load produces at most one increment and at most one clear; if any
  request succeeds (the sidebar's `api.listProjects()` poll always does), `attempts` is read
  back as 0, written as 1, and `attempts >= 2` is never reached. Reachable in principle
  without any attacker origin: `project` reaches the path unencoded
  (`api.ts:336-338` `scopeBase`, and `api.ts:357`, `:359`, `:374`, `:399`, `:429`, `:449`,
  `:565`), TanStack Router `decodeURIComponent`s path params
  (`web/node_modules/@tanstack/router-core/dist/esm/new-process-route-tree.js:411`), and Go's
  `ServeMux` 301-redirects a path that needs cleaning — measured directly against a mux built
  from this project's own patterns: `/api/projects//files -> 301
  Location="/api/projects/files"`, `/api/projects/a//b/files -> 301`. A route such as
  `/p/a%2F%2Fb/x` therefore yields one 301 (loop trigger) alongside successful polls (cap
  reset). Confidence on the mechanism is high (read directly); confidence that the `%2F%2F`
  delivery survives Caddy in production is medium — it is proven against the Go mux and the
  loopback dev listener, not against the live ingress. Independently of any crafted URL, this
  makes every future `/api/*` route that legitimately 3xx's (a trailing-slash
  canonicalisation, say) present as "session expired" and reload the app.
  **Resolution:** (a) do not clear the counter within a load in which `reAuthOnce()` has
  fired — better, only clear it once the page has survived some seconds with no re-auth, so
  the counter measures *reloads*, not *responses*; (b) `encodeURIComponent()` the `project`,
  `id`, and `token` interpolations in `api.ts` so a route param cannot reshape the request
  path; (c) narrow the trigger to `res.type === "opaqueredirect"` only for paths the API is
  known never to redirect, or log/telemetry the case so a genuine API redirect is not silently
  reported to the user as an expired session. `web/src/lib/api.session.test.ts` covers only
  the single happy path (one opaqueredirect, one reload) — it does not cover the cap, the
  clear-on-ok interaction, or a non-auth 3xx; a regression test for the loop belongs with the
  fix.

- **[low / confidence: high]** `web/src/lib/api.ts:517-535` — **`uploadFiles` was left on the
  broken check the diff exists to replace.** It still uses the default `redirect: "follow"`
  and still tests `res.redirected && res.url.includes("/oauth2/")`, which the diff's own
  rationale (`api.ts:298-305`) proves cannot fire against a cross-origin chain: the fetch
  rejects with `TypeError("Failed to fetch")` and is swallowed by the `catch` at `:522` as a
  generic `"upload failed"` with status 0. An upload attempted during an expired session
  therefore never triggers `reAuthOnce()` and tells the user nothing useful. This is
  fail-closed (nothing is written, no HTML body is mistaken for success), so it is a gap in
  the fix rather than a hole. **Resolution:** give the upload the same
  `redirect: "manual"` + `res.type === "opaqueredirect"` handling as `request()`.

- **[low / confidence: high]** `web/dist/client/_shell.html` (modified, tracked) — **the
  embedded bundle is half-staged.** The modified shell references
  `assets/index-TFG2hSpN.js` and `assets/routes-Bet_3uwM.js`, both of which are **untracked**,
  while twelve previously-embedded assets are staged for deletion only in the working tree. The
  repo's own rule is to stage by name and never `git add -A`; a commit that names
  `web/src/lib/api.ts` and `_shell.html` alone ships a `go:embed`ed SPA whose entry points do
  not exist in the binary, and the app fails to load entirely. Deploy integrity /
  availability. **Resolution:** commit the whole `web/dist/client` tree (new files and
  deletions) together with the source change, and consider a build check that every asset the
  shell references is present in the embed before the binary is tagged.

- **[low / confidence: medium]** `web/src/lib/api.ts:271-282` — **the `reauth` counter is
  parsed without shape validation.** `const { n, at } = JSON.parse(raw)` trusts `n` as a
  number; a stored `{"n":-1000,"at":<now>}` makes `attempts` negative so `attempts >= 2` never
  holds and the cap is disabled for the rest of the window. Writing it requires same-origin
  script, and the same-origin port preview is sandboxed into an opaque origin
  (`internal/httpapi/proxy.go:206-213`, `Content-Security-Policy: sandbox allow-scripts …`
  with no `allow-same-origin`) while public links live on their own token subdomain, so a
  project's app cannot reach it — hence low. **Resolution:** validate
  `Number.isInteger(n) && n >= 0 && Number.isFinite(at)` before using the record.

- **[low / confidence: medium]** `web/src/lib/api.ts:293-307` — **the hard defaults are
  spread-overridable.** `redirect: "manual"` (and `headers`) are written *before* `...init`,
  so any future caller that passes its own `redirect` silently disables the new behaviour. No
  current caller does (`method`, `body`, and `signal` only). Now that one of those defaults is
  security-relevant, the ordering is a trap. **Resolution:** place `redirect` (and `headers`,
  merged) after the `...init` spread so a caller cannot unset it by accident.

- **[low / confidence: low]** `deploy/caddy/ingress/generator.go:260` — **the trusted
  login-redirect set is the whole DNS zone.** `trust login redirect uri domain suffix
  {BaseDomain} path prefix /` is emitted with `BaseDomain` = the zone (`styrman.xyz`, per
  `deploy/.env.example:8`; the app is `${STYRMAN_HOSTNAME}.${BASE_DOMAIN}`), so every host in
  the zone is a trusted post-login `redirect_url` target — including the unauthenticated
  public-preview wildcard `*.${STYRMAN_PUBLIC_DOMAIN}` (`generator.go:118-149`), which serves
  content a project owner fully controls. **This diff neither introduces nor widens it** — the
  SPA now stops following the chain, and its own reload uses the current same-origin URL — and
  the practical ceiling is a trusted-looking phishing landing page: `proxy.go:169-181` deletes
  `Cookie`, `Authorization`, `Proxy-Authorization`, `X-Styrman-Ingress` and every `X-Caddy-*`
  header before the guest app sees them, and `proxy.go:212` sandboxes the same-origin preview.
  Confidence is low because I did not read caddy-security v1.1.64's suffix matcher to confirm
  it lacks a label boundary (`notcrew.styrman.xyz` string-suffix-matches `crew.styrman.xyz`).
  **Resolution:** narrow the trusted suffix to the app host rather than the zone, and record
  the decision either way.

## Notes — the four questions

1. **Can the reload-on-opaqueredirect be abused?** Not for navigation or data: the only
   action is `window.location.reload()`, which is same-origin and carries no
   attacker-controlled destination. It *can* be driven into an unbounded loop — see finding 1;
   the cap that was supposed to stop this does not hold under partial failure. A response
   from a **proxied project app** cannot reach this code at all: `request()` only ever fetches
   `/api/*`, which the styrman Go server answers itself (even `format`/`check`, which exec in
   the guest, are written by the server), and the preview is served on separate paths/origins
   under a `sandbox` CSP (`proxy.go:206-213`), so guest code can neither emit a 3xx into
   `request()` nor touch the `reauth` counter.

2. **Does `redirect: "manual"` remove protection or change cookie handling?** No, and it adds
   one. `redirect` and `credentials` are independent fetch options; `credentials` is untouched
   and still defaults to `same-origin`, so `/api/*` calls carry the caddy-security cookie
   exactly as before. What it removes is the ability for a followed redirect to land on the
   OAuth portal's HTML and be parsed as a successful API response — the precise fail-open the
   codebase documents at `api.ts:510-514`. `res.status` is 0 on an opaqueredirect and the code
   correctly falls back to 401 (`api.ts:310`) and returns before `res.json()`.

3. **Is any `/api/*` endpoint legitimately redirecting today?** No. The only two
   `http.Redirect` call sites in the tree are `internal/httpapi/server.go:472`
   (`redirectSlash`, registered solely on `GET /link/{token}`, `GET /shared/{token}`,
   `GET /p/{project}/{term}/port/{port}`, `GET /proxy/{project}/{port}` —
   `server.go:296,303,308,309`) and `internal/httpapi/proxy.go:86` (the cold-attach bounce
   inside `serveProxy`, reached only from the port-preview and link handlers). Both are on
   paths a browser navigates to, never on an endpoint the SPA fetches. Unmatched `/api/` and
   `/ws/` paths get a JSON 404 and never fall through to `http.FileServerFS`
   (`server.go:370-373`), so the static handler's own directory/`index.html` redirects are
   unreachable from `/api/*`. The one residual 3xx source on an `/api/*` path is the mux's own
   path-cleaning 301 — finding 1.

4. **Everything else in the blast radius.** Open redirect via `redirect_url`: unchanged by
   this diff; see the low/low finding above. The `sessionStorage` counter: see findings 1 and
   4. The 20 s abort: no adverse interaction — `AbortSignal.timeout(20000)` rejects with a
   `TimeoutError` `DOMException` that never reaches `reAuthOnce()`, and the callers that need
   longer (`startProject`/`stopProject`, 150 s, `api.ts:390`/`:395`) pass `signal` through
   `init` without touching `redirect`. `usePolled` (`web/src/lib/hooks.ts:47-72`) surfaces the
   thrown `ApiError` as `error` text and does not retry faster on failure, so it does not
   amplify the loop beyond the 1.2 s reload cadence.

## Observation (not a finding) — the 900 s token lifetime

`deploy/caddy/ingress/generator.go:257-265` (`writeSecurityBlock`, authentication portal) and
`writeAuthzPolicy` at `:325-361` set no `crypto default token lifetime`; a repo-wide grep of
`deploy/` finds none. caddy-security therefore applies its 900 s default, so a session expires
every 15 minutes — and with this diff that expiry is now *visible*: a toast and a full page
reload, every quarter hour, instead of the silent broken polls it produced before.

The trade-off of lengthening it: a longer lifetime widens the window in which a stolen or
replayed JWT stays valid, and the cookie is scoped to the whole zone (`cookie domain
{BaseDomain}`, `generator.go:259`), so that window applies across every styrman host. Against
that, the lifetime is *not* the revocation control here: `authGuard` re-checks
`s.users.IsAuthorized(email)` against the userstore on every single request
(`internal/httpapi/auth.go:64`), so removing a user takes effect immediately regardless of how
long their JWT lives — the token proves identity, the userstore proves permission, which is
exactly R8's two layers (`REQUIREMENTS.md:55-56`). That materially lowers the cost of a longer
lifetime; the residual risk is theft of a longer-lived cookie, not stale authorization.
Recommendation: set the value explicitly rather than inheriting a vendor default, choose it
deliberately (a working-day 8 h is defensible on this reasoning), and record it in the deploy
spec so it is a decision rather than an accident. Fixing finding 1 first matters more — a
15-minute reload is an annoyance; a 1.2-second reload loop is not.

## Checked

- **Secrets** — the diff introduces none. Scanned the changed source and the rebuilt
  `web/dist/client` bundle for key/token/private-key shapes; the only hex matches were xterm
  glyph tables in `assets/dist-ofy-F-8X.js`. `reAuthOnce`'s toast and the thrown `ApiError`
  carry no server-supplied detail, so nothing from the ingress or the API is echoed into the
  UI or the console. No `.env`-shaped path is touched.
- **Injection** — the parsing boundary here is the request URL. `path`, `key`, and credential
  `name`/`id` are `encodeURIComponent`'d; `project`, terminal `id`, and link `token` are not
  (`api.ts:336-338` and the templates listed in finding 1), which is what makes the mux 301
  reachable from a route param. No SQL surface (label + JSON stores, no database) and the diff
  touches no exec or template path.
- **Authz/authn** — unchanged. No check is added, removed, or reordered; the guard chain
  `previewHostRewrite → authGuard → csrfGuard → mux` (`server.go:404`) is untouched, and
  `redirect: "manual"` does not alter credential handling (see note 2). The only two 401
  responders (`auth.go:61`, `auth.go:92`) are global conditions that fail *every* request at
  once, which is why the cap held for them and fails for the new per-request 3xx class.
- **Crypto** — nothing hand-rolled in or near the diff. Sessions and JWT remain entirely
  inside caddy-security v1.1.64 (`deploy/caddy/Dockerfile:13`,
  `generator.go:246-265`); the signing key is pinned from the environment rather than
  generated in-tree (`generator.go:262-265`). The `reauth` counter is a UX rate-limit on
  reloads, not a security control, and is treated as such above.
- **Dependencies** — no change: `git diff` on `web/package.json`, `web/package-lock.json`,
  `go.mod`/`go.sum`, and `deploy/caddy/ingress/go.mod`/`go.sum` is empty; the new test uses
  vitest, already present. `npm audit` in `web/`: **found 0 vulnerabilities** (also with
  `--omit=dev`). `bash deploy/vulncheck.sh` (govulncheck v1.7.0 inside the pinned
  `golang:1.25.13-trixie` builder, both modules): **2 called vulnerabilities per module —
  GO-2026-4887 and GO-2026-4883**, both docker/docker daemon-side with no fix available, which
  is the accepted set the script's own header documents; exit 0. The host toolchain's bare
  `govulncheck ./...` (go1.25.0) reports 31 findings, which is precisely the toolchain artifact
  `deploy/vulncheck.sh` exists to eliminate — not a real exposure of the shipped binary.
- **GCP surface** — none touched, and none exists for this deployment: styrman is self-hosted
  on the operator's Debian host (`AGENTS.md` Deploy, `.agents/docs/deploy/gcp.md` records GCP
  as not applicable). No IAM binding, service account, Cloud Run flag, Secret Manager
  reference, or service-account key file appears anywhere in the range reviewed.

---

## Round 2 (2026-09-03)

Range reviewed: b3dad3c..working tree — `web/src/lib/api.ts` (tracked, modified) and the
untracked `web/src/lib/api.session.test.ts`, re-read in full against each round-1 finding.
Blast radius re-walked for the parts the new code touches: `usePolled`'s poll cadences
(`web/src/lib/hooks.ts:40-80` and its 20 call sites — every poller fires immediately on
mount, which is what makes the 10 s deferral safe), `csrfGuard`
(`internal/httpapi/server.go:475-515`), and the `web/dist/client` chunk graph. No Go file
and no dependency manifest changed in the round-2 delta (`git diff --name-only`: only
`web/src/lib/api.ts`, `TODO.md`, and three docs files), so round 1's server-side analysis
and its `deploy/vulncheck.sh` result stand unchanged.

### Verdict: FINDINGS — all low; the medium is closed

The reload loop is genuinely bounded now: the counter counts reloads, not responses. The
clear is deferred 10 s *and* guarded by `reAuthTriggered`, and since every `usePolled`
poller issues its first request on mount, the crafted-param 301 lands at t≈0 while the
reload lands at t=1.2 s — the document is gone long before the clear timer could run. The
regression test drives three loads and pins the behaviour (two reloads, the "keeps failing"
toast, counter still present after 20 s); `npx vitest run src/lib/api.session.test.ts` →
**5 passed**, `tsc -b` and `oxlint` → exit 0. What is left is one incompletely-closed low
(a *corrupt* counter, as opposed to a malformed-shape one, still disables the cap and does
so permanently), the still-unstaged `dist` tree, and one deferral whose recorded rationale
understates the residual.

### Per round-1 finding

- **[medium] reload cap defeated under partial failure — CLOSED.** Resolution (a) adopted
  as described. Verified by reading `api.ts:323-340`: the clear is one `setTimeout` per load
  (`reAuthCleared` is set synchronously in the same tick as the `!reAuthCleared` check, so
  concurrent in-flight requests cannot double-schedule) whose body re-tests `reAuthTriggered`
  at *fire* time. Both orderings are covered — re-auth before the ok response (guard returns)
  and ok response before re-auth (navigation at 1.2 s destroys the timer). No reference
  leaks across loads: a real reload tears down the JS realm; in the vitest harness
  `vi.resetModules()` leaves the previous instance's timer pending, but that closure reads
  its *own* module's `reAuthTriggered` (true once that load re-authed), so a stale timer
  cannot clear a live counter — which is exactly what the third load of the "bounds the
  reload loop" test asserts.
  - **Resolution (c) — rebuttal ACCEPTED.** No `/api/*` endpoint redirects today (round-1
    note 3; no Go change since), and a future legitimate 3xx is now bounded to two reloads
    per minute and announced by a toast rather than silently looping. The residual cost is a
    misleading *label* ("session expired") on a legitimate 3xx — a diagnosis wart, not a
    security property.
  - **Resolution (b) — deferral ACCEPTED, recorded rationale NOT accepted as written.**
    Keeping `encodeURIComponent` out of a bug fix and carrying it as a TODO is a fair scope
    call. But the TODO entry's "not exploitable … bounded to two reloads and an error toast"
    describes only the loop, which was one *consequence* of unencoded interpolation, not the
    weakness itself. See the new finding below; the item should be prioritised on the right
    reason.
- **[low] `uploadFiles` left on the broken check — CLOSED.** `api.ts:536-540` passes
  `redirect: "manual"` and `:548` tests `res.type === "opaqueredirect"`, and the non-throwing
  contract is preserved (`saved: []` + an `ApiError`, never a throw), so the callers that
  must act on the saved list are unaffected. Covered by "uploadFiles re-authenticates too".
- **[low] half-staged `dist` — STILL OPEN (deferred to commit time).** The working tree is
  now self-consistent — I checked the whole chunk graph, not just the shell: every
  `assets/*.js` referenced by `_shell.html` *and* by the built chunks resolves to a file that
  exists. But two of the five assets the shell names, `assets/index-BA7F6YGy.js` and
  `assets/routes-PE0_8PtQ.js`, are still untracked, and fourteen old assets are deleted only
  in the working tree. Note the hashes moved again since round 1
  (`index-TFG2hSpN` → `index-BA7F6YGy`, `routes-Bet_3uwM` → `routes-PE0_8PtQ`): the bundle
  was rebuilt between reviews, which is precisely how a partially-staged embed gets shipped.
  The stated plan closes it; it is not closed yet. Re-verify with the same asset-existence
  check *after* staging, against the index rather than the working tree.
- **[low] counter parsed without shape validation — PARTIALLY FIXED; see new finding.** The
  shape check at `api.ts:274-284` is correct and closes the negative-`n` case (a stored
  `{"n":-1000}` is now ignored and the record self-heals to `{n:1}` on the same call, which
  "ignores a malformed counter" pins). A *syntactically* corrupt record is not covered.
- **[low] spread-overridable default — CLOSED.** `redirect: "manual"` now sits after
  `...init` (`api.ts:308-317`), so no caller can unset it. `headers` remains before the
  spread; I accept that as out of scope — it carries only `Content-Type`, no credential
  travels in a header (the ingress cookie rides `credentials`), and no caller passes headers.
- **[low/low] trusted login-redirect suffix = whole zone — ACCEPTED as out of scope.**
  Pre-existing, not widened by this diff, and now recorded in `TODO.md` with the resolution
  and the "not widened by BUG-001" note. Remains open as a project item. The 900 s token
  lifetime observation is likewise recorded as an operator decision with the reasoning intact.

### New in round 2

- **[low / confidence: medium]** `web/src/lib/api.ts:271-292` — **a *corrupt* `reauth`
  record disables the reload cap permanently, and the shape check does not catch it.**
  `JSON.parse(raw)` sits in the same `try` block as the `sessionStorage.setItem` that writes
  the incremented counter. A value that is not valid JSON (`{oops`) throws *before* the
  write, so control jumps to the `catch`, `attempts` stays 0, and — the sticky part — the
  corrupt record is never overwritten. Every subsequent load repeats it: `attempts` is 0,
  `attempts < 2` holds, and `window.location.reload()` fires again, without limit and without
  the 60 s window ever mattering (the freshness test is downstream of the throw). This is a
  strictly worse failure than the negative-`n` case the fix closed, which self-heals on the
  first call. I confirmed the control flow by running the block's exact shape in isolation:
  `attempts = 0 | counter written = false | stored = {oops`. Reachability is the same threat
  model as round-1 finding 4 and is why this is low, not medium — writing the key needs
  script on the app's own origin, and round 1 established that a guest app has none (port
  previews are sandboxed into an opaque origin, `internal/httpapi/proxy.go:206-213`; public
  links live on their own token subdomain). Availability only. **Resolution:** wrap only the
  parse, so a read failure falls through to `attempts = 0` while the write still runs and
  overwrites the bad record — e.g. a small `readCounter()` that returns 0 on any parse or
  shape failure, with the `setItem` outside its `try`. Add the corrupt-record case to
  `api.session.test.ts` next to the malformed-shape one: plant `"{oops"`, assert the record
  becomes `{n:1,…}`.
- **[low / confidence: medium]** `web/src/lib/api.ts:354-357` (`scopeBase`) and the
  `/api/projects/${id}` templates — **unencoded route params let a crafted link retarget a
  UI action to a different endpoint, which is more than the reload the TODO records.** The
  browser resolves `..` segments and treats a `?` inside the segment as the start of the
  query before the request leaves, so a route param is not confined to its slot in the
  template: a link of the form `/p/<crafted>/<term>` can make a button the victim clicks
  issue its POST or DELETE against a *different* `/api/projects/...` path than the one the
  template spells. `csrfGuard` cannot see it — the request genuinely is same-origin, so the
  `Origin`/`Host` comparison at `server.go:490-495` passes — and its body-size class is
  chosen by path *suffix* (`server.go:500-513`), which a reshaped path can also change.
  The ceiling is low and bounded by server-side scoping: identity comes from the ingress
  header, every handler resolves resources under that user, so nothing crosses a tenant
  boundary and no privilege is gained — the victim can only be steered into an action on
  their *own* resources. Confidence is medium: reasoned from the templates and URL
  semantics, not executed (no exploit built, by policy). **Resolution:** unchanged from
  round 1 — `encodeURIComponent()` the `project`, terminal `id`, and link `token`
  interpolations. Keep the deferral if scope demands, but restate the TODO item's rationale:
  it is an input-encoding gap at a request-construction boundary, not merely a reload nuisance.
- **[informational]** `api.ts:290-292` — if `sessionStorage` is unavailable altogether (a
  browser configured to block storage for the site), the `catch` leaves `attempts = 0` on
  every load and the cap is off entirely, so a persistently failing sign-in reloads every
  1.2 s. Pre-existing (2026-08-30), untouched by this diff, and not reachable by any other
  party; noted because the widened trigger makes the no-storage path easier to enter. The
  test's `memoryStorage()` never throws, so this branch is uncovered.

### Checked (round 2 delta)

- **Secrets** — none introduced. The new code adds one numeric constant and a shape check;
  `reAuthOnce`'s two toast strings are static and carry no server-supplied text, so nothing
  from the ingress reaches the UI or the console. The test file contains no credential-shaped
  literal.
- **Injection** — the request-URL boundary is unchanged and still the weak spot: `path`,
  `key`, and credential `name`/`id` are encoded; `project`, terminal `id`, and link `token`
  are not (see the second new finding). No new exec, template, or query surface.
- **Authz/authn** — untouched. No Go file changed; the guard chain
  `previewHostRewrite → authGuard → csrfGuard → mux` is byte-identical, and
  `redirect: "manual"` still does not affect `credentials`, so the session cookie rides
  every `/api/*` call exactly as before.
- **Crypto** — nothing hand-rolled added or changed; sessions and JWT remain wholly inside
  caddy-security. The `reauth` record is still a UX reload limiter, not a security control,
  and is judged as one.
- **Dependencies** — `git diff --stat` on `web/package.json`, `web/package-lock.json`,
  `go.mod`, `go.sum`, and `deploy/caddy/ingress/go.{mod,sum}` is empty; the test uses vitest,
  already present. `npm audit` in `web/`: **found 0 vulnerabilities**. `deploy/vulncheck.sh`
  was **not** re-run and is not stale: no Go source or module file changed in the round-2
  delta, so round 1's result (2 called, unfixable docker-daemon findings per module, exit 0)
  applies to the same code.
- **GCP surface** — none touched and none exists for this self-hosted deployment; unchanged
  from round 1.
