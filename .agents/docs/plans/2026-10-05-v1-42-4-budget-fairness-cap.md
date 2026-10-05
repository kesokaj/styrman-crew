---
owner: main
date: 2026-10-05
---
# Plan — v1.42.4: memory budget, tenant fairness, admin resource cap, BUG-074/091   (2026-10-05)

## Why
The v1.42.3 gate report (`reviews/2026-10-05-preflight-v1.42.3.md`) and handoff
(`handoffs/2026-10-05-*-v1.42.3*.md`) left these open, and the operator asked for
"everything done, nothing in roadmap and todo". The operator confirmed the scope
2026-10-05 ("go all") over this numbered list:

1. One server-wide memory budget (1 GiB; ≤ 256 MiB per tenant) for building
   responses and reading request bodies across every API endpoint; when it is
   full, a request waits up to 2 s, then gets 503 "server busy, try again". The
   server runs under `mem_limit: 4g` with `GOMEMLIMIT=3GiB` (ADR-085), so
   enough concurrent large responses can OOM-kill it and drop every tenant's
   sessions.
2. Per-tenant fairness: each tenant holds at most 2 slots of each existing pool
   (editor/file 8, transcript read 8, search 4).
3. Admin cap: two admin settings, "max CPUs" and "max memory" for non-admin
   projects, default half the host. For a non-admin owner "Unlimited" means up
   to the cap and asking above it is refused; admins are uncapped; existing
   non-admin projects above the cap are trimmed at their next start.
   (Operator, earlier: "admin cap".)
4. The slot read deadline only on requests that carry a body; a body that times
   out gets 408, not 400.
5. Token tooling: `styrman-update` and `styrman-ctl` refuse an empty token file;
   the doc snippet fixes an existing file's mode and refuses an empty file.
6. E2E: a green run removes its screenshot dir; clearing or closing the history
   panel cancels the running search.
7. BUG-091: the BUG-092 focus wait in the git-identity fixture; closed after two
   green e2e runs.
8. BUG-074: a per-deployment session cookie name (short hash of the base
   domain); everyone is signed out once when it ships.
9–10. Operator steps (narrow updater token, signing-key passphrase): handed back
   in the handoff's Next Steps, not built.

Recon: `plans/2026-10-05-recon-v1-42-4.md`.

## Scope
In: items 1–8.
Out: capping the per-user setup VM (created at the whole host,
orchestrator.go:834-840; the request says "non-admin projects"); making the budget numbers configurable (constants; D3); capping disk for
non-admins (the request names CPU and memory only); a non-admin identity in the
e2e/Tier 2 harnesses (both run their server in `STYRMAN_AUTH=none` mode, where
every request is admin, auth.go:90-92; `web/e2e/run.sh:135`,
`integration.sh:26,50`).
Follow-ups noted, not built: none yet.

## Surfaces touched
- `internal/membudget/` (new, stdlib): the weighted per-tenant byte budget.
- `internal/httpapi/server.go` (`RoutesWithAuth` chain :611, `runOptsFor`/`resourcesOf` :108-123, `ensureFor` :315, callers :331/:997/:1192), `internal/httpapi/budget.go` (new: middleware + weight table), `explorer.go` (`acquireSlot` :481, `fileSlot` :463, slot handlers, `settings` :561, `updateRuntime` :615), `format.go`, `check.go`, `resources.go` (:63 `limitsFor`, :71 GET, :116-143 PUT, :234), `network.go:230`.
- `internal/userstore/userstore.go` (new `IsAdminUsername`).
- `internal/files/files.go` (`List`: a byte bound beside `maxListEntries`, :43, :208-254).
- `internal/orchestrator/resources_start_test.go:23`, `resources_test.go:24` (callers of `SetOperator`).
- `web/src/lib/historySearch.ts` (new, pure search controller) + `historySearch.test.ts`.
- `.agents/docs/testing/styrman/README.md:409` (`run.sh` screenshot wording).
- `internal/config/config.go` (operator settings `userMaxCPUs`, `userMaxMemoryGiB`; `SetOperator`; settings.json load).
- `web/src/lib/api.ts`, `web/src/lib/resources.ts`, `web/src/components/ResourcesDialog.tsx`, `web/src/routes/settings.tsx` (`RuntimeEditor` :965), `web/src/components/SidePanelHistory.tsx`.
- `deploy/caddy/ingress/config.go:46-49`, `generator.go` (:147-152, :263-300 portal block, :364 authz policy, :435-437), `generator_test.go`.
- `deploy/update/styrman-update` (:559-578, :879), `deploy/styrman-ctl` (`private_file` :547, token prompt :600), tests `internal/config/styrman_update_test.go`, `styrman_ctl_test.go`.
- `.agents/docs/deploy/remote.md:81-96`, `.agents/docs/specs/styrman/deploy.md:256-269` (token snippet).
- `web/e2e/run.sh` (:134 `SHOTS`, exit cleanup :24-107), `web/e2e/run.mjs` (:1005-1035 identity fixture; BUG-092 wait pattern :424-434).
- Docs: `specs/styrman/api.md`, `specs/styrman/guide.md`, `testing/styrman/README.md`, `DECISIONS.md` (ADR-086, ADR-087, ADR-088), bugs BUG-074, BUG-091.

