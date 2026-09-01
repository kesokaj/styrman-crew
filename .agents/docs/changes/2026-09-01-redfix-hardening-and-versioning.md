# Redfix: bounded log buffer + Caddy admin API, watcher self-heal, no-mutable-tags versioning

Date: 2026-09-01. Third batch of today's work, following the whole-project
audit (`.agents/docs/changes/2026-09-01-audit-v1.16.0.md`) and interrupted
mid-flight by the reboot recorded in
`.agents/docs/changes/2026-09-01-incident-reboot-outage.md` — read that
entry first for the mechanism this batch's ADR-054 closes. Gates the
v1.16.1 release, cut in the step immediately after this commit.

## 1. CLOSED: the two 🔴s from the audit

**🔴-1, unbounded guest-controlled log buffer** (`internal/orchestrator`) —
`Orchestrator.Logs`'s `ContainerLogs` read used an unbounded
`strings.Builder`, so a hostile or noisy guest process could OOM the
server. Fixed with a new `tailBuffer` (`exec.go`), a bytes.Writer-style
sink that keeps only the LAST `max` bytes written (the opposite retention
policy from `cappedBuffer`, which keeps the first `max` and exists for a
different threat — reusing it would have kept the *oldest* log output, the
wrong end). `trimLogTail` (also `exec.go`) cleans up the tail-truncated
window afterward: strips a partial first line and prepends a
"… (older lines truncated) …" banner, unless the discard boundary landed
exactly on a line start (tracked via a `lastDrop` byte, not just a
boolean, so the exact-boundary case doesn't wrongly eat a complete first
line). Closed in code; ships live with the next server rebuild — this is
a Go server change, not something the caddy image or the reboot touched.

**🔴-2, unauthenticated Caddy admin API on host loopback + a floor below
2.11.3** (`deploy/caddy/ingress`, `deploy/caddy/Dockerfile`) — the admin
API's `/load` is unauthenticated-by-design on Caddy (no bearer token,
origin checks are browser-only defense-in-depth). The real controls are:
(a) a new `isLoopbackAddr` guard (`loopback.go`) that requires `CADDY_ADMIN`
to resolve to a loopback address **with an explicit numeric port**
(`strconv.Atoi` in range, closing a round-1 gap where a bare host would
have bound port 80, the public HTTP server); (b) the generated Caddyfile
now emits an explicit `admin <loopback-addr>` block on every load — this
is the line that structurally prevents the admin listener from ever
moving off loopback, including on a *rejected* load (the exact 19:28
incident shape); (c) the Caddy floor raised to **2.11.4**, with the
Dockerfile now pinning `caddy:2.11.4-builder`/`caddy:2.11.4` plus every
xcaddy plugin (`caddy-security@v1.1.64`, `caddy-dns/cloudflare@v0.2.4`,
`caddy-ratelimit@v0.1.0`) instead of floating tags — a floating builder had
made the tag→binary mapping non-deterministic, so a rollback couldn't
reliably restore what had actually run. `EXPOSE 2019` dropped from the
Dockerfile (inert under `network_mode: host`, but was actively
contradicting the new invariant).

**Code-closed vs. live-closed, stated honestly:** the code above closed
🔴-2 in the tree well before the reboot. The reboot then proved the gap
between "closed in the tree" and "closed on the running host" — the
container that came back up after `sudo reboot` was still the old
2026-08-30 `:latest` image, which has neither the loopback guard nor the
admin-pinning block. The 19:40 emergency deploy put the 16:49 pre-redfix
build back up (see the incident entry); the build carrying *this* commit's
final tree — round-2/round-3 fixes and the watcher self-heal below — gets
its own image build, smoke test, and deploy in the v1.16.1 release step
immediately following this commit. Until that step runs, "🔴-2 is closed"
means the code, not yet the live container.

## 2. Review gates — three rounds, all reports on disk

**Round 1** (`reviews/2026-09-01-review-redfix.md`) — REQUEST CHANGES, 11
findings, none blocking the commit but two undermining the closure claim:
the `origins` directive's comment/test asserted a guarantee Caddy does not
provide (it's browser-only defense-in-depth, not `/load` auth — corrected
to say so), and 🔴-2 wasn't remediated in anything that runs yet (the
Dockerfile was untouched at that point). The other 9 were correctness/test
nits (a spurious `dropped` flag on an exact-`max`-byte write into an empty
buffer, the trim eating a complete first line on an exact boundary, no
test for `Logs`'s post-processing, a hand-rolled `contains()` reimplementing
`strings.Contains`, `adminAddr` computed from a second independent env
read instead of the validated value, file naming, an honest govulncheck
correction, and doc gaps). All 11 fixed; round-3 (below) re-confirms none
were deferred.

