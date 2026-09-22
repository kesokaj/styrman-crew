---
owner: reviewer
date: 2026-09-22
---
# Review — project-simplify   (2026-09-22)

Range reviewed: the whole repository at HEAD (c05219c), no diff

## Status: COMPLETE

## Verdict: APPROVE WITH NITS

Lens per dispatch: criteria 1 (dead code), 5 (simplicity) and 6 (decoupling),
plus reuse and efficiency. Criteria 2, 3 and 4 are **not in this lens**.
Style is not a finding. Each finding proposes the shorter version.

**Nothing is broken.** I went looking for the four things the dispatch named —
code serving only a superseded ADR, exported identifiers with no caller,
duplication across `cmd/` / `internal/` / `deploy/`, and helpers that survived a
refactor — and the first two came back empty after direct verification. The
superseded designs were genuinely *deleted*, not left behind: `image/harness/`
(ADR-062) and `internal/netpolicy/` (ADR-065/066, 1,049 lines) do not exist.
Every exported function, type, const and var in **both** Go modules has a real
production caller, and `web/src`'s 48 modules have no unreferenced export. The
worst thing I found is a duplicated security helper in the newest code
(`deploy/gcp/install.sh`) that has *already* diverged from the copy it was
forked from — F1/F2 below, both low. The rest is retirement debt and one
god-object seam.

## Findings

