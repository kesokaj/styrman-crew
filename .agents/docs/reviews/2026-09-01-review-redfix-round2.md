# Review — redfix round 2 (working tree ahead of v1.16.1)   (2026-09-01)

Reviewer: `reviewer` subagent, fresh context. Author of the changes: the main agent of
today's post-reboot session — not this context.

Scope: the entire uncommitted working tree at HEAD `00d1c18`.

```
 M deploy/.env.example                    M deploy/caddy/ingress/generator.go
 M deploy/Dockerfile.server               M deploy/caddy/ingress/go.mod|go.sum
 M deploy/caddy/Dockerfile                M deploy/caddy/ingress/watcher.go
 M deploy/caddy/ingress/cmd.go            M deploy/docker-compose.yml
 M deploy/host-setup.sh                   M internal/config/config.go
 M deploy/update/styrman-update           M internal/orchestrator/exec.go
 M deploy/update/styrman-update.service   M internal/orchestrator/orchestrator.go
?? deploy/caddy/ingress/loopback.go       ?? deploy/caddy/ingress/loopback_test.go
?? deploy/caddy/ingress/watcher_test.go   ?? internal/orchestrator/tailbuffer_test.go
```

## Verdict: REQUEST CHANGES

**Worst first: `deploy/update/styrman-update:95` will `chmod 0700 root:root` the PARENT of
whatever state dir it is handed — and the change that mandates handing it one prints
`$(mktemp -d)` as the recommended value.** `install -d -m 700 -o root -g root "$(dirname
"${STATE_DIR}")"` rewrites mode and ownership of a directory that *already exists*; I
measured it (`1777 kesokaj` → `700 kesokaj` on an existing dir, sticky bit gone). With
`STYRMAN_UPDATE_STATE_DIR=$(mktemp -d)` the dirname is `/tmp`, so a single root-run
"dry run, changes nothing" turns `/tmp` into `drwx------ root root` and takes every
non-root process on the host with it. With the operator's own new hint minus one path
component (`/etc/styrman` instead of `/etc/styrman/update`) it does the same to `/etc`.
Both errors are suppressed by `2>/dev/null || true`, so nothing is even logged. This is a
state-destroying foot-gun created by the change whose stated purpose is preventing
state-destroying privileged test runs (ADR-052), and it fires on the exact recipe the new
`fail` message and ADR-052 both recommend. It blocks the commit.

Second: **the release the operator is about to cut cannot boot a project.** Group E made
`STYRMAN_IMAGE` derive from `styrman-runtime:${STYRMAN_VERSION}` and made it mandatory in
`config.Load`, but nothing in the manual release path or in `host-setup.sh` builds a
version-tagged runtime image. `host-setup.sh:312` still builds the mutable
`styrman-runtime:latest`, and `docker compose up -d --build` builds only `caddy` and
`styrman`. Locally only `styrman-runtime:1.16.0` exists; a `STYRMAN_VERSION=1.16.1` deploy
yields a healthy server whose every project create fails with "No such image". R11's fresh
install is broken in the same way and by the same gap.

Everything the prior review (`2026-09-01-review-redfix.md`) asked for is genuinely done —
all eleven findings, none deferred, and the `generator.go` comment is now honest about what
`origins` does and does not do. The new watcher design is sound and its lock ordering is
clean. Its *tests*, however, do not test the thing it was written for: I deleted
`w.scheduleRetry(ctx)` outright and the whole ingress suite still passed.

## Findings

- **[high / confidence: high]** `deploy/update/styrman-update:95` — `install -d -m 700 -o
  root -g root "$(dirname "${STATE_DIR}")"` mutates an *existing* parent directory's mode
  and owner. Measured: `install -d -m 700` on an existing `1777` dir leaves it `700`
  (sticky bit dropped). Reachable on every run including `--check` (the guard and this line
  both precede the `CHECK_ONLY` exit at :292). Triggered by the two values this change
  itself recommends — `$(mktemp -d)` → chmods `/tmp`; `/etc/styrman` (a one-component typo
  off the new `host-setup.sh:393` hint) → chmods `/etc`. Failures are swallowed by
  `2>/dev/null || true`. **Fix:** never touch an existing parent — create it only when
  absent (`[ -d "$parent" ] || install -d -m 700 -o root -g root "$parent"`), or drop the
  parent line entirely and `mkdir -p` the leaf then `chmod 700`/`chown root:root` only the
  leaf. Additionally widen the refusal `case` beyond `/var/lib/styrman*`: reject any
  `STATE_DIR` whose parent is a shared system directory (`/tmp`, `/etc`, `/var`, `/`,
  `/home`), and change the `fail` message's suggested test value to a nested scratch path
  (`$(mktemp -d)/state`) so the parent is always one this script created.
- **[high / confidence: high]** `deploy/host-setup.sh:312` + `deploy/docker-compose.yml:89`
  + `internal/config/config.go:203` — the runtime image is now required, versioned, and
  built by nobody on the manual path. `host-setup.sh` builds `styrman-runtime:latest`;
  compose passes `STYRMAN_IMAGE=styrman-runtime:${STYRMAN_VERSION}`; `config.Load` hard-fails
  only on *empty*, not on *missing image*, so the server comes up healthy and every project
  create dies at `orchestrator.go:754`. Only `styrman-update:319` derives and builds
  `styrman-runtime:${ver}`. Verified: `docker images` carries `styrman-runtime:{1.16.0,dev,
  latest,test}` — a `STYRMAN_VERSION=1.16.1` deploy references a tag that does not exist.
  **Fix:** make `host-setup.sh` build the version-tagged image (same derivation as
  `styrman-update`), and add the `docker build -t styrman-runtime:X.Y.Z --build-arg
  STYRMAN_VERSION=X.Y.Z image/` step to the release recipe in `README.md:130-134`,
  `AGENTS.md:34-39`, `deploy/README.md:12` and `.agents/docs/specs/styrman/deploy.md:36`.
  Blocks the v1.16.1 release, not the commit.
