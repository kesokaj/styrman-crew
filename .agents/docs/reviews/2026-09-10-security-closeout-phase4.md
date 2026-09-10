---
owner: security
date: 2026-09-10
---
# Security review — close-out phase 4 (terminal re-authorization, item I)   (2026-09-10)

Range reviewed: 7dae197..working tree, restricted to `.agents/scratch/phase4.diff`
(`internal/config/config.go`, `internal/httpapi/server.go`, `internal/terminal/bridge.go`,
new `internal/terminal/reauth_test.go`, `web/src/components/TerminalPane.tsx`, `web/src/lib/api.ts`).
Blast radius followed outward: `internal/httpapi/auth.go`, `internal/identity`, `internal/userstore`,
`internal/config` (AuthMode/DevAddr gates), `deploy/docker-compose.yml`, `deploy/caddy/ingress/generator.go`
(header injection), `github.com/coder/websocket@v1.8.15` close/write semantics.

## Verdict: FINDINGS (all low; no exploitable defect found in the change itself)

The change does what it claims and fails in the safe direction. The email that is re-checked is
the one `authGuard` already validated at upgrade time; a client has no channel to it (the vendored
ingress strips inbound `X-Caddy-Email`/`X-Styrman-Ingress`/`X-Token-User-Email` before setting them,
and header mode refuses to start without the ingress token). `conn.Close` from the watch goroutine
is documented-safe in coder/websocket and cannot leak a byte of terminal output past the close frame.
Nothing here rises above low. What is worth recording is that the mechanism is narrower than the word
"revocation" suggests: removing a user closes their terminals within 60 s but does not stop their
microVM, their tmux sessions, the agents running inside them, or a preview-proxy WebSocket, and the
env-derived admin allowlist cannot be revoked at all without a restart. That gap belongs in the docs,
not in more code.

## Findings

- **[low / confidence: high]** `internal/terminal/bridge.go:380` — the re-check is a *terminal*
  revocation, not an access revocation, and nothing says so. When `users.Remove` drops an email,
  `authGuard` refuses every REST/WS request on the next request (`internal/httpapi/auth.go:64`,
  evaluated per request — no session cache), and this watch closes each open terminal within 60 s.
  But the project microVM keeps running, the tmux sessions keep running, whatever `claude`/`agy`
  process was mid-task keeps executing with network egress, and the credential broker
  (`internal/credbroker`, bridge-only, caller identified by bridge IP, no userstore consult)
  keeps serving that user's current credentials into the VM. Exploitable in principle only by a
  user who is being removed *while* something is running — they lose the window into the VM, not
  the VM. Remediation: no code change; state the boundary in `.agents/docs/specs/styrman/guide.md`
  (and the Settings → Users copy) — "removing a user cuts their access within a minute; stop or
  delete their projects to stop their compute" — or, if operators expect otherwise, have the
  remove handler enqueue a stop of that user's projects.

