---
owner: security
date: 2026-09-11
---
# Security review — browser view, phase 4 (answers to security round 3)   (2026-09-11)

Range reviewed: `d239293..de3ea47` (single commit `de3ea47`), `web/dist` excluded per the dispatch.

Working tree at review time: clean apart from `.claude/agent-memory/*` (outside
the range). `git rev-parse HEAD` = `96a5f78` — **HEAD is not `de3ea47`**; every
finding below is stated against the blobs at `de3ea47`, pinned here, and the
on-disk files I read were verified to be those blobs:

| path | blob at de3ea47 |
|------|-----------------|
| `internal/httpapi/browser.go` | `e67f603` |
| `internal/httpapi/server.go` | `d71bee3` |
| `internal/orchestrator/browserpeer.go` | `d135a1f` |
| `image/configs/styrman-browser` | `db148ec` |
| `web/src/lib/browserInput.ts` | `9455672` |
| `web/src/components/BrowserPane.tsx` | `e78b418` |
| `web/e2e/run.mjs` | `f631005` |
| `.agents/docs/testing/styrman/integration.sh` | `d8eb83d` |

## Verdict: FINDINGS

**No high, no new tenant-boundary crossing, and the round-3 medium that
mattered — the server-wide relay cap — is genuinely closed**: `relayCounter`
(browser.go:264-297) bounds 8 per user under a global 32, refusals are logged
at Warn with which cap bit, and the 30 s heartbeat (browser.go:432-450) frees a
slot from a pane that vanished without a FIN. The worst thing in the diff is a
**claim that outruns the code**: `browserpeer.go` now refuses to guess the
server's bridge address, and `browser.go:71-73` says "an unknown peer means no
forwarder — the launcher fails closed", but the launcher's own `peer_range`
(`styrman-browser:73-77`) still falls back to `STYRMAN_CRED_BROKER` — a value
that is always present in the guest env (ADR-030) — so end to end the system
opens :9222 to exactly the address the Go change stopped naming. The exposure
is small (only the server and that one project sit on that bridge), but the
security property is now asserted in three comments and held by none of them,
which is how the next change gets built on it. Second, the heartbeat is
one-directional and shares the pane's write lock: it can tear down a *healthy*
relay after 5 s of write-lock contention, and it never detects a dead
*upstream*.

## Round-3 findings — status

| # | round-3 finding | status at `de3ea47` |
|---|---|---|
| 1 | **medium** — global relay cap, no keepalive, leaked slots | **Resolved** (two residuals in the heartbeat findings below). Per-user 8 + global 32 + Warn log + 30 s pane heartbeat. The "one user locks everyone out forever" scenario is gone: a user's ceiling is 8/32 and an abandoned pane frees itself in ≤ 40 s. |
| 2 | **medium** — `--network=host` reaches Chromium's 9229 | **Accepted, reason judged SOUND but understated** — see the `--network=host` info finding below. The launcher header no longer claims mitigation ("KNOWN BOUNDARY"), which was the actual complaint; the follow-up (pipe transport) is named. |
| 3 | **low** — `normalizeUrl` forwards `javascript:`/`data:`/`file:` | **Resolved.** Only `http`, `https` and exactly `about:blank` pass (`browserInput.ts:177-187`); everything else returns null and the pane says so (`BrowserPane.tsx:360-366`). I re-derived the refusal regex by hand against `javascript:alert(1)`, `javascript://%0a…`, `data:`, `file:`, `view-source:`, `blob:`, `ftp://`, `about:settings`, `https:/\/x`, and the `host:port` shapes — all correct, refusals fail closed, and `javascript:1`-style residue degrades to `http://javascript:1` (a host named "javascript", inert). |
| 4 | **low** — a hostile page drives the pane (dialog notice, unbounded tab strip, refresh storm, screencast rate) | **Partially resolved** — dialog `type`/`message` are type-checked and cut to 200 chars (`BrowserPane.tsx:218-225`). Untouched and NOT listed as accepted anywhere: the notice still renders as `Page alert: …` in styrman's own status line with no origin prefix (the phishing half of the finding); `upsert` still appends without a cap (`:228-234`); `refreshTargets` still runs a full `Target.getTargets` on every `loadEventFired` and top-level `frameNavigated` (`:239-251`); screencast is still `everyNthFrame: 1`, 4096×4096, acked before decode (`:139`, `:201-215`). See the pane-steering finding below. |
| 5 | **low** — CDP payload shapes cast, never checked | **Partially resolved** — the screencast frame and the dialog are now guarded. The two `Target.target{Created,InfoChanged}` handlers still dereference `params.targetInfo` *before* `upsert`'s guard runs (`:253-256`), and `cdp.ts:134-141` still dispatches handlers with no `try`, which was the other half of the remediation. See the target-handler finding below. |
| 6 | **low** — `sameOriginOrAbsent` compares host only, not scheme | **Accepted; reason does not answer the finding** — see the Origin finding below. Impact stays low. |
| 7 | **low** — a running forwarder keeps its first peer | **Resolved.** `socat.range` is written, compared on every `ensure_forwarder`, and a differing peer kills and replaces the forwarder (`styrman-browser:99-108`, stamp written at `:107`); the stamp is in `refuse_links` (`:128`) and removed by `do_stop` (`:214`). The second half of the remediation — a log line so the operator can see *why* a pane cannot connect — was not done: `BrowserPeer` still logs nothing, and `browserExec` silently omits the env var when the peer is empty (`browser.go:77-79`). |
| 8 | **low** — `BrowserPeer` names an address the server may not hold | **Partially resolved — the guess moved to the guest, it did not go away.** See the first finding below. |
| 9 | **info** — line-anchored `grep` peer check | **Resolved.** `[[ =~ ]]` with an unquoted, whole-string-anchored ERE (`styrman-browser:78`). Verified empirically in bash: `1.2.3.4` matches; `$'1.2.3.4\nevil'` and `1.2.3.4 x` do not. Residual info: `999.1.1.1` still matches (no octet range check) and reaches socat, which refuses to parse it — fail-closed. |
| 10 | **info** — host-mode branch returns the bridge gateway | **Accepted, dev only; reason sound.** Production sets `STYRMAN_SELF_CONTAINER`; `TestBrowserPeer` pins both branches. Round 3 asked for one line in `docs/deploy/local.md` — not present in this range (`git diff` touches no deploy doc). Unrecorded, not re-listed. |
| 11 | **info** — stale 9222/9229 link/share records still shown while 404ing | **Accepted; reason sound** — the 404 is unconditional and precedes `serveProxy`; cosmetic. |

