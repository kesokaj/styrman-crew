---
owner: security
date: 2026-09-11
---
# Security review — browser view, phase 2 (server routes + CDP relay)   (2026-09-11)

Range reviewed: e7f7857..0e543e3 (the single commit `0e543e3`, HEAD at review start)

Blobs pinned as read (the working tree drifted DURING this review — see
"Working tree drift"; every finding is stated against these blobs):

| path | blob at 0e543e3 |
|------|-----------------|
| `internal/httpapi/browser.go` | `91d41f2` |
| `internal/httpapi/proxy.go` | `920c52a` |
| `internal/httpapi/shares.go` | `957edc8` |
| `internal/httpapi/links_api.go` | `6e1b378` (unchanged by the diff; in the blast radius) |
| `internal/httpapi/server.go` | `7688216` |
| `internal/orchestrator/ports.go` | `548bc59` |
| `internal/config/config.go` | `d2dbcfc` |
| `image/configs/styrman-browser` | `b165e4d` |
| `image/entrypoint.sh` | `1b56329` |
| `image/Dockerfile` | `1c9dde1` |

## Verdict: FINDINGS

**The browser-port refusal was landed on the three routes that PARSE a port and
on none of the two that SERVE one from persisted state.** `previewPortAllowed`
is called from `proxyPort` (`proxy.go:49`) and `sharePort` (`shares.go:115`),
which covers the preview proxy, the share PUT and the link mint. But
`serveLink` (`links_api.go:191`) and `serveShared` (`links_api.go:257`) take
the port from the stored `Link` record — `l.Port` — and call `serveProxy`
without ever consulting `previewPortAllowed`. Every released server before this
commit accepted any port in 1024–65535 at the mint, so a link or share record
naming **9229 (the standard `node --inspect` port, a plausible thing to have
previewed) or 9222** can already exist in `/var/lib/styrman/users/<u>/links.json`
and in `project.SharedPorts`. After this upgrade those records still serve:
`/link/<token>/` is `authGuard`-exempt (`auth.go:38`), so the visitor needs no
Google login at all, and `serveProxy` dials from the SERVER, whose bridge
address is exactly the one peer socat's new `range=` admits. The result is the
phase-1 high finding intact on a path nobody re-checked: unauthenticated,
internet-reachable, full DevTools control of a guest — `file://` reads of the
brokered credentials (R3), which are shared across every project that user
owns. This is the repo's own BUG-035 shape a second time: the mechanism
("9222/9229 never leave the owner") reaches the callers that were edited, not
the callers that actually run on a warm deployment.

Everything else is smaller. The relay's authz is right (owner-only via
`getProjectFor`, never `previewAccess`); its dial target is not steerable by
the guest's `/json/version` *body*; the Origin discipline is correct in both
directions. The residual set is a followed-redirect SSRF out of the server's
privileged network position, side effects that run before the WebSocket origin
check, and two ways the guest-side `range=` control can be weakened or
mis-aimed.

## Findings

- **[high / confidence: high]** `internal/httpapi/links_api.go:191` and
  `:257` (`serveLink`, `serveShared`) — **a pre-existing link or share record
  for 9222/9229 is still served.** Both handlers read `l.Port` out of the link
  store and hand it straight to `serveProxy`; `previewPortAllowed` is never
  called on this path, and `serveProxy` (`proxy.go:81`) has no port rule of its
  own. `createLink` (`links_api.go:66`) and `setShares` now refuse the mint,
  which protects *new* records and is what the new test
  (`browser_test.go:155-178`) and the new integration check exercise — the
  serve path is tested by neither. Scenario, no exploit needed: a user on
  v1.27.x previews `node --inspect` on 9229, mints a public preview link with a
  long TTL and forgets it; the host is upgraded to the browser image; the first
  time anything starts the project's browser (the Browser pane, or an agent's
  first MCP tool call), the old link — a plain URL on the public preview host,
  no login — is a DevTools endpoint. `httputil.ReverseProxy` passes the upgrade
  through and `pr.SetURL` rewrites Host to the guest IP literal, which is
  precisely the form Chromium's anti-rebinding host check accepts. Remediation:
  call `previewPortAllowed(l.Port)` at the top of `serveLink` and `serveShared`
  (404 for the public route, no oracle; 400 for the gated one), **and** sweep
  the persisted state once at startup — drop link records whose `Port` is
  9222/9229 and the same keys from every project's `SharedPorts` — because a
  refusal at serve time still leaves a live-looking link in the owner's UI.
  Add the serve-path case to `TestBrowserPortRefusedOnPreviewSharesLinks`: it
  currently proves only that the port cannot be *minted*.

