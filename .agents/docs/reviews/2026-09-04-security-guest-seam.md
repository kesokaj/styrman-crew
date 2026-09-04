---
owner: security
date: 2026-09-04
---
# Security review — guestRuntime seam in internal/httpapi   (2026-09-04)

Range reviewed: 48cb698..worktree — 8 modified files (`internal/httpapi/{check,credentials,credharvest,explorer,format,git,server,stats}.go`)
plus the untracked `internal/httpapi/guestexec_test.go`. Blast radius followed outward:
`internal/orchestrator` (`Status`/`Exec` signatures and their tenant-binding), every
`s.guest` call site and its caller chain, `internal/httpapi/server.go` construction,
`cmd/styrman/main.go` (the only production `httpapi.New` caller), `internal/files.Trust`
(global registration reached from `New`), and the test harness's config surface.
The v1.22.0 git verbs themselves are **out of scope** — reviewed at
`2026-09-04-security-phase6-git-verbs.md` and `2026-09-04-security-release-v1.22.0.md`.

## Verdict: FINDINGS

Nothing here is exploitable, and the two questions that actually mattered come back
clean: the routing is a **pure receiver rename** at all 15 sites (verified mechanically,
not by eye — see Q1), and `Server.guest` **cannot be nil or anything but the real
orchestrator in production** (unexported field, single writer in `New`, single production
constructor, `main` exits before `httpapi.New` if `orchestrator.New` fails). The worst
thing on the record is a documentation defect with a security consequence: the new test
file's header claims *"Handler-level tests for every endpoint that reaches into a
project's microVM"*, and that is false — **11 of the 15 routed sites have no coverage,
including both credential-path sites the seam was flagged for**. A future reviewer who
takes that sentence at face value will believe the credential harvest and push exec paths
are exercised by unit tests. They are not. Everything else is low: the seam converts a
compile-time-guaranteed tenant lookup into an interface contract that is nowhere written
down, and the fake that now stands in for it violates that contract by design.

## Findings

- **[medium / confidence: high]** `internal/httpapi/guestexec_test.go:19-21` — the header
  comment claims coverage of "every endpoint that reaches into a project's microVM". Four
  of the fifteen routed call sites are covered (`git.go:59` gitContext, `git.go:90`
  gitExec, `check.go:358`, `format.go:92`). Eleven are not: `credharvest.go:149`
  (`guestOut`, the harvest read path), `credharvest.go:255`
  (`pushCredsToRunningProjects`), `credentials.go:200` (`importCredentials`),
  `explorer.go:545` + `explorer.go:553` (`importGitCredential` — the gh-token read),
  `explorer.go:646` (`listNotifications`), `git.go:886` (`gitSummary`), `server.go:837`
  (createProject's `run` closure), `server.go:856` (`getProject`), `stats.go:61` +
  `stats.go:74` (`projectStats`). The exercised route set is literally
  `"/api/projects/"` prefixed only. Why this is a security problem and not a style nit:
  the two credential sites are the ones this change was sent for review over, and a
  claimed-covered-but-uncovered exec path is how a regression on a credential read
  ships green. Remediation: narrow the sentence to what is true — "the git verbs, the
  formatter and the syntax checker" — and list the uncovered sites explicitly so the
  gap stays visible instead of being papered over.

- **[low / confidence: high]** `internal/httpapi/server.go:69-72` — `guestRuntime`'s doc
  describes `Status` as "where the container is", i.e. a lookup. It is more than that:
  the `ContainerID` it returns is the **only** thing binding the subsequent `Exec` to the
  right tenant's microVM. Under `*orchestrator.Orchestrator` that binding was enforced
  unconditionally by the label lookup; behind the interface it is an unstated contract.
  Production is fine (one implementation, assigned in `New`, and every call site passes
  the server-derived `u.Username` from `s.who(r)`/`identity.FromContext`, never a
  client-supplied id). The exposure is forward-looking: a second implementation, or a
  fake used to assert a security property, is held to nothing. Remediation: state the
  invariant on the interface — "the returned ContainerID belongs to `user`/`project`; a
  caller may exec into it on that basis alone".

- **[low / confidence: high]** `internal/httpapi/guestexec_test.go:44-52` — `fakeGuest.Status`
  ignores `user` and `project` entirely, always returns `ContainerID: "container-1"`, and
  **never returns an error**. Two consequences. (1) No test written against this fake can
  ever catch a handler that passes the wrong user to `Status` — the exact confused-deputy
  shape the invariant above exists to prevent — so the fake bakes in "ownership does not
  matter". (Cross-tenant access by project id is still gated for real in these tests by
  the genuine per-user `project.Store`, which is why this is low and not medium.) (2) The
  error half of "status mapping and error handling" that the seam comment claims to have
  made testable is still uncovered: `gitContext`'s `serr != nil` branch, `getProject`'s
  500, and `importCredentials`' 409-on-error are unreachable through this fake.
  Remediation: give `fakeGuest` a `statusErr` field and a per-(user,project) container
  map, and add one test that a handler asking for another user's project does not get a
  container id.