## Interfaces

### 1. Memory budget (`internal/membudget`)
```go
package membudget
var ErrBusy = errors.New("server busy, try again")
type Budget struct{ /* mu, cond-free: a mutex + a waiter channel list */ }
func New(total, perTenant int64) *Budget
// Acquire reserves n bytes for tenant, waiting until ctx is done. n is
// clamped to perTenant (a request larger than a tenant's share is admitted
// alone, never refused forever). Returns a release func (idempotent) or
// ErrBusy when ctx ends first. Admission is FIFO-fair: a waiter is never
// overtaken by a later, smaller request of the same tenant.
func (b *Budget) Acquire(ctx context.Context, tenant string, n int64) (release func(), err error)
```
FIFO is per tenant; across tenants a waiter is admitted as soon as both the
total and its own tenant's share fit (a tenant at its share never blocks
another tenant). Tests read internal state from inside the package.
Constants in `internal/httpapi/budget.go`: `budgetTotal = 1 << 30`,
`budgetPerTenant = 256 << 20`, `budgetWait = 2 * time.Second`. The `Server`
holds `budget *membudget.Budget`, built in `New` from the constants; tests
replace the field with a small budget.

Middleware `func (s *Server) budgetGuard(mux *http.ServeMux) http.Handler`,
placed `s.previewHostRewrite(s.authGuard(authMode, csrfGuard(s.budgetGuard(mux))))`
— after csrfGuard so the body is already wrapped by its `MaxBytesReader`, and
after authGuard so `identity.FromContext(r.Context()).Username` is set. It calls
`_, pattern := mux.Handler(r)` and looks the pattern up in `routeWeight
map[string]weightFn` (`type weightFn func(r *http.Request) int64`); a pattern
not in the table uses `defaultWeight` (2 MiB + body, below). Weight 0 = not
charged; `Acquire` is not called.
Tenant key: `Username`; an empty Username (preview-only users) uses the
tenant key `"-"`. `body(r)` uses `bodyCap(path)`, csrfGuard's per-path cap
switch (server.go:704-717) extracted into one function both call.

Weight table (bytes reserved before the handler runs; `body(r)` =
`min(r.ContentLength, bodyCap(path))` when ContentLength ≥ 0, else
`bodyCap(path)`; 0 for GET/HEAD, which csrfGuard does not cap and no handler
reads). Weight-0 rows for `/link/`, `/shared/`, `/proxy/`, `/p/` cover every
method they are registered for (server.go:495-507). The "every table pattern
resolves" test builds a request from each pattern by substituting a literal
for each `{x}` and asserts `mux.Handler` returns that same pattern.

Body read deadline: the middleware sets `slotReadTimeout` (30 s) as the read
deadline on every charged request that carries a body (same body test as §4),
except the two upload routes (a 512 MiB upload may legitimately take longer;
their 4 MiB reservation is small). So no reservation is held by a body that
trickles. A decode that hits it is a 400 on ordinary handlers (unchanged
code) and 408 on the three §4 handlers.
| Patterns | Weight | Why |
|---|---|---|
| `GET /ws/*`, `/link/*`, `/shared/*`, `/proxy/*`, `/p/*`, `GET /`, `GET .../download`, `GET /api/health`, `GET /api/version` | 0 | streamed or constant; holding a reservation for a WebSocket's life would starve the budget |
| `GET .../file`, `GET .../home/file` | 14 MiB | `files.MaxReadBytes` 2 MiB raw + up to 6× JSON escaping (explorer.go:456) |
| `PUT .../file`, `PUT .../home/file`, `POST .../format`, `POST .../check` | 3 × body(r) + 2 MiB | the 8 MiB body decoded into a string, then re-encoded |
| `POST .../upload`, `POST .../home/upload` | 4 MiB | multipart is streamed to disk (explorer.go:329); parts buffers only |
| every `/api/projects/{project}/git/*` route | 14 MiB | `maxExecOutput` 2 MiB (orchestrator/exec.go:32) × (1 raw + 6 escaped); the implementer checks whether stderr is buffered beside stdout and doubles the weight if so |
| `GET /api/git/summary` | 56 MiB | 4 concurrent execs (git.go:972) × 14 MiB |
| `GET .../files`, `GET .../home/files` | 14 MiB | `files.List` gains a 2 MiB bound on the sum of entry name+path bytes (Truncated set, like the 5000-entry bound): 2 MiB × 7 |
| `GET .../history` | 16 MiB | ≤ 5000 transcripts (history.go:64) × guest-chosen file names ≤ 255 B + metadata, × 7 |
| `GET .../history/{id}` | 9 × n + 1 MiB, n = `?bytes` clamped to 4 MiB, or `history.Read`'s 256 KiB default when absent/0 | the one size known up front (explorer.go:545); raw + stripped copy + 6× escaped + slack |
| `GET /api/settings/credentials/{kind}/{id}/content` | 4 MiB | credstore caps a file credential at 256 KiB (credstore.go:307) |
| `GET .../history/search` | 32 MiB | `searchOne` holds a 4 MiB tail, its string, the stripped copy, two `ReplaceAll` copies and a lowered copy that grows 3× on invalid UTF-8 (history.go:172-185): ~25-30 MiB peak; output ≤ 200 × 240 runes |
| `GET .../logs` | 4 MiB | `Logs` keeps a 256 KiB tail (orchestrator.go:1720) |
| everything else (default) | 2 MiB + body(r) | small JSON; body ≤ 1 MiB default cap |

