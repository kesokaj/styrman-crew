---
owner: security
date: 2026-09-11
---
# Security review — browser view, phase 3 (SPA Browser pane + phase-2 round-2 fixes)   (2026-09-11)

Range reviewed: 0e543e3..d239293 (single commit `d239293`), `web/dist` excluded.

Blobs pinned as read (the working tree drifted DURING this review — see
"Working tree drift"; every finding below is stated against these blobs):

| path | blob at d239293 |
|------|-----------------|
| `internal/httpapi/browser.go` | `d0c5126` |
| `internal/httpapi/links_api.go` | `c99e26e` |
| `internal/httpapi/server.go` | `ab694b1` |
| `internal/orchestrator/browserpeer.go` | `baf55f9` |
| `image/configs/styrman-browser` | `091cf5d` (the on-disk file is NOT this blob any more) |
| `image/entrypoint.sh` | `9eec74b` |
| `web/src/components/BrowserPane.tsx` | `5053051` |
| `web/src/lib/cdp.ts` | `3f7d2d6` |
| `web/src/lib/browserInput.ts` | `16927f9` |
| `web/src/lib/editorPane.ts` | `e47aff5` |
| `web/src/components/SidePanelPorts.tsx` | `97aeb6f` |

## Verdict: FINDINGS

**Nothing here crosses a tenant boundary, and the phase-2 high finding is
genuinely closed** — but the relay cap that replaced "no bound at all" made a
per-user problem into a platform-wide one: `browserRelayMax = 32` is a
SERVER-WIDE counter (`browser.go:253,299`), so one signed-in user holding 32
Browser panes open 503s every other user's `/ws/browser/*`, and because the
relay has no ping and no idle deadline (contrast the terminal bridge,
`internal/terminal/bridge.go:341-360`, which pings and marks `browserGone`) a
slot is only released when the read loop ends — a client that vanishes without
a FIN holds its slot until TCP keepalive, hours later. The count ratchets and
nothing logs reaching the cap. The prior review asked for one relay per (user,
project) closed with `CloseReplaced`; the global counter is the one control in
this diff that can hurt a user who did nothing.

On the SPA question the answer is reassuring and worth stating plainly: **a
hostile page rendered inside the guest browser cannot script the SPA.** Every
string the page controls — tab title, target URL, `alert()` text, the launcher
log tail in a 4004 close reason — reaches the DOM only as React children or as
a `title=`/`aria-label` attribute, all escaped; the pane contains no
`dangerouslySetInnerHTML` (the SPA's only one is `FileEditor.tsx:408`,
untouched here) and never puts a CDP-supplied URL into an `href`,
`window.open` or `location`. No CDP reply steers a socket or a REST call
either: the pane makes **zero** REST calls, and `browserSocketUrl`
(`api.ts:878`) is built from the route param plus `location.host`; ids
(`targetId`, `sessionId`) coming back from the guest are only ever
`JSON.stringify`d into the next CDP frame. `normalizeUrl`'s tolerance of
`javascript:`/`data:`/`file:` therefore does not touch styrman's origin — the
navigation happens in the guest — and drops to a low.

## Findings

- **[medium / confidence: high]** `internal/httpapi/browser.go:253` and
  `:299-305` — **the relay cap is global, so one user can deny the Browser
  view to every other user, and leaked slots make that permanent.**
  `browserRelays` is one `atomic.Int32` on the `Server`, checked after
  `getProjectFor` (so unauthenticated callers cannot consume slots — that part
  is right) and released only by the `defer` when `browserSocket` returns,
  i.e. when `relayWebSockets` ends. Scenario, no exploit needed: a user opens
  the Browser pane in a dozen project tabs, sleeps the laptop, and comes back
  the next day — each of those sockets is still counted, because the pane's
  `pump` is blocked in `src.Read(ctx)`, `r.Context()` is no longer cancelled
  once `websocket.Accept` has hijacked the connection, and neither leg sends a
  ping. Repeat across a week of flaky Wi-Fi on a two- or three-user host and
  the 33rd handshake — belonging to somebody else — gets `503 too many browser
  connections` forever, with no log line naming the cause. Availability only;
  no data crosses. Remediation: bound per (user, project) and close the
  previous relay with `config.CloseReplaced`, the pattern
  `internal/terminal/bridge.go:279` already implements; add `conn.Ping` with a
  deadline on the pane leg so a dead client frees its slot; log at Warn when
  the bound is hit.

