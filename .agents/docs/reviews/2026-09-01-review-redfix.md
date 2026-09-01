# Review — redfix (the two 🔴 from the 2026-09-01 whole-project audit)   (2026-09-01)

Reviewer: `reviewer` subagent, fresh context. Author of the fixes: main agent — not this
context. (This context co-authored the *audit* being remediated, not the remediation.)

Scope: the uncommitted working tree.

```
 M deploy/caddy/ingress/cmd.go            M internal/orchestrator/exec.go
 M deploy/caddy/ingress/generator.go      M internal/orchestrator/orchestrator.go
 M deploy/caddy/ingress/go.mod|go.sum
?? deploy/caddy/ingress/helpers_loopback.go   ?? deploy/caddy/ingress/loopback_test.go
?? internal/orchestrator/tailbuffer_test.go
```

## Verdict: REQUEST CHANGES

Two bounded items, neither of which is a defect in the shipped logic — the logic is sound.

**Worst first: the `admin … { origins … }` block does not do what its comment and its test
say it does, and that claim is about to enter the permanent security record.** Read against
`caddy/v2@v2.11.3/admin.go` (verified locally, not from memory): the origin check runs only
`if h.enforceOrigin || hasOriginHeader || hasSecHeader` (`admin.go:866`), `getOrigin` reads
only `Origin`/`Referer` — **never `Host`** (`admin.go:959`), and for a *loopback* listen
address `allowedOrigins` already defaults to `{localhost:P, 127.0.0.1:P, ::1:P}`
(`admin.go:336-343`). So the comment's "Caddy's default accepts any Origin/Host" is false,
and "naming origins makes `/load` caller-bound" is false: any local process issuing a plain
`curl -X POST localhost:2019/load` with no `Origin` header still gets unauthenticated,
whole-config replacement. The directive narrows three allowed origins to one. That is
harmless and marginally tidier — it is not the control that closes 🔴-2. The controls that
*do* close it are the loopback guard (b) and the version bump (c).

Second: **🔴-2 is not remediated in anything that runs.** `deploy/caddy/Dockerfile` is
untouched, `styrman-caddy` is not rebuilt, and the running binary has neither the `admin`
block nor Caddy 2.11.3. `go.mod` raising the floor *will* reach the image (xcaddy's
synthesized main module resolves `max(builder $CADDY_VERSION, our require)` under MVS), and
that combination — Caddy 2.11.3 plus a **floating** `--with github.com/greenpau/caddy-security`
— has never been compiled. The red stays open until `docker compose build caddy` succeeds
and the container is restarted.

Everything else is nits. The `tailBuffer` arithmetic is *correct*: I ran a randomized
differential test (copy of the type, `max` 1..64, 300 trials each, writes of 0..2·max) against
the reference "last `max` bytes of the concatenation" — byte-for-byte equal in every trial,
`len ≤ max` always, no aliasing, no lost bytes, and `cap` measured at exactly `max` (ratio
1.00) across 20 000 mixed writes with periodic > 1 MiB spikes into a 256 KiB window. The
`copy` is a legal overlapping forward move (memmove semantics) and `p` can never alias
`t.buf`. The panic call-out is right and the new form is right: `Dropped()` cannot produce a
negative index, and a **non-truncated response is byte-identical to the old code** — with one
exact-boundary exception, below.

## Findings

