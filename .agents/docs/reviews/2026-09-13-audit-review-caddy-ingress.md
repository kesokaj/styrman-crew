---
owner: reviewer
date: 2026-09-13
---
# Review — audit: deploy/caddy/ingress   (2026-09-13)

Scope: `deploy/caddy/ingress/` — all non-test `.go` files (`cloudflare.go`,
`cmd.go`, `config.go`, `generator.go`, `labels.go`, `loopback.go`,
`module.go`, `watcher.go`), `go.mod`/`go.sum`, and the sibling
`deploy/caddy/Caddyfile.base`/`deploy/caddy/Dockerfile` /
`deploy/docker-compose.yml` caddy service block for wiring context. This is
an `/audit` scan, not a diff review: judged against `REQUIREMENTS.md`
(R8, R12), `AGENTS.md`, `.agents/docs/bugs/INDEX.md`, and
`.agents/docs/specs/styrman/{deploy,api,guide,versioning}.md`. Repo HEAD
pinned: `e56e775708a9062cc6432f5c3278770bed63b685` (clean tree). Every file
cited below is read at that commit.

## Verdict: APPROVE WITH NITS

Nothing broken was found. This module has clearly been through several prior
audit/security rounds (commit history: "audit fixes 3/n: ingress email
injection, path traversal, a stale contract, dep divergence",
"ingress: the admin loopback guard verifies an IP, it no longer trusts a
name", "the portal issues eight-hour sessions", etc.) and it shows: every
value that reaches the generated Caddyfile verbatim (hostnames, emails, the
admin bind address) is validated with a comment explaining the specific past
incident it closes, `go build`/`go vet`/`go test ./...` are all clean at HEAD
(verified bare, rc=0 each), and the security-relevant header-stripping
(`X-Styrman-Ingress`, the caddy-security upstream claim headers) is applied
consistently across all three site-block writers (`writeAppBlock`,
`writePreviewWildcard`, `writePrivateWildcard`). The only findings are one
genuinely dead exported constant and one unvalidated (but low-blast-radius,
self-healing) input edge case.

## Findings

- **[low / confidence: high]** `deploy/caddy/ingress/config.go:47` —
  `AuthCookieName = "AUTHP_ACCESS_TOKEN"` has zero callers anywhere in the
  repository (checked both the ingress module and the rest of the tree,
  including tests). Per review-standards criterion 1 (dead code), this is a
  removal, not a comment-and-keep. Fix: delete the constant, or if it is
  meant for a planned server-side cookie check, wire that caller in the same
  change that adds it.

- **[low / confidence: medium]** `deploy/caddy/ingress/labels.go:64-68` and
  `generator.go:373-380` — `sanitizeHostname` can produce an empty string
  (e.g. a container named `"___"` or a `caddy_hostname` label of all
  underscores/spaces) or the same output for two differently-named
  containers (e.g. `My_App` and `my-app` both sanitize to `my-app`). Neither
  `ParseLabels` nor `GenerateCaddyfile` rejects an empty or duplicate
  hostname; the result is two identical (or one blank, `.<AppsDomain>`)
  Caddyfile site addresses. `watcher.go`'s own defense (`noteReloadFailure` /
  `scheduleRetry`) means the practical effect is bounded — Caddy rejects the
  malformed/duplicate config at `/load`, the watcher logs and retries
  forever, and the previously-good config keeps serving — so this is not an
  outage on its own, and reaching it requires an operator to hand-label two
  infrastructure containers in `docker-compose.yml` (the only containers
  this ingress currently discovers; project microVMs are not caddy-labeled).
  Fix, if worth doing: reject an empty `sanitizeHostname` result and track
  hostnames seen so far in `discoverApps`/`GenerateCaddyfile` to fail loudly
  (log + skip) on a collision instead of emitting invalid Caddyfile and
  relying on the retry loop to mask it.

No high or medium findings. Specifically checked and found sound:
- `extractContainerIP` (`watcher.go:361-392`): the "avoid project bridges"
  filter matches on Docker NETWORK name (`sdb-<uuid>`), and per
  `internal/config/config.go:451` (`NetworkName`) every project network is
  named `sdb-<uuid>` regardless of LAN/sandbox mode — only the bridge
  INTERFACE (a lower-level, separate concept, not visible in
  `NetworkSettings.Networks`) changes to `lan-<hash>` under ADR-067. So a
  LAN-mode project does not slip through this filter; R12's isolation
  guarantee is not affected by this code.