**Round 2** (`reviews/2026-09-01-review-redfix-round2.md`) — REQUEST
CHANGES against the fuller tree (the no-mutable-tags versioning work had
landed by then too). Two high findings: `deploy/update/styrman-update`'s
`install -d -m 700 -o root -g root "$(dirname "$STATE_DIR")"` re-modes an
*existing* parent directory — measured turning a `1777 kesokaj` `/tmp`
into `700 root:root`, exactly the state-destroying class ADR-052 exists to
prevent, and triggered by the value the script's own error message
recommends testers pass (`$(mktemp -d)`, whose dirname is `/tmp`); and
nothing on the manual/fresh-install path built a version-tagged
`styrman-runtime` image, so a versioned deploy would boot a healthy server
on which every project create fails. Also flagged two "test theatre" gaps
— deleting the watcher's `scheduleRetry` call, and reverting the
exact-`max` `dropped` fix, both left their respective suites green — plus
several mediums/lows (caddy floor mismatch with the Dockerfile pin, tag
namespace inconsistency between `v`-prefixed and bare versions,
`host-setup.sh` still defaulting to `:latest`).

**Round 3** (delta re-review, appended to the same file) — APPROVE WITH
NITS. All round-2 blocking findings verified closed **by mutation, not by
reading**: the `install -d` fix only ever creates a missing parent/leaf,
never re-modes an existing one (confirmed against `/tmp`, `/`, `/etc`, a
symlinked and a `..`-relative path via `realpath -m` canonicalization);
both test-theatre gaps are now mutation-proven (deleting `scheduleRetry`
or reverting the `dropped` fix now fails the suite). One new medium
surfaced and was fixed in the same pass: `host-setup.sh`'s version
derivation ran under `set -euo pipefail`, so a tagless clone made the
whole provisioning script exit silently at that line — the `-abbrev=0`
form plus an explicit `|| true` fallback closed it.

**Security** (`reviews/2026-09-01-security-redfix.md`) — FINDINGS, its one
high the same `install -d` foot-gun round 2 found independently; mediums
on the stale `golang:1.25.3-trixie` pin (govulncheck: 31 CALLED stdlib
findings against that toolchain, ~23 fixed only after 1.25.3) and the
caddy builder's unpinned/unasserted Go toolchain; lows on the loopback
guard's unbounded port string and `localhost`-by-name acceptance, and the
`/var/lib/styrman` prefix-match evasion (relative paths, `//` doubling,
symlinks). All blocking items closed by round 3; the caddy-builder
toolchain assertion and base-image digest pinning were explicitly deferred
(TODO.md).

## 3. Watcher self-heal (ADR-054)

New `deploy/caddy/ingress/watcher.go` retry/fatal machinery, the direct
fix for the reboot incident's mechanism: a rejected `/load` (admin
answered, config refused) is never latched as applied — it schedules a
retry on a fixed 15s timer, indefinitely, including when the failure comes
from `discoverApps` rather than the reload itself in the tightened form.
Separately, ≥4 consecutive **transport**-level failures (admin
unreachable from this process, not just a rejected config) call `Fatal()`,
which exits the process with status 1 so `restart: unless-stopped` recovers
with a fresh minimal admin config. Verified against the real
`caddycmd`/`cmd.Main()` exit contract that a non-nil error does produce
`os.Exit(1)`. New tests
(`TestRejectedReloadArmsRetryAndFatalDoesNot`,
`TestConsecutiveUnreachableTurnsFatalButRejectionsDoNot`, and siblings) are
mutation-proven — the round-2 test-theatre finding on this exact code is
closed.

## 4. ADR-052 enforcement in code

`styrman-update` now *requires* `STYRMAN_UPDATE_STATE_DIR` — no default,
and no default that resolves inside `/var/lib/styrman`. The path is
canonicalized with `realpath -m` before the refusal check, closing the
prefix-match evasions the security review found (`//var/lib/styrman`,
`../`-relative, a symlink into the production dir). The
`install -d`/`chmod`/`chown` footgun measured independently by two agents
(security review and round-2 review) is fixed by creating the parent and
leaf **only when missing** — an existing directory, at any depth, is never
re-moded. `STYRMAN_UPDATE_STATE_DIR=/etc/styrman/update` is pinned
**only** in `styrman-update.service`'s `Environment=` line — the
production path exists in exactly one place now, not duplicated as a
script default.

## 5. No-mutable-tags versioning (ADR-053) — the operator's standing rule, implemented

- `deploy/docker-compose.yml`: `image: styrman-caddy:${STYRMAN_VERSION:?set
  STYRMAN_VERSION=X.Y.Z — mutable tags are banned}` and the equivalent for
  `styrman-server`; unset `STYRMAN_VERSION` hard-fails `docker compose`
  rather than silently falling back to anything. `STYRMAN_IMAGE` derives as
  `${STYRMAN_IMAGE:-styrman-runtime:${STYRMAN_VERSION}}`.