- **[low / confidence: high]** `internal/httpapi/guestexec_test.go:407,586,657` — the
  strongest claim in the injection tests, *"nothing may reach git at all"*, is asserted
  with `f.lastGit(sub)`, which only matches argv of the exact shape
  `git -C <ws> <sub>`. A future regression in which a rejected value reaches git under a
  different argv shape (a different subcommand at index 3, or a `git -C ws -c ...` form)
  would leave the assertion silently satisfied. `gitCheckout` provably validates before
  any `Exec` today and `gitContext` runs only `Status`, so `len(f.calls) == 0` is
  available and is strictly stronger. Remediation: assert `len(f.calls) == 0` (or scan
  every recorded argv for the injected token) in
  `TestCheckoutRejectsArgvInjectionBeforeAnyGitCall`,
  `TestStashOpEnumIsClosed` and `TestCommitDiffRejectsABadHashBeforeAnyGitCall`.

- **[low / confidence: high]** `internal/httpapi/guestexec_test.go:33-35` — `fakeGuest`'s
  doc justifies its mutex with "creating a project kicks off a background boot that also
  reaches the guest". Under `guestTest` that is not what happens: `cfg.Image` is empty, so
  `createProject` takes the `s.cfg.Image == ""` early return at `server.go:793` and the
  boot goroutine never starts — `server.go:837`'s exec closure is never reached. The mutex
  is still correct and worth keeping (`Exec` is called from `gitSummary`/notification
  goroutines), but the stated reason implies coverage of the createProject exec path that
  does not exist. Remediation: correct the comment; keep the mutex.

- **[low / confidence: medium]** `internal/httpapi/guestexec_test.go:106-125` — the seam is
  deliberately partial (and `server.go:74-76` says so honestly), but `guestTest` gives no
  warning that the server it returns is only *partly* detached from the host Docker
  daemon. `s.orch` still serves `EnsureRunning` (`server.go:204` `ensureFor`,
  `server.go:1017` `startProject`), `Adopt` (`server.go:631` `listProjects`,
  `credharvest.go:243` the credential push), `Destroy` (`server.go:915`) and
  `DestroySetup` (`setup.go:133,148`), and only `createProject` carries the
  `cfg.Image == ""` test guard. A future test that uses this harness against
  `GET /api/projects`, a terminal WS, or `/api/settings/credentials/import` would reach
  the host's live daemon — `pushCredsToRunningProjects` would call the real `Adopt` and
  enumerate the host's real `styrman-*` containers. That is the ADR-052 foot-gun class
  (privileged side effects from a unit test), even though no test in this file triggers
  it: the routes exercised are `/api/projects/<id>/…` only, `orchestrator.New` merely
  builds a client without dialling, and `StateDir`/`HomesDir` are `t.TempDir()`.
  Remediation: a sentence on `guestTest` naming the routes that still reach real Docker,
  or extend the `cfg.Image == ""` guard to `ensureFor`/`startProject`.

- **[low / confidence: high]** `internal/httpapi/server.go:53-55` — `// Server wires the API
  handlers to their dependencies.` is now orphaned directly above `guestRuntime`, so
  godoc attributes the Server's doc to the interface and `Server` is left undocumented.
  Cosmetic, no security impact, but it is in this diff. Remediation: move the line back
  onto `type Server struct`.

- **[low / confidence: medium]** `internal/httpapi/guestexec_test.go:634-648` —
  `TestMutationsRefuseAStoppedVM` asserts only `w.Code < 400` fails, so any 4xx passes,
  including a 400 raised for a reason unrelated to the stopped VM. It does have teeth
  today (the bodies used are all valid, so removing the stopped-VM guard would yield a
  200), but the assertion does not pin *which* refusal happened. Remediation: assert the
  specific status (409) the stopped-VM path returns.

## Answers to the four questions asked

