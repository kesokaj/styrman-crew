---
owner: main
date: 2026-09-11
---

# Browser view — Phase 4: e2e checks, docs, and security round 3 fixes (2026-09-11)

Phase 4 of 4: four new e2e checks (91 total, up from 87), full documentation, security round 3 fixes for the Phase 3 review findings, and fixes for BUG-039 and BUG-040 discovered during Phase 4's integration run.

## Summary

**Verification at completion:** Static analysis clean (go vet, npm check, shellcheck). Unit and integration tests green: 172 frontend tests (cdp, browserInput, editorPane, BrowserPane, api); 4 new Go tests (BrowserPeer fail-closed logic, per-user relay cap + heartbeat, browser port link/share refusal, fake network setup). Image rebuilt and Kata-probed: peer validator refuses multi-line injection, changed peer replaces the forwarder. Tier 4 e2e solo: passed=91 failed=0 skipped=2, "no uncaught client errors" gate PASS. Not verified: agent path end-to-end (operator's manual walk, deferred to follow-up); containerized deploy (release smoke).

## The code

**web/e2e/run.mjs:** four Browser-view checks inside the LAN window (cannot reach sandbox bridge on this hardened host; executed after project rebuild with peer re-assigned):
1. "Browser pill opens the browser view and a frame arrives" — checks that clicking the Browser tab shows the pane and receives a screencast frame.
2. "typing a localhost URL renders the fixture app" — types `:3000` (a running fixture test server on the project) into the address bar; asserts page title "Browser fixture" and a new frame arrived after navigation.
3. "Open in Browser from the Ports panel switches the view and navigates" — the Ports sidebar's browser button switches to the Browser view and opens the port's URL.
4. "terminal session survived the browser round-trip" — sanity check that a terminal command still works after the browser testing. Fixture typed into terminal after awaiting prompt to avoid history-expansion (`!`) on first two runs.

**internal/httpapi/browser.go:** (Phase 3 fixes, round 3 remediation):
- Per-user relay cap (8) beside the global cap (32) with a 30 s heartbeat ping toward the pane (`relayWebSockets`) to free abandoned slots when a tab vanishes. Refusal logged at Warn level. Test: `TestBrowserRelayOriginAndCap` (global + per-user scenarios).

**internal/orchestrator/browserpeer.go:** (Phase 3 fixes, low findings):
- Fails closed when the server's own inspect fails or lacks the network (never assumes broker IP). Logs at Warn.
- Added `TestBrowserPeer` (4 cases: container self-attach success, fallback on missing network, missing container, missing inspection). Fake Docker client gains network gateway and per-container addresses.

**web/src/lib/browserInput.ts:** (Phase 3 fix, low finding):
- `normalizeUrl` accepts ONLY `http`, `https`, `about:blank` with TypeScript guard. Refuses `javascript:`, `data:`, `file:`, `view-source:`, `blob:`, `ftp:` with a notice. Test added.

**web/src/components/BrowserPane.tsx:** (Phase 3 fix, low findings):
- CDP frame/dialog/target payload shapes checked with `typeof` guards (no cast-based assumptions on unvalidated keys).
- JS dialog text cut to 200 chars; dialog prefixed with its origin (e.g., "localhost:3000 says:") and styled as page content, not styrman chrome.
- Tab strip capped (no unbounded growth from hostile pages). Screencast ack moved after decode (not before).
- Address bar notice when URL scheme is refused (one-line alert to operator).

**image/configs/styrman-browser:** (Phase 3 fix, low finding + reviewer medium):
- Peer validator is whole-string bash regex (multi-line and option-injecting values refused probe).
- Forwarder records its peer in `${DIR}/socat.range` and is replaced when peer changes (re-launch on mismatch).
- Header states the `--network=host` boundary (dev-only, accepted residual, follow-up: pipe transport).

**runtime-image.md:** "Known boundary" paragraph documenting `--network=host` reachability and the v1 trade-offs (plan follow-up: pipe transport).

**internal/httpapi/server.go:** BUG-039 fix — `listTerminals` logs and reports empty list instead of 500 when a durable annotation (browser or ports) fails; prevents sidebar blank-out during stop/rebuild.

**integration.sh:** BUG-040 fix — added `STYRMAN_ITEST_PORT` env var; refuses a bound port and errors loudly rather than silently testing the wrong server. `.agents/docs/testing/styrman/README.md` updated with the note and the new Tier 4 check count (87 → 91).

**Docs by documentation agent:**
- `guide.md` §1, §2 "Browser view", §4 "Using the browser" with the shared-profile warning (agent and operator cookies are shared; listed as accepted per ADR-068 D1) and v1 limits.
- `README.md` one-line mention.
- `ROADMAP.md` "Now" section.

## Defects found during Phase 4 and fixed

**BUG-039** (observed during integration.sh run, on line 93): GET `/api/projects/{id}/terminals` answered 500 during stop/rebuild window, blanking the sidebar terminal list. **Root cause:** `listTerminals` called `BrowserStatus()` and `ListeningPorts()` for annotation, neither failing gracefully when the guest was down. **Fix:** log and skip failed annotations; return the durable terminal list with partial info rather than 500. Rule: a listing from a durable store must never 500 on a failed best-effort annotation — degrade, keep rows, log why.

**BUG-040** (observed on e2e run 2; first run 1 failure): integration.sh hard-codes port 18080 and hangs when a local dev server occupies it, silently testing the wrong server and missing a real issue. **Root cause:** The port was fixed to match the compose deploy; a concurrent local server on the same port shadows the test. **Fix:** added `STYRMAN_ITEST_PORT` env var (default 18080); `.agents/docs/testing/styrman/integration.sh` accepts an override from the environment and refuses a bound port with error, making port collision visible. Rule: tests that depend on a network port must use a unique or over rideable default, never assume exclusivity.

**E2E history for context:** run 1 (concurrent jobs) — 4 failures: pre-existing focus-after-toggle check (BUG-038 class, load blip), two fixture browser checks (fixture never started), client-error gate (ERR_NETWORK_CHANGED + BUG-039 500). Run 2 (solo, debug) — fixture checks only: typed command included `<!doctype` and `!` triggered history expansion ("event not found"); fixture never started. Run 3 (solo, no `!`, prompt awaited) — green.

## Verification

**Static:** `npm run check` (tsc -b) clean; `npm run lint` (oxlint) clean; `go vet ./...` clean; ingress module `go vet`/`go test` clean; `shellcheck` clean.

**Unit tests:** `npm run test` (vitest) green (172 tests: new test for `normalizeUrl` refusing schemes; browserInput +1 test, cdp/BrowserPane/editorPane existing count maintained). `go test -race ./...` green (new 4 tests for BrowserPeer and relay cap). Full log: `.agents/scratch/all-tests-phase4.log`.

**Docker build:** `docker build -t styrman-runtime:test --build-arg STYRMAN_VERSION=dev-<ts> image/` green.

**Kata integration (Tier 2/4 combined):** `.agents/docs/testing/2026-09-11-verify-browser-phase4.md` recorded.
- Image start with `STYRMAN_BROWSER_PEER=<gateway>` → 9222 up, host curl `/json/version` 200; stop → closed. Peer validator refuses multi-line, option-injection. Changed peer replaces socat.
- `bash .agents/docs/testing/styrman/integration.sh`: all checks pass (87 checks, Phase 4 adds 4 browser e2e checks in the solo run above; integration.sh itself has 93 checks per Tier 2 definition).

**Tier 4 e2e (Playwright, solo):** `bash web/e2e/run.sh` solo (host quiet): `passed=91 failed=0 skipped=2`. Client-error gate: PASS (no uncaught exceptions). The four new Browser-view checks all passed: address bar navigation (fixture title check, screenshot recorded), Ports panel button, terminal survived round-trip.

**Security round 3 disposition** (per `.agents/docs/reviews/2026-09-11-security-browser-phase3.md`, 2 medium / 6 low / 3 info):
- **Medium#1 (relay cap):** Fixed with per-user bound (8) + 30 s heartbeat + refusal log.
- **Medium#2 (host-network boundary):** Accepted (v1 boundary; follow-up: pipe transport documented in plan follow-ups).
- **Low findings (6):** All fixed (URL scheme guard, CDP payload checks, dialog origin prefix, peer validator, forwarder re-launch on peer change, fallback logging).
- **Info (3):** Peer validator whole-string match, bridge gateway dev-only, stale state sweep deferred to follow-up.

## Files changed

New:
- `web/e2e/run.mjs` — four new Browser-view checks

Modified:
- `internal/httpapi/browser.go` — relayCounter (global 32 + per-user 8), heartbeat ping
- `internal/httpapi/server.go` — BUG-039 fix (listTerminals degrades failed annotations)
- `internal/orchestrator/browserpeer.go` (new file from Phase 3) — added fail-closed logging; `browserpeer_test.go` with 4 test cases
- `web/src/lib/browserInput.ts` — `normalizeUrl` rejects non-http schemes; test
- `web/src/components/BrowserPane.tsx` — CDP frame/dialog checks, origin-prefixed dialogs, tab cap, ack-after-decode
- `image/configs/styrman-browser` — whole-string peer validator, range stamp/replace on peer change
- `runtime-image.md` — "Known boundary" paragraph
- `.agents/docs/testing/styrman/integration.sh` — BUG-040 fix (port override env var)
- `.agents/docs/testing/styrman/README.md` — Tier 4 count 87→91, integration port note (BUG-040)
- `.agents/docs/specs/styrman/guide.md`, `README.md`, `ROADMAP.md` — documentation (by `documentation` agent)
- `.agents/docs/plans/2026-09-11-browser-view.md` — Phase 4 boxes ticked (except operator's manual agent walk, kept for follow-up); follow-ups listed (pipe transport, separate profile, stale sweep, lockfile, re-seed cap, idle stop)

## Agents dispatched

- **documentation:** guide.md, README.md, ROADMAP.md (cold-read for accuracy; no corrections needed).
- **reviewer:** Phase 3 diff at commit d239293 (`.agents/docs/reviews/2026-09-11-review-browser-phase3.md`; APPROVE WITH NITS: medium forwarder peer-change, low browserpeer test coverage — both fixed this phase).
- **security:** Phase 3 diff at commit d239293 (`.agents/docs/reviews/2026-09-11-security-browser-phase3.md`; FINDINGS: 2 medium, 6 low, 3 info; disposition above).
- **verifier:** Phases 1–3 integration (`.agents/docs/testing/2026-09-11-verifier-browser-phase1-3.md` PASS, 7 steps).
- **bugs:** BUG-039 (terminals list 500), BUG-040 (port collision).
- **changelog:** this entry.

## Requirements

R4 (Browser testing surface, amended 2026-09-11 with operator approval in commit a9ae789): met by SPA pane with agent-shared session in per-project Chromium profile, full user interaction capability, manual operator walk and agent MCP access available (Phase 4's last two boxes deferred to follow-up).

R3 (per-project isolation): per-project profile at `~/.styrman/browser/profile` per home; credentials isolated.

R12 (project isolation): relay enforces sameOriginOrAbsent (scheme-blind, mitigated by ingress TLS termination); no cross-project steering; peer-restricted socat (except dev-only host-mode, documented).

ADR-068 amended with round 3 findings disposition (phase-4 commit).