## Findings

- **[low / confidence: high]** `image/configs/styrman-browser:73-77` +
  `internal/httpapi/browser.go:71-79` + `internal/orchestrator/browserpeer.go:26-37`
  — **the "fails closed" peer is closed only on the Go side; the guest still
  makes the exact guess that was removed, and three comments now state a
  property the system does not have.** `BrowserPeer` returns `""` when the
  self-inspect fails or the server is not on the project network, and
  `browserExec` then omits `STYRMAN_BROWSER_PEER` from the exec env entirely —
  at which point `peer_range`'s `elif [ -n "${STYRMAN_CRED_BROKER:-}" ]` fires.
  That variable is set in every project container (ADR-030,
  `config.go:50`) and a `docker exec` inherits the container's environment, so
  the fallback is not hypothetical: it is the normal path whenever the inspect
  misses. Scenario: the server container restarts, `EnsureSelfAttached` finds
  the static broker address taken and attaches dynamically, a later inspect
  blips — the server passes no peer, the guest computes the broker's static
  address anyway and (per finding 7's new `socat.range` logic)
  *replaces* the working forwarder with one admitting an address the server may
  not hold. Outcome is availability-only today, because R12 leaves only this
  server and this project on that bridge, so nobody else can occupy that
  address. What makes it worth recording is the documentation: `browser.go:71`
  ("an unknown peer means no forwarder — the launcher fails closed"),
  `browserpeer.go:27-29` ("naming the broker's intended address could admit an
  address this server does not hold") and the launcher header (which states the
  fallback *and* then "it fails closed rather than guessing" three lines later)
  now disagree with each other and with the code. Remediation: either delete
  the `STYRMAN_CRED_BROKER` branch of `peer_range` so the server is the single
  source of the peer (matching all three comments), or keep it and correct
  every one of those comments; and log at Warn in `BrowserPeer`/`browserExec`
  when the peer is empty, naming project and network, which is also finding 7's
  missing half.

- **[low / confidence: medium]** `internal/httpapi/browser.go:432-450`
  (heartbeat) — **the ping shares the pane connection's write lock and only
  sees pongs while the pane pump is inside `Read`, so it can close a healthy
  relay; a pane that must then be reconnected by hand.** `pane.Ping` →
  `writeControl` wraps its own 5 s ctx (`coder/websocket@v1.8.15
  write.go:276-293`) and takes `writeFrameMu`; the data pump holds that same
  mutex for the whole flush of a message to the pane — up to
  `browserRelayReadLimit` = 16 MiB. Independently, the pong is only delivered
  by whoever is calling `pane.Read`, i.e. `pump(upstream, pane)`, which spends
  half its time blocked in `upstream.Write`. Scenario: the operator watches an
  animating page (`everyNthFrame: 1`, 4096×4096) over a phone tether; a 5-second
  stall in either direction makes `Ping` return an error, the relay closes the
  pane with `StatusGoingAway`/"no heartbeat", and because the socket had
  already opened, `BrowserPane.tsx:275-296` does *not* auto-retry — the operator
  gets "disconnected" and a Reconnect button, mid-session. Availability/UX only,
  and it is self-limiting (no reconnect storm, so no exec amplification).
  Remediation: give the ping its own write path budget (ping with a timeout
  ≥ the worst-case frame flush, or track liveness from the last successful
  *read* instead of a synchronous ping), and consider lowering
  `browserRelayReadLimit` for this relay — a CDP frame is ~200 KiB, not 16 MiB.

- **[low / confidence: high]** `internal/httpapi/browser.go:432-450` — **the
  heartbeat is one-directional: nothing detects a dead upstream, so the slot
  leak the fix targeted still exists in its other half.** Only `pane.Ping` is
  sent; the guest leg is never probed and has no idle deadline. Scenario: the
  microVM's socat forwarder or Chromium dies in a way that leaves the TCP
  connection open (guest kernel wedged, `kill -STOP`), while the operator's
  laptop keeps answering pings from a tab left open in the background. The
  relay lives, the slot stays taken, and the pane shows a frozen last frame
  with no error — the user has no signal that the browser is gone. One of the
  8 per-user slots per such event. Remediation: ping the upstream on the same
  ticker (or set a read deadline on the upstream leg), and treat its failure as
  `CloseBrowserUnavailable` so the pane says "browser disconnected" rather than
  showing a stale frame.

- **[low / confidence: high]** `internal/httpapi/server.go:1137-1148`
  (`listTerminals` degradation) — **a persistent guest-scan failure is now
  invisible to the owner and reads as "everything is paused", and the one
  destructive path that does not fail closed is `deleteAllTerminals`.** The
  change itself is right for the transient case it was written for (BUG-039,
  the 30–50 s rebuild window) and it *improves* one thing: the raw error, which
  used to be written into the 500 body, now goes only to the log. Two residuals.
  (a) The failure has no expiry and no client-visible marker: if the scan fails
  for a lasting reason, the sidebar shows every terminal as "paused"
  (`TerminalList.tsx:99-118`), hides the Pause control, and drops the bell flag
  — the operator can conclude an agent session ended when it is still running
  inside the VM holding brokered credentials. (b) The single delete fails
  closed (`server.go:1259-1262`: `KillSession` error → 500, record kept), but
  "close all" does not — `deleteAllTerminals` (`:1277-1282`) logs the kill
  failure and deletes the record regardless. A user who sees a sidebar full of
  false "paused" entries is exactly the user who reaches for "close all", and
  in the scan-failure window that leaves live tmux sessions with no record,
  hence no UI handle and no further way to kill them from the SPA. The
  `deleteAllTerminals` behaviour is pre-existing; what is new is a UI state that
  invites it. Remediation: carry the degradation to the client (a
  `liveUnknown: true` field, rendered as "state unknown" rather than "paused"),
  and make `deleteAllTerminals` stop on a kill failure the way the single
  delete already does.

- **[low / confidence: high]** `web/src/components/BrowserPane.tsx:253-256` and
  `web/src/lib/cdp.ts:134-141` — **two target handlers still dereference
  guest-controlled `params` before any guard, and one throwing handler still
  silences the rest.** `upsert((params as { targetInfo: TargetInfo }).targetInfo)`
  reads `.targetInfo` off `msg.params`; a `Target.targetCreated` event with no
  `params` (or `params: null`) throws a TypeError inside the `for (const fn of
  set)` loop, which aborts the remaining handlers for that message and surfaces
  as an uncaught error in `ws.onmessage` — the e2e's own "no uncaught client
  errors" gate would catch it, which is the point: it is reachable by anything
  holding the guest's DevTools port (an in-guest process, an MCP-driven agent,
  or the `--network=host` case below), not just by Chromium. Remediation: the
  round-3 remediation, unchanged — guard the two dereferences the way
  `upsert`'s body already does, and wrap `fn(msg.params, sid)` in a try so one
  bad payload cannot take the others with it.

