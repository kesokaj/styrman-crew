---
owner: explorer
date: 2026-09-10
---
# Recon — TODO.md closeout, items A–O   (2026-09-10)

## Question
For each open TODO.md item A–O: exact file(s)/line ranges, current code shape,
minimal fix shape, covering test (or its absence), and risk — enough to plan
one pass that closes them.

## Summary
All fifteen items located and confirmed against current source; TODO.md's own
descriptions turned out accurate everywhere checked (worth noting: TODO items
K and L reference `check.go`/`format.go`, which live in `internal/httpapi/`,
not `image/` as a literal reading might suggest — they are the shell-script
*generators*, not guest-side Go). Several items (D's roster test, F's
migration, E's caller) already have solid coverage and are genuinely small;
C, I, and O are real refactors (O is the big one — a `dockerAPI` seam over
~19 methods). B needs a product decision (strip vs gitignore) more than code.
The nine-item K bundle is nine independent one-liners, safe to batch.

## A — Converge 8 shared dependencies (ingress vs root go.mod)

Confirmed via `deploy/verify-commit.sh`'s own embedded python (lines ~65–82),
re-run directly:

```
github.com/docker/go-connections:                            root v0.8.1  vs ingress v0.5.0
github.com/felixge/httpsnoop:                                  root v1.1.0  vs ingress v1.0.4
github.com/go-logr/logr:                                       root v1.4.4  vs ingress v1.4.3
go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp:  root v0.71.0 vs ingress v0.68.0
go.opentelemetry.io/otel:                                       root v1.46.0 vs ingress v1.43.0
go.opentelemetry.io/otel/metric:                                root v1.46.0 vs ingress v1.43.0
go.opentelemetry.io/otel/trace:                                 root v1.46.0 vs ingress v1.43.0
golang.org/x/sys:                                               root v0.47.0 vs ingress v0.46.0
```
Exactly 8, exactly the ones TODO names (go-connections, httpsnoop, logr,
three otel packages) plus x/sys and... that's 8 total (3 otel + 5 others).

- Ingress build: `deploy/caddy/Dockerfile` — `xcaddy build` in stage 1
  (`caddy:2.11.4-builder@sha256:…`), digest-pinned base in stage 2
  (`caddy:2.11.4@sha256:…`). `xcaddy` resolves deps itself; the module's
  `go.mod`/`go.sum` govern what `go test` sees but not necessarily what
  `xcaddy build` finally pins (xcaddy vendors its own resolution) — **open
  question**: bumping `deploy/caddy/ingress/go.mod` may not be sufficient by
  itself to change what ships; verify with a real `docker build` of
  `deploy/caddy/Dockerfile` after bumping.
- Test command: `cd deploy/caddy/ingress && go vet ./... && go test ./...`
  (same line `deploy/verify-commit.sh:44` runs in the gate).
- Fix: bump each of the 8 in `deploy/caddy/ingress/go.mod` (`go get
  <pkg>@<root-version>` inside that module), `go mod tidy`, rerun
  `go test ./...` there, then rebuild `deploy/caddy/Dockerfile` to confirm
  the compiled binary picks it up (the TODO explicitly says this is an
  "untested eight-package bump" — budget for surprises, especially
  OpenTelemetry's frequent breaking internal API moves 0.68→0.71).
- Risk: not a size problem (8 `go get` calls) but a behavior-change problem —
  none of these are exercised by `deploy/caddy/ingress`'s own unit tests
  except indirectly; the real proof is the smoke test the Dockerfile comment
  references (OAuth login) — no automated equivalent exists today.

## B — `web/dist/client/_shell.html` timestamp churn

Confirmed present: `python3` byte scan on the committed file finds one
13-digit millisecond timestamp, `1789030752690`, embedded in TanStack's
prerendered router payload (the file is 7606 bytes; `.gitattributes` forces
it to diff as text — see the comment there explaining the file's NUL byte
is unrelated, BUG-007 follow-up).

- Build entry point: `web/package.json` `"build": "vite build && tsc -b"`.
  No `web/app.config.ts` exists in this repo (checked, not found) — the
  TanStack Start config must be `web/vite.config.ts` or similar; **open
  question**: did not locate the exact prerender plugin invocation that
  stamps `u:<timestamp>` — need to grep vite config / `@tanstack/start`
  plugin options before writing a vite-plugin-based fix.
- `deploy/verify-commit.sh`'s freshness check (`==> web/dist freshness`)
  compares **filenames only** (`find client -type f | sort`), not file
  content, so a churning timestamp inside `_shell.html` does NOT fail that
  gate today — normalizing the value would not break it either way. Good:
  whichever fix is chosen (post-build `sed` vs vite plugin) is safe against
  this gate as long as it doesn't rename the file.
- Minimal fix: a `sed` line appended to the `build` npm script (e.g.
  `sed -i -E 's/u:[0-9]{10,}/u:0/' dist/client/_shell.html` after `vite
  build`) is the least invasive — no build-tool internals to understand,
  one line, easy to test with `grep -c 'u:0'`. A vite plugin is more
  "correct" but requires understanding TanStack Start's prerender internals
  first (open question above).
- Risk: low — this is a build-output cosmetic fix; the actual runtime
  behavior of the timestamp (if any downstream JS reads it) is unverified —
  grep `u:` usage in `web/src` before blindly zeroing it, in case it's a
  cache-buster the router relies on rather than dead metadata.

## C — `internal/httpapi/network.go` `setProjectNetwork` writes inside lock

`internal/httpapi/network.go:91` is `setProjectNetwork`; the closure passed
to `s.mode.WithProject(...)` starts at line 129 and runs until ~line 224
(the full LAN-toggle state machine: re-read record under lock, `Status`,
`ModeMismatch`, `persist()`, `Stop`, `EnsureRunning`), calling `writeErr`/
`writeJSON` directly at **7 different return points** inside the closure,
all while the project lock (via `WithProject`) is held. `modeRuntime`
(`internal/httpapi/server.go:86`) is the existing narrow interface used to
make this testable without real Docker: `WithProject(user, project string,
fn func(orchestrator.Project))`.

- Minimal fix shape: change the closure's signature to return
  `(status int, body any)` (or a small `result` struct) instead of writing
  directly; each of the 7 `writeErr(w, ...)`/`writeJSON(w, ...)` calls
  becomes `return status, errBody{...}` / `return status, viewBody`; after
  `WithProject` returns, one `writeJSON(w, result.status, result.body)` (or
  `writeErr` if it's an error shape) at the end of `setProjectNetwork`.
  Complication: the two success paths use `writeJSON` with `lanView` and the
  error paths use `writeErr` with `error` — the result type needs to carry
  either, or normalize to `(status int, body any)` and have one writer at
  the end regardless of shape (writeErr already just wraps into a JSON
  error envelope — check its signature before assuming body types match).
  There's also a `go func(){...}()` background goroutine spawned inside the
  closure (self-attach) *after* the final success write area — verify it
  doesn't need to move outside too (it's fire-and-forget, unaffected by the
  refactor either way).
- No test file was found specifically covering `setProjectNetwork`'s response
  writing (only reviews reference it) — check
  `internal/httpapi/network_test.go` if it exists before writing new tests;
  not directly confirmed to exist in this pass (open question — grep it at
  fix time).
- Risk: this is the exact mechanism three security review rounds already hit
  (BUG-028/029/031, see TODO's DONE items above it) — AGENTS.md's rule "when
  a SECOND round of findings lands on the same mechanism... stop fixing and
  ask" already fired once and the operator chose to restructure rather than
  keep patching; this item is now genuinely mechanical (io write ordering),
  not another defense to bolt on — low risk if kept narrowly scoped to
  moving the writes.

## D — `toolDetails` + `publicKeyLine` test gap

- `toolDetails` — `internal/httpapi/setup.go:208-292`. Builds `tools []toolInfo`
  for git/ssh/github/claude/agy. The ssh block (`setup.go:236-253`) reads
  `.ssh/id_ed25519.pub` via `files.ReadRawNoFollow(h, ".ssh/id_ed25519.pub",
  8192)`, takes the first line, and gates on `publicKeyLine(line)`
  (`setup.go:298-314`) before setting `ssh.Details = []string{pub}`.
- `publicKeyLine` — `setup.go:298`, already has its own unit test
  (`internal/httpapi/setup_pubkey_test.go`, `TestPublicKeyLine`, lines 9-33)
  covering accept/reject shapes directly — but that test calls the function,
  not the HTTP handler, so it cannot see whether `toolDetails` actually wires
  a symlinked/hard-linked/copied `.pub` correctly (the TODO's exact point).
- Existing handler test: `internal/httpapi/setup_roster_test.go` has
  `toolsByName(t, s)` (lines 28-38, calls `s.toolDetails` via httptest) and
  `TestRosterReadsTheNewestSeededProjectHome` — but that test seeds only
  `.claude/*`/`.gemini/*` roster files under `ProjectHome`, never touches
  `.ssh/id_ed25519.pub` under `UserHome`. No test plants a symlink/hardlink/
  copy `.pub` and asserts `Details` is empty — TODO's claim confirmed
  exactly.
- Minimal fix: new test in `setup_pubkey_test.go` (safe: file exists, so
  append per BUG-005 rule) or a new file, using `testServer(t)` +
  `toolsByName(t, s)` from `setup_roster_test.go`, seeding `UserHome`
  (`s.cfg.UserHome("tester")`) with three `.ssh/id_ed25519.pub` variants:
  `os.Symlink` to the private key, `os.Link` (hardlink) to the private key,
  and a byte-copy of the private key content — assert `Details` is `nil`/
  empty for all three, and one positive case with a real-shaped pub line.
- `stripControls` — `internal/httpapi/server.go:1292-1298` (right after
  `clampTitle`, `server.go:1290`) — `strings.Map` dropping `unicode.IsControl`,
  `unicode.Cf`, and explicit U+2028/U+2029. Signature:
  `func stripControls(t string) string`.
- Risk: low, purely additive test.

## E — `unmarkIfStillRunning`

`internal/orchestrator/orchestrator.go:1144-1156`:
```go
func (o *Orchestrator) unmarkIfStillRunning(_ context.Context, user, project string) {
    ictx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    insp, err := o.docker.ContainerInspect(ictx, config.ContainerName(project))
    if err == nil && insp.State != nil && insp.State.Running {
        o.stopped.Delete(key(user, project))
    }
}
```
Two callers: `orchestrator.go:1130` (inside `stopIfIdle`/reaper-adjacent path,
per the surrounding comment "provably did not happen") and `orchestrator.go:1239`
(second call site, same file — not yet read in full; grep confirms exactly
two call sites total).

- Fix (a) from TODO: require two `Running` samples ~2s apart — restructure
  to inspect once, sleep ~2s, inspect again, only delete the marker if BOTH
  report Running. Note the function already ignores its `ctx` parameter
  (`_ context.Context`) and builds its own 5s budget — a two-sample version
  needs to either widen that budget (~7-8s) or take two independent 5s
  budgets.
- Fix (b): add one `o.logger.Warn(...)` on the inspect-error branch and the
  not-running branch (currently both fall through silently — the `if err ==
  nil && ... Running` is the only branch taken; every other outcome is a
  no-op with zero logging).
- No test found covering `unmarkIfStillRunning` directly in this pass (not
  grepped exhaustively for a `_test.go` — check
  `internal/orchestrator/*_test.go` for `unmarkIfStillRunning` or
  `TestStopped`/`o.stopped` before assuming none exists).
- Risk: low/narrow per TODO's own words; four review rounds already landed
  on this path (BUG-030 family) — same AGENTS.md "second round, ask whether
  to delete" caution applies if a THIRD post-fix review finds something new
  here; both (a) and (b) are pure additions (timing + logging), not new
  branches, so risk of introducing a new defect is low.

## F — `migrateWorkspaces` / `forEachProject` / `LegacyWorkspaceExists`

- `migrateWorkspaces` — `cmd/styrman/main.go:249-292`. Resolves the legacy
  source via `files.ContainedPath(cfg.UserHome(user), "projects/"+p.Dir)`
  (line 252) — **already confined**, not a raw path join; the TODO's
  characterization ("resolves with EvalSymlinks and renames by path") looks
  stale/inaccurate against current code — `files.ContainedPath` is the
  confinement primitive (`internal/files/files.go:180`), not `EvalSymlinks`
  directly. Worth re-reading `files.ContainedPath`'s implementation before
  assuming the residual risk is unchanged; it may already be closed and the
  TODO simply hasn't been checked off.
- `forEachProject` — `main.go:299-320`: iterates `cfg.StateDir/users/*`,
  loads each user's `project.Store`, calls `fn(user, p)` for every project.
  Used by both `migrateWorkspaces` and `ensureRepos` (`main.go:322+`).
- `config.LegacyWorkspaceExists` — `internal/config/config.go:379-388`: stats
  `UserHome/projects/<dir>`, treats an existing-but-empty dir (Docker's bind
  mount-point stub) as "doesn't exist", unreadable as "exists" (fail safe).
- Every reference to the legacy path: `main.go:252` (migrateWorkspaces
  source), `main.go:339` (`ensureRepos`'s guard: never create the new
  workspace while `LegacyWorkspaceExists` — would permanently shadow real
  data), `internal/orchestrator/orchestrator.go:372` ("workspace migration
  incomplete for %s — restart the server to retry", inside presumably
  `EnsureRunning`'s dir-resolution path).
- Test: `cmd/styrman/migrate_test.go` exists and specifically covers this
  (the file `.agents/rules/writing-files.md`'s BUG-005 story is literally
  about a `migrate_test.go` file, i.e. this exact file — extra care needed:
  append, never `cat >`/`Write` over it).
- What deleting the whole legacy path removes: `migrateWorkspaces`,
  `LegacyWorkspaceExists`, the `main.go:339` guard branch in `ensureRepos`,
  the `orchestrator.go:372` error branch, and the confinement code path in
  `migrateWorkspaces` — a real simplification, but TODO says "this host has
  none" (all migrated at v1.19.x) which is a claim about ONE deployment, not
  a guarantee about every deployment of this self-hosted software; deleting
  outright is a decision for the operator (removes a safety net for anyone
  upgrading from pre-ADR-027, if that's still a supported upgrade path).
- Risk: medium — TODO itself flags this as still-open at "medium/medium" in
  the original 2026-09-04 review; the safer of the two options (confine via
  `os.Root` vs delete) is confinement, since `ContainedPath` already exists
  and is already used at the read site — the actual gap may be narrower than
  TODO describes (worth re ­verifying against `files.ContainedPath`'s
  guarantees before scoping work).

## G — `internal/httpapi/git.go:73` unconfined `.gitconfig` read

`gitIdentity` — `internal/httpapi/git.go:77-88` (not literally line 73 in
current numbering — verify with `grep -n func gitIdentity` at fix time,
off-by-a-few from TODO's line number, likely just file drift):
```go
func (s *Server) gitIdentity(user string) (name, email string) {
    cfg := filepath.Join(s.cfg.UserHome(user), ".gitconfig")
    get := func(key string) string {
        out, err := exec.Command("git", "config", "--file", cfg, key).Output()
        ...
    }
    return get("user.name"), get("user.email")
}
```
This is a **path join + `exec.Command`**, not a `files.*` call — the one
base-home read outside `internal/files`'s confined roots, exactly as TODO
says.

- `files.ContainedPath(root, rel string) (string, error)` —
  `internal/files/files.go:180` — resolves `rel` under `root`, refusing an
  escaping symlink component; returns the safe absolute path.
- `files.ReadRaw(root, rel string, max int64) ([]byte, error)` —
  `internal/files/files.go:420` — already used elsewhere in the same file
  (`git.go`'s sibling reads) and in `setup.go`'s `toolDetails`.
- Minimal fix: read `.gitconfig` content via `files.ReadRaw(s.cfg.UserHome(user),
  ".gitconfig", <reasonable max>)` instead of `filepath.Join` + `exec.Command`,
  then parse `user.name`/`user.email` out of the INI text directly (small
  parser, similar to what `toolDetails`'s `git` block already does at
  `setup.go:222-231` — pattern-match `strings.HasPrefix(line, "name = ")`/
  `"email = "` — could literally reuse that logic) rather than shelling out
  to `git config --file`. This also drops an `exec.Command` dependency,
  which is a plus.
- No test found for `gitIdentity` specifically in this pass — grep
  `internal/httpapi/git_test.go` for `gitIdentity` at fix time.
- Risk: low — TODO itself calls it low severity (the file is the user's own,
  so a planted symlink leaks nothing new); real risk is behavior drift if
  the hand-rolled INI parse diverges from `git config`'s actual parsing
  rules (quoting, multiple `[user]` sections, includes) — keep the fallback
  simple and note the trade explicitly in a comment.

## H — `web/src/lib/sshKey.ts`

Read in full (72 lines). Current regexes:
```ts
const PUBLIC_KEY =
  /^(ssh-(ed25519|rsa|dss)|ecdsa-sha2-[^\s]+|sk-[^\s]+)[ \t]+[A-Za-z0-9+/=]+([ \t]+[^\r\n]*[^\s])?$/;
```
and the control-char reject in `sshPublicKey`:
```ts
if (/[\p{Cc}  ]/u.test(line.replaceAll("\t", " "))) continue;
```
TODO items 31/33/35 (server-side `publicKeyLine` type tightening,
`sshKey.ts` `\p{C}` widening, and comment control-char stripping) all target
this same file/its server twin — **this frontend regex ALREADY requires the
full type token** (`ssh-(ed25519|rsa|dss)`, `ecdsa-sha2-...`, `sk-...`) —
i.e. TODO item 31 (tightening `publicKeyLine`'s type check server-side)
is NOT yet mirrored on the Go side (`setup.go:298`'s `publicKeyLine` only
checks `strings.HasPrefix(typ, "ssh-")` etc., no exact enum) — the frontend
is ahead of the backend here, worth doing both together for consistency.
`abbreviateKey` (lines 60-72) does not special-case bidi/RLO collapse — TODO
item 33 confirmed: only `\p{Cc}` + two line separators rejected, not the
full `\p{C}` category (Cf formatting chars like RLO U+202E, ZWSP, BOM,
U+00AD soft hyphen, NBSP all pass through untouched into the title attribute
and clipboard).
- Fix shape (small): widen the reject regex to `\p{C}` (keep excluding TAB,
  which is explicitly allowed as a field separator), and in `abbreviateKey`
  collapse Unicode space category before rendering (`\p{Zs}` → single space,
  or reject them the same way).
- No dedicated test file confirmed read in this pass; comment at top of
  `sshKey.ts` says "Kept free of React so vitest covers the parsing
  (sshKey.test.ts)" — that file should exist; verify and extend it with RLO/
  ZWSP/BOM/NBSP cases per TODO item 33 before/with the fix.
- Risk: low, cosmetic (own-row-only spoofing per TODO's own words).

## I — `internal/terminal/bridge.go` WS re-auth

- `NewBridge` — `bridge.go:55`. `ServeHTTP` — `bridge.go:82-157`: reads
  `identity.FromContext(r.Context()).Username` at line 83 — i.e. identity is
  established once, at HTTP-upgrade time, via the same `authGuard` middleware
  every REST request goes through (`internal/httpapi/server.go`'s
  `authGuard`, which re-checks the userstore via `s.users.IsAuthorized(...)`
  on every REST request — confirmed by reading `authGuard`'s header-mode
  branch). Because a WS upgrade is itself one HTTP request, `authGuard` DOES
  run for it — but after the 101 Switching Protocols, the connection is a
  raw stream with no further HTTP-layer authorization checks. `serve` —
  `bridge.go:219-421` — is the long-lived per-session loop; nothing in it
  re-checks the userstore.
- Where a periodic re-check would go: inside `serve`'s main loop (or a
  goroutine spawned from it) — needs a ticker (e.g. every N minutes) that
  calls `userstore.Store.IsAuthorized(email)` (note: `IsAuthorized` takes an
  email, not a username — `identity.User` carries both; `bridge.go` currently
  only keeps `user` = username from context, so the closure would need the
  email too, or a store method keyed by username — check `userstore.go`'s
  exact signatures, `IsAuthorized(email string) bool` at
  `internal/userstore/userstore.go:117`) and closes the socket with code
  4003 on removal (4000/4001 codes are already used elsewhere per AGENTS.md's
  WS protocol description — 4003 is free).
- `userstore` membership surface (`internal/userstore/userstore.go`):
  `IsAdmin(email string) bool` (110), `IsAuthorized(email string) bool`
  (117), `List() []string` (172) — `IsAuthorized` is the one to call on a
  timer.
- Risk: the close code needs to reach the frontend's terminal reconnect
  logic gracefully (check `web/src/` for existing 4000/4001 handling and add
  4003 alongside, not instead) — a WS ticker running for potentially many
  concurrent sessions needs its own cancellation tied to session teardown to
  avoid goroutine leaks; `serve`'s existing session lifecycle (channels/ctx)
  should already provide a natural place to hook a `time.Ticker` with
  `defer ticker.Stop()`.

## J — Ingress `trust login redirect uri domain suffix` + caddy validate gate

- `deploy/caddy/ingress/generator.go:262` (not literally 260, close enough —
  drift): `fmt.Fprintf(b, "            trust login redirect uri domain suffix
  %s path prefix /\n", cfg.BaseDomain)` inside `writeSecurityBlock`
  (`generator.go:248-...`).
- No test asserting this specific line's VALUE was found via `grep -n "trust
  login redirect\|redirect uri domain suffix" deploy/caddy/ingress/*_test.go`
  — zero hits. TODO's claim ("Ingress: … narrow the suffix … confirm caddy-
  security's matcher has a label boundary") is confirmed as untested.
- `deploy/verify-commit.sh` builds `deploy/caddy/ingress` only as a Go
  module (`go vet`/`go test`, line 44) — it never builds the actual Caddy
  DOCKER image (`deploy/caddy/Dockerfile`) nor runs `caddy validate`/`caddy
  adapt` on a generated Caddyfile. Confirmed: no `caddy validate`/`caddy
  adapt` string anywhere under `deploy/`.
- Minimal fix for the redirect-suffix finding: narrow
  `cfg.BaseDomain` to the specific app host in the format string, or confirm
  (and comment/test) that caddy-security's suffix matcher already has an
  implicit label boundary (may be a no-op fix if the library is already
  safe — check caddy-security's docs/source for `domain suffix` matching
  semantics before changing the generated config, since a wrong narrowing
  could break legitimate redirects across the zone's public-preview
  subdomains).
- Wiring `caddy validate` into the gate: would need
  `deploy/verify-commit.sh` to `docker build -f deploy/caddy/Dockerfile`
  (parallel to how it already builds `deploy/Dockerfile.server` and
  `image/Dockerfile`), then `docker run` that image with `caddy validate
  --config <generated-Caddyfile>` against one authenticated app's generated
  output — TODO references `testing/2026-09-04-verify-session-lifetime.md`
  as the manual precedent to convert into an automated step.
- Risk: adding a full Caddy image build to `verify-commit.sh` extends its
  runtime (already builds 2 other images); the redirect-suffix narrowing
  risks breaking real preview-link redirects if done carelessly — test
  against a real generated Caddyfile for an app with the public-preview
  wildcard before shipping.

## K — Nine carried-over lows

All confirmed present, independent, one-line-ish each:

1. **`deploy/update/styrman-update.timer`** — `Persistent=true` present
   under `[Timer]` alongside `OnBootSec=5min`/`OnUnitActiveSec=5min` — inert
   on a monotonic timer per TODO (systemd's `Persistent=` only matters for
   calendar (`OnCalendar=`) timers). Fix: delete the line.
2. **`deploy/host-setup.sh` step "8b"/"8c" ordering** — grep shows only
   `# ─── 8c. Tag-gated auto-update` (line 257) and a comment at line 356
   noting "Step 8b, the restic→GCS host backup layer, was REMOVED" — i.e.
   **8b no longer exists as a step**, so TODO's "step 8c is still physically
   ordered before step 8b" may already be MOOT (8b was removed entirely, not
   just reordered). Re-verify by reading the file's step numbering in full
   before treating this as still open — likely closeable as "already
   resolved by 8b's removal," not a code change.
3. **`deploy()` `fail` vs `return 1`** — `deploy/update/styrman-update`:
   the `deploy()` function body (found via grep, exact function boundaries
   not fully re-read) has multiple `return 1` lines (299, 320, 326, 332) for
   build/health-check failures, but per TODO the checkout-error path still
   calls `fail` (which the script defines to `exit` outright) instead of
   `return 1`, bypassing the "failed-tag latch" the other paths hit. Fix:
   find the specific checkout `git checkout`/`git fetch` inside `deploy()`
   and change its error handler from `fail "..."` to `echo "..." >&2;
   return 1`, matching the surrounding style.
4. **`mktemp -p .`** in `internal/httpapi/check.go:406` (`checkScript`) and
   `internal/httpapi/format.go:120` (`formatScript`) — confirmed: both build
   an identical `F=$(mktemp -p . --suffix=.%s) || ...` shell fragment run in
   the guest's workspace root on every check/format call (debounced, but
   still per-tick). Fix: use a dot-dir, e.g. `mktemp -p .styrman-tmp` (create
   the dir first, `mkdir -p .styrman-tmp` in the same script) so temp files
   don't appear in `git status`/file explorer of the user's actual project
   root even transiently.
5. **Terraform checker `STYRMAN_TOOL gcc`** — `internal/httpapi/check.go:123`:
   `return &checker{tool: "tofu", cmd: \`printf 'STYRMAN_TOOL gcc\n'; tofu fmt
   -check "$F" >/dev/null\`}` — confirmed mislabelled exactly as TODO says
   (runs `tofu fmt -check`, reports itself as `gcc`). Fix: change the printf
   literal to `STYRMAN_TOOL tofu` (or whatever the frontend/parser expects
   for a non-gcc-format tool — check `parseCheckOutput`/`missingTool` in the
   same file for how the tool name is consumed before renaming, in case
   `gcc` selects a specific output-format parser branch that `tofu fmt
   -check`'s actual output still matches — if so, the display label and the
   parser-selection key may need to be decoupled rather than just renamed).
6. **Dead trap globs** — `checkScript`/`formatScript` both do
   `trap 'rm -f "$F" "${F}.err" "${F}.out"' EXIT` (check.go:407) — but the
   script itself never creates `${F}.err` (only `${F}.out` from `formatScript`'s
   equivalent, and even `checkScript`'s own trailing `rm -f "$F" "${F}.out"`
   already covers `.out` before the trap would ever need to). Fix: trim the
   trap to just `rm -f "$F"` (and whatever it ACTUALLY creates) since the
   explicit `rm -f` before each successful exit already cleans up.
7. **`checkFile`/`CodeEditor.tsx` no `AbortSignal`** — `web/src/lib/api.ts`
   `checkFile` (~line 605) calls `request<CheckResult>(...)` with no `init`
   object, so it falls back to `request()`'s default
   `AbortSignal.timeout(20000)` (line 363) — not caller-abortable. CodeMirror's
   `linter()` (CodeEditor.tsx:269) awaits the check promise per debounced doc
   change but has no way to cancel an in-flight one when the doc changes
   again before the 20s timeout — matches TODO. Fix: thread an `AbortSignal`
   through `checkFile(project, path, content, signal?)` → `request()`'s
   `init.signal`, and have `CodeEditor.tsx`'s linter create/abort an
   `AbortController` per debounce cycle.
8. **`lintGutter()`/`linter()` unconditional mount** — `CodeEditor.tsx:268-269`
   — both are in the base `EditorView` extensions array with no per-filetype
   gating; confirmed they mount regardless of whether `checkerFor` (Go side)
   would ever return non-nil for the open file's extension. Fix: only include
   `lintGutter()`/`linter(...)` in the extensions array when the file's
   extension has a known checker (mirror `checkerFor`'s switch cases on the
   frontend, or have the backend's `/api/settings/tools`-adjacent endpoint
   report supported extensions once and gate on that).
9. **`EditorPane.tsx` "Discard" button discards nothing** — confirmed exactly:
   `EditorPane.tsx`'s `confirmClose` dialog (lines ~203-215, TODO said
   ~171-178 — line-number drift, same logic) `onConfirm` handler is:
   ```tsx
   onConfirm={() => {
     setConfirmClose(false);
     onShowTerminal();
   }}
   ```
   — it never resets dirty state or the draft buffer; it just closes the
   dialog and switches views, leaving the "discarded" edits sitting in the
   per-terminal draft store (per the v1.19.0 change note, drafts are now
   keyed by terminal ID and persist across switches) — so the user's edits
   are NOT actually discarded, contradicting the button's label. Fix: the
   `onConfirm` needs to also clear/reset the draft for `loaded.req` (whatever
   the pane store's reset method is — check the store referenced by `st` in
   this component, likely `usePaneStore` or similar, not read in this pass).

Risk for K as a batch: each is independent and small; #2 may already be
moot (verify before touching); #5's rename could affect a parser branch
keyed on the string "gcc" (check before renaming); the rest are safe
one-line-to-few-line changes. Good candidate for one combined commit with
nine small diffs, but each deserves its own test-touch where a test exists
(only #4/#5/#6 have `internal/httpapi/check_test.go`/`format_test.go`
coverage worth checking for string-matching against the old `STYRMAN_TOOL
gcc`/trap literals before changing them — a test asserting the exact script
string would break on rename).

## L — `styrman-runtime:test` freshness

- `web/e2e/run.sh:14`: `IMAGE="${1:-styrman-runtime:test}"`.
- `.agents/docs/testing/styrman/integration.sh:18`: same default,
  `IMAGE="${1:-styrman-runtime:test}"`, with a comment (lines 9-10)
  explicitly noting production's `deploy/.env` sets `STYRMAN_IMAGE=
  styrman-runtime:dev`, so `:test` is a deliberately separate, isolated tag —
  not re-stamped by test runs.
- BUG-026 (`.agents/docs/bugs/BUG-026-stale-pinned-runtime-test-image.md`,
  read in full) is the root-cause record: the `:test` tag went stale
  (2026-09-04 08:22 build) while `image/skel/bash_exports` changed later the
  same day (14:59, v1.22.0's `GOPATH="$HOME/.go"`), producing 3 false
  integration failures that read as a regression. Fix was a manual rebuild;
  no prevention landed — this TODO item IS the prevention work.
- What `image/` sources would decide staleness: `image/Dockerfile`,
  `image/entrypoint.sh`, `image/skel/*` (shown concretely responsible in
  BUG-026), and `image/harness/` (the vendored mirror, has its own
  `SHA256SUMS`/`COMMIT` files already used by `verify-commit.sh` for a
  DIFFERENT integrity check — could potentially be reused/extended here).
- Fix idea per TODO: either (a) have the suites assert `:test`'s build time
  postdates the newest mtime/commit-touch under `image/`, or (b) make
  rebuilding `:test` a prerequisite step both suites run automatically
  (`docker build -t styrman-runtime:test image/` before executing, skippable
  via an env var for fast iteration). (b) is more robust against silent
  staleness but adds build time to every suite run; (a) is cheaper but only
  catches drift, doesn't fix it — TODO explicitly leaves the choice open
  ("decide whether the suites should assert the tag's age or rebuild it").
- Risk: low, testing-infrastructure only; whichever is chosen, keep `:test`
  and `:dev` semantically separate (the whole point per the integration.sh
  comment) — don't let a "rebuild before test" step accidentally retag
  `:dev`.

## M — `default-address-pools` + firewall unit ordering

- `default-address-pools` is **not currently set** in `deploy/host-setup.sh`'s
  daemon.json merge (`host-setup.sh:90-145`, read in full) — the merge
  handles `runtimes.mvm`, `features.time-namespaces`, `ipv6`/`ip6tables`
  (both forced off), `live-restore`, `log-driver`/`log-opts`, but nothing
  named `address-pools`. TODO item (d) under the ADR-065 follow-ups bundle
  ("pin default-address-pools to 172.16.0.0/12... today the conf validator's
  overlap refusal fails closed") is confirmed genuinely unimplemented — this
  is real, additive work: add a `default-address-pools` key to the same
  python `cfg` dict with the 172.16.0.0/12 base/size, following the existing
  `changed = True` idempotence pattern.
- Firewall unit ordering: `deploy/firewall/styrman-firewall.service` has
  `After=docker.service` only — **no `Before=` line exists**. TODO item (f)
  ("run the base section Before=docker.service to close the pre-existing
  ≤15s boot window") wants this reversed/added — but note items (e) and (f)
  literally describe wanting Docker-restart-survival AND running the
  firewall BEFORE docker starts, which are two different concerns (live-
  restore across a daemon restart, vs the boot-time race before docker is
  even up) — read them as two separate small tasks, not one.
  `styrman-firewall.timer` re-runs every 60s (`OnUnitActiveSec=60s`,
  `OnBootSec=15s`) — the "≤15s boot window" is exactly this `OnBootSec=15s`
  value, i.e. between docker.service starting and the firewall's own first
  boot-triggered run.
- BUG/ADR notes constraining this: ADR-036/067 (referenced throughout
  AGENTS.md's firewall description) — read `.agents/docs/architecture/
  DECISIONS.md`'s ADR-067 entry before touching unit ordering, since it's
  the entry that "closed" several fail-opens in this exact area very
  recently (v1.25.0, per git log) — a change here risks reopening something
  that ADR-067 just closed three review rounds to fix. **Do not touch
  firewall unit ordering without re-reading ADR-067 in full first.**
- Risk: medium for the unit-ordering half (touches boot-time security
  ordering that just went through heavy review), low for the
  `default-address-pools` addition (purely additive daemon.json key,
  same idempotent-merge pattern already used for 5 other keys).

## N — Provisioning state

- No dedicated `Project.State` field was found in `internal/project`'s
  package files (`store.go`, `uuid.go`) — `grep -rn "\"creating\""` across
  `internal/project` returned nothing; the string `"creating"` is only
  synthesized in `internal/httpapi/server.go` at the two `POST` handlers
  that create projects (lines 841 and 893): `writeJSON(w,
  http.StatusCreated, projectView{Project: p, State: "creating"})`. The
  `projectView` struct (`server.go:619-...`) has a `State string
  \`json:"state"\`` field that is populated per-response, not stored — for
  GET/list responses, `state := st.State` (server.go:907) reads the live
  orchestrator status (`st` from `orch.Status(...)`, which reports
  "running"/"stopped"/"absent" etc., not "creating" — "creating" is a
  purely synthetic client-visible state that only ever appears in the
  immediate 201 response body of project creation, never persisted or
  re-derived on a later GET).
- `provisionRepo` — `internal/httpapi/provision.go:113-...` — the background
  goroutine that fills a newly-created project's workspace (clone or GitHub
  repo). `setProvisionError`/`provisionError` (lines 28, 53) manage the
  error string surfaced to the SPA; `bootFailed` (line 86) is the shared
  failure path with the same `provisionSpec`.
- SPA rendering: `web/src/components/Sidebar.tsx` — state badge logic at
  lines ~218-330: `p.state === "running"`/`"deleting"` branches render
  differently; `p.provisionError && !p.deleteError && p.state !== "deleting"`
  (line 281) renders the provision-error badge; `p.deleteError && p.state
  !== "deleting"` (line 273) renders the delete-error badge. **No branch
  for `p.state === "creating"` was found in the grep** — worth confirming
  directly whether the Sidebar shows anything distinct during the create→
  clone window (TODO's core complaint: "no provisioning state in the UI for
  the window between the 201 and the clone finishing").
- e2e coverage: `web/e2e/run.mjs` has multiple `provisionError` checks
  (lines 1296-1368, "failed clone reports provisionError") but **zero
  `deleteError` occurrences** — confirms TODO's second item (Sidebar's
  `deleteError` badge has no e2e coverage) exactly.
- Fix shape for the missing provisioning-state UI: needs either (a) the
  201 response's `"creating"` state to be echoed back on subsequent GETs
  until `provisionRepo` finishes (would need a stored/derived flag, not
  synthetic-on-create-only), or (b) the SPA to locally track "just created,
  not yet confirmed a repo" client-side until the first successful poll —
  (a) is more correct (survives a page reload mid-provision) and needs a
  small addition to how `projectView`'s `State` is computed for GET, likely
  keying off `provisionError == "" && <no .git yet>` or a small persisted
  marker; this is the more involved half of item N.
- Risk: R13 ("every project IS a repo") is a named requirement — the UI gap
  is cosmetic (state exists, just not shown), but any fix that changes how
  "creating" is derived needs to not violate R13's timing guarantees.
  `deleteError` badge test is low-risk, pure test-writing, copy the
  `provisionError` pattern from `run.mjs`.

## O — Fake Docker client seam

`grep -on "o\.docker\.[A-Za-z]*" internal/orchestrator/*.go` → **61 call
sites**, **19 distinct methods** (matches TODO's count exactly):
`ContainerInspect` (×11), `ContainerRemove` (×10), `ContainerStart` (×3),
`ContainerStop` (×2), `ContainerCreate` (×1), `ContainerList` (×4),
`ContainerLogs` (×1), `ContainerExecCreate`/`ContainerExecAttach`/
`ContainerExecInspect`(×2)/`ContainerExecResize` (in `exec.go`, ×6 total),
`NetworkCreate` (×1), `NetworkRemove` (×5), `NetworkDisconnect` (×2),
`NetworkInspect` (×6), `NetworkConnect` (×3), `NetworkList` (×2),
`ImageInspect` (×1), `Ping` (×1). Spread across `orchestrator.go` (bulk),
`exec.go` (the 6 Exec-family calls), `ports.go` (1 `ContainerInspect`).

- Functions that would become unit-testable per TODO: `ensureRunningLocked`
  (166 lines per TODO's own sizing — the biggest win, since it's where
  BUG-002/003's symlink-on-bind-source guard lives), `create`,
  `bridgeModeMismatch`, `currentBridge`, `stopMarked`,
  `unmarkIfStillRunning` (item E above — same function, second reason to fix
  it now), `Status`, `Destroy`.
- Precedent to copy: `internal/httpapi/server.go:76-88`'s `guestRuntime` and
  `modeRuntime` — narrow interfaces scoped to exactly what ONE caller needs
  (2 methods, 1 method respectively), not a full Docker client mirror; a
  fake implementing `guestRuntime` already exists and is used in
  `internal/httpapi/guestexec_test.go` (26 handler tests). The same style
  applied to `internal/orchestrator` would mean a `dockerAPI` interface
  listing only the ~19 methods orchestrator code actually calls (a real
  subset of `*client.Client`'s hundreds of methods), with `o.docker`
  retyped from `*client.Client` to this interface, and a hand-written fake
  in a new `_test.go` file implementing it (call-recording + scriptable
  return values, matching the shape `guestexec_test.go`'s fake likely
  already uses — worth reading that fake's structure directly before writing
  a new one, to match its conventions).
- Risk: this is explicitly sized by TODO itself as "its own task, roughly
  the size of the seam just done" (the `guestRuntime` seam, which shipped as
  its own v1.22.0 work item) — NOT a quick win, the largest single item in
  this whole batch. `EnsureRunning`/`ensureRunningLocked` is also where the
  guest-writable-paths rule (`.agents/rules/guest-writable-paths.md`) lives
  most densely (`realBelow`/`projectHomeIsReal` mount-source symlink guard)
  — a refactor here needs the symlink-planting test pattern that rule
  mandates (`internal/orchestrator/realbelow_test.go` already exists per
  that rule file — read it before writing the new fake, to make sure the
  interface change doesn't accidentally bypass or duplicate that guard).

## Conventions

- **Test server construction**: `testServer(t *testing.T) *Server` in
  `internal/httpapi/provision_test.go:25-42` is the canonical pattern —
  `t.TempDir()`, a real `config.Config{StateDir, HomesDir, Username: "tester",
  AuthMode: "none", SelfContainer: "styrman-server"}`, `slog.New(slog.
  DiscardHandler)`, a real `userstore.New(...)`, a real
  `orchestrator.New(cfg, logger)` (no Docker mock exists today — this is
  exactly what item O would change), then `New(cfg, users, orch, logger)`.
  Helper functions (`newProject`, `seed`, `toolsByName`) live alongside
  their primary test file and are reused across `*_test.go` files in the
  same package.
- **Narrow interface seams**: `guestRuntime`/`modeRuntime` in
  `internal/httpapi/server.go` — small, caller-scoped interfaces over the
  orchestrator, not full mirrors; this is the pattern item O should follow
  for `internal/orchestrator` itself.
- **Error wrapping**: `fmt.Errorf("... : %w", err)` throughout, almost
  always with a human sentence explaining what state the system is now in
  (see `network.go`'s error messages — each one states explicitly whether
  anything changed, e.g. "so the mode was NOT changed"). This is a strong,
  consistent house style — new error paths should match it, not just wrap.
- **Logging**: `slog` with structured key-value pairs (`s.logger.Warn("...",
  "project", p.ID, "requested", on, ...)`), always at a `Warn` or `Info`
  level tied to state that matters operationally, never bare `fmt.Println`.
- **Comments as design record**: nearly every non-trivial function/branch in
  this codebase carries a comment naming the BUG-/ADR-/review that produced
  it — new code touching these files should keep that convention (helps the
  next recon and the next reviewer).
- **File safety**: `.agents/rules/writing-files.md` and
  `guest-writable-paths.md` are load-bearing for this codebase specifically
  — `migrate_test.go` (item F) is literally the file BUG-005 was about;
  treat every `Write`/`cat >` on an existing path as a hazard by default.

## Risks

- **O is the real size driver.** If this "one pass" is meant to be scoped
  tightly, consider splitting O into its own follow-up rather than bundling
  it with the other fourteen — it alone is comparable in size to all of K
  combined.
- **B needs a product decision** (strip the timestamp vs gitignore the
  churn) before code — don't default to "just sed it" without confirming
  the timestamp isn't read anywhere in `web/src` first (not verified in this
  pass).
- **M's firewall unit-ordering half touches an area three security review
  rounds (ADR-067) just finished closing** — re-read ADR-067 before any
  edit near `styrman-firewall.service`/`.timer`.
- **F's actual residual risk may be smaller than TODO describes** — the
  legacy-workspace read already goes through `files.ContainedPath`, not a
  raw `EvalSymlinks`+rename; verify current risk before scoping a fix that
  may already be half-done.
- **A's fix may not be sufficient by itself** — bumping `go.mod` versions in
  the ingress module doesn't guarantee `xcaddy build` (which does its own
  dependency resolution) picks them up; the real proof is a Dockerfile
  rebuild, not just `go test`.
- **K item 2 (host-setup 8b/8c ordering) may already be moot** — step 8b
  (restic→GCS backups) was removed entirely per a comment in the file;
  confirm before spending time "reordering" a step that no longer exists.
- **K item 5's rename risk** — `STYRMAN_TOOL gcc` may be a parser-selection
  key, not just a display label; check `parseCheckOutput`'s consumption of
  the tool name before renaming blindly.
- Several items reference line numbers that have drifted by a handful of
  lines from TODO.md's original text (normal churn since those entries were
  written) — re-`grep -n` immediately before editing rather than trusting
  the numbers quoted here or in TODO.md verbatim.