- `internal/config/config.go`: `Load()` now refuses an empty
  `STYRMAN_IMAGE` outright — this was previously a silent `:latest`-style
  fallback baked into the default. No test called `Load()` at all before
  this batch (round-1 verifier FAIL); `TestLoadRequiresExplicitImage` now
  covers both the refusal and the success path, mutation-verified (round-2
  verify independently re-ran the mutation rather than trusting the
  changelog claim).
- `deploy/update/styrman-update`: already built `styrman-runtime:X.Y.Z`
  per deploy (this predates the batch); now passes environment explicitly
  through `docker compose`, so a stale `deploy/.env` on the deploy clone
  can't override the version being deployed.
- `deploy/host-setup.sh`: the manual/fresh-install path builds a
  version-tagged runtime image too (was the round-2 blocker — a versioned
  deploy would otherwise boot healthy with zero bootable projects). Version
  is derived with `git describe --tags --abbrev=0`, stripped of its `v`
  prefix to match `styrman-update`'s own `${LATEST#v}` shape (round-2 found
  the two paths disagreed on tag namespace); the round-3 fix added a
  fallback for a tagless/non-git clone after `set -euo pipefail` was found
  to make that path fail silently.
- `deploy/Dockerfile.server`: `FROM golang:1.25.13-trixie` (was
  `1.25.3-trixie`, whose comment claimed a fix that didn't actually cover
  the stdlib findings — security review measured 31 CALLED govulncheck
  findings against it). `deploy/caddy/Dockerfile`: `caddy:2.11.4-builder`/
  `caddy:2.11.4` plus pinned plugin versions (§1 above); `go.mod`'s own
  caddy floor bumped to match (`v2.11.4`) after round 2 found it lagging
  the Dockerfile pin, so the module's own tests compile against what
  actually ships.
- Production `deploy/.env`'s `STYRMAN_IMAGE=styrman-runtime:dev` line is
  now commented out with a note explaining the operator rule, so the live
  deployment can't silently keep the mutable `:dev` tag.

## 6. Documentation

New `.agents/docs/specs/styrman/versioning.md` (operator-requested),
covering the full tag → image → deploy chain. Swept: `README.md`,
`.agents/docs/specs/styrman/{deploy,guide,api}.md`,
`.agents/docs/testing/styrman/README.md` (including the ADR-052
privileged-test scratch-prefix convention section), `AGENTS.md`. A round-2
addendum found the doc pass landed mid-review and left one line wrong
(`README.md` describing `host-setup.sh`'s runtime build as a "smoke-test
tag production never runs" — round 3 confirmed the script now builds the
exact tag production boots, and the line was corrected to say so).

## Verification

Reports on disk, no fixes claimed only in chat:
`reviews/2026-09-01-review-redfix.md`,
`reviews/2026-09-01-review-redfix-round2.md` (round-3 delta appended in
the same file), `reviews/2026-09-01-security-redfix.md`,
`testing/2026-09-01-verify-redfix.md` (round 1 — **FAIL**, the
`config.Load()` coverage gap above), `testing/2026-09-01-verify-redfix-round2.md`
(round 2 — **PASS**, no blocking findings).

Suites: `integration.sh` 57 passed / 0 failed / 3 skipped (documented
host-firewall preview-proxy skips), `web/e2e/run.sh` 48 passed / 0 failed /
2 skipped. Both modules `go test -race -count=1 ./...` clean: main module
107 `--- PASS` lines (105 at round 1, +2 for the new `config.Load` and
`tailBuffer`-boundary tests), ingress module 47 (46 at round 1, +1 for the
watcher retry test). `gofmt`/`go vet`/`shellcheck -S error`/`bash -n`
clean throughout. Scratch-tagged builds
(`styrman-caddy:verify-round2-20260901`,
`styrman-server:verify-round2-20260901`) confirmed the Caddy binary
resolves to `v2.11.4` with all four plugins present and the server builder
stage resolves to `go1.25.13` — no production tag or container touched by
verification (ADR-052 honored throughout, including the root-run updater
guard matrix, all against scratch `/tmp` paths).

Honest wording, per the round-1 review's correction: `govulncheck` on each
module leaves 2 unfixable *module* findings (`docker/docker`, no upstream
fix available), not "2 findings" outright — the stdlib findings that made
up the rest (31 in the main module, 27 in the ingress module, both against
outdated local toolchains) are addressed by the 1.25.13 pin; the shipped
ingress binary was independently confirmed to build with go1.26.7.

## Honest limits

Fresh Google OAuth login smoke after the 19:40 emergency deploy was
curl-level (portal → `accounts.google.com` redirect confirmed), not a full
browser token exchange. LAN-unreachability of the admin port is asserted
structurally (the loopback bind + the guard) rather than probed live from
a second host on the network. Both are reasonable for this batch's scope
but are not the same claim as "verified end to end" — the v1.16.1
release's own verifier pass is what closes 🔴-2's live half (see §1).