- **[medium / confidence: high]** `internal/httpapi/browser.go:360` and `:292`
  — **the relay follows HTTP redirects, so a compromised guest chooses where
  the server connects.** `browserEndpoint` uses `http.DefaultClient.Do` (up to
  10 redirects) and `websocket.Dial(dialCtx, wsURL, nil)` falls back to
  `http.DefaultClient` with a `CheckRedirect` that only rewrites the URL scheme
  and returns nil (`coder/websocket@v1.8.15/dial.go:88-102`) — redirects are
  followed there too. What answers on `<guest-ip>:9222` is entirely the project
  user's choice: they have a shell in the VM, can kill the launcher and bind
  their own listener. A `302 Location: http://<anything>/…` therefore turns the
  server into (a) an arbitrary-GET SSRF and (b) a bidirectional WebSocket proxy
  to any host:port the *server* can reach. That network position is strictly
  more privileged than the guest's: the server is joined to **every** project's
  bridge for the preview proxy, and the sandbox firewall (ADR-036/067) that
  confines a guest to the internet only does not constrain it. So a sandbox-mode
  project regains reach into the host's private ranges and other tenants'
  bridges, and the frames come back to whoever holds the pane socket — which
  the same user can be, with any WS client and their own session cookie. The
  comment at `browser.go:348-352` correctly says the guest's *answer* is not
  trusted for WHERE to connect; the redirect is the channel that was not
  considered. Remediation: one dedicated `*http.Client{CheckRedirect: func(*http.Request,
  []*http.Request) error { return http.ErrUseLastResponse }}` for `/json/version`,
  and pass it as `websocket.DialOptions{HTTPClient: …}` for the upgrade; assert
  `resp.Request.URL.Host == host` after the GET.

- **[medium / confidence: medium]** `image/configs/styrman-browser:65-68` —
  **the gateway fallback fails OPEN toward the host.** When
  `STYRMAN_CRED_BROKER` is empty the launcher admits the bridge's default
  gateway, i.e. the HOST's address on that bridge, and every process on the host
  of any uid then passes `range=` and gets unauthenticated full CDP control of
  that guest's browser. The fallback is selected by "the env var is empty", not
  by any assertion that the server is host-run: `brokerIPOnBridge`
  (`orchestrator.go:504-517`) returns "" whenever `SelfContainer` **or**
  `BrokerAddr` is unset, and the env is baked in at container *create* time, so
  a project created in such a window keeps an empty value for its whole life
  while the deployment is otherwise containerized. ADR-068's amendment states
  the fallback is the dev case; the code cannot tell the two apart. Related,
  and fail-closed rather than open: `EnsureSelfAttached` (`orchestrator.go:645-651`)
  falls back to a *dynamic* IP when the static broker IP is taken, after which
  the server's source address no longer matches the `range=` the guest computed
  — the pane simply stops working, with no diagnostic naming the cause.
  Remediation: have the orchestrator set an explicit positive signal (e.g.
  `STYRMAN_BROWSER_PEER=<ip>`) and run no forwarder when it is absent; never
  infer the peer from a route.

- **[low / confidence: high]** `image/configs/styrman-browser:58-70` + `:111` —
  **the peer is string-chopped, never validated, and lands inside a socat
  option list.** `host` is `${STYRMAN_CRED_BROKER#*://}` with the path and port
  trimmed; nothing asserts it is an IPv4 literal. A value containing a comma
  survives into `"TCP-LISTEN:$3,fork,reuseaddr,bind=0.0.0.0,range=$5,max-children=4"`
  as *additional socat options* — a second `range=` in the injected text is the
  control ADR-068 D2 depends on, switched off from inside the string it is
  written in. Today the only party that can supply a hostile value is the guest
  itself (the `mcp`/`start` verbs run from a login shell whose env it owns), and
  a guest that wants an open port can simply bind one — no boundary is crossed,
  which is why this is low. It is recorded because the next caller need not be
  the guest, and because a control that can be disabled by an unvalidated string
  should not be described as "admits ONE peer" without the validation. There is
  no shell injection: `$5` is a single positional argument inside double quotes.
  Remediation: match `^[0-9]{1,3}(\.[0-9]{1,3}){3}$` and run no forwarder
  otherwise.

