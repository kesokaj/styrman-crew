---
owner: main
date: 2026-09-11
---

# Browser view — Phase 2: server side and review findings (2026-09-11)

Phase 2 of 4: server-side launch routes, browser relay, port filtering, and all fixes for review findings from Phase 1 commit e7f7857.

## Summary

**Verification at completion:** `go vet + go test -race ./...` green; ingress module green; new browser_test.go (7 tests) and ports_browser_test.go added; shellcheck clean; docker build green; two Kata probes passed; integration test `passed=90 failed=0 skipped=3` (twice, before and after the peer restriction). Boot→ready 2.72 s.

## The code

**config.go:** Three constants: `BrowserPort = 9222`, `BrowserChromePort = 9229` (guest-side loopback), `CloseBrowserUnavailable = 4004` (pane must not auto-reconnect on launch failure; shows reason + Retry button).

**internal/httpapi/browser.go (new, 320 lines):** Routes are owner-only via `getProjectFor` (non-owner → 404). The three launcher verbs run through the existing `guestRuntime` seam, which is testable without Docker via `s.browserUpstreamFn`:

- `GET /api/projects/{id}/browser` — status probe (200): returns `{"running":bool}` or `{"running":false}` if project is not running.
- `POST /api/projects/{id}/browser/start` — launcher: calls Status first to confirm the project is running, then `docker exec <container> /styrman/bin/styrman-browser start` as the guest user with HOME set. Returns 409 if not running; 503 + Retry-After: 1 on a fresh bridge attach (socat not yet listening); 502 with the launcher's log tail (max 120 bytes, cut on rune boundary) if launch fails. Guest-side errors logged with command name and exit status.
- `POST /api/projects/{id}/browser/stop` — returns 204 on success or 409 if not running. Always idempotent.
- `GET /ws/browser/{id}` — CDP relay (WebSocket upgrade). Plain-HTTP failures (404/502/503) before the upgrade attempt. On a stopped project, closes 4002. On launch failure (reason passed from start), closes 4004. Upstream endpoint discovered from the guest's `/json/version` and re-hosted to the guest bridge IP (not loopback). Upstream dial sends NO Origin header (Chromium rejects any Origin). Text frames pumped both ways; read limits 16 MiB guest-side / 1 MiB pane-side. Guest dropping → pane closed 1011 "browser disconnected".

**previewPortAllowed refusal:** `previewPortAllowed` now refuses 9222 and 9229 with 400 in `proxyPort` (the port-preview proxy) and in `sharePort`/`getShares`/`setShares`/`createLink` (prevent listing or sharing the browser ports).

**internal/orchestrator/ports.go:** Pure parsing extracted into `parseListeningPorts(log, ports)` which drops both browser ports from the list shown in the Ports panel.

**Tests:** `internal/httpapi/browser_test.go` (7 tests, 350 lines): fake DevTools server (httptest, responds to `/json/version` and WebSocket upgrades), tests for start/stop/status/relay, origin rejection (a fake refuses any Origin header; test sends one and expects 403), empty-ports handling. `internal/orchestrator/ports_browser_test.go`: port filtering (9222/9229 dropped).

**Integration checks (3 new):** Ports list omits 9222/9229 while the browser listens; `/proxy/{id}/9222/` and the share PUT answer 400; owner's status/start/stop round-trip passes. Existing agy check tightened: jq on command name + disabled==false (reviewer low finding). In-guest start check now curls 127.0.0.1:9229 (ChromeDevTools MCP loopback). README: 93 checks defined, 90 green + 3 auto-skip on this host.

**Docs:** `api.md` "Browser (ADR-068)" section; `terminal-protocol.md` "Browser relay" section + 4004 close code row; `runtime-image.md` Browser section updated; plan phase 2 ticked.

## Defects found during Phase 1 reviews and fixed in Phase 2

All findings from `.agents/docs/reviews/2026-09-11-review-browser-phase1.md` (REQUEST CHANGES: 1 high + 1 low) and `.agents/docs/reviews/2026-09-11-security-browser-phase1.md` (FINDINGS: 2 high, 3 medium, 3 low) addressed:

**HIGH / security#1 & reviewer HIGH:** Chromium launched with `--remote-allow-origins=*` → any page rendered in the browser could open its own DevTools socket. **Fixed:** removed the flag entirely. Chromium admits a WebSocket upgrade from a client that sends NO Origin header (the Go relay, and chrome-devtools-mcp) and refuses every Origin (`http://localhost:9222`, `http://evil.example`, etc.) with 403. Verified under Kata: no Origin → 101 Switching Protocols; any Origin → 403. Relay test's fake DevTools server refuses any Origin, so a regression fails the test.

**HIGH / security#2:** Phase 1 left port 9222 previewable, shareable, and publicly linkable (anyone with a link could read the home via `file://` URLs in the browser). **Fixed:** `previewPortAllowed` now refuses 9222 and 9229; `parseListeningPorts` filters both from the Ports panel; tests + integration checks confirm. Reviewer's note that image and server ship separately is acknowledged: versioning.md states a release carries all three images at one tag (no staggered shipments).

