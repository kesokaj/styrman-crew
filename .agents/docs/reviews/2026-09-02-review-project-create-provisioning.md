# Review — project-create provisioning (attach-before-clone, provisionError, guestHostname)   (2026-09-02)

Scope: `503fc8a..working-tree` — uncommitted working-tree diff vs HEAD (503fc8a), limited to
`git diff HEAD -- internal web/src` (diff sha1 `9808ea71d6a7`). `web/dist/**` (build output),
the `.claude/agent-memory/**` deletions, and the already-committed `.agents/docs/**` are out of scope.

> The working tree MOVED TWICE during this review: `a1e4e0eac525` → `5932f2f7ce9b`
> (a refactor extracted `provisionRepo`/`provisionSpec`, added `redactSecrets`, added
> `internal/httpapi/provision_test.go`) → `9808ea71d6a7` (the probe pacing moved from package
> vars to `Server.provisionDelay` / `Server.brokerProbeInterval`). All findings and line numbers
> below are against `9808ea71d6a7`. **Re-check the hash before acting on this report.**

## Verdict: REQUEST CHANGES

The root-cause fix is correct and, after the refactor, genuinely well tested — moving
`EnsureSelfAttached` ahead of provisioning and gating on an in-guest broker probe is the right
diagnosis of the right bug, and `TestProvisionRepoAttachesBeforeAnyCredentialedCommand` pins the
ordering so it cannot regress silently. The worst problem is that the change, whose entire purpose
is "provisioning failures must stop being silent", still leaves one silent: when the microVM fails
to boot on a `github: private|public` create, nothing is recorded and the user gets a project with
no GitHub repo and no message (`server.go:884` guards the new `setProvisionError` behind
`req.CloneURL != ""`). Close behind it, `guestErrMessage` re-introduces a bug a previous audit
already fixed 570 lines below it — `clampTitle` (`server.go:1292`) strips the same control bytes and
walks back to a rune boundary with `utf8.RuneStart` "(audit nit)", while the new copy slices
`msg[:300]` blind. Then: `waitForBroker` has no "a broker cannot exist here" guard, so every
clone/GitHub create in the documented host-mode dev workflow burns ~30–35 s of connect timeouts
before failing exactly as it would have failed instantly; and `redactSecrets` leaves `redactURL`
behind as a redundant *and* lossy second redactor whose doc comment is now orphaned onto an
unrelated `var`. None of these are hard to fix and none undermine the core change — but the first
reintroduces the class of bug this diff exists to kill, so it should not ship as-is.

## Findings

- **[medium / confidence: high]** `internal/httpapi/server.go:884` — a `github: "private"|"public"`
  create whose microVM fails to boot records **no** `provisionError`. The branch reads
  `if req.CloneURL != "" { s.setProvisionError(…, "clone skipped: the project VM did not start"); … }`,
  so the GitHub path falls through to a bare `logger.Error` and returns; the user sees a project
  that quietly has no GitHub repo — the exact failure mode this change exists to eliminate. Fix:
  hoist the message out of the `CloneURL` guard (`if req.CloneURL != "" || req.GitHub == "private" ||
  req.GitHub == "public"`, or reuse `provisionSpec.wantsGitHub()`), reworded "repo setup skipped:
  the project VM did not start"; keep `gitrepo.EnsureRepo` under the `CloneURL` guard where it
  belongs.

- **[medium / confidence: high]** `internal/httpapi/server.go:713-722` (`guestErrMessage`) —
  duplicates an existing sanitizer and drops the fix that sanitizer already carries.
  `clampTitle` at `server.go:1292` has the byte-identical `strings.Map` control filter and then
  bounds its length with an explicit rune-boundary walk (`for cut > 0 && !utf8.RuneStart(t[cut])
  { cut-- } // never split a multi-byte rune (audit nit)`). `guestErrMessage` copies the filter and
  then does `msg = msg[:300] + "…"`, which can split a multi-byte rune — `strings.Map` normalizes the
  guest bytes to valid UTF-8 first, so the cut is exactly where an invalid fragment gets produced,
  and `encoding/json` then coerces it to U+FFFD. The two functions also disagree on the control
  set: neither strips Unicode C1 (U+0080–U+009F) or the bidi/format controls (U+202E, U+2066–U+2069,
  U+2028/U+2029), and here the text is fully guest-controlled and lands in the sidebar `title`
  tooltip (`Sidebar.tsx:280-287`). Not XSS — React sets `title` as a property and escapes the text
  node — but a project owner can be shown a deliberately reversed or line-broken message. Fix:
  extract one `clampGuestText(s string, max int) string` carrying the `utf8.RuneStart` walk plus
  `unicode.Cf` and the C1 range, and call it from both `clampTitle` and `guestErrMessage`.

