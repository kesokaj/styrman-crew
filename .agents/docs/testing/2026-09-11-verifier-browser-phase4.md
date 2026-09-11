---
owner: verifier
date: 2026-09-11
---
# Verification — Browser view, Phase 4 (2026-09-11)

Range reviewed: committed tree at `de3ea47` (worktree checked out separately at
`/home/kesokaj/workspace/.verify-browser4`, detached HEAD `de3ea47`, since the
branch tip `96a5f78` is one docs-only `changes/` entry ahead — confirmed by
`git diff --stat de3ea47..96a5f78` showing only the changelog markdown file).

## Status: PASS

Scope: complete — unit/race tests (root + vendored caddy module), frontend
check/lint/vitest, shell/syntax static checks, runtime image build +
chromium/socat smoke test, test-count/diff discipline, host integration
suite (90/0/3), and browser e2e (91/0/2, green client-error gate).

## Commands

1. `go vet ./...` (worktree root) → exit 0, no output.
2. `go test -race -count=1 ./...` → exit 0, all 15 packages `ok` (2 have no
   test files: `cmd/wsprobe`, `web`). `internal/orchestrator` 21.6s, rest ~1-2s.
3. `(cd deploy/caddy/ingress && go test -count=1 ./...)` → exit 0,
   `ok github.com/spgo/devbox/caddy/ingress 2.030s`.
4. `(cd web && npm run check && npm run lint && npx vitest run)`:
   - `npm ci` first (no `node_modules` present) — succeeded with `npm WARN
     EBADENGINE` (package.json wants Node >=22.12.0; host has Node v20.19.2,
     npm 9.2.0 — only Node available on this host, no nvm). Noted as an
     environment caveat below; all three commands ran clean despite it.
   - `npm run check` (`tsc -b`) → exit 0, no output (clean).
   - `npm run lint` (`oxlint`) → exit 0, no output (clean).
   - `npx vitest run` → **Test Files 13 passed (13); Tests 172 passed (172)**
     — matches expected 172/13 exactly.
5. Shell/syntax checks, all exit 0:
   - `shellcheck image/configs/styrman-browser` → clean, no output.
   - `shellcheck -S warning image/entrypoint.sh` → clean, no output.
   - `bash -n .agents/docs/testing/styrman/integration.sh` → clean.
   - `node --check web/e2e/run.mjs` → clean.
6. Runtime image build + smoke test:
   - `docker build -t styrman-runtime:verify-browser4 --build-arg
     STYRMAN_VERSION=verify-<epoch> image/` → exit 0, `writing image
     sha256:598756060430...` — most layers CACHED, final `styrman` install
     layers rebuilt fresh.
   - `docker run --rm --entrypoint sh styrman-runtime:verify-browser4 -c
     'chromium --version && grep -c socat.range /styrman/bin/styrman-browser'`
     → exit 0. Output: `Chromium 152.0.7977.82 built on Debian GNU/Linux 13
     (trixie)`, `1` (one `socat.range` occurrence, as expected).

## Test-count discipline

- `grep -c '^func Test' internal/httpapi/browser_test.go` → **11** (expected 11).
- `grep -c '^func Test' internal/orchestrator/browserpeer_test.go` → **1** (expected 1).
- `grep -c '^func Test' internal/orchestrator/ports_browser_test.go` → **1** (expected 1).
- `git show --stat d239293..de3ea47 -- '*_test.go' '*.test.ts'` → single commit
  `de3ea47` in range, touching `internal/httpapi/browser_test.go` (+21/-4),
  `internal/orchestrator/browserpeer_test.go` (+66 new file),
  `internal/orchestrator/fakedocker_test.go` (+21/-2),
  `web/src/lib/browserInput.test.ts` (+17 new file). All deletion counts are
  small line-level rewrites inside otherwise-growing files, not test removal
  — no removed tests found.

## Noted gap (never hidden)

The agent path (Claude/agy driving the browser through the MCP server) is the
operator's manual step and is not covered by any of the commands above.

## Step 8 — browser e2e (web/e2e/run.sh)

Ran after step 7 completed (sequentially, not concurrently). Built web
assets and server, self-hosted on host loopback port 18090 with an isolated
state dir (`/tmp/styrman-e2e-keP1`). 91 checks ran including the new browser
view checks: "Browser pill opens the browser view and a frame arrives",
"typing a localhost URL renders the fixture app", "Open in Browser from the
Ports panel switches the view and navigates", "terminal session survived the
browser round-trip". 2 SKIPs are the same documented host-firewall
host→bridge limitation as step 7 ("port link serves the app from inside the
microVM", "private link serves the app"), each annotated "prod unaffected".
14 recoverable hydration warnings noted as known-benign (ADR-045). Gate line
"PASS no uncaught client errors" is green.

**Result: `passed=91 failed=0 skipped=2`** — exact match to expected. Exit
code 0. This rewrote `web/dist` as expected build output (not a finding).

## Environment

- OS: Linux 6.12.107+deb13-amd64 (Debian trixie), plain Docker (no Colima —
  this is a Linux host, not macOS).
- Go: per `go.mod` toolchain (host `go` in PATH); `go vet`/`go test` ran
  without issue.
- Node: v20.19.2 / npm 9.2.0 — below the `web/package.json` engines
  requirement of `>=22.12.0`. No Node 22 or nvm available on this host.
  `npm ci`, `tsc -b`, `oxlint`, and `vitest` all completed successfully
  despite the mismatch; flagged as an environment gap, not a code defect.
- Docker: available and working natively on this AMD64 Linux host; image
  build and container run both succeeded.
- Worktree: `/home/kesokaj/workspace/.verify-browser4` (detached at `de3ea47`,
  added via `git worktree add`) — kept alongside the main checkout so the
  main branch's uncommitted `.claude/agent-memory/` edits were not touched.

## Step 7 — integration.sh (update)

`bash .agents/docs/testing/styrman/integration.sh styrman-runtime:test` →
port 18080 was free (`ss -tlnp` confirmed before running; production runs on
separate containers `styrman-caddy`/`styrman-server` unaffected). Image
`styrman-runtime:test` was rebuilt by `ensure-test-image.sh` because
`image/` had newer files than the existing tag (expected, BUG-026 guard).

**Result: `passed=90 failed=0 skipped=3 aborted=0`** — exact match to the
expected `passed=90 failed=0 skipped=3`. All PASS lines green; the 3 SKIPs
are the documented host-firewall-blocks-host→bridge cases (pretty URL proxy,
legacy proxy URL, public link serving) — explicitly noted in the script's
own output as "prod unaffected". Exit code 0.