- **[low / confidence: high]** `internal/httpapi/browser.go:246-300` —
  **every side effect of the WebSocket route runs before the origin check.**
  `csrfGuard` (`server.go:588-597`) exempts GET, and the only origin check on
  this route is inside `websocket.Accept`, which is called *last* (`:300`). By
  then `EnsureSelfAttached` has run, `browserStart` has done a `docker exec` of
  the launcher, and the server has made an HTTP GET plus a WebSocket dial into
  the guest. A cross-origin page loaded by a signed-in styrman user, knowing a
  project UUID, therefore forces a Chromium launch (~250 MB of the guest's
  budget, ADR-068 D7) and a chain of execs in a project it cannot read a byte
  from — Accept refuses the frames, so this is unwanted state and resource
  consumption, not disclosure. Remediation: do the Host/Origin comparison (or
  simply `websocket.Accept`) before `browserStart`, keeping the current
  pre-upgrade HTTP failures for the cases that must stay plain HTTP.

- **[low / confidence: high]** `internal/httpapi/browser.go:246-346` — **no
  bound on concurrent relays.** Each accepted socket costs a `docker exec`
  (serialized in-guest behind `flock -w 20`, so N connections queue for up to
  20 s each while holding a server goroutine and a Docker API call), two
  goroutines, and up to 16 MiB + 1 MiB of in-flight frame buffers; nothing caps
  them per user, per project or globally. `relayWebSockets` itself is clean —
  both pumps are unblocked by the deferred `cancel()` and the explicit
  `Close` calls, so goroutines do not leak — and `SetReadLimit` on both sides is
  the right shape. Combined with the previous finding the fan-out is reachable
  cross-origin. Remediation: one relay per (user, project), replacing the
  previous one exactly as the terminal bridge does with `CloseReplaced`.

- **[low / confidence: medium]** `image/configs/styrman-browser:88-90` and
  `:123-125` — `refuse_links` runs **after** `mkdir -p "${DIR}" "${PROFILE}"`
  and `chmod 700 "${DIR}"`, so those two still traverse a symlinked
  `~/.styrman` or `~/.styrman/browser`; only the later `: > "${LOG}"` is
  genuinely covered. Same uid throughout, so the blast radius is the planter's
  own files (`chmod 700` on a directory it points at) — which is why this stays
  low and why the launcher's comment is right that no privilege boundary is
  crossed. It does not fully deliver what the comment claims. Remediation: call
  `refuse_links` first, then create; or create with plain `mkdir` and refuse a
  `${DIR}` that is not already a real directory.