- **[medium / confidence: high]** `internal/httpapi/server.go:675-687` (`waitForBroker`) — no guard
  for "a broker cannot exist on this deployment". `cmd/styrman/main.go:104` starts the broker only
  when `cfg.BrokerAddr != "" && cfg.SelfContainer != ""`, and `EnsureSelfAttached` is a no-op when
  `SelfContainer == ""`. In host-mode dev the guest's profile script still *synthesizes*
  `STYRMAN_CRED_BROKER` from its own IP (`image/configs/styrman-credentials.sh:25-31`), so the probe
  addresses a host that is not there and cannot become there. Cost per attempt: the `/env` curl +
  the `/files` curl (both run by the login shell, `--connect-timeout 1` each) + the `/healthz` curl
  (`--connect-timeout 1`) + `brokerProbeInterval` 500 ms ≈ 3.5 s; × `brokerProbeAttempts = 10` ≈
  **30–35 s added to every credentialed create, every time, in local dev**, with a guaranteed failure
  at the end. Fix: `if s.cfg.SelfContainer == "" || s.cfg.BrokerAddr == "" { return false }` at the
  top of `waitForBroker` (or skip the wait in `provisionRepo`), logging once at debug that the broker
  is disabled.

- **[medium / confidence: high]** `internal/httpapi/server.go:774-795` — `redactURL` is now both
  redundant and buggy, and its doc comment has been orphaned. `redactSecrets` subsumes it at the one
  remaining call site (`server.go:622`, `redactURL(ps.cloneURL)`), and `redactURL`'s positional scan
  mangles any URL with `@` outside userinfo — `cloneURLRe` explicitly admits `@` in the path, so
  `https://github.com/o/r@v1.git` logs as `https://***@v1.git`, destroying the host and the path
  while redacting nothing. Separately, the comment block now reads `// redactURL strips any
  userinfo … (R10)` immediately followed by `// redactSecrets strips …`, and the whole block attaches
  to `var userinfoRe` (line 781), leaving `func redactURL` (line 787) with no doc comment at all — a
  godoc defect the `golang` skill's "doc comment begins with the identifier" rule catches. Fix:
  delete `redactURL` and `TestRedactURL`, call `redactSecrets(ps.cloneURL)` at line 622, and let the
  surviving comment sit on `userinfoRe`/`redactSecrets` where it belongs.

- **[medium / confidence: medium]** `internal/httpapi/server.go:583` + `:603-611` + `:912`
  (`provisionSpec.attach func()`) — the attach discards `EnsureSelfAttached`'s `fresh` return, and
  the comment justifying the pause is false at this placement. `orchestrator.go:469-477` states the
  contract: "Returns true when a fresh attach happened … so callers on a request path should
  redirect the client to retry rather than proceed", and `proxy.go:76` honours it. For a brand-new
  project the attach is *always* fresh. The comment claims the delay is "the same guard startProject
  uses" — but in `startProject` (`server.go:1024`) the sleep runs before the response flushes; here
  it runs *after* `EnsureRunning`, which holds the per-project lock through a ≤90 s `waitReady`, so
  the 201 flushed long ago. What `provisionDelay` actually does is fire the attach ≈1 s after the
  boot lock releases — precisely when the client's `POST /terminals` (blocked on that same lock via
  `ensureFor` → `EnsureRunning`) has just opened its terminal WebSocket. `TerminalPane` treats 1006
  as transient and reconnects with a 2 s backoff (`TerminalPane.tsx:216-221`), so this degrades
  rather than breaks — but it is a self-inflicted blip against a "< 5 s to agent prompt" SLA. Fix:
  either perform the self-attach inside `EnsureRunning` after `create()` and before `waitReady`
  (while the boot lock still blocks every client path), or type `attach` as `func() bool` and at
  minimum log when a fresh attach lands after the lock has already been released. Confidence is
  medium because I could not observe a WS drop; the severity rests entirely on the codebase's own
  repeated assertion that a fresh attach kills in-flight connections.

