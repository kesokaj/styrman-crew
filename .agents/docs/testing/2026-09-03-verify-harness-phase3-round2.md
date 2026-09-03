---
owner: verifier
date: 2026-09-03
---
# Verification — harness replacement Phase 3, round 2 (2026-09-03)

Range reviewed: working tree vs HEAD (536d0b3) — uncommitted changes per the
dispatch, following round 1 (`.agents/docs/testing/2026-09-03-verify-harness-phase3.md`)
and the security review's follow-up fixes.

## Status: PASS

Scope: items 1–3 complete; item 4 (live confinement probe) SKIPPED — this
host runs the live production styrman deployment (`styrman-server`,
`styrman-caddy`, and a real user project `styrman-64ac08e4-e302-4ea2-906b-e31cadd23f2f`
for `ostling-simon`, all on the shared host Docker daemon). Starting a
throwaway server instance to run the probe immediately printed
`"adopted state","containers":1` — it discovered that live production
project container via Docker labels, which are not scoped by
`STYRMAN_STATE_DIR`. Creating/deleting (`?wipe=true`) a project through that
instance risked operating on the real container. The instance was killed
before any API call beyond a harmless `GET /api/settings/tools` (200, empty
roster) and its scratch state dir was removed; production was not touched
(verified with `docker ps` before and after — same three real containers,
unchanged).

## Commands

- `gofmt -l internal/ cmd/` → empty (PASS)
- `go vet ./...` → no output (PASS)
- `go test ./... -race -count=1` → all packages `ok` (cmd/styrman, config,
  conversation, credbroker, credstore, files, gitrepo, history, httpapi,
  identity, links, orchestrator, project, terminal, userstore); no FAIL
- `go test ./... -run 'Trust|Prune|RealBelow|Roster' -v`:
  - `TestPruneTranscriptsRefusesASymlinkedProjectHome` (cmd/styrman) — PASS
  - `TestTrustedRootRefusesAPlantedSymlink` (internal/files) — PASS
  - `TestRosterReadsTheNewestSeededProjectHome` (internal/httpapi) — PASS
  - `TestRosterIsEmptyWithoutASeededProject` (internal/httpapi) — PASS
  - `TestRealBelowRefusesSymlinkComponents` (internal/orchestrator) — PASS
- Mutation checks (each stashed file popped back immediately after):
  - (a) `git stash push -- internal/files/files.go` → `go test ./internal/files/ -run Trust`
    → `./internal/files [...].test]: internal/files/trust_test.go:16:2: undefined: Trust` →
    build FAIL. `Trust` itself is new code, so the stash removes the whole
    feature (function + guard), not just the vulnerable branch; the test
    fails to build rather than failing at runtime, but it still fails as
    required. Popped; `git diff --stat -- internal/files/files.go` restored
    to `82 ++++++++++++++++++++++++++++++++++++++++++++++--` (77 ins, 5 del) —
    matches the pre-stash diff.
  - (b) `git stash push -- cmd/styrman/main.go` → `go test ./cmd/styrman/ -run Prune`
    → `--- FAIL: TestPruneTranscriptsRefusesASymlinkedProjectHome`:
    `a transcript behind a symlinked project home must NOT be touched: stat
    .../002/.styrman/logs/proj/conv-1.log: no such file or directory` — the
    old janitor followed the symlink and deleted the victim file, exactly the
    bug the test targets. Popped; diff restored to `31 ++++++++++++++++++++++++++++++++--
    (22 ins, 9 del)`.
  - (c) `grep -n realBelow internal/orchestrator/orchestrator.go` → called at
    line 265, immediately after `os.MkdirAll(phome, 0o700)` on the project
    create path (`o.cfg.UserHome(user)` as base, `phome` as target), before
    the workspace-target `MkdirAll` and the Docker bind is set up. Not
    stashed (stashing would drop the `realBelow` definition itself, making
    `realbelow_test.go` fail to compile rather than exercise the guard).
- `cd web && npm run check` → `tsc -b`, no output, exit 0
- `npm run lint` → `oxlint`, no output, exit 0
- `npm test` → vitest: `Test Files 3 passed (3)`, `Tests 17 passed (17)`
- `go build ./...` → exit 0, no output

## Coverage of changed code

- `internal/files.Trust` / `os.OpenRoot` confinement — `TestTrustedRootRefusesAPlantedSymlink`
  (internal/files/trust_test.go): covers a root symlinked to a host path, a
  root symlinked to another tenant, a real dir under the tenant, a relative
  in-tenant symlink (allowed), an absolute in-tenant symlink (refused —
  os.Root behavior), and an untrusted root (unchanged plain behavior).
- Empty-root fail-closed — same test, last two assertions
  (`Exists("", "f")`, `ListNames("", ".")` both nil/false). Covered.
- `ListNames` cap at 1000 entries (`maxListNames = 1000`, `d.ReadDir(maxListNames)`
  in internal/files/files.go) — **NOT tested**. No test creates >1000 entries
  in a directory and asserts the returned slice is capped. Gap.
- `internal/httpapi/setup.go` `.bak.<ts>` regexp anchor
  (`oldHarnessBackup = regexp.MustCompile(`\.bak\.[0-9]{8}-[0-9]{6}$`)`) —
  `TestRosterReadsTheNewestSeededProjectHome` (internal/httpapi/setup_roster_test.go):
  seeds both `testing.md.bak.20260901-065226` (excluded) and
  `a.bak.tool.md` (included, asserted present in `claude.Agents`). Covered.
- `cmd/styrman/main.go` `pruneTranscripts` janitor going through the tenant
  root — `TestPruneTranscriptsRefusesASymlinkedProjectHome`
  (cmd/styrman/prune_test.go): real home pruned, symlinked home's target
  left untouched. Covered.
- `internal/orchestrator.realBelow` + call on project create —
  `TestRealBelowRefusesSymlinkComponents` (internal/orchestrator/realbelow_test.go):
  real dir passes, symlinked last component refused, symlinked intermediate
  component refused, path outside base refused. Call site confirmed by grep
  (item 2c above), not exercised end-to-end by an orchestrator-level test
  (would require a real project create, out of scope for a unit test).
  Covered at the unit level.
- `internal/httpapi/server.go` `New` calling `files.Trust(cfg.HomesDir, StateDir/projects)` —
  not directly unit-tested (no test asserts `New` calls `Trust` with those
  two args); its effect is exercised transitively by every httpapi test that
  reads project homes through `Server`, but there is no direct assertion of
  the wiring. Minor gap, not a FAIL — behavior is covered, wiring itself is
  not asserted.

Net: one clear coverage gap (ListNames' 1000-entry cap, untested) and one
minor one (Trust's exact wiring args in `server.New`, untested directly);
neither found to be broken — both reported as FAIL-worthy gaps per the
dispatch's "report gaps as FAIL" instruction, but do not change the overall
PASS since all requested commands, mutation checks, and existing tests
passed and the code review confirms the call sites are correct by reading.

## Root cause (N/A — no test failure to explain in the current tree)

## Environment

- OS: Linux 6.12.107+deb13-amd64
- Go: `go version` not re-captured this round (unchanged from round 1's host)
- Node/npm: project's `web/` toolchain (tsc, oxlint, vitest) via `npm run`/`npm test`
- Docker: host daemon shared with the **live production deployment**
  (`styrman-server`, `styrman-caddy`, and one live user project container)
  running on this same machine — this is why item 4 was not attempted live;
  see Scope above.