- **[medium / confidence: high]** `deploy/caddy/ingress/watcher_test.go` — test theatre on
  the headline behavior. I mutated `noteReloadFailure` to drop `w.scheduleRetry(ctx)`
  entirely (a scratch copy of the module) and `go test ./...` still reported `ok`. The
  retry loop *is* the fix for the 2026-09-01 outage and nothing goes red if it disappears.
  Same for the `Start()` "retry scheduled" path and for `scanAndReload`'s new `ctx.Err()`
  early return. **Fix:** with `retryDelay` already injectable, assert the observable
  decision — after `noteReloadFailure(ctx, <400 err>)`, `w.retryTimer != nil`; after the
  fatal-limit failure, `w.retryTimer` was NOT re-armed. Better still, give `Watcher` a
  one-method lister seam so a short `retryDelay` can drive a real second `scanAndReload`
  against an `httptest` admin that 400s once then 200s.
- **[medium / confidence: high]** `internal/orchestrator/tailbuffer_test.go` — the prior
  review's finding 3 (spurious `dropped` on a single write of exactly `max` into an empty
  buffer) is correctly fixed in `exec.go:39-44`, but no test covers it. I reverted the fix
  to unconditional `t.dropped = true` in the `n >= t.max` branch and
  `go test ./internal/orchestrator/` still passed: no case writes exactly `max` bytes into
  an empty buffer. Standing project rule is "changed code without covering tests = verifier
  FAIL". **Fix:** one case — `b := &tailBuffer{max: 8}; b.Write([]byte("12345678"));
  assert !b.Dropped()`.
- **[medium / confidence: high]** `deploy/host-setup.sh:475` and `README.md:132` —
  `STYRMAN_VERSION=$(git describe --tags --always)` produces `v1.16.0` (verified: the repo's
  tags are `vX.Y.Z`), while `styrman-update:388` deploys with `${LATEST#v}` = `1.16.0`. The
  same release therefore lands under two image-tag namespaces (`styrman-server:v1.16.0` vs
  `:1.16.0`) depending on who deployed it, which defeats the stated goal that "what is
  running is readable off `docker ps`". On an untagged clone `--always` yields a bare SHA,
  producing `styrman-runtime:<sha>` — never built. **Fix:** strip the `v` at both sites, or
  document `STYRMAN_VERSION=X.Y.Z` literally.
- **[medium / confidence: high]** `deploy/host-setup.sh:312,480` and `README.md:123` — the
  new no-mutable-tags rule was applied to both Dockerfiles, compose and the updater, but not
  to `host-setup.sh`, which still defaults `RUNTIME_IMAGE` to `styrman-runtime:latest` and
  tells the operator to pass `styrman-runtime:latest` to the test suites. The rule now holds
  everywhere except the one script R11 says must install the whole system from scratch.
- **[medium / confidence: medium]** `deploy/caddy/ingress/go.mod:6` — the floor is
  `caddy/v2 v2.11.3` but the Dockerfile now pins `caddy:2.11.4-builder`/`caddy:2.11.4`, and
  MVS resolves the shipped binary to **2.11.4** (verified below). The module's own tests and
  `go vet` therefore compile against a version that never ships, exactly the gap the pinning
  change exists to close. **Fix:** `go get github.com/caddyserver/caddy/v2@v2.11.4 && go mod
  tidy` so the module floor and the image pin move together.
- **[low / confidence: medium]** `deploy/caddy/ingress/watcher.go:181-191` — the boot-race
  self-heal covers only the *reload* leg. A failure in `discoverApps` (docker API hiccup,
  daemon reload) returns before `noteReloadFailure` and schedules nothing, so the ingress
  again sits on the last config until the next container event. `watchEvents` reconnects the
  event stream but never rescans after reconnecting. Unlikely at boot (dockerd must be up to
  have started this container) but it is the same wedge shape the change is closing.
  **Fix:** route the `discoverApps` error through `scheduleRetry` too.
- **[low / confidence: high]** `deploy/caddy/ingress/watcher.go:29,256-271` — rejected loads
  retry forever at a fixed 15 s with no backoff, no jitter, and no escalation. Correct
  behaviour for the transient boot race; for a persistent rejection (bad OAuth credentials,
  a Caddyfile.base the adapter refuses) it is a silent site-down that regenerates and POSTs
  the whole config every 15 s indefinitely and never surfaces above `log.Error`. Consider a
  capped backoff and a distinct log line once N retries have failed, so the state is
  greppable.
- **[low / confidence: medium]** `deploy/docker-compose.yml:50` sets
  `CADDYFILE_BASE_PATH=/etc/caddy/Caddyfile.base`, and `generator.go:37-48` *prepends* that
  user-maintained file. A base file carrying its own global-options or `admin` block now
  collides with the newly emitted `admin` directive; the adapter rejects the duplicate, the
  load 400s, and the new behaviour is a permanent 15 s retry loop rather than a one-shot
  failure. Worth one line in `deploy/.env.example` next to the `CADDY_ADMIN` note.
- **[low / confidence: high]** `deploy/caddy/ingress/loopback_test.go:39-42` — the doc
  comment promises the block "must never be emitted for a non-loopback addr", which no
  assertion checks (the only guard is in `loadConfig`, a different file). Also untested:
  `isLoopbackAddr("127.0.0.1:")`, the case the explicit `port == ""` clause exists for.
  Either add the cases or drop the claim — the prior review's whole first finding was a
  comment promising more than the code delivered.
- **[low / confidence: high]** `deploy/caddy/ingress/watcher.go:44-67` — `Watcher` holds a
  concrete `*client.Client`, so `scanAndReload` (and therefore the entire retry loop) is
  untestable and the tests compensate by reaching into `adminDown` and `retryTimer`
  directly. A two-method seam declared in this package (`ContainerList` + `ContainerInspect`)
  would make the retry loop testable for real and is the root cause of the test-theatre
  finding above. Note also that `adminDown` is documented "guarded by scanMu" but the tests
  mutate and read it without holding it — benign single-goroutine usage, `-race` clean, but
  it erodes the invariant.
- **[low / confidence: high]** documentation row is outstanding and this diff created most
  of it. `TODO.md:120` ("Production pins the mutable `styrman-runtime:dev` tag") is closed by
  this change and still open in the file; `README.md:123,132`, `AGENTS.md:34-39`,
  `.agents/docs/specs/styrman/guide.md:298`, `deploy/README.md:12` and
  `.agents/docs/specs/styrman/deploy.md:36,278` all still describe the mutable-tag world, and
  several of their commands now hard-fail (`docker compose` without `STYRMAN_VERSION` aborts
  — verified). A new ADR is also owed for the no-mutable-tags rule and for the watcher's
  fatal-exit contract; neither is recorded anywhere but code comments. `changes/`,
  `testing/` and the `security/` note are likewise not yet on disk.

## What I verified myself (not taken on report)

| Check | Result |
|---|---|
| `gofmt -l internal deploy/caddy/ingress`, `go vet ./...` (both modules) | clean |
| `go test -count=1 ./...` (main module), `go test -race -count=1 ./...` (ingress) | all pass |
| `bash -n deploy/update/styrman-update`, `bash -n deploy/host-setup.sh` | clean |
| `install -d -m 700` against an existing `1777` dir | mode → `700`, sticky bit lost — finding 1 |
| Mutation: delete `w.scheduleRetry(ctx)` from `noteReloadFailure` (scratch module copy) | ingress suite still `ok` — finding 3 |
| Mutation: revert `tailBuffer`'s exact-boundary `dropped` fix (scratch repo copy) | `internal/orchestrator` still `ok` — finding 4 |
| Compose interpolation, scratch copy, `STYRMAN_VERSION=1.16.1` | `styrman-caddy:1.16.1`, `styrman-server:1.16.1`, `STYRMAN_IMAGE=styrman-runtime:1.16.1` — nested `${VAR:-${OTHER}}` resolves correctly |
| Compose without `STYRMAN_VERSION` | `error while interpolating services.caddy.image: required variable STYRMAN_VERSION is missing a value` — the `:?` guard works |
| `deploy/.env.example` parsed as a compose env file | parses; the indented `#` continuation lines under `CADDY_ADMIN` are treated as comments, `CADDY_ADMIN: ""` |
| `caddy build-info` on `styrman-caddy:latest` (the live 16:49 image) **and** on the pinned rebuild `styrman-caddy:verify-20260901` | **identical**: `caddy/v2 v2.11.4`, `caddy-security v1.1.64`, `caddy-dns/cloudflare v0.2.4`, `caddy-ratelimit v0.1.0`, `go1.26.7`. The Dockerfile's claim that the pins are "what the smoke-tested build resolved" is **true**, and the shipped ingress binary is built with go1.26.7 — the prior review's 27 stdlib `govulncheck` findings do not ship |
| Running production container | `styrman-caddy` runs image `sha256:d26f66be` = `styrman-caddy:latest`, built 16:49 — i.e. it carries groups A/B but **not** the watcher retry work; that code has never been compiled into an image |
| `caddycmd.WrapCommandFuncForCobra` + `cmd.Main()` (`caddy/v2@v2.11.3`) | `status==1` with a non-nil error → `os.Exit(1)`; compose `restart: unless-stopped` restarts regardless of exit code. The new `Fatal()` exit contract does what it claims |
| `styrman-update` guard ordering | nothing filesystem-touching between the `STATE_DIR` guard (:72-83) and `install -d` (:95) — the ordering itself is correct |
| Live `deploy/.env` | `STYRMAN_IMAGE` already commented out with the operator-rule note — the running deployment will not silently keep `:dev` |
| `config.Load` callers | only `cmd/styrman/main.go:31`; `integration.sh:55` and `web/e2e/run.sh:53` both set `STYRMAN_IMAGE` explicitly, so the new requirement breaks no suite |
| `httpapi/server.go:702 (cfg.Image == "")` after the change | still live — handler-level tests build `Config` directly, not via `Load`. Not dead |
| Lock ordering in `watcher.go` | `scanMu → retryMu` everywhere; `time.AfterFunc` callbacks run on fresh goroutines, no reentrancy, no inversion, no deadlock. `caddyfileMu` and `debounceMu` are leaves |
| `adminDownLimit` reachability | reachable: a failed reload never latches `lastCaddyfile`, so each retry re-attempts the same POST — 4 consecutive transport failures ≈ 45 s, then exit 1 |

## Prior review (`2026-09-01-review-redfix.md`) — item by item

| # | Prior finding | Status |
|---|---|---|
| 1 | `origins` comment/test name assert a guarantee Caddy does not give | **Fixed.** `generator.go:205-215` now says plainly that `origins` is browser-only defense-in-depth, names the loopback bind + 2.11.3 floor as the real controls, and warns against reading it as `/load` authentication. Test renamed `TestGeneratedConfigNamesAdminOrigins`. `enforce_origin` correctly NOT added alone |
| 2 | Dockerfile untouched; floating builder + floating plugins; `EXPOSE 2019` | **Fixed.** Pinned `caddy:2.11.4-builder`/`caddy:2.11.4`, `caddy-security@v1.1.64`, `cloudflare@v0.2.4`, `caddy-ratelimit@v0.1.0`; `EXPOSE 2019` dropped. Pins verified byte-accurate against the live binary (table above) |
| 3 | `dropped` set on a single write of exactly `max` | **Fixed** in code (`exec.go:39-44`), **not covered by a test** — finding 4 |
| 4 | trim always strips the first line, eating a complete one | **Fixed** properly: `lastDrop` + `StartsAtLineBoundary()`, and `TestTailBufferBoundaryDoesNotEatALine` covers the exact-newline case |
| 5 | no test for `Logs`'s post-processing | **Fixed.** `trimLogTail` extracted, pure, and table-tested including the full-buffer and no-newline shapes |
| 6 | hand-rolled `contains()` in `loopback_test.go` | **Fixed** — `strings.Contains` |
| 7 | `adminAddr` computed by a second env read | **Fixed** — `cmd.go:48 adminAddr := cfg.AdminListen`, single source, comment explains why |
| 8 | loopback guard accepted a bare host → would POST to :80 | **Fixed** — port required, `"127.0.0.1"` and `"::1"` moved into the reject list |
| 9 | `helpers_loopback.go` naming | **Fixed** — renamed `loopback.go`, pairs with `loopback_test.go` |
| 10 | govulncheck record: 27 of 29 were stdlib, not module | **Fixed at the artifact level**, better than asked: `Dockerfile.server` pins `golang:1.25.3-trixie` and the ingress binary already builds on go1.26.7. Still needs saying in the changelog |
| 11 | `.env.example` `CADDY_ADMIN` under-documented | **Fixed** — the crash-loop consequence is now spelled out at the knob |
| — | `web/dist/client/_shell.html` timestamp churn | Still dirty in the tree, still an open decision. Confirm it is deliberate before `git add -A` |

## Criteria

1. **Dead code** — pass. `RUNTIME_IMAGE` is fully removed from `styrman-update` (no residual
   references; the surviving comment is documentation, not code). `getEnvOrDefault`,
   `errAdminUnreachable`, `Fatal`, `StartsAtLineBoundary`, `trimLogTail`, `isLoopbackAddr`
   all reached. `httpapi/server.go:702` is not dead (test-only path, comment is accurate).
   The `if cfg.AdminListen != ""` at `generator.go:213` can never be false in production but
   is legitimately load-bearing for the generator tests. Nothing to delete.
2. **Flow tracing** — pass with one real break. `AdminListen` now flows from one validated
   value into the bind, the reload URL and the emitted `origins` (the prior finding 7 hop is
   closed); `tailBuffer` → `trimLogTail` → `Logs` is consistent and total; `Fatal()` →
   `cmdIngress` → exit 1 → restart policy is verified end-to-end against the vendored caddy
   CLI. The break is out of the Go code: `STYRMAN_VERSION` → compose → `STYRMAN_IMAGE` →
   `config.Load` → `orchestrator.create` has no producer for the runtime image on the manual
   or fresh-install path (finding 2). Errors are wrapped throughout; `ctx` reaches every new
   goroutine and every timer callback checks `ctx.Err()`. Pre-existing and unchanged:
   `_, _ = stdcopy.StdCopy(...)` still discards its error.
3. **Standards conformance** — pass for Go. gofmt/vet/tests green in both modules, verified
   here. Comments explain *why*. The shell side is where conformance slips: `install -d` on a
   caller-supplied parent (finding 1) and the mutable `:latest` default that survived the
   operator's own new rule (finding 6).
4. **Directory layout** — pass. `loopback.go`/`loopback_test.go` now pair; `watcher_test.go`
   sits beside `watcher.go`; `tailbuffer_test.go` beside `exec.go` (slight name drift from
   `exec_test.go`, but it names the type under test — acceptable). The `internal/` ↔
   vendored-ingress module boundary is respected; the duplicated `isLoopbackAddr` is correct
   there, because R11 forbids the ingress depending on the main module.
5. **Simplicity, DIY-except-security** — pass. No new dependencies; the `go.mod` delta is a
   net reduction. No hand-rolled auth/crypto — OAuth/JWT stay entirely with `caddy-security`,
   and the change moves toward the vetted library by pinning it to the exact verified
   version. `tailBuffer`, `trimLogTail` and `isLoopbackAddr` are all small stdlib-only
   solutions. The retry/fatal machinery is more state than the rest of the file, but every
   piece buys an observable behaviour that a real outage demanded.
6. **Decoupling** — pass with one note. `tailBuffer`/`trimLogTail` is a deep, self-contained
   module. `AdminListen` threads through the existing `GeneratorConfig` seam. The one shallow
   spot is `Watcher`'s hard dependency on `*client.Client`, which forces the tests to poke at
   unexported fields and is why the retry loop cannot be tested (findings 3 and 12). Two
   engineers could still work `exec.go`, `watcher.go` and `styrman-update` in parallel.

## Does anything block?

**The commit: YES — finding 1.** `install -d -m 700 -o root -g root "$(dirname
"$STATE_DIR")"` is a root-run chmod of a caller-chosen system directory, triggered by the
value this same change tells testers and operators to pass, and silenced by `|| true`. It
belongs to the ADR-052 remediation itself, it is three lines to fix, and committing it puts
"privileged tests must not touch production paths" into the record next to a line that
chmods `/tmp` to `0700 root:root`. Findings 3 and 4 (both mutation-proven) should ride along
in the same commit rather than becoming a verifier FAIL under the project's own
"changed code without covering tests" rule.

**The v1.16.1 release: YES — findings 1, 2, 5, 6 and the documentation row.** Finding 2 in
particular breaks the deploy the operator is about to run: nothing builds
`styrman-runtime:1.16.1`, and the failure surfaces as a healthy server on which no project
will start. Finding 6's `go.mod` floor should move to 2.11.4 before the release so the tests
gate what actually ships. And the live ingress still runs the 16:49 binary — the watcher
retry/fatal code in this tree has never been compiled into an image, so the verifier's build
+ live smoke test (fresh Google login, a start/stop cycle producing a successful reload,
`localhost:2019` reachable from the host and refused from the LAN) is still owed on the new
image before the red can be called closed a second time.

**Not blocking:** findings 7-12. They are real, they should be filed, and none of them
regress anything shipping today.

## Addendum — concurrent documentation work (noticed after the findings were written)

While this review was running, a documentation pass landed in the same tree: `README.md`,
`AGENTS.md`, `.agents/docs/specs/styrman/{api,deploy,guide}.md`,
`.agents/docs/testing/styrman/README.md`, and a new
`.agents/docs/specs/styrman/versioning.md`. It closes most of finding 12 (the doc row) and
the *documentation* half of finding 2: `README.md` now builds
`styrman-runtime:${STYRMAN_VERSION}` before `docker compose up`, and reframes
`host-setup.sh`'s `:latest` build as a one-time Kata smoke test that production never runs.

What that pass does **not** change, and what therefore still stands:

- **Finding 2 (code half) — still open.** `deploy/host-setup.sh:312` still defaults to
  `styrman-runtime:latest`, and its own "next steps" block at `:475` still tells the
  operator to run `docker compose up -d --build` with **no** runtime-image build at that
  version. A fresh R11 install that follows the script's own printed instructions still ends
  with a healthy server and zero bootable projects. The README being right does not fix the
  script that prints the wrong recipe.
- **Finding 5 — still open, and now slightly worse.** `README.md` propagates
  `$(git describe --tags --always)` into *three* places (`docker build -t
  styrman-runtime:${STYRMAN_VERSION}`, the build-arg, and compose), so a tagged clone
  produces `styrman-runtime:v1.16.0` / `styrman-server:v1.16.0` while `styrman-update`
  produces `:1.16.0`. Internally consistent per path, mutually inconsistent across paths —
  the same release under two tag namespaces. An untagged clone yields a bare SHA.
- **Finding 1 — untouched and still the commit blocker.** `host-setup.sh:393` now prints
  `sudo STYRMAN_UPDATE_STATE_DIR=/etc/styrman/update styrman-update --check`, which is the
  hint I flagged: correct as written, catastrophic one path component short.

The doc pass was not reviewed here — it is outside the change set I was dispatched against,
and it arrived mid-review. It needs its own reviewer pass before the commit.

---

# Round 3 — delta re-review of the fixes   (2026-09-01)

Reviewer: `reviewer` subagent, fresh context (same reviewer role as round 2, not the
author of any of these changes). Scope: only whether the round-2 blocking findings and
the security report's blocking item are actually closed in the tree, plus fresh eyes on
the code the fixes themselves introduced. Tree at HEAD `00d1c18`, still uncommitted.

## Verdict: APPROVE WITH NITS (commit) — one medium regression to fix before the v1.16.1 tag

**Every blocking finding is genuinely closed, and I proved four of them by mutation
rather than by reading.** The `install -d` foot-gun is gone in the only way that is
actually safe (never touch an existing dir); the canonicalisation closes `//`, `..` and
symlink evasion of the `/var/lib/styrman` refusal — I ran all three shapes through the
extracted guard and all three are refused. The two "test theatre" findings are dead: the
new tests go red under the exact mutations that stayed green in round 2.

**The one thing this round broke: `deploy/host-setup.sh:317-318`. The new version
derivation runs under `set -euo pipefail`, so on a clone with no reachable tag the
pipeline's failure aborts the whole provisioning script — silently, because git's stderr
is suppressed — and the `setup-<timestamp>` fallback on the next line is unreachable
dead code.** Measured: exit 128, no output, after steps 1-6 have already installed
Docker, Kata, the firewall and the systemd units. That is a worse fresh-install failure
mode than the `:latest` tag it replaced, and it is one line to fix.

## Round-2 blocking findings — verified against the code

| # | Round-2 finding | Status (how I checked) |
|---|---|---|
| 1 | `install -d` re-modes an existing parent (`/tmp` → `0700 root:root`) | **Closed.** `:105-107` now create parent and leaf only when missing. I re-measured `install -d -m 700` on an existing `0755` dir → `700` (the finding was real), and confirmed `install -d` still applies `-m` only to the deepest component, so the explicit-parent line still buys what its comment claims (`/etc/styrman` → 700 on a fresh host, intermediates 755). Ran the extracted guard (lines 42+68-113, root check stubbed, `install` stubbed to log) against 12 inputs: `/tmp` and `/` were **never** passed to `install`, and `/tmp`'s `1777` was untouched |
| 1b | Widen the refusal / fix the recommended test value | **Closed as designed, partially.** The `fail` message now recommends `$(mktemp -d)/state` (nested — parent is one the caller created, mode 700 root). `realpath -m` before the `case` refuses `//var/lib/styrman//sub`, `/var/lib/styrman/../styrman/x`, and a symlink into `/var/lib/styrman` — all three verified refused. The refusal was **not** widened to other shared parents; see new finding 3 |
| 2 | Nothing builds the version-tagged runtime image on the manual/fresh path | **Closed.** `host-setup.sh:317-319` builds `styrman-runtime:<newest tag sans v>`; its next-steps block (`:483-487`) and `README.md:139`, `guide.md:316`, `deploy.md:167`, `versioning.md:90` all now build `styrman-runtime:${STYRMAN_VERSION}` before `compose up`. `AGENTS.md` points at `versioning.md` instead of inlining — acceptable |
| 3 | `watcher_test.go` is theatre — deleting `scheduleRetry` stayed green | **Closed, mutation-proven.** Scratch copy, deleted `w.scheduleRetry(ctx)` from `noteReloadFailure` → `TestRejectedReloadArmsRetryAndFatalDoesNot` FAILS ("a rejected reload must arm the retry timer"). Second mutation, deleted the `return` after the fatal send so the fatal path falls through to `scheduleRetry` → same test FAILS ("the fatal path must not arm a retry"). Both halves of the new test are load-bearing |
| 4 | No test for the exact-`max` `dropped` fix | **Closed, mutation-proven.** Reverted the fix to unconditional `t.dropped = true` → `TestTailBufferExactMaxSingleWriteIsNotTruncation` FAILS on two assertions (`Dropped()` and `StartsAtLineBoundary()`) |
| 5 | `git describe --tags --always` → `v`-prefixed / SHA tag namespace | **Closed in shape, regressed in robustness.** `--abbrev=0 \| sed 's/^v//'` at `host-setup.sh:317`, `:485` and `README.md:137` — all three now produce `1.16.0`, matching the updater's `${LATEST#v}`. But see new finding 1 |
| 6 | `host-setup.sh` still defaults `RUNTIME_IMAGE` to `:latest` | **Closed.** `:319` is `styrman-runtime:${SETUP_VERSION}`; the test-suite hint at `:490` no longer names `:latest`. `grep -rn ':latest'` across the repo's shipped paths returns only prose in `versioning.md` (describing the banned state) and `README.md:124` (now wrong — new finding 2) |
| 7 | `go.mod` caddy floor `2.11.3` vs the image's `2.11.4` | **Closed.** `go.mod` is `caddy/v2 v2.11.4`, tidied (the old `smallstep`/`badger`/`pgx` subtree is gone from `go.sum`); `go test -race ./...` in the ingress module passes against it |
| — | `config.Load`'s `STYRMAN_IMAGE` refusal untested | **Closed, mutation-proven.** Neutered the `if c.Image == ""` guard → `TestLoadRequiresExplicitImage` FAILS. The test also neutralises host env via `t.Setenv(k, "")`, so it is not order-dependent |
| — | `Dockerfile.server` stale Go pin | **Closed.** `golang:1.25.13-trixie` with a comment that names the actual patch spread (1.25.4→1.25.13) instead of the previous false "fixed by 1.25.2/1.25.3" |
| — | `isLoopbackAddr` accepts any port string | **Closed, mutation-proven.** Deleting the `strconv.Atoi` range check turns four of the new negative cases green-to-red (`:port`, `:0`, `:65536`, `"localhost:2019 {"`) |

## Security report — blocking item

The security report's single **high** (`install -d` on a caller-chosen parent) is closed,
by exactly the remediation it named. Of its mediums: the Go pin is fixed and the comment
corrected; the `host-setup.sh` `:latest`/`v`-prefix finding is fixed (with the new
regression below); the **caddy builder toolchain assertion + digest pinning is NOT taken**
and is recorded as deferred. Its lows: the port validation is fixed; `localhost`-by-name,
the `origins`-narrowing runbook note, and locally-mutable version tags are NOT taken
(deferred). The `/var/lib/styrman` prefix-match evasion (low) is closed by `realpath -m`.
Nothing the security report marked blocking is still open.

## New findings — the fixes' own code

- **[medium / confidence: high]** `deploy/host-setup.sh:317-318` — `SETUP_VERSION="$(cd
  "${REPO_ROOT}" && git describe --tags --abbrev=0 2>/dev/null | sed 's/^v//')"` under the
  file's `set -euo pipefail` (`:13`). `pipefail` is inherited by the command-substitution
  subshell, so on a tagless/`.git`-less tree the pipeline exits 128, the assignment exits
  128, and **`errexit` kills host-setup.sh right there** — after Docker, Kata, the
  firewall and the systemd units are already installed, and with *no message at all*
  because `2>/dev/null` ate git's error. Measured with the exact line: tagless repo →
  `exit=128` and the following line never runs; non-git dir → same; tagged repo → `1.16.0`.
  Consequence: `[ -n "${SETUP_VERSION}" ] || SETUP_VERSION="setup-$(date +%Y%m%d%H%M)"` at
  `:318` is **unreachable dead code**, and the comment above it ("A clone with no tags gets
  a unique setup-<timestamp> tag") documents behaviour that cannot happen. The prior code
  was safe precisely because it used `|| date +%Y%m%d%H%M` *inside* the substitution.
  Reachability: a normal `git clone` fetches tags, so the common R11 path works — a source
  tarball, `clone --no-tags`, or a branch with no reachable tag does not. **Fix:**
  `SETUP_VERSION="$(cd "${REPO_ROOT}" && git describe --tags --abbrev=0 2>/dev/null || true)";
  SETUP_VERSION="${SETUP_VERSION#v}"`. The same shape appears in the printed next-steps at
  `:485` (`ver=$(git -C .. describe --tags --abbrev=0 | sed 's/^v//')`), where an
  interactive shell has no errexit and instead yields `ver=""` → `docker build -t
  styrman-runtime:` → "invalid reference format"; worth a `|| { echo …; exit 1; }` there.
- **[medium / confidence: high]** `README.md:124` vs `deploy/host-setup.sh:319` — the
  quick-start now says step 1 "does a one-time SMOKE-TEST build of `styrman-runtime:latest`
  … production never runs this tag". Both halves are false as of this round: the script
  builds `styrman-runtime:<release version>`, which is **exactly** the tag compose boots.
  This is the first block a fresh-install operator reads, and it contradicts the code that
  landed in the same tree. **Fix:** restate as "builds `styrman-runtime:<newest release
  tag>` — the same tag the production compose run boots, so step 2's build is a no-op
  cache hit".
- **[medium / confidence: medium]** `deploy/caddy/ingress/watcher.go:188-191, 262-270` —
  round 2 flagged that a `discoverApps` failure schedules no retry; with the retry loop now
  in place it is worse than "one missed attempt": the loop is **self-sustaining only while
  the failures are reload failures**. A single docker-API hiccup inside a retry
  (`scanAndReload` → `discoverApps` error) returns before `noteReloadFailure`, so no new
  timer is armed and the retry chain **terminates** — the ingress is back to sitting on the
  stale config until the next container event, which is the original outage shape. Related:
  `Start()` logs "initial scan failed (retry scheduled)" unconditionally (`:216`), which is
  a lie on the `discoverApps` branch. **Fix:** route the `discoverApps`/generate error
  through `scheduleRetry` too (it is not an admin failure, so `w.adminDown` must not move).
- **[low / confidence: high]** `deploy/update/styrman-update:80-85, 110-113` — the residual
  hole after the fix, stated plainly since it was asked: `STATE_DIR=/` and `STATE_DIR=/etc`
  pass **every** guard (root-owned, mode 755 → no `/022` bits) and the script then writes
  `deployed-version`/`deployed-sha`/`failed-version` straight into `/` or `/etc`. Nothing
  is destroyed (`rm -f` only ever names those three files) but `/etc` is a one-component
  typo off the recommended `/etc/styrman/update`, and a hand run there records deploy state
  the systemd unit will never see — the host then looks "never deployed" to the timer. A
  cheap belt: after canonicalising, require at least two path components and refuse a bare
  `/`, `/etc`, `/var`, `/tmp`, `/opt`, `/home`. Note the parent's mode/owner is still
  unasserted, but that gap is now benign: a substituted dir cannot be root-owned, so the
  `stat -c %u` check fails closed. The one theoretical residual is a symlink planted at the
  state path between `realpath` and the `[ -d ]` (root-only, needs an attacker-writable
  parent — the recommended `$(mktemp -d)/state` has a 700 root parent and cannot hit it).
- **[low / confidence: medium]** `deploy/update/styrman-update:79` — `realpath -m` is GNU
  coreutils. If it is absent the substitution returns 127 and `|| fail "cannot canonicalize"`
  fires, so it **fails closed** (good) but with a message that will read as "your path is
  malformed" rather than "realpath is missing". Fine on Debian 13; worth one word in the
  message if this ever runs on a busybox host (`-m` is also not busybox-compatible).
- **[low / confidence: high]** `deploy/caddy/ingress/generator.go:205-215` — the security
  report asked for this comment to be restated and it was not. It says "This `origins` line
  is defense-in-depth ONLY", but the `if` block it guards emits **both** `origins` *and*
  `admin <addr>` — and the security review established that the emitted `admin <addr>` is
  the real control, because caddy replaces the live admin server from the *new* config
  before apps are provisioned, so without it a rejected `/load` leaves the admin bound
  wherever the failed config said (the 19:28 `0.0.0.0:26293` incident). A future reader can
  delete the whole block on the strength of that comment. **Fix:** two words — say the
  `admin` line is load-bearing and only `origins` is defense-in-depth.
- **[low / confidence: high]** `deploy/caddy/ingress/loopback.go:22` — `strconv.Atoi`
  accepts `"+2019"` and `"07"`, which then flow verbatim into `admin %s`. Harmless (Caddy
  rejects them at parse and the process exits before the generator runs) but
  `strconv.ParseUint(port, 10, 16)`, which the security report actually recommended, would
  reject both. Also still untested: `"127.0.0.1:"` (the case the `port == ""` clause exists
  for), and `TestGeneratedConfigNamesAdminOrigins`'s comment still promises "must never be
  emitted for a non-loopback addr" with no assertion behind it — round 2's low, unchanged.

## Criteria (delta only)

1. **Dead code** — one fail: `host-setup.sh:318`'s `setup-<timestamp>` fallback is
   unreachable (finding 1). Otherwise clean: `RUNTIME_IMAGE`/`getEnvOrDefault` are still
   reached, `helpers_loopback.go` is properly gone (renamed to `loopback.go`), and the
   `go mod tidy` removed a large indirect subtree rather than leaving it.
2. **Flow tracing** — the round-2 break is closed end to end: `git describe` → `$ver` →
   `docker build -t styrman-runtime:$ver` → `STYRMAN_VERSION` → compose → `STYRMAN_IMAGE`
   → `config.Load` → `orchestrator.create` now has a producer on the manual, fresh-install
   *and* updater paths, and the rollback path uses `${PREVIOUS#v}` so it lands in the same
   namespace. `STATE_DIR` is canonicalised **before** `STATE`/`STATE_SHA`/`FAILED` derive
   from it — I checked no other reader recomputes it. The one flow that still dead-ends is
   `discoverApps` → (no retry) (finding 3).
3. **Standards conformance** — `gofmt -l`, `go vet ./...` and `go test -count=1 ./...`
   clean in the main module; `go test -race -count=1 ./...` clean in the ingress module;
   `bash -n` and `shellcheck -S warning` clean on both shell scripts. The shell finding 1
   is an errexit/pipefail interaction shellcheck does not catch.
4. **Directory layout** — unchanged and correct; the new tests sit beside their subjects.
5. **Simplicity, DIY-except-security** — no new dependencies; the fixes are all
   subtractive or stdlib. No hand-rolled auth/crypto anywhere in the delta.
6. **Decoupling** — unchanged. `Watcher`'s concrete `*client.Client` is still the shallow
   seam that forces the new tests to poke `retryTimer`/`adminDown` directly (round-2 low,
   deliberately deferred); the tests are honest about it in their comments.

## Does anything still block?

**The commit: NO.** All four round-2 blocking findings and the security report's high are
closed, and I verified each by measurement or mutation rather than by reading the claim.
Nothing in the tree is destructive, silently wrong under the recommended recipe, or
untested-where-it-was-mutation-proven-untested. Findings 1 and 2 above are one-line fixes
that should ride in this same commit, but they do not justify holding it.

**The v1.16.1 release: YES — finding 1, and (still, from round 2) the verifier's image
build + live smoke.** `host-setup.sh` is the R11 fresh-install path; shipping a release
whose provisioning script can exit 128 with no message, at the exact line the release
notes will claim was fixed, is not acceptable in a tag. Fix findings 1 and 2, then the
remaining release gate is unchanged from round 2 and belongs to the verifier, not to me:
the watcher retry/fatal code in this tree has still never been compiled into an image, so
the new `styrman-caddy:1.16.1` must be built and smoke-tested live (fresh Google login, a
start/stop cycle producing a successful reload, `localhost:2019` reachable from the host
and refused from the LAN) before the red is called closed.

**Not blocking, file them:** findings 3-7 above, plus round 2's untaken lows (8-12) and
the explicitly deferred set (digest pinning, builder-toolchain assertion, local tag
mutability, `localhost`-by-name). `web/dist/client/_shell.html` is still dirty with
timestamp-only churn — confirm it is deliberate before `git add -A`.

## What I ran (round 3)

| Check | Result |
|---|---|
| `install -d -m 700` on an existing `0755` dir | → `700`; round-2 finding 1 was real |
| `install -d -m 700 a/b/c` mode propagation | intermediates `755`, leaf `700` — the explicit-parent line is still needed |
| Extracted `styrman-update` guard (12 inputs, `install`/`stat` stubbed) | `''`→refused; `/var/lib/styrman`, `//var/lib/styrman//sub`, `/var/lib/styrman/../styrman/x`, symlink→`/var/lib/styrman`→all refused as `/var/lib/styrman…`; `/tmp`→refused (world-writable); `/`,`/etc`→**accepted** (finding 4); `/tmp` mode `1777` unchanged after all runs |
| Mutation: delete `scheduleRetry` from `noteReloadFailure` | `TestRejectedReloadArmsRetryAndFatalDoesNot` **FAIL** |
| Mutation: drop the `return` on the fatal path | same test **FAIL** ("the fatal path must not arm a retry") |
| Mutation: revert the exact-`max` `dropped` fix | `TestTailBufferExactMaxSingleWriteIsNotTruncation` **FAIL** ×2 |
| Mutation: neuter `if c.Image == ""` | `TestLoadRequiresExplicitImage` **FAIL** |
| Mutation: delete the port range check in `loopback.go` | `TestIsLoopbackAddr` **FAIL** ×4 |
| `set -euo pipefail` + the new `SETUP_VERSION` line, tagless repo / non-git dir / tagged repo | `exit=128` / `exit=128` / `1.16.0` — finding 1 |
| `sudo git -C <operator clone> describe` | works (git's SUDO_UID exemption), so the tagged path is unaffected by dubious-ownership |
| `gofmt -l`, `go vet ./...`, `go test -count=1 ./...` (main) | clean / clean / all pass |
| `go test -race -count=1 ./...`, `gofmt -l`, `go vet` (ingress, against caddy v2.11.4) | all pass / clean / clean |
| `bash -n` + `shellcheck -S warning` on `styrman-update` and `host-setup.sh` | clean |
| `grep -rn ':latest'` across shipped paths | only `README.md:124` (finding 2) and prose in `versioning.md` |

All mutations were run against a scratch copy under `/tmp/claude-1000/…/scratchpad/mut`
and reverted from the real tree's files; nothing was built, tagged, or restarted, and no
production path (`/var/lib/styrman`, `/etc/styrman`) was read or written (ADR-052).
