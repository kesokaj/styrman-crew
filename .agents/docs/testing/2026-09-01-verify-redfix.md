# Verification — redfix / boot-race self-heal / ADR-052 / no-mutable-tags (2026-09-01)

## Status: FAIL

Scope: the full uncommitted working tree described in the session context —
redfix of the two 🔴 audit findings (unbounded log buffer, unauthenticated
Caddy admin API), the boot-race watcher self-heal, the ADR-052 updater
state-dir hardening, and the operator no-mutable-image-tags rule. Gates the
v1.16.1 release.

Everything RUNS clean — every test, every build, both self-hosted suites, the
shell scripts, the updater guard behavior. The FAIL is a single coverage gap:
**`internal/config/config.go`'s new "STYRMAN_IMAGE is required" validation in
`Load()` has zero test coverage** — no test in the repository calls
`config.Load()` at all, success or failure path. This is exactly the check
the task asked me to make ("check internal/config tests actually exercise the
new error"), and it does not.

## Commands run

### 1. Main module

```
gofmt -l internal/orchestrator deploy/caddy/ingress internal/config deploy/update
```
→ no output (clean).

```
go vet ./...
```
→ no output (clean). `go version go1.25.0 linux/amd64`.

```
go test ./... -race -count=1
```
→ all packages `ok`, no failures:
```
ok  	.../cmd/styrman	1.028s
?   	.../cmd/wsprobe	[no test files]
ok  	.../internal/backup	1.032s
ok  	.../internal/config	1.014s
ok  	.../internal/conversation	1.026s
ok  	.../internal/credbroker	1.033s
ok  	.../internal/credstore	1.017s
ok  	.../internal/files	1.023s
ok  	.../internal/gitrepo	1.036s
ok  	.../internal/history	1.015s
ok  	.../internal/httpapi	1.096s
?   	.../internal/identity	[no test files]
ok  	.../internal/links	1.031s
ok  	.../internal/orchestrator	1.017s
ok  	.../internal/project	1.014s
ok  	.../internal/terminal	1.015s
ok  	.../internal/userstore	1.012s
?   	.../web	[no test files]
```
`go test ./... -race -count=1 -v | grep -c '^--- PASS'` → **105** individual
`t.Run`/`func Test*` passes, 0 failures.

### 2. Ingress module (`deploy/caddy/ingress`, separate `go.mod`)

```
cd deploy/caddy/ingress && go vet ./...
```
→ clean.

```
go test -race -count=1 ./...
```
→ `ok  	github.com/spgo/devbox/caddy/ingress	3.090s`.
`-v | grep -c '^--- PASS'` → **46** passes, 0 failures.

### 3. Shell scripts

```
bash -n deploy/update/styrman-update    → syntax-ok
bash -n deploy/host-setup.sh            → syntax-ok
shellcheck -S error deploy/update/styrman-update deploy/host-setup.sh
```
→ exit 0, no findings at error severity. shellcheck 0.10.0.

### 4. Updater guard behavior (scratch only — ADR-052 hard constraint honored)

Ran as root via `sudo` per the task; every state dir used was either unset or
a fresh `mktemp -d` under `/tmp`. `/var/lib/styrman` and `/etc/styrman` were
never touched.

**a) No `STYRMAN_UPDATE_STATE_DIR`:**
```
sudo env -i PATH="$PATH" bash deploy/update/styrman-update --check
```
→
```
[styrman-update] FATAL: STYRMAN_UPDATE_STATE_DIR is not set — production:
systemctl start styrman-update.service (or pass /etc/styrman/update
explicitly); tests: pass a scratch prefix, e.g. $(mktemp -d) (ADR-052)
```
exit 1. **PASS** (expected refusal).

**b) `STYRMAN_UPDATE_STATE_DIR=/var/lib/styrman/x`:**
```
sudo env -i PATH="$PATH" STYRMAN_UPDATE_STATE_DIR=/var/lib/styrman/x bash deploy/update/styrman-update --check
```
→
```
[styrman-update] FATAL: STYRMAN_UPDATE_STATE_DIR=/var/lib/styrman/x is
inside /var/lib/styrman — refusing (ADR-052)
```
exit 1. **PASS** (expected refusal).

**c) Scratch dir `$(mktemp -d)`:**
```
SCRATCH=$(mktemp -d /tmp/styrman-verify-updater-XXXXXX)
sudo env -i PATH="$PATH" STYRMAN_UPDATE_STATE_DIR="$SCRATCH" bash deploy/update/styrman-update --check
```
→
```
[styrman-update] FATAL: no deploy clone at /opt/styrman (see host-setup.sh step 8c)
```
exit 1 — the guard passed and execution reached the *next* check
(`DEPLOY_DIR="${STYRMAN_DEPLOY_DIR:-/opt/styrman}"`), proving guard ordering
is correct. **PASS** (this is the documented expected next failure).
Scratch dir removed with `sudo rm -rf "$SCRATCH"` immediately after.

### 5. Changed Dockerfiles — scratch tags only

