---
owner: verifier
date: 2026-09-11
---
# Verification — Browser View, Phases 1–3 (d239293)

Range reviewed: 0e543e3..d239293 (single commit, working tree matched HEAD=d239293 at run start; two files — `.agents/docs/specs/styrman/guide.md` and `web/e2e/run.mjs` — were locally modified by a concurrent e2e session per the caller's note and were ignored/untouched here)

## Status: PASS

Scope: complete — all 7 requested steps ran, in the requested order, step 7 after step 6, none concurrently with `web/e2e/run.sh`.

## Commands

1. `go vet ./...` → exit 0, no output.
2. `go test -race -count=1 ./...` → exit 0, all 15 packages `ok` (2 have no test files: `cmd/wsprobe`, `web`); `internal/orchestrator` 21.6s, rest ~1s each.
3. `(cd deploy/caddy/ingress && go test -count=1 ./...)` → exit 0, `ok github.com/spgo/devbox/caddy/ingress 2.026s`.
4. Frontend, `web/`:
   - `npm run check` (tsc -b) → exit 0.
   - `npm run lint` (oxlint) → exit 0.
   - `npx vitest run` → exit 0. **Test Files 13 passed (13), Tests 171 passed (171)** — matches the 171/13 expected in `.agents/docs/testing/2026-09-11-verify-browser-phase3.md`.
5. `shellcheck image/configs/styrman-browser` → exit 0, no findings.
   `shellcheck -S warning image/entrypoint.sh` → exit 0, no findings.
6. `docker build -t styrman-runtime:verify-browser3 --build-arg STYRMAN_VERSION=verify-<ts> image/` → exit 0 (host is plain Linux Docker, no Colima involved — `colima: command not found`, expected on this box, build proceeded natively).
   `docker run --rm --entrypoint sh styrman-runtime:verify-browser3 -c 'chromium --version && grep -c ensure_forwarder /styrman/bin/styrman-browser && test -s /styrman/npm-global.tree.json'` → exit 0:
   - `Chromium 152.0.7977.82 built on Debian GNU/Linux 13 (trixie)`
   - `grep -c ensure_forwarder` → `3`
   - `npm-global.tree.json` non-empty → `TREE_OK`
7. `bash .agents/docs/testing/styrman/integration.sh styrman-runtime:test` (tagged from the image built in step 6) → **`passed=90 failed=0 skipped=3 aborted=0`**, matching the expected 90/0/3 (93 checks) for this firewall-hardened host. The 3 skips are the known host-firewall-blocks-host→bridge preview-proxy cases, explicitly noted in the script's own output as "prod unaffected". Ran after step 6, not concurrently, ~1 minute wall time (well under the ~10 min estimate on this box).

## Test-count discipline

- `grep -c '^func Test' internal/httpapi/browser_test.go` → **11** (expected 11, match).
- `grep -c '^func Test' internal/orchestrator/ports_browser_test.go` → **1** (expected 1, match).
- `grep -c "it("` `web/src/lib/cdp.test.ts` → **6** (expected 6, match); vitest's own per-file run confirms 13 files / 171 tests total, consistent.
- `grep -c "it("` `web/src/lib/browserInput.test.ts` → **11** (expected 11, match).
- `git show --stat 0e543e3..d239293 -- '*_test.go' '*.test.ts'` → the range is a single commit (d239293); diff touches `internal/httpapi/browser_test.go` (+126), `web/src/lib/api.encoding.test.ts` (+21/-3), `web/src/lib/browserInput.test.ts` (+115, new file), `web/src/lib/cdp.test.ts` (+106, new file), `web/src/lib/editorPane.test.ts` (+43). Total 408 insertions, 3 deletions — the only deletions are 3 lines inside `api.encoding.test.ts` (import-line rewrites), no test bodies removed. No BUG-005-style silent test loss.

## Coverage of changed code

- `internal/httpapi/browser.go` (owner-only browser routes, CDP relay) — covered by `internal/httpapi/browser_test.go` (11 test funcs) and exercised live by integration.sh's "browser (ADR-068)" section.
- `internal/orchestrator` browser-port handling — covered by `internal/orchestrator/ports_browser_test.go` and integration.sh's "ports list omits the browser's 9229" / "preview proxy refuses the browser port with a 400" checks.
- `web/src/lib/cdp.ts`, `web/src/lib/browserInput.ts` — covered by `cdp.test.ts` (6 tests) and `browserInput.test.ts` (11 tests).
- `image/configs/styrman-browser` — shellcheck clean; `ensure_forwarder` presence confirmed in the built image; behavior exercised live by integration.sh (start/status/stop routes, MCP registration for claude and agy).
- `image/entrypoint.sh` changes — shellcheck (warning level) clean.

## Noted gaps (explicit, not hidden)

- **Live relay end-to-end** (browser pane in the actual web UI driving CDP screencast + input through the WS relay in a real browser) is **not** covered by this run. It is covered only by `web/e2e/run.sh` (running concurrently in another session, out of scope here per the caller's instruction) and by a hand-walk log referenced in the phase-3 plan/spec.
- **The agent path** — `claude`/`agy` actually driving the browser via the MCP tool inside a live session — is a manual operator step, not exercised by any automated suite here or in integration.sh (integration.sh only confirms MCP *registration*, not an agent actually invoking it).
- Preview-proxy checks for the browser port are exercised via the 400-refusal path only (host firewall skips the 2 general preview-proxy PASS/SKIP cases, as expected and unrelated to the browser feature).

## Environment

- OS: Linux 6.12.107+deb13-amd64 (Debian, plain Docker — no Colima on this host; Docker used natively).
- `go version`: not captured explicitly but `go vet`/`go test` ran clean against the module's declared toolchain.
- Node/npm: project-declared versions via `web/package.json` (not modified).
- Docker: native `docker build`/`docker run`, no buildx cross-platform step requested/needed (`docker build .` default arch matched host amd64, image is AMD64-only per AGENTS.md).
- Integration suite ran against real Kata (`mvm` runtime), port 18080, state dir `/tmp/styrman-itest-sI0I`.
