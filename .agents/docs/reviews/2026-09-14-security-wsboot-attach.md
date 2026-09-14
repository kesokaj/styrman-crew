---
owner: security
date: 2026-09-14
---
# Security review — wsboot-attach   (2026-09-14)

Range reviewed: working tree vs `d938605d9ff1eee1f93c7522dbc4c72953ef59d5` (HEAD), limited to
the three named paths. Blobs pinned as reviewed (`git hash-object`):

| path | blob |
|---|---|
| `internal/httpapi/server.go` | `51bc7e0c62a957b7d3758103054a0f06c5fb7709` |
| `internal/orchestrator/orchestrator.go` | `f5cd95db4056e0317fe6c1003a4cf94010ab8372` |
| `internal/httpapi/wsboot_attach_test.go` (untracked) | `222f36c7f070d0107c621420d8a1d65295699ab9` |

`git diff --numstat`: `server.go` +29/-1, `orchestrator.go` +3/-1 (comment only). Neither shows
`-`/`-`; `file` reports the new test as ASCII text — BUG-007 check passed, all three are readable
source. `go vet ./internal/httpapi/ ./internal/orchestrator/` exit 0;
`go test ./internal/httpapi/ -run TestWSBoot -count=1` exit 0 (`ok`, 0.003s).

## Verdict: FINDINGS

Nothing here grants a non-owner or an unauthenticated caller anything new, and the authorization
chain around the new attach is sound end to end (see Checked). The worst thing on the record is an
**observability gap, not a breach**: `EnsureSelfAttached` returns a single `bool` that means
"freshly attached", conflating "already attached, good" with "attach failed, the server is NOT on
that bridge at the broker address" — and `wsBoot` discards even that. Genuine attach failures are
deliberately not memoized, and the static-broker-IP path silently falls back to a dynamic IP, so
the terminal this change exists to fix can still spawn a login shell with no reachable credential
broker, with a single server-side WARN as the only evidence. The same address is load-bearing for
an in-guest access-control decision (ADR at `DECISIONS.md:2624`: the CDP forwarder accepts only
`<peer>/32` parsed from `STYRMAN_CRED_BROKER`), so a silent fallback silently disables that too.
Both fail in the safe direction; neither is visible to the path that depends on them.

## Findings