On `ErrBusy`: `w.Header().Set("Retry-After", "1")`, `writeErr(w, 503,
membudget.ErrBusy)`, and a `slog.Warn("memory budget full", tenant, pattern,
weight)` at most one line per 10 s. The release runs after
`next.ServeHTTP` returns (deferred, so a handler panic releases too).

Fairness bound, accepted (D16): `budgetTotal = 4 × budgetPerTenant`, so a
tenant at its share never blocks another while at most three tenants are at
their share; with four or five tenants each at their share the rest wait, then
get 503. REQUIREMENTS.md:11 allows ≤ 5 users; the operator's numbers stay.

Order vs the slot pools: budget first (may wait ≤ 2 s → 503), then the
handler's slot (no wait → 429). The slot pools stay (they bound concurrent CPU
and disk work; the budget bounds memory).

### 2. Per-tenant slot fairness
```go
type slotPool struct{ ch chan struct{}; mu sync.Mutex; held map[string]int; perTenant int }
func newSlotPool(n, perTenant int) *slotPool
// acquireSlot takes one slot for the request's tenant or writes 429 and
// returns nil. Tenant = identity.FromContext(r.Context()).Username ("-" if empty).
func (s *Server) acquireSlot(w http.ResponseWriter, r *http.Request, p *slotPool, msg string) (release func())
```
Per-tenant share = half the pool: file 4 of 8, transcript read 4 of 8, search
2 of 4 (D4, superseded number: one person's editor alone runs read + save +
format + idle check, `FileEditor.tsx:186-252`, so a share of 2 would 429 their
own save). Over the tenant's share: 429 with "you already have N of these
running; try again when one finishes"; pool full: 429 with the existing
message. The client's existing error toast shows either; no client change. Callers: readFile, writeFile, formatFile,
checkFile (`s.fileSlots`), searchHistory (`s.searchSlots`), readHistory
(`s.readSlots`); each `defer release()`.