```
docker build -t styrman-caddy:verify-20260901 deploy/caddy
```
→ succeeds. `xcaddy build --with github.com/greenpau/caddy-security@v1.1.64
--with github.com/caddy-dns/cloudflare@v0.2.4 --with
github.com/mholt/caddy-ratelimit@v0.1.0` against `caddy:2.11.4-builder`
resolves and compiles (`caddy version` → `v2.11.4`). Runtime stage
`caddy list-modules | grep -E "(security|cloudflare|rate_limit|ingress)"`
prints all four: `dns.providers.cloudflare`, `http.handlers.rate_limit`,
`ingress`, `security`. Image tagged `styrman-caddy:verify-20260901` only —
`styrman-caddy:latest` / `:pre-2.11.3` untouched.

```
docker build -f deploy/Dockerfile.server --build-arg STYRMAN_VERSION=verify -t styrman-server:verify-20260901 .
```
→ succeeds (context = repo root, `golang:1.25.3-trixie` builder). Image
tagged `styrman-server:verify-20260901` only — `styrman-server:latest`
untouched.

ARM64: out of scope per the v1.16.0 release entry (AMD64-only single-host
target) — not attempted, no QEMU cross-build run.

### 6. Compose config rendering — versioned tags required

```
STYRMAN_VERSION=9.9.9 docker compose -f deploy/docker-compose.yml config | grep image:
```
→
```
    image: styrman-caddy:9.9.9
    image: styrman-server:9.9.9
```
**PASS.**

```
env -i PATH="$PATH" HOME="$HOME" docker compose -f deploy/docker-compose.yml config
```
(STYRMAN_VERSION unset) → exit 1:
```
error while interpolating services.caddy.image: required variable
STYRMAN_VERSION is missing a value: set STYRMAN_VERSION=X.Y.Z — mutable
tags are banned
error while interpolating services.styrman.image: required variable
STYRMAN_VERSION is missing a value: set STYRMAN_VERSION=X.Y.Z — mutable
tags are banned
```
**PASS** (refuses as required).

### 7. Self-hosted suites

```
bash .agents/docs/testing/styrman/integration.sh
```
(default image `styrman-runtime:test`, isolated state dir under `/tmp`, port
18080) → **passed=57 failed=0 skipped=3** (3 skips are the documented
host-firewall preview-proxy cases, "prod unaffected"). Matches the expected
57/0/3.

```
bash web/e2e/run.sh
```
(builds `web/` SPA, then the server, self-hosts on loopback port 18090,
isolated state dir under `/tmp`) → **passed=48 failed=0 skipped=2**, plus the
final "no uncaught client errors" gate PASS. Matches the expected 48/2.
This run rewrote tracked `web/dist/client/*` with fresh content-hashed asset
names (documented, expected build-output dirt, not stray edits).

Kata prereq verified first: `docker run --runtime mvm --rm debian:trixie
uname -r` → `6.18.28`.

## Coverage of changed code

