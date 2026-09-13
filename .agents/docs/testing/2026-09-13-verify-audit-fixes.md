---
owner: verifier
date: 2026-09-13
---
# Verification — audit-fixes batch (2026-09-13)

Range reviewed: 7066301..e18d62c

## Status: PASS

Scope: build/vet/fmt/unit tests (both Go modules) + web tsc/lint/vitest + shellcheck on all six named scripts, run to completion. Integration and e2e tiers deliberately NOT run here (main thread's job); integration log was still mid-run and e2e log did not yet exist at time of writing, so their tiers are cited as "still running" / "not started", not PASS.

## Commands

- `go build ./...` → rc=0
- `go vet ./...` → rc=0
- `gofmt -l ./cmd ./internal` → printed nothing, rc=0
- `go test -race -count=1 ./...` → rc=0, all 15 packages `ok` (cmd/styrman, internal/{config,conversation,credbroker,credstore,files,gitrepo,history,httpapi,identity,links,orchestrator,project,terminal,userstore}); cmd/wsprobe and web have no test files (expected)
  - New tests confirmed present (via grep across `*_test.go`) and individually run with `-run` + `-race -count=1`, all PASS:
    - `TestCredentialSnippetTrustsInjectedBrokerAddress`, `TestCredentialSnippetMarker` — internal/credbroker/snippet_test.go
    - `TestEnsureRunningRecreatesTheContainerWhenItsNetworkVanished` — internal/orchestrator/seam_test.go
    - `TestLoadBoundsDockerDisk` — internal/config/config_test.go
    - `TestStyrmanUpdateVerifyTagMarkers` — internal/config/styrman_update_test.go
    - `TestWebSocketEntryPointsRefuseCrossOriginBeforeAnySideEffect`, `TestSameOriginOrAbsent` — internal/terminal/origin_test.go
    - `TestProjectEnvRoutes`, `TestProjectStatsRoute` — internal/httpapi/projectenv_stats_test.go
    - `TestFormatScriptPlacesTheTempFileBesideTheEditedFile`, `TestFormatDirConfinesToTheWorkspace` — internal/httpapi/format_test.go
- `(cd deploy/caddy/ingress && go test ./...)` → rc=0, includes `TestGenerateCaddyfile_PortalRedirectSuffixCookieAndPreviewCookieStrip` and `TestSanitizeHostnameRefusesEmpty`, both PASS, plus the full existing suite (~55 tests)
- `(cd web && npm run check)` → `tsc -b`, rc=0
- `(cd web && npm run lint)` → `oxlint`, rc=0, no findings
- `(cd web && npm test)` (bare, rc captured separately from any pipe per verify-bare rule) → rc=0; `Test Files 15 passed (15)`, `Tests 177 passed (177)`; confirmed `web/src/lib/api.timeout.test.ts` and `web/src/lib/clips.test.ts` are new files present in the run
- `bash -n` on the six scripts:
  - `deploy/update` is a directory, not a script — the actual file is `deploy/update/styrman-update` (task's shorthand path did not exist); `bash -n` on that path → rc=0
  - `deploy/host-setup.sh` → rc=0
  - `deploy/styrman-ctl` → rc=0
  - `image/configs/styrman-credentials.sh` → rc=0
  - `image/configs/profile-motd.sh` → rc=0
- `shellcheck -S warning` on the same five real paths (`deploy/update/styrman-update`, `deploy/host-setup.sh`, `deploy/styrman-ctl`, `image/configs/styrman-credentials.sh`, `image/configs/profile-motd.sh`) → rc=0 for all, no warnings

## Coverage of changed code

Mapped from `git diff --stat 7066301..e18d62c` (70 files):

- internal/credbroker/broker.go (log read faults) — covered: internal/credbroker/broker_test.go, snippet_test.go family (package tests build/exercise the git()/GetGit path; no dedicated case forces a non-ENOENT read fault — grep confirms no test asserting the new Warn log line). **Gap**: the specific `b.logger.Warn(...)` branch on a non-not-exist error is not directly exercised by name — report as a narrow FAIL line, not hidden.
- internal/credstore/credstore.go (added `os.Chmod` on pre-existing dir) — internal/credstore/credstore_test.go, projectenv_test.go exist and exercise `New()`; grep found no test asserting the chmod-on-preexisting-dir behavior specifically. **FAIL line**: chmod branch not directly covered by name.
- internal/history/history.go (regular-file parity check before/after open) — covered by internal/history/fifo_test.go `TestReadDoesNotBlockOnAPlantedFIFO` (plants a FIFO at the transcript path and calls `Read`; exercises the new Lstat/IsRegular guard path, since a FIFO is the concrete non-regular case the change is written against).
- internal/project/uuid.go (`NewUUID` now returns `(string, error)`) and internal/project/store.go (quarantine-on-ID-error path in `NewStore`, propagated error in `Add`) — internal/project/store_test.go has `TestStoreRoundTrip`, `TestSaveRecreatesMissingDir`, `TestListNewestFirst` which call `Add` (and so `NewUUID`) and pass, confirming the new two-return signature compiles and the happy path works. **No test found** (grepped `NewUUID(` across internal/project/*_test.go — zero hits) that forces `crypto/rand.Read` to fail and asserts the quarantine-to-`.rejected` behavior in `NewStore` or the propagated error from `Add`. **FAIL line**: the actual audit-fix behavior (data-loss-avoidance on rand failure) has no test forcing that path; only the pre-existing happy path is re-verified.
- internal/httpapi/auth.go (`path.Clean(r.URL.Path)` before `isLinkPath`) — grepped all `internal/httpapi/*_test.go` for `isLinkPath` / `link/\.\.` / `path.Clean` in test context: no hits beyond the source comment. **FAIL line**: no test exercises a `/link/../...` request through `authGuard` to confirm the traversal-neutral guard decision.
- internal/terminal/bridge.go (cross-origin check moved to first statement) — covered: `TestWebSocketEntryPointsRefuseCrossOriginBeforeAnySideEffect` (internal/terminal/origin_test.go), confirmed PASS above.
- internal/httpapi/format.go, format_test.go — covered by the two named new tests, confirmed PASS.
- internal/httpapi/projectenv_stats_test.go — covers the new project env/stats routes, confirmed PASS.
- internal/config/config.go (STYRMAN_DOCKER_DISK_GB bound) — covered by `TestLoadBoundsDockerDisk`, confirmed PASS.
- internal/config/styrman_update_test.go / deploy/update/styrman-update (signature marker verification) — covered by `TestStyrmanUpdateVerifyTagMarkers`, confirmed PASS; `bash -n` and shellcheck clean on the script itself.
- deploy/caddy/ingress/{generator.go,labels.go,config.go} — covered by `TestGenerateCaddyfile_PortalRedirectSuffixCookieAndPreviewCookieStrip`, `TestSanitizeHostnameRefusesEmpty`, both PASS, plus full ingress suite green.
- internal/files/files.go — **known accepted gap (per brief)**: Read's I/O-fault branch has no seam to test; reported as FAIL line per instructions, not hidden. `internal/files/files_test.go` line-count shrank (deletions only, `ContainedPath` removed) — no new test needed there since it's a deletion.
- web/src/components/SidePanelGit.tsx — **known accepted gap (per brief)**: e2e-only coverage; not exercised by vitest.
- web/src/lib/api.ts, diffReveal.ts, clips.ts — covered by api.timeout.test.ts, diffReveal.test.ts (existing, updated), clips.test.ts (new); all in the 177-passing vitest run.
- image/Dockerfile — **per brief**: the image build itself is the check. `.agents/scratch/image-rebuild-audit.log` shows the rebuild ran and completed (`ensure-test-image: styrman-runtime:test rebuilt`, `rc=0`) but the log as captured at time of writing does NOT contain the `go version` / `node --version` / `claude --version` / `agy --version` output lines the brief describes checking inside the image — either they ran in a different log/step not present in this file, or they had not yet been appended. **Report, not hidden**: cite the rebuild success but flag that the four version-check outputs were not found in `.agents/scratch/image-rebuild-audit.log` as read.
- deploy/host-setup.sh, deploy/styrman-ctl, image/configs/styrman-credentials.sh, image/configs/profile-motd.sh — `bash -n` + shellcheck clean (see Commands); no Go/vitest unit coverage applies (shell scripts), consistent with the brief's own check list for these paths.
- .agents/docs/plans/*, .agents/docs/reviews/* — docs-only, no code coverage expected.
- web/dist/client/* (hashed asset renames) — build artifacts, not independently tested; covered transitively by `npm run check`/`npm test` building against current `src/`.

## Background tiers (not run by verifier; cited only)

- `.agents/scratch/integration-audit.log`: 126 lines, 79 PASS lines counted, no FAIL seen in the read portion, but the file ends mid-run at `=== stop + delete (wipe) ===` with no closing `passed=`/`rc=` line — **still running** at time of writing.
- `.agents/scratch/e2e-audit.log`: file does not exist yet — **not started / still running**.
- `.agents/scratch/image-rebuild-audit.log`: exists, 3 lines, ends `rc=0` (image rebuild only, not the four version-check commands — see gap noted above).

## Environment

Linux 6.12.107+deb13-amd64 (host, not the Kata guest). Go/Node versions not separately queried by the verifier for the host toolchain (build/test all passed against whatever is on PATH). shellcheck 0.10.0.

## Not examined

- Integration (`integration.sh`) and e2e (`web/e2e/run.sh`) tiers were explicitly out of scope for this verifier per dispatch; their logs were read but were incomplete at write time (see Background tiers above) — re-check after they finish.
- Did not independently rebuild or inspect `image/Dockerfile`'s content diff beyond citing the rebuild log; did not run `go version`/`node --version`/`claude --version`/`agy --version` myself inside the image.
- Did not exercise the `crypto/rand` failure path for `NewUUID`, the credstore chmod-on-preexisting-dir path, the credbroker non-ENOENT log-warn path, or a literal `/link/../` HTTP request against `authGuard` — all four are named above as coverage gaps (FAIL lines), not silently passed.

---

## Extension to e6ce8c4 (2026-09-13)

Range reviewed: v1.29.0..e6ce8c4

### Status: PASS

Scope: extends the batch above to the two commits that followed it —
25bdae8 (docs, records, two new coverage tests: `TestNewTightensAPreexistingDirMode`
in internal/credstore, the cleaned-path case in `TestRouting` in internal/httpapi)
and e6ce8c4 (review-round audit fixes: format.go's exclude line now matched with
`grep -F` instead of a regex that never matched; orchestrator now removes the
stale container before recreating its network; a leftover block deleted from
credentials.go; api.timeout.test.ts gains a spy proving no `AbortSignal.any`
composite is built). Same command set as the base log, run bare per
`.agents/rules/verify-bare.md`, against the current working tree at e6ce8c4.
Integration and e2e tiers not run here by design; see Background tiers below.

### Commands

- `go build ./...` → rc=0
- `go vet ./...` → rc=0
- `gofmt -l ./cmd ./internal` → printed nothing, rc=0
- `go test -race -count=1 ./...` → rc=0, all 15 packages `ok`
  (cmd/styrman 1.0s, internal/config 5.4s, internal/conversation 1.0s,
  internal/credbroker 1.1s, internal/credstore 1.0s, internal/files 1.0s,
  internal/gitrepo 1.0s, internal/history 1.0s, internal/httpapi 1.9s,
  internal/identity 1.0s, internal/links 1.0s, internal/orchestrator 21.7s,
  internal/project 1.0s, internal/terminal 1.1s, internal/userstore 1.0s);
  cmd/wsprobe and web have no test files (expected)
  - Individually re-run with `-run` + `-race -count=1`, all PASS:
    - `TestNewTightensAPreexistingDirMode` — internal/credstore → rc=0
    - `TestRouting` — internal/httpapi (includes the cleaned-path case) → rc=0
    - `TestFormatScriptPlacesTheTempFileBesideTheEditedFile` — internal/httpapi → rc=0
    - `TestEnsureRunningRecreatesTheContainerWhenItsNetworkVanished` — internal/orchestrator → rc=0
- `(cd deploy/caddy/ingress && go test ./...)` → rc=0, `ok` (cached from
  identical source since e18d62c; no ingress files changed in 25bdae8/e6ce8c4
  per `git diff --stat`)
- `(cd web && npm run check)` → `tsc -b`, rc=0
- `(cd web && npm run lint)` → `oxlint`, rc=0, no findings
- `(cd web && npm test)` → bare, rc=0; `Test Files 15 passed (15)`,
  `Tests 177 passed (177)` (same count as the base log — e6ce8c4's
  api.timeout.test.ts change adds a spy assertion inside an existing test,
  not a new test case)
- `bash -n` on the six named scripts:
  - `deploy/update` is a directory; the real file is
    `deploy/update/styrman-update` → rc=0
  - `deploy/host-setup.sh` → rc=0
  - `deploy/styrman-ctl` → rc=0
  - `image/configs/styrman-credentials.sh` → rc=0
  - `image/configs/profile-motd.sh` → rc=0
- `shellcheck -S warning` on the same five real paths → rc=0 for all, no warnings
- `node --check web/e2e/run.mjs` → rc=0

### Coverage of changed code (25bdae8, e6ce8c4 only — base commits already mapped above)

- internal/credstore/credstore.go (chmod-on-preexisting-dir branch, flagged
  as a gap in the base log) — now covered:
  `TestNewTightensAPreexistingDirMode` (internal/credstore/credstore_test.go),
  confirmed PASS individually. Closes the base log's credstore FAIL line.
- internal/httpapi/auth.go (`path.Clean` before `isLinkPath`, flagged as a
  gap in the base log) — now covered: `TestRouting`'s cleaned-path case
  (internal/httpapi), confirmed PASS individually. Closes the base log's
  auth.go FAIL line.
- internal/httpapi/format.go (exclude line now matched with `grep -F`) —
  covered by the existing `TestFormatScriptPlacesTheTempFileBesideTheEditedFile`
  and `TestFormatDirConfinesToTheWorkspace`, both still PASS; no dedicated
  new test asserts the literal-vs-regex distinction by name, but the existing
  test exercises the same code path end to end.
- internal/orchestrator/orchestrator.go (remove-before-create ordering) —
  covered by `TestEnsureRunningRecreatesTheContainerWhenItsNetworkVanished`,
  confirmed PASS individually (same test as the base log; e6ce8c4 changes
  the implementation this test already exercises).
- internal/httpapi/credentials.go (leftover block deleted) — deletion only;
  `internal/httpapi/*_test.go` package tests still build and pass (see full
  `go test` run above); no new test needed for a pure deletion, consistent
  with `.agents/rules/writing-files.md`'s framing of a deletion diff.
- web/src/lib/api.timeout.test.ts (spy proving no `AbortSignal.any`
  composite) — covered: file is part of the 177-passing vitest run; the new
  assertion lives inside the existing timeout-test file, not a separate count.
- Docs-only changes in 25bdae8 (ADR-069, ADR-030 amendment, changelog,
  TODO, testing README, deploy README, guide) — no code coverage applies.

### Background tiers (not run by verifier; cited only)

- `.agents/scratch/integration-audit.log` (e18d62c's tree, pinned image):
  `passed=91 failed=0 skipped=3`, rc=0.
- `.agents/scratch/e2e-audit.log` (e18d62c's tree, pinned image):
  `passed=92 failed=0 skipped=2`.
- A second run on the final e6ce8c4 tree is in progress at time of writing:
  `.agents/scratch/integration-release-v1.30.0.log` (confirmed live —
  `integration.sh` process observed running under pid 1720971 at the time
  this section was written) and `.agents/scratch/e2e-release-v1.30.0.log`
  (queued to start after integration finishes, per the background shell's
  own command line). Neither log carries a final `passed=`/`rc=` line yet —
  **not cited as PASS here**; re-check both files for their closing lines
  before treating v1.30.0's release tiers as verified.

### Environment

Linux 6.12.107+deb13-amd64 (host). shellcheck 0.10.0. Same toolchain as the
base log; not re-queried.

### Not examined

- The live integration/e2e release-tier run on the e6ce8c4 tree
  (`integration-release-v1.30.0.log`, `e2e-release-v1.30.0.log`) had not
  finished by the time this extension was written — no final `passed=`/`rc=`
  lines to cite. Whoever closes the v1.30.0 gate must read those two files'
  final lines before calling the release tiers green.
- Did not independently re-verify the base-log's remaining open gaps
  (credbroker non-ENOENT log-warn path, `NewUUID`/`crypto/rand` failure
  quarantine path) — unaffected by 25bdae8/e6ce8c4, still open per the base
  log.
