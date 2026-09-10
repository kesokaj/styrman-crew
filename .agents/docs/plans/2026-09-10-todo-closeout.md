---
owner: main
date: 2026-09-10
---
# Plan — TODO close-out: fix every open item or strike it   (2026-09-10)

## Why
The operator (2026-09-10): "Plan ALL todo items and make one big fix for all
that is left or remove from todo / roadmap", then "go all" to a per-item
recommendation. TODO.md had 29 open items; 10 were struck the same day with
reasons (commit follows this plan). This plan fixes the 19 that remain,
except the one operator-only item (bucket deletion, backup shredding), which
stays as the operator's. Recon: `plans/2026-09-10-todo-closeout-recon.md`
(explorer; item letters A–O below are its).

## Scope
In: every unstruck TODO item except the operator-only one. Out: a release
(not asked; the work ships with the next tag), ROADMAP Next/Later (empty by
design), the styrman-harness repo. Follow-ups noted, not built: none — the
point is that nothing is left.

## Surfaces touched
Go: `internal/httpapi/{network,setup,git,check,format,server,provision}.go`,
`internal/orchestrator/{orchestrator,exec,ports}.go`, `internal/terminal/bridge.go`,
`internal/config/config.go`, `cmd/styrman/main.go` (+ `migrate_test.go`),
`internal/files`. Web: `web/src/lib/{sshKey,api}.ts`,
`web/src/components/{CodeEditor,EditorPane,Sidebar}.tsx`, `web/package.json`,
`web/e2e/run.mjs`. Deploy: `deploy/caddy/ingress/go.mod`,
`deploy/verify-commit.sh`, `deploy/update/{styrman-update,styrman-update.timer}`,
`deploy/host-setup.sh`, `deploy/firewall/styrman-firewall.service`,
`.agents/docs/testing/styrman/integration.sh`, `web/e2e/run.sh`. Docs:
`specs/styrman/{api,guide,deploy}.md`, `deploy/remote.md`, testing README.

## Interfaces (concrete decisions)

**C — reply after the lock.** The `WithProject` closure in `setProjectNetwork`
returns nothing today and writes seven times. New shape:
```go
type reply struct{ status int; body any; err error }   // exactly one of body/err
var r reply
s.mode.WithProject(u.Username, p.ID, func(pr orchestrator.Project) { r = decide(pr) })
if r.err != nil { writeErr(w, r.status, r.err) } else { writeJSON(w, r.status, r.body) }
```
`decide` stays a closure (it needs `ctx`, `sp`, `u`, `p`, `on`, `persist`).
`r` is initialised FAIL-CLOSED before the call: `reply{status: 500, err:
errors.New("the mode was NOT changed: no decision was made")}`, so a runtime
that never invokes `fn` cannot produce a zero status. The self-attach
goroutine stays where it is (fire-and-forget). Tests: the existing 19 in
`network_test.go` keep their assertions; add one that the fake's
`WithProject` has RETURNED before the response is written (the fake sets
`inFn=false` on return; the handler test asserts the body was written after
that), and one that a fake which never calls `fn` yields the 500.
Process note: C and E are further rounds on the mechanisms behind
BUG-028..032; AGENTS.md's "ask what the mechanism is for" fired on 2026-09-10
and the operator's "go all" is the answer — do not reopen.

**D — `toolDetails` key test.** New `TestToolDetailsEchoesOnlyAPublicKey` in
`setup_pubkey_test.go` (append): `testServer(t)` + `toolsByName`, seed
`cfg.UserHome("tester")/.ssh/` with `id_ed25519` (PEM text) and, per
subtest, `id_ed25519.pub` as symlink → private, hard link → private, byte
copy of private, a real key line: `Details` empty for the first three,
`[key]` for the fourth. `files.Trust(cfg.HomesDir)` as production does.