| Changed behavior | Covering test | Verdict |
|---|---|---|
| `tailBuffer` (bounded tail retention, drop accounting) | `internal/orchestrator/tailbuffer_test.go`: `TestTailBufferBoundsHostileOutput`, `TestTailBufferKeepsTheNewest`, `TestTailBufferUnderCap`, `TestTailBufferBoundaryDoesNotEatALine` | covered |
| `trimLogTail` (banner, partial-first-line strip, boundary flag) | `TestTrimLogTail` (6 cases: verbatim, empty×2, mid-line strip, boundary keep, no-newline fragment) | covered |
| `Orchestrator.Logs` post-processing wiring | exercised transitively via `trimLogTail`/`tailBuffer` unit tests + `integration.sh`'s live logs paths (transcripts/terminal I/O) exercising `ContainerLogs`; no direct unit test of `Logs()` itself, matches the prior review's own risk note | covered (functional, not unit-isolated) |
| the exact-boundary fix (finding 3: single write of exactly `max` bytes into an EMPTY buffer must NOT set `dropped`) | **none.** No test writes exactly `max` bytes to a fresh `tailBuffer{}` and asserts `Dropped()==false` immediately after. `TestTailBufferBoundaryDoesNotEatALine`'s `b3` does an exact-`max` first write but never asserts `b3.Dropped()` before the second write. | **gap — low severity, non-blocking** (see below) |
| `isLoopbackAddr` (port-required, IPv4/IPv6/`localhost`, martian handling) | `deploy/caddy/ingress/loopback_test.go`: `TestIsLoopbackAddr` (5 loopback shapes, 9 rejected shapes incl. bare-host-no-port) | covered |
| generated `admin { origins }` block presence/honesty | `TestGeneratedConfigNamesAdminOrigins` | covered |
| `cfg.AdminListen` single-source-of-truth wiring (`cmd.go`: `adminAddr := cfg.AdminListen`, loopback guard in `loadConfig`) | exercised by `loopback_test.go` (guard logic) + live smoke test recorded in the prior review (`docker build` + restart + curl checks) | covered |
| watcher retry-forever-on-rejection vs. fatal-after-N-transport-failures | `deploy/caddy/ingress/watcher_test.go`: `TestReloadCaddyRejectedLoadIsAnErrorButNotUnreachable`, `TestReloadCaddySuccess`, `TestReloadCaddyTransportFailureIsUnreachable`, `TestConsecutiveUnreachableTurnsFatalButRejectionsDoNot` (incl. counter reset on a subsequent rejection) | covered |
| `config.Load()` — `STYRMAN_IMAGE` now REQUIRED, no `:latest` fallback | **none.** `grep -rl 'config\.Load\b' --include=*_test.go` and `grep -rl STYRMAN_IMAGE --include=*_test.go` across the whole repo both return nothing. `internal/config/config_test.go` never calls `Load()` — every existing test constructs `&Config{}` directly (`testCfg`) and exercises only `SetOperator`/`loadOverrides`, bypassing `Load()` entirely (and its unrelated pre-existing validations too — this is not a new hole in test *infrastructure*, but the new error path rides on it untested). | **gap — FAILS this run** |
| `deploy/update/styrman-update` ADR-052 guard (unset → refuse; inside `/var/lib/styrman` → refuse; scratch → proceed) | no Go/bash unit test exists; verified live in this run (§4) per the task's explicit instruction to test it that way, not via an automated test the task asked me to write (I don't write tests) | verified this run, but note: this remains **unautomated** — a future change could silently break the guard with nothing to catch it in CI |
| compose `STYRMAN_VERSION`/`STYRMAN_IMAGE` required-var wiring | no automated test (compose config isn't Go-testable in this repo); verified live in this run (§6) | verified this run, same unautomated caveat as above |
| Dockerfile version pins (caddy 2.11.4, golang 1.25.3, plugin versions) | no automated test; verified by the builds in §5 | verified this run |

## Environment

Tier: local only (no remote/cloud requested). OS: Linux 6.12.107+deb13-amd64
(host), Docker Engine 29.7.2 (plain Docker, no Colima — Linux host). Kata
`mvm` runtime confirmed working. `go version go1.25.0 linux/amd64` (the
toolchain itself; the pinned `golang:1.25.3-trixie` build image is what
ships, verified via the Dockerfile.server build in §5). Node v20.19.2 / npm
9.2.0 for the web/e2e tier. shellcheck 0.10.0.

Scratch artifacts created and left in place (all clearly non-production
tags, no cleanup requested by the task):
`styrman-caddy:verify-20260901`, `styrman-server:verify-20260901`. No
production container (`styrman-server`, `styrman-caddy`) was started,
stopped, or rebuilt by this run. `/var/lib/styrman` and `/etc/styrman` were
never touched.

Out-of-scope observation: a **concurrent documentation-agent update** landed
mid-run (files under `.agents/docs/specs/styrman/`, `.agents/docs/testing/
styrman/README.md`, `AGENTS.md`, `README.md`, `.claude/agent-memory/
documentation/*`, plus a new `.agents/docs/specs/styrman/versioning.md`, all
timestamped 19:57–19:58 CEST during this session). Not authored by this
verification run; noted here only because `git status` now shows more dirty
files than the session-context diff listed. Confirmed docs-only (spot-diffed
`.agents/docs/testing/styrman/README.md`); did not change any command or
expected count this log relies on, and both self-hosted suites (§7) were run
against the actual working-tree scripts, not the docs.

## Blocking findings

1. **`internal/config/config.go`'s new "`STYRMAN_IMAGE` required" validation
   in `Load()` has no covering test anywhere in the repository.** Neither the
   success path (`Load()` with `STYRMAN_IMAGE` set → no error) nor the new
   failure path (`Load()` with it unset → the specific error message) is
   exercised. `internal/config/config_test.go` never calls `Load()` at all.
   This is changed, security/availability-relevant behavior (a missing env
   var now hard-fails server startup instead of silently booting
   `styrman-runtime:latest`) shipping with zero regression coverage — per the
   constitution, changed code without a covering test is a FAIL, no
   exceptions.

## Non-blocking findings (do not gate this run, but should be tracked)

2. **`tailBuffer`'s exact-boundary fix (review finding 3) has no dedicated
   regression test.** The code fix is verifiably present (`cut==0` combined
   with `len(t.buf)==0` sets neither `lastDrop` nor `dropped`), and the
   surrounding suite is otherwise thorough, but no test pins "single write of
   exactly `max` bytes into an empty buffer → `Dropped()==false`" — the exact
   scenario the prior review's differential test caught as the one spot the
   old code diverged from the reference. A one-line regression test would
   close this permanently; right now a future refactor could silently
   reintroduce the spurious-truncation bug with the full suite still green.
3. The ADR-052 updater guard and the compose `STYRMAN_VERSION`/`STYRMAN_IMAGE`
   required-var behavior are verified only by hand in this log, not by any
   automated test — every future change to either relies on a human re-running
   §4/§6 exactly. Not a new gap (bash/compose here have no existing unit-test
   harness in this repo) but worth naming since both are now load-bearing
   safety controls.