- **[low / confidence: high]** `deploy/gcp/install.sh:~50` — `env_get` is a
  weaker fork of `deploy/styrman-ctl`'s `env_get`, and `install.sh` already
  sources `deploy/gcp/lib.sh`, so a shared home exists. styrman-ctl's version
  guards the key with `is_key` and hands it to awk as a **variable** matched
  literally with `index($0, k "=") == 1`; its comment records why ("security
  review: a key with a slash changed a sed program; with GNU sed's e flag that
  is command execution"). The fork is
  `grep -E "^$2=" "$1" | tail -n1`, interpolating `$2` straight into a
  **regex**. Every current caller passes a literal constant
  (`BASE_DOMAIN`, `GOOGLE_CLIENT_SECRET`, …) so there is no live exploit — the
  defect is that the hardening does not travel to the fork, and the next caller
  has no way to know only one of the two is safe.
  *Shorter version:* move `is_key` + `env_get` into `deploy/gcp/lib.sh` (or a
  `deploy/lib-env.sh` that both `styrman-ctl` and `install.sh` source) and
  delete the one-liner. Net −1 line in `install.sh`, one implementation to
  review instead of two.

- **[low / confidence: high]** `deploy/gcp/install.sh` `private_file` vs
  `deploy/styrman-ctl` `private_file` — an 11-line near-copy of a secret-file
  gate, **already diverged in two of its five checks**: (a) owner —
  styrman-ctl accepts `root` / `$SUDO_USER` / `$(id -un)`, install.sh accepts
  only `$(id -un)`; (b) parent directory — styrman-ctl rejects
  group/other-**writable** (`mode & 022`), install.sh demands mode 700
  (`mode & 077`). Both divergences are defensible in isolation, and nothing in
  either file says which are intentional; a reader hardening one copy cannot
  tell whether the other is deliberately laxer or simply stale. Two copies of a
  security check that have drifted once will drift again.
  *Shorter version:* one `private_file` in `lib.sh` taking the owner policy as
  an argument (`private_file <f> [--owner-self]`) and the stricter parent test
  (`& 077`) for both callers. Deletes 11 lines and gives the next hardening one
  place to land.

- **[low / confidence: high]** `internal/httpapi/server.go` (1,405 lines) and
  the `Server` struct (30+ fields, five independent mutexes) — the package's
  one seam is a grab-bag, and the coupling is concrete rather than aesthetic:
  `deletingMu` guards `deleting` / `deleteErrs` / `provisionErrs` (project
  lifecycle, server.go) **and** `credPushing` / `credPushRerun` (the per-user
  credential push, credharvest.go — the struct comment says so: "credPushing
  (deletingMu, …)"). Two unrelated features share a lock and a struct, so an
  engineer on credential push and an engineer on project deletion collide in
  the same declaration and must reason about each other's critical sections.
  The critical sections are tiny, so this is a decoupling finding, not a
  performance one. server.go additionally carries project CRUD, terminal CRUD,
  routing, CSRF, health and the JSON/clamp helpers in one file.
  *Shorter version:* give credharvest.go its own `credMu` beside its two maps
  (3 lines, removes the cross-feature lock entirely), and move the seven
  terminal handlers (`listTerminals` … `deleteAllTerminals`, ~140 lines) into
  `internal/httpapi/terminals.go` — the per-feature-file pattern the package
  already uses for `shares.go`, `network.go`, `proxy.go`, `credharvest.go`.
  server.go drops to construction + routing.

- **[low / confidence: medium]** Three independent converge paths for
  superseded designs run forever with no retirement criterion recorded
  anywhere: `deploy/update/styrman-update:178` `retire_harness_units()`
  (~35 lines, sheds ADR-073's units and four state files, on **every** tick);
  `deploy/firewall/install.sh:65` §2 "Converge away the ADR-065/066 host
  artifacts" (~40 lines); `deploy/firewall/styrman-firewall.sh:118` "One-shot
  cleanup: ADR-065/066 leftovers" (~50 lines of `iptables -S` parsing with a
  strict anti-injection regex), plus `deploy/firewall/test.sh:97` §4 testing
  it. The code itself is correct, idempotent and unusually well-commented —
  the finding is that ~125 lines and a test section now have no reachable
  consumer. The NUC is at v1.37.0 (so both converges have long since run), the
  GCP VM is deleted (c05219c), and a host created by `deploy/gcp/install.sh`
  never had ADR-065/066 artifacts to clean. The only consumer left would be a
  host frozen between v1.23 and v1.35 that upgrades later, which on a
  single-host pre-production platform is nobody.
  *Shorter version:* delete all three blocks in one release and record the
  removal in ADR-067's and ADR-074's Consequences ("the converge shipped
  v1.27.0–v1.36.0 and is removed in vX.Y.Z"). If the operator prefers to keep
  a safety margin, the minimum is a one-line retirement note at each block
  naming the release that deletes it — not `TODO.md`, which stays empty.
  Marked medium confidence only because retaining them is a defensible
  operator call, not because the reachability analysis is uncertain.

- **[low / confidence: medium]** `internal/links/store.go:96` `newToken()` —
  seven lines and an error return to make a 128-bit hex token from
  `crypto/rand.Read` + `hex.EncodeToString`. Both modules are `go 1.25`, where
  `crypto/rand.Text()` is the stdlib idiom for exactly this (the
  `review-standards` skill names it), returns ~130 bits, and cannot fail.
  *Shorter version:* `func newToken() string { return rand.Text() }`, which
  also deletes the `(string, error)` shape from its callers. **Caveat the fix
  must handle:** this changes new tokens from 32 lowercase hex chars to 26
  base32 chars, so before landing it, grep for anything that validates or
  routes on token shape (the ingress `generator.go`/`labels.go` path, any
  `[0-9a-f]{32}` regex, the e2e checks); already-persisted tokens keep working
  since they are stored verbatim. Confidence medium on the *safety* of the
  change for that reason, high that the current code is longer than it needs
  to be.

## Criteria

1. **Dead code — pass, in both modules.** No exported function, type, const or
   var in `cmd/` + `internal/` lacks a production caller: I enumerated all of
   them across the 43 non-test Go files and hand-inspected every one of the 21
   lowest-reference candidates (`ProjectFromNetworkName`, `RootRemoveContents`,
   `ReapSetupVMs`, `HasGuestUser`, `Dropped`, …) — each has a real call site.
   The same sweep over `deploy/caddy/ingress/` (the second module, 2,996 lines)
   returned exactly one candidate, `module.go:CaddyModule`, which is the Caddy
   framework's required interface method and therefore not dead. `web/src`
   (48 modules) has no unreferenced export. `cmd/wsprobe` **looks** orphaned but
   is not: `.agents/docs/testing/styrman/integration.sh:74` builds it and
   invokes it seven times — verified before reporting, per the dispatch.
   Superseded-ADR directories are genuinely gone (`image/harness/`,
   `internal/netpolicy/`). What remains of those designs is converge code, F4.

2. **Flow tracing — not in this lens.**

3. **Standards conformance — not in this lens.**

4. **Directory layout — not in this lens.**

5. **Simplicity (DIY-except-security) — pass, with F4/F5 as nits.**
   `go.mod` has **three** direct dependencies (`coder/websocket`,
   `docker/docker`, `golang.org/x/sys`), each load-bearing — exemplary
   stdlib-first for a 29k-line platform; the ingress module adds only
   `caddyserver/caddy/v2` and `docker/docker`, both unavoidable for what it is.
   No speculative abstraction found; the one-implementation interfaces that
   exist (`guestRuntime`, `modeRuntime`, `wsBooter`, `relayCounter`) all have
   test doubles, which is the documented exception. **Security exemption:
   clean.** Nothing hand-rolled — every `crypto/*` use is the stdlib consumed
   as designed: `crypto/rand.Read` for link tokens and UUIDs, `crypto/subtle`
   for the ingress-token constant-time compare, `sha1.Sum` purely as a
   bridge-name identifier derivation (not a security claim, and the comment
   says so), `math/rand/v2` only for human-readable terminal names. No
   hand-rolled JWT, session, MAC, KDF or password path; authentication is
   delegated to the Caddy ingress + portal.
   `internal/project/uuid.go`'s 9-line UUIDv4 is a deliberate
   dependency-avoidance with correct variant/version bits and a real error
   path — correct call, not a finding.

6. **Decoupling — mostly pass; F3 is the one real seam.** Package boundaries
   are clean and the `internal/*` stores (`project`, `links`, `credstore`,
   `conversation`, `userstore`, `history`) are deep modules with small
   interfaces and no cross-package reach-ins — the repeated
   `New`/`List`/`Get`/`Delete` names across them are parallel structure, not
   duplication. The two-module split is clean in the direction that matters:
   the ingress keys on its own generic label vocabulary
   (`styrman-ingress`, `styrman-launcher`, hostname/port/mode labels) and does
   **not** duplicate the server's `styrman.project` / `styrman.user` /
   `styrman.managed` label set, so it remains a general Docker-label ingress
   rather than a second copy of styrman's state model. The exceptions are the
   two god-files, `internal/orchestrator/orchestrator.go` (1,704 lines) and
   `internal/httpapi/server.go` (1,405), both of which force serialized edits;
   only server.go's shared-mutex coupling rises to a finding (F3). `writeJSON`
   existing in both `credbroker` and `httpapi` is correct — they are separate
   listeners and sharing it would couple the bridge-only broker to the public
   API for four lines.

**Reuse / efficiency (the dispatch's extra lens).** Reuse is the weak spot,
and it is localised entirely in the newest code: `deploy/gcp/install.sh`
re-implements two `styrman-ctl` helpers (F1, F2) despite already sourcing
`deploy/gcp/lib.sh`. The trivial `log`/`die`/`run`/`warn`/`usage` repeats
across the deploy scripts are **not** a finding — those scripts must stand
alone on a fresh host. On efficiency I found no hotspot worth reporting: the
`links` store documents and bounds its own O(N) rewrite
(`MaxLivePerProject = 20`), and the caches that exist (`previewOwners`,
`notifCache`, `proxies`) are deliberate and commented.

**Outside the requested lens, noted without being counted as findings** (all
criterion 3, which the dispatch excluded): `README.md:113` "Status" still reads
**v1.34.1** while HEAD is v1.37.0; `cmd/wsprobe/main.go:6`'s usage comment
still shows `?kind=shell`, a parameter that no longer exists — recorded as low
in `reviews/2026-09-01-audit-reviewer.md` and never fixed.

## Not examined

- **`image/`** (Dockerfile, `entrypoint.sh`, `styrman-setup`, `skel/`,
  `configs/`, `bump-pins.sh`) — not read at all. Most likely remaining home for
  a finding in this lens.
- **`deploy/caddy/ingress/` internals** — the exported-symbol sweep and the
  label-vocabulary comparison ran clean, but `generator.go` (503),
  `watcher.go` (415), `cloudflare.go` (341) and `labels.go` (190) were not read
  line by line for internal duplication or unreachable branches.
- **`web/` beyond the export sweep** — no read of component internals, no dead
  CSS/asset check, no `depcheck`/`npm ls` for unused frontend dependencies.
- **Unexported dead code in either Go module** — my sweep was
  exported-identifier-based plus a cross-package name collision check;
  package-private helpers orphaned by a refactor would not show up.
  `deadcode` could not substitute: the installed binary is built against
  go1.26 and the toolchain is go1.27.1, so it errors out in `internal/poll`
  before analysing anything. Rebuilding it would close this gap cheaply.
- **`internal/orchestrator/orchestrator.go` (1,704 lines) and
  `internal/httpapi/git.go` (1,001)** — sized and noted as god-files, but not
  read line by line for internal duplication or unreachable branches.
- **Criteria 2, 3 and 4** — excluded by the dispatch, not attempted.
