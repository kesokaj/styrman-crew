---
owner: reviewer
date: 2026-09-13
---
# Cold-reader check, round 2 — plan `2026-09-13-audit-fixes.md` (v1.30.0)   (2026-09-13)

Range reviewed: the plan file `.agents/docs/plans/2026-09-13-audit-fixes.md`
(working-tree revision, ` M` against HEAD `7066301`), the round-1 cold-read
`.agents/docs/reviews/2026-09-13-coldread-audit-fixes.md`, and the code
surfaces the plan names. Nothing was built, run, tested or deployed; every
claim below is read from the file:line cited.

## Verdict: design needs revision / implementation not ready

Round 1's 16 gaps and 20 questions are all *answered* in the plan — I checked
each one against the plan's own words (table at the end). Two of the answers
describe code motions that **cannot be written as described**, and both are on
the critical path. H1's new orchestrator change says
`recreateMissingNetwork` removes the container "so `ensureRunningLocked` falls
through to the create path" — the call sits at `orchestrator.go:353`, *inside*
`case err == nil:` of the switch opened at `:344`, and a Go switch does not
re-dispatch on a reassigned `err`; the literal implementation leaves the
project with no container and a `waitReady` on a removed id. M8's
`__verify-tag` verb is placed "right after the arg loop", where the arg loop
(`styrman-update:60-66`) has already `exit 2`'d on it, `log()`/`fail()`
(`:68-69`) are not yet defined, and `REQUIRE_SIGNED`/`ALLOWED_SIGNERS`
(`:261-289`) are unbound under `set -u`. Below those, M11's test harness does
not exist and its check must be the *first* statement rather than merely
"before ensure", and three smaller decisions (M10's directive validation, M4's
zero-value WARN, M3's prettier call site) are unstated but cheap.

---

## 1. Gaps — what must still be decided

### G1 — H1: the fall-through the plan describes is unreachable  (high / confidence: high)
`internal/orchestrator/orchestrator.go:344` opens `switch { case err == nil:
… case errdefs.IsNotFound(err): … }`. `recreateMissingNetwork` is called at
`:353`, inside the `err == nil` arm, after a failed `ContainerStart`. Setting
`err = errdefs.NotFound(fmt.Errorf("recreated"))` there cannot enter the
create arm: the switch expression is evaluated once, and the arm has already
been chosen. Control leaves the case, falls past `:361` to `:442`, and calls
`waitReady(ctx, id, 90*time.Second)` with `id = insp.ID` (`:346`) — the id of
the container that was just removed. Result: the start fails with "did not
become ready", the project has **no** container, and it is only rebuilt on
some later `EnsureRunning` call.

The three existing recreate paths (`:280-290` workspace bind, `:299-310` image
upgrade, `:319-332` ADR-067 mode change) all sit **before** the switch for
exactly this reason — that is the only place `err = errdefs.NotFound(…)`
works.

**Decide, and write the shape into the plan:**
(a) a fourth pre-switch block — `if err == nil && (insp.State == nil ||
!insp.State.Running) { if the network is absent → ContainerRemove; err =
errdefs.NotFound(…) }`. Note this **changes the trigger** from "the start
failed" to "the network is missing, probed unconditionally", i.e. one extra
`NetworkInspect` per start of a stopped container — cheap, and
`bridgeModeMismatch` at `:319` already inspects the same network, so the two
can share `currentBridge`'s result (`:903-912`); or
(b) keep the post-failure trigger and restructure — extract the
`IsNotFound` arm's body (`:362-428`) into a method the `err == nil` arm can
call, or wrap the switch in a one-shot retry.

**The create path itself is fine** (the dispatch's second question): `create`
builds the missing network under the correct bridge name from `spec.lan`
(`:981-993` → `createBridgeNetwork`, `:891-897`), injects the *current*
broker address (`:1018-1020`), and the `IsNotFound` arm supplies everything
create needs — `ws` (`:366`), the workspace bind (`:412-413`), the project
home (`:373-404`) and `lan` (`:408`). Reachability is the only problem.

Second-order, also undecided: if `recreateMissingNetwork` stops creating the
network and connecting the container, then `createBridgeNetwork`'s comment
"Both call sites (create, recreateMissingNetwork)" (`:887-890`) and
`bridgeModeMismatch`'s "recreateMissingNetwork owns that case" (`:939-942`)
are stale, and `NetworkConnect` (`:1480`) and `invalidateSelfAttach`
(`:1486`) move to `create`. Say whether the function keeps building the
network (create then sees it and skips) or hands that to create too.

### G2 — H1: the ADR's stated reason is false, and the plan wants it enshrined  (medium / confidence: medium-high)
The plan argues no other path can give a container a new subnet without a new
env because "Docker itself refuses to remove a network with an attached
container". Docker refuses only while an endpoint is **active**; a *stopped*
container's endpoint is gone, which is precisely how the network disappears
under a stopped container in the first place — the scenario the code at
`:350-352` exists for ("an older buggy sweep removed it"). The invariant worth
recording is the one the fix creates: *every path that can change a project's
network recreates the container, so the env is re-injected*. Recording the
Docker-refuses claim instead would enshrine a false premise in ADR-069 —
exactly what round 1's G1 warned about.

### G3 — M8: the verb is placed where it cannot run, and the hoist list is short  (high / confidence: high)
The plan: "`styrman-update __verify-tag <tag> <allowed_signers> <repo>` handled
right after the arg loop and before the token/fetch block, which requires
hoisting `GIT_CRED_ARGS`, the `git()` wrapper and `verify_tag()` above that
point." Reading `deploy/update/styrman-update`:

- **Unreachable.** The arg loop is `:60-66`; its default arm is
  `*) echo "unknown flag: ${arg}" >&2; exit 2`. `__verify-tag` is not a flag,
  so the script exits 2 before anything "after the arg loop" runs. The verb
  must be dispatched *before* the loop, or the loop must learn to pass `__*`
  through (styrman-ctl's precedent dispatches verbs in `main`, `:948-958`).
- **`log`/`fail` are not hoisted.** They are defined at `:68-69`, after the
  arg loop. `verify_tag` calls `log` on every branch (`:298`, `:316-319`,
  `:324-331`, `:335`, `:338`). The plan's hoist list omits them.
- **Root and STATE_DIR.** `[ "$(id -u)" -eq 0 ] || fail "must run as root"`
  (`:77`) and the `STYRMAN_UPDATE_STATE_DIR` guards (`:82-104`) run before the
  token block. If the verb is dispatched after them the new Go test must run
  as root with a scratch state dir — i.e. it will `t.Skip` on every normal
  `go test ./...` and never actually run. Decide: dispatch the verb between
  `:69` and `:77`.
- **`REQUIRE_SIGNED` / `ALLOWED_SIGNERS` are unbound there** (the dispatch's
  question). They are assigned at `:261-273` and `:289`, *after* the token
  block; with `set -euo pipefail` (`:27`), `verify_tag`'s
  `[ "${REQUIRE_SIGNED}" = "1" ]` (`:295`) aborts the verb with "unbound
  variable". So the verb must set `REQUIRE_SIGNED=1` and
  `ALLOWED_SIGNERS="$2"` itself. Say so — and say the consequence: the verb
  exercises `verify_tag` with the policy read bypassed, so the
  `STYRMAN_UPDATE_REQUIRE_SIGNED=0` opt-out and the update.env
  ownership/mode guards (`:262-272`) remain untested by this batch.
- **`git()` pins `DEPLOY_DIR`.** `git() { command git -c
  safe.directory="${DEPLOY_DIR}" "${GIT_CRED_ARGS[@]}" "$@"; }` (`:229`) with
  `DEPLOY_DIR` from `:29`. The verb takes `<repo>`; decide whether it sets
  `DEPLOY_DIR="$3"` and `cd`s there, or passes `-C`. (The hoist of
  `GIT_CRED_ARGS=(…)` at `:215` **is** correct: the token block only appends
  with `+=` at `:228`, and the wrapper expands the array at call time.)
- **Gating.** styrman-ctl's `__` hooks are gated on `STYRMAN_CTL_TEST=1` plus
  a TMPDIR containment check (`deploy/styrman-ctl:879-889`, "security
  review"). `__verify-tag` is read-only but runs `git` **as root** against an
  operator-supplied repo and reads an operator-supplied allowed_signers path.
  Decide whether it carries the same gate; today the plan adds an ungated
  internal verb to a root-run script.
- **GNUPGHOME cleanup.** "a fresh empty temp dir (removed after)" has no
  removal point: `verify_tag` returns 1 from inside the `if ! git … verify-tag`
  branch (`:334-337`), so the obvious
  `g=$(mktemp -d); GNUPGHOME="$g" git …; rm -rf "$g"` leaks the dir on the
  refusal path. Specify a trap or an explicit cleanup on both exits.

### G4 — M11: "before ensure" is too weak, and the cited harness does not exist  (medium / confidence: high)
`internal/terminal` has **no** test that constructs a `Bridge` —
`autostart_test.go`, `close_test.go`, `reauth_test.go` and `setuptool_test.go`
are all pure-function tests — so "the bridge tests' fake ensure records calls"
describes a harness that must be written from scratch. That matters because
`Bridge.orch` is a concrete `*orchestrator.Orchestrator` (`bridge.go:37`), not
an interface, and the terminal entry point touches `b.storeFor` (`:119`),
`store.Get` (`:136`) and `b.orch.Stopped` (`:156`) **before** the `b.ensure`
at `:166`. If the Origin check merely lands "before ensure", the 403 test
needs a real orchestrator and a populated terminal store; if it is the
**first statement** of the handler, the test can use a zero-value `Bridge`
with a recording `ensure` and a nil `orch`. Decide: first statement, both
handlers.

Naming, so the implementer does not hunt: the project entry point is
`(*Bridge).ServeHTTP` (`bridge.go:114`), registered as
`mux.Handle("GET /ws/term/{project}/{term}", s.bridge)`
(`internal/httpapi/server.go:389`) — there is no `ServeTerminal`, and the path
is `/ws/term/…`, not the plan's `/ws/terminal/…`. The move direction is sound:
`httpapi` already imports `terminal` (`server.go:244`), so
`terminal.SameOriginOrAbsent` called from `httpapi/browser.go:345` creates no
cycle, and the helper (`browser.go:327-334`) depends only on `net/url`,
`strings` and `*http.Request`.

### G5 — M10: a new Caddyfile directive is asserted only as a string  (medium / confidence: medium)
M10 adds `cookie samesite lax` to the portal block
(`deploy/caddy/ingress/generator.go:260-275`, beside the existing
`cookie domain` at `:263`) and its stated acceptance is "Generator test asserts
both lines". A generator test proves the bytes are emitted, not that
caddy-security v1.1.64 (`deploy/caddy/Dockerfile:13`) *accepts* the directive;
an unsupported directive fails at Caddy config load, i.e. the ingress does not
come up after the deploy. The project already has the right check for exactly
this class of change — `.agents/docs/testing/2026-09-04-verify-session-lifetime.md:45`
runs `docker run styrman-caddy:<v> caddy validate --config /etc/caddy/Caddyfile
--adapter caddyfile` against the real generated file. Decide: add that as
M10's acceptance (and it covers the `domain suffix .<BASE>` line too).

### G6 — M4: the new WARN fires on an absent field  (low / confidence: high)
`internal/config/config.go:297` is `if o.DockerDiskGB >= 5 && o.DockerDiskGB
<= 500 { … }`. An overrides file carrying only `idleTimeout` unmarshals
`DockerDiskGB` as 0, so the obvious `else { WARN }` prints an
"out of range: 0" line on every boot for a perfectly valid file. State that
the WARN skips the zero value (the "keep" sentinel `SetOperator` uses at
`:326`).

### G7 — M3: two small placement facts the Where column omits  (low / confidence: high)
- `--ignore-path /dev/null` belongs in `formatInPlace`'s prettier arm
  (`internal/httpapi/format.go:47`), not in `formatScript` (`:119`) — the
  plan's Where column names only `:119`.
- The comment fix is two-sided: `format.go:84` **already** claims "Temp file
  in the workspace (WorkDir=ws) so formatters discover repo config" while
  `formatScript`'s first line (`:120`) says "Temp file outside the workspace,
  as in checkScript". They contradict each other today; both change.

Everything else in M3 checks out against the code: the exec already sets
`WorkDir: ws` (`format.go:95`) so `$PWD` needs no new parameter;
`ExecSpec.Env` (`internal/orchestrator/exec.go:19`) is passed to the exec
create as a list (`exec.go:161,244`), never through the `bash -c` string, so
the env-passing form is confined as claimed; and `listFiles`
(`internal/httpapi/explorer.go:53-61`) is a single handler serving both the
project and home routes (`server.go:429,439`), so one filter covers both.

---

## 2. Readiness — questions a first-pass implementer must ask

**BLOCKING**
1. H1: which restructuring makes the create path reachable — a fourth
   pre-switch block (trigger becomes "network absent", probed on every start
   of a stopped container) or an extraction/retry of the `IsNotFound` arm?
   (G1)
2. H1: does `recreateMissingNetwork` still create the network and keep its
   name, or does it become "remove the container and signal recreate", with
   `createBridgeNetwork`'s and `bridgeModeMismatch`'s comments updated? (G1)
3. M8: where exactly is `__verify-tag` dispatched, given the arg loop exits 2
   on it and `log`/`fail` are defined after it? (G3)
4. M8: does the verb set `REQUIRE_SIGNED=1` and `ALLOWED_SIGNERS="$2"`
   itself (it must, under `set -u`), and is the test therefore accepted as
   not covering the update.env policy read? (G3)
5. M8: is the verb gated like styrman-ctl's `__` hooks
   (`STYRMAN_CTL_TEST=1` + TMPDIR containment), or ungated? (G3)
6. M11: is the Origin check the FIRST statement of `ServeHTTP`/`ServeSetup`
   (so the test needs no orchestrator), or merely somewhere before `ensure`?
   (G4)

**NON-BLOCKING**
7. H1: what does the ADR record as the reason — the Docker-refuses claim, or
   "every network change recreates the container"? (G2)
8. M8: where is the GNUPGHOME temp dir removed on the refusal path? (G3)
9. M8: does the verb `cd` into `<repo>` and reset `DEPLOY_DIR` for the
   `safe.directory` pin? (G3)
10. M10: is `caddy validate` against the generated Caddyfile added as
    acceptance, or is the string assertion accepted? (G5)
11. M4: does the persisted-value WARN skip `DockerDiskGB == 0`? (G6)
12. M3: `--ignore-path /dev/null` goes in `formatInPlace` — confirm the other
    formatters' arms are untouched. (G7)
13. M3: `body.Path` for a HOME-scope file is posted to the workspace-scoped
    `/api/projects/{project}/format` route today
    (`web/src/components/FileEditor.tsx:238` ignores `scope.kind`;
    `server.go:379` registers only the project route). With M3 that path now
    picks a *directory*: a home-relative dir either does not exist under
    `$PWD` (falls back, fine) or collides with a same-named workspace dir
    (temp file in the wrong place, still confined). Confirm the fallback is
    the intended behaviour and that the 400 rule cannot reject a shape that
    formats successfully today.
14. H1: with two probes, the marker rule needs an order — `/env` succeeding
    then `/files` failing must leave the marker SET. "removed on the next
    success" is ambiguous for two calls.
15. H1: `styrman-credentials.sh:35,39` return early when `curl` or `jq` is
    absent, before any probe — no marker written, no stale marker cleared.
    Accepted (both are in the image), or does the missing-tool case get a
    marker too?
16. M3: the `.git/info/exclude` append is gated on `$PWD/.git` being a
    *directory*; in a git worktree or submodule `.git` is a file, so the
    pattern is never excluded there. Accepted?
17. M9: `ARG GO_VERSION=1.27.1` drops the `go` prefix that today's
    `image/Dockerfile:88-90` carries in `${GO_VERSION}` (it reads
    `.[0].version`, e.g. `go1.27.1`, and interpolates it straight into the
    tarball URL) — the URL becomes `go${GO_VERSION}.linux-amd64.tar.gz`.
    Mechanical, but worth one word so the sha does not get blamed.

**Answered without needing to ask** — the uuid signature change's test impact
(the dispatch's last item): `NewUUID` has no test of its own and no test
callers (`internal/project/` holds only `store_test.go` and
`store_lan_test.go`; `grep NewUUID --include=*_test.go` is empty), so
`NewUUID() (string, error)` compiles everywhere after the two production call
sites (`store.go:100`, `store.go:155`) are updated — no existing test changes.
The change also deletes `uuid.go:13`'s `fallback-%d` id, which is the one
shape `uuidRe` (`store.go:106,137`) would have quarantined on the next load,
so the two new tests the plan implies (Add → 500 with nothing persisted;
migration → `.rejected` and continue) are the whole of the new coverage.

---

## 3. Round-1 gaps and questions — decided or not

All 16 gaps are addressed in the plan's own words; three of the answers are
unimplementable or incomplete as written (marked ✗).

| R1 | Plan's decision (quoting) | |
|---|---|---|
| G1 H1 orchestrator | "it changes to remove that (stopped) container and return 'recreated', so `ensureRunningLocked` falls through to the create path" | ✗ G1 |
| G2 marker | empty broker → "remove any stale marker and return"; "did not answer = curl exit non-zero, for `/env` and for `/files` alike"; marker holds "`<ISO time> <url>`, written after a non-fatal `mkdir -p`"; MOTD "a SEPARATE `if [ -f marker ]` block … inside the existing `[ -t 1 ]` gate (accepted)" | ✓ (Q14/15) |
| G3 sweep | "RESCOPED, never repointed: `find "$D" -maxdepth 1 -name '.styrman-fmt-*' -mmin +60 -delete`" | ✓ |
| G4 mktemp/$WS | "the exec already runs with `WorkDir: ws`, so the script uses `$PWD`"; `mktemp -p "$D" .styrman-fmt-XXXXXX --suffix=.ext` | ✓ |
| G5 per-dir config / ignore files | "the temp file sits BESIDE the edited file"; "`prettier --write --ignore-path /dev/null "$F"`" | ✓ (G7) |
| G6 visibility / reversal | `.git/info/exclude` append + `listFiles` filter; "This deliberately REVERSES the 2026-09-01 review low …" | ✓ (Q16) |
| G7 M8 sourceability | "it gains an INTERNAL verb like `styrman-ctl`'s `__` hooks" | ✗ G3 |
| G8 M8 prereqs | "`t.Skip`s when `ssh-keygen` or a git without `gpg.format=ssh` is absent" | ✓ |
| G9 M11 helper/scope | "the helper MOVES: `terminal.SameOriginOrAbsent(r)` … BOTH `ServeTerminal` and `ServeSetup` refuse 403 before `ensure`/`EnsureSetup`" | ✗ G4 |
| G10 uuid 2nd caller | "the load-time migration (`store.go:100`) quarantines the record into `.rejected` like its siblings and continues" | ✓ |
| G11 M4 applyOperator | "`Load`: … fatal at boot"; "(`:297`) already IGNORES … gains a WARN line" | ✓ (G6) |
| G12 M10 refuter | "CONFIRMED by the refuter"; apex "is not a served site"; explicit `:port` no longer matches | ✓ (G5) |
| G13 M6 untested branch | "recorded here and in the changelog as an accepted FAIL line for the verifier" | ✓ |
| G14 Antigravity | "DECISION — no pin … a build-time pin would be theatre"; ADR-069 records what floats | ✓ |
| G15 `--ignore-scripts` | "WITHOUT `--ignore-scripts` … Acceptance: the existing `claude --version && agy --version`" (present at `image/Dockerfile:99`) | ✓ |
| G16 release ordering | "the release ships with H1 verified by unit test and image build only … the proof is taken and recorded in the handoff right after the deploy" | ✓ |

Round-1 questions 1-20: all answered by the text above — 1 (H2) by
"`checkFile` itself combines `AbortSignal.any([signal, AbortSignal.timeout(20000)])`",
which I re-audited: `web/src/lib/api.ts` passes an explicit signal at exactly
four places (`:506` 210 s, `:518`/`:523` 150 s, `:657` 120 s) plus
`checkFile` (`:621-625`), whose only caller is `FileEditor.tsx:192` with a bare
`AbortController`. The caller audit in the plan is complete and correct.
Questions 4, 5, 6 are re-opened by G3/G4 above; the rest stand answered.

---

## Criteria (applied to the plan as the artifact)

1. **Dead code** — pass. M5 (`ContainedPath` at `internal/files/files.go:180`
   plus `files_test.go:147-168`, and I confirmed no other caller),
   `AuthCookieName`, `ExternalLinkIcon`, `BrowserPane.attached`, the orphan
   JSDoc: all scheduled for deletion.
2. **Flow tracing** — fail, on two hops: H1's switch arm (G1) and M8's script
   prologue (G3); M11's pre-ensure ordering (G4) is a third, softer one. H2's
   and uuid's traces, wrong in round 1, are now correct.
3. **Standards conformance** — pass with one note: the new root-run internal
   verb has no gate decision (G3), against the precedent the same repo set for
   `styrman-ctl`'s hooks.
4. **Directory layout** — pass. New tests sit beside their subjects or follow
   the existing script-test convention (`internal/config/styrman_ctl_test.go`,
   `internal/httpapi/browser_launcher_test.go`).
5. **Simplicity (DIY-except-security)** — pass. No new dependency; M8 hardens
   an existing signature gate, M9 pins rather than vendors, M10 tightens a
   redirect allowlist in the vetted plugin's own config.
6. **Decoupling** — pass now that the Origin helper moves with the dependency
   edge (`httpapi` → `terminal`), not against it.

## Not examined

- Nothing was built, run, tested, linted or deployed: no `go test`, no vitest,
  no `docker build`, no shellcheck, no `caddy validate`, no integration/e2e.
- `cookie samesite lax` could not be checked against caddy-security v1.1.64 —
  the module is not in this machine's module cache; hence G5 is about the
  missing verification, not a claim that the directive is wrong.
- M9's published sums (Go 1.27.1, Node 22.23.2, the three base-image digests)
  were not verified against the publishers.
- M1/M2 (`SidePanelGit.tsx:429,528`), M12 (`api.md` rewrite), M13's handler
  tests, and every item in the Low — *do* list except uuid, `ContainedPath`,
  `files.Read` and the config bound.
- `deploy/host-setup.sh:372`, `deploy/styrman-ctl:646`, the ingress lows
  (Cookie strip, `path.Clean`, `sanitizeHostname`) and the web lows.