- **[low / confidence: high]** `internal/httpapi/server.go:554-563` — the `if msg == ""` clearing
  branch of `setProvisionError` is unreachable from production code: nothing ever calls it with an
  empty string. `TestProvisionErrorLifecycle` asserts a lifecycle the server never performs, and its
  comment — "an empty message clears the entry so a project never keeps a stale badge" — is not true
  of the shipped system. Concretely: a project that recorded "clone skipped: the project VM did not
  start" keeps the red badge forever even after the user starts the VM and clones by hand; the only
  way to clear it is to delete the project or restart the server. Fix: either call
  `setProvisionError(user, id, "")` on the success paths in `provisionRepo` (and on a successful
  `startProject`), or delete the branch and the test case. Do not leave a branch whose only caller
  is its own test.

- **[low / confidence: medium]** `internal/httpapi/server.go:603-660` — R13's window widened with no
  UI signal. For a `cloneUrl` create the workspace is deliberately *not* git-initialized before the
  201 (`server.go:848`), so it stays an empty non-git directory until provisioning finishes; the new
  broker wait adds up to ~35 s to that window, and the terminal is reachable throughout it (the boot
  lock releases before `provisionRepo` starts). If the user or an agent writes anything into
  `~/projects/<dir>` during the window, `git clone -- <url> .` then fails with "destination path '.'
  already exists and is not an empty directory" and the project falls back to an empty repo. The
  project also reports plain `state: "running"` the whole time — there is no `provisioning` state to
  mirror the existing `deleting` one. Fix: add a `provisioning` in-flight flag alongside
  `provisionErrs` (same map key, same mutex, same `projectView` treatment) so the sidebar can say
  "setting up the repo…"; optionally clone into a temp dir and move, or `git init` + `remote add` +
  `fetch`, so a non-empty workspace is not fatal.

- **[low / confidence: high]** `internal/httpapi/provision_test.go` — the file has no
  `internal/httpapi/provision.go`. `provisionRepo`, `provisionSpec`, `waitForBroker`,
  `guestErrMessage`, `setProvisionError`/`provisionError`, and the `brokerProbe*` constants were all
  added to `server.go`, now 1308 lines and the package's largest file by a wide margin — a god-file
  already carrying routing, project CRUD, the delete state machine, redaction helpers, and
  `clampTitle`, which two engineers touching adjacent features cannot edit without colliding. The
  tests correctly recognize provisioning as its own concern; the source should too. Fix: move the
  provisioning block into `internal/httpapi/provision.go` so the test file sits beside the code it
  covers (review-standards criterion 4).

- **[low / confidence: high]** `internal/httpapi/provision_test.go:23-37` — `testServer` is the sixth
  ad-hoc server constructor in this package's tests (`server_test.go:31`,
  `explorer_files_test.go:260`, `credentials_test.go:38`, `shared_link_test.go:46`,
  `isolation_test.go:39` all build the same `New(cfg, users, orch, logger)` by hand with slightly
  different config). Fix: promote one shared helper returning both `*Server` and its `http.Handler`,
  and delete the duplicates rather than adding a seventh next time.