- The admin API loopback guard (`loopback.go`), the ingress-token strip-then-set
  pattern (three call sites in `generator.go`), and the reload-retry/fatal
  state machine (`watcher.go`) all match their own extensive comments and
  are exercised by `watcher_test.go`'s mutation-style tests (e.g.
  `TestRejectedReloadArmsRetryAndFatalDoesNot`, matching BUG-036's rule of
  proving a guard actually gates something).
- No BUG-INDEX mechanism is reincarnated here: no `os.OpenRoot` on
  guest-writable paths, no `$(...)`-wrapped side effect under `set -e`
  (this is Go, not shell), no piped verification feeding a commit.

## Criteria

1. **Dead code** — fail (one finding): `AuthCookieName` unused constant.
   Everything else exported (`App`, `AccessMode`, `GeneratorConfig`,
   `ParseLabels`, `GenerateCaddyfile`, `NewWatcher`, `NewCloudflareClient`,
   `DetectPublicIP`) has live callers in `cmd.go`/tests.
2. **Flow tracing** — pass. `cmdIngress` → `loadConfig` → `caddy.Run` →
   `NewWatcher` → `Start` → `scanAndReload`/`watchEvents` is a clean,
   consistently-locked chain (`scanMu` guards `adminDown`, `caddyfileMu`
   guards `lastCaddyfile`, each timer has its own mutex); every `err` is
   wrapped with `%w` and context or explicitly classified
   (`errAdminUnreachable` vs. rejected-load); the admin-unreachable path
   propagates to `Fatal()` and `cmd.go` exits nonzero for the restart policy,
   matching the documented recovery story.
3. **Standards conformance** — pass. Idiomatic Go: no floating goroutines
   (each has an exit path via `ctx.Done()`), `context.Context` threaded
   through Docker calls, `log/slog` JSON logging, `init()` used only for the
   two Caddy-plugin-API registrations with an explicit "approved exception"
   comment per project convention. `go build`, `go vet`, `go test ./...` all
   exit 0 at HEAD (run bare, status read before writing this report, per
   `.agents/rules/verify-bare.md`).
4. **Directory layout** — pass. Correctly isolated as its own module under
   `deploy/caddy/ingress/`, tests beside their source
   (`labels_test.go`/`labels_email_test.go` next to `labels.go`, etc.),
   nothing stray at the repo root.
5. **Simplicity (DIY-except-security)** — pass, with the security exemption
   correctly applied: OAuth/session handling is caddy-security (a vetted
   Caddy plugin), not hand-rolled; the module's own code is glue (label
   parsing, Caddyfile templating, Docker watching) using only the approved
   deps (`caddyserver/caddy`, `docker/docker`) already declared in
   `REQUIREMENTS.md`'s constraints section (ingress deps are historically
   tracked separately from the server's approved-dep list, and none of the
   indirect deps in `go.mod` are hand-added application logic).
6. **Decoupling** — pass. Small, well-scoped files (labels vs. generator vs.
   watcher vs. cloudflare vs. cmd), each independently editable; `App` is a
   plain data-transfer struct with no cross-package reach-ins; the module
   has no import of and no dependency on the main `styrman` module (per
   `deploy/caddy/VENDORED.md`, deliberately forked rather than imported, per
   `AGENTS.md`'s "port files, don't import the devbox module" rule).

## Not examined

- `deploy/caddy/Dockerfile` and the wider `deploy/docker-compose.yml` beyond
  the `caddy`/`styrman` service blocks quoted above (image build layers,
  volumes for other services).
- Live behavior against a real Caddy/Docker daemon or the Cloudflare API
  (no network calls were made; `cloudflare.go`'s HTTP logic was read, not
  exercised against a live token).
- `caddy-security`/`caddyserver/caddy` and their transitive dependencies'
  own source (trusted as vetted per the simplicity exemption; supply-chain
  scanning is `deploy/vulncheck.sh`'s job, which already covers this module
  explicitly — confirmed by reading the script, not re-run here).
- Historical commits/ADRs prior to the ones surfaced by `git log --oneline
  -- deploy/caddy/ingress` and the three newest `.agents/docs/changes/`
  entries (none of which touch this directory — the most recent substantive
  change here predates the reviewed changes-log window).
- The Playwright e2e and integration suites (`web/e2e/run.sh`,
  `.agents/docs/testing/styrman/integration.sh`) were not run; they do not
  target this ingress module directly per `AGENTS.md`.
