---
owner: verifier
date: 2026-09-13
---
# Verification — browser review fixes   (2026-09-13)

Range reviewed: 61608ca..c4128f5

## Status: PASS

Scope: unit/vet/build tiers, vendored Caddy ingress module, frontend
check/lint/test, shell syntax + shellcheck, and coverage mapping all ran
directly, bare, exit status read before any further step. Integration and
e2e tiers were NOT run by me (Docker/Kata self-hosting, per instruction) —
their logs, already produced by the main thread today, were read and their
result lines are cited below as evidence, not as something I executed.

## Commands

- `go build ./...` → exit 0, no output.
- `go vet ./...` → exit 0, no output.
- `go test -race ./...` → exit 0. All 16 packages `ok`. Per-package
  top-level test counts (re-run `-v` per package, `--- PASS: Test<Name> ` /
  `--- FAIL` at top level only, excluding subtests):
  - `internal/httpapi`: exit 0, 130 top-level PASS, 0 FAIL.
  - `internal/orchestrator`: exit 0, 29 top-level PASS, 0 FAIL (21.666s,
    includes the port-scan fake-exec tests).
  - `internal/terminal`: exit 0, 7 top-level PASS, 0 FAIL.
  - `internal/config`: exit 0, 29 top-level PASS, 0 FAIL.
  - Named new tests, all found and passing:
    `TestBrowserRelayClosesOnRevocation`, `TestBrowserRelayOversizeUpstreamFrameIs1009`,
    `TestCloseReasonFitsAndKeepsRunes` (all `internal/httpapi/browser_test.go`);
    `TestLauncherPinsPortsAndCaps`, `TestLauncherPeerRange`,
    `TestLauncherKillGroupFile` (`internal/httpapi/browser_launcher_test.go`, new file);
    `TestSplitPortScan`, `TestParseDockerPorts`, `TestPortScanScriptEmitsSentinel`
    (`internal/orchestrator/ports_browser_test.go`); the new `--dry-run`
    ordering case inside `TestStyrmanCtlStatusJSON`
    (`internal/config/styrman_ctl_test.go`, diff verified — asserts
    `--dry-run status --json` and `--dry-run --help` both work with the
    global flag before the verb).
- `(cd deploy/caddy/ingress && go test ./...)` → exit 0, `ok` (cached from
  an unmodified module — this range does not touch it).
- `(cd web && npm run check)` → exit 0 (`tsc -b`, no output = clean).
- `(cd web && npm run lint)` → exit 0 (`oxlint`, no output = clean).
- `(cd web && npm test)` → exit 0. `vitest run`: **13 test files, 171 tests,
  all passed.** `web/src/lib/browserInput.test.ts` line 99 confirmed:
  `"inserts an AltGr (Ctrl+Alt) character — that is typing, not a chord"`,
  asserting `isChord` false for ctrl+alt, true for ctrl-only/meta, false for
  alt-only.
- `bash -n image/configs/styrman-browser deploy/styrman-ctl .agents/docs/testing/styrman/integration.sh` → exit 0, no output (all three parse clean).
- `shellcheck image/configs/styrman-browser` → exit 0, no output.
- `shellcheck -S warning deploy/styrman-ctl .agents/docs/testing/styrman/integration.sh` → exit 0, no output.

## Integration and e2e tiers — read, not run, by me

Per instruction, these self-host on Docker/Kata; the MAIN THREAD ran them
earlier today and I only read the resulting logs.

- `.agents/scratch/integration-2026-09-13.log` (mtime 09:22 today):
  final line `passed=90 failed=0 skipped=3 aborted=0`, `rc=0`. Browser
  section (`=== browser (ADR-068: ...) ===`) shows six PASS lines: "browser
  MCP server registered for claude and agy", "styrman-browser start answers
  /json/version with the sandbox on", "ports list omits the browser's 9229
  while it listens", "preview proxy refuses the browser port with a 400",
  "browser status/start/stop routes answer for the owner (9222 up,
  hidden)", "styrman-browser stop ends it (status reports not running)" —
  matches the expected count and content exactly.
- `.agents/scratch/e2e-2026-09-13.log` (mtime 09:27 today): final line
  `passed=92 failed=0 skipped=2`, `rc=0`, including `PASS switching to the
  terminal and back resumes the screencast` — matches expected exactly.

## Coverage of changed code (`git diff --stat 61608ca..c4128f5`, excluding
docs/plans and built `web/dist/` assets)

- `internal/httpapi/browser.go` — `internal/httpapi/browser_test.go` (relay
  revocation-close, 1009 oversize-frame, close-reason truncation) and
  `internal/httpapi/browser_launcher_test.go` (launcher pinning, peer
  range, kill-group file). Covered.
- `internal/httpapi/browser_launcher_test.go` (new file) — self;
  `TestLauncherPinsPortsAndCaps`/`TestLauncherPeerRange`/
  `TestLauncherKillGroupFile` all present and passing. Covered.
- `internal/httpapi/browser_test.go` — itself the test file; net +2 funcs
  (11→13, see count below). Covered by its own execution.
