---
owner: verifier
date: 2026-09-08
---
# Verification — whole-project audit (2026-09-08)

Range reviewed: HEAD `414be72` ("handoff: v1.24.0 shipped and live; the
feature has never been used"), working tree clean against it. Note: HEAD
advanced from `af99c97` (v1.23.0 handoff, the commit named in this session's
initial git-status snapshot) to `414be72` (v1.24.0, ADR-066 LAN parity) while
this audit was running — other agents landed 21 commits of real code
(`git log af99c97..414be72` — LAN-parity phases 1–6, a release gate, and the
post-release/handoff records) during the session. All test runs recorded
below (`go test`, `npm test`, `deploy/firewall/test.sh`,
`integration.sh styrman-runtime:1.24.0`) were executed against the tree as it
stood at HEAD `414be72` — the `styrman-runtime:1.24.0` image tag used for
integration testing corroborates this (it would not exist yet at
`af99c97`). No stale-tree risk: `git status --short` was clean (only new
untracked audit-artifact files from this session) at every check.

## Status: PASS (suites) / FAIL lines below (coverage gaps)

Scope: ran all five commanded suites to completion; browser e2e (Tier 4)
deliberately NOT run per instruction (last green 89/0/2 stands, unverified
here). Coverage map covers `internal/`, `cmd/`, `deploy/firewall/`,
`deploy/update/`, `deploy/caddy/ingress/`, `web/src/` at file granularity
with `go tool cover -func` spot checks for ambiguous cases; not every
function in every file was traced.

## Commands

1. `go build ./... && go vet ./... && go test ./... -count=1` → build clean,
   vet clean, **all 17 root packages PASS** (`cmd/styrman`, `internal/config`,
   `conversation`, `credbroker`, `credstore`, `files`, `gitrepo`, `history`,
   `httpapi`, `identity`, `links`, `netpolicy`, `orchestrator`, `project`,
   `terminal`, `userstore`; `cmd/wsprobe` and `web` report "no test files").
   `internal/netpolicy` (which embeds `TestFirewallScriptInNetns`) printed
   `passed=125 failed=0` — matches README's "125 checks since ADR-066".
2. `(cd deploy/caddy/ingress && go test ./...)` → **PASS**, all listed tests
   green (module `github.com/spgo/devbox/caddy/ingress`, separate `go.mod`
   confirmed not covered by #1).
3. `(cd web && npm run check && npm run lint && npm test)`:
   - `npm run check` (tsc -b) → clean, no output, exit 0.
   - `npm run lint` (oxlint) → clean, no output, exit 0.
   - `npm test` (vitest run) → **`Test Files 8 passed (8)`, `Tests 85 passed
     (85)`** — matches README's "85 tests across eight files" exactly.
4. `unshare -Urnm bash deploy/firewall/test.sh` → **`passed=125 failed=0`**,
   exit 0. All 25 numbered sections (env-unset base rules through the
   install.sh ordering + syntax/shellcheck checks) PASS.
5. `bash .agents/docs/testing/styrman/integration.sh styrman-runtime:1.24.0`
   — image existed locally (`docker images` confirmed
   `styrman-runtime:1.24.0`), used as instructed, no fallback to `:1.23.0`
   needed. → **`passed=79 failed=0 skipped=4 aborted=0`**, exit 0. The 4
   skips are the documented host-firewall-blocked proxy/link checks and the
   opt-in live-LAN block (`STYRMAN_IT_LIVE_LAN` unset) — expected on this
   host per the script's own comments.

   **Count discrepancy vs README — RESOLVED, README is wrong.** README §Tier
   2 (line 9) states "82 checks; 4 auto-SKIP". The actual observed run this
   session is **79 PASS + 4 SKIP = 83 total**, counted two ways: the
   script's own printed summary (`passed=79 failed=0 skipped=4 aborted=0`)
   and an independent `grep -c` of `PASS`/`SKIP` lines in the captured log
   (79 and 4 respectively, same numbers). Both agree with each other and
   disagree with the README's 82. I did not find a corresponding "83" or
   updated count anywhere in README (checked the Tier 2 paragraph and its
   surrounding ADR-065/066 prose — it describes checks qualitatively, not by
   a running total that was bumped). Given the script is the executable
   source of truth and the run is internally consistent and reproducible,
   **the README's "82 checks" line is stale by one check** — most likely a
   check was added during the ADR-066 LAN-parity work (the same commits that
   moved HEAD from `af99c97` to `414be72` during this session touched
   `deploy/firewall/test.sh` by +157 lines, and `integration.sh` itself
   sits right next to that work) and the doc line was not bumped alongside
   it. I did not identify which single check is the delta (would require
   diffing `integration.sh`'s `ok`/`skip` call sites against an
   `af99c97`-era checkout) — that specific line-level identification is
   listed under Not examined. This is a doc-accuracy finding to log and fix
   in README, not a suite failure: nothing failed, both counts are
   internally self-consistent, only the two disagree.

## Coverage of changed code

No working-tree diff against HEAD exists (clean checkout) — nothing to map
to "changed code" in the usual sense for THIS session's own edits (this
session wrote only new doc/report files, no source). The deliverable
instead is the full coverage map below, ordered by risk within each FAIL
list, covering the whole tree as instructed.

### Go — `internal/`, `cmd/` (per-package `go test -cover`)

| package | coverage | note |
|---|---|---|
| `internal/netpolicy` | 94.7% | firewall script + status parsing, thorough |
| `internal/gitrepo` | 93.8% | |
| `internal/credbroker` | 92.4% | |
| `internal/identity` | 80.0% | |
| `internal/files` | 79.3% | |
| `internal/credstore` | 78.6% | |
| `internal/links` | 77.6% | |
| `internal/config` | 75.8% | |
| `internal/userstore` | 69.9% | |
| `internal/project` | 66.5% | |
| `internal/conversation` | 63.3% | |
| `internal/history` | 60.9% | |
| `internal/httpapi` | 51.2% | |
| `cmd/styrman` | 39.2% | |
| `internal/orchestrator` | 9.7% | Docker-dependent; unit tests exercise only pure logic (`busy_test.go`, `tailbuffer_test.go`, `realbelow_test.go`, `orchestrator_workspace_test.go`); the rest runs against real Docker only in Tier 2 |
| `internal/terminal` | 8.5% | same shape: `autostart_test.go`/`close_test.go`/`setuptool_test.go` cover pure decision logic; the WS/tmux bridge itself is Tier 2 + Tier 4 only |

Low % in `orchestrator`/`terminal` is by design per the testing README ("no
mocks in the orchestrator path") — most of those 0.0%-per-function lines
(`EnsureRunning`, `create`, `Destroy`, `Stop`, `NewBridge`, `ServeHTTP`,
`serve`, ports listing, etc.) ARE exercised by
`integration.sh`'s full-lifecycle run, just not by `go test`. Flagged
separately below only where NEITHER tier reaches the function.

### FAIL — no test coverage in ANY tier (unit, integration, or e2e), ordered by risk

0. **`deploy/update/styrman-update`** (695 lines, root-run, no sibling
   `test.sh`). Confirmed by filesystem: `deploy/update/` holds only
   `styrman-update`, `styrman-update.service`, `styrman-update.timer` — no
   `test.sh`, `*_test.*`, or `bats`/shellcheck harness anywhere in the
   directory, unlike `deploy/firewall/` whose `styrman-firewall.sh` has
   `test.sh` running 125 automated checks under `unshare` for exactly the
   same "root script, no default state paths" shape. `styrman-update`
   pulls/verifies version tags, rebuilds both images, health-checks, rolls
   back on failure, and self-installs its own binary/unit files onto the
   host from a verified git object — the highest-consequence untested
   surface in the repo (it can deploy AND roll back production with no
   automated check of either path). README documents an elaborate MANUAL
   procedure for exercising `self_install()` (source specific functions
   into a scratch script, run by hand as root against a scratch git repo —
   `.agents/docs/testing/styrman/README.md` lines ~165–210) but this is a
   documented-by-hand procedure, not a script anyone or any CI runs; there
   is no evidence in this session that it was actually run recently (no
   dated log entry found for it). This now ranks above the httpapi gaps
   below: it is root-privileged, deploy/rollback-capable, and has zero
   automated coverage versus `deploy/firewall`'s 125-check sibling.
1. **`internal/httpapi/stats.go`** (`projectStats`, `parseStats`,
   `projectLogs` — all 0.0% unit; no `stats_test.go`; `integration.sh` never
   hits a stats/logs API route, only `docker logs` directly). Parses
   cgroup/docker stats output and serves project logs to the browser —
   parsing untrusted/variable-format container output with no test.
2. **`internal/httpapi/projectenv.go`** (`listProjectEnv`, `putProjectEnv`,
   `projectEnvValue`, `deleteProjectEnv` — all 0.0% unit, no test file,
   `integration.sh` never exercises `/env`). Read/write path for
   per-project environment variables reaching the guest; untested CRUD
   surface.
3. **`internal/httpapi/proxy.go`**: `listPorts`, `proxyPort`,
   `proxyPrefix` at 0.0% unit. `integration.sh`'s "ports detected" check
   passes and exercises SOME of this file (`serveProxy` 40%, `proxyFor`
   69.8%, `stripCookieDomains` 66.7%), but the two preview-URL-proxying
   checks that would reach `proxyPort` are SKIPped on this host (host
   firewall blocks host→bridge) — so `listPorts`/`proxyPort`/`proxyPrefix`
   specifically have no confirmed exercise on THIS run. Production traffic
   does hit this path; re-verify on a host where those two checks don't
   skip, or note explicitly this run leaves it dark.
4. **`internal/config/config.go`**: `SetupContainer`, `LegacyWorkspaceExists`,
   `ProjectWorkspace`, `UserStateDir`, `NetPolicyDir`, `DefaultUser`,
   `ContainerName` — 0.0% unit (no direct test), but these are path-builder
   helpers almost certainly exercised indirectly through the live server
   `integration.sh` drives (legacy-migration, per-user dirs, container
   naming all appear as PASS lines there) — lower risk than 1–3 because the
   failure mode (wrong path/name) would very likely surface as an
   integration FAIL rather than pass silently. Listed for completeness, not
   equal risk to 1–3.
5. **`cmd/styrman/main.go`**: `main` and `transcriptJanitor` at 0.0% unit —
   expected for `main` (integration.sh runs the real built binary, so it IS
   exercised end-to-end, just not under `go test`); `transcriptJanitor`'s
   background sweep loop is not directly asserted by either tier as far as
   this audit traced — lowest risk of this group, background janitor logic.

### Known-uncovered, already named by the dispatch (confirmed present, not re-argued)

- `deploy/firewall/install.sh` — structural checks only (`bash -n`,
  shellcheck, `--dry-run`, function-ordering check inside `test.sh`'s §24);
  real run is a manual operator install per the README.
- `web/src/components/LocalNetwork.tsx` — e2e-only (ADR-065/066 Tier 4
  checks), confirmed no `.test.tsx`/unit reference exists for it.
- `orchestrator.BridgeNameForNetwork`'s Docker path — confirmed 0.0% in the
  cover profile; it IS listed as a function orchestrator.go carries, but its
  Docker-network-lookup branch is unexercised by either tier here.
- `config.IngressNetwork` — not found as a distinct top-level identifier in
  `config.go`'s func list this run (may be a field/const, not a func); did
  not chase further given budget — treat the dispatch's claim as
  authoritative, not independently re-confirmed.

### `deploy/caddy/ingress/` (separate module)

All exported entry points have direct tests: `generator.go` ↔
`generator_test.go`, `labels.go` ↔ `labels_test.go`, `cloudflare.go` ↔
`cloudflare_test.go`, `loopback.go` ↔ `loopback_test.go`, `watcher.go` ↔
`watcher_test.go` + `watcher_ip_test.go`. `module.go` (Caddy plugin
registration/glue) and `cmd.go` (CLI entry) have no direct test file — did
not run `go tool cover -func` on this module to confirm function-level gaps
given budget; flagged as **not examined at function granularity**, not as a
confirmed FAIL.

### `web/src/`

Per README, only `web/src/lib/*.ts` has a unit-test framework (vitest, added
2026-09-02); ALL of `web/src/components/*.tsx` and `web/src/routes/*.tsx`
have zero `.test.tsx` files — the project's own doc already states this
("most TypeScript/React logic is still covered only by
type-checking+lint plus, where applicable, Tier 4 e2e"). Restating each of
the ~24 component files as an individual FAIL line would just repeat that
one sentence 24 times; instead:

**`web/src/lib/*.ts` files with NO test file and NO test in this run's
scope** (tsc/lint cover only types/style, not behavior) — ordered by risk:

- `lib/api.ts` (882 lines) — PARTIALLY covered: `api.test.ts` covers
  `projectNameFromRepoURL`/`cloneURLError`/`projectNameError`;
  `api.session.test.ts` covers `request()`/`uploadFiles` session-expiry
  handling. The bulk of the file (every other API call wrapper) has no
  direct unit test — only e2e/manual reaches it. FAIL for the untested
  remainder given the file's size and that it is the sole client↔server
  contract surface.
- `lib/terminal.ts` (433 lines) — no test file at all. WS/PTY client logic;
  same risk class as the untested server-side `terminal/bridge.go`
  functions above — both halves of the same channel are dark to unit tests.
- `lib/hooks.ts` (334 lines) — no test file.
- `lib/projects.ts`, `lib/clips.ts`, `lib/fonts.ts`, `lib/toast.ts` — no
  test files, smaller/lower-risk.

**Every file under `web/src/components/` and `web/src/routes/`** (24
component files + 5 route files + `router.tsx`) — no `.test.tsx` anywhere in
the tree; type-check + lint only, confirmed by `find ... -name "*.test.tsx"`
returning nothing. Not re-listing each by name per the note above; treat as
one FAIL class: "React component/route logic, zero unit coverage,
type-check+e2e only."

## Environment

- OS: Linux 6.12.107+deb13-amd64 (this container/session).
- Go: `go build`/`go vet`/`go test` ran against whatever `go` is on PATH
  (not the pinned builder image — that pin is `deploy/vulncheck.sh`'s
  concern, not `go test`'s; not independently re-verified against
  `deploy/Dockerfile.server`'s pinned version this run).
- Node: v20.19.2, npm 9.2.0.
- Docker: `styrman-runtime` images present locally through `:1.24.0`;
  `unshare -Urnm` available and used for the firewall test without root.
- `deploy/vulncheck.sh` (Setup Commands step "Security") was **NOT run**
  this session — not in the dispatch's five commanded steps; see Not
  examined.

## Not examined

- `bash deploy/vulncheck.sh` — govulncheck security scan, not in the
  dispatch's command list; not run. README says expected exit 3 (accepted
  `docker/docker` baseline) — unverified this session.
- `web/e2e/run.sh` — explicitly excluded by the dispatch (and by the
  coordinator's follow-up); last known green 89/0/2 stands unverified.
- `deploy/verify-commit.sh` — release-gate script, out of scope for a
  working-tree audit (no tag/release in play).
- `deploy/update/styrman-update`'s documented manual `self_install()`
  procedure — not executed this session (it requires a scratch root-owned
  git repo + systemd stub); confirmed only that no AUTOMATED script for it
  exists, not whether the manual procedure was ever run and when.
- Function-level `go tool cover -func` was run for `orchestrator`,
  `terminal`, `httpapi`, `cmd/styrman`, and `config` only. NOT run for
  `conversation`, `credbroker`, `credstore`, `files`, `gitrepo`, `history`,
  `identity`, `links`, `netpolicy`, `project`, `userstore` — their
  package-level percentages (60–95%) were treated as adequate signal given
  budget; a handful of low-level helper functions inside those packages
  could still be individually uncovered without this audit catching it.
- `deploy/caddy/ingress/module.go` and `cmd.go` — named above as
  not-function-traced; no `go tool cover -func` was run for that module.
- Did not independently verify `config.IngressNetwork`'s uncovered status
  (could not locate it as a func in a quick grep — likely a field/const);
  took the dispatch's claim as given.
- Did not identify which specific check(s) `integration.sh` gained since
  README's "82 checks" line was last true (established the 83-vs-82
  disagreement and that README is stale, but not a line-level diff of the
  script against an `af99c97`-era copy to name the added check).
- No git diff of THIS session's own edits exists to review as "changed
  code" beyond the new doc files it wrote — this was a full-project static
  audit, not a change-scoped verification of a single feature branch.