### 3. Admin resource cap
Config (`internal/config`): operator fields `userMaxCPUs int`,
`userMaxMemoryGiB int` (0 = "half the host", resolved at use). settings.json
keys `userMaxCpus`, `userMaxMemoryGiB` with `omitempty` (0 is written as
absent, matching `operatorOverrides`, config.go:284). Load-time validation:
negative → WARN and 0 (the existing overrides loader's style). Upper bound
checked at the API against the live host (config has no host facts).
```go
func (c *Config) UserCaps() (cpus, memoryGiB int)
type OperatorSettings struct{ DockerDiskGB int; IdleTimeout time.Duration; UserMaxCPUs, UserMaxMemoryGiB int }
func (c *Config) Operator() OperatorSettings
// UpdateOperator runs fn on a copy of the current settings under persistMu,
// validates the result (disk 5-500, idle 0 = never or >= 10m, caps >= 0), persists it
// atomically, then applies it. Replaces SetOperator(diskGB, idle); every
// field is explicit, so there is no "0 = keep" sentinel.
func (c *Config) UpdateOperator(fn func(*OperatorSettings) error) error
```
Callers of the old `SetOperator`: `updateRuntime` (explorer.go:615),
`internal/orchestrator/resources_start_test.go:23`, `resources_test.go:24`.
Effective cap (`internal/httpapi/resources.go`):
```go
// userCap returns the non-admin ceiling against host h: the configured value,
// or half the host (at least 1) when 0, never above the host maximum.
func (s *Server) userCap(h orchestrator.Host) (cpus, memoryGiB int)
```
half = `max(1, h.CPUs/2)`, `max(1, h.MaxMemoryGiB()/2)`; a configured value
above the host is clamped to the host. When `h.MaxMemoryGiB() == 0` (a host too
small to set any memory size, orchestrator/resources.go:46-57) there is no
memory cap: memory stays 0 = the host, as today, and the PUT keeps its existing
"must be 0" rule (resources.go:154-157).

Owner admin (resolved per start, so a demotion takes effect at the next start):
```go
// userstore: true when an environment admin email derives to slug.
func (s *Store) IsAdminUsername(slug string) bool
// httpapi: mirrors authGuard. True when cfg.AuthMode == "none" (every
// request on that listener is admin, auth.go:90-92 — the e2e and Tier 2
// servers), or the username is an env admin's, or cfg.DevAddr is set and the
// username is cfg.DefaultUser()'s (the dev listener's identity, also admin).
func (s *Server) ownerIsAdmin(username string) bool
```
`runOptsFor` becomes a method `(s *Server) runOptsFor(sp *userSpace, user string, p project.Project, guest string)`
(`userSpace` has no user field, server.go:48-52) and the free function is
deleted, so every caller is changed by the compiler: `ensureFor` passes its
`user`, the other four `u.Username`. Its `resourcesOf` closure reads the
record and, when `!s.ownerIsAdmin(user)`,
maps CPUs 0 or > cap → cap and MemoryGiB 0 or > cap → cap. The host comes from
`s.size.Host(ctx)` (the `sizeRuntime` seam, server.go:96-100; cached after its
first success) with `ctx = context.WithTimeout(context.Background(), 10*time.Second)`,
because `RunOpts.Resources` takes no ctx (orchestrator.go:196). The record is
NOT rewritten: lifting the cap restores the stored wish. If the host read
fails, the closure returns the error (the start fails, exactly as when the
record read fails today — never a silent uncapped start). The tests exercise
the method and its closure directly with a fake `sizeRuntime`, admin and
non-admin; the five callers (network.go:230, resources.go:234, server.go:331
via `ensureFor`, :997, :1192) are covered by the compiler.

One capping function serves start, GET and PUT:
```go
// capped returns r with the non-admin ceiling applied (identity for admins).
func (s *Server) capped(h orchestrator.Host, owner string, r orchestrator.Resources) orchestrator.Resources
```

API:
- `GET /api/projects/{id}/resources`: `limits.maxCpus` / `limits.maxMemoryGiB`
  are the cap for a non-admin owner, the host for an admin; new field
  `limits.capped bool`. The reported `cpus` and `memoryGiB` are
  `capped(record)` for a non-admin (resources.go:89 today reports the record),
  so the dialog, which fills its form from them (resources.ts:25) and refuses
  values above `limits` (:47), always opens with a valid form: an over-cap
  project shows the cap and a disk-only save succeeds (it then stores the
  capped value — D17). `pending` compares the live container against
  `WantSizing(h, capped(record), live)`.
- `PUT /api/projects/{id}/resources`: the owner is the requester (projects are
  owner-scoped, `getProjectFor`), judged by `ownerIsAdmin(u.Username)`, the
  same function the start uses. Non-admin: `cpus` 0 or 1..cap, `memoryGiB` 0
  or 1..cap, else 400 "at most N CPUs for your projects; an admin can raise
  this in Settings". 0 stays "Unlimited" = up to the cap. The rebuild decision
  (`want`, resources.go:194) and the reply's `limits` (:195) use
  `capped(h, owner, req)`, so a capped project PUT from N=cap to 0 is
  persist-only, not a rebuild.
- `GET /api/settings`: adds `userMaxCpus`, `userMaxMemoryGiB` (the configured
  values, 0 = half the host). No host read.
- `PATCH /api/settings/runtime` (admin): `dockerDiskGB` and `idleTimeout`
  keep today's shape and meaning (0 / "" = keep; the client always sends both,
  settings.tsx RuntimeEditor); adds `userMaxCpus *int`, `userMaxMemoryGiB *int`:
  absent = keep, 0 = half the host, 1..host max (host from `s.size.Host`),
  else 400. All applied through one `UpdateOperator` call.
Web: ResourcesDialog — when `limits.capped`, the Unlimited checkbox label reads
"Unlimited (up to N)". Settings → Runtime: two numeric fields in the admin's
`RuntimeEditor` ("Max CPUs per non-admin project, 0 = half the host"); nothing
new for non-admins.

