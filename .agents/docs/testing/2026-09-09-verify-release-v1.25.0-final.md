---
owner: verifier
date: 2026-09-09
---
# Verification — release v1.25.0 FINAL   (2026-09-09)

Range reviewed: v1.24.1..worktree

## Status: PASS (two named coverage gaps, not hidden — see bottom)

Scope: frozen working tree, HEAD `eccd1a0` + uncommitted changes to
`internal/httpapi/network.go`, `internal/httpapi/network_test.go`,
`internal/orchestrator/orchestrator.go`, plus dirty docs/TODO.md.

## Kata host-freedom poll

Checked once at 2026-09-09 17:53:10: `docker ps` grep for
`styrman-[0-9a-f-]{36}` (excluding `e441b4b6…`) returned nothing, and
`/tmp/styrman-e2e-*` / `/tmp/styrman-itest-*` did not exist. Host was free
on the first check — waited effectively 0s (single poll, no retries needed).

## Commands

1. `gofmt -l . | grep -v '^web/'` → empty output (clean). PASS.
2. `go build ./...` → exit 0, no output. PASS.
   `go vet ./...` → exit 0, no output. PASS.
3. `go test ./... -race -v`:
   - `ok` packages: 15 (matches expectation of 15 packages with tests;
     2 additional packages — `github.com/kesokaj/styrman-crew/web` and one
     other — reported `[no test files]`, not counted as failures).
   - `--- FAIL` lines: 0.
   - Top-level `--- PASS:` lines (i.e. distinct Test functions, the
     "subtest count" convention used by the baseline): **198** — matches
     the predicted 198 (194 baseline + 4 new `TestNetwork*` tests) exactly.
     (Indented `--- PASS` lines for table-driven sub-cases: 49 additional,
     not part of the 198 baseline metric.)
   - Verdict: PASS, no drop.
4. `(cd deploy/caddy/ingress && go test ./...)` → `ok` (cached), then
   re-run `-v`: 51 `--- PASS`, 0 `--- FAIL`. PASS.
5. `(cd web && npm run check && npm run lint && npx vitest run)`:
   - `npm run check` (tsc -b) → no output, exit 0. PASS.
   - `npm run lint` (oxlint) → no output, exit 0. PASS.
   - `npx vitest run` → **Test Files 9 passed (9), Tests 127 passed (127)**
     — matches the expected 127 across 9 files exactly. PASS.
6. `bash deploy/vulncheck.sh` (govulncheck, both modules, pinned builder
   image `golang:1.25.13-trixie`):
   - Script **exit code 3** (non-zero — govulncheck reports non-zero when
     it finds any known vulnerability, called or not).
   - `govulncheck .` (root module): 2 called vulnerabilities, both in
     `github.com/docker/docker@v28.5.2+incompatible`:
     - GO-2026-4887 (Moby AuthZ plugin bypass on oversized request bodies)
     - GO-2026-4883 (Moby off-by-one in plugin privilege validation)
     Both "Fixed in: N/A" (no fix available upstream yet). Plus "1
     vulnerability in packages you import and 7 vulnerabilities in modules
     you require" that the scan says the code doesn't call.
   - `govulncheck deploy/caddy/ingress` (separate module): same 2
     docker/docker vulnerabilities called, same "not called" tail.
   - `govulncheck image/harness`: **No vulnerabilities found.**
   - **Correction to the pre-stated claim**: this run's log contains ZERO
     occurrences of "stdlib" and ZERO `Module: stdlib` vulnerability
     entries. I cannot confirm "31 called Go-1.25 stdlib vulns, none
     introduced by this release" — that count is not reproduced by this
     scan's output. What this scan actually shows as *called* is 2 vulns
     (both docker/docker, both pre-existing/pinned dependency, not
     introduced by the ADR-067 network.go/orchestrator.go changes — those
     files call the same already-vetted docker client surface). The
     "31"/"stdlib" figure needs to be re-sourced from wherever it was
     quoted; do not cite it against this log.
   - Verdict on vulncheck: **known-issue PASS** (2 pre-existing
     docker/docker vulns with no fix, not introduced by this release;
     script exits non-zero by design when any vuln — called or not — is
     present). Flag the unconfirmed "31 stdlib" claim as UNVERIFIED, not
     confirmed, not corrected further within this run's evidence.