- **[low / confidence: medium]** `internal/httpapi/provision_test.go` (`TestProvisionErrorSurfacesInTheProjectAPI`,
  the DELETE at the end) — the test issues a real `DELETE /api/projects/{id}`, whose background
  goroutine reaches `orchestrator.Destroy` (`orchestrator.go:1027`) and calls
  `docker.ContainerRemove` with no `cfg.Image == ""` escape hatch. `createProject` has exactly that
  guard for exactly this reason ("it would otherwise reach the host's real Docker daemon and race
  test cleanup with its writes", `server.go:854-856`). The test passes either way because its
  assertion only reads the map cleared synchronously in the handler, but it makes a unit test depend
  on the host's Docker daemon. Fix: assert the clearing through `setProvisionError` + a direct
  handler call, or extend the `cfg.Image == ""` guard to the delete path.

- **[low / confidence: high]** `internal/httpapi/provision_test.go:44,55-64` — `recorder.reply`'s
  `n int` parameter is computed on every call (the loop over `r.steps`) and read by no caller. Dead
  parameter and dead work. Fix: drop `n` from the signature.

- **[low / confidence: high]** `web/src/components/NewProjectDialog.tsx:20,37,106-109` —
  `nameEdited` latches permanently on the first keystroke, including one the user reverts. Two
  reachable consequences: (a) type anything in the name field, clear it, then switch to
  "Clone a repo" and paste a URL — the auto-name never engages again, the helper text claims
  "The repo is cloned into the project workspace.", and Create stays disabled over an empty field;
  (b) with `nameEdited === false`, switching *away* from "Clone a repo" sets `derivedName = ""` and
  the visible name silently reverts to `""` — the field the user was looking at empties itself.
  Neither can submit a name the user did not see: `effectiveName` drives both the input `value` and
  the `api.createProject` argument, which I traced. A derived name colliding with an existing project
  surfaces as the server's 409 in the dialog's error line, which is acceptable. Fix: clear
  `nameEdited` when the field is emptied (`setNameEdited(e.target.value !== "")`) and keep the last
  derived name in `name` when leaving clone mode.

- **[low / confidence: high]** `web/src/lib/api.ts:210-219` (`projectNameFromRepoURL`) — I could not
  construct an input that yields a name passing the client `nameValid` gate but failing the server's
  `^[a-z][a-z0-9-]{0,30}$`: the chain guarantees `[a-z0-9-]` only, a leading letter, no trailing
  dash, and ≤31 chars, in that order, and the final `.replace(/-+$/, "")` after `.slice(0,31)` closes
  the one gap that ordering could leave. The rough edge is that a half-typed URL produces a confident
  wrong answer — `https://github.com` derives `github-com` and `https://gitlab.com/group/` derives
  `group` — and `cloneValid` accepts both, so a user can create a project named after a hostname.
  Fix (optional): derive only when the URL has at least two path segments after the host.

## Criteria

1. **Dead code — fail.** Three items, all "delete", with evidence: `redactURL` is redundant with
   `redactSecrets` at its one surviving call site and is lossy on `@`-in-path URLs;
   `setProvisionError`'s empty-message branch has no production caller, only its own test;
   `recorder.reply`'s `n` parameter is computed and never read. No commented-out blocks and no
   orphaned files were introduced, and `gitrepo.EnsureRepo`, `guestErrMessage`, and `guestHostname`
   are all live.
2. **Flow tracing — fail (one gap).** The happy path traces cleanly end to end:
   `createProject` → goroutine → `EnsureRunning` → `provisionRepo(attach, waitForBroker, run)` →
   `setProvisionError` → `provisionErrs` → `listProjects`/`getProject` → `projectView.ProvisionError`
   → `Project.provisionError` (TS) → `Sidebar` badge. Keys are consistently `user+"/"+projectID`, the
   requester's own username is always the lookup key, and cross-user isolation is pinned by
   `TestProvisionRepoRecordsCloneFailure`. `deletingMu` is never held across another lock, so there
   is no ordering hazard, and `provisionErrs` is bounded by the project count (cleared on delete),
   not unbounded. Contexts propagate: the attach delay and the probe interval are both cancellable
   `select`s on `ctx.Done()`. The gap is the boot-failure branch — the `github` path exits without
   recording anything (finding 1) — plus the deliberately discarded `fresh` signal (finding 5).
3. **Standards conformance — fail (minor).** `gofmt -l internal/` clean, `go vet ./...` clean,
   `go test ./internal/httpapi ./internal/orchestrator` green, `tsc -b` and `oxlint` clean, all
   re-run against the current tree. One Go doc-comment defect remains: `redactURL` lost its doc
   comment to `var userinfoRe`. (The earlier "Vars, not consts" comment error on the `brokerProbe*`
   const block was corrected in the `9808ea71d6a7` revision — noted so it is not re-reported.)
   The `bash -lc` switch is safe on PATH grounds: Debian's `/etc/profile` resets `PATH`, but `git`,
   `gh`, and `curl` all live in `/usr/bin`; `image/skel/profile` only sources `~/.bashrc`, which
   returns immediately for non-interactive shells (`image/skel/bashrc:7`); the MOTD is `[ -t 1 ]`
   guarded; and nothing in the login path `cd`s out of the exec's `WorkingDir`. `shellQuote` +
   `cloneURLRe` + `ValidateName` leave no injection: the clone URL is single-quoted after `--`,
   `p.Name` matches `^[a-z][a-z0-9-]{0,30}$`, and the `--private|--public` flag is reachable only
   through `wantsGitHub()`.
4. **Directory layout — fail (minor).** `internal/httpapi/provision_test.go` has no
   `internal/httpapi/provision.go`; the code it covers was appended to a 1308-line `server.go`.
   Everything else lands correctly: `internal/` for private code, the frontend fully contained in
   `web/src/`, `guestHostname` beside `EnsureRunning` in the orchestrator, tests beside their
   packages.
5. **Simplicity (DIY-except-security) — pass.** No new dependencies; stdlib only (`regexp`,
   `strings`, `time`, `context`). No hand-rolled auth, crypto, password hashing, session handling,
   or JWT/OIDC — credentials keep flowing through the existing broker (ADR-030/032) and `gh`/`git`'s
   own helpers, which is exactly right, and the change's whole thrust is to make that vetted path
   actually reachable. `provisionSpec`'s injected `attach`/`run` is the rare abstraction that pays
   for itself: it is what makes the ordering — the actual bug — testable without a Docker daemon,
   and `TestProvisionRepoAttachesBeforeAnyCredentialedCommand` is the best thing in this diff. The
   two overlapping redactors and the two copies of the control-byte filter are the only complexity
   that buys nothing.
6. **Decoupling — pass with a caveat.** `provisionRepo` is a clean, deep seam: one call, two
   injected collaborators, no reach-through into orchestrator internals. The caveat is its host
   file — living in `server.go` serializes the provisioning concern against routing, CRUD, and the
   delete state machine (criterion 4). The frontend seam is clean: `projectNameFromRepoURL` is a
   pure exported function in `lib/api.ts`, and the `Sidebar` badge reads one optional field.
   `guestHostname` is correctly scoped, and I verified by grep across `image/`,
   `deploy/caddy/ingress/`, and the Go tree that nothing depends on the old `styrman-` *hostname*
   prefix: the only consumers are `PS1`'s `\h` (`image/skel/bashrc:44`) and the `host` field in
   `stats.go`. Container naming (`styrman-<uuid>`) and the ingress's
   `strings.HasPrefix(containerName, "styrman-")` reserved-hostname check
   (`deploy/caddy/ingress/labels.go:71`) both key off the container *name*, not the guest hostname,
   and are unaffected. Existing containers keep the old hostname until recreated, which
   `.agents/docs/specs/styrman/runtime-image.md:255-268` already documents.

## Notes for the trail (not findings against this diff)

- The specs at HEAD (`api.md:15-17`, `guide.md:200-221`, `runtime-image.md:255-268`) already describe
  `provisionError`, `waitForBroker`, the attach-first ordering, and `guestHostname` — the
  documentation row was committed **ahead of** the code it documents, so `503fc8a`'s spec currently
  describes behavior `503fc8a`'s code does not have. This diff closes that gap; worth avoiding the
  ordering next time (owned docs land in the same commit as the code they describe).
- A verification log for this change already exists at
  `.agents/docs/testing/2026-09-02-verify-project-create-provisioning.md` (status PASS), written
  against the earlier `a1e4e0eac525` tree. It predates the `provisionRepo` refactor, the
  `Server`-field pacing, and `provision_test.go`, and its "zero test coverage for
  `waitForBroker`/`provisionRepo`" gaps have since been closed — re-run it against the final tree
  before the commit closes.
- `web/dist/client/assets/*` churn is uncommitted alongside this change. `AGENTS.md` requires the
  embedded `dist` to match `web/src` at commit time; out of this review's scope, but do not let the
  two diverge in the commit.