- `internal/httpapi/server.go` — new `stillAuthorized`/`browserReauthEvery`
  fields wired into the browser relay are exercised by
  `TestBrowserRelayClosesOnRevocation`. Covered.
- `internal/orchestrator/ports.go` — `internal/orchestrator/ports_browser_test.go`
  (`TestSplitPortScan`, `TestParseDockerPorts`, `TestPortScanScriptEmitsSentinel`).
  Covered.
- `internal/orchestrator/ports_browser_test.go` — self, net +3 funcs (1→4). Covered.
- `internal/terminal/bridge.go` — diff is only the `revocationWatch` →
  `RevocationWatch` export rename plus call-site updates; still exercised
  by `internal/terminal/reauth_test.go`'s three `TestRevocationWatch*`
  tests (unchanged pass/fail behavior, net 3→3 funcs, calls updated to the
  new exported name). Covered.
- `internal/config/styrman_ctl_test.go` — self; net funcs unchanged (15→15,
  the new case is a sub-check ADDED INSIDE the existing
  `TestStyrmanCtlStatusJSON`, not a new top-level func — confirmed by
  reading the diff, not just the count). Covered.
- `deploy/styrman-ctl` — exercised end-to-end (real script invoked via
  `exec.Command`) by `internal/config/styrman_ctl_test.go`. Covered.
- `image/configs/styrman-browser` — no Go test invokes this script by
  name; coverage is `bash -n` + `shellcheck` (both clean, above) plus the
  integration tier's real in-guest invocation (six browser PASS lines,
  cited above). No unit-level coverage — acceptable per the project's
  stated testing model (shell scripts are integration-tested, not
  unit-tested), but noted explicitly.
- `.agents/docs/testing/styrman/integration.sh` — the test script itself;
  its own run (read above) is its coverage.
- `web/src/lib/browserInput.ts` — `web/src/lib/browserInput.test.ts` (new
  AltGr/`isChord` case, line 99). Covered.
- `web/src/lib/browserInput.test.ts` — self, new case added (+11 lines),
  confirmed present and passing (171/171 vitest).
- `web/src/lib/api.ts` — `browserStatus`/`browserStop` client wrappers
  removed (server routes still exist for the operator API + integration
  tier per the new comment); `web/src/lib/api.encoding.test.ts` updated in
  the matching direction (its two rows for `browserStatus`/`browserStop`
  removed, not left dangling). `browserStart`'s encoding row remains and
  passes. `ApiError` export change is type-only, no direct test, low risk.
  Covered for the touched surface.
- `web/src/lib/api.encoding.test.ts` — self; two rows removed to track the
  two removed api.ts exports (a real code-path removal, not a stray
  deletion — confirmed by reading the diff against api.ts's export
  removal).
- `web/src/components/BrowserPane.tsx` (343-line diff) — **no unit test
  file for this component.** Coverage is e2e-only:
  `web/e2e/run.mjs`'s new "switching to the terminal and back resumes the
  screencast" check plus the existing browser-pane checks in the same
  suite (all read as PASS above). No unit-level coverage — reported, not a
  gap I am hiding.
- `web/src/routes/p.$project.$term.tsx` (50-line diff) — same: no route
  unit test, e2e-only coverage via the same run.
- `AGENTS.md` — docs-only, no test needed.
- `web/dist/client/**` — built output, not source; regenerated by `npm run
  check`/build path implicitly, not separately tested.

## Test-count drift (before/after, no `git stash` used)

Go (`grep -c '^func Test'` on `git show 61608ca:<path>` vs HEAD):
- `internal/config/styrman_ctl_test.go`: 15 → 15 (new case added inside an
  existing func, not a new func).
- `internal/httpapi/browser_launcher_test.go`: 0 → 3 (new file).
- `internal/httpapi/browser_test.go`: 11 → 13 (net +2; diff shows only
  additions of `TestBrowserRelayClosesOnRevocation` and
  `TestBrowserRelayOversizeUpstreamFrameIs1009`, no removals — confirmed by
  `git diff --...  | grep '^-func\|^+func'`, which shows only two `+func`
  lines and no `-func` for a test).
- `internal/orchestrator/ports_browser_test.go`: 1 → 4 (net +3, the three
  named new tests).
- `internal/terminal/reauth_test.go`: 3 → 3 (rename-only diff, no test
  added or removed).

vitest: 171 tests at HEAD (13 files) vs the count stated in
`.agents/docs/testing/styrman/README.md` ("147 tests across eleven
files"). **Drift confirmed: 171 actual vs 147 stated, and 13 files vs 11
stated** — the README is out of date for this range and was not updated as
part of it. Reporting only, not fixing (per instruction).

## Environment

OS: Linux 6.12.107+deb13-amd64. Go: repo module (see `go.mod`, not
independently queried beyond `go build`/`go vet`/`go test` all succeeding).
Node/npm: whatever `web/`'s installed toolchain resolves to (`vitest
v4.1.11` observed in test output). shellcheck 0.10.0. Docker/Colima not
invoked by me for this run — integration/e2e were run by the main thread
per instruction.