### 4. Read deadline and 408
`acquireSlot` sets `SetReadDeadline` only when `r.Body != nil && r.Body !=
http.NoBody && r.ContentLength != 0`. A shared `decodeJSON(w, r, v) bool` in
`explorer.go` writes 408 "request body took too long" when the decode error
`errors.Is(err, os.ErrDeadlineExceeded)`, else 400 as today; used by
writeFile, formatFile, checkFile (the three slot holders that read a body).
Other handlers are unchanged (no read deadline is set on them). The "GET slot
holder has no read deadline" test: a GET readHistory/readFile handler kept
busy past `slotReadTimeout` on a real server still completes with 200.

### 5. Token tooling
`styrman-update`: next to the existing owner/mode checks (:567-572) and at the
build-secret path (:879), an empty token file — nothing left after
`tr -d '[:space:]'`, so a lone newline counts as empty — is an error
"token file <path> is empty", same exit path as a wrong mode. `styrman-ctl`:
a new `token_file <path>` = `private_file` plus the same whitespace-stripped empty check ("token file
<path> is empty"), used where a TOKEN file is read — `run_host_setup` (:445),
the image build secret (:510, :512) and `setup_auto_update` (:564).
`private_file` itself is unchanged (it also guards `--env-file` :472 and the
signers file :571). `setup_auto_update` already refuses an empty token
(:569); a test pins it. Doc snippet (remote.md, deploy.md): `chmod 600
~/fine-grained-token` first, then the guard uses `sudo test -s` (an empty
installed file counts as not installed).

### 6. E2E and history panel
`run.sh`: just before `exit "${status}"` (:174), when status is 0, remove
`${SHOTS}` after asserting it matches `/tmp/styrman-shots-*`, and print
"screenshots removed (green run)"; on failure keep it and print its path as
today.
History search: the search state moves into a pure controller
`web/src/lib/historySearch.ts` (the repo's tested-module pattern; no DOM test
dependency is added):
```ts
export function createHistorySearch(deps: {
  search: (q: string, signal: AbortSignal) => Promise<HistoryMatch[]>;
  onMatches: (m: HistoryMatch[] | null) => void;
  onSearching: (b: boolean) => void;
  onError: (msg: string) => void;
}): { run(q: string): Promise<void>; cancel(): void }
```
`cancel()` aborts the in-flight request, bumps the generation (a late
response is dropped), sets matches to null and searching to false. `run("")`
is `cancel()`. `SidePanelHistory.tsx`: a search still starts only on Enter
(`run(q)`, as today at :83; no search-as-you-type); an `onChange` that leaves
the box empty calls `cancel()`; the clear button renders while `searching` as
well as when `matches !== null` (today only the latter, :91-109) and calls
`cancel()`; a `useEffect` cleanup calls `cancel()` on unmount.

### 7. BUG-091
`run.mjs` identity fixture: after each `.xterm textarea` focus, before
`setIdentity` types, wait as BUG-092 does: `page.waitForFunction(() =>
document.activeElement?.classList.contains("xterm-helper-textarea"), null,
{ timeout: 5000 })`, on both the first attempt and the retry. Closed when
two full e2e runs are green AND print no `DIAG BUG-091` line (the fixture's
retry would hide a first-attempt loss). Must-fail proof, per rule
e2e-checks-leave-the-suite-as-they-found-it, with temporary uncommitted edits
to `run.mjs` (reverted with `git checkout -- web/e2e/run.mjs`, then
`git diff --stat` empty): (a) blur the terminal before the FIRST `setIdentity`
only — expect a `DIAG BUG-091` line and `failed=0` (the retry restores the
identity); (b) blur before both attempts — expect the identity check and the
checks that depend on it (commit/stash/branch/history, run.mjs:1009-1013) to
fail; the verify log names each failed check, and any failure outside that
dependency list is a second defect.

### 8. BUG-074
`deploy/caddy/ingress/config.go`: the constant becomes
```go
// CookiePrefix is this deployment's session-cookie prefix: "AUTHP" plus the
// first 8 upper-case hex digits of sha256(BaseDomain).
func CookiePrefix(baseDomain string) string
func AuthCookieName(baseDomain string) string // CookiePrefix(d) + "_ACCESS_TOKEN"
```
Portal block: `set cookie name prefix <prefix>` (caddy-security v1.1.64
`caddyfile_authn_misc.go:65-77`, which also sets the portal's token validator
cookie name). Authorization policy: `set access_token cookie name
<AuthCookieName>` (`caddyfile_authz_misc.go:80-88`). Both cookie-strip regexes
(generator.go:152, :437) strip the WHOLE portal cookie family of every
deployment, not just this one's name: `(?:^|;\s*)AUTHP[0-9A-F]*_[A-Z_]+=[^;]*`
(`const portalCookieStrip`). Reason: after the rollout a browser still holds
the legacy `AUTHP_ACCESS_TOKEN` (valid 8 h, generator.go:294, same pinned
JWT key) and, on a parent-domain deployment, that deployment's renamed cookie;
both reach this host's preview and public hosts, and a guest app could replay
them under this deployment's derivable name. Generator tests assert the legacy
name, this deployment's name and a foreign hash's name are stripped, and a
guest cookie (`app_session=1`) survives.

## Failure modes
- Budget full for 2 s: 503 "server busy, try again", `Retry-After: 1`; the web
  client shows the existing error toast. A tenant at its 256 MiB share waits /
  gets 503 while other tenants are admitted.
- One reservation larger than the tenant share: clamped, admitted alone.
- Slot share exhausted: 429 with the "you already have N of these running" text; pool full: 429
  as today.
- Host read fails at a non-admin start: the start fails with the host error
  (logged), as a record-read failure does today. Admin starts never read it for
  the cap.
- Non-admin PUT above the cap: 400 with the cap in the message; settings PATCH
  above host: 400.
- Existing non-admin project at "Unlimited" or above the cap: at its next start
  the size differs from the live container → rebuilt (home and workspace
  survive; container-only state does not — ADR-083's existing rebuild rule).
- Empty token file: `styrman-update` logs the error and skips the update tick
  (as for a wrong mode); `styrman-ctl` exits non-zero before changing anything.
- BUG-074 rollout: every browser is signed out once (the authorizer reads only
  the new name), then signs in normally; the legacy cookie is still stripped
  from guest traffic until it expires.

## Verification
Tiers: unit (Go `-race` for `membudget`, `httpapi`, `userstore`, `config`;
ingress module; vitest), Tier 2 `integration.sh`, e2e `run.sh` twice, the caddy
image build plus a `caddy adapt` of a generated config, `deploy/verify-commit.sh`.
- Budget: with the budget forced small, N concurrent heavy requests from one
  tenant get 503 after ~2 s while a second tenant's request succeeds; releasing
  admits the waiter; every registered pattern in the weight table resolves via
  `mux.Handler` (no stale entry).
- Fairness: a tenant's request beyond its share gets 429 while another tenant
  still gets a slot.
- Cap: a non-admin project at Unlimited starts at the cap size; an admin's at
  the host; non-admin PUT above the cap → 400; settings PATCH persists across a
  config reload.
- 408: a slot holder whose body stalls past the read deadline gets 408 on a real
  server; a GET slot holder has no read deadline.
- Token: an empty file makes `styrman-update` and `styrman-ctl` fail with
  "empty" (Go-driven script tests).
- BUG-074: two base domains produce different cookie names; the generated
  portal and policy carry them; `caddy adapt` in the built image accepts the
  generated config; its JSON shows the portal's `cookie_name_prefix` =
  `AUTHP<hash>` and every authorization policy's `access_token_cookie_names`
  = `["AUTHP<hash>_ACCESS_TOKEN"]` (go-authcrunch authz/config.go:84; a grep
  for the singular field would match the portal alone).
E2E criteria: golden path — the whole e2e suite green twice with no
`DIAG BUG-091` line, and no screenshot dir left after a green run. Edge — the
history panel: clearing during a search leaves the result list empty and not
"searching", even when the old response arrives later (vitest on the
controller).
Live (after deploy, by `/preflight`): dave signs in with the new cookie name.

## Phases
- [x] Cold-reader check — cap reached at round 3 (`reviews/2026-10-05-cold-read-v1-42-4-round1.md`, `-round2.md`, `-round3.md`); the residue is resolved by D20-D22, the operator's reply to the cap question TAKEN AS go (it does not name them), 2026-10-05: "What the fuck are you doing. Why cant you cut a new version/release?" (reply to the cap question offering exactly D20-D22)
### Phase 1 — server: budget, fairness, 408
- [x] ADR-086 (memory budget) appended to DECISIONS.md — verify: read — expect: entry present
- [x] `internal/membudget` + tests (wait/admit, per-tenant share, clamp, FIFO, ctx timeout → ErrBusy, release idempotent) — verify: `go test -race ./internal/membudget/` AI — expect: ok, red first
- [x] `budgetGuard` + weight table + tests (503 after wait with Retry-After; second tenant admitted; every table pattern resolves; WS route uncharged) — verify: `go test -race ./internal/httpapi/` AI — expect: ok — docs: specs/styrman/api.md
- [x] `files.List` byte bound + test — verify: `go test ./internal/files/` — expect: ok, red first — docs: specs/styrman/api.md (listing row, :166)
- [x] slotPool fairness + tests — verify: same — expect: ok — docs: specs/styrman/api.md
- [x] read deadline only with a body; `decodeJSON` 408/400 (no 413, D14) + real-server test — verify: same — expect: ok — docs: specs/styrman/api.md
- [x] Checkpoint — expect: `go test -race ./...` and `go vet ./...` exit 0 — evidence: main agent bare `go vet ./...` + `go test -race -count=1 ./...` rc=0 before 4a2b076 (D24); final `testing/2026-10-05-verify-v1.42.4-full-r4.md` PASS, 2026-10-05
### Phase 2 — admin cap
- [x] ADR-087 (admin cap, amends ADR-083) — verify: read — expect: entry present
- [x] config `OperatorSettings`/`UpdateOperator` + persistence tests; update the two orchestrator test callers — verify: `go test ./internal/config/ ./internal/orchestrator/` — expect: ok
- [x] `IsAdminUsername`, `ownerIsAdmin` (AuthMode none / env admin / dev user), `capped`, `runOptsFor` method + tests on the method and closure with a fake `sizeRuntime` — verify: `go test -race ./internal/httpapi/ ./internal/userstore/` — expect: ok
- [x] resources GET/PUT cap + settings GET/PATCH — verify: same — expect: ok — docs: specs/styrman/api.md, specs/styrman/guide.md
- [x] web: ResourcesDialog label, Settings Runtime fields — verify: `(cd web && npm run check && npm run lint && npm test)` — expect: exit 0
- [x] Checkpoint — expect: Go + web suites exit 0 — evidence: main agent bare runs rc=0 before c53396a (D24); final `testing/2026-10-05-verify-v1.42.4-full-r4.md` PASS, 2026-10-05
### Phase 3 — tooling, web, e2e, BUG-074
- [x] token-file refusals + script tests; doc snippet — verify: `go test ./internal/config/` — expect: ok, red first — docs: deploy/remote.md, specs/styrman/deploy.md
- [x] `historySearch.ts` controller + vitest; SidePanelHistory uses it — verify: `(cd web && npm run check && npm run lint && npm test)` — expect: exit 0
- [x] run.sh SHOTS removal; run.mjs BUG-091 focus wait (must-fail run per rule e2e-checks-leave-the-suite) — verify: `bash web/e2e/run.sh` twice — expect: failed=0 both, no `DIAG BUG-091`, no new `/tmp/styrman-shots-*` left — docs: testing/styrman/README.md
- [x] ADR-088 (per-deployment cookie name); ingress CookiePrefix + generator + tests — verify: `(cd deploy/caddy/ingress && go test ./...)`; caddy image build + `caddy adapt` — expect: ok; adapted JSON names the new cookie
- [x] Checkpoint — expect: verify-commit.sh, Tier 2, e2e ×2 green — evidence: `testing/2026-10-05-verify-v1.42.4-full-r4.md` PASS (Tier 2 146/0/3; e2e 101/0/2 twice, no DIAG BUG-091), 2026-10-05
### Phase 4 — Done
- [x] reviewer + security over the range; docs lens; changes entry; bugs BUG-074/091 closed
- [x] `/preflight` v1.42.4

## Risks
Assumptions:
- caddy-security v1.1.64 `set cookie name prefix` also makes the portal accept
  the renamed cookie, and the authorization policy needs its own
  `set access_token cookie name` — checked by `caddy adapt` in the built image,
  first task of the BUG-074 item.
- `mux.Handler(r)` returns the same pattern the mux will route to (stdlib
  contract) — checked by the table-resolution test.
- `h.MaxMemoryGiB()` is the host maximum the dialog shows today
  (resources.go:63) — checked by the cold read, round 1.
- A guest cookie named `AUTHP…_X` would be stripped too — accepted: the prefix
  is the portal's namespace.

## Decisions
D1 — 2026-10-05 — Scope is items 1–8 of the numbered list — by: operator "go all"
D2 — 2026-10-05 — The budget is an outer middleware with a per-pattern weight table and a 2 MiB+body default, not per-handler acquires — by: implementer (one seam reaches every route, rule mechanism-reaches-every-caller)
D3 — 2026-10-05 — Budget numbers are constants (1 GiB, 256 MiB, 2 s) — by: operator scope line 1
D4 — 2026-10-05 — Slot pools stay beside the budget; fairness is 2 per tenant per pool — by: operator scope line 2
D7 — 2026-10-05 — Per-tenant share is half of each pool (file 4, read 4, search 2) — supersedes D4's number — by: operator 2026-10-05, asked after cold read round 1 G9 (`reviews/2026-10-05-cold-read-v1-42-4-round1.md`), answered "Half of each pool (Recommended)"
D16 — 2026-10-05 — Budget stays 1 GiB / 256 MiB per tenant; the 4-tenant fairness bound is accepted and stated — by: operator's numbers (scope line 1); cold read round 2 G4 (`reviews/2026-10-05-cold-read-v1-42-4-round2.md`)
D17 — 2026-10-05 — Resources GET reports capped values to a non-admin; saving the dialog stores them — by: cold read round 2 G1
D18 — 2026-10-05 — Every charged request with a body (uploads excepted) gets the 30 s read deadline — by: cold read round 2 G4
D19 — 2026-10-05 — History search stays Enter-triggered; emptying the box and the clear button cancel — by: cold read round 2 G6
D8 — 2026-10-05 — The cookie strip removes every deployment's `AUTHP*` portal cookies, legacy name included — by: cold read round 1 G1
D9 — 2026-10-05 — Owner admin mirrors authGuard: AuthMode none, env admin, or the dev listener's default user — by: cold read round 1 G2
D10 — 2026-10-05 — One `capped()` for start, GET and PUT; the PUT's rebuild decision uses it — by: cold read round 1 G3
D11 — 2026-10-05 — `UpdateOperator(fn)` replaces `SetOperator(disk, idle)`; PATCH fields are pointers — by: cold read round 1 G4
D12 — 2026-10-05 — `files.List` gains a 2 MiB name+path byte bound; listings, history list and credential content get their own weights — by: cold read round 1 G5
D13 — 2026-10-05 — History search state moves to a pure `src/lib` controller tested by vitest; no DOM test dependency — by: cold read round 1 G8
D14 — 2026-10-05 — No 413 mapping, no `InUse()`, no effective-cap fields in GET /api/settings, no non-admin settings rows — by: cold read round 1 scope findings (not requested)
D15 — 2026-10-05 — styrman-ctl's token refusal is a `token_file` helper at the four token reads; `private_file` unchanged — by: cold read round 1 G10
D5 — 2026-10-05 — Non-admin 0 = Unlimited = up to the cap; the cap is applied by clamping at start, never by rewriting the record — by: operator scope line 3; ADR-083's "clamp, never refuse" at start
D6 — 2026-10-05 — Cookie prefix = "AUTHP" + 8 hex of sha256(BaseDomain) — by: operator scope line 8
Rejected: a configurable budget — values never vary per deployment.
Rejected: rejecting 0 for non-admins — the dialog's Unlimited default would break for every non-admin's new project.
Rejected: a stored admin flag on the user record — admin is the env list (userstore.go:110); a derived lookup cannot drift from it.
D20 — 2026-10-05 — Resources GET reports a stored 0 as 0 (the dialog shows "Unlimited (up to N)" ticked) and a stored value above the cap as the cap; PUT accepts 0; only the start maps 0 → cap — supersedes D17's 0 case — by: cold read round 3 G1, operator 2026-10-05 (cap-gate reply)
D21 — 2026-10-05 — The body read deadline is cleared once the body is fully read (a reader wrapper clears it on EOF), so it never cancels the request's context — by: cold read round 3, operator 2026-10-05
D22 — 2026-10-05 — `UpdateOperator` validates only the fields the PATCH changes — by: cold read round 3, operator 2026-10-05
D23 — 2026-10-05 — No `deadlineBody` wrapper: go1.25 net/http already clears the read deadline when the body hits EOF (`startBackgroundRead`, net/http/server.go:697); `TestTheBodyDeadlineEndsWithTheBody` pins it (a mutant that never cleared the deadline passed, which is how this was found) — supersedes D21's mechanism — by: implementer
D24 — 2026-10-05 — Phases are checkpointed by the main agent's bare `go test -race ./...` runs; one verifier dispatch covers the whole range before the gate — by: implementer, after the operator's "Why cant you cut a new version/release?" (speed)
D25 — 2026-10-05 — `UpdateOperator(OperatorUpdate)` with keep-semantics per field (disk 0, idle < 0, nil cap); `SetOperator(disk, idle)` stays as its wrapper, so no caller changes — supersedes D11's shape — by: implementer
D26 — 2026-10-05 — Unchanged runtime fields are keep in `updateRuntime` too (the page re-sends every field) — completes D22 on the shipped path — by: review `reviews/2026-10-05-review-v1.42.3-5a8afa1.md`, finding 1
D27 — 2026-10-05 — The body cap and the upload deadline exemption follow the matched route pattern, applied in `budgetGuard` (ADR-086 amendment 1) — by: security `reviews/2026-10-05-security-v1.42.4-v1.42.3-5a8afa1.md`, finding 1
D28 — 2026-10-05 — Tunnel status in csrfGuard and budgetGuard follows the matched route (`isTunnelRoute`, ADR-086 amendment 2); authGuard unchanged (fails closed) — by: security `reviews/2026-10-05-security-v1.42.4-addendum-5a8afa1-07f5517.md`, finding 1
D29 — 2026-10-05 — authGuard judges link/preview/shared by route too (ADR-086 amendment 3) — supersedes D28's "authGuard unchanged" — by: security `reviews/2026-10-05-security-v1.42.4-addendum2-07f5517-561c23b.md`, finding 1