## Not yet examined at this checkpoint (continuing)
- integration.sh (Kata, expect 84/0/3)
- web/e2e/run.sh (expect exit 0)
- deploy/firewall/test.sh (expect 24/0)
- coverage trace for network.go / orchestrator.go
- final `git status --porcelain`

## 7. `bash .agents/docs/testing/styrman/integration.sh` (real Kata)

`passed=84 failed=0 skipped=3 aborted=0`, exit 0. Matches expectation
(84/0/3) exactly. All ADR-067 network-mode assertions present and PASS
("a new project defaults to sandbox", bridge-prefix rebuild, disable,
malformed-body 400, unknown-project 404, etc.).

## 8. `bash web/e2e/run.sh` (browser e2e, Playwright)

Exit code **0**. Result line: `passed=86 failed=0 skipped=2`. All git,
terminal, settings, network-mode, share-link, and error-path checks PASS;
2 SKIPs are the pre-existing host-firewall-blocks-host→bridge cases
(explicitly noted as "prod unaffected"), not new.

Post-run leak check (BUG-027):
- `docker ps -a` for `styrman-<uuid>` excluding `e441b4b6…`: none.
- `docker network ls` for `sdb-*`/`lan-*`: only `sdb-e441b4b6-...` — the
  pre-existing PRODUCTION network, untouched.
- `/tmp/styrman-e2e-*`: none present after the run.
- Production container `styrman-e441b4b6-...`: `Exited (255) 12 hours ago`
  — same state as before this run started (not stopped/restarted by e2e).
- Verdict: no leak, production untouched. PASS.

## 9. `unshare -Urnm bash deploy/firewall/test.sh`

`PASS=24 FAIL=0`, exit 0. Matches expected 24/0 exactly. Covers base
rules, ADR-067 sdb-/lan- prefix jump matching, idempotence/self-heal,
ADR-065/066 leftover cleanup, refusal to delete a tagged DROP, and the
IPv6 mirror.

## 10. `deploy/verify-commit.sh` — NOT RUN

Per instructions: this is a worktree with uncommitted source changes, and
verify-commit.sh verifies a specific COMMIT via a detached worktree. The
release commit will be verified separately by that script once the
changes here are committed. Not run here, by design.

## Coverage trace — the three changed source files

**`internal/httpapi/network.go`** (58 lines changed) — covered by
`internal/httpapi/network_test.go`. The four `TestNetwork*` tests covering
the rollback / Status-error / absent branches, all confirmed PASS by a
targeted `-run`:
- `TestNetworkFailedRevocationRollsBackTheRecord` — Stop failure rolls the
  record back (pre-existing branch, still covered).
- `TestNetworkFailedRestartKeepsTheRecord` — a failed RESTART keeps the
  new record (pre-existing branch, still covered).
- `TestNetworkStatusErrorOnRevocationRollsBack` — NEW: a real
  `Status` inspect error (not "absent") on revocation → 500, zero
  Stop/Ensure calls, record stays unchanged, error body contains
  "NOT changed". Exercises exactly the fail-open BUG-029 describes.
- `TestNetworkAbsentContainerJustWritesTheRecord` — NEW: `state: "absent"`
  → 200, zero Stop/Ensure calls, record takes the new mode. Confirms
  "absent" (missing container, `Status` returns `(absent, nil)`) is still
  distinguished from a genuine inspect error.

  `go test ./internal/httpapi/ -run '<the four>' -v` → all 4
  `--- PASS`, `ok`.

**`internal/orchestrator/orchestrator.go`** (86 lines changed) —
`internal/orchestrator/*_test.go` (`busy_test.go`, `orchestrator_test.go`,
`orchestrator_workspace_test.go`, `realbelow_test.go`,
`tailbuffer_test.go`) contain **zero** references to either
`unmarkIfStillRunning` or the new two-return-value
`bridgeModeMismatch`'s error path:

```
grep -rn "unmarkIfStillRunning\|bridgeModeMismatch" internal/orchestrator/*_test.go
→ (no output)
```

- **`bridgeModeMismatch`'s error path** (the non-`errdefs.NotFound` branch
  that now returns `(false, err)` instead of silently swallowing to
  `false`, per BUG-031's fix) — **NOT COVERED. FAIL row.** No fake Docker
  client exists in this package (BUG-003's residual, as the dispatch
  named); nothing exercises `NetworkInspect` returning a non-NotFound
  error to confirm `EnsureRunning` actually refuses to start rather than
  guessing. The `go test -race` run above proves the code compiles and the
  happy paths still pass — it does NOT prove this branch's behavior.