**Q1 — is the routing behaviour-preserving at all 15 sites? Yes, mechanically verified.**
Not by reading: every `-` line of the diff was normalised `s.orch.` → `s.guest.` and
compared against the `+` lines. The residue is exactly the `guestRuntime` declaration, its
doc block, the `guest` field, and `guest: orch` in `New`. Not one call site changes its
context, argument order, error handling, or position relative to a guard. Specifically on
the flagged sites: `credharvest.go:149` (`guestOut`) keeps the same `ctx`, the same
`STYRMAN_SKIP_CRED_PULL=1` env (H6) and the same `err != nil || code != 0` → `("" ,false)`
mapping; `credharvest.go:255` keeps the per-project 30 s `ectx`, the `ecancel()` before the
error check, and the `continue`-not-abort semantics; `credentials.go:200` keeps
`err != nil || st.State != "running"` → 409 fused into one condition, still after
`sp.projects.Get` (which is the real authorization); `server.go:837`'s closure keeps its
captured `ctx` and `containerID` and is still assigned to `spec.run` before
`provisionRepo`. The interface method set is signature-identical to the concrete methods
(`orchestrator.go:147`, `exec.go:148`) — it would not compile otherwise. `go build ./...`,
`go vet ./internal/httpapi`, and `go test -count=1 -race ./internal/httpapi` all pass
(83 tests, 40 of them the new file's).

**Q2 — can `Server.guest` be nil or non-orchestrator in production? No.** The field is
unexported, so only package `httpapi` can write it; the only writes in the tree are
`server.go:154` (`guest: orch`) and `guestexec_test.go:123`. `New` is the only `Server`
constructor outside `_test.go` files (the three `&Server{…}` literals are all in existing
tests), and `cmd/styrman/main.go:58-62` exits before reaching `httpapi.New` at line 113 if
`orchestrator.New` fails, so `orch` is never nil there. The typed-nil trap
(`guest != nil` while the boxed `*Orchestrator` is nil) is inert: `grep` finds no
`s.orch == nil` / `s.guest == nil` check anywhere in the tree, so no logic depends on a
nil comparison the interface could defeat. Note for the record that the three existing
`&Server{…}` test literals leave `guest` nil — that is parity with the old nil `orch`
(same panic class, not a regression).

**Q3 — does the interface weaken an existing security property?** Not in production, and
the seam is genuinely narrow as advertised. The one real transfer is the second finding:
`Status`'s tenant-binding stopped being a property of a concrete type and became an
unstated interface contract, and the sole fake violates it. No authorization decision
moved: every `Status` call still passes `u.Username` derived server-side from the ingress
identity, and the per-user `project.Store` lookup still precedes it at every site.

**Q4 — misleading claims in the test file?** Yes — the coverage overclaim (medium), the
stale background-boot rationale, the `lastGit`-shaped negative assertions, and the fake's
never-failing `Status`. All four are recorded above. One further framing note, not a
finding: `guestTest` sets `AuthMode: "none"`, so none of these tests traverse the
`X-Styrman-Ingress` / `X-Caddy-Email` guard. That matches the package's existing test
convention and nothing claims otherwise, but "handler-level tests" should not be read as
covering the auth layer.

## Checked

- **Secrets** — none introduced. No credential-, token-, key- or password-shaped literal in
  `guestexec_test.go` (grepped); the only committed secret-shaped path in the tree is
  `deploy/.env.example`, `.env`/`*.env`/`.env.*` are gitignored with an `!.env.example`
  negation, and `deploy/.env` is absent locally. No new logging: the diff adds no `Warn`/
  `Error` call, so no credential can newly reach a log line. The credential values handled
  at `credharvest.go:149` still flow only into `credstore`, never into a response or a log.
- **Injection** — no new parsing boundary and no new external input reaches the changed
  lines; the diff changes a receiver, not an argument. The `ExecSpec.Cmd` slices are
  byte-identical before and after at all 15 sites, so the argv-construction surface is
  untouched. The new tests exercise the existing validators (`validBranchName`, the stash
  op enum, the commit-hash shape) rather than adding any input path of their own.
- **Authz/authn** — unchanged. No check was moved, removed, or reordered: verified by the
  mechanical line comparison, and re-read at the two credential sites and `gitContext`.
  All seven `Status` call sites still pass server-derived `u.Username`
  (`s.who` → `identity.FromContext`), never a client-supplied identifier, and each is still
  preceded by the per-user `project.Store` lookup that is the actual ownership gate. The
  residual is documented above as the interface-contract finding.
- **Crypto** — nothing touched. No hashing, signing, session, token or JWT/OIDC code in the
  blast radius; no hand-rolled construction introduced.
- **Dependencies** — none added: `go.mod`, `go.sum`, `web/package.json` and
  `web/package-lock.json` are untouched by the diff, and the new test file imports only
  stdlib plus four first-party packages. `bash deploy/vulncheck.sh` (the authoritative run,
  inside the pinned `golang:1.25.13-trixie` builder): root module and
  `deploy/caddy/ingress` each report the two known, fix-unavailable docker/docker findings
  **GO-2026-4887** and **GO-2026-4883** (`github.com/docker/docker@v28.5.2+incompatible`);
  `image/harness` clean; script exit 3, exactly as its own header documents for accepted
  daemon-side findings. Recorded as accepted, unchanged by this diff. For contrast, host
  `govulncheck ./...` on the machine's go1.25.0 toolchain reports 31 findings, all of them
  the stdlib set the shipped go1.25.13 build does not carry — the documented host false
  positive, not a regression. `npm audit --omit=dev` in `web/`: **0 vulnerabilities**
  (`web/` is outside this diff).
- **GCP surface** — not applicable: styrman is self-hosted (`.agents/docs/deploy/gcp.md`
  records this), and the diff contains no IAM binding, service account, Cloud Run flag,
  or key material. Nothing in the blast radius reaches a cloud API.