**MEDIUM#3:** An inner container the user runs could reach 9222 on the bridge and read the home. **Fixed:** socat now runs with `range=<peer>/32,max-children=4` where the peer is the server's own bridge address. Parsed from STYRMAN_CRED_BROKER in a containerized deploy, else the default gateway (loopback dev only); with neither, no forwarder runs. In-guest clients (the MCP verb) use Chromium's loopback port 9229 directly. Verified under Kata: env unset → host (gateway) 200; foreign STYRMAN_CRED_BROKER → host refused; loopback works.

**MEDIUM#4:** Operator and agents share one Chromium profile/cookies — prompt injection on a tested page could read an operator's signed-in session. **Accepted risk** (ADR-068 D1, operator-approved design). Phase 4 guide will say "do not sign in to anything in the Browser pane you would not hand the agent"; a second profile or target allowlist is a noted follow-up.

**MEDIUM#5:** npm transitive tree unpinned, lifecycle scripts as root at build. **Mitigated:** `npm install -g --ignore-scripts` (package needs none; `--help` proves it) and resolved global tree recorded at `/styrman/npm-global.tree.json` for diffing. Lockfile-based install is a follow-up.

**LOW#6:** Launcher creates paths by name under the guest-writable home, follows symlinks. **Fixed:** `refuse_links` rejects a symlinked state dir, profile, log, pid or lock file (same uid, self-inflicted, but `: > LOG` through a link would truncate transcripts the server reads).

**LOW#7:** socat unbounded forks. **Fixed:** `max-children=4`.

**LOW#8:** Registration failures discarded, stamp withheld forever = every-boot re-seed. **Partly fixed:** entrypoint now logs the failing command's last line. Every-boot retry is the pre-existing HM/HI design (a home that breaks its own seeding is the user's); WARN says why. Retry cap is a follow-up.

**Reviewer LOW:** agy check was a substring grep. **Fixed:** jq on command name + disabled==false.

**INFO:** apt pins by archive. **Recorded** and agreed.

## Verification

**Static:** `go vet ./...` clean; `shellcheck -S warning` clean on all shell.

**Unit tests:** `go test -race ./...` green. New test files above. Ingress module `(cd deploy/caddy/ingress && go test ./...)` green.

**Docker build:** `docker build -t styrman-runtime:browser-p2 --build-arg STYRMAN_VERSION=dev-<ts> image/` green.

**Kata integration (Tier 2):** `.agents/docs/testing/2026-09-11-verify-browser-phase2.md` recorded. `bash .agents/docs/testing/styrman/integration.sh styrman-runtime:browser-p2`: `passed=90 failed=0 skipped=3` (three auto-skip on this host; the three new browser checks passed; boot→ready 2.72 s). Run twice to confirm peer restriction: before setting socat's peer range, foreign containers could reach 9222 (test expected 400 from share PUT, got 200 — found unfiltered) → fixed `socat range=<peer>/32` → foreign containers refused, share PUT → 400 as expected.

## Files changed

New:
- `internal/httpapi/browser.go` (320 lines)
- `internal/httpapi/browser_test.go` (350 lines, 7 tests)
- `internal/orchestrator/ports_browser_test.go` (test)

Modified:
- `internal/config/config.go` (3 constants)
- `internal/httpapi/httpapi.go` (four new routes + previewPortAllowed check)
- `internal/orchestrator/ports.go` (parseListeningPorts extracted, filters 9222/9229)
- `image/configs/styrman-browser` (socat range + max-children, refuse_links, logging)
- `.agents/docs/specs/styrman/api.md` (Browser section)
- `.agents/docs/specs/styrman/terminal-protocol.md` (Browser relay + 4004)
- `.agents/docs/testing/styrman/integration.sh` (+3 checks)
- `.agents/docs/testing/styrman/README.md` (counts 87→90)
- `.agents/docs/plans/2026-09-11-browser-view.md` (Phase 2 ticked)

## Agents dispatched

- **reviewer:** Phase 1 diff at commit e7f7857 (`.agents/docs/reviews/2026-09-11-review-browser-phase1.md`).
- **security:** Phase 1 diff at commit e7f7857 (`.agents/docs/reviews/2026-09-11-security-browser-phase1.md`).
- **verifier:** Phase 2 diff (builds, tests, integration Tier 2, two Kata probes).
- **changelog:** this entry.

## Requirements

R4 amended 2026-09-11 commit a9ae789 with operator approval: "A Browser view showing the project's own in-guest Chromium, shared with the agents for testing what they build, is a testing surface and not an IDE (ADR-068, operator approval 2026-09-11)."

R3 (per-project isolation): met by profile in `~/.styrman/browser/profile` per project home.

ADR-068: Browser view accepted 2026-09-11 (architecture, agent access, R4 sentence); amendment below on the DevTools port access control.