- **[medium / confidence: high]** `image/configs/styrman-browser:43,139`
  (`CHROME_PORT=9229`, `--remote-debugging-port`) — **the socat peer
  restriction does not cover a container run with `--network=host` inside the
  project's own dockerd, which the launcher's comment implies it does.**
  Chromium's own DevTools listener is unauthenticated on the guest's loopback;
  the `range=<peer>/32` forwarder guards only the bridge door. A container the
  user (or an agent) starts with `--net=host` shares the GUEST's network
  namespace, so it reaches `127.0.0.1:9229` directly, with no peer check, and a
  DevTools client can read `file://` inside the guest — which is exactly the
  brokered credentials (R3), shared across every project that user owns. That
  is a real gain for a pulled image: without `--net=host` it has neither the
  guest's filesystem nor the credentials. The launcher's header says "A
  container the user runs inside this project's dockerd can reach the guest's
  bridge IP but is refused by socat's `range=` — … an image the user merely
  runs must not gain that"; for the host-network case that claim is false. The
  prior report recorded this as an open residual (phase-1 finding 3); this diff
  does not address it and the comment still reads as if it were closed.
  Remediation: prefer `--remote-debugging-pipe` (or a unix socket) and give the
  `mcp` verb the pipe, leaving no TCP listener in the guest netns at all; if
  that is not workable with `chrome-devtools-mcp`, correct the comment and say
  so in the operator guide — an accepted risk that is written down is fine, one
  that reads as mitigated is not.

