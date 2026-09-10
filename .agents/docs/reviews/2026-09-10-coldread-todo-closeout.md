---
owner: reviewer
date: 2026-09-10
---
# Review — cold read of the TODO close-out plan   (2026-09-10)

Range reviewed: `.agents/docs/plans/2026-09-10-todo-closeout.md` (with
`.agents/docs/plans/2026-09-10-todo-closeout-recon.md` and `TODO.md` as cited
context), plus the code surfaces the plan names. No diff — this is the
cold-reader check on a plan document, not on a change set.

## Verdict: REQUEST CHANGES

Three items cannot be implemented as written because they rest on code that
does not work the way the plan says. Item **29** builds its e2e check on a
`deleteError` "seeded in the project store … which the server reads on the next
list" — `deleteError` lives only in `Server.deleteErrs`, an in-memory map
(`internal/httpapi/server.go:139-149`, read at `:1047-1052`); nothing reads it
from a store, so neither the primary nor the fallback induction path exists.
Item **I** (60 s WS re-authorization against `users.IsAuthorized(email)`) fires
in `auth-none` mode, where `authGuard` mints `identity.User` from
`cfg.DefaultUser()` with `Authorized: true` (`internal/httpapi/auth.go:85-87`)
while `IsAuthorized` returns false for any email not in the store and for ""
(`internal/userstore/userstore.go:116-128`) — every local-dev and
`web/e2e/run.sh` terminal would close with 4003 after a minute, and the plan's
verification line for that phase is "e2e green". Item **J1** narrows the
redirect suffix to `cfg.AppHost`, a field that does not exist and whose intent
is ambiguous against six host fields (`deploy/caddy/ingress/generator.go:12-28`)
serving several legitimate post-login redirect targets. Everything else is
implementable but leaves ~10 decisions to the implementer.

## Findings — gaps that must be decided before implementation

- **[high / confidence: high]** `internal/httpapi/server.go:139-149,1047-1052`
  (item 29) — `deleteError` is in-memory only; `deletingState` returns
  `s.deleteErrs[key]`. The plan's fallback ("seed a `deleteError` in the
  project store via the test's state dir, which the server reads on the next
  list") is impossible, and the primary path (`DELETE` of a project whose
  container is absent) is not an error path — `Destroy` tolerates absence via
  `errdefs.IsNotFound` (`internal/orchestrator/orchestrator.go:696,707`). Decide
  the induction mechanism up front: e.g. a test-only injection point, an
  orchestrator fake behind the existing `guestRuntime`/`modeRuntime` seam, or
  demote item 29 to a vitest/component test of the Sidebar badge.
- **[high / confidence: high]** `internal/httpapi/auth.go:85-87` +
  `internal/userstore/userstore.go:116-128` (item I) — the ticker must not run,
  or must consult something else, when `AuthMode` is `none`. `identity.User`
  already carries `Authorized`; decide whether the bridge re-checks
  `IsAuthorized(email)` only in header mode, or whether the injected `authz`
  is the thing that differs per mode. Without this the e2e suite and every
  local session lose their terminals every 60 s.
- **[high / confidence: medium]** `deploy/caddy/ingress/generator.go:12-28,264`
  (item J1) — no `AppHost` field exists. The portal issues redirects for
  `ShellDomain`, for role-gated apps under `AppsDomain`, and for email-gated
  previews under `PrivateDomain`; narrowing the single `trust login redirect
  uri domain suffix` line to one host silently breaks the others. Decide which
  hosts stay trusted, whether caddy-security accepts repeated `trust login
  redirect uri` lines, and what the generator emits when
  `PrivateDomain`/`AppsDomain` are empty — before reading the matcher source,
  not after.
- **[medium / confidence: high]** `internal/httpapi/server.go:836-842,857-860`,
  `internal/httpapi/provision.go:113-132` (item N) — "set before the 201,
  deleted at the end of `provisionRepo` and `bootFailed`" leaks the map entry on
  four paths: the `s.cfg.Image == ""` early 201 (which is every handler-level
  test), the delete-in-flight early return in the boot goroutine, and
  `provisionRepo`'s `ctx.Done()` and plain-`git init` returns. A leaked entry
  pins the project at "provisioning" for the process's lifetime. Specify a
  single `defer` that owns the deletion, and who sets the entry (one handler,
  two 201 sites — the plan says "the create handlers", there is one).
- **[medium / confidence: high]** item N state precedence — the plan shows
  "provisioning" only when the container state is *not* `absent`, but the
  container is absent for the first part of the window (`EnsureRunning` has not
  created it yet), which is exactly the window the item exists to cover; and
  `deleting` is computed in the same place (`server.go:906-912`, `:686-692`).
  Decide the precedence order (deleting > provisioning > live status) and
  whether absent+provisioning renders the chip.
- **[medium / confidence: high]** `internal/httpapi/git.go:77-88` +
  `internal/httpapi/setup.go:226-231` (item G) — the plan says parse `[user]`
  `name`/`email` "with the same line scan `toolDetails` uses", but that scan
  ignores sections entirely and matches the literal prefixes `"name = "` /
  `"email = "`. `git config --file` accepts `name=x`, leading tabs,
  case-insensitive section and key names, quoted/escaped values, and a `[user]`
  section reopened later. `gitIdentity` feeds the commit author
  (`internal/httpapi/git.go:328`) and the credential harvest
  (`internal/httpapi/credharvest.go:52`), so a stricter parser silently drops a
  user's identity from every commit. Decide the accepted grammar explicitly,
  and say whether `toolDetails`'s git card moves to the same parser (it would
  change what that card shows). Side note: `git config --file` does not follow
  includes by default, so the "behaviour difference accepted" may be no
  difference — check before writing the comment.