- **[low / confidence: medium]** `image/entrypoint.sh:163,166` — the new
  `log "  claude mcp add: …"` lines put **guest-controlled text into the boot
  log**. `claude`/`agy` run with `HOME` set to a home the guest owns, so a
  crafted `~/.claude.json` chooses the stderr that gets logged. `log()` is
  `echo "[styrman] $*"` under `#!/usr/bin/env bash`, so backslash escapes are
  not expanded (dash's `echo` would have expanded them) and `tail -1` bounds it
  to one line — but ANSI/terminal control sequences pass through verbatim into
  whatever the operator `docker logs`s, and the line length is unbounded.
  Logging the failure was the right fix for phase-1's silent-retry finding; the
  input is simply not sanitised. Remediation: `printf '%s\n'` and strip control
  characters, the way `guestErrMessage` (`server.go:672`) already does for
  guest text that reaches the API.

- **[info]** `internal/httpapi/browser.go:74-77` — `guestUser` reaches
  `ExecSpec.User` and `HOME=/home/<guestUser>`. Checked rather than assumed:
  `profileStore.setGuestUser` (`profile.go:76-80`) enforces `guestNameRe` plus a
  reserved-name list, and `load` re-validates a hand-edited file, so `root` and
  path-shaped values cannot reach the exec. No finding.

## Prior findings (2026-09-11 phase 1) — status

| # | phase-1 finding | status at 0e543e3 |
|---|-----------------|-------------------|
| 1 | 9222 previewable / shareable / linkable | **Partially resolved** — mint and proxy refuse; the **serve** path does not (high finding above). `parseListeningPorts` also drops both ports from the Ports panel. |
| 2 | `--remote-allow-origins=*` | **Resolved** — flag gone; the relay sends no Origin (`browser.go:287-292`) and the test's fake refuses any Origin. Residual: the refusal is asserted against a *fake*; no check proves the real Chromium refuses an Origin'd upgrade. |
| 3 | no auth on the endpoint; inner container reaches it | **Mitigated, not closed** — `range=<peer>/32` admits one source address. ADR-068's amendment states plainly that no inner-container probe was run. Residuals: the two findings above (unvalidated peer, gateway fallback), and the unchanged fact that anything in the guest's own netns (`docker run --net=host` inside the project's dockerd) still reaches Chromium's 9229 on loopback with no auth. |
| 4 | agents and operator share one browser profile | **Not addressed** (accepted risk, ADR-068 D1). Still worth one line in the operator guide: do not sign in to anything in the Browser pane you would not hand the agent. |
| 5 | `npm install -g` unpinned tree, lifecycle scripts as root | **Partially resolved** — `--ignore-scripts` added and the resolved tree recorded at `/styrman/npm-global.tree.json` (`Dockerfile:117-123`), which makes drift detectable. Still no lockfile and no integrity pin, so two builds of one styrman version can still contain different transitive code. |
| 6 | `mkdir -p` / `chmod` / `: > LOG` follow symlinks | **Mostly resolved** — `refuse_links` added; ordering residual above. |
| 7 | socat `fork` with no `max-children` | **Resolved** — `max-children=4`. Note 4 is tight: a few half-closed relays (there is no idle timeout on the socat children) can lock the pane out until they expire. |
| 8 | registration failure silent, re-seeds every boot | **Resolved** — output is captured and logged; still no cap on consecutive re-seed attempts, which was the smaller half of the finding. |

## Checked

- **Secrets** — none added. No credential, token or key in the diff; no `.env`,
  key or `.tfvars` path touched. The new error bodies carry the launcher's log
  tail (`browser.go:99,115,126`, capped at 2048 bytes) to the project **owner
  only**; that text can contain whatever the guest writes into its own Chromium
  log, which is the owner's own data. `git log -p e7f7857..0e543e3 | grep -iE
  'BEGIN (RSA|EC|OPENSSH|PRIVATE)|api[_-]?key|secret' ` → nothing.
- **Injection** — the launcher stays `set -euo pipefail`, quotes every
  expansion and passes `$1..$5` as positional arguments to the inner `bash -c`
  rather than interpolating them; no shell injection. The one unvalidated value
  is the socat *option* list (low finding above). Server side, no guest or
  client string reaches a shell, a SQL statement or a path join: the exec `Cmd`
  is a constant plus a fixed verb, and the project ID is resolved through the
  owner's own store.
- **Authz/authn** — **correct on all four new routes.** All of them go through
  `getProjectFor` (`server.go:637`), which resolves the caller's own space by
  their slug and does `sp.projects.Get(id)`; `previewAccess` — the function that
  grants a shared Google email or a link visitor access — is never called here.
  A share-list guest is stopped twice: `authGuard` (`auth.go:71`) refuses any
  non-authorized email on a path that is neither preview- nor shared-shaped, and
  `/api/projects/…/browser*` and `/ws/browser/…` are neither; and even past it,
  `space("")` fails `userSlugRe` (`server.go:295`). An admin has no cross-user
  path either — admin-ness is not consulted, and the store is per user. CSRF:
  the two POSTs are covered by `csrfGuard`'s Origin check (`server.go:588`); the
  WS handshake is covered by `websocket.Accept`'s own `authenticateOrigin`
  (`accept.go:228-260`, Origin host must equal `r.Host`), but only after the
  route's side effects — the low finding above.
- **Crypto** — nothing hand-rolled and nothing added: no auth, hashing,
  session, JWT or OIDC code in the diff. Identity remains the ingress header
  plus the constant-time ingress-token compare that already existed.
- **Dependencies** — no Go module change: `go.mod`/`go.sum` are untouched by
  this diff, and `github.com/coder/websocket v1.8.15` was already a direct
  dependency (the terminal bridge). One image-level change:
  `chrome-devtools-mcp@1.9.0` now installs with `--ignore-scripts`.
  `govulncheck ./...` **was run** against an extracted copy of `0e543e3`
  (root module): "31 vulnerabilities from 1 module and the Go standard
  library". Exactly **2 are non-stdlib** — `GO-2026-4887` and `GO-2026-4883`,
  both in `github.com/docker/docker@v28.5.2+incompatible`, both pre-existing
  (`go.mod` untouched by this diff) and both still `Fixed in: N/A`; track a
  Docker SDK bump. The other **29 are stdlib findings against my local
  toolchain** (`go1.25.0`) with fixes at or below go1.25.13, and
  `deploy/Dockerfile.server:9` builds with `golang:1.25.13-trixie` — the
  released binary already has them. I did not run the project's own
  `deploy/vulncheck.sh`, which scans **both** modules (the vendored Caddy
  ingress is a separate module the root scan misses) inside that pinned builder
  image; treat its output as authoritative. `npm audit` was not run and could
  not be: the new package is a global install inside the image with no lockfile
  describing it (that absence is phase-1 finding 5, still open); `web/`'s own
  dependency surface is untouched by this diff.
- **GCP surface** — none. Self-hosted platform (`docs/deploy/gcp.md`: not
  applicable); the diff touches no IAM binding, service account, Cloud Run flag,
  `--allow-unauthenticated` or Secret Manager reference, and adds no service
  account key file.

## Working tree drift (not part of the reviewed range)

`internal/httpapi/browser.go` and `internal/httpapi/browser_test.go` both moved
under this review (` M` in `git status`). At `0e543e3` the refusal is a
package-level `errBrowserPort` built once from `config.BrowserPort`, so both
ports are refused with a message naming 9222; the working tree has inlined the
`fmt.Errorf` so the message names the port asked for, and tightened the test's
assertion to match. **I first read the committed code against the working
tree's test and reported a disagreement; that was wrong and is retracted here.**
The commit is self-consistent: its own test asserts only the substring
"project's browser". Verified by extracting `0e543e3` with `git archive` into
`.agents/scratch/secrev-0e543e3` and running `go test ./internal/httpapi/ -run
TestBrowser` there — `ok … 0.046s`. The drift is a cosmetic improvement and
changes nothing in the findings above.

## Not examined

- Only `go test ./internal/httpapi/ -run TestBrowser` was run, against an
  extracted copy of `0e543e3` (see "Working tree drift"). The full
  `go test ./...`, the separate `deploy/caddy/ingress` module, `npm audit`,
  `integration.sh` and `web/e2e/run.sh` were **not** run here — `verifier`
  owns `deploy/verify-commit.sh 0e543e3`.
- `web/src/components/BrowserPane.tsx` and the rest of the SPA side: not in
  this diff, not read. The relay's frames reach it; its handling of a 4004
  close and of the reason text is unreviewed.
- The runtime image was not built and no Kata guest was booted, so nothing here
  is a claim about the live `range=` behaviour, the inner-container case, or
  Chromium's real Origin refusal. ADR-068's amendment records two host-level
  probes and explicitly records that the inner-container claim was not measured.
- `chrome-devtools-mcp`'s own source and transitive tree: not fetched, not
  audited.
- Plans, handoffs and the conversation, per the dispatch. Docs in the diff were
  read only where they state a security property (ADR-068's amendment, the
  three new integration checks, `specs/styrman/api.md`'s route table).

Verdict: **FINDINGS** — one high, two medium, five low. The high one
(`serveLink`/`serveShared` serve a persisted 9222/9229 record with no login)
must be closed before this server version reaches a host that has ever minted a
preview link, and the fix is worth a state sweep as well as a guard.