- **[low / confidence: high]** `web/src/components/BrowserPane.tsx:139`,
  `:218-234`, `:239-251` — **the page-steers-the-pane half of round-3 finding 4
  is neither fixed nor recorded as accepted.** The dialog notice is now length-
  capped but still rendered as `Page alert: <text>` in styrman's own status
  line with no origin prefix: a page loaded in the shared profile can write
  "Session expired — sign in again at …" into what looks like a platform
  message, text-only, and it fires while the pane is hidden because the route
  keeps it mounted. `upsert` still grows `tabs` without a cap, and
  `refreshTargets` still issues a `Target.getTargets` round trip through the
  relay on every load and every top-level navigation, so a page reloading in a
  loop turns each reload into relay traffic. Remediation: prefix the notice
  with the tab's origin and style it as page content; cap the tab list;
  coalesce `refreshTargets` behind a short timer.

- **[low / confidence: medium]** `internal/httpapi/browser.go:315-322`
  (`sameOriginOrAbsent`, unchanged) — **accepted with a reason that answers a
  different question.** The phase-4 note justifies it as "the ingress
  terminates TLS, so the server never sees the scheme" — true of the *request's
  own* scheme, but the check compares the **Origin header's** host while
  ignoring the Origin header's scheme, which the server does see. So
  `Origin: http://crew.styrman.xyz` is accepted on the TLS-terminated host.
  Exploiting it needs a page served in plaintext on that exact name (an MITM on
  :80, or any future service there), so impact stays low and unchanged from
  round 3 — but the recorded reason should not be relied on next time.
  Remediation (two lines): compare scheme+host against an origin derived from
  config.