- **[low / confidence: medium]** `internal/httpapi/server.go:305` (and the API at
  `internal/orchestrator/orchestrator.go:619-673`) — **the attach's success is unobservable to the
  path that depends on it.** `EnsureSelfAttached` returns `false` on three different outcomes:
  memo hit, "already exists" (both good), and a genuine `NetworkConnect` failure (bad, logged
  `Warn`, deliberately not memoized so the next call retries). It also downgrades silently from the
  fixed broker IP to a dynamic IP when the static address is rejected (`orchestrator.go:653-659`).
  `wsBoot` ignores the return entirely, then the bridge upgrades the socket and spawns
  `bash -l`, whose `/etc/profile.d` snippet pulls credentials from `STYRMAN_CRED_BROKER` — an
  address the server may not hold. Exploitable in principle only as a silent degradation: the
  terminal comes up credential-less (BUG-060's exact symptom, on the failure branch), and
  `DECISIONS.md:2624`'s in-guest `range=<peer>/32` CDP gate — which treats the broker IP as proof
  the caller is the server — matches nothing, so the browser relay is refused. Failing closed is
  the right direction; being unable to *tell* is the defect, and a security control whose
  precondition can vanish without a signal is the family of BUG-034 ("silence from your own
  instrumentation is not evidence"). Remediation: widen the return to distinguish
  attached-at-broker-IP / attached-dynamically / not-attached (or return an `error`), and have
  `wsBoot` log the project when it is not the first, so the credential-less terminal is
  diagnosable from the server log without correlating an unrelated WARN.

- **[low / confidence: low]** `internal/httpapi/server.go:300-307` — **a synchronous,
  uncancellable, 30-second-worst-case Docker call now sits inside the `GET /ws/term/...` handler,
  before the upgrade.** `EnsureSelfAttached` takes no `ctx` by design and builds its own detached
  `context.WithTimeout(context.Background(), 30*time.Second)`, so `r.Context()` cancellation
  (client gone, ingress timeout) is ignored; with the static-IP probe it is a `NetworkInspect`
  plus one or two `NetworkConnect` round-trips. Because genuine failures are not memoized, a hung
  or slow Docker daemon is re-dialled on *every* terminal connect. `http.Server` sets only
  `ReadHeaderTimeout: 10s` (`cmd/styrman/main.go:138-141`), which does not bound handler time, and
  there is no `WriteTimeout` or `http.TimeoutHandler`, so each blocked handshake pins a goroutine
  and an ingress→server connection for the full 30 s. Requires an authorized user and is bounded:
  the frontend caps at `MAX_RETRIES = 8` with exponential backoff to 15 s
  (`web/src/components/TerminalPane.tsx:33,252-257`), and the path already blocked for seconds
  inside `EnsureRunning`. So this widens an existing window rather than opening a new class — but
  it is a new failure mode on this path, where before the change there was no Docker-network call
  at all. Remediation: bound the attach's contribution to the handler (short deadline, let the
  in-guest broker retry cover the rest), or memoize *failures* with a short TTL so a hung daemon
  is not re-dialled per connect.

- **[low / confidence: medium] — pre-existing, restated because the diff pulls it into a hotter
  path** `deploy/docker-compose.yml:79-81` — **`net.ipv4.ip_forward=0` is asserted nowhere at
  runtime.** It is the only thing making the multi-bridge-attached server "structurally incapable
  of being used as a cross-project router" (REQUIREMENTS.md:77,81,117), and it exists solely as a
  compose `sysctls:` entry: a `docker run` deploy, an edited compose, or a future orchestration
  change loses R12's cross-project guarantee with no warning. This diff does not introduce it, but
  it makes the server's attachment to a tenant bridge happen **promptly and synchronously on a
  path any user triggers by opening a terminal**, rather than lazily on a preview request — so the
  unasserted invariant is now load-bearing on the hottest path in the product. Already on record
  as item 9 of `.agents/docs/reviews/2026-09-01-audit-security.md`; unchanged remediation: read
  `/proc/sys/net/ipv4/ip_forward` at startup when `cfg.SelfContainer != ""` and refuse to serve
  (or log at ERROR) when it is 1.

- **[low / confidence: low]** `internal/httpapi/server.go:287-298` (the `wsBoot` doc comment) —
  **the justification for discarding `fresh` is narrower than the behaviour it justifies.** It
  reasons only about the triggering client ("a client whose handshake a fresh attach disturbed
  simply reconnects into the memoized attach"), but a fresh attach reconfigures the *single* server
  container netns shared by every tenant's live terminals — which is precisely what the
  orchestrator's own NOTE at `orchestrator.go:448-453` warns about. Two mitigating facts I did
  confirm: the triggering client does recover (`ensureRunningLocked` clears the deliberately-stopped
  marker at `orchestrator.go:444` *before* the readiness wait, so the boot-param-less automatic
  reconnect passes the `?boot=1` gate at `internal/terminal/bridge.go:156-167` instead of getting
  close 4002), and the production server publishes no host ports (`deploy/docker-compose.yml:121`,
  `networks: [styrman-ingress]`; Caddy is `network_mode: host` and dials the container IP), whereas
  the NOTE's observed ERR_CONNECTION_REFUSED was on a published dev port — so the real blast is
  likely smaller than the NOTE implies. Per this project's own rule, a comment that states a
  narrower scope than the behaviour is read as a guarantee. Remediation: widen the comment to name
  the cross-tenant scope, or verify once on the live host and record the result.
  **Unverified and folded in here rather than asserted:** whether attaching an additional bridge
  changes the server container's *default route* (Docker selects a gateway across attached
  networks), which would put the server's own egress on a tenant's bridge — where a guest that is
  root in its own microVM shares the L2 segment. I did not run it; it is pre-existing to the
  multi-bridge design in any case. One-shot check: `docker exec styrman-server ip route` before and
  after a project attach.

## Checked

- **Secrets** — none in the diff. The new code logs nothing at all; `EnsureSelfAttached`'s WARN
  carries `project` + `error` only, and `credbroker` logs `user`/`project`/`host` but never token
  values (`internal/credbroker/broker.go:96`). No committed `.env`, key, or credential touched.
- **Injection** — the attach's one attacker-adjacent input is the project ID reaching
  `config.NetworkName(projectID)` = `"sdb-" + projectID`. `ensureFor` resolves it through the
  *caller's own* per-user store by exact map lookup (`internal/project/store.go:254-262`) and passes
  the store-canonical `p.ID`, never the raw path value; stored IDs are pinned to lowercase UUIDv4
  by regex on load (`store.go:112,144`). So the network name can only ever be the caller's own
  bridge. No path, command, SQL, or template construction in the diff.
- **Authz/authn** — sound, no new reach. `authGuard` (`internal/httpapi/auth.go:27-115`) demands a
  constant-time-compared `X-Styrman-Ingress` token in header mode, and `config.Load` **fails
  closed** without one (`internal/config/config.go:226-229`), so the "project VM spoofs
  `X-Caddy-Email` against :8080 on the bridge" path is shut; preview-only guests (`Authorized:
  false`) are 403'd on everything that is not `isPreviewPath`/`isSharedPath`, which `/ws/term/...`
  is not. `Bridge.ServeHTTP`'s **first statement** is `SameOriginOrAbsent`
  (`internal/terminal/bridge.go:114-118`), so a forged cross-origin GET boots and attaches nothing.
  The `?boot=1` deliberately-stopped gate runs **before** `b.ensure`, so the new attach cannot
  resurrect a stopped project. `wsBoot` attaches only after `EnsureRunning` succeeds — pinned by
  `TestWSBootDoesNotAttachWhenTheBootFails`, the right invariant. Credential exposure is unchanged:
  the broker authorizes by source IP via `ResolveProjectByIP` with a `containerHoldsIP` check
  (`orchestrator.go:100-110`), and a guest receives its own owner's credentials — ADR-030/032's
  design, not a new grant. The set of bridges the server joins is also unchanged; six pre-existing
  call sites already attach (`cmd/styrman/main.go:109`, `httpapi/network.go:241`, `proxy.go:82`,
  `browser.go:250`, `server.go:937`, `server.go:1142`). The delta is *timing*, not *reach*.
- **Crypto** — nothing cryptographic in the diff; no hand-rolled auth, hashing, session, or JWT
  code introduced. The identity chain continues to rest on the ingress's OAuth plus
  `crypto/subtle.ConstantTimeCompare` on the shared token.
- **Dependencies** — no change: `git diff --stat -- go.mod go.sum` is empty; the new test imports
  only stdlib plus an in-repo package. `govulncheck ./...` ran on the host (exit 0) and reports 31
  called vulnerabilities, **all Go standard library under the host toolchain go1.25.0** (e.g.
  GO-2025-4007, fixed in crypto/x509@go1.25.3). This is an artifact of my host toolchain, **not**
  the shipped artifact: `deploy/Dockerfile.server:9` and `image/Dockerfile:8` pin
  `golang:1.25.13-trixie` by digest, which is past every fix listed. The authoritative run is
  `bash deploy/vulncheck.sh` (govulncheck inside that pinned builder for both modules) — **I did
  not execute it**, so the host numbers above should not be read as a release signal. Zero
  called vulnerabilities came from third-party modules.
- **GCP surface** — not applicable; styrman is self-hosted and `.agents/docs/deploy/gcp.md` records
  it as such. No IAM binding, service account, Cloud Run flag, or key file appears anywhere in the
  blast radius.

## Not examined

- `deploy/vulncheck.sh` was **not run** (containerized, needs the pinned builder image) — the
  govulncheck numbers above are the host toolchain's, with the caveat stated.
- The vendored Caddy ingress module (`deploy/caddy/ingress/`, a separate Go module with its own
  security-critical tests) — read only for how `STYRMAN_INGRESS_TOKEN` is injected.
- `deploy/firewall/styrman-firewall.sh` — read only for its `sdb-`/broker handling (the
  same-bridge `physdev` RETURN that keeps the broker reachable from a sandbox project); the rest
  of the ruleset, and the `lan-*` path, were not audited here.
- `image/` (entrypoint, `styrman-setup`, the `/etc/profile.d` broker snippet that consumes
  `STYRMAN_CRED_BROKER`) — the consumer of the address this change exists to make reachable.
- `web/` beyond `TerminalPane.tsx`'s reconnect loop, read only to size the retry amplification.
- **No live-host verification of anything.** Every claim above is from source reading plus the
  local `go vet` / `go test` runs named at the top. The default-route question in the fourth
  finding, and the cross-tenant scope of a fresh attach in the same finding, are explicitly
  unverified and marked low confidence for that reason.

## Outcome

- **1 [low/med] attach failure discarded by `wsBoot` — REBUTTED, no change.**
  The failure branch is not silent: `EnsureSelfAttached` itself logs
  `self-attach to project network failed` (and `broker static IP attach
  failed — retrying dynamic` for the fallback) at Warn, and the guest's
  login shell reports the outcome in its MOTD (BUG-054). Failing the
  terminal on an attach failure would turn a credential outage into a
  shell outage — the user could then not even open the terminal that shows
  the MOTD. Same handling as every pre-existing attach path (the goroutines
  after create/start/toggle discard the value too). The CDP peer gate
  matching nothing on that branch is the documented fail-closed behaviour.
- **2 [low/low] uncancellable 30 s attach inside the handshake — REBUTTED,
  no change.** The same handshake already waits up to 90 s on
  `EnsureRunning`'s readiness poll; a Docker API hang stalls the boot
  before it can stall the attach. The detached context is by design (the
  attach must complete even if the client gives up, see its doc comment),
  and the 60 s memo makes the steady-state cost zero. A separate bound
  would add a mechanism for a case the boot itself does not bound.
- **3 [low/med, pre-existing] `ip_forward=0` unasserted at runtime — OUT
  OF SCOPE, carried.** Not introduced or aggravated here: attaching to a
  bridge changes no sysctl, the port-preview proxy has attached
  per-request since v1.14, and the WS attach is memoized for 60 s. Carried
  to the handoff's Next Steps as an operator decision (a compose-level
  assertion would be a deploy change, reviewed on its own).