- **[medium / confidence: high]** `deploy/caddy/ingress/generator.go:205-214`, `loopback_test.go:33-42`
  — the comment and the test name assert a guarantee Caddy does not provide (evidence above).
  `origins` is a browser cross-site mitigation only, it is not consulted at all when no
  `Origin`/`Referer`/`Sec-Fetch-*` header is present, and the default for a loopback bind was
  already restrictive. Fix: rewrite the comment to say what is true ("narrows the default
  origin set to the one address we reload from; the real controls are the loopback bind and
  the 2.11.3 floor"), and rename `TestGeneratedConfigRestrictsAdminOrigins` to something it
  actually proves (e.g. `TestGeneratedConfigPinsAdminListenAndOrigins`). If real enforcement
  is wanted, it takes **two** changes together: emit `enforce_origin` *and* set
  `req.Header.Set("Origin", "http://"+w.adminAddr)` in `watcher.go:237` `reloadCaddy` —
  without the second, every reload 403s. Do not add `enforce_origin` alone.
- **[medium / confidence: high]** `deploy/caddy/Dockerfile` (unchanged) — the fix is not in the
  artifact. `caddy:2-builder` floats and `--with github.com/greenpau/caddy-security` floats;
  neither has been built against 2.11.3. The audit's own remediation line also asked to "pin
  what `xcaddy` actually produces" and flagged `Dockerfile:24 EXPOSE 80 443 2019` — both still
  open. `EXPOSE 2019` is inert under `network_mode: host` but now actively contradicts the new
  "this port must never be reachable" invariant; drop it. Smoke test required before the red is
  called closed — see *Not run* below.
- **[low / confidence: high]** `internal/orchestrator/exec.go:37-40` — `dropped` is set in the
  `n >= t.max` branch even when nothing was discarded: a single write of *exactly* `max` into
  an empty buffer. Confirmed by differential test (the only divergence from the reference in
  the whole run). Consequence at `orchestrator.go:1243`: a spurious "… (older lines truncated) …"
  banner **and** the deletion of a genuine first line, on output that was never truncated. Fix:
  `case n > t.max:` for the flag, or hoist `t.dropped = t.dropped || len(t.buf)+n > t.max`
  above the switch. StdCopy frames are ≲16 KiB so a 262 144-byte single write is near-impossible
  in production — but `Dropped()` drives a user-visible decision and should mean what it says.
- **[low / confidence: high]** `internal/orchestrator/orchestrator.go:1245-1249` — the trim now
  *always* strips through the first `\n`, so when the discard boundary lands exactly on a line
  start the first retained line is complete and gets silently deleted. The old code handled this
  (`cutMidLine`); the information needed to reproduce it was thrown away with the byte. Fix:
  record the last discarded byte in `tailBuffer` (`lastDropped byte`) and strip only when it was
  not `\n`. Frequency ≈ 1/line-length, impact one line, banner already present — hence low.
- **[low / confidence: high]** no test covers `Orchestrator.Logs`'s post-processing — which is
  precisely where the `out[len(out)-maxBytes-1]` panic was introduced and fixed. `tailBuffer`
  itself is well covered; the banner/first-line-strip is covered by nothing. Extract
  `func trimTruncatedLogs(out string, dropped bool) string` and test it, including the
  full-buffer and no-newline-at-all cases. Standing rule: changed code without covering tests
  is a verifier FAIL, and this is the one line of the diff with a known historical bug.
- **[low / confidence: high]** `deploy/caddy/ingress/loopback_test.go:45-52` — hand-rolled
  `contains()` reimplements `strings.Contains`, and the package *already* has `assertContains`
  (`generator_test.go:281`). Delete it and use the existing helper; the generic package-scope
  name will collide sooner or later. (Simplicity / stdlib-first.)
- **[low / confidence: high]** `deploy/caddy/ingress/cmd.go:45` vs `:130` — the value that is
  **validated** (`cfg.AdminListen`) and the value that is actually **bound** and used as the
  reload URL (`adminAddr`, a second independent `getEnvOrDefault`) are computed separately. They
  agree today only by coincidence; the guard is coupled to the listener by luck, not dataflow,
  and the emitted `origins` must match the reload `Host` exactly. Fix: `adminAddr := cfg.AdminListen`.
- **[low / confidence: medium]** `deploy/caddy/ingress/helpers_loopback.go:14-20` — it does not
  mirror `internal/config/config.go:472` as the comment claims. That one *requires* `host:port`
  (`SplitHostPort` error → `false`); this one falls through and accepts a bare host, and
  `loopback_test.go:11` pins that leniency ("127.0.0.1", "::1"). So `CADDY_ADMIN=127.0.0.1`
  passes the guard and then `reloadCaddy` POSTs to `http://127.0.0.1/load` — **port 80, the
  public HTTP server**. Reject when a port is absent. Everything else about the function is
  right: **accepting all of 127.0.0.0/8 is correct** (Linux drops martian 127/8 from the wire,
  `route_localnet` is 0, Docker bridges cannot reach host loopback, and R-constraints exclude
  kube-proxy); `net.ParseIP` fail-closes on octal/decimal IP obfuscation; `::ffff:127.0.0.1`
  resolves loopback correctly. `strings.Trim(host, "[]")` is sloppy (accepts `"::1]"`) but only
  ever admits valid loopback literals, so it is not a hole.
- **[low / confidence: high]** file naming/layout: `helpers_loopback.go` is the only `helpers_*`
  file in a flat package (`cmd.go`, `config.go`, `generator.go`, `labels.go`, `module.go`,
  `watcher.go`), and its test is `loopback_test.go` — the names do not pair. Either move
  `isLoopbackAddr` into `config.go` (where `getEnvOrDefault` and the env keys live, and where an
  unfamiliar engineer would look) or rename the file to `loopback.go`. "helpers_" is the start of
  a grab-bag.
- **[low / confidence: high]** the verification claim needs one correction for the record. I ran
  `govulncheck ./...` on the ingress module: **29** called vulnerabilities — 2 in
  `docker/docker@v27.5.1` (the unfixable ones, correct) **and 27 in the Go standard library**,
  because the local toolchain is go1.25.0 and the fixes land in 1.25.2/1.25.3. Not introduced
  here, and not shipped (`deploy/Dockerfile.server` floats `golang:1.25-trixie`, the caddy image
  floats `caddy:2-builder`), but the changelog should say "only 2 remaining *module* findings",
  not "only 2 findings".
- **[low / confidence: high]** docs / DoD outstanding. `deploy/.env.example:70` still describes
  `CADDY_ADMIN` as a bare "caddy admin listener override" with no hint that a non-loopback value
  now **hard-fails startup** of a `restart: unless-stopped`, `network_mode: host` container —
  i.e. whole-site-down crash loop. Document the constraint next to the knob. The working tree
  also carries no `changes/` entry, no `testing/` verification log, and no `security/` note for a
  dependency + security-surface change.

## What I verified myself (not taken on report)

| Check | Result |
|---|---|
| `go build ./...`, `go vet ./internal/orchestrator/...` (main module) | clean |
| `go test -race -count=1 ./...` (main module, full) | all pass |
| `go vet ./...`, `go test -count=1 ./...` (ingress module) | all pass |
| `gofmt -l internal/orchestrator deploy/caddy/ingress` | clean |
| `go mod verify` (ingress) | all modules verified |
| `go mod tidy` in a scratch **copy** of the ingress module | `go.mod` **and** `go.sum` byte-identical → tidy-clean |
| `tailBuffer` vs reference "last max bytes", randomized, max 1..64 × 300 trials | byte-identical every trial; `len ≤ max` always; only `Dropped()` diverges (finding 3) |
| `cap(tailBuffer.buf)` over 20 000 mixed writes incl. >1 MiB spikes, max = 256 KiB | worst cap = 262 144 = **1.00 × max** — no reslice pinning, no unbounded growth |
| Caddyfile adapter, real `caddy/v2@v2.11.3`: `admin <a> { origins <a> }` | parses; adapts to `{"admin":{"listen":"localhost:2019","origins":["localhost:2019"]}}` — syntactically valid and structurally what is claimed |
| Same adapter: `metrics { per_host }`, `order … before basicauth` (the 2.9-era spellings this generator emits) | both still parse under 2.11.3 — **no adapter regression from the bump** |
| Caddy 2.11.3 `admin.go` origin semantics | see finding 1 — `Host` is not checked, and the check is skipped entirely without `Origin`/`Referer`/`Sec-Fetch-*` |
| `replaceLocalAdminServer` behaviour when a loaded config carries an `admin` block | admin server is replaced on *every* load already; old server is shut down **asynchronously** so the in-flight `/load` response is still delivered → the new block introduces no reload-breakage |
| Only `ContainerLogs` call site in the repo | `orchestrator.go:1220` — the finding is fully covered, no second unbounded site |
| Project containers set `Tty`? | no (`container.Config` at `orchestrator.go:753` leaves it false) → the log stream is daemon-multiplexed, so `StdCopy`'s own frame-size allocation is daemon-controlled, not guest-controlled. The bound holds. **If a project container ever gets `Tty: true`, `StdCopy` would parse guest bytes as frame headers and can allocate up to 4 GiB per frame — `tailBuffer` would not save it.** Worth a comment at the `Tty` decision site. |
| `CADDY_ADMIN` in this deployment | unset in `deploy/.env`; `docker-compose.yml:44` passes `CADDY_ADMIN=${CADDY_ADMIN:-}` = empty → `getEnvOrDefault` returns `localhost:2019`. **The new guard cannot trip on the live host.** |

## Answers to the questions asked

- **Is refusing to start the right failure mode?** Yes. It matches the `config.go:180`
  precedent, and it is unreachable in this deployment (row above). The residual risk is purely
  operational — `restart: unless-stopped` turns a typo into a site-down crash loop — and the
  mitigation is documentation, not a softer failure. Do not downgrade it to a warning.
- **Could the bump break the `xcaddy` build?** Possibly, and it is untested. MVS means the image
  gets ≥2.11.3 regardless of what `caddy:2-builder` pins, so the floating `caddy-security` must
  compile against 2.11.3. The Caddyfile *adapter* side I did test and it is fine (`metrics
  { per_host }` and `order … basicauth` both still parse). The plugin side can only be settled by
  running the build.
- **`integration.sh` / e2e?** Agree neither is needed — correctly reasoned. But the logs path *is*
  user-reachable (`projectLogs` → `LogsDialog`, 30 s poll), so one manual pass over the ops-panel
  logs view on the rebuilt server is worth more than either suite here.
- **Does the ingress change need a live smoke test before shipping?** Yes, and it is the gate on
  🔴-2. Before rebuilding, tag the rollback: `docker tag styrman-caddy:latest
  styrman-caddy:pre-2.11.3` — the old image is the only copy of the currently-working binary and
  a rebuild overwrites `:latest`. Then: (1) `docker compose build caddy` succeeds; (2) in the new
  image `caddy version` ≥ 2.11.3 and `caddy list-modules` still lists security/cloudflare/
  rate_limit/ingress (the Dockerfile's own `RUN … grep` covers this); (3) restart, site serves and
  **Google OAuth still works** — caddy-security across a major-version jump is the real risk, and
  an existing session surviving is *not* proof, test a fresh login; (4) start/stop a project and
  confirm the watcher logs `caddy reloaded successfully` (the first reload carrying the new
  `admin` block); (5) `curl -sS localhost:2019/config/ | head -c 200` from the host works and the
  same from another LAN host is refused.

## Criteria

1. **Dead code** — pass. Nothing orphaned; `cappedBuffer` still has its `Exec` callers,
   `tailBuffer`/`Dropped`/`AdminListen`/`isLoopbackAddr` are all reached. The only removal
   *owed* is pre-existing and named by the audit: `deploy/caddy/Dockerfile:24 EXPOSE 2019`.
   One duplicate helper to delete: `loopback_test.go` `contains()`.
2. **Flow tracing** — pass with one nit. `Logs` → `projectLogs` (owner-scoped via
   `getProjectFor`, `r.Context()` propagated, `rc.Close()` deferred on every path); the sole
   `ContainerLogs` site is the fixed one; `AdminListen` flows loadConfig → GeneratorConfig →
   `writeGlobalOptions` → `/load`, and the emitted `origins` matches the reload URL's Host
   because both derive from `CADDY_ADMIN` — but via two separate reads (finding 7). Residual,
   pre-existing: `_, _ = stdcopy.StdCopy(...)` still discards its error, so a mid-stream read
   failure silently returns partial logs with no marker; and memory is now bounded while
   *wall-clock* is not (2000 guest-controlled lines still stream in full), which pins one
   goroutine and one Docker conn for the requesting owner only — acceptable, worth knowing.
3. **Standards conformance** — pass except the hand-rolled `contains` (stdlib-first) and the
   two comments that state things that are not true (`generator.go:205-211`,
   `helpers_loopback.go:8-10`). gofmt/vet/tests verified green by me, not taken on report.
4. **Directory layout** — pass with a nit. Tests sit beside their code; the Go module boundary
   between `internal/` and the vendored `deploy/caddy/ingress` is respected (the duplicated
   `isLoopbackAddr` is *correct* here — they are separate modules and R11 forbids the ingress
   depending on the main module). Nit: `helpers_loopback.go` naming (finding 9).
5. **Simplicity, DIY-except-security** — pass. `tailBuffer` is ~20 lines of stdlib slice work
   and is the simplest thing that satisfies both constraints (bounded bytes + newest-first);
   rejecting `cappedBuffer` reuse was the right call and the reasoning is recorded in the code.
   No new dependencies — the `go.mod` delta is a *reduction* (~130 fewer indirect requires) and
   is tidy-clean. No hand-rolled auth/crypto: OAuth/JWT stay with caddy-security, and the change
   moves *toward* the vetted library by bumping it. The one DIY slip is `contains()` in the test.
6. **Decoupling** — pass. `tailBuffer` is a self-contained `io.Writer` with a two-method
   surface, no coupling to Docker or HTTP, independently testable — a deep module. It is
   *not* goroutine-safe and does not claim to be; safe here only because `StdCopy` writes
   sequentially — worth one line saying so, since it takes both stdout and stderr and reads
   like a concurrent sink. `AdminListen` threads through the existing `GeneratorConfig` seam
   rather than adding a new one. Two engineers could work `exec.go` and the ingress module in
   parallel without collision.

## Is anything SHIP-blocking?

**Nothing here blocks the commit.** No correctness defect, no regression, every gate green,
and the working tree cannot break the live host (the caddy container is not rebuilt and the
`CADDY_ADMIN` guard cannot trip on this deployment).

**Two things block the claim that the 🔴s are closed:**

1. 🔴-2 is not closed until `docker compose build caddy` succeeds and the restarted container
   passes the 5-step smoke test above. Right now the running ingress has neither the version
   bump nor the `admin` block. `govulncheck` on the *module* is not evidence about the
   *binary*.
2. The `generator.go` comment and the test name must be corrected before this lands, because
   they are the record — a future audit reading them will believe the admin API is
   origin-authenticated when it is not. Everything else on the list is a nit that can ride
   along or follow.

🔴-1 (the log buffer) **is** closed by this change, in code that is deployed by a normal server
rebuild, and I could not break it.

## Out-of-scope observation (noticed mid-review)

`web/dist/client/_shell.html` became dirty **during** this review (mtime 16:34:53, alongside a
full rewrite of `web/dist/client/assets/` and `favicon.svg`) — a concurrent web build, not
anything the reviewer ran; no Go code in this repo builds web assets (`web/embed.go` only
embeds `all:dist/client`). The entire byte delta is one embedded epoch-ms timestamp
(`u:1788271052459` → `u:1788273293045`). Because `web/dist` is **tracked**, every web build
dirties this file with a pure-timestamp diff and it will keep riding along in unrelated
commits. Not part of this change set, left untouched, and worth a separate decision: either
gitignore the built shell or strip the timestamp. Whoever commits the redfix should confirm
this file is deliberate before staging `-A`.