- **[low / confidence: high]** `internal/userstore/userstore.go:157` — `Remove` refuses to revoke
  an env-derived (`STYRMAN_ALLOWED_EMAILS`) email, which is exactly the admin set. So the new watch
  can never fire for an admin: revoking an admin still requires editing compose env and restarting
  the server, and until that restart their terminals are re-checked against a store that keeps
  answering `true` (`IsAuthorized`, line 124). Pre-existing and deliberate (the env set is the
  can't-lock-yourself-out floor), but it means "re-checked every 60 s" is only true for the
  runtime-added half of the allowlist. Remediation: none required; note it beside the finding above
  so nobody believes the timer covers admins.

- **[low / confidence: medium]** `internal/httpapi/server.go:222` — `stillAuthorized` is selected
  from `cfg.AuthMode`, while the auth mode actually in force is per-listener
  (`RoutesWithAuth("none")` for the loopback dev listener, `cmd/styrman/main.go:149`). The two
  disagree whenever `STYRMAN_DEV_ADDR` is set on a header-mode server. Today the disagreement is
  benign-and-fail-closed: `cfg.DefaultUser()` returns `AllowedEmails[0]`, which is env-derived and
  therefore always authorized. But with `STYRMAN_ALLOWED_EMAILS` empty and users.json non-empty —
  a configuration `cmd/styrman/main.go:52` explicitly permits, since it checks `users.List()` and
  not `AllowedEmails` — `DefaultUser()` returns `""` (`internal/config/config.go:401`),
  `IsAuthorized("")` returns false (line 119), and every dev-door terminal is killed after 60 s
  with 4003 for no reason a reader could diagnose. Availability only, dev-only, and in the safe
  direction. Remediation: pass the effective auth mode down with the request (the session already
  carries `email`; carry "was this identity store-backed" beside it) or skip the check when
  `s.email == ""`, with a comment saying which listener that is.

- **[low / confidence: high]** `internal/terminal/bridge.go:459-470` — the stopped-project override
  runs *after* the revoked override, so a revoked user whose project is also stopped is logged and
  re-closed as 4002 "project stopped". The client is unaffected (the socket already carries 4003;
  the second `conn.Close` is a documented no-op — `casClosing`, close.go:100), so this is a log-truth
  defect only: the audit line for a revocation says "project stopped". Given that this log line is
  the only durable record that an access revocation took effect, it is worth getting right.
  Remediation: move the `if revoked.Load()` override below the `Stopped` override, or make the
  revoked branch `return`-shaped so nothing can overwrite it.

- **[low / confidence: low]** `internal/httpapi/server.go:156,855` — `provisioning` is a `sync.Map`
  keyed by project ID alone, while the sibling `deleting` map is keyed `user + "/" + id`. Project
  IDs are server-minted UUIDv4 and validated by shape on load (`internal/project/store.go:106,138`),
  so no user can choose one and there is no realistic cross-tenant collision; the impact even on a
  collision is a wrong `state` string in one list response. Flagged only because the asymmetry with
  `deleting` invites a future ID source that *is* client-influenced. Remediation: key it
  `user + "/" + id` like its neighbour.

### Adjudicated and clear (the five questions asked)

1. **Trust boundary — can a client influence which email is re-checked? No.** `session.email` is
   `identity.FromContext(r.Context()).Email` captured at upgrade (`bridge.go:114-115`, `189-190`),
   set only by `authGuard`. In header mode that value came from `X-Caddy-Email` behind a mandatory
   constant-time `X-Styrman-Ingress` compare (`auth.go:54-58`; `config.go:207` refuses to start
   header mode with an empty token), and `deploy/caddy/ingress/generator.go:137,407` strips
   `X-Caddy-Email`, `X-Styrman-Ingress`, `X-Token-User-Email` from the inbound request before
   re-setting them from the auth portal's placeholder — so a spoofed header never survives, and
   `Header.Get` returning the first value cannot be gamed. `/ws/term/{project}/{term}` and
   `/ws/setup` (`server.go:370-371`) are neither `isLinkPath`, `isPreviewPath`
   (`shares.go:101-107`) nor `isSharedPath`, so a preview-only guest (`Authorized:false`,
   `Username:""`) is 403'd at `auth.go:71` and never reaches the bridge. Both handshake and
   re-check normalize through `userstore.normalize`, so no case/whitespace mismatch can make the
   two disagree.
2. **`AuthMode=none` exemption adds no exposure.** `deploy/docker-compose.yml:86` sets
   `STYRMAN_AUTH=header`; `config.go:194` refuses `none` on a non-loopback `STYRMAN_ADDR` and
   `config.go:199` rejects any third mode, with `auth.go:88-93` as a fail-closed backstop. In
   `none` mode `authGuard` mints `identity.User{Admin:true, Authorized:true}` for every request
   with *no* credential at all (`auth.go:85-87`) — i.e. a misconfigured `none` production has no
   auth at the handshake either, so skipping the periodic re-check cannot widen anything. The nil
   check returns immediately (`bridge.go:76-78`) and is unit-tested
   (`reauth_test.go:TestRevocationWatchIgnoresANilCheck`).
3. **Removal semantics / residual.** `Remove` deletes from users.json and fsync-renames
   (`userstore.go:153-169`); `authGuard` calls `IsAuthorized` on **every** request, so REST and any
   new WS handshake are refused immediately — there is no session cache to outlive it (the 8 h
   ADR-063 token is an *ingress* session; the styrman-side permission check is per request). The
   residual for an already-open socket is therefore ≤ 60 s of terminal stream, and `/ws/setup` is
   covered identically (`bridge.go:189-190,214`). The uncovered long-lived stream in the same class
   is the port-preview proxy: `httputil.ReverseProxy` (`internal/httpapi/proxy.go:146`) forwards
   protocol upgrades, so an established preview WebSocket is never re-checked — though preview
   access is granted by the per-port share list rather than the userstore, so a removed user retains
   it by design as a preview-only guest. Worth an explicit line in the docs; not a defect in this diff.
4. **Cross-goroutine `conn.Close`: no panic, no leaked bytes.** coder/websocket v1.8.15 documents
   "All methods may be called concurrently except for Reader and Read" (`conn.go:30`); `Close` is
   idempotent via `casClosing` (`close.go:100`), so the handler's own `conn.Close` at
   `bridge.go:476` is a no-op returning `net.ErrClosed`. `writeFrame` holds `writeFrameMu`
   (`write.go:289`), so a data frame already in flight completes whole — no partial frame — and
   once the close frame is written `closeSentErr` makes every later `conn.Write` return
   `net.ErrClosed` (`write.go:310-314,360-363`), which the exec pump treats as its exit condition
   (`bridge.go:399-401`). Net effect: not one byte of terminal output is delivered after the
   revocation decision. What *was* delivered is up to 60 s of output the user was authorized to see
   at the instant it was produced — that is the accepted residual, stated plainly, not a leak.
   Ordering is sound: `revoked.Store(true)` precedes the close, so the `revoked.Load()` at
   `bridge.go:453` after `<-streamDone` observes it. Blocking is bounded (5 s write + 5 s peer wait
   + ≤15 s `waitGoroutines`), which only delays `ex.Close()`; `wg.Wait()` cannot deadlock because
   the watch goroutine always returns after `onRevoked`.
5. **DoS: the ticker is not the bound.** One `time.Ticker` + one goroutine per open socket, on top
   of the three goroutines, a Docker exec stream and a Kata microVM the same socket already costs —
   the marginal cost is noise, and the 60 s `IsAuthorized` call takes an uncontended mutex.
   Unauthenticated parties cannot open one at all (authGuard precedes the upgrade). An authorized
   user *can* hold unboundedly many: `conversation.Store.Create` (`internal/conversation/store.go:80`)
   caps neither terminals per project nor projects' terminals in total, and `activeConns` only
   dedupes *per* session key (one socket per terminal record, `bridge.go:276-283`). That
   pre-existing absence of a cap is the real limit, and it is bounded in practice by the user's own
   VM resources rather than the server's. No new exposure from this change; a per-user terminal cap
   remains a reasonable hardening item for a later pass.

## Checked

- **Secrets** — none introduced. The new log line (`bridge.go:381`) carries `user` (username slug)
  and `session` key, not the email or any token; no credential is read or echoed anywhere in the diff.
  No `.env`, key, or credential file touched.
- **Injection** — no new parsing boundary. The one new external-ish input path is the email string,
  which is compared against a map (no query, no shell, no path built from it). `revocationWatch` takes
  closures, not data. Frontend hunks add a numeric close-code constant and a string; no
  `dangerouslySetInnerHTML`, no URL construction.
- **Authz/authn** — the substance of this review; see the five adjudications above. The change
  *adds* an authorization check where there was none and orders it correctly; the residual gaps are
  scope (VM/agents keep running) and the env-admin exemption, both recorded as low.
- **Crypto** — nothing hand-rolled, nothing added. The identity path continues to rely on the
  ingress's OAuth and a `crypto/subtle.ConstantTimeCompare` on the ingress token (`auth.go:40,55`).
- **Dependencies** — no new module in the phase-4 diff (`go.mod` untouched by it); `AbortSignal.any`
  in `web/src/lib/api.ts` is a platform API, not a package. `govulncheck ./...` run directly on the
  root module — result appended below. `npm audit` not run: the phase-4 web hunks add no dependency,
  and `web/package.json`/`deploy/caddy/ingress/go.{mod,sum}` are modified in the working tree by
  *other* phases, outside this range (see Not examined).
- **GCP surface** — none. Self-hosted deployment; `.agents/docs/deploy/gcp.md` is marked not
  applicable. No IAM, service account, or Secret Manager surface in the diff.
- **File hygiene (project rule)** — `git diff --numstat` shows text line counts for every changed
  file (`bridge.go` 65/4, `server.go` 45/14, `config.go` 5/0, `api.ts` 26/4, `TerminalPane.tsx`
  10/0); `internal/terminal/reauth_test.go` is new and untracked (`??`), colliding with nothing,
  and `file` reports UTF-8 text. A scan for raw control bytes over the changed Go files found none.
  `go vet` clean; `go test ./internal/terminal/ ./internal/userstore/` passes, including the three
  new `revocationWatch` cases.

### govulncheck

Authoritative run: `bash deploy/vulncheck.sh` (both Go modules plus the image harness, inside the
pinned `golang:1.25.13-trixie` builder, exit 0):

- root module: **2 affected** — `GO-2026-4887` (Moby AuthZ plugin bypass on oversized request
  bodies) and `GO-2026-4883` (off-by-one in Moby plugin privilege validation), both in
  `github.com/docker/docker@v28.5.2+incompatible`, both **`Fixed in: N/A`**. These are daemon-side
  Moby defects reached through the client module's symbol graph; styrman installs no Docker
  authorization plugin and no plugins at all, and there is no fixed release to move to. Pre-existing,
  untouched by this diff, no action available beyond tracking the advisories.
- `deploy/caddy/ingress`: the same two, same reasoning.
- `image/harness`: no vulnerabilities found.

For contrast, a bare `govulncheck ./...` on this workstation reports 31, because the local toolchain
is `go1.25.0` and 29 of those are standard-library advisories fixed in `go1.25.x`. The shipped binary
is built by `deploy/Dockerfile.server` from `golang:1.25.13-trixie`, so those 29 do not apply to
production — which is precisely why `deploy/vulncheck.sh` exists and why its result, not the local
one, is the one recorded here.

## Not examined

- The vendored Caddy ingress module (`deploy/caddy/ingress/`) was read only for its header-injection
  semantics (`generator.go:137,170,176,407`); its own tests were not run and its `go.mod`/`go.sum`
  changes in the working tree belong to another phase and were not reviewed.
- Working-tree changes outside `.agents/scratch/phase4.diff` — `internal/httpapi/check.go`,
  `format.go`, `deploy/firewall/*`, `deploy/host-setup.sh`, `deploy/update/*`, `web/e2e/*`,
  `web/package.json`, `web/src/components/{CodeEditor,EditorPane,FileEditor,Sidebar}.tsx`,
  `web/src/lib/{editorPane,sshKey}.ts`, and the regenerated `web/dist/client` bundle — are other
  phases of the close-out and were not adjudicated here.
- `npm audit` not run (no dependency change in range); `bash deploy/vulncheck.sh` (the pinned-builder,
  both-modules path) not run — the local `govulncheck` was used instead.
- No runtime verification against a real Kata host: the revocation path was reasoned about from the
  code and the library's documented semantics plus the new unit tests, not observed end to end.
  An integration check that removes a user and asserts the 4003 close within ~60 s would be the
  honest confirmation and does not exist yet.

## Response (main, 2026-09-10)

- **Finding 3 (low) — fixed.** The re-check treats an identity with no
  email as minted, not looked up, and never revokes it; a store-backed
  identity always carries the ingress email.
- **Findings 1 and 2 (low) — documented, not changed.** This is terminal
  revocation by design: the plan's item I closed "a removed user's open
  tab keeps streaming", nothing more. Projects, tmux sessions and brokered
  credentials of a removed user keep running until an admin stops them;
  env-provided admins cannot be removed without an env edit and restart.
  Both sentences go into the guide's user-management paragraph in Phase 6
  (documentation agent) so the boundary is stated where an admin reads it.