- **[low / confidence: medium]** `web/src/lib/browserInput.ts:178` —
  **`normalizeUrl` forwards `javascript:`, `data:`, `blob:`, `file:` and
  `view-source:` to `Page.navigate`.** `javascript:alert(1)` falls through to
  `http://javascript:alert(1)` (harmless), but `javascript://%0a<payload>`
  matches the `^[a-z][a-z0-9+.-]*://` branch and is passed through verbatim.
  Nothing in the SPA navigates to it — the target is the guest's Chromium — so
  this is not an XSS in styrman; the residual is a paste-driven self-XSS inside
  a profile ADR-068 D1 says the operator SHARES with the agents: if Chromium
  honours a `javascript:` URL from `Page.navigate` (recent versions reject it —
  hence medium confidence, unverified here), the payload runs in the currently
  loaded page's origin, which may be a site the operator signed into in that
  profile. `file:` is accepted knowingly (the guest user's own files) but also
  lets a pasted `file:///…` path be exfiltrated by a page the same profile
  later loads. Remediation: allowlist `http`/`https`/`about:blank` and refuse
  the rest with a notice; the shortcut cases (`:3000`, bare host) already cover
  every intended use.

- **[low / confidence: high]** `web/src/components/BrowserPane.tsx:218-223`,
  `:224-246`, `:138-142` — **a hostile page drives the operator's pane: it
  writes system-looking notices, grows the tab strip without bound, and sets
  the relay's data rate.** `Page.javascriptDialogOpening` is auto-accepted and
  its `message` is rendered as the pane's status line (`Page alert: …`),
  visually indistinguishable from a styrman message — a phishing surface
  ("session expired, sign in at …"), text-only but in styrman's own chrome, and
  it fires even while the pane is hidden (the route keeps it mounted,
  `p.$project.$term.tsx:212`). `upsert` appends to `tabs` with no cap, and
  `refreshTargets` runs a full `Target.getTargets` on every `loadEventFired`
  and every top-level `frameNavigated`, so a page reloading in a loop turns
  each reload into a round trip through the server's relay. The screencast is
  requested at `everyNthFrame: 1`, 4096×4096, and the ack is sent BEFORE decode
  (`:207`), so an animating page streams as fast as the link allows — with the
  global cap above, one user's busy page becomes everyone's problem.
  Remediation: cap the tab list and coalesce `refreshTargets`; prefix the
  dialog notice with the tab's origin ("localhost:3000 says:") and style it as
  page content; ack after decode or raise `everyNthFrame`.

- **[low / confidence: high]** `web/src/components/BrowserPane.tsx:201-217` and
  `web/src/lib/cdp.ts:134-141` — **CDP payload shapes are cast, never
  checked.** `params as {data, sessionId, metadata}` then `p.metadata.deviceWidth`
  unguarded: a frame event without `metadata` throws inside `handleMessage`'s
  dispatch loop, which has no `try/catch`, so the remaining handlers for that
  message are skipped and the error surfaces as an uncaught exception in
  `ws.onmessage`. `img.src = "data:image/jpeg;base64," + f.data` (`:198`)
  concatenates an unvalidated value into a data URI; the MIME type is fixed and
  the value reaches only `Image`/`drawImage`, so there is no script sink today —
  it is recorded because the pane's only peer is currently Chromium, and the
  cast is what would give way first if that ever stopped being true.
  Remediation: `typeof` guards on `data`/`metadata` before use; wrap each
  handler call in `cdp.ts:140` so one bad payload cannot silence the others.

- **[low / confidence: high]** `internal/httpapi/browser.go:272-277`
  (`sameOriginOrAbsent`) — **host-only comparison; scheme is not checked.**
  What it gets right, checked rather than assumed: `Origin: null` parses to an
  empty `Host` and is refused; an absent Origin passes by design (browsers
  always send one on a WS handshake, curl does not); a guest-controlled preview
  page cannot reach the route from its own origin at all, because
  `previewHostRewrite` (`server.go:530-569`) rewrites EVERY path arriving on a
  token host into `/link/<token>/…`, and from the main host its Origin does not
  match. The residual: `http://crew.styrman.xyz` and `https://crew.styrman.xyz`
  compare equal, so a page served in plaintext on the same name (an MITM on
  :80, or any service ever run there) counts as same-origin; and an
  `Origin: https://host` against a `Host: host:8443` mismatches, which fails
  closed — the safe direction. `websocket.Accept`'s own check has the identical
  shape, so this is a consistency note, not a regression introduced here.
  Remediation: compare scheme+host and derive the expected origin from config
  rather than from the request's own `Host` header.

- **[low / confidence: high]** `image/configs/styrman-browser:84-91`
  (`ensure_forwarder`) — **a running forwarder keeps the first peer it was ever
  given.** The function returns early on `forwarder_up` without comparing the
  peer, so when the server's address on that bridge changes — its container
  restarts and `EnsureSelfAttached` falls back to a dynamic IP because the
  static broker address is taken (`orchestrator.go:645-651`) — socat keeps
  admitting the old /32 and the pane can never connect until someone stops the
  browser, with a 4004 "browser endpoint unreachable" that names nothing.
  Fails closed, availability only. **The working tree already fixes this** (a
  `${DIR}/socat.range` stamp compared on every `ensure_forwarder`, with a
  replace when it differs); that change is uncommitted and outside this range —
  see "Working tree drift". Remediation: the working tree's, plus a log line
  from `BrowserPeer` (next finding) so the cause is visible.

- **[low / confidence: medium]** `internal/orchestrator/browserpeer.go:26-34` —
  **the fallback names an address the server may not hold, silently.** When
  `ContainerInspect(SelfContainer)` errors, or the project network is simply
  absent from its `NetworkSettings.Networks`, `BrowserPeer` returns
  `brokerIPOnBridge` — the *computed* static broker address (subnet
  broadcast-1), with no check that this server actually holds it, and the two
  cases are indistinguishable to the caller. After a dynamic-IP attach that
  address belongs to nobody, so the guest admits a peer that never connects.
  The guest-side `STYRMAN_CRED_BROKER` fallback (`styrman-browser:67-71`) makes
  the very same guess, so the two fallbacks agree and both fail the same way.
  Nothing in this function logs. Remediation: log at Warn when the inspect
  misses (naming the project and the network), and let the guest-side fallback
  be the only place the guess is made.

- **[info]** `image/configs/styrman-browser:72` — the IPv4 check is
  `printf '%s' "$host" | grep -Eq '^…$'`, which is **line**-anchored: a
  multi-line peer value passes if any one line matches, and the whole value
  (newline included) is then emitted into socat's option string, where it fails
  to parse and the forwarder stays down. Only the guest can set such a value
  today, and the outcome is fail-closed. Use `case "$host" in` or
  `[[ $host =~ … ]]` on the whole value.

- **[info]** `internal/orchestrator/browserpeer.go:35-44` — the host-mode branch
  (`SelfContainer == ""`) still returns the bridge **gateway**, so on a
  host-run dev server any local process of any uid matches `range=` and gets
  unauthenticated DevTools on that guest. This is the prior review's medium
  "fails OPEN toward the host", now reduced: the choice is made by the SERVER,
  which knows whether it is containerized, instead of being inferred guest-side
  from an empty env — exactly the remediation asked for. Production sets
  `STYRMAN_SELF_CONTAINER=styrman-server` (`deploy/docker-compose.yml:96`), so
  the branch is dev-only. Worth one line in `docs/deploy/local.md`.

- **[info]** `internal/httpapi/links_api.go:191-196,263-268` — the serve-time
  guard is in and correct, but the **state sweep** the prior report also asked
  for was not done: a stale 9222/9229 link or share record still exists and
  still looks live in the owner's UI while returning 404. No security impact
  (the 404 is unconditional and precedes `serveProxy`); one support ticket.

## Prior findings (2026-09-11 phase 2) — status

| # | phase-2 finding | status at d239293 |
|---|-----------------|-------------------|
| 1 | **high** — `serveLink`/`serveShared` serve a persisted 9222/9229 record with no login | **Resolved** — `previewPortAllowed(l.Port)` now gates both, before `serveProxy` and after the token lookup, with a plain `http.NotFound` (no oracle). Only the startup state sweep is missing (info above). |
| 2 | **medium** — relay follows HTTP redirects (SSRF + WS proxy out of the server's privileged position) | **Resolved** — `browserHTTPClient` (`browser.go:261-266`) returns `http.ErrUseLastResponse`, and it is passed to `websocket.Dial` via `DialOptions.HTTPClient`. Verified in the dependency: `cloneWithDefaults` (coder/websocket@v1.8.15 `dial.go:68-105`) preserves the caller's `CheckRedirect` and converts `Client.Timeout` into a context deadline rather than erroring, so the 10 s timeout is safe. A 3xx now ends the dial with a non-101 status. |
| 3 | **medium** — gateway fallback fails open toward the host | **Mitigated by design change** — explicit `STYRMAN_BROWSER_PEER` from the server; no peer, no forwarder. Residual is dev-only (info above). |
| 4 | **low** — peer string-chopped, unvalidated, lands in socat's option list | **Resolved** — IPv4-literal check; no comma can reach the option list. Line-anchoring nit recorded as info. |
| 5 | **low** — side effects before the origin check | **Resolved** — `sameOriginOrAbsent` is the first statement of `browserSocket` (`:290`), ahead of `getProjectFor`, the attach, the exec and the guest dial. |
| 6 | **low** — no bound on concurrent relays | **Replaced by a different problem** — a global cap, not a per-(user,project) one; see the medium finding above. |
| 7 | **low** — `refuse_links` runs after `mkdir -p`/`chmod` | **Not addressed** — `launch` and `do_start` still create `${DIR}` before/around the check in the same order. Unchanged blast radius (same uid), so not re-listed. |
| 8 | **low** — guest-controlled text into the boot log | **Resolved** — `tr -cd '[:print:]' \| cut -c1-200` on both lines (`entrypoint.sh:160,163`). |

## Checked

- **Secrets** — none added. `git log -p 0e543e3..d239293 -- . ':(exclude)web/dist' | grep -iE 'BEGIN (RSA|EC|OPENSSH|PRIVATE)|api[_-]?key|secret|token ='` → nothing but the existing `X-Styrman-Ingress` naming. No `.env`, `*.pem`, `*.key` or credential JSON touched. The pane surfaces the launcher's log tail (4004 close reason) to the project OWNER only; that text is the owner's own guest output.
- **Injection** — no new shell, SQL or path sink. The launcher passes `$1..$4` as positional arguments to an inner `bash -c`, quoted throughout; the one value that lands inside socat's option list is now IPv4-validated. Server side the exec `Cmd` is a constant plus a fixed verb. SPA side: every CDP frame is `JSON.stringify`d, and the only string concatenation into a URL-shaped value is the fixed `data:image/jpeg;base64,` prefix (low finding above). `browserEndpoint` rewrites the host itself and requires the `/devtools/browser/` path prefix, so the guest's `/json/version` body cannot choose a destination.
- **Authz/authn** — unchanged and correct on all four browser routes: `getProjectFor` resolves the caller's own space, `previewAccess` is never consulted, admin-ness is not consulted. The WS route's Origin check now precedes every side effect; the two POSTs remain covered by `csrfGuard` (`server.go:593-605`). Guest-controlled preview content cannot reach `/ws/browser/*` (see the `sameOriginOrAbsent` finding). `previewPortAllowed` now covers all five paths that can name a port: `proxy.go:49`, `shares.go:117`, `links_api.go:193,265`, plus the Ports-panel filter in `orchestrator/ports.go:145`.
- **Crypto** — nothing hand-rolled, nothing added. No auth, hashing, session, JWT or OIDC code in this diff.
- **Dependencies** — `go.mod`, `go.sum`, `web/package.json` and `web/package-lock.json` are **unchanged** by this range (`git diff --stat` on those paths is empty); no new dependency to justify, and `cdp.ts`'s comment is right that a CDP client needs none. `govulncheck ./...` **run** (go1.25.0, root module): 31 findings — exactly **2 non-stdlib** (`GO-2026-4887`, `GO-2026-4883`, both `github.com/docker/docker@v28.5.2+incompatible`, both still `Fixed in: N/A`, both pre-existing); the other 29 are stdlib against my local go1.25.0 toolchain with fixes ≤ go1.25.13, and `deploy/Dockerfile.server` builds on `golang:1.25.13-trixie`. The vendored `deploy/caddy/ingress` module was **not** scanned here — `deploy/vulncheck.sh` (both modules, pinned builder image) remains authoritative. `npm audit --omit=dev` in `web/` **run**: 1 high, `js-yaml@4.3.1` (GHSA-2883-xcg3-v3hh), reached only as `@tanstack/react-start → start-plugin-core → xmlbuilder2 → js-yaml`, i.e. build-time, not shipped to V8, and untouched by this diff. The image's global `chrome-devtools-mcp` tree still has no lockfile (phase-1 finding 5, open).
- **GCP surface** — none. Self-hosted; no IAM binding, service account, Cloud Run flag, `--allow-unauthenticated` or Secret Manager reference in the diff.

## Working tree drift (not part of the reviewed range)

`image/configs/styrman-browser` changed **on disk during this review** (` M` in
`git status`, alongside `ROADMAP.md`, `README.md`, `specs/styrman/guide.md`,
`internal/orchestrator/fakedocker_test.go`, `web/e2e/run.mjs`, and two
untracked files including `internal/orchestrator/browserpeer_test.go`). The
committed blob `091cf5d` — the one every finding above is stated against, read
back with `git show d239293:image/configs/styrman-browser` — has
`ensure_forwarder` return early on `forwarder_up`. The on-disk version adds a
`${DIR}/socat.range` stamp, compares it, and replaces a forwarder whose peer
changed (credited in its own comment to a "review finding, Phase 3"); that
closes the stale-peer low finding and is covered by the extended
`refuse_links` list. Nothing else in the drift touches a security property I
relied on.

## Not examined

- **No code was run and nothing was built**: no `go test`, no `go vet`, no
  `npm run check`/`vitest`, no `integration.sh`, no `web/e2e/run.sh`, no image
  build, no Kata guest. Nothing here is a claim about live behaviour —
  `verifier` owns `deploy/verify-commit.sh d239293`. In particular the
  `--network=host` reachability of `127.0.0.1:9229` (medium finding) is
  reasoned from the netns semantics, not measured, and ADR-068's amendment
  already records that no inner-container probe was ever run.
- Whether Chromium's `Page.navigate` honours a `javascript:` URL (the low
  finding's medium confidence) — not probed.
- `web/src/lib/*.test.ts` (the new `browserInput`/`cdp`/`editorPane` tests) were
  not read as tests; I read the code they cover. `web/dist` excluded per the
  dispatch, so the EMBEDDED SPA was not compared against `web/src` — a stale
  `dist` would ship different code than reviewed here.
- `chrome-devtools-mcp`'s own source and transitive tree: not fetched, not
  audited (unchanged by this diff).
- The vendored `deploy/caddy/ingress` module: untouched by this range, not
  scanned here.
- Plans, handoffs and the conversation, per the dispatch. Docs in the range were
  read only where they state a security property (ADR-068 + amendment,
  `REQUIREMENTS.md` R8/R12, `.agents/rules/`, `bugs/INDEX.md`).

Verdict: **FINDINGS** — two medium, six low, three info; no high. The phase-2
high is closed and the SPA pane holds up against a hostile page (no HTML sink,
no URL the SPA navigates, no CDP-steerable socket); the thing to fix before
this ships is the server-wide relay cap with no keepalive, which lets one
user's abandoned tabs take the Browser view away from everybody.