**5 — full type token server-side.** `publicKeyLine` accepts the type only
if it matches `^(ssh-(ed25519|rsa|dss)|ecdsa-sha2-nistp(256|384|521)|sk-(ssh-ed25519|ecdsa-sha2-nistp256)@openssh\.com)$`
(the OpenSSH key types that produce a `.pub`). `sshKey.ts`'s `PUBLIC_KEY`
already requires a full token; it is aligned to the same enum so the two
never disagree. What the server-side enum buys: the API is consumed by
more than this browser page, so the server, not the SPA, is where the
guarantee lives (defence in depth, one place). Tests both sides: `ssh-
AAAA`, `sk- A`, `sk-ant-api03-…` rejected; each real type accepted. Only
`id_ed25519.pub` is read today, so the other types are exercised by the
unit test, not the handler.

**7 — comment stripping.** `publicKeyLine` returns the SANITISED line:
signature becomes `func publicKeyLine(line string) (string, bool)`; the
comment field goes through `stripControls` (server.go) and Unicode spaces
collapse to one ASCII space; `toolDetails` echoes the returned string.

**E — `unmarkIfStillRunning`.** Both callers hold the per-project lock, so
the two samples must NOT sleep under it. The marker's stored VALUE becomes
the stop's start time (`o.stopped.Store(key, time.Now())`; `Stopped()`
still only asks presence). `unmarkIfStillRunning(user, project, marker
time.Time)` returns at once and does its work in a goroutine: inspect,
sleep 2 s, inspect again; if BOTH report Running, `o.stopped.CompareAndDelete(key,
marker)` — so a NEWER stop that re-marked the project during the 2 s keeps
its own marker (its value differs). Each inspect has its own 5 s budget.
`Warn` on an inspect error ("marker kept: cannot inspect") and on
not-running ("marker kept: the container is down — the stop half-succeeded").
The goroutine is bounded (≤ 12 s) and needs no stop hook. Callers change
from `o.unmarkIfStillRunning(ctx, user, project)` to passing the value they
stored. Unit test lands in Phase 5 (needs the fake).

**F — delete the legacy workspace path.** Remove `migrateWorkspaces`,
`config.LegacyWorkspaceExists`, `ensureRepos`'s guard branch, the
orchestrator's "workspace migration incomplete" branch, and their tests in
`cmd/styrman/migrate_test.go` (deleted tests are of deleted code; the
commit message and `changes/` entry name each). Decision basis: single
self-hosted deployment (AGENTS.md "single NUC target"), migrated at
v1.19.x; a pre-ADR-027 tree would today be at `homes/<u>/base/projects/`,
and the server logs a WARN at every start for the missing path. Also
removes handoff item 7's benign WARN.

**G — gitconfig.** `gitIdentity` reads `.gitconfig` with
`files.ReadRaw(s.cfg.UserHome(user), ".gitconfig", 64<<10)` and parses it
with a new `gitUserFromConfig(text string) (name, email string)` in
`git.go`. Grammar, matching `git config --file` for the two keys that
matter: section headers `[user]` case-insensitive, with optional
surrounding whitespace; keys `name`/`email` case-insensitive; `=` with any
whitespace around it (`name=x` accepted); a value optionally in double
quotes with `\"` and `\\` unescaped; `#` and `;` start a comment outside
quotes; the LAST occurrence wins, across reopened `[user]` sections; any
other section is skipped, including subsections (`[user "x"]` is not
`[user]`). `toolDetails`'s git `Ready` flag becomes "email non-empty" from
the same parse. `toolDetails`'s git card switches to the same
function so the card and the commit identity cannot disagree (its display
changes only where the old prefix scan was wrong: `name=x` and quoted
values). Tests: the six grammar cases above plus a missing file. The `git
config` exec goes away. Includes: verify at implementation whether `git
config --file` honours `[include]`; the comment states the measured
difference, if any.

**H — frontend key hygiene.** Reject `\p{C}` (all of it) except TAB;
`abbreviateKey` collapses `\p{Zs}` runs to one space. Tests: RLO, ZWSP,
BOM, U+00AD, NBSP cases.