- **[medium / confidence: high]** `internal/orchestrator/orchestrator.go:1109-1114,1209-1213,1144-1156`
  (item E) — both callers hold the per-project lock, so the new "two inspects
  2 s apart, fresh 10 s budget" holds that lock for up to ~12 s on the
  stop-failure path, blocking `EnsureRunning`, terminal attach and the network
  toggle for that project. Decide whether that is acceptable or whether the
  second sample happens outside the lock (which changes the signature the plan
  says is unchanged).
- **[medium / confidence: medium]** `internal/httpapi/check.go:404-412`,
  `internal/httpapi/format.go:~120` (item K4) — "`.styrman-tmp/` added to the
  workspace's git excludes by the same script (`.git/info/exclude`,
  idempotent)" is unspecified in the ways that matter: the script is a guest
  shell fragment that runs on every debounced check *and* format, so two
  concurrent appends can interleave; `.git` may be a file (worktree); and the
  script runs in `~/projects/<dir>`, not necessarily a repo root. Decide
  whether the exclude is written once server-side at provisioning instead.
- **[medium / confidence: high]** item K8 offers two mutually exclusive designs
  in one sentence (a constant mirrored in `api.ts` with "a Go test that the two
  lists agree", vs. `checkable: [...]` added to `GET /api/settings/tools`).
  The endpoint is `toolDetails` (`internal/httpapi/server.go:422` →
  `internal/httpapi/setup.go:207`), a credential/roster payload the editor does
  not fetch today, so option 2 implies a new fetch or store; option 1's
  cross-language "test that the lists agree" cannot exist without parsing TS.
  Pick one.
- **[low / confidence: high]** item C — `reply`'s zero value is `status: 0`;
  if the closure ever returns without setting it (a `modeRuntime` fake that
  does not invoke `fn`), `writeJSON(w, 0, nil)` panics in `WriteHeader`.
  Specify a fail-closed default (500 "the mode was NOT changed"). Also name
  that `decide` stays a closure — it needs `ctx`, `sp`, `u`, `p`, `on`,
  `persist`.
- **[low / confidence: high]** item O — 11 branches key on
  `errdefs.IsNotFound` and three sites manufacture `errdefs.NotFound(...)`
  (`internal/orchestrator/orchestrator.go:150,289,308,330,362,696,707`,
  `internal/orchestrator/ports.go:19`). The fake's "scriptable per-method
  error" must produce errors that satisfy `errdefs.IsNotFound`, or the planted
  cases exercise the wrong branch. Also pin the exact `*client.Client`
  signatures (docker v28: `ImageInspect` is variadic) — the plan's sketch omits
  them for 17 of the 19 methods. Method counts in the sketch are slightly stale
  (`ContainerRemove` 11 not 10, `ContainerStart` 4 not 3); harmless, but the
  interface must be generated from a fresh grep.
- **[low / confidence: high]** `deploy/update/styrman-update:560` (item K3) —
  the checkout failure already does `log "checkout ${ref} failed"; return 1`;
  there is no `fail` inside `deploy()`. Like K2, this looks already-closed;
  verify and strike rather than budget an edit.
- **[low / confidence: medium]** item 5 — `publicKeyLine`'s only call site
  reads `.ssh/id_ed25519.pub` (`internal/httpapi/setup.go:243-246`), so the new
  type enum is exercised for ed25519 only, and `web/src/lib/sshKey.ts`'s
  `PUBLIC_KEY` already requires a full type token. State what the server-side
  enum buys (defence in depth for other API consumers) so a reviewer does not
  read it as duplicated work.
- **[low / confidence: high]** item B — the plan's node script must round-trip
  the NUL byte the root `.gitattributes` documents in
  `web/dist/client/_shell.html`; `.agents/rules/writing-files.md` (BUG-007)
  requires `git diff --numstat` on that path after the change. `web/scripts/`
  does not exist yet (new directory, fine). The `u:1789030752690` token is
  really there — confirmed.
- **[low / confidence: medium]** process — items C and E are the 4th and 5th
  rounds on mechanisms that produced BUG-028/029/030/031/032. `AGENTS.md`
  ("when a SECOND round of findings lands on the same mechanism … stop fixing
  and ask what the mechanism is for") already fired here. The plan should
  record in one line that the operator's "go all" is that answer, so the next
  reviewer does not reopen it.

## Readiness — questions a first-pass implementer must ask

1. Item 29: how is a `deleteError` induced end-to-end, given it is never
   persisted?
2. Item I: what happens in `AuthMode=none` — skip the ticker, or trust
   `identity.User.Authorized`?
3. Item I: `NewBridge` has six parameters and one caller
   (`internal/terminal/bridge.go:55`, `internal/httpapi/server.go:212`); does
   `authz` become a seventh, and how does the email reach `serve` (and does
   `/ws/setup` re-check too)?
4. Item J1: which host(s) stay trusted for post-login redirects — `ShellDomain`
   only, or also `AppsDomain`/`PrivateDomain` wildcards? Can the directive
   repeat?
5. Item N: where exactly is the map entry deleted so no early return leaks it,
   and does the chip show while the container is still `absent`?
6. Item G: what INI grammar is accepted, and does the settings git card switch
   to it?
7. Item E: is a ~12 s hold of the per-project lock on the stop-failure path
   acceptable?
8. Item K4: who writes `.git/info/exclude`, and how is it safe against two
   concurrent check/format runs?
9. Item K8: mirrored constant or `checkable` in the API reply?
10. Item K5: does the display label change alone satisfy "the UI must show
    tofu", given `tool` from `STYRMAN_TOOL` is both the parser key and the
    displayed string (`internal/httpapi/check.go:376-399`)?
11. Item C: what status does a closure that set nothing produce?
12. Item O: what error type does the fake return for "absent", and is the
    interface method list generated from a fresh grep at implementation time?
13. Item L: `ensure-test-image.sh` lands under `.agents/docs/testing/styrman/`
    — is an executable script in the docs tree intended (integration.sh is
    already there, so probably yes), and what does it do when `docker build`
    fails mid-suite?
14. Item F: the release notes and `deploy/remote.md` wording for a host that
    still holds a legacy tree — who writes it, and in which phase (Phase 1's
    box does not mention docs)?

## Criteria

1. **Dead code** — pass, and the plan is on the right side of it: F deletes
   `migrateWorkspaces`/`LegacyWorkspaceExists` and their two tests
   (`cmd/styrman/migrate_test.go:169,230`; four other tests in that file
   survive), K6 trims a trap glob the script never creates, K1 deletes an inert
   `Persistent=true`. The BUG-005 deletion-count discipline is named.
2. **Flow tracing** — fail, and it is where every high finding sits: N's map
   has four un-traced exits, I's ticker was not traced into `auth-none`, 29's
   `deleteError` was not traced to its owner, E's new sleep was not traced up to
   its two lock-holding callers.
3. **Standards conformance** — mostly pass; `.agents/rules/writing-files.md`
   (append to `setup_pubkey_test.go`, explain deletion counts) and
   `guest-writable-paths.md` (G routed through `files.ReadRaw`, O keeps the
   `realbelow` planting pattern) are respected. The `AGENTS.md` "second round on
   the same mechanism" rule is unaddressed (low finding above). `sync.Map` is
   written with type parameters in the N sketch; stdlib `sync.Map` is not
   generic — cosmetic in a plan, but the implementer will notice.
4. **Directory layout** — pass. New files land where their neighbours are:
   `internal/orchestrator/docker.go` (does not exist yet — no BUG-005 hazard),
   `fakedocker_test.go` beside it, `web/scripts/normalize-shell.mjs`,
   `.agents/docs/testing/styrman/ensure-test-image.sh` beside `integration.sh`.
5. **Simplicity (DIY-except-security)** — pass with one caution: nothing
   security-sensitive is hand-rolled except the `.gitconfig` parser (G), which
   replaces `git config --file` with a hand scan; that is a correctness risk,
   not a crypto/auth one, and it is called out above. O's `dockerAPI` is a
   19-method interface with one production implementation, justified by the
   tests it unlocks (BUG-002/003 guard), and it is sequenced last.
6. **Decoupling** — pass. O follows the existing `guestRuntime`/`modeRuntime`
   seam convention; C keeps `modeRuntime` unchanged. The one new coupling is
   K8's proposal to hang `checkable` off the settings-tools payload, which ties
   the editor's lint gating to a credentials endpoint — see the finding.

## Not examined

- `internal/files` internals beyond the signatures of `Trust`, `ReadRaw`,
  `ReadRawNoFollow`, `ContainedPath`, `Exists`.
- `web/src/components/{CodeEditor,EditorPane,Sidebar}.tsx` and
  `web/e2e/run.mjs` were not opened; items H, K7, K9 and the e2e mechanics of
  N/29 are assessed from the plan and recon only (except the close-code
  constants, which live in `web/src/lib/api.ts:883-885` and
  `web/src/components/TerminalPane.tsx`, not the `terminalSocket.ts` the plan
  guesses at).
- `deploy/host-setup.sh`'s daemon.json merge (M(d)), ADR-067 itself, and the
  firewall script/tests: only `styrman-firewall.service` (no `Before=`, as the
  plan says) and `.timer` (`OnBootSec=15s`) were read. M(f)'s measurement plan
  is not assessable without running it.
- Items A and L were taken from the recon; the divergence list was not re-run.

## Response (main, 2026-09-10, round 1)

Every gap and question is answered IN THE PLAN (revised in place), not
here — the plan is the artifact the implementer reads. Summary of the
decisions: 29 demoted to a pure `projectBadge` + vitest (no e2e can induce
`deleteError`); I re-checks only in header mode (nil func in `none`), email
captured at upgrade, both WS handlers, 4003 mapped in `TerminalPane.tsx`;
J emits one trust line per authenticated domain (shell exact, apps and
private wildcards), struck if the directive cannot repeat; N sets the map
entry once before the boot goroutine whose first statement is the `defer`
delete, precedence deleting > provisioning > live, chip shows while absent;
G's grammar is stated (case-insensitive sections/keys, `=` spacing, quoted
values, comments, last wins, reopened sections) and the git card uses the
same parser; E takes its second sample OUTSIDE the lock in a bounded
goroutine with `CompareAndDelete` on the marker's stop-time value; K2 and
K3 verified already closed (struck); K4 temps go to `~/.styrman/tmp`; K5
adds a parser case if the shape differs; K8 is one design with an e2e
drift guard; C initialises the reply fail-closed; O's fake answers absent
with `errdefs.NotFound`; B round-trips bytes; L's build failure is FATAL;
F's docs land in Phase 1's box. The process note about C/E and "go all"
is in the plan.

---

# Round 2 — cold re-read of the revised plan   (2026-09-10)

Range reviewed: `.agents/docs/plans/2026-09-10-todo-closeout.md` (revised, working
tree — the file is not yet committed) plus the code surfaces it names. Same
cold-reader check as round 1, re-answering (1) gaps still undecided and (2)
questions a first-pass implementer would still have to ask.

## Verdict: REQUEST CHANGES — `design needs revision`, `implementation not ready`

Eleven of the thirteen round-1 gaps are closed against the code. The one that is
not is **J1**, and it is worse than round 1 read it: the revision is now
*specific enough to check*, and checking it shows the narrowing is a **no-op in
this deployment**. `AppsDomain` defaults to `BaseDomain`
(`deploy/caddy/ingress/cmd.go:136`; `deploy/.env.example:74` leaves `APPS_DOMAIN`
unset — "unused by default"), and `PREVIEW_DOMAIN` is
`public.${STYRMAN_HOSTNAME}.${BASE_DOMAIN}` (`deploy/.env.example:37`), i.e. a
suffix-match of `BaseDomain`. So "one line per trusted domain, omitting empties"
emits `trust login redirect uri domain suffix <BaseDomain>` — byte-identical to
`generator.go:264` today — and the public-preview wildcard, the one host the item
exists to untrust, stays trusted. The plan does not say what to emit in that
case. Secondly, the plan calls `ShellDomain` "the SPA": it is not. `ShellDomain`
is used only for an app whose container hostname is the literal `shell`
(`generator.go:376-379`, `config.go:111`), and styrman is discovered as
`caddy_hostname: '${STYRMAN_HOSTNAME:-styrman}'`
(`deploy/docker-compose.yml:125`) — an app under `AppsDomain`. A trusted list
built from `ShellDomain` + `PrivateDomain` alone would lock the operator out of
the SPA the moment `APPS_DOMAIN` is set to anything narrower.

## Round-1 items re-checked against the code — CLOSED

- **29 → `projectBadge` vitest.** Sound. `deleteError` is in-memory only
  (`internal/httpapi/server.go:139-149,1047-1052`), so the demotion is the honest
  call, and the `lan.ts`/`lan.test.ts` pattern it cites exists
  (`web/src/lib/lan.ts`, `web/src/lib/lan.test.ts`). One residual, below.
- **I — nil in `AuthMode=none`, seventh `NewBridge` parameter.** Both hold.
  `NewBridge` takes exactly six parameters today (`internal/terminal/bridge.go:55-61`)
  and has one call site (`internal/httpapi/server.go:212`), where `s.users`
  (`server.go:113`) is in scope, so `cfg.AuthMode == "header"` → `s.users.IsAuthorized`,
  else nil is a two-line change. `AuthMode` is exactly `"none"`/`"header"`
  (`internal/config/config.go:194`), and `IsAuthorized("")` is false
  (`internal/userstore/userstore.go:117-120`), so skipping in `none` is required,
  not optional. `identity.FromContext` (`internal/identity/identity.go:58`) is
  reachable in both handlers. Close code 4003 fits the existing convention on
  both sides (`internal/config/config.go:58-66`, `web/src/lib/api.ts:883-885`).
- **N — single `defer`, `deleting > provisioning > live`.** Holds. There is one
  create handler (`internal/httpapi/server.go:759`) with two 201 sites
  (`:841`, `:893`), and the `s.cfg.Image == ""` site at `:841` spawns no
  goroutine, so "set immediately before the `go func()`" leaks nothing there. A
  `defer s.provisioning.Delete(p.ID)` registered first in the goroutine covers
  the delete-in-flight return (`:857-860`), `bootFailed` (`:871`), and
  `provisionRepo`'s returns, and still runs after the existing recover defer on
  a panic. `provisioning` while the container is `absent` is right: the list
  computes `state = "absent"` unless `Adopt` knows the container
  (`:684-687`), which is exactly the boot window.
- **G — stated grammar.** The grammar as written (case-insensitive `[user]`,
  case-insensitive keys, `=` with any whitespace, optional double quotes with
  `\"`/`\\`, `#`/`;` comments outside quotes, last-wins) is decidable and strictly
  wider than what it replaces (`internal/httpapi/setup.go:226-231` matches the
  literal prefixes `"name = "`/`"email = "`). The include question is safely
  deferred: `git config --file` defaults to `--no-includes`, so the measured
  difference is expected to be none — the plan's "state the measured difference,
  if any" is the right instruction.
- **E — async two-sample + `CompareAndDelete` on a `time.Time` marker.** Both
  halves check out. `sync.Map.CompareAndDelete` exists since Go 1.20 and the
  module is `go 1.25.0` (`go.mod:3`, toolchain `go1.25.0`). `time.Time` is a
  comparable struct, so `CompareAndDelete` cannot panic on it, and the monotonic
  reading is *carried inside* the compared fields and preserved by plain copies
  through `any` — no `Round(0)`, no marshalling on this path — so the goroutine's
  captured marker compares equal to the stored one and unequal to any later
  `time.Now()`. The only way to alias two markers is two `Store`s in the same
  nanosecond with a failed stop between them; not credible. Both `Store` sites
  (`internal/orchestrator/orchestrator.go:1121`, `:1234`) and both
  `unmarkIfStillRunning` call sites (`:1130`, `:1239`) must change together, and
  the plan says so. Nothing reads the marker's value — `Stopped` is presence-only
  (`:1161-1164`) — so `true` → `time.Time` is safe. Moving the sleep off the
  per-project lock removes round 1's ~12 s hold.
- **K4 — temp dir outside the workspace.** Works. Both call sites set
  `HOME=/home/<guest>` (`internal/httpapi/check.go:360`,
  `internal/httpapi/format.go:94`), so `mktemp -p "$HOME/.styrman/tmp"` expands,
  and the diagnostic parsers are path-agnostic: `reGCC` is
  `^[^:]*:(\d+):(\d+):…` (`check.go:130`), and an absolute POSIX path contains no
  colon, so an out-of-tree temp file still parses. This also removes round 1's
  concurrent-append hazard entirely, since the `.git/info/exclude` write is now
  only a contingency.
- **K8 — one design.** Now singular (`CHECKABLE_EXTENSIONS` in `api.ts` +
  an e2e drift probe), and the drift guard is implementable: the check endpoint
  answers `{"supported":…,"tool":…}` (`check.go:396-399`) and answers
  `unavailable:true` *with* the tool name when the binary is missing
  (`:385-390`), so the probe does not depend on the test image carrying every
  linter.
- **C — fail-closed default.** Correct and sufficient: `reply{status: 500, …}`
  before `WithProject` removes the `writeJSON(w, 0, nil)` panic round 1 found,
  the seam (`internal/httpapi/server.go:87`) is unchanged, and the fake it wants
  to assert against already exists (`internal/httpapi/network_test.go:290`).
- **O — `errdefs.NotFound` fake answers.** Correct: the package in use is
  `github.com/docker/docker/errdefs` (`internal/orchestrator/orchestrator.go:26`),
  whose `NotFound(err)` constructor the production code itself already calls
  (`:289`, `:308`, `:330`, `:987`), so a fake built the same way lands on the
  eleven `IsNotFound` branches.
- **K3/K2 strikes** — verified: the checkout failure is `log …; return 1`
  (`deploy/update/styrman-update:560`); strike with the note is right.

## Findings — gaps still undecided

- **[high / confidence: high]** plan §J(1) (`.../2026-09-10-todo-closeout.md:143-153`)
  vs. `deploy/caddy/ingress/cmd.go:136` + `deploy/.env.example:37,74` — with
  `APPS_DOMAIN` unset, `AppsDomain == BaseDomain`, so the per-domain lines
  reduce to today's single `BaseDomain` line (`generator.go:264`) and
  `*.public.styrman.<base>` remains a trusted redirect target. Decide what the
  generator emits when `AppsDomain` equals or contains `PreviewDomain` —
  enumerate the actual app FQDNs instead of a wildcard suffix, require
  `APPS_DOMAIN` to be set narrower, or strike the item as unachievable with a
  suffix matcher. As written the item ships a diff and changes nothing.
- **[high / confidence: high]** plan §J(1):144 ("`ShellDomain` (the SPA)") vs.
  `deploy/caddy/ingress/generator.go:376-379`, `config.go:111`,
  `deploy/docker-compose.yml:125` — the SPA's FQDN is
  `<STYRMAN_HOSTNAME>.<AppsDomain>`, not `ShellDomain`; `ShellDomain` is reached
  only by an app whose hostname is literally `shell`, which this deployment does
  not run. The trusted set must be derived from the discovered app hostnames (or
  from `AppsDomain` explicitly), or a narrowed list breaks login for the shell
  itself. Fix the premise before the generator test's "exact lines" are written.
- **[medium / confidence: high]** plan §J(1):148-151 ("reading the vendored
  go-authcrunch parser at implementation") vs. `deploy/caddy/ingress/go.mod:5-8`
  and `deploy/caddy/Dockerfile:13` — there is no vendored copy:
  `github.com/greenpau/caddy-security@v1.1.64` is not a dependency of the
  ingress module (only `caddy/v2` and `docker` are) and is pulled by `xcaddy`
  inside the builder image; the module cache on this host has no `greenpau/`
  entry. The strike-if-not-repeatable rule is a good decision rule but names a
  source that does not exist. Say where the parser is read (`go mod download
  github.com/greenpau/caddy-security@v1.1.64` into a scratch module, or `docker
  run` the builder stage), or the precondition silently becomes "guess".
- **[medium / confidence: high]** plan §N:213-216 vs. §Verification:310-312 —
  the two disagree and neither yields a failing assertion. §N says the e2e
  asserts "the list shows `provisioning` or `running`", which passes today,
  before the feature exists; §Verification promises the golden path shows
  "provisioning then running in the sidebar". The existing failed-clone check
  already polls `GET /api/projects` every 2 s for up to 120 s across a real
  microVM boot (`web/e2e/run.mjs:1299-1312`), so a deterministic assertion
  ("at least one poll observed `state === 'provisioning'` before the terminal
  state") is available — decide that, or drop the e2e claim. Related: no Go test
  is named for the new precedence, and handler tests cannot reach it through the
  create path (`cfg.Image == ""` sets no entry, `server.go:836-842`), so the
  precedence must be tested by seeding `s.provisioning` in-package. Changed
  behaviour with no covering test is a DoD failure.
- **[low / confidence: high]** plan §29:220-225 vs.
  `web/src/components/Sidebar.tsx:268-289` — the Sidebar does not render one
  badge: `deleting`, `deleteError` and `provisionError` chips are three
  independent conditionals and `<LanBadge>` renders alongside them whenever the
  project is not deleting. A pure `projectBadge(p)` returning a single value
  therefore has an unstated boundary: does it subsume `LanBadge` (changing what
  the sidebar shows), and does it return one badge or a list? Decide, or the
  "precedence" the vitest asserts will not be the precedence the UI has.
- **[low / confidence: medium]** plan §G:104-119 vs.
  `internal/httpapi/setup.go:224` — the stated grammar omits two cases the real
  parser must answer: `[user "sub"]` (git reads that as `user.sub.name`, NOT
  `user.name` — a parser that treats it as `[user]` would put the wrong name in
  every commit) and trailing whitespace on an unquoted value. Also unstated:
  whether `toolInfo.Ready = strings.Contains(gitcfg, "email")` moves to the new
  parser along with `Details`. Name the two cases in the grammar and in the
  tests.
- **[low / confidence: high]** plan Phase 6:348 — item I adds close code 4003 to
  a protocol documented in `AGENTS.md` ("close 4000 = exited, 4001 = replaced")
  and in `specs/styrman/api.md`; the documentation box names only api.md's
  `provisioning`. Add 4003 (and the `AGENTS.md` protocol line) to that box, or
  the DoD docs step will miss it.

## Readiness — questions a first-pass implementer would still have to ask

1. **J1:** what does the generator emit when `AppsDomain == BaseDomain` (the
   production default), given that a `BaseDomain` suffix re-trusts the public
   preview wildcard the item is meant to exclude?
2. **J1:** which host stands for the SPA — `ShellDomain` (unused here) or the
   discovered `<STYRMAN_HOSTNAME>.<AppsDomain>`?
3. **J1:** where is go-authcrunch v1.1.64's `trust login redirect uri` parser
   read from, since it is not a dependency of the ingress module?
4. **N:** what does the e2e assert such that it fails if `provisioning` is never
   rendered — and which Go test covers `deleting > provisioning > live`, given
   the create handler never populates the map in handler tests?
5. **29:** does `projectBadge` own the LAN badge too, and does it return one
   badge or an ordered list?
6. **G:** is `[user "sub"]` in or out, and does the settings card's `Ready`
   switch to the new parser?
7. **K4:** if a checker turns out to need the temp file inside the workspace,
   the plan falls back to `ensureRepos` writing `.git/info/exclude` — but item F
   is editing `ensureRepos` in the same phase-1 window; which lands first?

## Criteria (round 2)

1. **Dead code** — pass, unchanged from round 1; F/K1/K6 still delete rather
   than comment out, and `git config --file`'s exec disappears with G.
2. **Flow tracing** — much improved: N, I and E now trace to their real exits
   (verified above at `server.go:836-893`, `bridge.go:55-61`,
   `orchestrator.go:1121-1156,1234-1240`). The remaining untraced hop is J1's,
   where the plan's domain model does not match the generator's
   (`generator.go:376`) or the deployment's (`docker-compose.yml:125`).
3. **Standards conformance** — pass. `sync.Map` usage, the `errdefs` seam and
   the close-code constants follow the existing conventions; BUG-005/007
   disciplines are named in B and F.
4. **Directory layout** — pass, unchanged; `web/src/lib/projectBadge.ts` sits
   beside `lan.ts` as the existing `*.test.ts` pattern expects.
5. **Simplicity (DIY-except-security)** — pass with the standing caution on G's
   hand-rolled INI scan (correctness, not crypto). J1 touches an auth surface
   but only by configuration of a vetted plugin; nothing is hand-rolled.
6. **Decoupling** — pass. K8's second design (hanging `checkable` off the
   credentials payload) is gone, which removes round 1's only new coupling.

## Not examined (round 2)

- `deploy/host-setup.sh`'s daemon.json merge (M(d)) and M(f)'s netns
  measurement — unchanged from round 1, still not assessable on paper.
- Items A and L were not re-verified beyond round 1's reading; the divergence
  list was not re-run.
- `web/e2e/run.mjs` was read only around the failed-clone check
  (`:1275-1330`); the K8 drift-probe insertion point was not located.
- `internal/files.ReadRaw`'s signature was confirmed only from its call site
  (`internal/httpapi/setup.go:217`), not from its definition.
- The revised plan is uncommitted, so no `git diff` of round 1 → round 2 was
  available; the comparison is against round 1's quoted text.

## Response (main, 2026-09-10, round 2)

- J1 (both highs) — STRUCK in the plan with your reason: `APPS_DOMAIN`
  unset makes `AppsDomain == BaseDomain` and the shell is an app under it,
  so no trust list can keep the shell and exclude the preview wildcard. The
  fix is a domain-layout choice (`APPS_DOMAIN` sub-zone), recorded as an
  operator option in `deploy/remote.md` in Phase 3. J2 stays.
- N (medium) — the transient e2e assertion is gone. Precedence is a pure
  `deriveState` with a unit test for every combination, plus a handler test
  that sets `s.provisioning` directly and asserts GET/list; the chip is
  covered by `projectBadge`'s vitest. The E2E criteria section no longer
  claims to observe the window.

---

# Round 3 — cold re-read of the revised plan   (2026-09-10)

Range reviewed: `.agents/docs/plans/2026-09-10-todo-closeout.md` (revised,
working tree — still uncommitted, so no round-2 → round-3 `git diff` was
available) plus the code surfaces it names. Same cold-reader check,
re-answering (1) gaps still undecided and (2) questions a first-pass
implementer would still have to ask.

## Verdict: REQUEST CHANGES — `design needs revision`, `implementation not ready`

The J1 STRIKE is the right call and its stated reason checks out against the
code. What does not check out is the compensating deliverable the strike buys:
Phase 3 charters a `deploy/remote.md` sentence saying the operator can narrow
the trusted redirect zone by setting `APPS_DOMAIN` to a sub-zone, "after which
the existing single line already excludes the preview wildcard". The existing
single line is `trust login redirect uri domain suffix {cfg.BaseDomain}`
(`deploy/caddy/ingress/generator.go:264`) — the ONLY such line in the repo,
hard-coded to `BaseDomain` and never reading `AppsDomain`. Setting
`APPS_DOMAIN` changes app FQDNs (`generator.go:376-379`) and nothing about the
trust line, and the preview/private wildcards are defined UNDER `BASE_DOMAIN`
by construction (`deploy/.env.example:37,42`), so no value of `APPS_DOMAIN`
narrows anything. The plan is about to publish operator-facing guidance about
an auth surface that has zero effect. Everything else from round 2 is closed;
the residue is J2's two unstated mechanics and three one-line decisions.

## Round-2 items re-checked against the code — CLOSED

- **J1 strike premise.** Verified: `AppsDomain` defaults to `BaseDomain`
  (`deploy/caddy/ingress/cmd.go:136`, `deploy/.env.example:74`), and the shell
  IS an app under `AppsDomain` — `writeAppBlock` uses `ShellDomain` only when
  `app.Hostname == ShellHostname` (`generator.go:373-379`) and this deployment
  discovers styrman as `caddy_hostname: '${STYRMAN_HOSTNAME:-styrman}'`
  (`deploy/docker-compose.yml:125`). Both round-2 highs are answered by
  striking, and the round-2 medium (where go-authcrunch's parser would be read)
  disappears with the item. The strike text in the plan states the reason
  correctly.
- **N verification.** Sound and implementable. `testServer(t)` returns
  `*Server` from an in-package test (`internal/httpapi/provision_test.go:25`),
  so a handler test can seed `s.provisioning` directly without any new seam,
  and `deriveState(deleting, provisioning bool, live string)` is a pure
  function whose combinations are exhaustively testable. Dropping the transient
  e2e assertion is the honest call — the rewritten E2E criteria section
  (plan:311-317) no longer claims to observe the window, and the failed-clone
  e2e check keeps its `provisionError` assertion.

## Findings — gaps still undecided

- **[high / confidence: high]** plan §J(1):141-149 + Phase 3:339 vs.
  `deploy/caddy/ingress/generator.go:264` — the trust line is
  `fmt.Fprintf(b, "            trust login redirect uri domain suffix %s path prefix /\n", cfg.BaseDomain)`;
  it is the only occurrence in the repo (`grep -rn 'trust login redirect'
  deploy/caddy/`) and it never reads `AppsDomain`. The planned `deploy/remote.md`
  sentence therefore documents a knob that does nothing: an operator who sets
  `APPS_DOMAIN=apps.<base>` still trusts `*.<BaseDomain>`, preview wildcard
  included. Narrowing without touching the generator would require moving
  `BASE_DOMAIN` itself, which also moves the cookie domain (`generator.go:263`)
  and every default app FQDN. Decide one: (a) strike with "not achievable by
  configuration; a generator change is required, recorded and not done", and
  write THAT in `deploy/remote.md`; or (b) keep the doc sentence but state the
  full change it requires (new `BASE_DOMAIN` + DNS + cert + cookie-domain
  consequences). As written, Phase 3 ships a false statement about an auth
  surface.
- **[medium / confidence: high]** plan §J(2):150-158 vs.
  `deploy/caddy/ingress/generator_test.go:195-198` and the module layout — "the
  generator's test helper (or a tiny `go run ./deploy/caddy/ingress/cmd/gen` if
  none exists — prefer reusing the test's fixture writer)" names a helper that
  does not exist: the only `os.WriteFile` in the generator tests writes the
  `Caddyfile.base` INPUT, and every test asserts on the returned string; nothing
  writes a generated Caddyfile to disk. There is also no `package main` anywhere
  in the ingress module (`grep -rn '^package main' deploy/caddy/ingress/` is
  empty), so option two is a NEW main package inside the module that xcaddy
  builds from `/src/caddy-ingress` (`deploy/caddy/Dockerfile:10-16`). Decide the
  producer before the phase — an env-gated `TestWriteCaddyfileFixture`, or the
  new `cmd/gen` — because the choice adds code to the release build path.
- **[medium / confidence: medium]** plan §J(2):154-157 vs.
  `deploy/caddy/ingress/generator.go:202,255,256,275,342` — the generated
  Caddyfile references `{env.CLOUDFLARE_API_TOKEN}`, `{env.GOOGLE_CLIENT_ID}`,
  `{env.GOOGLE_CLIENT_SECRET}` and `{env.STYRMAN_JWT_KEY}`, and `caddy validate`
  adapts AND provisions modules. The planned `docker run --rm -v … caddy
  validate` passes no environment, so caddy-security provisions an OAuth
  provider with an empty client id/secret and the ACME DNS module an empty
  token: the gate step either fails for a reason unrelated to the Caddyfile
  syntax it is meant to check, or passes only because the failure is tolerated.
  Name the dummy `-e` values (and, if provisioning still refuses, say that
  `caddy adapt --config … && caddy validate` is the fallback) in the step
  itself, not at implementation time.
- **[low / confidence: high]** plan Phase 6:352 — unchanged from round 2: the
  documentation box still names only api.md's `provisioning`. Item I adds close
  code **4003**, and the WS close-code protocol is documented in
  `.agents/docs/specs/styrman/api.md:86` (which documents 4002) and in
  `AGENTS.md` ("close 4000 = exited, 4001 = replaced"). Add 4003 to that box or
  the DoD docs step will miss a protocol change.
- **[low / confidence: medium]** plan §G:104-119 vs.
  `internal/httpapi/setup.go:225` — two round-2 points remain unwritten. (a)
  `[user "sub"]`: the stated grammar's literal reading ("any other section is
  skipped") happens to give git's answer, but the named test list ("the six
  grammar cases above plus a missing file") has no case for it, so the one
  sub-case that would silently put a wrong author on every commit is untested.
  (b) `git.Ready = strings.Contains(gitcfg, "email")` is not said to move to
  the parser, so the settings card can report git ready off a `[github] email =`
  line while `gitIdentity` returns empty. State both in one sentence each.
- **[low / confidence: high]** plan §29:219-226 and §N:212-214 vs.
  `web/src/components/Sidebar.tsx:268-289` — still unstated whether
  `projectBadge(p)` returns ONE badge or a list, and whether it subsumes
  `<LanBadge project={p} />`, which today renders alongside the
  deleting/deleteError/provisionError chips whenever the project is not
  deleting. A vitest asserting a single-value "precedence" over a UI that
  renders two badges concurrently asserts a precedence the UI does not have.
  One line settles it.

## Readiness — questions a first-pass implementer would still have to ask

1. **J1:** what exactly does the `deploy/remote.md` sentence say, given
   `APPS_DOMAIN` cannot affect `generator.go:264`?
2. **J2:** what writes the Caddyfile the validate step consumes — an env-gated
   test, or a new `cmd/gen` main inside the module xcaddy builds?
3. **J2:** which environment variables does the `docker run … caddy validate`
   invocation set, so that provisioning the OAuth and ACME-DNS modules is not
   what fails?
4. **29/N:** does `projectBadge` own `LanBadge` too, and does it return one
   badge or an ordered list?
5. **G:** is `[user "sub"]` a named test case, and does `toolInfo.Ready` move to
   the new parser?

Questions 4 and 5 are one-line decisions an implementer could make alone;
1–3 are not — 1 produces operator documentation, 2 adds code to the release
build path, and 3 decides whether a release-gate step means anything.

## Criteria (round 3)

1. **Dead code** — pass, unchanged: F/K1/K6 delete rather than comment out, and
   G removes the `git config --file` exec (`internal/httpapi/git.go:79-86`).
2. **Flow tracing** — pass for the Go items (N's map exits, I's ticker, E's
   marker were traced in round 2 and the plan text still matches the code). The
   remaining untraced hop is J1's compensating doc, which was not traced to the
   only line that implements the behaviour it describes.
3. **Standards conformance** — pass; `.agents/rules/writing-files.md`
   disciplines (B's `--numstat`, F's deletion counts, D appending to
   `setup_pubkey_test.go`) are still named.
4. **Directory layout** — pass, with the J2 caveat above: a new `cmd/gen`
   package would be the first main in a module that exists to be a Caddy
   plugin.
5. **Simplicity (DIY-except-security)** — pass. J1's strike REMOVES a change to
   an auth surface rather than adding one, which is the right direction; the
   standing caution on G's hand-rolled INI scan (correctness, not crypto)
   remains.
6. **Decoupling** — pass, unchanged from round 2.

## Not examined (round 3)

- `deploy/host-setup.sh`'s daemon.json merge (M(d)) and M(f)'s netns
  measurement — unchanged from rounds 1-2, still not assessable on paper.
- Items A, L, K1-K9 (except K4/K5/K8 as previously read), B, D, 5, 7, H and O
  were not re-verified in this round; round 2 closed them and their plan text
  is unchanged.
- `caddy validate`'s provisioning behaviour with empty `{env.…}` placeholders
  was reasoned from the generated directives, not executed — hence medium
  confidence on that finding.
- The plan is still uncommitted; the comparison is against round 2's quoted
  text, not a diff.

## Response (main, 2026-09-10, round 3) — gate closed by the operator

Three rounds; every finding left in round 3 was item J. Per the feature
workflow's three-round cap the question went to the operator, who chose
"Strike both": J1 (redirect-trust narrowing, infeasible under this
deployment's domain layout — your round-2/3 analysis is the recorded
reason) and J2 (`caddy validate` in the gate: needs a generator entry
point and dummy secrets, adds an image build per gate run). The plan's J
section and Phase 3 box are rewritten to say so; the `deploy/remote.md`
"APPS_DOMAIN" sentence you showed to be a no-op is gone. The three lows
(G subsections + `Ready`; `projectBadge` scope vs `LanBadge`; 4003 in the
docs box) are folded into the plan. Implementation starts from this
revision.