- **[low / confidence: high]** `.agents/docs/testing/styrman/integration.sh:26-31`
  — **the new port guard reports "not bound" when it cannot look, which is the
  failure mode BUG-040 was about.** `ss -ltn 2>/dev/null | awk … | grep -qE` is
  an unverified instrument: if `ss` is absent from the runner's PATH (it is in
  `iproute2`, not guaranteed in a minimal container or a `PATH`-trimmed
  environment), stderr is discarded, the pipeline yields nothing, `grep` fails,
  and the suite proceeds exactly as it did before the fix — and the next
  hand-walk's server answers 90 checks again. The project's own AGENTS.md rule
  covers this verbatim: "before trusting a monitor… confirm it can emit at
  all". Two smaller notes on the same lines: `${PORT}` is interpolated
  unquoted into an ERE and later into URLs (a malformed `STYRMAN_ITEST_PORT`
  becomes a regex, not an error), and the check is a TOCTOU — nothing stops a
  process from taking the port between the probe and the server's bind.
  Remediation: fail if `ss` is missing; then assert the port is *ours* after
  startup (probe a nonce endpoint or match the server's own pid via `ss -ltnp`)
  rather than only before it — an "is it mine" check after bind is immune to
  both the missing-tool and the TOCTOU case.

- **[info]** `image/configs/styrman-browser:21-26` (`--network=host`, round-3
  medium 2, accepted) — **the acceptance is honest and I agree with it; the
  wording understates who chooses.** The header no longer claims the peer
  restriction covers it, names `runtime-image.md`, and names the fix (pipe
  transport). Judgement on the reason: "that is the project user's own choice
  inside their own VM" is true of *running* an image with `--net=host`, but the
  consequence — a DevTools client reading `file://` inside the guest, i.e. the
  brokered credentials that ADR-030/032 keep current across **every project
  that user owns** — is not what a user thinks they are consenting to when they
  paste a `docker run --net=host` line from a README. Remediation while the
  pipe transport is pending: say "reads the brokered credentials shared across
  all of your projects" in the accepted-risk text, so the operator's consent is
  to the actual consequence.

- **[info]** `internal/httpapi/browser.go:264-297` (`relayCounter`) — read for
  the growth question the dispatch asked and **clean**: `release` deletes the
  key at the last slot (`:290-295`), so `byUsr` is bounded by concurrent users,
  not by lifetime users; `total` cannot go negative (one `acquire`, one
  deferred `release`, both on the same path); `acquire` runs after
  `getProjectFor`, so an unauthenticated or non-owner caller still consumes no
  slot; the ping goroutine cannot leak past the deferred `cancel()`
  (`:411-412`) and all three channels are buffered, so no pump blocks on send.
  Residual by arithmetic, not a defect: four users at their per-user ceiling
  still exhaust the global 32, so the platform-wide cap remains reachable by a
  small number of ordinary users on a many-user host — worth a note in the
  operator guide if the host ever grows past a handful of people.

## Checked

- **Secrets** — none added. `git diff d239293..de3ea47 -- . ':(exclude)web/dist' | grep -inE 'BEGIN (RSA|EC|OPENSSH|DSA|PRIVATE)|api[_-]?key|password|secret|bearer |authorization:'` → three hits, all benign: two are quotations of the round-3 report's own "Checked" section, one is the string `file:///etc/passwd` in a `normalizeUrl` refusal test. No `.env`, `*.pem`, `*.key`, `*.tfvars` or credential JSON in the changed path list. New log lines carry usernames and project UUIDs only; the `listTerminals` change moves a raw error *out* of the HTTP body and into the log, which is the right direction.
- **Injection** — no new sink. The peer value is the only external string reaching a command line and is now whole-string IPv4-anchored (verified empirically in bash, incl. the multi-line case); socat's option list gets `<ip>/32` and nothing else. The `setsid -f bash -c` inner script still passes everything positionally and quoted. No new SQL/path/template sink; the SPA's only string-into-URL concatenation remains the fixed `data:image/jpeg;base64,` prefix. `integration.sh`'s `${PORT}`-into-regex is test-only and recorded above.
- **Authz/authn** — unchanged on all four browser routes: `sameOriginOrAbsent` is still the first statement of `browserSocket` (`:333`), `getProjectFor` is still the ownership check, `previewAccess` and admin-ness are still never consulted, and the relay slot is taken *after* ownership so the cap cannot be consumed anonymously. `listTerminals` still resolves through `getProjectFor` before anything else; its degradation changes reporting only, never who may see what. `previewPortAllowed` untouched.
- **Crypto** — nothing hand-rolled, nothing added; no auth, hashing, session, JWT or OIDC code in the range.
- **Dependencies** — `go.mod`, `go.sum`, `web/package.json`, `web/package-lock.json` and both ingress-module files are **unchanged by this range** (`git diff --stat` on those paths is empty); no new dependency to justify. `govulncheck ./...` **run** (local go1.25.0, root module): 31 findings, exactly **1 non-stdlib module** — `github.com/docker/docker` (`GO-2026-4887`, `GO-2026-4883`, both still `Fixed in: N/A`, both pre-existing); the remainder are stdlib against my local toolchain, fixed at ≤ go1.25.13, and `deploy/Dockerfile.server` builds on `golang:1.25.13-trixie`. Identical to round 3. `npm audit --omit=dev` in `web/` **run**: 1 high, `js-yaml@4.3.1` (GHSA-2883-xcg3-v3hh), build-time only via `@tanstack/react-start`, not shipped to V8, untouched here. The vendored `deploy/caddy/ingress` module was **not** scanned in this review — `deploy/vulncheck.sh` (both modules, pinned builder image) remains authoritative.
- **GCP surface** — none. Self-hosted; no IAM binding, service account, Cloud Run flag, `--allow-unauthenticated` or Secret Manager reference anywhere in the range.

## Not examined

- **Nothing was built or run from this range**: no `go build`/`go vet`/`go test`, no `npm run check`/`vitest`, no `integration.sh`, no `web/e2e/run.sh`, no image build, no Kata guest. `verifier` owns that; the phase-4 verify log's counts are not re-derived here. In particular the heartbeat's false-positive window is reasoned from `coder/websocket@v1.8.15`'s `writeControl`/`writeFrame` locking, which I read, not measured on a slow link; and the `STYRMAN_CRED_BROKER` fallback (lead finding) is reasoned from the launcher's shell plus Docker's exec-inherits-container-env semantics, not observed in a live guest.
- `web/e2e/run.mjs`'s four new checks were read as test code only (they start a `python3 -m http.server` fixture inside the guest; no server-side surface). I did not assess whether pinning the Browser checks inside the LAN window leaves the sandbox-mode relay path untested — that is a coverage question for `verifier`, not a security property.
- The other Go/TS test files in the range were read for weakened assertions (none found: `TestBrowserPeer` pins both fail-closed branches, the relay-cap test pins cross-user non-interference) but not for completeness. No test exercises the heartbeat path or the `listTerminals` degradation.
- `chrome-devtools-mcp`'s own source and transitive tree: unchanged, not fetched, still lockfile-less (phase-1 finding 5, open).
- `web/dist` excluded per the dispatch, so the embedded SPA was not compared against `web/src`; a stale `dist` would ship different code than reviewed here.
- Plans, handoffs and the conversation, per the dispatch. `.agents/docs/testing/2026-09-11-verify-browser-phase4.md` was read only for the accepted-risk list I was asked to judge.

Verdict: **FINDINGS** — no high, no medium, eight low, two info. The round-3
medium on the relay cap is closed and the URL-bar and fail-closed-peer fixes
are real; what is left is a documented security property (the peer "fails
closed") that the guest launcher quietly undoes, a heartbeat that can drop a
healthy session and never notices a dead guest, and a terminals list that now
hides a lasting failure behind the word "paused".
