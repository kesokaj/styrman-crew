---
owner: reviewer
date: 2026-09-08
---
# Review — standards audit, whole project   (2026-09-08)

Range reviewed: the tree as it stands at `414be72` (v1.24.0 shipped). This is an
audit of the code, not of a diff — every finding asks "is this file right",
not "was this change reasonable".

Scope worked, in the priority order the dispatch gave: `internal/` (all 15
packages, read in full for `files`, `orchestrator`'s guard surface,
`netpolicy`, `credbroker`, and `httpapi`'s auth/routing/network/git/explorer);
`deploy/firewall/styrman-firewall.sh` and `install.sh` in full;
`web/src` (typing discipline, async/error handling, the API boundary);
`cmd/styrman/main.go` in full; `deploy/caddy/ingress` at the security-relevant
surface (header injection/stripping in `generator.go`). See "Not examined".

## Verdict: APPROVE WITH NITS

Nothing here is broken in a way that costs isolation, tenancy or data today.
The isolation boundary — `internal/files`' `os.Root` confinement,
`orchestrator`'s `realBelow` on every Docker bind path, the root firewall's
tighten-before-loosen reconcile, the ingress's strip-then-inject header
handling — is correct at every site I traced, and the reasoning is written
down where the next reader will need it. `go vet` is clean, `go mod tidy` is a
no-op, the direct dependency set is exactly the two REQUIREMENTS approves,
there is no dead Go code (every exported symbol has a caller), and `web/src`
contains no `any`, no `@ts-ignore` and one justified `!`.

The worst thing I found is a trap rather than a defect: the firewall script's
"generic over a rule-family table" claim is not quite true. `scan_current` —
the sole producer of `PURGE`, i.e. the only thing that ever REVOKES a rule —
prefilters lines on the hard-coded literal `--comment "styrman-lan`, which is
not derived from `FAMILY_TAGS`. Today's three tags all happen to start with
`lan`, so it works. A fourth family tagged anything else would be installed by
`reconcile` and never revoked by anyone — loosening without tightening, the one
shape the file's own header says it must never have. Everything else is
documentation drift (`api.md` is a release behind ADR-066), one packaging
question (`internal/httpapi` is several packages wearing one name), and a set
of low-severity consistency items.

## Findings

- **[medium / confidence: high]** `deploy/firewall/styrman-firewall.sh:588` —
  `scan_current` filters candidate lines with the literal
  `case "${line}" in *'--comment "styrman-lan'*)`. That literal is the only
  thing in the reconcile that is not generic over `FAMILY_TAGS`/`FAM_RE`. A
  fourth family whose comment tag does not begin with `styrman-lan` (say
  `styrman-vpn:`) would be desired and inserted by `reconcile`, would never
  enter `SEEN` or `PURGE`, and would therefore survive the disappearance of
  its marker forever — the rule outlives the revocation. It would also escape
  `MALFORMED` detection, so a corrupted instance of it is never cleaned either.
  `count_applied` does NOT share the bug (it tries every `FAM_RE` with no
  prefilter), which is what makes this asymmetry easy to miss: the status file
  would still report the project as fully applied.
  Fix: derive the prefilter from the table — `*'--comment "styrman-'*` covers
  every tag the project will ever mint — or delete the prefilter and let the
  per-family regexes decide (the chain dumps are a few dozen lines; the
  prefilter buys nothing measurable). Answering the dispatch's question
  directly: the script IS followable as a stranger — the family table,
  `rule_args` dispatch and the tighten-then-loosen order read cleanly — and a
  fourth family is "a row plus two small functions" for every part of the
  machinery EXCEPT this one line, which is exactly where the failure would be
  silent.

- **[medium / confidence: medium]** `deploy/firewall/install.sh:345` —
  `iptables -S STYRMAN-FWD | head -12 | sed 's/^/    /'` runs under
  `set -euo pipefail`. On a host where `STYRMAN-FWD` holds more than 12 rules
  (base is 7; three families × prefixes × LAN-enabled projects passes 12 with
  two or three grants), `head` exits first, `iptables` takes SIGPIPE, exits
  141, `pipefail` propagates it and `set -e` kills the installer — AFTER every
  install step has already succeeded. `deploy/host-setup.sh:215` reads that
  exit status and prints `WARN: deploy/firewall/install.sh failed — projects
  stay isolated only if a previous script is installed`, which is untrue and
  will send an operator chasing a non-problem; `styrman-update`'s
  `host_converge` reads it the same way. Confidence is medium only because
  whether SIGPIPE fires depends on the output fitting the 64 KiB pipe buffer
  before `head` exits, so it is intermittent — which makes it worse to
  diagnose, not better. Fix: `iptables -S STYRMAN-FWD 2>/dev/null | head -12 |
  sed …  || true`, or capture into a variable first.

- **[medium / confidence: high]** `.agents/docs/specs/styrman/api.md` (Projects
  table, `GET/PUT /api/projects/{id}/network` rows) — the spec is a release
  behind the code. It documents the admin `host` object as
  `{available, reason, version, requiredVersion, appliedAt, lanDev, lanCidrs,
  lanProjects, warnings}`. `internal/httpapi/network.go:45-64` has shipped five
  more fields since v1.24.0/ADR-066 — `allowHost`, `allowPublishedPorts`,
  `hostAddrs`, `pubSubnets`, `ingressBridge` — plus `credentialImpact` on the
  `lanView` body itself (`network.go:38`). `api.md` mentions none of them
  (`grep -c` over all six names: 0), and does not mention ADR-066 at all. The
  frontend already types all six (`web/src/lib/api.ts:44-72`), so the spec is
  now the only wrong copy. AGENTS.md's Definition of Done step 3 requires the
  `documentation` pass whenever an interface changes; this one was missed.
  Fix: extend both rows, and add the ADR-066 host/published-port semantics and
  the `credentialImpact` contract (why the SSH key is withheld, and that the
  latch is permanent).

- **[medium / confidence: high]** `internal/httpapi` — the dispatch asked
  whether this is one thing or several. It is several. `Server`
  (`server.go:82-157`) carries 25 fields and five distinct mutexes spanning
  seven unrelated subsystems: user spaces, the preview reverse-proxy cache,
  port shares, LAN net-policy + an ingress-bridge cache, the credential store,
  in-flight delete/provision bookkeeping, and the credential-push
  single-flight. `RoutesWithAuth` (`server.go:302-478`) is a 175-line register
  of every route in the product. `explorer.go` (692 lines) is the clearest
  symptom: it holds the file explorer (`listFiles`…`downloadFile`), transcript
  history (`listHistory`, `searchHistory`, `readHistory`), the settings
  endpoint, runtime tuning (`updateRuntime`), git-credential import
  (`importGitCredential`), user administration (`addUser`, `removeUser`) and
  the notifications scan — seven concerns whose only shared property is that
  they hang off `*Server`. The concrete cost is the one the decoupling
  criterion names: two engineers working adjacent features (say, the file
  explorer and user admin) serialize on the same file and the same struct,
  and every new subsystem adds another field and another mutex to `Server`
  rather than landing behind a seam.
  This is a standards/structure finding, not a bug — nothing misbehaves. The
  cheap first cut is to split by seam without moving `Server`: `explorer.go` →
  `explorer.go` (files) + `history_api.go` + `settings.go` + `admin.go`, and
  give each subsystem a small struct of its own fields that `Server` embeds,
  so the mutex and its data live together. `guestRuntime` (`server.go:77`) is
  the model to copy — a deliberately narrow, well-documented seam.

- **[medium / confidence: high]** `internal/files/files.go:53-80` and its only
  registration at `internal/httpapi/server.go:170` — the tenancy boundary of
  package `files` is process-global mutable state (`var trusted struct{…;
  bases []string}`) installed as a side effect by an unrelated package at an
  unspecified moment in startup. Every `files.*` entry point calls `openRoot`,
  and `openRoot` falls back to bare `os.OpenRoot(root)` — the exact
  pre-BUG-002 behaviour that followed a guest-plantable symlink — for any root
  it does not recognise, silently and with no error. Nothing today reaches a
  `files.*` function before `httpapi.New`: I checked `cmd/styrman/main.go`,
  whose only `files` use is `ContainedPath` (main.go:308), which does not go
  through `openRoot`. So this is latent, not live. But the failure mode is
  BUG-002 reintroduced with no test that could catch it: a future boot sweep,
  a second entry point, or a small CLI that imports `internal/files` gets the
  unconfined path by default. The golang skill's "no package-level mutable
  state; dependencies passed explicitly" is the rule this violates.
  Fix: make the bases an explicit dependency — a `files.Roots` value
  constructed once from the config and passed to the callers that need it —
  or, if the global stays, have `openRoot` FAIL rather than fall back when no
  base has been registered, so an unregistered process cannot silently degrade.

- **[low / confidence: high]** Symlink-guard uniformity (the dispatch's second
  question). The guard is applied everywhere it needs to be, but through four
  different mechanisms with no single place saying which to use when:
  1. `internal/files/files.go:87` `openRoot` — `os.OpenRoot(<base>/<tenant>)`
     plus `Root.OpenRoot(rest)`, driven by the `Trust` registry.
  2. `internal/orchestrator/orchestrator.go:1300` `realBelow` /
     `:1322` `projectHomeIsReal` — an `Lstat` walk of every component (a
     check-then-use, correct here only because ADR-064 made the tenant dir
     server-only), alongside `:1332` `mkdirAllConfined`, which uses `os.Root`.
  3. `internal/netpolicy/dir.go:87` `checkMarkersDir` — one `Lstat` on the
     directory, then plain `os.OpenFile`/`os.Remove` by joined path
     (`dir.go:120-137`). Sound, because the marker dir sits beside `users/`
     and never under a mounted home, but it is a third idiom.
  4. `cmd/styrman/main.go:460` (`pruneTranscripts`) and `:524`
     (`migrateBaseHomes`) — raw `os.OpenRoot(filepath.Join(cfg.HomesDir,
     u.Name()))`, hand-rolling exactly the tenant hop that
     `files.OpenRoot` (`files.go:551`) is exported to provide. Both are
     correct; both will drift from `files.openRoot` the day its rules change,
     and `files.OpenRoot`'s doc comment claims "the history store" is its
     caller, which is now only half true.
  Plus `files.ContainedPath` (`files.go:151`), an `EvalSymlinks` +
  string-prefix check with a documented TOCTOU, used at main.go:308.
  Fix: nothing urgent. Route main.go's two roots through `files.OpenRoot`, and
  add a short "which guard when" note to `.agents/rules/guest-writable-paths.md`
  — that file names the three call sites but not the rule for choosing between
  the two primitives.

- **[low / confidence: high]** `internal/credbroker/broker.go:37` — the
  `LanChecker.LanExposed` contract comment reads "must be pessimistic: see
  `netpolicy.Exposed`. It reports the union of the record, root's applied set,
  and 'cannot confirm'." `netpolicy.Exposed` does not exist anywhere in the
  tree (the string appears exactly once, in this comment), and the only
  implementation — `httpapi.LanExposed`, `internal/httpapi/network.go:312` —
  documents at length that it does the OPPOSITE by design: it returns
  `p.LanEverEnabled || p.LanAccess` and "consults root's firewall status not
  at all", because the union version handed the key to a project that still
  had LAN reach (release review round 3). The interface's stated contract is
  the superseded rule. No runtime effect — the wrapper at `broker.go:69`
  fails closed regardless — but the next person implementing this interface
  will implement the wrong thing from the doc comment.
  Fix: replace the comment with the latch rule and drop the dangling
  `netpolicy.Exposed` reference.

- **[low / confidence: medium]** `web/src/lib/api.ts:436` — `return body as T`.
  No API response is validated at runtime; every one of the ~40 typed
  endpoints trusts the `as T`. The checklist's boundary rule aside, this has a
  concrete failure mode that the `api.md` drift above makes real: when the
  server's shape and the hand-written interface disagree, the client renders
  `undefined` deep inside a component instead of failing at the boundary with
  a message. The rest of the file is careful — `redirect: "manual"` after the
  spread, `AbortSignal.timeout`, the shape-checked `sessionStorage` record at
  `:359-380` — which makes the one unchecked cast stand out. Given "stdlib
  first, every dependency justified", pulling in zod for this is not obviously
  right; a handful of narrow `isProject(x): x is Project` guards on the three
  or four shapes that actually drive state would be proportionate.

- **[low / confidence: high]** `cmd/styrman/main.go` (589 lines) — `cmd/`
  carries real business logic, against the golang skill's "cmd/ stays thin":
  `migrateLegacy` (:250), `migrateWorkspaces` (:305), `ensureRepos` (:383),
  `transcriptJanitor`/`pruneTranscripts` (:423/:439), `migrateBaseHomes`
  (:515) and `migrateTenantIntoBase` (:551). They have their own tests
  (`migrate_test.go`, `prune_test.go`) — the tell that they are library code
  living in the wrong place. The practical consequence is item 4 of the
  previous finding: because they are not in `internal/`, they reimplement
  `files`' tenant-root hop instead of calling it. Fix: `internal/migrate` for
  the four migrations, and move the transcript janitor next to
  `internal/history`, leaving `main` to wire and sequence them.

- **[low / confidence: medium]** `deploy/firewall/install.sh:53-56` —
  `--lan-allow-host` and `--lan-allow-published-ports` are validated at the
  flag (`case "$2" in 0|1)`), but `--ingress-bridge` and `--lan-cidr` are
  taken verbatim into `firewall.conf`. A value containing a character outside
  the reconciler's `[A-Za-z0-9./ ,_-]` line grammar makes `read_conf` treat
  the line as unparseable, which calls `conf_fail` and closes EVERY LAN
  allowance on the host — for every project, not just the mistyped one. It
  fails closed, and the reason lands in `/run/styrman-firewall/status`, so
  nothing is exposed; but a typo in one flag silently revokes an unrelated
  project's access with no message at the installing terminal. Fix: validate
  both at the flag with the same shapes root will apply
  (`^br-[0-9a-f]{12}$`, `^a.b.c.d/n$`) and `die` on a bad one.

## Criteria

1. **Dead code — pass.** No dead Go code. I enumerated every exported symbol in
   all 15 `internal/` packages and checked for callers outside the defining
   package: the only hits are exported-but-package-local
   (`links.Expired`/`ListFor`, `netpolicy.ParseStatus`, `project.NewUUID`, and
   `tailBuffer`'s `Dropped`/`StartsAtLineBoundary`, which are methods on an
   unexported type and so effectively private) — over-exported, not dead, and
   not worth a change. `go mod tidy` produces no diff; the direct requires are
   exactly `coder/websocket` + `docker/docker`. No commented-out blocks in the
   Go or shell I read. `web/src` has no orphaned modules that `tsc -b` would
   not already flag. The one thing I'd call out under this heading is
   documentation rather than code: `api.md` still discusses the removed
   checkpoint/`/diff` endpoints, but it does so explicitly as removals
   (lines 44-45, 161), which is correct and useful.
2. **Flow tracing — pass.** Traced end to end: the LAN toggle
   (`web/src/components/LocalNetwork.tsx` → `PUT /api/projects/{id}/network` →
   `setProjectNetwork` → `project.Store.SetLanAccess` + `netpolicy.SetMarker` +
   `Kick` → the spool → `styrman-firewall.sh`'s `collect_desired`/`reconcile`
   → the status file → `netpolicy.ParseStatus`/`Derive` → back into
   `projectView`), including the rollback path's exact-pair restore
   (`network.go:216`, BUG-018) and the LAN-exposed credential latch
   (`network.go:312` → `credbroker.lanExposed`, fail-closed on nil and on
   error). Traced the guest exec path (`gitContext` → `guest.Exec` with
   `GIT_CEILING_DIRECTORIES`), the boot path (`EnsureRunning`'s
   `projectHomeIsReal` before create AND before start, BUG-003), and startup
   sequencing in `main`. Errors are wrapped with `%w` and context throughout;
   `errors.Is` is used for sentinels; goroutines all have a `ctx.Done()` exit;
   `defer Close` follows every open. Two flow observations are recorded as
   findings above (the firewall prefilter, `files.Trust` ordering); nothing
   else calls into a stub or drops an error.
3. **Standards conformance — pass with the nits above.** Go: `go vet` clean;
   `log/slog` JSON everywhere; config validated once in `internal/config` and
   not re-read in handlers; `context.Context` first-parameter discipline holds;
   no `panic` outside the one `//go:embed`-cannot-fail site
   (`server.go:437`, correctly justified). TypeScript: `strict` project refs,
   no `any`, no `@ts-ignore`/`@ts-expect-error`, no floating promises (the
   `void`s at `SidePanelGit.tsx:52`, `projects.ts:57`, `LocalNetwork.tsx:111`
   are all deliberate and paired with a `.catch`), `catch (e)` is `unknown`
   under strict and narrowed before use. The deviations I'd cite are the
   package-level mutable state in `internal/files`, `cmd/` thickness, and the
   unvalidated API boundary in `api.ts` — all above.
4. **Directory layout — pass with one nit.** `cmd/<app>/` + `internal/` + one
   `web/`; no `pkg/`; no stray TS at the repo root; tests sit beside their
   source in every package I looked at; the vendored ingress is correctly a
   separate module under `deploy/caddy/ingress`. The nit is the migration and
   janitor code in `cmd/styrman/` rather than `internal/` (finding above), and
   `internal/httpapi`'s internal file organisation — `explorer.go` in
   particular is not where an unfamiliar engineer would look for user
   administration or the settings endpoint.
5. **Simplicity, DIY-except-security — pass.** The security exemption is
   respected: no hand-rolled auth, crypto, hashing, sessions or JWT/OIDC. OAuth
   is the Caddy ingress's, the ingress-origin token is compared with
   `crypto/subtle.ConstantTimeCompare` (`auth.go:38,57`), preview-link tokens
   and project UUIDs come from `crypto/rand` (`links/store.go:9`,
   `project/uuid.go:4`); the only `math/rand/v2` is friendly terminal names
   (`conversation/names.go:5`), which is correct. Dependencies are two direct
   requires, both named in REQUIREMENTS. Abstraction is restrained —
   `guestRuntime` is the only interface introduced for testability and its
   doc comment justifies the narrowness and states the tenancy contract an
   interface cannot enforce. The one place complexity is not obviously bought
   is the firewall script's family-table generalisation, which currently
   serves three families that share a chain and a tag prefix; it is the right
   call given ADR-066 added two families at once, but see the finding — the
   generalisation must be finished to be worth its cost.
6. **Decoupling — the weakest of the six.** Package boundaries between
   `internal/*` are real seams: `orchestrator` knows nothing of projects
   stores, `credbroker` depends on two small consumer-side interfaces
   (`Resolver`, `LanChecker`) declared where they are used, `netpolicy` owns
   the marker/status grammar and nothing else, `files` owns confinement. No
   cross-package reach-ins and no test importing another package's
   unexported helpers. The problem is inside `internal/httpapi`, which is
   effectively seven modules behind one `Server` god-struct — the finding
   above. `web/src` is fine: features depend on `lib/`, never the reverse, no
   intra-feature barrels.

## Not examined

Named honestly, so nobody reads this as broader than it is.

- `internal/terminal/bridge.go` (469 lines) and `internal/conversation/` — the
  WS/tmux terminal path. Read only at the seam (`termStoreFor`, `ensureFor`,
  `guestUserFor` in `server.go`); the protocol itself (binary/JSON framing,
  close codes 4000/4001, resize) and the tmux session manager were not
  reviewed against `specs/styrman/terminal-protocol.md`.
- `internal/credstore/credstore.go` (491 lines) — read only `IsHostReaching`'s
  contract from its two call sites. The store's file layout, permissions and
  the import/export paths were not audited.
- `internal/httpapi`: `proxy.go`, `links_api.go`, `shares.go`, `stats.go`,
  `check.go`, `format.go`, `setup.go`, `provision.go`, `credharvest.go` were
  read only at the symbol level (signatures + doc comments). The preview-proxy
  authorization path (`previewAccess`, `serveLink`, `serveShared`) deserves its
  own security pass and did not get one here.
- `internal/orchestrator/orchestrator.go` — read `EnsureRunning`, the guard
  helpers, `rootOpAllowed`/`RootRemoveContents`, `RemoveStaleSetupVMs` and the
  cache/lock primitives. `create`, `Destroy`, `Adopt`, `CleanupLegacy`,
  `RunIdleReaper` and `exec.go`'s stream handling were not traced.
- The test suites. I did not run `go test ./...`, the ingress module's tests,
  `web` vitest, `deploy/firewall/test.sh`, the integration script or the
  browser e2e — this was a read audit, and the `verifier` owns that tier. I
  therefore make no claim about coverage adequacy for any package; I only
  observed that every changed-behaviour area I inspected has a `_test.go`
  beside it.
- `image/` (Dockerfile, entrypoint, `styrman-setup`, the vendored harness under
  `image/harness/`) — not opened beyond a `grep` for symlink handling.
- `deploy/caddy/ingress` — only `generator.go`'s header strip/inject surface
  and the package's symbol map. `watcher.go`, `cloudflare.go`, `cmd.go`,
  `config.go`, `labels.go`, `loopback.go` were not reviewed.
- `deploy/host-setup.sh`, `deploy/update/styrman-update`, `deploy/vulncheck.sh`,
  `deploy/verify-commit.sh`, `deploy/docker-compose.yml`,
  `deploy/Dockerfile.server` — only the two lines of `host-setup.sh` that call
  the firewall installer.
- `web/src/components/*` — scanned all of them for typing/async violations, but
  read only `LocalNetwork.tsx`'s seam. `settings.tsx` (1078 lines),
  `FileExplorer.tsx` (683) and `SidePanelGit.tsx` (617) were not read for
  correctness.
- Specs other than `api.md`: `guide.md`, `deploy.md`, `runtime-image.md`,
  `terminal-protocol.md`, `versioning.md` were not checked for drift.