**I — WS re-authorization.** `NewBridge` gains a seventh parameter
`stillAuthorized func(email string) bool` (nil = never re-check). The
server passes `s.users.IsAuthorized` when `cfg.AuthMode == "header"` and
nil otherwise — in `AuthMode=none` (local dev, e2e) identity is minted from
`cfg.DefaultUser()` and is not in the store, so a re-check there would close
every terminal; skipping is the correct behaviour, not a gap. Both WS
handlers (`ServeHTTP`, project terminals; `/ws/setup`) capture
`identity.FromContext(r.Context()).Email` at upgrade time and pass it to
`serve`. `serve` runs a `time.Ticker` (interval from an unexported field,
default 60 s, `defer Stop`) alongside its existing read loop; on
`!stillAuthorized(email)` it writes close **4003** "access revoked" and
returns. Frontend: `TerminalPane.tsx` (which already maps 4000/4001/4002)
adds 4003 → status "revoked", no reconnect, message "Access revoked". Test:
`internal/terminal` bridge test with a fake that flips to false, interval
10 ms, asserting close code 4003 and that a nil func never closes.

**J — ingress.** BOTH STRUCK by the operator (2026-09-10, after the
cold-reader's third round, "Strike both"). (1) Redirect-trust narrowing:
with `APPS_DOMAIN` unset `AppsDomain == BaseDomain` and the shell is an app
under it, and the generator's single `trust login redirect uri domain
suffix` line reads `BaseDomain` only (`generator.go:264`), so no trust list
can keep login working and exclude the preview wildcard; the review's
"landing-page ceiling" (cookies and identity stripped before a guest app)
stands as the accepted residual. (2) `caddy validate` in the gate: the
generated config resolves `{env.*}` secrets at load, the ingress module has
no generator entry point, and the step would add a Caddy image build to
every gate run to prove syntax of a config filled with dummy values; the
manual validation precedent (`testing/2026-09-04-verify-session-lifetime.md`)
stands. TODO lines record both.

**K — nine lows.** (1) delete `Persistent=true`. (2) verify 8b is gone; if
so, no change — note it. (3) ALREADY CLOSED (verified 2026-09-10: `deploy()`'s
checkout failure does `log …; return 1`, `styrman-update:560`) — strike
with that note, no edit; likewise (2): step 8b was removed entirely. (4)
temp files leave the workspace: `mktemp -p "$HOME/.styrman/tmp"` after
`mkdir -p` of it (the guest already owns `~/.styrman/`), so nothing
appears in the project tree and no git exclude is needed; a checker that
needs the file inside the workspace (module context) is identified at
implementation and, if one exists, keeps a workspace temp under a
`.styrman-tmp/` that `ensureRepos` adds to `.git/info/exclude` ONCE,
server-side, at provisioning. (5) `STYRMAN_TOOL <name>` is both the parser
key and the label (`check.go:376-399`): the shell prints `STYRMAN_TOOL
tofu`; the parser's `case "gofmt", "shellcheck", "gcc"` gains `"tofu"` if
`tofu fmt -check` emits the same `file:line:col` shape, else a `"tofu"`
case of its own (measured at implementation by running the binary in the
test image); the UI shows "tofu". (6) trim the trap to what is created.
(7) `checkFile(project, path, content, signal?)` → `request(..., {signal})`;
`CodeEditor`'s linter keeps one `AbortController` per debounce and aborts
the previous. (8) ONE design: `api.ts` exports `CHECKABLE_EXTENSIONS`
mirroring `checkerFor`'s cases; `lintGutter()`/`linter()` are added to the
extensions only when the open file matches. Drift guard: `web/e2e/run.mjs`
asks the check endpoint for one file per listed extension (expects a tool
name) and for `.xyz` (expects "no checker"), so the two lists are compared
end to end without cross-language parsing. (9) "Discard" clears the draft
for the closing file in the pane store and marks it clean before switching
views; vitest for the store's `discard(req)`.

**B — `_shell.html` timestamp.** `npm run build` becomes
`vite build && tsc -b && node scripts/normalize-shell.mjs` which replaces
the single `u:<13 digits>` in `dist/client/_shell.html` with `u:0`. First
confirm by grep that nothing in `web/src` or the runtime reads that value
(TanStack Router's dehydrated `u` = router "updatedAt", informational).
Confirmed 2026-09-10: nothing in `web/src` reads it. The script edits
bytes, not text (`Buffer` replace), so the NUL byte `.gitattributes`
documents in that file round-trips; `git diff --numstat` on the path after
the change is part of the box (BUG-007). `verify-commit.sh`'s freshness
check compares names only; unaffected.

**N — provisioning state.** Server: `Server.provisioning sync.Map` keyed
by project ID. There is ONE create handler with two 201 sites; the entry
is set immediately before the boot goroutine is spawned (the `cfg.Image ==
""` early 201 spawns none and sets none), and the goroutine's FIRST
statement is `defer s.provisioning.Delete(p.ID)`, so every return — the
delete-in-flight check, `bootFailed`, `provisionRepo`'s `ctx.Done()` and
plain `git init` paths — releases it. `projectView`'s `State` precedence for
GET/list: `deleting` > `provisioning` > live status; `provisioning` shows
while the container is still absent (that IS the window). API doc:
`state` gains `provisioning`. The precedence is a pure function
`deriveState(deleting, provisioning bool, live string) string` in
`server.go`, unit-tested for every combination; a handler test sets
`s.provisioning` directly and asserts GET and list report `provisioning`
for an absent container, and `deleting` when both are set. SPA: Sidebar
shows a "provisioning…" chip (same slot as deleting) through
`projectBadge` (item 29's function, vitest); no interaction change. No e2e
assertion is made on the transient state — a clone that fails fast may
never be observed mid-window, and an assertion that passes either way is
not a test. The existing failed-clone e2e check keeps asserting that
`provisionError` renders and that the project is usable afterwards.

**29 — deleteError badge.** `deleteError` is in-memory only
(`server.go:139-149`) and `Destroy` tolerates an absent container, so no
e2e can induce it without a fault-injection seam. Demoted, deliberately: the
badge choice moves out of `Sidebar.tsx` into a pure `projectBadge(p)` in
`web/src/lib/projectBadge.ts` (the `lan.ts` pattern; the LAN chip stays in
`lanChip`/`LanBadge` — this covers the state/error badge only), unit-tested for
`deleteError`, `provisionError`, `deleting`, `provisioning`, and their
precedence; the Sidebar renders its result. The item's intent — the badge
cannot silently stop rendering — is met by vitest, and the TODO line says so.

**A — dependency convergence.** In `deploy/caddy/ingress`: `go get` each of
the 8 to the root's version, `go mod tidy`, `go test ./...`, then `docker
build -f deploy/caddy/Dockerfile` and `docker run … caddy version` +
`caddy list-modules | grep security` to prove xcaddy picked them up; and J's
`caddy validate` on a generated config. `verify-commit.sh`'s divergence
list must print empty.

**L — `styrman-runtime:test` freshness.** Both suites call a shared
`.agents/docs/testing/styrman/ensure-test-image.sh <tag>`: if the image is
missing or `docker image inspect --format '{{.Created}}'` is older than the
newest mtime under `image/` (excluding `image/harness/.git`), rebuild with
`docker build -t <tag> --build-arg STYRMAN_VERSION=test-$(date +%s) image/`.
`STYRMAN_TEST_IMAGE_NO_REBUILD=1` skips. Never touches `:dev`. The
script lives beside `integration.sh` in the docs testing tree, as that
script does. A failed `docker build` is FATAL: the suite exits non-zero
before running one check, so a stale image can never produce results.

**M — host.** (d) `host-setup.sh`'s daemon.json merge adds
`default-address-pools: [{"base":"172.16.0.0/12","size":24}]` when absent
(existing networks keep their subnets; only new ones draw from the pool).
(f) Decide by measurement, in a throwaway netns as the firewall tests do
(`unshare -Urnm`): pre-create `DOCKER-USER`, start `dockerd` there (or read
Docker's source for `DOCKER-USER` handling — it creates the chain only if
absent and never flushes it). If Docker preserves a pre-existing chain,
`styrman-firewall.service` gets `Before=docker.service` + `Wants=network-online.target`
and the base rules run at boot before Docker; if not, keep `After=` and
lower `OnBootSec` to 5 s, and write the finding into ADR-067's amendment
list. Either way the operator runs `sudo bash deploy/firewall/install.sh`
once (root); the plan's e2e criterion for (f) is the boot ordering shown by
`systemctl show -p After -p Before styrman-firewall.service`.

**O — the Docker seam.** `internal/orchestrator/docker.go`:
```go
type dockerAPI interface {  // the 19 methods orchestrator code calls, with client.Client's signatures
    ContainerInspect(ctx, id string) (container.InspectResponse, error)
    ContainerCreate(...) ; ContainerStart ; ContainerStop ; ContainerRemove ; ContainerList ; ContainerLogs
    ContainerExecCreate ; ContainerExecAttach ; ContainerExecInspect ; ContainerExecResize
    NetworkCreate ; NetworkInspect ; NetworkRemove ; NetworkConnect ; NetworkDisconnect ; NetworkList
    ImageInspect ; Ping
}
```
The method list and signatures are generated from a fresh `grep -o
'o\.docker\.[A-Za-z]*'` at implementation and copied from docker v28's
`client.Client` (`ImageInspect` is variadic). `Orchestrator.docker` retyped
to `dockerAPI` (`*client.Client` satisfies it; `New` unchanged). The fake's
"absent" answers are `errdefs.NotFound(...)` so the eleven
`errdefs.IsNotFound` branches are the ones exercised; a scripted generic
error must NOT satisfy it. `fakedocker_test.go`: a call-recording fake with a
scriptable per-method error and an in-memory container/network table
(state Running flag, network bridge option), following `guestexec_test.go`'s
fake style. Tests (each a planted-state case): `ensureRunningLocked` —
stopped container on wrong-prefix bridge → removed and recreated on the
right bridge; network inspect error → refuses to start; symlinked project
home → refuses (the `realbelow_test.go` planting pattern); running
container → left alone, marker cleared; `WithProject`+handler order —
stop, persist, start under one hold (a fake that fails `ContainerStart`
leaves the marker set, record written); `stopMarked` + `unmarkIfStillRunning`
— stop fails and container still running → marker cleared after two
samples; stop fails and container down → marker kept + Warn; `Status`
absent/stopped/running/error; `currentBridge` found/missing/error.
`internal/httpapi`'s fakes stay.

## Failure modes
- C: identical replies; only the write moves after the unlock.
- F: a host that still had a legacy workspace would see its projects with an
  EMPTY `~/projects/<dir>` after upgrade (the tree stays on disk under
  `homes/<u>/base/projects/`). Recorded in the release notes and
  `deploy/remote.md`; not this host.
- I: a removed user's tab closes with 4003 within 60 s; no reconnect.
- J: a wrong suffix breaks post-login redirects for the whole app —
  `caddy validate` proves syntax, the e2e login path (operator, fresh
  sign-in after the next release) proves behaviour.
- L: a missing/old `:test` image is rebuilt (minutes) before the suite;
  the log says so.
- N: a reload mid-provision shows "provisioning" instead of "running".
- O: no runtime change — `*client.Client` still satisfies the interface.

## Verification
Unit: `go test ./... -race` (+ new tests named per item), vitest.
Integration: `integration.sh` (84/0/3 baseline). e2e: `web/e2e/run.sh`
(86/0/2 baseline) + new checks (N, 29). Gate: `deploy/verify-commit.sh HEAD`
(with the new caddy validate step, divergence list empty). Host: (f)'s
ordering check after the operator's `install.sh`. Cloud smoke: n/a.
E2E criteria: golden path — the full e2e suite and the integration suite
stay green on the final tree with the new checks (K8 drift guard, L image
freshness); the provisioning chip is proven by the handler test + vitest,
and seen by the operator on the next project created from a clone URL;
edge — a removed user's open terminal closes with "Access revoked" within a
minute (bridge test; optional manual check by the operator on a throwaway
user).

## Phases
### Phase 0 — Records (done today except the commit)
- [x] Ten strikes in TODO.md with reasons; ROADMAP "Now" points at this plan — verify: `git diff --stat TODO.md ROADMAP.md`, AI
### Phase 1 — Server: small fixes
- [x] C reply after the lock + test — verify: `go test ./internal/httpapi -race`, AI
- [x] D `toolDetails` key test (symlink/hardlink/copy/real) — verify: same, AI
- [x] 5+7 `publicKeyLine` full type enum, sanitised comment, tests — verify: same, AI
- [x] E two-sample unmark + Warn — verify: build/vet; unit test in Phase 5, AI
- [x] F delete the legacy workspace path and its tests; `documentation` updates `deploy/remote.md` (upgrade note: a host still holding `homes/<u>/base/projects/<dir>` must move it by hand before upgrading) and the change entry carries the same sentence — verify: `go test ./...`, `git diff --stat cmd/ internal/config` explained, server start log has no migration WARN, AI
- [x] G confined gitconfig read + parser test — verify: `go test ./internal/httpapi`, AI
- [x] changes entry, commit, push — AI (commit follows; Phase 1 DONE 2026-09-10: reviewer approve-with-nits fixed, security 3 lows fixed, verifier PASS)
### Phase 2 — Web
- [x] H `\p{C}` + `abbreviateKey` space collapse + tests — verify: vitest, AI
- [x] K7 AbortSignal through `checkFile`; K8 lint gutter gated on `checkable`; K9 Discard clears the draft (+ store test) — verify: `npm run check && lint && test`, AI
- [x] B `_shell.html` normalisation script (after confirming nothing reads `u`) — verify: two builds produce identical `_shell.html`, e2e green, AI
- [x] N provisioning state: server map + `deriveState` + handler test + API doc + Sidebar chip via `projectBadge` — verify: `go test ./internal/httpapi`, vitest, AI
- [x] 29 `projectBadge` extraction + vitest — verify: `npm test`, AI
- [x] rebuild dist, changes entry, commit, push — AI
### Phase 3 — Deploy and scripts
- [x] A bump the 8 ingress deps; ingress tests; caddy image build proves the versions — verify: `verify-commit.sh` divergence list empty, AI
- [x] K1 timer flag; K2 and K3 already closed — strike with the note; K4 temp outside the workspace; K5 tool label + parser case; K6 trap — verify: `bash -n`, `go test ./internal/httpapi` (check/format tests), AI
- [x] L `ensure-test-image.sh` wired into both suites — verify: run with a touched `image/skel` file → rebuild observed, AI
- [x] M(d) address pool in host-setup merge (idempotent, `python3` merge test by running the snippet on a fixture) — verify: dry run against a fixture daemon.json, AI; applied by the operator later
- [x] M(f) ordering decided by measurement; unit change or `OnBootSec`; ADR-067 amendment — verify: firewall tests (`deploy/firewall/*_test*`) + `systemctl show` after the operator's install, both
- [x] changes entry, commit, push — AI (M(f)'s boot-order verification is the operator's after the next `install.sh`)
### Phase 4 — Terminal re-authorization
- [x] I ticker + 4003 + frontend handling + bridge test — verify: `go test ./internal/terminal -race`, vitest, e2e green, AI
- [x] changes entry, commit, push — AI (with Phase 2, one commit: the two share api.ts and server.go)
### Phase 5 — The Docker seam
- [ ] O `dockerAPI` + fake + the listed orchestrator tests (incl. E's) — verify: `go test ./internal/orchestrator -race`, mutation-check the wrong-bridge and symlink cases, AI
- [ ] BUG-003 residual closed in TODO; changes entry, commit, push — AI
### Phase 6 — Definition of Done over the whole change
- [ ] verifier (all tiers, counts before/after), reviewer (diff), security (auth/ingress/files touched), documentation (api.md `provisioning` state and WS close 4003 "access revoked", guide, deploy docs, testing README counts), changelog, bugs — verify: artifacts in `testing/`, `reviews/`, `changes/`, AI
- [ ] integration 84+/0 and e2e 86+/0 on the final tree — verify: logs, AI
- [ ] TODO.md has no unticked item except the operator's; ROADMAP "Now" updated — verify: `grep -c '^- \[ \]' TODO.md` = 1, AI

## Risks
- O is the size driver (61 call sites); it is last so the rest lands even if
  O runs long. A session ending mid-O resumes at its box.
- A may change what xcaddy builds — the Docker build is the proof, and the
  operator's fresh sign-in after the next release is the behavioural one.
- M(f) touches boot ordering in the area ADR-067 just closed; measurement
  before change, and the ADR gets the amendment either way.
- F deletes a migration; the only known deployment has no legacy tree.