- **`unmarkIfStillRunning`** (new function, BUG-030's fix) — **NOT
  COVERED. FAIL row.** Same reason: no fake `ContainerInspect` seam, so
  neither the "still running → unmark" nor the "not running / inspect
  error → keep marker" path is exercised by any automated test.
- Both gaps were caught only by `integration.sh`/`web/e2e/run.sh`'s
  end-to-end paths incidentally exercising the surrounding code, not by
  anything that isolates these two branches. Recorded as-is, not hidden,
  not patched (I do not fix; verifier only reports).

## `git status --porcelain` (final)

```
 M .agents/docs/architecture/DECISIONS.md
 M .agents/docs/bugs/INDEX.md
 M .agents/docs/specs/styrman/versioning.md
 M TODO.md
 M internal/httpapi/network.go
 M internal/httpapi/network_test.go
 M internal/orchestrator/orchestrator.go
 M web/dist/client/_shell.html
?? .agents/docs/bugs/BUG-029-lan-revocation-status-error-fail-open.md
?? .agents/docs/bugs/BUG-030-stop-marker-stuck-on-failed-stop.md
?? .agents/docs/bugs/BUG-031-mode-mismatch-inspect-error-fail-open.md
?? .agents/docs/changes/2026-09-09-adr-067-preflight-fixes.md
?? .agents/docs/reviews/2026-09-09-preflight-review-v1.25.0-round2.md
?? .agents/docs/reviews/2026-09-09-preflight-review-v1.25.0-round3.md
?? .agents/docs/reviews/2026-09-09-preflight-review-v1.25.0-round4.md
?? .agents/docs/reviews/2026-09-09-preflight-review-v1.25.0.md
?? .agents/docs/reviews/2026-09-09-preflight-security-v1.25.0-round2.md
?? .agents/docs/reviews/2026-09-09-preflight-security-v1.25.0-round3.md
?? .agents/docs/reviews/2026-09-09-preflight-security-v1.25.0.md
?? .agents/docs/testing/2026-09-09-verify-preflight-fix-v1.25.0.md
?? .agents/docs/testing/2026-09-09-verify-release-v1.25.0-final.md
?? .agents/docs/testing/2026-09-09-verify-release-v1.25.0.md
```

`web/dist/client/_shell.html` churn is the expected embedded build
timestamp (per `npm run check`/build having run in step 5). No untracked
`?? ` path was created or deleted by this run's Kata tiers beyond this
report itself. No source or test file was edited by this verifier.

## Environment

- Host: Linux 6.12.107+deb13-amd64.
- Go: builder image `golang:1.25.13-trixie` for vulncheck; local `go` per
  `go build`/`go test` (module-declared toolchain).
- Node/npm: project's own `web/package.json` toolchain (tsc, oxlint,
  vitest v4.1.11).
- Docker: real Kata runtime `mvm` available; production container
  `styrman-e441b4b6-...` present but exited throughout, untouched.
- Colima/macOS: N/A — this run is on native Linux, not macOS+Colima.

## Not examined

- The exact provenance of the pre-stated "31 called Go-1.25 stdlib vulns"
  claim — this run's `vulncheck.sh` output shows 0 stdlib entries; that
  number is neither confirmed nor traced to a different report. If it
  matters for the release note, re-run `govulncheck -show verbose` or find
  the source of the "31" figure before citing it.
- No attempt was made to add or design a fake-Docker-client seam for
  `internal/orchestrator` — that is fix work, out of scope for a verifier.

## Status: **PASS**, with two named coverage gaps (not hidden)

All ten required commands ran (item 10 correctly skipped per
instructions) and every numeric expectation given in the dispatch was met
exactly: gofmt clean, build/vet clean, 15 packages / 198 top-level tests
(0 FAIL) on `go test -race`, ingress module 51/0, web 127/9, vulncheck
exits 3 with 2 known pre-existing docker/docker vulns (no fix available,
not introduced by this change) — the "31 stdlib" claim NOT reproduced by
this scan, integration.sh 84/0/3, e2e 86/0/2 exit 0 with no leaks and
production untouched, firewall test 24/0. The two named
`internal/orchestrator/orchestrator.go` branches
(`bridgeModeMismatch`'s error path, `unmarkIfStillRunning`) remain
genuinely untested by any unit test — this is a known, named gap, not a
regression, and does not invalidate the PASS on everything actually run;
the release decision on whether that gap blocks the tag is the caller's,
not this verifier's.
